// Package driver wires together the ATDD pipeline runtime: it loads the
// process-flow YAML, registers gates / actions / agents, applies override and
// verify decorators, and walks the named process end to end.
//
// The driver is deliberately thin — the heavy lifting lives in the runtime
// sub-packages (statemachine, gates, actions, verify, override, board,
// classify, release, clauderun). This file's job is to compose them and
// expose two run modes:
//
//   - Run with Options.IssueNum > 0 → implement-ticket mode: pre-resolve the
//     project item for the given issue, seed Context, and skip the picker by
//     starting the main process at MOVE_TICKET_IN_PROGRESS.
//   - Run with Options.IssueNum == 0 → manage-project mode: the YAML's main
//     process runs from START, picking the top Ready ticket from the project
//     board.
//
// Agent dispatch (v2): every user_task whose `agent:` value is something
// other than `human` shells out to the `claude` CLI via the clauderun
// package. clauderun reads the embedded per-agent prompt (from
// internal/atdd/runtime/agents/prompts/), substitutes ${name} placeholders
// from the live Context, and hands the rendered string to `claude -p` as
// the agent's full one-shot input — there is no parent-claude harness or
// Task-tool indirection. Success is detected by HEAD diff (a fresh commit
// on the same branch). v1's "pause and let the operator launch the agent
// in a second window" behaviour is preserved as a fallback under
// Options.ManualAgents — it lets us bisect "did v2 misroute the agent?"
// against "did v1 see the commit?" without two parallel binaries.
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/actions"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/override"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/trace"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/verify"
	"github.com/optivem/gh-optivem/internal/files"
	"github.com/optivem/gh-optivem/internal/promptio"
)

// DefaultProcessName is the entry process loaded by every public CLI command.
const DefaultProcessName = "main"

// Options bundles every driver knob that callers (the `gh optivem implement`
// command and tests) might want to set. Zero values yield a usable
// configuration: load the embedded canonical YAML, enter DefaultProcessName,
// no overrides, real shell-outs.
type Options struct {
	// YAMLPath, when non-empty, points the driver at an on-disk YAML file
	// instead of the canonical embedded document (statemachine.DefaultYAML).
	// Empty → load the embedded YAML via statemachine.LoadDefault.
	YAMLPath string

	// ProcessName is the entry process. Empty → DefaultProcessName.
	ProcessName string

	// IssueNum, when > 0, makes `gh optivem implement` skip the picker
	// (PICK_TOP_READY) and pre-resolve the project item for the given issue.
	// Zero (the default) keeps the picker in the flow.
	IssueNum int

	// ProjectURL overrides config-based project resolution. Optional; when
	// empty, board.ResolveProjectURL loads RepoPath/gh-optivem.yaml (or the
	// file passed via ConfigPath, if set) and reads `project.url`.
	ProjectURL string

	// RepoPath overrides the working directory used for project resolution
	// and for shell-outs. Optional; defaults to cwd.
	RepoPath string

	// Autonomous skips human-approval STOPs. In v2 it also flips agent
	// dispatch into headless `claude -p` mode. Default (false) runs
	// `claude` interactively so the operator can observe / interject.
	Autonomous bool

	// ManualAgents falls back to the v1 "pause and let the operator
	// launch the agent in a second window" behaviour at every user_task
	// dispatch. Default (false) shells out to the `claude` CLI via the
	// clauderun package. ManualAgents is mutually exclusive with the
	// override hooks (the prompt-construction layer is what consumes
	// them — bypass that and they have nothing to attach to).
	ManualAgents bool

	// Override holds the per-node override hooks (Extra / Replace).
	// Populated by the cobra layer from gh-optivem.yaml's node_extras: /
	// node_replacements: fields. nil leaves the dispatcher unmodified.
	Override *override.Hooks

	// AgentPromptOverrides is a map from embedded-agent name (e.g.
	// "atdd-test") to a prompt body that replaces the canonical embedded
	// prompt for that agent. Sourced from gh-optivem.yaml's agent_prompts:
	// map by the cobra layer; the values are the file contents, not the
	// file paths (the CLI reads at startup so missing-file failures surface
	// there). Unrecognised agent names are rejected at projectconfig.Validate.
	AgentPromptOverrides map[string]string

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

	// Stdout / Stderr are the diagnostic targets. nil → os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Stdin is the agent-dispatch pause reader. nil → os.Stdin.
	Stdin io.Reader
}

