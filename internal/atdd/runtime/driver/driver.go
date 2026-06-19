// Package driver wires together the ATDD pipeline runtime: it loads the
// process-flow YAML, registers gates / actions / agents, applies override and
// verify decorators, and walks the named process end to end.
//
// The driver is deliberately thin — the heavy lifting lives in the runtime
// sub-packages (statemachine, gates, actions, verify, override, tracker,
// release, clauderun). This file's job is to compose them and run the
// pipeline against a specific ticket: pre-resolve the project item for
// Options.IssueNum, seed Context, then walk the main process from START.
//
// Agent dispatch (v2): every user-task whose `agent:` value is something
// other than `human` shells out to the `claude` CLI via the clauderun
// package. clauderun reads the embedded per-agent definition (from
// internal/atdd/assets/runtime/agents/atdd/), substitutes ${name} placeholders
// from the live Context, and hands the rendered string to `claude -p` as
// the agent's full one-shot input — there is no parent-claude harness or
// Task-tool indirection. Success is detected by HEAD diff (a fresh commit
// on the same branch). v1's "pause and let the operator launch the agent
// in a second window" behaviour is preserved as a fallback under
// Options.ManualAgents — it lets us bisect "did v2 misroute the agent?"
// against "did v1 see the commit?" without two parallel binaries.
package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/process/actions"
	"github.com/optivem/gh-optivem/internal/atdd/process/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/process/configcheck"
	"github.com/optivem/gh-optivem/internal/atdd/process/gates"
	"github.com/optivem/gh-optivem/internal/atdd/process/verify"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/trace"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/factory"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/kernel/gitignore"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/userstate"

	"github.com/mattn/go-isatty"
)

// DefaultProcessName is the entry process loaded by every public CLI command.
const DefaultProcessName = "main"

// ErrPendingHuman is the sentinel a dispatcher returns when it reaches a
// `category: human` node in an unattended run (no operator TTY). Rather than
// blocking the interactive `claude` TUI on stdin nobody will answer (run #69:
// ~2h14m of dead wall-clock), the run yields: it ends here, the machine is
// freed, and Run discards the uncommitted in-progress edits so a later resume
// re-enters the human gate from the last clean committed phase with an
// operator present. It is a normal, expected outcome — NOT a failure halt — so
// the CLI maps it to ExitCodePendingHuman, distinct from success (0) and error
// (1). Recognised via errors.Is through the engine's %w-wrapped propagation.
var ErrPendingHuman = errors.New("pending human: category:human node reached with no operator TTY (unattended run) — yielded; resume with an operator present")

// ExitCodePendingHuman is the process exit code for an ErrPendingHuman yield.
// Distinct from 0 (run completed) and 1 (run failed) so a rehearsal / CI
// harness can tell "paused, awaiting a human" apart from done and crashed.
const ExitCodePendingHuman = 2

// stdinIsTTYFn reports whether stdin is an interactive terminal. A package var
// (like nowFn) so tests can force the answer without a real TTY. Mirrors the
// isatty stdout check used for trace colorisation and the stdinIsTTY helper in
// cross_repo_commands.go.
var stdinIsTTYFn = func() bool { return isatty.IsTerminal(os.Stdin.Fd()) }

// Options bundles every driver knob that callers (the `gh optivem implement`
// command and tests) might want to set. Zero values yield a usable
// configuration: load the embedded canonical YAML, enter DefaultProcessName,
// no overrides, real shell-outs.
type Options struct {
	// YAMLPath, when non-empty, points the driver at an on-disk YAML file
	// instead of the canonical embedded document (process.DefaultYAML).
	// Empty → load the embedded YAML via process.Load.
	YAMLPath string

	// ProcessName is the entry process. Empty → DefaultProcessName.
	ProcessName string

	// Target scopes the run to one pipeline slice (plan 20260530-1725). Zero
	// value (TargetUnset) is the no-arg full pipeline — Run walks ProcessName
	// exactly as today. A scoped value routes Run into the slice's named
	// sub-process via resolveScopedEntry instead (after a git-state resume
	// guard). Set by the flag layer (Item 4) from --target; SliceProcess and
	// ParseTarget in target.go are the SSoT for the mapping + validation.
	Target Target

	// Channel narrows a channel-split Target to one channel token (api / ui).
	// Required for TargetDriverAdapter / TargetSystem and rejected for
	// TargetTest — resolveScopedEntry enforces the rule and validates the token
	// against the project channels: SSoT. Empty for the no-arg full run and the
	// agnostic test slice.
	Channel string

	// IssueNum is the issue to implement. The driver pre-resolves the
	// project item for this issue before walking the main process.
	IssueNum int

	// ResolvedIssue, when non-nil, receives the tracker.Issue that
	// preResolveIssue looked up at startup. Lets the cobra layer print
	// the ticket URL in its exit banner without re-querying the tracker.
	// Optional; nil leaves existing behaviour untouched.
	ResolvedIssue *tracker.Issue

	// ProjectURL overrides config-based project resolution. Optional; when
	// empty, the driver uses cfg.Project.URL from the loaded gh-optivem.yaml
	// (or the file passed via ConfigPath, if set). Threaded into the tracker
	// adapter constructor via factory.Open.
	ProjectURL string

	// RepoPath overrides the working directory used for project resolution
	// and for shell-outs. Optional; defaults to cwd.
	RepoPath string

	// Headless flips agent dispatch into `claude -p` (one-shot, no
	// interactive UI) mode. Default (false) runs `claude` interactively
	// so the operator can observe / interject. Human-STOP semantics are
	// covered by Approval (CategoryHuman is always-implicit); Headless
	// is strictly about how the claude subprocess is invoked.
	Headless bool

	// Approval is the resolved auto-approve policy. The cobra layer reads
	// it off cmd.Context() (via cmdctx.Approval) and assigns it here so
	// every confirmation site inside the driver (humanStop, the three
	// approve dispatchers, release Commit) routes through approval.Confirm
	// with the appropriate category. Zero value (Auto=false) preserves
	// today's "always prompt" semantics, so tests that don't set Approval
	// keep working.
	Approval approval.Resolved

	// ManualAgents falls back to the v1 "pause and let the operator
	// launch the agent in a second window" behaviour at every user-task
	// dispatch. Default (false) shells out to the `claude` CLI via the
	// clauderun package. ManualAgents is mutually exclusive with the
	// override hooks (the prompt-construction layer is what consumes
	// them — bypass that and they have nothing to attach to).
	ManualAgents bool

	// Override holds the per-node override hooks (Extra / Replace).
	// Populated by the cobra layer from gh-optivem.yaml's node_extras: /
	// node_replacements: fields. nil leaves the dispatcher unmodified.
	Override *override.Hooks

	// TaskPromptOverrides is a map from embedded MID task-name (e.g.
	// "write-acceptance-tests") to a prompt body that replaces the canonical
	// embedded prompt for that task. Sourced from gh-optivem.yaml's
	// task_prompts: map by the cobra layer; the values are the file
	// contents, not the file paths (the CLI reads at startup so missing-file
	// failures surface there). Unrecognised task names are rejected at
	// projectconfig.Validate.
	TaskPromptOverrides map[string]string

	// ConfigPath is the resolved gh-optivem.yaml path. The caller (cobra
	// layer in implement_commands.go) populates it via projectconfig.ResolvePath
	// so flag > env > <cwd>/gh-optivem.yaml precedence is applied once and
	// the driver sees a single, always-non-empty path. Missing-file is a
	// hard error.
	ConfigPath string

	// LogFile, when non-empty, mirrors everything Stdout and Stderr would
	// emit during the run — clauderun banners, the driver's resolution
	// banner, and the per-node trace stream — to the named file. Wired
	// from `--log-file <path>`. Existing files are truncated. Empty
	// string disables file mirroring — output still streams to Stdout/
	// Stderr so the operator can follow live.
	//
	// The mirror is byte-for-byte: clauderun's colored banners include
	// ANSI escape sequences when stdout is a TTY, so the file will too.
	// View with `less -R`, or set `NO_COLOR=1` (the `color` package
	// honors it) to get a plain-text file.
	LogFile string

	// KeepRuns caps how many directories under <repoPath>/.gh-optivem/runs/
	// the driver retains at startup. The current run's directory is created
	// after pruning, so the post-prune count is min(N, on-disk-count) + 1.
	// Zero (the default) disables pruning — useful in tests where the runs/
	// directory is irrelevant. Negative values are rejected at the CLI;
	// the runtime treats anything <=0 as "skip pruning".
	KeepRuns int

	// ShowPrompt threads through to clauderun.Options.ShowPrompt for every
	// dispatch so `gh optivem implement --show-prompt` dumps the full
	// rendered prompt to stdout before each agent launches. Off by default
	// — the prepared-prompt summary banner is always on.
	ShowPrompt bool

	// ClaudeRunDeps lets tests inject fake `claude` and `git` runners
	// without spawning real subprocesses. Production callers leave this
	// zero-valued; clauderun falls back to real execClaude / execGit.
	ClaudeRunDeps clauderun.Deps

	// AgentSet binds the prompt directory every dispatch resolves agent
	// bodies, tuning and suffixes from. nil → agents.DefaultAgentSet() (the
	// built-in ATDD set), applied in withDefaults, so production and existing
	// tests need not set it. Binding an alternate set here rebinds the agent
	// layer for the whole run without touching process-flow.yaml — the
	// agent-axis swap point. Threaded into registerAgentDispatchers (which
	// names the dispatchers to register), the per-dispatch LoadTuning lookup,
	// and clauderun.Options.AgentSet.
	AgentSet *agents.AgentSet

	// Stdout / Stderr are the diagnostic targets. nil → os.Stdout / os.Stderr.
	// Stdout is the fallback writer when Out is nil (test paths that pre-date
	// the level-tagged sink architecture); production paths populate Out via
	// installLogFileMirror, which routes Phase-level writes to every sink and
	// Detail-level writes only to verbose sinks.
	Stdout io.Writer
	Stderr io.Writer

	// Stdin is the agent-dispatch pause reader. nil → os.Stdin.
	Stdin io.Reader

	// Out routes Fprint sites by level (Phase / Detail) to the sink set
	// (terminal + optional --log-file). Populated by installLogFileMirror at
	// startup from Stdout + LogFile + TerminalLevel + LogFileLevel. nil →
	// withDefaults builds an outlog.Default(Stdout) so legacy single-writer
	// test fixtures keep seeing every Fprint.
	Out *outlog.Out

	// TerminalLevel is the maximum level the terminal sink accepts. Defaults
	// to outlog.Phase (clean operator view). --verbose flips to outlog.Detail
	// to restore the full firehose on the terminal.
	TerminalLevel outlog.Level

	// LogFileLevel is the maximum level the --log-file sink accepts when
	// LogFile is non-empty. Defaults to outlog.Detail (forensic firehose).
	// --log-level=phase narrows the log to match the default terminal view.
	LogFileLevel outlog.Level
}

// Run loads the YAML, wires the registries, applies decorators, optionally
// pre-resolves an issue, and walks the chosen process.
//
// The return is named (runErr) so the deferred run-end tail can stamp the
// overall verdict into runState and write the human digest before the
// engine's error propagates to the cobra layer.
func Run(ctx context.Context, opts Options) (runErr error) {
	opts = opts.withDefaults()

	var eng *statemachine.Engine
	var err error
	if opts.YAMLPath == "" {
		eng, err = process.Load()
		if err != nil {
			return fmt.Errorf("driver: load embedded YAML: %w", err)
		}
	} else {
		eng, err = statemachine.LoadFile(opts.YAMLPath)
		if err != nil {
			return fmt.Errorf("driver: load YAML %q: %w", opts.YAMLPath, err)
		}
	}

	if _, ok := eng.Processes[opts.ProcessName]; !ok {
		source := opts.YAMLPath
		if source == "" {
			source = "embedded"
		}
		return fmt.Errorf("driver: process %q not in YAML %q", opts.ProcessName, source)
	}

	repoPath, err := resolveRepoPath(opts.RepoPath)
	if err != nil {
		return fmt.Errorf("driver: %w", err)
	}

	// Load config up-front so its project.url can flow into action deps —
	// without this, IMPLEMENT_TICKET and friends fall back to
	// loading a hard-coded gh-optivem.yaml filename in repoPath and miss
	// any `--config <other-name>.yaml` the operator passed.
	cfg, err := loadDriverConfig(opts.ConfigPath, repoPath)
	if err != nil {
		return err
	}

	// opts.ProjectURL (set via --project-url) wins over cfg.Project.URL
	// so an operator can override on the fly without editing config.
	resolvedProjectURL := opts.ProjectURL
	if resolvedProjectURL == "" && cfg != nil {
		resolvedProjectURL = cfg.Project.URL
	}

	// Build opts.Out before constructing dependency Deps — actions.Deps
	// reads Out.Detail when wiring realShell so subprocess output routes
	// through the level-tagged sink set. Done here (not at the original
	// post-Bind location) so the order is: build Out → construct Deps →
	// register actions → wrap agents (those each saw opts.Out via the
	// captured opts value).
	logClose, err := installLogFileMirror(&opts)
	if err != nil {
		return fmt.Errorf("driver: %w", err)
	}
	defer logClose()

	gateReg := gates.New()
	gates.RegisterAll(gateReg, gates.Deps{Approval: opts.Approval})

	actionReg := actions.New()
	actions.RegisterAll(actionReg, actions.Deps{
		Out:        opts.Out,
		Stderr:     opts.Stderr,
		ProjectURL: resolvedProjectURL,
		RepoPath:   repoPath,
		Config:     cfg,
		Engine:     eng,
	})

	agentReg := agents.New()
	registerAgentDispatchers(agentReg, opts.AgentSet)

	eng.GateFn = gateReg.Lookup
	eng.ActionFn = actionReg.Lookup
	eng.AgentFn = agentReg.Lookup

	// Statically unroll the channel loop in change-system-behavior from the
	// project-declared `channels:` (plan 20260530-1702 Item 4). The channel
	// set is project-dependent, so this in-memory rewrite happens per run
	// from config rather than in the shared static YAML. Done before Bind so
	// the synthesized per-channel call-activity nodes get NodeFns resolved.
	// Absent/empty channels: skip the unroll and keep the single static node
	// (today's `suite: acceptance` behaviour) — graceful fallback for older
	// configs. Channels are already enum-validated by projectconfig.Validate.
	if cfg != nil && len(cfg.Channels) > 0 {
		if err := eng.UnrollSystemChannels(cfg.Channels); err != nil {
			return fmt.Errorf("driver: unroll system channels: %w", err)
		}
		// Same per-channel decomposition for the RED System Driver adapter step
		// (plan 20260530-1725 Item 0, D-adapter-ownership option A). The driver
		// adapter is channel-specific (MyShopApiDriver / MyShopUiDriver), so it
		// unrolls into one dispatch per channel just like the system step. Kept
		// inside the RED cascade, gated by GATE_SYSTEM_DRIVER_PORTS_CHANGED; the
		// no-arg full run is unchanged for a single-channel project.
		if err := eng.UnrollSystemDriverAdapterChannels(cfg.Channels); err != nil {
			return fmt.Errorf("driver: unroll system driver adapter channels: %w", err)
		}
	}

	// Statically unroll the external-system driver-adapter contract cycle from
	// the project-declared `external-systems:` registry (plan 20260615-0755).
	// Like channels, external systems are project-dependent, so the single
	// IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS anchor in shared-contract is
	// rewritten in memory into one guarded clone per registered system, each
	// carrying its baked external-system-name + real-kind. Done before Bind so
	// the synthesized clones get NodeFns resolved; order-independent of the
	// channel unrolls (they target different processes). Guarded on a non-empty
	// registry — a project with no external systems never reaches the anchor
	// (its entry gate is port-keyed), so the single static node is harmless.
	if cfg != nil && len(cfg.ExternalSystems) > 0 {
		names := cfg.ExternalSystemNames()
		realKind := make(map[string]string, len(names))
		for _, name := range names {
			realKind[name] = string(cfg.ExternalSystems[name].RealKind)
		}
		if err := eng.UnrollExternalSystems(names, realKind); err != nil {
			return fmt.Errorf("driver: unroll external systems: %w", err)
		}
	}

	if err := eng.Bind(); err != nil {
		return fmt.Errorf("driver: bind engine: %w", err)
	}

	// Per-run diagnostic state: timestamp + monotonic dispatch counter,
	// shared by every dispatcher closure registered by wrapAgentDispatchers.
	// Used to compose <run-ts>/<seq>-<agent>.prompt.md log paths so files
	// sort in dispatch order regardless of clock granularity.
	runTs := nowFn().UTC().Format("20060102-150405")
	runState := &runState{
		runTimestamp: runTs,
		repoPath:     repoPath,
		pidRunDir:    resolvePidRunDir(runTs, opts.Stderr),
		// Snapshot HEAD up front so the run-end digest can list the commits
		// produced between now and the final HEAD. Best-effort: empty when
		// repoPath isn't a git repo — the digest just omits the section.
		baseSHA: fullHeadSHA(repoPath),
		// Wall-clock start for the step summary's headline total.
		started: nowFn(),
	}
	// Print the per-agent summary table (model / effort / elapsed / tokens /
	// cost) on every Run exit — success AND error. Deferred so a fix-loop
	// dispatch that busts halfway still shows what ran before the bust.
	// Registered AFTER `defer logClose()` so this fires first (LIFO) and
	// the summary lands in the --log-file before the file is closed.
	// Step-execution summary (agents + commands, with per-step timing and a
	// wall-clock total). Registered BEFORE the agent-summary defer so it runs
	// AFTER it (LIFO): the agent cost table prints first, then the step
	// timeline. Same "always print, success and error" policy.
	defer func() { runState.printStepSummary(opts.Out.Phase) }()
	defer func() { runState.printAgentSummary(opts.Out.Phase) }()

	// Live execution-flow tree (plan 20260604-1632): the decolored, readable
	// sibling of the --log-file colored stream, written incrementally to
	// <run-ts>/flow.txt so a halted run still leaves the partial tree on disk
	// (D3/D5). The writer is threaded into trace.WrapAll below and rendered
	// from the same per-dispatch Event records the live stream consumes (D2).
	// Fail-soft: if the file can't be opened we warn and continue with a nil
	// writer — the trace is informational, never load-bearing (Item 4),
	// matching the openEventsLog / PID-marker policy.
	flowStart := nowFn()
	runDir := filepath.Join(repoPath, ".gh-optivem", "runs", runState.runTimestamp)
	var flowTree *trace.TreeWriter
	if f, err := openFlowFile(runDir); err != nil {
		fmt.Fprintf(opts.Stderr, "driver: warning: open flow.txt: %v\n", err)
	} else {
		flowTree = trace.NewTreeWriter(f)
		flowTree.WriteHeader(trace.TreeHeader{
			RunTimestamp: runState.runTimestamp,
			RepoPath:     repoPath,
			Process:      opts.ProcessName,
			IssueNum:     opts.IssueNum,
		})
		// Footer carries the named runErr (final verdict) and the dispatch
		// count straight from runState's records — the same tally
		// printAgentSummary renders — so the two cannot diverge. Registered
		// here so it fires before printAgentSummary / logClose (LIFO) but
		// after the digest tail; flow.txt is its own file, independent of
		// the --log-file logClose.
		//
		// After the footer lands, echo the flow.txt path to the operator's
		// Phase sink so the closing pointer cluster (right after `Run digest:`,
		// which fires first via its later-registered defer) surfaces the
		// readable execution tree — the mainstream "stream live, then point at
		// the artifact" convention. The live colored stream already showed the
		// content; this line just tells the operator where the clean nested
		// copy lives. Mirrored into --log-file too (logClose fires last).
		flowPath := filepath.Join(runDir, "flow.txt")
		defer func() {
			flowTree.WriteFooter(trace.TreeFooter{
				Result:     runErr,
				WallClock:  nowFn().Sub(flowStart),
				CommitSHA:  headCommitSHA(repoPath),
				Dispatches: len(runState.snapshotRecords()),
				RunDir:     runDir,
				LogFile:    opts.LogFile,
			})
			f.Close()
			fmt.Fprintf(opts.Out.Phase, "Execution flow: %s\n", flowPath)
		}()
	}

	// Post-Bind decoration order matters:
	//   1. Wrap user-task agent dispatch with per-node info-printer (uses
	//      RawNode metadata only available after Bind).
	//   2. Apply verify pre/post-condition decorators (commit-message HEAD
	//      checks).
	//   3. Apply override hooks — they sit on top of verify so a
	//      node_replacements: swap short-circuits both the verify check
	//      and the agent dispatcher (the documented escape-hatch behaviour).
	//   4. Apply the trace decorator last so its entry/exit lines bracket
	//      every other decorator's behaviour. The operator sees the
	//      composed call as one node fire.
	wrapAgentDispatchers(eng, opts, cfg, runState)
	verify.WrapAll(eng, verify.Deps{})
	wrapOverride(eng, opts.Override)

	// Diagnostic-side effects on the consumer repo, both idempotent so
	// re-running the driver from a fresh checkout never leaves the
	// developer with mystery files committed by mistake. Pruning failures
	// surface as warnings — they shouldn't block a real ticket run.
	if err := ensureGhOptivemGitignore(repoPath); err != nil {
		fmt.Fprintf(opts.Stderr, "driver: warning: ensure .gitignore for .gh-optivem/: %v\n", err)
	}
	if opts.KeepRuns > 0 {
		if err := pruneOldRuns(filepath.Join(repoPath, ".gh-optivem", "runs"), opts.KeepRuns); err != nil {
			fmt.Fprintf(opts.Stderr, "driver: warning: prune old runs: %v\n", err)
		}
	}
	// Trace is the canonical Phase-level emitter: BPMN node enter/exit
	// banners must reach the terminal even when --log-file is also set.
	// Pass opts.Out.Phase so the trace decorator writes to every sink
	// whose MaxLevel >= Phase (the terminal by default, plus the log file
	// when --log-file is on).
	//
	// Colorize stays anchored to the real os.Stdout TTY check — opts.Out.Phase
	// is a MultiWriter, not an *os.File, so the trace package cannot infer
	// TTY-ness from it. ANSI bytes still land verbatim in the log file
	// (less -R or NO_COLOR=1 strip them) per the LogFile contract.
	trace.WrapAll(eng, trace.Deps{
		Out:      opts.Out.Phase,
		RepoPath: repoPath,
		Colorize: isatty.IsTerminal(os.Stdout.Fd()),
		Tree:     flowTree,
	})
	// Phase-boundary banners sit OUTSIDE trace so each `[phase] start …`
	// / `[phase] end …` bracket wraps the corresponding
	// `[trace …] > / OK …` pair for the same call-activity. Applied last
	// so its wrap is the outermost layer on the targeted nodes.
	wrapPhaseBoundaries(eng, opts.Out.Phase)
	// Step-summary command capture: wrap every LOW execute-command
	// run-command service task so each shell command is timed and recorded
	// as a step. Agent steps are recorded at dispatch time in
	// newClaudeRunDispatcher; together they populate runState.steps in
	// execution order. See step_summary.go.
	wrapStepRecorders(eng, runState, opts.Stderr)
	printConfig(opts.Out.Phase, opts, cfg, repoPath)

	sCtx := statemachine.NewContext()
	seedScopeState(sCtx, cfg)
	// Per-run artifact directory — used by service tasks that materialize
	// inter-phase artifacts (e.g. materialize_parsed_concepts writing
	// <run_dir>/parsed-concepts.md for refine-acceptance-criteria).
	sCtx.Set("run_dir", filepath.Join(repoPath, ".gh-optivem", "runs", runState.runTimestamp))

	// Run-end tail: stamp the overall verdict into runState, then write the
	// human digest (summary.md) beside the machine sidecar and echo its
	// path. Registered here (after sCtx exists) so the closure can read the
	// ticket fields the parse-ticket action lands on sCtx during the walk
	// and the verdict from the named runErr. Fires before the deferred
	// printAgentSummary (LIFO) and before logClose, so the digest is on
	// disk and the verdict stamped before the table prints and the log file
	// closes. Best-effort: a write failure warns to stderr and never alters
	// the run outcome (D6), mirroring appendSummaryLine.
	defer func() {
		runState.setResult(runErr)
		digestPath := runState.summaryMarkdownPath()
		commits := commitsSince(repoPath, runState.baseSHA)
		if err := writeRunDigest(digestPath, runDigest{
			issueNum:           sCtx.GetString("issue-num"),
			title:              sCtx.GetString("issue-title"),
			url:                sCtx.GetString("issue-url"),
			description:        sCtx.GetString("description"),
			acceptanceCriteria: sCtx.GetString("acceptance-criteria"),
			records:            runState.snapshotRecords(),
			result:             runErr,
			commits:            commits,
			compareURL:         compareURL(repoPath, runState.baseSHA, len(commits)),
			steps:              runState.snapshotSteps(),
			wallClock:          runState.wallClock(),
		}); err != nil {
			fmt.Fprintf(opts.Stderr, "driver: warning: write run digest: %v\n", err)
		} else if digestPath != "" {
			fmt.Fprintf(opts.Out.Phase, "Run digest: %s\n", digestPath)
		}
	}()

	if opts.IssueNum > 0 {
		if err := preResolveIssue(ctx, opts, sCtx, cfg); err != nil {
			return fmt.Errorf("driver: pre-resolve issue #%d: %w", opts.IssueNum, err)
		}
	}

	// Scoped slice entry (plan 20260530-1725 Items 2b–2d, 3). A --target slice
	// lives deep inside change-system-behavior, so a direct RunProcess skips the
	// intake phases (PARSE_TICKET) that seed ticket State the slice's agents
	// read (acceptance-criteria, checklist, …). Run parse-ticket here so the
	// slice sees the same ticket context the full walk gives it, then route into
	// the resolved slice (which may refuse on a not-DONE upstream slice).
	entryProcess := opts.ProcessName
	if opts.Target != TargetUnset {
		if opts.IssueNum > 0 {
			if fn := actionReg.Lookup("parse-ticket"); fn != nil {
				if out := fn(sCtx); out.Err != nil {
					return fmt.Errorf("driver: scoped --target %s: parse ticket: %w", opts.Target, out.Err)
				}
			}
		}
		proc, err := resolveScopedEntry(opts.Target, opts.Channel, cfg, repoPath, sCtx)
		if err != nil {
			return fmt.Errorf("driver: %w", err)
		}
		entryProcess = proc
	}

	runErr = eng.RunProcess(entryProcess, sCtx)
	if errors.Is(runErr, ErrPendingHuman) {
		// Pending-human yield (Step 1): discard the uncommitted in-progress
		// edits so a later resume re-enters the human gate from the last clean
		// committed phase (scoped.go refuses a DIRTY slice). Tracked working-
		// tree mods only (`reset --hard HEAD`); untracked files are left as-is
		// per the plan's decided discard scope. Best-effort — a failed reset
		// warns but does not change the pending-human outcome.
		if _, gerr := gitRunFn(repoPath, "reset", "--hard", "HEAD"); gerr != nil {
			fmt.Fprintf(opts.Stderr, "driver: warning: discard uncommitted edits on pending-human yield: %v\n", gerr)
		}
	}
	return runErr
}