// Run loads the YAML, wires the registries, applies decorators, optionally
// pre-resolves an issue, and walks the chosen process.
func Run(ctx context.Context, opts Options) error {
	opts = opts.withDefaults()

	// Pre-flight the `claude` CLI when subprocess dispatch is enabled,
	// so missing-binary or missing-credentials failures surface at
	// startup instead of after several service-task spinners scroll by.
	// Skipped under --manual-agents (the v1 fallback that doesn't need
	// the CLI at all).
	if !opts.ManualAgents {
		if err := preflightFn(ctx); err != nil {
			return fmt.Errorf("driver: %w", err)
		}
	}

	var eng *statemachine.Engine
	var err error
	if opts.YAMLPath == "" {
		eng, err = statemachine.LoadDefault()
		if err != nil {
			return fmt.Errorf("driver: load embedded YAML: %w", err)
		}
	} else {
		eng, err = statemachine.LoadFile(opts.YAMLPath)
		if err != nil {
			return fmt.Errorf("driver: load YAML %q: %w", opts.YAMLPath, err)
		}
	}

	process, ok := eng.Processes[opts.ProcessName]
	if !ok {
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
	// without this, MOVE_TICKET_IN_PROGRESS and friends fall back to
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

	gateReg := gates.New()
	gates.RegisterAll(gateReg, gates.Deps{})

	actionReg := actions.New()
	actions.RegisterAll(actionReg, actions.Deps{
		ProjectURL: resolvedProjectURL,
		RepoPath:   opts.RepoPath,
		Autonomous: opts.Autonomous,
	})

	agentReg := agents.New()
	registerAgentDispatchers(agentReg)

	eng.GateFn = gateReg.Lookup
	eng.ActionFn = actionReg.Lookup
	eng.AgentFn = agentReg.Lookup

	if err := eng.Bind(); err != nil {
		return fmt.Errorf("driver: bind engine: %w", err)
	}

	// Per-run diagnostic state: timestamp + monotonic dispatch counter,
	// shared by every dispatcher closure registered by wrapAgentDispatchers.
	// Used to compose <run-ts>/<seq>-<agent>.prompt.md log paths so files
	// sort in dispatch order regardless of clock granularity.
	runState := &runState{
		runTimestamp: nowFn().UTC().Format("20060102-150405"),
		repoPath:     repoPath,
	}

	// Post-Bind decoration order matters:
	//   1. Wrap user_task agent dispatch with per-node info-printer (uses
	//      RawNode metadata only available after Bind).
	//   2. Apply verify pre/post-condition decorators (commit-message HEAD
	//      checks).
	//   3. Apply override hooks — they sit on top of verify so a
	//      node_replacements: swap short-circuits both the verify check
	//      and the agent dispatcher (the documented escape-hatch behaviour).
	//   4. Apply the trace decorator last so its entry/exit lines bracket
	//      every other decorator's behaviour. The operator sees the
	//      composed call as one node fire.
	wrapAgentDispatchers(eng, opts, runState)
	verify.WrapAll(eng, verify.Deps{})
	wrapOverride(eng, opts.Override)

	logClose, err := installLogFileMirror(&opts)
	if err != nil {
		return fmt.Errorf("driver: %w", err)
	}
	defer logClose()

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
	trace.WrapAll(eng, trace.Deps{
		Out:      opts.Stdout,
		RepoPath: repoPath,
	})
	printConfig(opts.Stdout, opts, cfg, repoPath)

	sCtx := statemachine.NewContext()
	seedScopeState(sCtx, cfg)
	if opts.ProjectURL != "" {
		sCtx.Set("project_url", opts.ProjectURL)
	}

	if opts.IssueNum > 0 {
		if err := preResolveIssue(ctx, opts, sCtx, cfg); err != nil {
			return fmt.Errorf("driver: pre-resolve issue #%d: %w", opts.IssueNum, err)
		}
		// Skip START → PICK_TOP_READY when running main. The pre-resolution
		// has already populated everything PICK_TOP_READY would have set;
		// MOVE_TICKET_IN_PROGRESS is the next service task downstream.
		if opts.ProcessName == DefaultProcessName {
			process.Start = "MOVE_TICKET_IN_PROGRESS"
		}
	}

	return eng.RunProcess(opts.ProcessName, sCtx)
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

// installLogFileMirror opens opts.LogFile (when non-empty), wraps
// opts.Stdout and opts.Stderr to tee into it, and returns a close func
// the caller defers. When opts.LogFile is empty, the writers are left
// untouched and the close func is a no-op.
//
// The mutation is in-place on the caller's Options so every downstream
// site that already reads opts.Stdout / opts.Stderr (printConfig, the
// resolve-issue banner, the trace decorator, every clauderun.Dispatch
// invocation that pulls Stdout/Stderr from Options) automatically gains
// file-mirroring without a per-call-site change.
func installLogFileMirror(opts *Options) (func(), error) {
	if opts.LogFile == "" {
		return func() {}, nil
	}
	f, err := os.OpenFile(opts.LogFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return func() {}, fmt.Errorf("open log file %s: %w", opts.LogFile, err)
	}
	opts.Stdout = io.MultiWriter(opts.Stdout, f)
	opts.Stderr = io.MultiWriter(opts.Stderr, f)
	return func() { f.Close() }, nil
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
		cfg, err := projectconfig.Load(repoPath)
		if err != nil {
			return nil, fmt.Errorf("driver: %w", err)
		}
		return cfg, nil
	}
	cfg, err := projectconfig.LoadFromPath(configPath)
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
		}
		if !cfg.SystemTest.IsEmpty() {
			fmt.Fprintf(w, "  system_test:   %s (lang: %s, repo: %s)\n",
				cfg.SystemTest.Path, cfg.SystemTest.Lang, cfg.SystemTest.Repo)
		}
		if !cfg.ExternalSystems.Stubs.IsEmpty() || !cfg.ExternalSystems.Simulators.IsEmpty() {
			if !cfg.ExternalSystems.Stubs.IsEmpty() {
				fmt.Fprintf(w, "  ext stubs:     %s (repo: %s)\n",
					cfg.ExternalSystems.Stubs.Path, cfg.ExternalSystems.Stubs.Repo)
			}
			if !cfg.ExternalSystems.Simulators.IsEmpty() {
				fmt.Fprintf(w, "  ext sims:      %s (repo: %s)\n",
					cfg.ExternalSystems.Simulators.Path, cfg.ExternalSystems.Simulators.Repo)
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
// ${repo_strategy}, ${repos}, ${architecture}, and ${allowed_roots}.
// ${allowed_roots} is a pre-rendered multi-line block listing the paths
// the agent is allowed to write into; the rendering happens here once
// per run rather than at every dispatch site. Empty values are left
// absent. nil cfg is a no-op.
//
// State (not Params) is the right destination: these four facts are
// project-scoped and stable for the entire run, alongside issue_title /
// project_title / ticket_checklist (also written via Set). The
// dispatcher reads them back via ctx.GetString, which is a State lookup
// — writing to Params would silently expand to "" at substitution time.
func seedScopeState(sCtx *statemachine.Context, cfg *projectconfig.Config) {
	if cfg == nil {
		return
	}
	if cfg.RepoStrategy != "" {
		sCtx.Set("repo_strategy", cfg.RepoStrategy)
	}
	if repos := cfg.Repos(); len(repos) > 0 {
		sCtx.Set("repos", strings.Join(repos, ","))
	}
	if cfg.System.Architecture != "" {
		sCtx.Set("architecture", cfg.System.Architecture)
	}
	if rendered := renderAllowedRoots(cfg); rendered != "" {
		sCtx.Set("allowed_roots", rendered)
	}
}

// renderAllowedRoots produces the multi-line "Allowed write roots" block
// the atdd-task / atdd-chore prompts substitute via ${allowed_roots}.
// The block lists every tier the agent is allowed to edit, plus a
// separate external-systems section when those are declared.
//
// Returns "" when cfg has no architecture (the caller leaves the param
// unset, and the template variable expands to "").
func renderAllowedRoots(cfg *projectconfig.Config) string {
	if cfg == nil || cfg.System.Architecture == "" {
		return ""
	}
	var b strings.Builder

	// System + tests block.
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		fmt.Fprintf(&b, "- System: %s (lang: %s)\n",
			cfg.System.Path, cfg.System.Lang)
	case projectconfig.ArchMultitier:
		fmt.Fprintf(&b, "- Backend: %s (lang: %s)\n",
			cfg.System.Backend.Path, cfg.System.Backend.Lang)
		fmt.Fprintf(&b, "- Frontend: %s (lang: %s)\n",
			cfg.System.Frontend.Path, cfg.System.Frontend.Lang)
	}
	if !cfg.SystemTest.IsEmpty() {
		fmt.Fprintf(&b, "- System tests: %s (lang: %s)\n",
			cfg.SystemTest.Path, cfg.SystemTest.Lang)
	}

	// External-systems block — only when declared. Stubs first (cycle 2),
	// simulators second (cycle 3).
	ext := cfg.ExternalSystems
	if !ext.Stubs.IsEmpty() || !ext.Simulators.IsEmpty() {
		b.WriteString("\nExternal-system roots (modify only when the ticket calls for stub/sim changes):\n")
		if !ext.Stubs.IsEmpty() {
			fmt.Fprintf(&b, "- Stubs: %s\n", ext.Stubs.Path)
		}
		if !ext.Simulators.IsEmpty() {
			fmt.Fprintf(&b, "- Simulators: %s\n", ext.Simulators.Path)
		}
	}

	return b.String()
}

// preflightFn is the per-Run function that verifies the `claude` CLI
// is on PATH and authenticated. Production points at preflightClaude;
// tests can swap it for a no-op or canned-error stub. The seam is a
// package-level var rather than an Options field because pre-flight is
// a startup-time concern, not part of the per-run dispatch surface.
var preflightFn = preflightClaude

// preflightClaude runs `claude --no-update-check --version` as a cheap
// health check at driver startup. Failure surfaces with operator
// guidance pointing at the auth bootstrap doc — without this, missing
// credentials manifest as a confusing "exited non-zero" several
// service-task spinners deep into the run.
func preflightClaude(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "claude", "--no-update-check", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		tail := strings.TrimSpace(stderr.String())
		if tail == "" {
			tail = err.Error()
		}
		return fmt.Errorf(
			"claude CLI pre-flight failed: %s\n  Ensure `claude` is on PATH and authenticated via `claude /login` (credentials live in ~/.claude/).\n  Use --manual-agents to fall back to the v1 two-window workflow without the CLI.",
			tail)
	}
	return nil
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
	return o
}

// preResolveIssue populates Context with everything PICK_TOP_READY would
// have set, reading from `gh project view` + `gh project item-list` (via
// board.FindIssue). Called once at driver startup in implement-ticket mode.
// cfg is the pre-loaded project config (may be nil if no gh-optivem.yaml and
// no --config); supplied by Run so the load happens once per driver
// invocation.
func preResolveIssue(ctx context.Context, opts Options, sCtx *statemachine.Context, cfg *projectconfig.Config) error {
	projectURL := opts.ProjectURL
	if projectURL == "" {
		resolved, err := board.ResolveProjectURLFromConfig(cfg, opts.ConfigPath)
		if err != nil {
			return fmt.Errorf("resolve project URL: %w", err)
		}
		projectURL = resolved
	}
	sCtx.Set("project_url", projectURL)

	pick, err := board.FindIssue(ctx, opts.IssueNum, board.Options{
		ProjectURL: projectURL,
		RepoPath:   opts.RepoPath,
	})
	if err != nil {
		return err
	}
	projectName := pick.ProjectTitle

	sCtx.Set("issue_num", strconv.Itoa(pick.IssueNum))
	sCtx.Set("issue_url", pick.IssueURL)
	sCtx.Set("issue_title", pick.Title)
	sCtx.Set("issue_repo", pick.Repo)
	sCtx.Set("project_id", pick.ProjectID)
	sCtx.Set("project_title", projectName)
	sCtx.Set("item_id", pick.ItemID)

	projectLabel := projectURL
	if projectName != "" {
		projectLabel = fmt.Sprintf("%s (%s)", projectName, projectURL)
	}
	fmt.Fprintf(opts.Stdout, "Resolved issue #%d %q (%s) on project %s.\n",
		pick.IssueNum, pick.Title, pick.Repo, projectLabel)
	return nil
}

// registerAgentDispatchers registers a no-op base dispatcher for every
// agent that has an embedded prompt (filesystem walk via agents.Names).
// The substantive prompt-and-pause behaviour is layered on after Bind by
// wrapAgentDispatchers, which has access to per-node RawNode metadata
// (description, phase_doc). Adding a new agent is now: drop a prompt
// under internal/atdd/runtime/agents/prompts/, recompile.
func registerAgentDispatchers(r *agents.Registry) {
	noop := func(ctx *statemachine.Context) statemachine.Outcome {
		return statemachine.Outcome{}
	}
	for _, name := range agents.Names() {
		r.Register(name, noop)
	}
}

// wrapAgentDispatchers replaces every user_task NodeFn with a
// per-node closure that has access to the YAML node's Raw metadata
// (description, phase_doc, agent name) and the per-run Options:
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
func wrapAgentDispatchers(eng *statemachine.Engine, opts Options, rs *runState) {
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
			case raw.Agent == "human":
				node.Fn = newHumanStopDispatcher(opts, raw, nodeID)
			case opts.ManualAgents:
				node.Fn = newManualAgentDispatcher(opts, raw, inner)
			default:
				node.Fn = newClaudeRunDispatcher(opts, raw, rs, inner)
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
		description := statemachine.ExpandParams(raw.Documentation, ctx.Params)

		fmt.Fprintln(opts.Stdout)
		if description != "" {
			fmt.Fprintf(opts.Stdout, "[%s] %s\n", nodeID, description)
		} else {
			fmt.Fprintf(opts.Stdout, "[%s] STOP\n", nodeID)
		}
		ok, err := promptio.ConfirmYN(opts.Stdin, opts.Stdout, "  Approve?")
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("read STOP confirmation at %s: %w", nodeID, err)}
		}
		if !ok {
			return statemachine.Outcome{Err: fmt.Errorf("user aborted at %s", nodeID)}
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
		if err := promptForAgent(opts, raw, ctx.Params); err != nil {
			return statemachine.Outcome{Err: err}
		}
		return inner(ctx)
	}
}