// resolveRepoPath returns the absolute path the driver treats as the
// consumer repo. Empty input falls back to the process CWD; non-empty
// is returned as-is (no canonicalisation, since callers and tests may
// pass either absolute or relative paths).
func resolveRepoPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return cwd, nil
}

// installLogFileMirror builds opts.Out from the configured sinks
// (terminal at opts.TerminalLevel; optional --log-file at opts.LogFileLevel)
// and tees opts.Stderr into the log file when present. Returns a close
// func the caller defers.
//
// Stderr keeps its today-shape MultiWriter — Stderr is low-volume
// (warnings + interactive prompt prefixes) so no level filtering is
// applied. Subprocess writers and every Fprint site in the runtime now
// pick a level via opts.Out instead of writing through opts.Stdout.
func installLogFileMirror(opts *Options) (func(), error) {
	sinks := []outlog.Sink{{W: opts.Stdout, MaxLevel: opts.TerminalLevel}}
	close := func() {}
	if opts.LogFile != "" {
		f, err := os.OpenFile(opts.LogFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return func() {}, fmt.Errorf("open log file %s: %w", opts.LogFile, err)
		}
		sinks = append(sinks, outlog.Sink{W: f, MaxLevel: opts.LogFileLevel})
		opts.Stderr = io.MultiWriter(opts.Stderr, f)
		close = func() { f.Close() }
	}
	opts.Out = outlog.New(sinks...)
	return close, nil
}

// loadDriverConfig returns the parsed config for the run. configPath is
// the resolved gh-optivem.yaml path supplied by the cobra layer (which
// applies projectconfig.ResolvePath); missing-file is a hard error.
//
// Programmatic callers (embedded driver smoke tests) may pass an empty
// configPath when they have no config to provide — that case falls back
// to the legacy Load(repoPath) behaviour where absence returns nil.
func loadDriverConfig(configPath, repoPath string) (*projectconfig.Config, error) {
	if configPath == "" {
		cfg, err := configcheck.Load(repoPath)
		if err != nil {
			return nil, fmt.Errorf("driver: %w", err)
		}
		return cfg, nil
	}
	cfg, err := configcheck.LoadFromPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("driver: %w", err)
	}
	return cfg, nil
}

// printConfig writes a one-shot configuration banner to w so the operator
// sees the resolved consumer repo, config source, project URL/name, repo
// strategy, and scope axes at startup — well before the first agent
// dispatch (or pre-resolve failure) would otherwise reveal them.
//
// Script-specific values (worktree path, rehearsal branch, build source
// dir) deliberately stay in the rehearsal wrapper — those have no meaning
// outside the rehearsal scenario, so the binary doesn't know about them.
func printConfig(w io.Writer, opts Options, cfg *projectconfig.Config, repoPath string) {
	source := configSourceLabel(opts.ConfigPath, cfg, repoPath)
	fmt.Fprintln(w, "Configuration:")
	fmt.Fprintf(w, "  consumer repo: %s\n", repoPath)
	fmt.Fprintf(w, "  config file:   %s\n", source)
	if opts.IssueNum > 0 {
		fmt.Fprintf(w, "  issue:         #%d\n", opts.IssueNum)
	}
	projectURL := opts.ProjectURL
	projectURLNote := ""
	if projectURL == "" && cfg != nil {
		projectURL = cfg.Project.URL
	}
	if opts.ProjectURL != "" {
		projectURLNote = " (from caller)"
	}
	fmt.Fprintf(w, "  project URL:   %s%s\n", orPlaceholder(projectURL, "(unset — pre-resolve will fail)"), projectURLNote)
	if cfg != nil {
		fmt.Fprintf(w, "  repo strategy: %s\n", orPlaceholder(cfg.RepoStrategy, "(unset)"))
		if repos := cfg.Repos(); len(repos) > 0 {
			fmt.Fprintf(w, "  repos:         %s\n", strings.Join(repos, ", "))
		}
		fmt.Fprintf(w, "  architecture:  %s\n", orPlaceholder(cfg.System.Architecture, "-"))
		switch cfg.System.Architecture {
		case projectconfig.ArchMonolith:
			fmt.Fprintf(w, "  system:        %s (lang: %s, repo: %s)\n",
				cfg.System.Path, cfg.System.Lang, cfg.System.Repo)
		case projectconfig.ArchMultitier:
			fmt.Fprintf(w, "  backend:       %s (lang: %s, repo: %s)\n",
				cfg.System.Backend.Path, cfg.System.Backend.Lang, cfg.System.Backend.Repo)
			fmt.Fprintf(w, "  frontend:      %s (lang: %s, repo: %s)\n",
				cfg.System.Frontend.Path, cfg.System.Frontend.Lang, cfg.System.Frontend.Repo)
		case projectconfig.ArchMicroservices:
			// One backend route per declared service, keyed by service name —
			// the microservices generalization of the single multitier
			// `backend:` line. backendServiceRoutes resolves the same
			// per-service TierSpec the system-test driver addresses, in
			// BackendServiceNames() (sorted) order so the banner is
			// deterministic. The single frontend stays one line (D5).
			for _, name := range cfg.BackendServiceNames() {
				route := cfg.System.BackendServices[name]
				fmt.Fprintf(w, "  backend %s: %s (lang: %s, repo: %s)\n",
					name, route.Path, route.Lang, route.Repo)
			}
			fmt.Fprintf(w, "  frontend:      %s (lang: %s, repo: %s)\n",
				cfg.System.Frontend.Path, cfg.System.Frontend.Lang, cfg.System.Frontend.Repo)
		}
		if !cfg.SystemTest.IsEmpty() {
			fmt.Fprintf(w, "  system-test:   %s (lang: %s, repo: %s)\n",
				cfg.SystemTest.Path, cfg.SystemTest.Lang, cfg.SystemTest.Repo)
		}
		for _, name := range cfg.ExternalSystemNames() {
			es := cfg.ExternalSystems[name]
			fmt.Fprintf(w, "  ext %s: real-kind=%s, stub: %s (repo: %s)\n",
				name, es.RealKind, es.Stub.Path, es.Stub.Repo)
			if !es.Simulator.IsEmpty() {
				fmt.Fprintf(w, "  ext %s sim:  %s (repo: %s)\n",
					name, es.Simulator.Path, es.Simulator.Repo)
			}
		}
	}
}

// configSourceLabel returns a human-readable description of where the
// driver loaded its config from, suitable for the printConfig banner.
//
// resolvedPath empty corresponds to the programmatic-caller path through
// loadDriverConfig (cfg loaded via Load(repoPath) tolerating absence);
// otherwise resolvedPath is the path the cobra layer's ResolvePath
// produced (flag > env > cwd) and the suffix flags how it was sourced.
func configSourceLabel(resolvedPath string, cfg *projectconfig.Config, repoPath string) string {
	if resolvedPath == "" {
		if cfg != nil {
			return filepath.Join(repoPath, projectconfig.Path)
		}
		return "(none — no gh-optivem.yaml at repo root)"
	}
	defaultPath, explicit := projectconfig.ResolvePath("")
	if explicit && resolvedPath == defaultPath {
		return resolvedPath + " ($" + projectconfig.EnvVar + ")"
	}
	if resolvedPath == defaultPath {
		return resolvedPath
	}
	return resolvedPath + " (--config)"
}

func orPlaceholder(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}

// seedScopeState copies repo-strategy and architecture from a loaded
// config into Context.State so agent prompts can substitute
// ${repo-strategy}, ${repos}, and ${architecture}. Empty values are left
// absent. nil cfg is a no-op.
//
// Per-phase scope (read/write path keys) is NOT seeded here — that
// information lives on the BPMN node, not the project config, and the
// dispatcher reads it via engine.Scope at dispatch time (plan
// 20260526-1448 Item 4).
//
// State (not Params) is the right destination: these facts are
// project-scoped and stable for the entire run, alongside issue-title
// (written by preResolveIssue) and the body-parsed description /
// acceptance-criteria / steps-to-reproduce / checklist (written by the
// parse-ticket service-task action — see actions.parseTicket). The
// dispatcher reads them back via ctx.GetString, which is a State
// lookup — writing to Params would silently expand to "" at
// substitution time.
func seedScopeState(sCtx *statemachine.Context, cfg *projectconfig.Config) {
	if cfg == nil {
		return
	}
	if cfg.RepoStrategy != "" {
		sCtx.Set("repo-strategy", cfg.RepoStrategy)
	}
	if repos := cfg.Repos(); len(repos) > 0 {
		sCtx.Set("repos", strings.Join(repos, ","))
	}
	if cfg.System.Architecture != "" {
		sCtx.Set("architecture", cfg.System.Architecture)
	}
	if lang := primaryLanguage(cfg); lang != "" {
		sCtx.Set("language", lang)
	}
}

// primaryLanguage picks the language seeded into ctx.State["language"] for
// every dispatch in this run. Prompts that vary their guidance per language
// (via the ${language} placeholder) resolve to the right slice on this value.
//
//   - Monolith → cfg.System.Lang.
//   - Multitier → cfg.System.Backend.Lang. The current ATDD prompts that
//     reference ${language} (test, dsl, driver, task) are backend-aligned;
//     the merged implement-system agent does not reference ${language} in
//     its stripped body, so a single seed suffices. If frontend-specific
//     language refs are introduced later, the dispatcher can override
//     per-agent without changing the schema.
//
// Returns "" when cfg has no architecture or the relevant Lang is unset;
// findUnfilledPlaceholders will then report ${language} as a leftover for
// any prompt that references it (the load-bearing semantics in D10).
func primaryLanguage(cfg *projectconfig.Config) string {
	if cfg == nil {
		return ""
	}
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		return cfg.System.Lang
	case projectconfig.ArchMultitier:
		return cfg.System.Backend.Lang
	default:
		return ""
	}
}

// backendServiceRoutes resolves the per-service routing handles the
// system-test driver addresses on a microservices system: a name → TierSpec
// map keyed by the `backend-services` map key (D4 — the key is both the
// service identity and the driver-routing handle, so each service is
// reachable on its own route/base-location). It generalizes the single
// backend the monolith / multitier driver assumes: those have exactly one
// route (System / System.Backend), microservices have N, one per declared
// service.
//
// Returns nil for monolith / multitier (and a nil cfg) so the single-backend
// path is untouched — those architectures resolve their one route directly
// off System / System.Backend as today, never through this map. The values
// reuse the declared TierSpec verbatim (validateBackendServices already
// enforces per-service path/repo/lang completeness), so no route-construction
// logic is forked per service. BackendServiceNames() is the sorted-iteration
// companion for any caller that needs deterministic order over the routes.
func backendServiceRoutes(cfg *projectconfig.Config) map[string]projectconfig.TierSpec {
	if cfg == nil || cfg.System.Architecture != projectconfig.ArchMicroservices {
		return nil
	}
	routes := make(map[string]projectconfig.TierSpec, len(cfg.System.BackendServices))
	for _, name := range cfg.BackendServiceNames() {
		routes[name] = cfg.System.BackendServices[name]
	}
	return routes
}

func (o Options) withDefaults() Options {
	if o.ProcessName == "" {
		o.ProcessName = DefaultProcessName
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Out == nil {
		o.Out = outlog.Default(o.Stdout)
	}
	if o.AgentSet == nil {
		o.AgentSet = agents.DefaultAgentSet()
	}
	return o
}

// preResolveIssue populates Context with the issue-resolution keys
// downstream actions consume (issue-num, issue-url, issue-title,
// issue-handle), by opening a tracker via factory.Open and calling
// Tracker.FindIssue. Called once at driver startup.
// cfg is the pre-loaded project config (may be nil if no gh-optivem.yaml and
// no --config); supplied by Run so the load happens once per driver
// invocation. opts.ProjectURL, when non-empty, overrides cfg.Project.URL
// for this run so the operator can point at a different board without
// editing config.
func preResolveIssue(ctx context.Context, opts Options, sCtx *statemachine.Context, cfg *projectconfig.Config) error {
	if cfg == nil {
		return fmt.Errorf("resolve issue #%d: gh-optivem.yaml is required for issue resolution", opts.IssueNum)
	}
	project := cfg.Project
	if opts.ProjectURL != "" {
		project.URL = opts.ProjectURL
	}
	tr, err := factory.Open(ctx, project)
	if err != nil {
		return fmt.Errorf("resolve issue #%d: %w", opts.IssueNum, err)
	}
	issue, err := tr.FindIssue(ctx, strconv.Itoa(opts.IssueNum))
	if err != nil {
		return fmt.Errorf("resolve issue #%d: %w", opts.IssueNum, err)
	}

	writeResolvedIssue(sCtx, issue)
	if opts.ResolvedIssue != nil {
		*opts.ResolvedIssue = issue
	}
	fmt.Fprintf(opts.Out.Phase, "Resolved issue %s %q (%s).\n", issue.ID, issue.Title, issue.URL)
	return nil
}

// writeResolvedIssue mirrors a tracker.Issue into the conventional Context
// keys downstream actions read. The runtime uses Issue.Handle as the
// opaque project-membership payload SetStatus consumes, and Issue.URL as
// the addressable form callers serialize to backend-native arguments.
//
// `ticket-id` is a backend-agnostic alias for the tracker-verbatim id
// (issue.ID), seeded so agent prompts can reference ${ticket-id} without
// caring whether the project's tracker is GitHub or Jira. issue-num and
// ticket-id carry the same value today; the alias exists so prompt
// vocabulary stays neutral.
func writeResolvedIssue(sCtx *statemachine.Context, issue tracker.Issue) {
	sCtx.Set("issue-num", issue.ID)
	sCtx.Set("issue-url", issue.URL)
	sCtx.Set("issue-title", issue.Title)
	sCtx.Set("issue-handle", issue.Handle)
	sCtx.Set("ticket-id", issue.ID)
}

// registerAgentDispatchers registers a no-op base dispatcher for every
// agent that has an embedded prompt (filesystem walk via agents.Names).
// The substantive prompt-and-pause behaviour is layered on after Bind by
// wrapAgentDispatchers, which has access to per-node RawNode metadata
// (description, agent). Adding a new agent is now: drop an agent
// definition under internal/atdd/assets/runtime/agents/atdd/, recompile.
func registerAgentDispatchers(r *agents.Registry, set *agents.AgentSet) {
	if set == nil {
		set = agents.DefaultAgentSet()
	}
	noop := func(ctx *statemachine.Context) statemachine.Outcome {
		return statemachine.Outcome{}
	}
	for _, name := range set.Names() {
		r.Register(name, noop)
	}
}

// wrapAgentDispatchers replaces every user-task NodeFn with a
// per-node closure that has access to the YAML node's Raw metadata
// (description, agent name) and the per-run Options:
//
//   - `agent: human` STOP nodes get a human-stop dispatcher that
//     prints the node ID + description before blocking on stdin, so
//     the operator can see what they're approving instead of the
//     bare "STOP — press Enter" prompt.
//   - Other agents get either a clauderun-based dispatcher (the v2
//     default — auto-launches the named Claude Code subagent via
//     the `claude` CLI) or, when opts.ManualAgents is true, the v1
//     pause-and-prompt fallback.
//
// rs threads the per-run timestamp + monotonic dispatch counter through
// every closure so each clauderun.Dispatch can compute a unique
// PromptLogPath without coordinating across nodes. nil is fine for
// tests that don't care about the run-log path; the closure falls back
// to an empty PromptLogPath which clauderun treats as "skip log".
func wrapAgentDispatchers(eng *statemachine.Engine, opts Options, cfg *projectconfig.Config, rs *runState) {
	// Tests construct Options directly without going through Run() →
	// withDefaults(), so opts.Out may be nil at this point. Defaulting
	// once here keeps every dispatcher closure safe to call without
	// nil-checking opts.Out on every Fprint site.
	opts = opts.withDefaults()
	for _, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind != statemachine.UserTask {
				continue
			}
			raw := node.Raw
			nodeID := id
			inner := node.Fn
			switch {
			case raw.Agent == "":
				continue
			case process.ID == "approve" && raw.Agent == "human":
				// LOW `approve` primitive (BPMN Phase D Item 6, Q-D2):
				// the GATE_APPROVED gateway routes on the value the
				// dispatcher writes to ctx.State["approval-outcome"], so
				// reject must return gracefully (Outcome{}) instead of
				// the hard halt newHumanStopDispatcher does for every
				// other STOP site. Wired ahead of the generic human
				// branch below so the lookup wins.
				node.Fn = newApproveDispatcher(opts, raw, nodeID)
			case raw.Agent == "human":
				node.Fn = newHumanStopDispatcher(opts, raw, nodeID)
			case opts.ManualAgents:
				node.Fn = newManualAgentDispatcher(opts, raw, inner)
			default:
				node.Fn = newClaudeRunDispatcher(opts, raw, eng, cfg, rs, inner)
			}
			process.Nodes[id] = node
		}
	}
}

// newHumanStopDispatcher returns a NodeFn for `agent: human` STOP
// nodes. It prints the node ID and the YAML description (with any
// ${...} placeholders expanded against the live Context.Params) so
// the operator can see what they're approving, then routes the y/n
// decision through promptio for consistent semantics with every
// other human prompt: explicit y/n required, no Enter shortcut.
//
// This replaces the bare `humanStop` from agents/registry.go for any
// process that's been wrapped by wrapAgentDispatchers — the registry
// version stays in place as the fallback for tests and code paths
// that bypass the driver wrapping.
func newHumanStopDispatcher(opts Options, raw statemachine.RawNode, nodeID string) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		description, err := statemachine.ExpandParams(raw.Name, ctx.Params, ctx.State)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("STOP banner at %s: %w", nodeID, err)}
		}

		fmt.Fprintln(opts.Out.Phase)
		if description != "" {
			fmt.Fprintf(opts.Out.Phase, "[%s] %s\n", nodeID, description)
		} else {
			fmt.Fprintf(opts.Out.Phase, "[%s] STOP\n", nodeID)
		}
		// CategoryHuman is always in the resolved confirm set, so this
		// always delegates to the interactive prompt regardless of --auto.
		// The BPMN human-STOP author chose this STOP precisely because no
		// machine decides it; --auto explicitly cannot opt out.
		ok, err := approval.Confirm(opts.Approval, approval.CategoryHuman, opts.Stdin, opts.Out.Phase, "  Approve?")
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("read STOP confirmation at %s: %w", nodeID, err)}
		}
		if !ok {
			return statemachine.Outcome{Err: fmt.Errorf("user aborted at %s", nodeID)}
		}
		return statemachine.Outcome{}
	}
}

// newApproveDispatcher returns the NodeFn for the ASK_HUMAN user-task
// inside the LOW `approve` primitive (BPMN Phase D Item 6, Q-D2).
// Prints the expanded `${question}` from the YAML node description,
// asks y/n through promptio (same explicit-y/n semantics as every
// other operator prompt), writes ctx.State["approval-outcome"] =
// "approved"|"rejected", and returns Outcome{} either way so the
// downstream GATE_APPROVED gateway routes on the state value instead
// of the engine halting at this node.
//
// This is sibling to newHumanStopDispatcher: that one hard-halts on
// rejection because every other STOP site in the runtime treats "no"
// as "abort the whole run". The `approve` primitive inverts that —
// reject is a routable outcome the caller owns (Q3 = A; caller-owned
// NO branch).
func newApproveDispatcher(opts Options, raw statemachine.RawNode, nodeID string) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		question, err := statemachine.ExpandParams(raw.Name, ctx.Params, ctx.State)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("approve banner at %s: %w", nodeID, err)}
		}

		fmt.Fprintln(opts.Out.Phase)
		if question != "" {
			fmt.Fprintf(opts.Out.Phase, "[%s] %s\n", nodeID, question)
		} else {
			fmt.Fprintf(opts.Out.Phase, "[%s] Approve?\n", nodeID)
		}
		// Category comes from the YAML — either the ASK_HUMAN node's
		// own `category:` field (statemachine.RawNode.Category, set
		// when the author pins it at the node level) or a `category:`
		// param threaded in from the call site via the approve
		// primitive's call-activity params. Call-site override wins
		// because the same `approve` LOW primitive is shared across
		// callers with different category needs. The parse-time
		// validator guarantees every approve site has one resolved
		// category, so any error here means the loader missed a site.
		cat, err := classifyApproveCategory(raw, ctx)
		if err != nil {
			return statemachine.Outcome{Err: err}
		}
		ok, err := approval.Confirm(opts.Approval, cat, opts.Stdin, opts.Out.Phase, "  Approve?")
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("read approve confirmation at %s: %w", nodeID, err)}
		}
		if ok {
			ctx.Set("approval-outcome", "approved")
		} else {
			ctx.Set("approval-outcome", "rejected")
		}
		return statemachine.Outcome{}
	}
}

// newManualAgentDispatcher returns the v1 pause-and-prompt wrapper. The
// operator launches the named agent in a second window (typically the
// Task tool in a separate Claude Code session), commits, then presses
// Enter at the driver's prompt to advance.
func newManualAgentDispatcher(opts Options, raw statemachine.RawNode, inner statemachine.NodeFn) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		if err := promptForAgent(opts, raw, ctx.Params, ctx.State); err != nil {
			return statemachine.Outcome{Err: err}
		}
		return inner(ctx)
	}
}