// newClaudeRunDispatcher returns the v2 dispatcher. It reads the override
// hints written to the Context state by override.Wrap, pulls the ticket
// fields populated by preResolveIssue / PICK_TOP_READY, and hands the lot
// to clauderun.Dispatch. The agent does not commit; the wrapping CLI
// stages and commits the working-tree delta after dispatch returns.
//
// rs supplies the per-dispatch PromptLogPath. nil rs (only happens in
// tests today) skips the log — clauderun treats empty PromptLogPath as
// "no diagnostics file".
func newClaudeRunDispatcher(opts Options, raw statemachine.RawNode, rs *runState, inner statemachine.NodeFn) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		extraText := ctx.GetString(override.KeyExtra)
		replaceText := ctx.GetString(override.KeyReplace)

		issueNum, _ := strconv.Atoi(ctx.GetString("issue_num"))

		agentName := statemachine.ExpandParams(raw.Agent, ctx.Params)
		// Node-level params (e.g. `failure_type: compile` on FIX_COMPILE)
		// are expanded against the live ctx scope and forwarded to the
		// prompt renderer so the agent body can branch on per-call-site
		// labels without a dedicated Options field per agent.
		var nodeParams map[string]string
		if len(raw.Params) > 0 {
			nodeParams = make(map[string]string, len(raw.Params))
			for k, v := range raw.Params {
				nodeParams[k] = statemachine.ExpandParams(v, ctx.Params)
			}
		}
		cOpts := clauderun.Options{
			Agent:           agentName,
			PhaseDoc:        statemachine.ExpandParams(raw.PhaseDoc, ctx.Params),
			NodeDescription: statemachine.ExpandParams(raw.Documentation, ctx.Params),
			IssueNum:        issueNum,
			IssueTitle:      ctx.GetString("issue_title"),
			IssueRepo:       ctx.GetString("issue_repo"),
			ProjectTitle:    ctx.GetString("project_title"),
			ProjectURL:      ctx.GetString("project_url"),
			Architecture:    ctx.GetString("architecture"),
			AllowedRoots:    ctx.GetString("allowed_roots"),
			Checklist:       ctx.GetString("ticket_checklist"),
			VerifyResults:   ctx.GetString("verify_results_text"),
			ChangedFiles:    fixVerifyChangedFiles(agentName, opts.RepoPath),
			NodeParams:      nodeParams,
			OverrideText:    extraText,
			RawPrompt:       replaceText,
			PromptOverride:  opts.AgentPromptOverrides[agentName],
			Autonomous:      opts.Autonomous,
			ShowPrompt:      opts.ShowPrompt,
			PromptLogPath:   rs.promptLogPath(agentName),
			RepoPath:        opts.RepoPath,
			Stdout:          opts.Stdout,
			Stderr:          opts.Stderr,
			Stdin:           opts.Stdin,
		}

		if err := clauderun.Dispatch(context.Background(), opts.ClaudeRunDeps, cOpts); err != nil {
			return statemachine.Outcome{Err: err}
		}
		return inner(ctx)
	}
}