// newClaudeRunDispatcher returns the v2 dispatcher. It reads the override
// hints written to the Context state by override.Wrap, pulls the ticket
// fields populated by preResolveIssue, and hands the lot to
// clauderun.Dispatch. The agent does not commit; the wrapping CLI
// stages and commits the working-tree delta after dispatch returns.
//
// rs supplies the per-dispatch PromptLogPath. nil rs (only happens in
// tests today) skips the log — clauderun treats empty PromptLogPath as
// "no diagnostics file".
//
// eng is the loaded statemachine.Engine; the dispatcher uses it to look
// up per-phase scope (engine.Scope) for the current MID, surfaced to the
// agent as ${scope_block} (plan 20260526-1448 Item 4). For fix-* recovery
// dispatches that have no MID node of their own, the lookup keys off
// ctx.State["originating-task-name"] so the recovery prompt inherits its
// caller's scope.
func newClaudeRunDispatcher(opts Options, raw statemachine.RawNode, eng *statemachine.Engine, cfg *projectconfig.Config, rs *runState, inner statemachine.NodeFn) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		extraText := ctx.GetString(override.KeyExtra)
		replaceText := ctx.GetString(override.KeyReplace)

		issueNum, _ := strconv.Atoi(ctx.GetString("issue-num"))

		agentName, err := statemachine.ExpandParams(raw.Agent, ctx.Params, ctx.State)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("dispatcher: agent template %q: %w", raw.Agent, err)}
		}
		// Build the prompt-substitution bag in three layers, mirroring the
		// scope chain `ExpandParams` already uses for service-tasks and
		// templated YAML fields:
		//
		//   1. ctx.Params — the call-activity stack's accumulated params
		//      (push/pop in wrapCallActivity). Carries values like
		//      ${test-names} declared at an outer call site and inherited
		//      through the fix-unexpected-failing-tests → execute-agent
		//      hops. Lowest
		//      precedence so node-level params can override.
		//   2. raw.Params — the YAML node's own `params:` block (e.g.
		//      `failure_type: compile` on FIX_COMPILE), expanded against
		//      the live ctx scope. Wins over inherited values so a
		//      per-node label can shadow.
		//
		// Without step 1, call-activity params would silently disappear at
		// the user-task hop because RUN_AGENT is a generic primitive with
		// no `params:` block of its own (see clauderun's
		// "prompt has unfilled placeholders" error).
		nodeParams := make(map[string]string, len(ctx.Params)+len(raw.Params))
		for k, v := range ctx.Params {
			nodeParams[k] = v
		}
		for k, v := range raw.Params {
			expanded, err := statemachine.ExpandParams(v, ctx.Params, ctx.State)
			if err != nil {
				return statemachine.Outcome{Err: fmt.Errorf("dispatcher: node param %q: %w", k, err)}
			}
			nodeParams[k] = expanded
		}
		// Unattended human gate (plan 20260615-1845 Step 1). A category:human
		// node forces interactive mode (Headless is AND-gated on category in
		// cOpts below), which blocks on the operator's TTY. In an unattended run
		// (rehearsal / CI / cron / piped — stdin is not a terminal) no operator
		// is present, so the TUI would stall indefinitely (run #69). Yield
		// instead: return the ErrPendingHuman sentinel WITHOUT dispatching, so
		// the machine is freed immediately. Run discards the uncommitted edits
		// and the CLI maps the sentinel to ExitCodePendingHuman; a later resume
		// (scoped.go) re-enters this gate from the clean committed tree with an
		// operator present.
		if nodeParams["category"] == "human" && !stdinIsTTYFn() {
			fmt.Fprintf(opts.Stderr, "Human gate reached (%s) with no operator TTY — yielding to pending-human; run is resumable from the last committed phase.\n", agentName)
			return statemachine.Outcome{Err: ErrPendingHuman}
		}
		tuning, err := opts.AgentSet.LoadTuning(agentName)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("dispatcher: load tuning for %q: %w", agentName, err)}
		}
		// Project-wide placeholders sourced from the loaded config. Carries
		// Family B path keys (system-driver-port, system-driver-adapter, …) and derived
		// Family A keys (sut-namespace, system-path, system-test-path, …)
		// for inlined phase-doc references in the prompt body. nil cfg
		// (CLI utility / test paths with no project context) leaves the
		// map nil; findUnfilledPlaceholders surfaces any unsubstituted
		// references at render time.
		var placeholders map[string]string
		if cfg != nil {
			placeholders = cfg.PlaceholderMap()
			// ${system-surface} is the per-dispatch production-code surface
			// the GREEN/redesign/refactor system prompts name. PlaceholderMap
			// only emits the monolith-only ${system-path} (empty on
			// multitier), so the driver resolves the surface here from cfg +
			// the dispatch's channel: monolith → the single system path;
			// multitier → the channel's tier (api→backend, ui→frontend) or
			// both tiers for a whole-system dispatch. Mirrors the
			// driver-resolved-per-phase Language pattern (plan 20260619-1120
			// A1). Unknown channel on multitier → unfilled, so
			// findUnfilledPlaceholders fail-fasts rather than silently
			// substituting "".
			if surface, ok := resolveSystemSurface(cfg, nodeParams["channel"]); ok && surface != "" {
				placeholders["system-surface"] = surface
			}
		}
		// Command-failure payload from upstream runCommand. State keys are
		// only populated when the LOW execute-command primitive's shell
		// dispatch failed and routed via GATE_COMMAND_SUCCEEDED → FIX;
		// on every other dispatch they're absent and the fields stay
		// zero-valued. fix-command-failed.md is the only prompt that
		// references the matching ${command_*} placeholders.
		//
		// Validation-failure payload from upstream validateOutputsAndScopes.
		// Same shape: state keys are populated only when the LOW
		// execute-agent primitive's post-RUN validation failed and routed
		// via the false branch → FIX. fix-missing-output.md and
		// fix-scope-diff.md are the only prompts that reference the
		// matching ${failing-task-name} / ${missing-outputs} /
		// ${violating-paths} placeholders.
		commandExitCode, _ := ctx.Get("command-exit-code").(int)
		// Per-phase scope (plan 20260526-1448 Item 4). Look up via
		// engine.Scope using `originating-task-name` for fix-* recovery
		// dispatches (which have no MID of their own — task-name is the
		// dynamic "fix-${failure-kind}"), and `task-name` otherwise. The
		// engine indexes scopes by the writing-agent MID's process ID =
		// task-name (verb), so the lookup must use the verb, not the
		// agent noun (plan 20260526-1701 split the two identities).
		//
		// Both keys live in ctx.Params (call-activity push at run.go:164),
		// not ctx.State — mirrors actions.phaseTaskName. Reading from
		// State here would silently fall through to agentName (the noun)
		// and Engine.Scope would miss, leaving ${scope_block} unfilled.
		//
		// agentName remains the third fallback for test fixtures that
		// dispatch a synthetic user-task without going through the LOW
		// execute-agent primitive, so neither task-name nor
		// originating-task-name is populated.
		scopeKey := ctx.Params["originating-task-name"]
		if scopeKey == "" {
			scopeKey = ctx.Params["task-name"]
		}
		if scopeKey == "" {
			scopeKey = agentName
		}
		var scopeRead, scopeWrite []string
		var scopeRationale string
		if eng != nil {
			r, w, _ := eng.Scope(scopeKey)
			scopeRead = r
			scopeWrite = w
			scopeRationale, _ = eng.ScopeRationale(scopeKey)
		}

		// Scope augmentation for fix-* recovery dispatches. Some
		// failure-kinds require the fixer to edit paths the originating
		// task's scope never lists (e.g. scope-diff-fixer may need to
		// widen `scopes:` in process-flow.yaml when the violation is
		// "scopes too narrow"). fixScopeAugmentation pins the rule; the
		// `fix-*` agent inventory comment at the top of process-flow.yaml
		// documents it so a YAML reader sees the augmentation next to
		// the dispatch site. failure-kind is absent on every non-fix
		// dispatch, so the lookup is a no-op outside the recovery path.
		if extra := fixScopeAugmentation[ctx.GetString("failure-kind")]; len(extra) > 0 {
			scopeRead = append(scopeRead, extra...)
			scopeWrite = append(scopeWrite, extra...)
		}

		// Compute the paired per-dispatch artefact paths in one seq bump
		// (prompt log + outputs JSONL + events JSONL). When rs is nil —
		// test fixtures that bypass the driver-managed runState — all
		// come back empty and clauderun treats them as "skip the log" /
		// "no outputs channel" / "no events audit".
		promptLog, outputFilePath, eventsLogPath, eventsTextLogPath, pidFilePath := rs.dispatchPaths(agentName)

		// Output channel (plan 20260526-2118). Resolve the writing-agent
		// MID's declared outputs the same way scope is resolved — via
		// engine.Outputs(originating-task-name|task-name). The OutputSpec
		// list is the single source of truth for three downstream
		// consumers: the GH_OPTIVEM_OUTPUT_KEYS env var (write-time CLI
		// allow-list), the ${expected_outputs} prompt section, and the
		// post-RUN validate-outputs-and-scopes presence check. When the
		// MID declares no outputs (or this dispatch isn't going through a
		// MID — e.g. test fixtures), the channel is unwired:
		// GH_OPTIVEM_OUTPUT_* env vars stay unset and the agent's `gh
		// optivem output write` refuses with "no outputs declared". The
		// path stash is also skipped so validate-outputs-and-scopes
		// treats it as no-op.
		//
		// Envelope exception (plans 20260528-1150, 20260606-1539): every
		// dispatch with a non-`none` scope must be able to emit the
		// scope-exception envelope per scope.md doctrine, even when its
		// MID declares no flag outputs — the prod-agent MIDs
		// (implement-system, update-system, the driver-adapter MIDs, …)
		// plus refactor-tests and the two fix-unexpected-*-tests agents.
		// The gate is "has a scope to violate" (!eng.IsScopeNone), not
		// category, so any scoped agent can raise the clean
		// STOP_SCOPE_VIOLATION halt instead of churning the FIX loop to
		// AGENT_FIX_EXHAUSTED. We merge the envelope keys into
		// GH_OPTIVEM_OUTPUT_KEYS (deduped against any the MID already
		// declares) so `gh optivem output write scope-exception-files=...`
		// is accepted; the per-dispatch JSONL path is the same one
		// rs.dispatchPaths already computed. validate-outputs-and-scopes'
		// readOutputsJSONL recognises the envelope key types as built-in
		// facts (statemachine.EnvelopeOutputSpecs), so the absent declared
		// list does not lose type fidelity. scope: none dispatches
		// (refine-acceptance-criteria) stay exempt.
		var (
			outputKeysSpec  string
			expectedOutputs []statemachine.OutputSpec
		)
		if eng == nil {
			ctx.Set("output-file-path", "")
			outputFilePath = ""
		} else {
			outs, _ := eng.Outputs(scopeKey)
			expectedOutputs = outs
			switch {
			case outputFilePath != "" && (len(outs) > 0 || !eng.IsScopeNone(scopeKey)):
				// Declared outputs OR a non-`none` scope: wire the channel
				// and merge the envelope keys in (deduped) so the writers
				// that already list them don't double-list and the scoped
				// agents that declare no flag outputs still get them.
				outputKeysSpec = encodeOutputKeysSpec(withEnvelopeSpecs(outs))
				ctx.Set("output-file-path", outputFilePath)
			default:
				// scope: none (refine-acceptance-criteria) or a no-MID
				// dispatch (test fixtures whose scopeKey falls back to the
				// agent noun, not a process — IsScopeNone returns false
				// there, but those have no outputFilePath so they land
				// here) → unwire the path so clauderun doesn't export
				// GH_OPTIVEM_OUTPUT_FILE in isolation, which would let
				// the agent write to a file the dispatcher never reads.
				// Also clear any path the previous agent's dispatch
				// stashed: without this, the next
				// validateOutputsAndScopes would re-read the prior JSONL
				// with this MID's empty declared list, fall through
				// coerceJSONOutputValue's default branch, and clobber
				// already-typed state keys ([]string → []any).
				ctx.Set("output-file-path", "")
				outputFilePath = ""
			}
		}

		nodeDescription, err := statemachine.ExpandParams(raw.Name, ctx.Params, ctx.State)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("dispatcher: node description template %q: %w", raw.Name, err)}
		}
		// Loop-attempt context: the engine stamped the current node's 1-based
		// visit count and max-visits cap onto ctx (generic "visit"
		// vocabulary). Read them once here; both the dispatch options
		// (${attempt-block} render) and the summary record (attempt N/M
		// suffix) consume the same pair. attemptMax == 0 means the node is
		// not a loop, and every downstream consumer no-ops on that.
		attemptNumber, _ := ctx.Get("visit-count").(int)
		attemptMax, _ := ctx.Get("visit-max").(int)
		cOpts := clauderun.Options{
			Agent:               agentName,
			AgentSet:            opts.AgentSet,
			NodeDescription:     nodeDescription,
			IssueNum:            issueNum,
			IssueTitle:          ctx.GetString("issue-title"),
			TicketID:            ctx.GetString("ticket-id"),
			Architecture:        ctx.GetString("architecture"),
			Subtype:             ctx.GetString("subtype"),
			Language:            ctx.GetString("language"),
			ScopeRead:           scopeRead,
			ScopeWrite:          scopeWrite,
			ScopeRationale:      scopeRationale,
			Checklist:           ctx.GetString("checklist"),
			AcceptanceCriteria:  ctx.GetString("acceptance-criteria"),
			ParsedConcepts:      ctx.GetString("parsed_concepts"),
			VerifyFailureOutput: ctx.GetString("verify_failure_output"),
			ChangedFiles:        fixChangedFiles(ctx, agentName, opts.RepoPath),
			CommandLine:         ctx.GetString("command-line"),
			CommandExitCode:     commandExitCode,
			CommandStderrTail:   ctx.GetString("command-stderr-tail"),
			FailingTaskName:     ctx.GetString("failing-task-name"),
			MissingOutputs:      ctx.GetString("missing-outputs"),
			ViolatingPaths:      ctx.GetString("scope-violating-paths"),
			NodeParams:          nodeParams,
			Placeholders:        placeholders,
			OverrideText:        extraText,
			RawPrompt:           replaceText,
			PromptOverride:      opts.TaskPromptOverrides[agentName],
			// human-tier dispatches (fix-* agents, refine-acceptance-criteria)
			// always launch interactively so the operator can drive the
			// conversation — mirrors approval.go's "human tier never bypasses"
			// rule on the dispatch side.
			Headless:          opts.Headless && nodeParams["category"] != "human",
			Approval:          opts.Approval,
			Model:             resolveDispatchModel(tuning, nodeParams),
			Effort:            tuning.Effort,
			ShowPrompt:        opts.ShowPrompt,
			PromptLogPath:     promptLog,
			EventsLogPath:     eventsLogPath,
			EventsTextLogPath: eventsTextLogPath,
			PidFilePath:       pidFilePath,
			OutputFilePath:    outputFilePath,
			OutputKeysSpec:    outputKeysSpec,
			ExpectedOutputs:   expectedOutputs,
			// Loop-attempt context (plan 20260616-0649). The engine writes the
			// current node's 1-based visit count and its max-visits cap onto
			// ctx (generic "visit" vocabulary); map them to the ATDD "attempt"
			// at this boundary. Only meaningful when visit-max > 0 (a looped
			// node, e.g. a fixer) — clauderun renders ${attempt-block} and the
			// summary suffixes the agent name only in that case.
			AttemptNumber: attemptNumber,
			AttemptMax:    attemptMax,
			RepoPath:      opts.RepoPath,
			Stdout:        opts.Stdout,
			Stderr:        opts.Stderr,
			Stdin:         opts.Stdin,
			Out:           opts.Out,
		}

		t0 := nowFn()
		runResult, runErr := clauderun.Dispatch(context.Background(), opts.ClaudeRunDeps, cOpts)
		elapsed := nowFn().Sub(t0)
		// Record the dispatch even on failure so the end-of-run summary
		// shows what ran before the bust — partial runs are common during
		// fix-loop debugging and the summary is most useful there.
		record := dispatchRecord{
			agent:         agentName,
			channel:       nodeParams["channel"],
			model:         tuning.Model,
			effort:        tuning.Effort,
			elapsed:       elapsed,
			usage:         runResult.Usage,
			err:           runErr,
			attemptNumber: attemptNumber,
			attemptMax:    attemptMax,
		}
		rs.appendRecord(record)
		// Mirror to the run's summary sidecar so `gh optivem run summary`
		// can replay any past run, AND so a binary crash between this
		// point and Run's deferred print-summary still leaves every
		// completed row on disk. Best-effort: a write failure is logged
		// as a warning and never blocks the dispatch.
		if err := appendSummaryLine(rs.summaryPath(), record); err != nil {
			fmt.Fprintf(opts.Stderr, "driver: warning: append summary sidecar: %v\n", err)
		}
		// Agent half of the step-execution summary. scopeKey is the MID
		// task-name (originating-task-name for fix-* recovery, agent noun for
		// no-MID test fixtures) — the same "BPMN step" identity the scope
		// lookup uses. The command half is recorded by wrapStepRecorders.
		rs.recordStep(stepRecord{
			name:     scopeKey,
			bpmnStep: scopeKey,
			channel:  nodeParams["channel"],
			kind:     stepKindAgent,
			elapsed:  elapsed,
			err:      runErr,
		}, opts.Stderr)
		if runErr != nil {
			return statemachine.Outcome{Err: runErr}
		}
		// Structured outputs flow through the JSONL channel
		// (GH_OPTIVEM_OUTPUT_FILE; see plan 20260526-2118). The
		// downstream validate-outputs-and-scopes action reads the file,
		// coerces values per the BPMN OutputSpec types, and flattens
		// them into ctx.State — replacing the old prose-YAML tail
		// parser that only worked in autonomous mode.
		return inner(ctx)
	}
}

// encodeOutputKeysSpec composes the GH_OPTIVEM_OUTPUT_KEYS env-var value
// from a writing-agent MID's declared OutputSpec list. Format is
// `key1:type1,key2:type2,...`, matching what the `output write` CLI's
// parseOutputKeysSpec expects. The `optional` flag is intentionally NOT
// encoded — write-time the CLI just needs to know "is this key
// declared, and what type?". Optional vs required is a read-time
// concern (the dispatcher's presence-check honours it).
// withEnvelopeSpecs appends the scope-exception envelope OutputSpecs to outs,
// skipping any whose key is already declared. The three writers and
// implement-dsl list the envelope explicitly, so for them this is a no-op;
// for every other non-`none`-scope dispatch (the prod-agent MIDs,
// refactor-tests, the two fix-unexpected-*-tests agents) it yields the
// envelope-only allow-list.
func withEnvelopeSpecs(outs []statemachine.OutputSpec) []statemachine.OutputSpec {
	seen := make(map[string]bool, len(outs))
	for _, o := range outs {
		seen[o.Key] = true
	}
	merged := append([]statemachine.OutputSpec(nil), outs...)
	for _, e := range statemachine.EnvelopeOutputSpecs() {
		if !seen[e.Key] {
			merged = append(merged, e)
		}
	}
	return merged
}

func encodeOutputKeysSpec(outs []statemachine.OutputSpec) string {
	if len(outs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(outs))
	for _, o := range outs {
		parts = append(parts, o.Key+":"+o.Type)
	}
	return strings.Join(parts, ",")
}

// fixScopeAugmentation maps a fix-* failure-kind to the extra paths
// the fixer is allowed to read and write beyond the originating task's
// scope. scope-diff is the only entry today: scope-diff-fixer may need
// to widen the call-site's `scopes:` list in process-flow.yaml when
// the violation is "scopes too narrow." Every other failure-kind
// inherits the originating task's scope unchanged. The matching
// inventory comment at the top of process-flow.yaml documents the
// rule so a YAML reader sees the augmentation next to the dispatch
// site.
var fixScopeAugmentation = map[string][]string{
	"scope-diff": {"internal/atdd/process/process-flow.yaml"},
}

// resolveDispatchModel returns the model a dispatch should run on: the agent's
// frontmatter `model:` by default, or its optional `model-later-channel:`
// override when this is a later-channel (common:false) dispatch. The rule is
// agent-agnostic — it fires for any agent that declares the override field.
// system-implementer is the only one today: it builds the shared common layer
// plus a forward-only migration on the first channel (common:true, sized by
// `model:`) but only a shallow per-channel adapter delta on later channels,
// hard-gated by the acceptance-${channel} suite, so that tier is safe on a
// cheaper model. An empty override (every other agent) means no downgrade.
//
// The whole model decision lives in the agent's frontmatter — both the default
// and the later-channel value — so changing or disabling the downgrade is a
// one-line YAML edit, not a code change. `common` is the orchestration's own
// per-channel flag (scoped.go binds it true only for the first channel).
func resolveDispatchModel(tuning agents.Tuning, nodeParams map[string]string) string {
	if nodeParams["common"] == "false" && tuning.ModelLaterChannel != "" {
		return tuning.ModelLaterChannel
	}
	return tuning.Model
}

// resolveSystemSurface computes the ${system-surface} placeholder for a
// dispatch — the production-code surface the GREEN/redesign/refactor
// system prompts name. The channel→tier mapping is pinned in Go here
// (plan 20260619-1120 A1); the config schema is unchanged, the surface is
// read off the existing System.Path / System.Backend / System.Frontend.
//
//   - monolith → the single System.Path.
//   - multitier + channel → that channel's tier path: api→backend,
//     ui→frontend.
//   - multitier + no channel (whole-system updater/refactorer dispatch) →
//     both tier paths joined in reader-friendly form ("backend/ and frontend/").
//
// Returns ok=false for an unknown channel on multitier (and for the
// not-yet-supported microservices / unset architectures) so the caller
// leaves ${system-surface} unfilled and findUnfilledPlaceholders fail-fasts,
// rather than silently substituting "" and writing code to an empty path.
func resolveSystemSurface(cfg *projectconfig.Config, channel string) (string, bool) {
	if cfg == nil {
		return "", false
	}
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		return cfg.System.Path, true
	case projectconfig.ArchMultitier:
		switch channel {
		case "":
			// Whole-system dispatch (updater/refactorer walk the Checklist
			// across every tier) — name both surfaces.
			return fmt.Sprintf("%s/ and %s/", cfg.System.Backend.Path, cfg.System.Frontend.Path), true
		case "api":
			return cfg.System.Backend.Path, true
		case "ui":
			return cfg.System.Frontend.Path, true
		default:
			return "", false
		}
	default:
		return "", false
	}
}

// fixChangedFiles returns the working-tree dirty-file listing (one
// path per line) the dispatcher passes into the ${changed_files}
// placeholder consumed by the fix-* fixer prompts. We only resolve a
// value for those agents because they are the only ones whose prompt
// templates reference the substitution — every other dispatch leaves
// the placeholder out of the template anyway, so paying for a
// `git status` on every node would be wasted work.
//
// Three of the fixers have a pre-WRITE snapshot delta stashed at
// ctx.State["phase-changed-files"] by validateOutputsAndScopes:
// fix-scope-diff (its own failure-kind), fix-unexpected-failing-tests,
// and fix-unexpected-passing-tests. The stash is narrower (and
// correct) than `git status --porcelain` — it excludes upstream-phase
// residue still uncommitted in the working tree, AND it survives the
// case where a parallel commit has already moved the WRITE-phase
// edits out of `git status`. Prefer the stash when present; fall
// back to the shell-out for defence-in-depth (e.g. tests that bypass
// validateOutputsAndScopes).
//
// fix-command-failed and fix-missing-output have no pre-WRITE
// snapshot — runCommand IS the executor for command-failed, and
// fix-missing-output may fire before any working-tree change exists.
// Those two always take the live shell-out path.
//
// On any shell error (no git in PATH, not a repo, …) we return the
// empty string. The fix-* prompts simply render an empty "Changed
// files" block; the agent can re-run `git status` itself if it needs
// the listing. The dispatch is feedback, not load-bearing.
func fixChangedFiles(ctx *statemachine.Context, agent, repoPath string) string {
	switch agent {
	case "fix-scope-diff",
		"fix-unexpected-failing-tests",
		"fix-unexpected-passing-tests":
		if v := ctx.GetString("phase-changed-files"); v != "" {
			return v
		}
		// Fall through to the live git-status fallback. The stash should
		// always be present after validateOutputsAndScopes runs; the
		// fallback exists for defence-in-depth.
	case "fix-command-failed", "fix-missing-output":
		// Live git-status only — no pre-snapshot exists for these dispatches.
	default:
		return ""
	}
	cmd := exec.Command("git", "status", "--porcelain")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(out), "\n")
}

// promptForAgent prints the per-node dispatch banner and blocks on stdin
// until the operator types Enter (continue) or `abort` (halt). v1 / fallback path.
//
// params + state are the live Context fields for the call-activity scope;
// templated fields in raw (e.g. ${agent} / ${change_type} inside the
// shared structural_cycle, ${failure-kind} from upstream bindings) are
// expanded against the params-then-state chain so the operator sees the
// substituted name in the banner instead of the literal placeholder.
func promptForAgent(opts Options, raw statemachine.RawNode, params map[string]string, state map[string]any) error {
	agent, err := statemachine.ExpandParams(raw.Agent, params, state)
	if err != nil {
		return fmt.Errorf("promptForAgent: agent template %q: %w", raw.Agent, err)
	}
	documentation, err := statemachine.ExpandParams(raw.Name, params, state)
	if err != nil {
		return fmt.Errorf("promptForAgent: documentation template %q: %w", raw.Name, err)
	}
	step := raw.ID

	fmt.Fprintln(opts.Out.Phase)
	fmt.Fprintf(opts.Out.Phase, "DISPATCH: %s\n", agent)
	if step != "" {
		fmt.Fprintf(opts.Out.Phase, "  Step: %s\n", step)
	}
	if documentation != "" {
		fmt.Fprintf(opts.Out.Phase, "  Phase: %s\n", documentation)
	}
	fmt.Fprintf(opts.Out.Phase, "  Launch the %s agent now (e.g. via the Task tool in Claude Code).\n", agent)
	fmt.Fprintln(opts.Out.Phase, "  When the agent's COMMIT lands on HEAD, approve to continue.")

	// Manual-agent dispatch is the v1 fallback for any user-task — the
	// operator is launching the agent by hand, which is inherently a
	// human-tier interaction. No prefix sniff needed.
	ok, err := approval.Confirm(opts.Approval, approval.CategoryHuman, opts.Stdin, opts.Out.Phase, "  Approve?")
	if err != nil {
		return fmt.Errorf("read agent-dispatch confirmation: %w", err)
	}
	if !ok {
		return fmt.Errorf("operator aborted at %s dispatch", agent)
	}
	return nil
}