// fixVerifyChangedFiles returns the working-tree dirty-file listing
// (one path per line) the dispatcher passes into atdd-fix-verify's
// ${changed_files} placeholder. We only shell out for that one agent
// because it is the only one whose prompt template references the
// substitution — every other dispatch leaves the placeholder out of
// the template anyway, so paying for a `git status` on every node
// would be wasted work.
//
// On any shell error (no git in PATH, not a repo, …) we return the
// empty string. The fix-verify prompt simply renders an empty
// "Changed files" block; the agent can re-run `git status` itself if
// it needs the listing. The dispatch is feedback, not load-bearing.
func fixVerifyChangedFiles(agent, repoPath string) string {
	if agent != "atdd-fix-verify" {
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
// params is the live Context.Params for the call_activity scope; templated
// fields in raw (e.g. ${agent} / ${phase_doc} / ${change_type} inside the shared
// structural_cycle) are expanded against it so the operator sees the
// substituted name in the banner instead of the literal placeholder.
func promptForAgent(opts Options, raw statemachine.RawNode, params map[string]string) error {
	agent := statemachine.ExpandParams(raw.Agent, params)
	phaseDoc := statemachine.ExpandParams(raw.PhaseDoc, params)
	documentation := statemachine.ExpandParams(raw.Documentation, params)
	step := raw.ID

	fmt.Fprintln(opts.Stdout)
	fmt.Fprintf(opts.Stdout, "DISPATCH: %s\n", agent)
	if step != "" {
		fmt.Fprintf(opts.Stdout, "  Step: %s\n", step)
	}
	if documentation != "" {
		fmt.Fprintf(opts.Stdout, "  Phase: %s\n", documentation)
	}
	if phaseDoc != "" {
		fmt.Fprintf(opts.Stdout, "  Phase doc: %s\n", phaseDoc)
	}
	fmt.Fprintf(opts.Stdout, "  Launch the %s agent now (e.g. via the Task tool in Claude Code).\n", agent)
	fmt.Fprintln(opts.Stdout, "  When the agent's COMMIT lands on HEAD, approve to continue.")

	ok, err := promptio.ConfirmYN(opts.Stdin, opts.Stdout, "  Approve?")
	if err != nil {
		return fmt.Errorf("read agent-dispatch confirmation: %w", err)
	}
	if !ok {
		return fmt.Errorf("operator aborted at %s dispatch", agent)
	}
	return nil
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
type runState struct {
	runTimestamp string
	repoPath     string
	seq          atomic.Int64
}

// promptLogPath composes <repoPath>/.gh-optivem/runs/<run-ts>/<seq>-<agent>.prompt.md
// for the current dispatch. Bumps the per-run sequence counter so log
// files sort in dispatch order regardless of clock granularity.
//
// Returns empty when rs is nil — used by tests that bypass the
// driver-managed runState; clauderun treats an empty PromptLogPath as
// "skip the log".
func (rs *runState) promptLogPath(agentName string) string {
	if rs == nil {
		return ""
	}
	seq := rs.seq.Add(1)
	filename := fmt.Sprintf("%03d-%s.prompt.md", seq, agentName)
	return filepath.Join(rs.repoPath, ".gh-optivem", "runs", rs.runTimestamp, filename)
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

// ensureGhOptivemGitignore appends ".gh-optivem/" as a line in
// <repoPath>/.gitignore when it isn't already present. Idempotent:
// existing matches (with or without trailing slash, with or without a
// leading "/") are accepted as already-ignoring. Creates .gitignore if
// missing. Used by both driver.Run (upgrade path for repos that pre-date
// this guardrail) and `gh optivem config init` (fresh consumer setup).
func ensureGhOptivemGitignore(repoPath string) error {
	return files.EnsureGitignoreLine(repoPath, ".gh-optivem/")
}