// classifyApproveCategory maps an `approve` BPMN node to its approval
// category. The lookup chain mirrors how BPMN params propagate from the
// call site down through the `approve` LOW primitive:
//
//  1. ctx.Params["category"] — call-site override; the primary path,
//     because the `approve` primitive is shared across callers with
//     different category needs (a caller threads its tier through
//     `category: ${category}` in its call-activity params).
//  2. raw.Category — node-level pin on the ASK_HUMAN YAML; used when
//     the author wants a primitive's approve gate to default to a
//     specific tier regardless of caller.
//
// No default — parse-time validator (statemachine.validateApprovalCategories)
// is the SSoT for "every approve site resolves a category." If this
// dispatch-time function ever falls through, the parse-time pass missed
// the site and the error names the node so the YAML can be fixed. A
// typo'd `category: foo` errors here explicitly rather than silently
// landing on a default tier.
func classifyApproveCategory(raw statemachine.RawNode, ctx *statemachine.Context) (approval.Category, error) {
	if v, ok := ctx.Params["category"]; ok && v != "" {
		c, err := approval.ParseCategory(v)
		if err != nil {
			return 0, fmt.Errorf("approve node %q: invalid category param: %w", raw.ID, err)
		}
		return c, nil
	}
	if raw.Category != "" {
		c, err := approval.ParseCategory(raw.Category)
		if err != nil {
			return 0, fmt.Errorf("approve node %q: invalid category attribute: %w", raw.ID, err)
		}
		return c, nil
	}
	return 0, fmt.Errorf("approve node %q: no category resolved (parse-time validator should have caught this)", raw.ID)
}

// topProcesses is the set of TOP-level processes whose direct
// call-activity children are CYCLE-level "phases" — the boundaries the
// operator's mental model treats as phase transitions in `[phase] start
// …` / `[phase] end …` banners. A call-activity living anywhere outside
// this set is NOT a phase boundary (e.g. nested HIGH/MID call-activities
// inside `change-system-behavior` stay bracketed only by `[trace …]`).
//
// The set is closed and small: matches the TOP enumeration documented
// in process-flow.yaml's "Level reading order" comment block and pinned
// in transitions_test.go's TestLoadSnapshot_AllProcessesParse. Adding a
// new TOP process requires updating both this set and the snapshot.
var topProcesses = map[string]bool{
	"main":             true,
	"refine-ticket":    true,
	"implement-ticket": true,
	"refactor":         true,
}

// wrapPhaseBoundaries decorates every call-activity node that lives
// inside a TOP process with an `[agent]`-style entry/exit banner pair
// emitted to w. The wrap fires only for call-activities whose parent
// process is in topProcesses — nested call-activities further down the
// tree pass through unchanged. Elapsed is measured locally around the
// inner NodeFn invocation, mirroring trace.go's wrap pattern.
func wrapPhaseBoundaries(eng *statemachine.Engine, w io.Writer) {
	if w == nil {
		return
	}
	for processName, process := range eng.Processes {
		if !topProcesses[processName] {
			continue
		}
		for id, node := range process.Nodes {
			if node.Kind != statemachine.CallActivity {
				continue
			}
			phaseName := node.Raw.Process
			inner := node.Fn
			node.Fn = func(ctx *statemachine.Context) statemachine.Outcome {
				outlog.WritePhaseBoundary(w, "start", phaseName, 0)
				t0 := nowFn()
				out := inner(ctx)
				outlog.WritePhaseBoundary(w, "end", phaseName, nowFn().Sub(t0))
				return out
			}
			process.Nodes[id] = node
		}
	}
}

// wrapOverride applies the override.Wrap decorator to every node. Wrapping
// happens for every node regardless of kind so the dispatcher's per-node
// Extra / Replace lookup is always populated (an empty hint map is a no-op
// at the inner layer). Hooks themselves are sourced from gh-optivem.yaml's
// node_extras: / node_replacements: via the cobra layer.
func wrapOverride(eng *statemachine.Engine, hooks *override.Hooks) {
	if hooks == nil {
		hooks = &override.Hooks{}
	}
	for _, process := range eng.Processes {
		for id, node := range process.Nodes {
			node.Fn = override.Wrap(node.Fn, id, hooks)
			process.Nodes[id] = node
		}
	}
}

// nowFn is the package-level clock. Production points at time.Now;
// tests can swap it to pin runState.runTimestamp deterministically.
var nowFn = time.Now

// runState carries the per-Run diagnostic context shared across every
// dispatcher closure: the run timestamp (used as the prompt-log
// directory name) and a monotonic dispatch counter.
//
// The counter is an atomic.Int64 so concurrent dispatches (none today,
// but the engine doesn't structurally rule it out) get unique seq
// numbers. zero seq is never used: promptLogPath calls Add(1) before
// formatting, so the first dispatch sees seq=1 → "001-…".
//
// pidRunDir is the per-run subdirectory of the user-level gh-optivem
// state directory (see userStateDir) where PID marker files live —
// shape `<userStateDir>/runs/<ts>-<parent-pid>/`. Computed once at
// construction; empty string when userStateDir resolution failed (a
// stderr warning was emitted at that point and the dispatch skips PID
// markers — same fail-soft policy as openEventsLog).
type runState struct {
	runTimestamp string
	repoPath     string
	pidRunDir    string
	seq          atomic.Int64

	// baseSHA is the full HEAD commit SHA captured at Run start, before any
	// agent fires. The run-end digest diffs it against the final HEAD
	// (`git log baseSHA..HEAD`) to list exactly the commits this run
	// produced — the same "commits in this branch" view GitHub shows on a
	// PR. Empty when the consumer dir wasn't a git repo (or git was
	// unavailable) at start; the digest then omits the Commits section.
	baseSHA string

	// records accumulates one dispatchRecord per clauderun.Dispatch call so
	// the end-of-run summary banner can show what every agent cost. Guarded
	// by mu — the engine is single-threaded today but rs is shared across
	// every dispatcher closure and nothing structurally rules out a future
	// concurrent walk.
	mu      sync.Mutex
	records []dispatchRecord

	// steps accumulates one stepRecord per executed MID-level atomic step
	// (agent dispatch or shell command), in execution order, for the
	// end-of-run step-execution summary. Guarded by mu alongside records.
	// See step_summary.go.
	steps []stepRecord

	// result is the overall run verdict — the error RunProcess returned
	// (nil on success). Stamped by Run's deferred tail before the digest
	// is written, so renderRunDigest can front the summary table with a
	// ✅/❌ line. Guarded by mu alongside records (same theoretical-
	// concurrency rationale).
	result error

	// started is the wall-clock instant Run began, stamped at construction.
	// The step summary's "wall-clock" reconciliation line is
	// nowFn().Sub(started). Zero in test fixtures that build runState
	// directly without this field.
	started time.Time
}

// dispatchRecord is one row in the end-of-run summary: which agent ran
// with which model + effort, how long it took, and what it cost in tokens
// (when the runner could parse a stream-json envelope). usage is nil for
// interactive dispatches (no envelope to parse) and for headless runs that
// crashed before emitting the terminal `type:"result"` event. err is set
// when clauderun.Dispatch returned an error so the summary can mark the
// row as failed.
type dispatchRecord struct {
	agent   string
	channel string
	model   string
	effort  string
	elapsed time.Duration
	usage   *clauderun.TokenUsage
	err     error

	// attemptNumber / attemptMax record this dispatch's position in a
	// max-visits loop (1-based number, loop cap), copied from the engine's
	// per-node visit count. Both zero for a non-looped single-pass node;
	// renderAgentSummary suffixes the agent name with ` (attempt N/M)` only
	// when attemptMax > 0, so single-pass rows are untouched.
	attemptNumber int
	attemptMax    int
}

// appendRecord records one completed dispatch. Safe to call with nil rs —
// the test fixtures that bypass the driver-managed runState rely on that
// to keep the no-runState dispatch path simple.
func (rs *runState) appendRecord(r dispatchRecord) {
	if rs == nil {
		return
	}
	rs.mu.Lock()
	rs.records = append(rs.records, r)
	rs.mu.Unlock()
}

// setResult stamps the overall run verdict (the error RunProcess
// returned, nil on success). Safe to call with nil rs — the test
// fixtures that bypass the driver-managed runState rely on that, same as
// appendRecord.
func (rs *runState) setResult(err error) {
	if rs == nil {
		return
	}
	rs.mu.Lock()
	rs.result = err
	rs.mu.Unlock()
}

// snapshotRecords returns a copy of the recorded dispatches so the
// summary printer can iterate without holding the mutex. Safe with nil
// rs (returns nil, mirroring appendRecord).
func (rs *runState) snapshotRecords() []dispatchRecord {
	if rs == nil {
		return nil
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]dispatchRecord, len(rs.records))
	copy(out, rs.records)
	return out
}

// resolvePidRunDir composes the per-run subdirectory of the user-level
// gh-optivem state directory (see userstate.Dir) where this dispatch's
// PID marker files live. Shape:
// `<userstate.Dir>/runs/<runTimestamp>-<parent-pid>`. The parent-pid
// suffix disambiguates simultaneous gh-optivem starts for the same
// user — two processes can't share a PID, so two concurrent runs
// can't collide on the same directory even when they tick into the
// same wall-clock second.
//
// Returns "" when userstate.Dir resolution failed; a stderr warning is
// emitted then so the operator sees the cause once at startup.
func resolvePidRunDir(runTimestamp string, stderr io.Writer) string {
	stateDir, err := userstate.Dir()
	if err != nil {
		fmt.Fprintf(stderr, "driver: warning: cannot resolve user state dir, orphan-recovery PID markers disabled: %v\n", err)
		return ""
	}
	return filepath.Join(stateDir, "runs", fmt.Sprintf("%s-%d", runTimestamp, os.Getpid()))
}

// dispatchPaths composes the per-dispatch diagnostic file paths in
// <repoPath>/.gh-optivem/runs/<run-ts>/ (project-local) plus the
// per-dispatch PID marker in
// <userStateDir>/runs/<run-ts>-<parent-pid>/ (user-level). Bumps the
// per-run sequence counter once and shares the resulting seq across:
//
//   - <seq>-<agent>.prompt.md         — promptLog (project-local)
//   - <seq>-<agent>.outputs.jsonl     — outputFile (project-local; agent's
//     `gh optivem output write` appends here)
//   - <seq>-<agent>.events.jsonl      — eventsLog (project-local; clauderun
//     tees the headless stream-json
//     stdout here so post-mortem can
//     replay every tool call / message
//     the agent emitted)
//   - <seq>-<agent>.events.log        — eventsTextLog (project-local;
//     human-readable sibling of
//     eventsLog, formatted to look
//     like the interactive Claude
//     Code transcript)
//   - <seq>-<agent>.pid               — pidFile (user-level; clauderun
//     writes the JSON marker for
//     orphan-recovery between
//     cmd.Start() and cmd.Wait(),
//     removed on clean exit)
//
// Sharing the seq keeps the four artefacts paired on disk, so when an
// operator inspects a failed dispatch they sit next to each other.
// Bumping once (vs once per path) also means the next dispatch's seq
// is N+1 instead of N+4.
//
// The PID file lives at the user level rather than alongside the
// others because the motivating force-cancel bug is `rm -rf
// worktrees/rehearsal-XYZ/` failing because orphan claude.exe holds
// handles inside the worktree — a project-local marker would die with
// the worktree and leave the orphan untrackable.
//
// Returns empty strings when rs is nil — used by tests that bypass
// the driver-managed runState; clauderun treats empty paths as "skip
// the log" / "no outputs channel" / "no events audit" / "no PID
// marker". pidFile is also empty when rs.pidRunDir is empty
// (userStateDir resolution failed at startup) — same fail-soft policy.
func (rs *runState) dispatchPaths(agentName string) (promptLog, outputFile, eventsLog, eventsTextLog, pidFile string) {
	if rs == nil {
		return "", "", "", "", ""
	}
	seq := rs.seq.Add(1)
	dir := filepath.Join(rs.repoPath, ".gh-optivem", "runs", rs.runTimestamp)
	promptLog = filepath.Join(dir, fmt.Sprintf("%03d-%s.prompt.md", seq, agentName))
	outputFile = filepath.Join(dir, fmt.Sprintf("%03d-%s.outputs.jsonl", seq, agentName))
	eventsLog = filepath.Join(dir, fmt.Sprintf("%03d-%s.events.jsonl", seq, agentName))
	eventsTextLog = filepath.Join(dir, fmt.Sprintf("%03d-%s.events.log", seq, agentName))
	if rs.pidRunDir != "" {
		pidFile = filepath.Join(rs.pidRunDir, fmt.Sprintf("%03d-%s.pid", seq, agentName))
	}
	return promptLog, outputFile, eventsLog, eventsTextLog, pidFile
}

// promptLogPath returns just the prompt-log slot of dispatchPaths.
// Retained for the test fixtures that pre-date the outputs / events
// channels and only assert against the prompt log path. Production
// callers use dispatchPaths to pair the five artefacts in one seq bump.
func (rs *runState) promptLogPath(agentName string) string {
	p, _, _, _, _ := rs.dispatchPaths(agentName)
	return p
}

// printAgentSummary writes the rs's recorded dispatches via the package
// renderer. No-op when rs is nil or has no recorded dispatches. Called
// from driver.Run's deferred tail so it fires on success AND on any
// error path, mirroring the existing per-dispatch banner's "always
// print" policy.
func (rs *runState) printAgentSummary(w io.Writer) {
	renderAgentSummary(w, rs.snapshotRecords())
}

// renderAgentSummary writes a per-agent table + totals row to w. One row
// per dispatch, in dispatch order. Columns:
//
//	#  agent  model  effort  elapsed  fresh  cached  out  cost
//
// `fresh` is input billed at ≥ full rate this turn (input + cache-creation),
// `cached` is the cheap cache-read reuse — split via clauderun.SplitInputTokens,
// the same helper the per-dispatch banner uses via formatUsageSuffix so the two
// views can't drift. `out` is output tokens; `cost` is the runner-reported
// total_cost_usd. Rows whose dispatch ran without a parsed envelope (interactive
// mode, or a headless run that crashed mid-stream) render fresh/cached/out/cost
// as "—".
// Failed dispatches get a "✗" prefix on the agent column so the operator
// can spot which row burned tokens without producing work.
//
// Single source of truth for the table shape — both the live banner
// (printAgentSummary method) and the historical replay (PrintSummaryFile)
// route through here so the two views stay byte-identical.
// summaryAgentLabel is the agent column's value for one row: the agent
// name, suffixed ` (attempt N/M)` when the dispatch was one pass of a
// max-visits loop (attemptMax > 0). Single-pass dispatches (attemptMax ==
// 0) render the bare agent name, so non-looped rows are unchanged. The
// leading dispatch-sequence `#` column stays orthogonal — attempt is a
// per-node loop position, not a global dispatch index. Shared by the
// width calc and the row printer so the column stays aligned.
func summaryAgentLabel(r dispatchRecord) string {
	if r.attemptMax > 0 {
		return fmt.Sprintf("%s (attempt %d/%d)", r.agent, r.attemptNumber, r.attemptMax)
	}
	return r.agent
}

func renderAgentSummary(w io.Writer, records []dispatchRecord) {
	if w == nil {
		return
	}
	if len(records) == 0 {
		return
	}

	// Column widths sized to the longest value in each column so the
	// table stays aligned regardless of which agents ran. Headers
	// participate in the width calc so a short-name run still has
	// readable column headings.
	agentW := len("agent")
	channelW := len("channel")
	modelW := len("model")
	effortW := len("effort")
	for _, r := range records {
		if w := len(summaryAgentLabel(r)) + 2; w > agentW { // +2 for "✗ " marker on failed rows
			agentW = w
		}
		if w := len(r.channel); w > channelW {
			channelW = w
		}
		if w := len(r.model); w > modelW {
			modelW = w
		}
		if w := len(r.effort); w > effortW {
			effortW = w
		}
	}

	var (
		totalElapsed time.Duration
		totalFresh   int
		totalCached  int
		totalOut     int
		totalCost    float64
		anyUsage     bool
	)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "=== Agent summary ===")
	fmt.Fprintf(w, "  #  %-*s  %-*s  %-*s  %-*s  %8s  %8s  %8s  %8s  %8s\n",
		agentW, "agent",
		channelW, "channel",
		modelW, "model",
		effortW, "effort",
		"elapsed", "fresh", "cached", "out", "cost")

	for i, r := range records {
		name := summaryAgentLabel(r)
		if r.err != nil {
			name = "✗ " + name
		}
		elapsed := r.elapsed.Round(time.Second).String()
		fresh, cached, out, cost := "—", "—", "—", "—"
		if r.usage != nil {
			freshTokens, cachedTokens := clauderun.SplitInputTokens(r.usage)
			outTokens := r.usage.OutputTokens
			if freshTokens > 0 || cachedTokens > 0 || outTokens > 0 || r.usage.TotalCostUSD > 0 {
				fresh = formatSummaryTokens(freshTokens)
				cached = formatSummaryTokens(cachedTokens)
				out = formatSummaryTokens(outTokens)
				cost = fmt.Sprintf("$%.2f", r.usage.TotalCostUSD)
				totalFresh += freshTokens
				totalCached += cachedTokens
				totalOut += outTokens
				totalCost += r.usage.TotalCostUSD
				anyUsage = true
			}
		}
		totalElapsed += r.elapsed
		fmt.Fprintf(w, "%3d  %-*s  %-*s  %-*s  %-*s  %8s  %8s  %8s  %8s  %8s\n",
			i+1,
			agentW, name,
			channelW, r.channel,
			modelW, r.model,
			effortW, r.effort,
			elapsed, fresh, cached, out, cost)
	}

	// Totals row.
	totalFreshStr, totalCachedStr, totalOutStr, totalCostStr := "—", "—", "—", "—"
	if anyUsage {
		totalFreshStr = formatSummaryTokens(totalFresh)
		totalCachedStr = formatSummaryTokens(totalCached)
		totalOutStr = formatSummaryTokens(totalOut)
		totalCostStr = fmt.Sprintf("$%.2f", totalCost)
	}
	fmt.Fprintf(w, "%3s  %-*s  %-*s  %-*s  %-*s  %8s  %8s  %8s  %8s  %8s\n",
		"", agentW, "", channelW, "", modelW, "", effortW, "totals",
		totalElapsed.Round(time.Second).String(),
		totalFreshStr, totalCachedStr, totalOutStr, totalCostStr)

	if !anyUsage {
		fmt.Fprintln(w, "(token + cost capture is headless-only; interactive runs show — for those columns)")
	}
}

// formatSummaryTokens renders a token count compactly for the agent
// summary table — `42` for sub-1k counts, `12.4k` for 1k+. Mirrors the
// shape clauderun's per-dispatch banner uses so the two views read the
// same. Duplicated locally rather than exported from clauderun because
// the helper is one line and exporting would invite drift.
func formatSummaryTokens(n int) string {
	if n < 1000 {
		return strconv.Itoa(n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// pruneOldRuns deletes all but the most recent (keep-1) directories in
// runsDir, sorted by mtime descending. The "-1" leaves room for the run
// we're about to create. Missing runsDir is a no-op (first-ever run on
// this consumer).
//
// Errors removing a single directory are surfaced back to the caller,
// which logs them as a warning — pruning is diagnostics, not load-bearing.
func pruneOldRuns(runsDir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	type entry struct {
		path  string
		mtime time.Time
	}
	var dirs []entry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, entry{
			path:  filepath.Join(runsDir, e.Name()),
			mtime: info.ModTime(),
		})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].mtime.After(dirs[j].mtime)
	})
	cutoff := keep - 1
	if cutoff < 0 {
		cutoff = 0
	}
	if len(dirs) <= cutoff {
		return nil
	}
	var firstErr error
	for _, d := range dirs[cutoff:] {
		if err := os.RemoveAll(d.path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// openFlowFile creates runDir (if absent) and truncates/opens
// <runDir>/flow.txt for the live execution-flow tree. runDir is the same
// per-run directory dispatchPaths writes the per-agent *.prompt.md into and
// the same one --keep-runs prunes, so flow.txt is covered by that pruning
// for free — no new pruning is added (Item 4). The 0o644 file + O_TRUNC
// mirror installLogFileMirror's --log-file handling.
func openFlowFile(runDir string) (*os.File, error) {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(runDir, "flow.txt"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
}

// headCommitSHA returns the short HEAD sha for the flow.txt footer, or "" on
// any error (no git, empty/detached repo). Best-effort: the SHA is footer
// garnish, never load-bearing, so a failure is swallowed silently rather
// than warned — unlike the file-open path, there is nothing the operator
// can act on.
func headCommitSHA(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// fullHeadSHA returns the full 40-char HEAD commit SHA, or "" when
// repoPath isn't a git repo (or git is unavailable). Captured at Run
// start as the base for the run-end commit list — full rather than
// abbreviated so the `base..HEAD` range and the compare URL are
// unambiguous.
func fullHeadSHA(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// commitsSince returns the commits reachable from HEAD but not from
// baseSHA — i.e. the commits created since baseSHA was captured — newest
// first, the same order GitHub lists a PR's commits. Returns nil when
// baseSHA is empty (no base was captured), when HEAD still equals baseSHA
// (the run committed nothing), or on any git error.
//
// The format string uses %x1f (unit separator) between fields and %x1e
// (record separator) between commits so subjects containing spaces, tabs,
// or em-dashes never confuse the split. Best-effort, mirroring
// headCommitSHA's fail-soft stance — the digest is diagnostic.
func commitsSince(repoPath, baseSHA string) []commitInfo {
	if strings.TrimSpace(baseSHA) == "" {
		return nil
	}
	cmd := exec.Command("git", "log",
		"--pretty=format:%h%x1f%s%x1f%an%x1f%cr%x1e",
		baseSHA+"..HEAD")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var commits []commitInfo
	for rec := range strings.SplitSeq(string(out), "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		fields := strings.Split(rec, "\x1f")
		if len(fields) < 4 {
			continue
		}
		commits = append(commits, commitInfo{
			shortSHA: fields[0],
			subject:  fields[1],
			author:   fields[2],
			relative: fields[3],
		})
	}
	return commits
}

// compareURL builds a GitHub "compare" link for the range the run
// committed (baseSHA…current-branch), or "" when there's nothing to link:
// no commits were made, no base was captured, or the repo has no
// recognizable GitHub remote. The head ref is the current branch name
// when on one (prettier, and tracks further pushes); it falls back to the
// abbreviated HEAD SHA in detached-HEAD state.
func compareURL(repoPath, baseSHA string, commitCount int) string {
	if commitCount == 0 || strings.TrimSpace(baseSHA) == "" {
		return ""
	}
	web := remoteWebURL(repoPath)
	if web == "" {
		return ""
	}
	head := currentBranch(repoPath)
	if head == "" || head == "HEAD" {
		if head = headCommitSHA(repoPath); head == "" {
			return ""
		}
	}
	return fmt.Sprintf("%s/compare/%s...%s", web, baseSHA, head)
}

// currentBranch returns the checked-out branch name, "HEAD" in detached-
// HEAD state, or "" on error.
func currentBranch(repoPath string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// remoteWebURL resolves the origin remote to its https browser URL,
// normalizing both transports — scp-style `git@github.com:owner/repo.git`
// and `https://github.com/owner/repo.git` both become
// `https://github.com/owner/repo`. Returns "" when there's no origin
// remote or the URL isn't a recognizable github.com remote (the compare
// link is GitHub-specific; a non-GitHub host gets no link rather than a
// broken one).
func remoteWebURL(repoPath string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	if repoPath != "" {
		cmd.Dir = repoPath
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(out))
	raw = strings.TrimSuffix(raw, ".git")
	switch {
	case strings.HasPrefix(raw, "git@github.com:"):
		return "https://github.com/" + strings.TrimPrefix(raw, "git@github.com:")
	case strings.HasPrefix(raw, "ssh://git@github.com/"):
		return "https://github.com/" + strings.TrimPrefix(raw, "ssh://git@github.com/")
	case strings.HasPrefix(raw, "https://github.com/"):
		return raw
	default:
		return ""
	}
}

// ensureGhOptivemGitignore appends ".gh-optivem/" as a line in
// <repoPath>/.gitignore when it isn't already present. Idempotent:
// existing matches (with or without trailing slash, with or without a
// leading "/") are accepted as already-ignoring. Creates .gitignore if
// missing. Used by both driver.Run (upgrade path for repos that pre-date
// this guardrail) and `gh optivem config init` (fresh consumer setup).
func ensureGhOptivemGitignore(repoPath string) error {
	return gitignore.EnsureLine(repoPath, ".gh-optivem/")
}
