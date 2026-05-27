// Package clauderun shells out to the `claude` CLI to dispatch a named ATDD
// agent for the current phase, replacing v1's "pause and let the operator
// launch the agent in a second window" workflow.
//
// Dispatch reads the embedded per-agent prompt (see internal/atdd/runtime/
// agents/embed.go), substitutes ${name} placeholders against the live ticket
// context, invokes `claude` (interactive or `claude -p` autonomous), and
// returns when the subprocess exits. The agent is instructed not to commit;
// staging and committing is the wrapping CLI's responsibility, after the
// dispatch returns and any human gates have fired.
//
// v2 architectural note: there is no parent-claude harness or Task-tool
// indirection. The rendered prompt IS the agent's full one-shot input —
// `claude -p` runs the agent's instructions directly.
//
// The package exposes a ClaudeRunner / GitRunner pair so tests can inject
// canned exit codes and HEAD values; the production defaults exec the real
// CLIs the same way the gates / actions / classify / release packages do.
package clauderun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	assetsync "github.com/optivem/gh-optivem/internal/assets/sync"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/expand"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Options bundles every input Dispatch needs to construct a prompt and run
// the subprocess. Zero values yield a usable configuration where it makes
// sense (Stdout/Stderr/Stdin default to the OS streams). Required fields
// (Agent, IssueNum, IssueTitle, NodeDescription) are
// not zero-defaulted because missing them yields a meaningless prompt.
type Options struct {
	// Agent is the subagent name to launch (e.g. "write-acceptance-tests").
	Agent string

	// NodeDescription is the YAML node's `name:` — surfaced in the
	// prompt so the agent has the same context the operator would have read.
	NodeDescription string

	// Ticket context — pulled from Context keys populated by preResolveIssue.
	IssueNum   int
	IssueTitle string

	// TicketID is the tracker-verbatim id (issue.ID), seeded by
	// writeResolvedIssue alongside IssueNum. Same value as IssueNum today,
	// but the dispatcher exposes it under the backend-agnostic ${ticket-id}
	// placeholder so prompts that compose user-visible disable-reason
	// strings (test-disabler, test-enabler) stay neutral on whether the
	// tracker is GitHub-numeric or Jira-prefixed. Load-bearing: when empty
	// AND the prompt references ${ticket-id}, findUnfilledPlaceholders
	// fails the dispatch fast — same rationale as Language / Checklist.
	TicketID string

	// Architecture is "monolith" or "multitier", surfaced to the agent
	// prompt via ${architecture}. Empty when no system.architecture is
	// declared in gh-optivem.yaml.
	Architecture string

	// Subtype is the ticket's structural subtype label (e.g.
	// "system-interface-redesign", "external-system-interface-redesign"),
	// sourced from ctx.State["subtype"] populated by CLASSIFY_TICKET_SUBTYPE.
	// Substituted into the prompt as ${subtype} AND used to gate
	// <!-- if:subtype=VALUE -->...<!-- end-if --> blocks so subtype-irrelevant
	// reference material is stripped before the prompt reaches the runner.
	// Empty when the dispatcher fires outside a classified-ticket flow.
	Subtype string

	// ScopeRead / ScopeWrite are the per-phase scope lists sourced from the
	// BPMN node's inline `read:` / `write:` (plan 20260526-1448 Item 4 +
	// spinoff 1536). The driver looks them up via engine.Scope at dispatch
	// time, joins each key against ProjectConfig.PlaceholderMap() at render
	// time, and the result is substituted into the prompt body via the
	// ${scope-block} placeholder.
	//
	// Replaces the v1 ${allowed_roots} mechanism (which rendered a flat
	// write-only block from projectconfig once per run). The new shape
	// carries both read and write sets and varies per phase node, so the
	// agent sees only what its current phase scope covers — not the
	// project's full path inventory.
	//
	// Load-bearing: when both lists are non-empty the renderer registers
	// ${scope-block}; when empty (e.g. `scope: none` phases or command-only
	// MIDs) the placeholder is left unfilled and findUnfilledPlaceholders
	// fails the dispatch fast if the prompt body references it.
	ScopeRead  []string
	ScopeWrite []string

	// Checklist is the body of the ticket's Checklist section as parsed
	// by intake.ParseSections (populated by the parse-ticket service-task
	// into ctx.State["checklist"]). Surfaced to the agent prompt
	// via the ${checklist} placeholder so structural-task agents don't
	// have to re-fetch the issue body via `gh issue view`. Empty when
	// the ticket has no Checklist (story / bug / legacy-coverage paths).
	//
	// Load-bearing: when empty AND the prompt body references
	// ${checklist}, findUnfilledPlaceholders fails the dispatch fast.
	// Mirrors the ${acceptance-criteria} pattern — only registered when
	// non-empty so the unfilled-placeholder check is the single
	// enforcement point that catches "ticket-kind expects a Checklist
	// but the body has none" at the dispatcher rather than letting the
	// agent see an empty section.
	Checklist string

	// AcceptanceCriteria is the body of the ticket's Acceptance Criteria
	// section as parsed by intake.Parse. Surfaced to the agent prompt via
	// the ${acceptance-criteria} placeholder so write-acceptance-tests can read the
	// scenarios intake already extracted instead of shelling out to
	// `gh issue view`. Load-bearing: when empty AND the prompt body
	// references ${acceptance-criteria}, findUnfilledPlaceholders fails
	// the dispatch fast — WRITE-ACCEPTANCE-TESTS is contractually scoped to
	// implementing AC, so an absent value is a wiring bug, not a valid
	// state. Mirrors the ${language} pattern: only registered when
	// non-empty so the unfilled-placeholder check is the single
	// enforcement point.
	AcceptanceCriteria string

	// ParsedConcepts is the absolute path to the parsed-concepts artifact
	// materialize_parsed_concepts writes at the start of the
	// backlog_refinement sub-process. Substituted into
	// refine-acceptance-criteria's ${parsed-concepts} placeholder so the
	// agent has a stable file path for the parsed ACs across the
	// CONFIRM_REFINEMENT human gate. Load-bearing: only registered when
	// non-empty so an absent value surfaces via
	// findUnfilledPlaceholders rather than silently substituting "" —
	// same rationale as Language / AcceptanceCriteria.
	ParsedConcepts string

	// VerifyResults is the formatted block describing every red-class
	// verifyCommandResult the most recent RUN_TESTS produced.
	// Substituted into fix-unexpected-passing-tests' / fix-unexpected-failing-tests' ${verify-results} placeholder so
	// the fix agent reads the same captured runner output the operator
	// saw inline. Empty for every other agent — the rendered prompt
	// just leaves the placeholder verbatim, which is harmless because
	// no other agent's prompt references this name.
	VerifyResults string

	// ChangedFiles is the working-tree diff (as `git status --porcelain`)
	// at the moment of dispatch. Substituted into fix-unexpected-passing-tests' / fix-unexpected-failing-tests' / fix-command-failed
	// ${changed-files} placeholder so the fix agent can scope its
	// reasoning to "what the WRITE phase just edited" without re-running
	// `git status`. Empty when the dispatcher couldn't shell out (e.g.
	// tests with no Git seam).
	ChangedFiles string

	// CommandLine / CommandExitCode / CommandStderrTail carry the
	// diagnostic payload runCommand stashes in ctx.State on shell failure
	// (`command-line`, `command-exit-code`, `command-stderr-tail`). The
	// driver reads those keys and populates these fields ahead of every
	// claude dispatch; on a non-failed run, ctx.State has nothing under
	// those keys and the fields stay zero-valued.
	//
	// Load-bearing for fix-command-failed: the prompt body references
	// ${command}, ${command-exit-code}, ${command-stderr-tail}. CommandLine
	// is registered only when non-empty (mirroring Checklist /
	// AcceptanceCriteria) so findUnfilledPlaceholders fails the dispatch
	// fast when the recovery path tries to render fix-command-failed
	// with an empty command — that means the state-fallback path in
	// ExpandParams broke or runCommand silently skipped its stash, and
	// the operator wants to know now rather than after the agent reads a
	// blank prompt. CommandExitCode + CommandStderrTail piggy-back on
	// CommandLine's non-emptiness (they're a unit — failure either
	// populates all three or none).
	//
	// Other prompts don't reference these placeholders, so populating
	// the fields on every dispatch (rather than agent-gating in the
	// driver) is a no-op outside the recovery path.
	CommandLine        string
	CommandExitCode    int
	CommandStderrTail  string

	// FailingTaskName / MissingOutputs / ViolatingPaths carry the
	// diagnostic payload validateOutputsAndScopes stashes in ctx.State on
	// validation failure (`failing-task-name`, `missing-outputs`,
	// `scope-violating-paths`). The driver reads those keys and populates
	// these fields ahead of every claude dispatch; on a non-failed run,
	// ctx.State has nothing under those keys and the fields stay
	// zero-valued.
	//
	// Load-bearing for fix-missing-output / fix-scope-diff: their prompt
	// bodies reference ${failing-task-name} + (${missing-outputs} or
	// ${violating-paths}). Each placeholder is registered in
	// renderPrompt only when its source field is non-empty (mirroring
	// CommandLine), so findUnfilledPlaceholders fails the dispatch fast
	// when the recovery path tries to render either prompt with empty
	// state — that means validateOutputsAndScopes silently skipped its
	// stash, and the operator wants to know now rather than after the
	// agent reads a blank prompt.
	//
	// Other prompts don't reference these placeholders, so populating
	// the fields on every dispatch is a no-op outside the recovery path.
	FailingTaskName     string
	MissingOutputs      string
	ViolatingPaths      string

	// Language is the target language for this dispatch (e.g. "go",
	// "typescript"). Substituted into the prompt's ${language} placeholder
	// so per-language reference docs (under
	// ~/.gh-optivem/references/code/language-equivalents/<lang>.md)
	// resolve to the right slice. The
	// driver chooses the value per phase — backend phases pass the
	// backend lang, frontend phases the frontend lang. Load-bearing:
	// when empty AND the prompt body references ${language},
	// findUnfilledPlaceholders fails the dispatch fast rather than
	// silently substituting an empty path.
	Language string

	// NodeParams carries the YAML node's `params:` block (already expanded
	// against ctx.Params at dispatch time). Substituted into the prompt
	// template by name — e.g. `failure_type: compile` on the FIX_COMPILE
	// node surfaces as ${failure_type} = "compile" in the rendered prompt.
	// This is how per-call-site labels reach the agent body without an
	// agent-specific Options field. Lower precedence than the fixed-schema
	// placeholders (issue-num, issue-title, phase, …) — those win on key
	// collision.
	NodeParams map[string]string

	// Placeholders carries the project-wide ${name} substitutions the
	// dispatcher pulls from ProjectConfig.PlaceholderMap() — Family B
	// path keys (driver-port, driver-adapter, at-test, …) plus the
	// derived Family A keys (sut-namespace, system-path, system-test-path,
	// architecture, language). Inlined phase-doc placeholders that used
	// to be resolved at materialization time now live in the prompt body
	// itself; this map is how the dispatcher gets them filled at render
	// time. Lowest precedence — NodeParams and the fixed-schema set
	// (architecture, language, scope-block, …) win on key collision so
	// existing per-dispatch overrides keep their meaning.
	Placeholders map[string]string

	// OverrideText is the per-node extra text from override.Hooks.Extra
	// (sourced from gh-optivem.yaml's node_extras:), interpolated into the
	// prompt template. Empty string is fine.
	OverrideText string

	// RawPrompt, when non-empty, replaces the templated prompt entirely.
	// Used by override.Hooks.Replace (sourced from gh-optivem.yaml's
	// node_replacements:) where the operator wants to swap the whole prompt
	// rather than append text via OverrideText. When set, every other
	// prompt-shaping field (NodeDescription, OverrideText, …) is ignored.
	RawPrompt string

	// PromptOverride, when non-empty, replaces the embedded MID task prompt
	// body (i.e. agents.Prompt(opts.Agent)) with this string. Unlike
	// RawPrompt, the override still goes through ${name} expansion against
	// the live ticket context and still has OverrideText appended. Sourced
	// from gh-optivem.yaml's task_prompts: map, where the operator wants
	// to swap the canonical prompt for one named task without touching
	// the surrounding render machinery.
	PromptOverride string

	// Autonomous — when true, run via `claude -p` (one-shot, headless).
	// When false, run interactively so the operator can observe / interject.
	Autonomous bool

	// Model, when non-empty, becomes the `--model` flag on the claude
	// invocation (e.g. "sonnet", "haiku", "opus", or a full model id like
	// "claude-sonnet-4-6"). Empty → no flag, claude inherits the user's
	// session default. Per-agent tuning lets mechanical-scaffolding agents
	// (e.g. write-acceptance-tests PROTOTYPES) skip the Opus tax that creative agents
	// (e.g. fix-unexpected-failing-tests) actually need.
	Model string

	// Effort, when non-empty, becomes the `--effort` flag on the claude
	// invocation (low / medium / high / xhigh / max). Empty → no flag.
	// Lowering effort caps the per-turn thinking budget — the right knob
	// for agents that produce many small sequential tool calls, where the
	// per-call thinking is the dominant token cost.
	Effort string

	// PromptLogPath, when non-empty, is the file path Dispatch writes the
	// rendered prompt to (creating parent dirs as needed) before invoking
	// the runner. The driver computes a per-run-and-dispatch path so the
	// log persists after the run, unlike the materializePrompt tempfile
	// (deleted on dispatch exit). I/O failure is a non-fatal warning to
	// Stderr — diagnostics shouldn't break the dispatch.
	PromptLogPath string

	// OutputFilePath is the absolute path to the JSONL file the agent's
	// `gh optivem output write` invocations append to. Exported into the
	// subprocess env as GH_OPTIVEM_OUTPUT_FILE. The file is NOT pre-created
	// — when the agent makes no writes, the file simply does not exist
	// after the run (the dispatcher's reader treats a missing file as an
	// empty result). Empty string skips the export (used when the
	// call-activity declares no outputs).
	OutputFilePath string

	// OutputKeysSpec is the comma-separated allow-list of declared output
	// keys with their types — shape `key1:type1,key2:type2,...`. Exported
	// into the subprocess env as GH_OPTIVEM_OUTPUT_KEYS. The `output
	// write` CLI parses this to reject unknown keys mid-run, giving the
	// agent immediate feedback on a typo'd key. Empty string when the
	// call-activity declares no outputs — `output write` then refuses
	// with "no outputs declared for this agent".
	OutputKeysSpec string

	// ExpectedOutputs is the writing-agent MID's declared output contract
	// rendered for the prompt's ${expected-outputs} placeholder. The
	// driver populates it from Engine.Outputs(taskName) so prompt authors
	// never hand-write an "Outputs" section — the BPMN declaration is the
	// single source of truth. Empty slice (or nil) → ${expected-outputs}
	// substitutes to empty; agents with no declared outputs simply have
	// no Required/Optional contract table in their rendered prompt.
	ExpectedOutputs []statemachine.OutputSpec

	// ShowPrompt, when true, dumps the full rendered prompt to Stdout
	// between the prepared-prompt summary banner and the ENTERING AGENT
	// banner. Off by default; useful for debugging template edits or
	// auditing a new agent's body.
	ShowPrompt bool

	// RepoPath is the working directory the subprocess runs in (and the
	// directory git rev-parse / git log query). Empty → current cwd.
	RepoPath string

	// ProjectConfig, when non-nil, triggers per-project reference-doc
	// materialization ahead of prompt rendering. Dispatch calls
	// assetsync.MaterializeProject against ProjectConfig.PlaceholderMap()
	// so the agent reads docs with concrete project paths instead of
	// `${name}` placeholders, and ${references-root} in the rendered
	// prompt resolves to <RepoPath>/.gh-optivem/references rather than
	// the user-global ~/.gh-optivem/references. Required alongside
	// RepoPath — when either is missing, Dispatch falls back to the
	// user-global references root for backward compat with CLI
	// utilities and scaffold flows that legitimately have no project
	// context.
	ProjectConfig *projectconfig.Config

	// BinaryVersion is the gh-optivem binary version stamped into the
	// per-project materialization sidecar so a tool upgrade triggers
	// a re-materialize. Empty matches any sidecar value (used by tests).
	BinaryVersion string

	// Stdout / Stderr targets for the dispatch banners and (in autonomous
	// mode) the streamed subprocess output. nil → os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Stdin is the operator's TTY in interactive mode. nil → os.Stdin.
	Stdin io.Reader
}

// ClaudeRunner runs the `claude` CLI. The default implementation is
// execClaude. RunOpts is a struct rather than a varargs slice because the
// runner has to choose between interactive and `-p` invocations and stream
// stdout/stderr back to the driver during long autonomous runs.
type ClaudeRunner interface {
	Run(ctx context.Context, opts RunOpts) (RunResult, error)
}

// RunOpts is the cross-cut between Options and the subprocess invocation.
// It hides the autonomous-vs-interactive flag-shape decision behind the
// runner so the production runner can evolve without touching Dispatch.
//
// OutputFilePath / OutputKeysSpec, when non-empty, are exported into the
// subprocess env as GH_OPTIVEM_OUTPUT_FILE / GH_OPTIVEM_OUTPUT_KEYS so the
// agent's `gh optivem output write` CLI calls land in the right file with
// the right allow-list. Mirrors the same fields on Options. Both modes
// (interactive and autonomous) honour the export uniformly — the JSONL
// channel is the cross-mode replacement for the prose-YAML tail that used
// to work only in autonomous mode.
type RunOpts struct {
	Prompt         string
	Autonomous     bool
	Model          string
	Effort         string
	Dir            string
	Stdin          io.Reader
	Stdout         io.Writer
	Stderr         io.Writer
	OutputFilePath string
	OutputKeysSpec string
}

// RunResult is what the runner reports back to Dispatch. Usage is best-effort
// — populated only when the runner can parse a structured envelope (currently
// autonomous mode via `claude -p --output-format json`). Interactive mode
// leaves it nil and the banner falls back to elapsed-time-only.
//
// ResultText is the agent's final response body — the `result` field from
// the `claude -p --output-format json` envelope. Populated only in
// autonomous mode (interactive mode prints directly to the operator's TTY
// and has no envelope to parse). Used by the exit-banner result echo so
// the operator sees the agent's final message inline. Structured outputs
// flow through the per-dispatch JSONL channel
// (GH_OPTIVEM_OUTPUT_FILE) — not through ResultText anymore.
type RunResult struct {
	Usage      *TokenUsage
	ResultText string
}

// TokenUsage is the cost/throughput summary surfaced in the exit banner.
// Field names mirror the `claude -p --output-format json` envelope so the
// JSON shape can be decoded directly into this struct.
type TokenUsage struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	TotalCostUSD             float64 `json:"-"`
}

// GitRunner runs the `git` CLI in a given directory. Mirrors the GhRunner
// shape used elsewhere but adds a working-directory parameter because the
// driver may dispatch against a sub-checkout.
type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Deps lets tests substitute fake runners. Production callers pass a
// zero-value Deps and Dispatch falls back to real `claude` / `git`.
type Deps struct {
	Claude ClaudeRunner
	Git    GitRunner
}

func (d Deps) withDefaults() Deps {
	if d.Claude == nil {
		d.Claude = execClaude{}
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	return d
}

// Dispatch builds the prompt, runs the subprocess, and reports back. The
// agent is told not to commit; staging and committing belongs to the
// wrapping CLI, which fires after Dispatch returns.
//
// Returns the runner's RunResult — populated even on a non-nil error
// when the runner ran at all, so callers that care about partial state
// (e.g. token usage from a failed run, or the result text from a clean
// run for downstream `outputs:` parsing) can read it. On errors that
// fire before the runner is invoked (snapshot, render, materialize) the
// returned RunResult is its zero value.
//
// Errors are returned for:
//   - Subprocess exit status non-zero (stderr surfaced; a small classifier
//     turns known rate-limit / auth signatures into actionable messages
//     before falling through to the generic wrapper).
//   - Subprocess exit zero but the agent switched branches mid-run (the
//     pre/post snapshot diff catches this so the operator gets a clear
//     "switched branches" message before the wrapping CLI tries to commit
//     against the wrong branch).
//   - Any of the surrounding git / template steps failing.
//
// HEAD moving during dispatch is no longer a halt condition: an agent that
// commits anyway is a misbehaviour for the wrapping CLI to flag, not for
// clauderun. Pre/post snapshots also surface stranded untracked files
// (created by the agent but never `git add`ed) as a non-fatal warning
// after the success banner.
func Dispatch(ctx context.Context, deps Deps, opts Options) (RunResult, error) {
	deps = deps.withDefaults()
	opts = opts.withDefaults()

	// When a ProjectConfig is in hand and we have a project root to write
	// into, materialize the embedded reference docs against the project's
	// placeholder map so the agent reads docs with concrete paths. The
	// returned root is forwarded to renderPrompt so ${references-root}
	// resolves to the project-local copy. When either field is missing
	// we leave projectReferencesRoot empty and the user-global references
	// root applies — the fallback path covers CLI utilities, scaffold
	// flows, and tests that legitimately have no project context.
	var projectReferencesRoot string
	if opts.ProjectConfig != nil && opts.RepoPath != "" {
		var err error
		projectReferencesRoot, err = assetsync.MaterializeProject(
			opts.RepoPath, opts.BinaryVersion, opts.ProjectConfig.PlaceholderMap())
		if err != nil {
			return RunResult{}, fmt.Errorf("clauderun: materialize project references: %w", err)
		}
	}

	prompt := opts.RawPrompt
	if prompt == "" {
		var err error
		prompt, err = renderPromptWithReferencesRoot(opts, projectReferencesRoot)
		if err != nil {
			return RunResult{}, fmt.Errorf("clauderun: render prompt: %w", err)
		}
		if leftovers := findUnfilledPlaceholders(prompt); len(leftovers) > 0 {
			return RunResult{}, fmt.Errorf(
				"clauderun: prompt has unfilled placeholders after substitution: %s\n  this usually means the field was not seeded into Context.State before dispatch — check seedScopeState and preResolveIssue",
				strings.Join(leftovers, ", "))
		}
	}

	if opts.PromptLogPath != "" {
		if err := writePromptLog(opts.PromptLogPath, prompt); err != nil {
			fmt.Fprintf(opts.Stderr, "clauderun: warning: failed to write prompt log %s: %v\n", opts.PromptLogPath, err)
		}
	}

	preState, err := snapshotRepo(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		return RunResult{}, fmt.Errorf("clauderun: snapshot before dispatch: %w", err)
	}

	writePreparedPromptBanner(opts, prompt)
	if opts.ShowPrompt {
		fmt.Fprintln(opts.Stdout, prompt)
	}
	writeEnterBanner(opts)
	startedAt := nowFn()

	// Tee stderr so we can classify rate-limit / auth failures after a
	// non-zero exit without losing the operator-visible stream. Bounded
	// to a reasonable cap to avoid pathological memory growth on a chatty
	// runner.
	var stderrCapture cappedBuffer
	stderrCapture.cap = 64 * 1024
	runStderr := io.MultiWriter(opts.Stderr, &stderrCapture)

	runResult, runErr := deps.Claude.Run(ctx, RunOpts{
		Prompt:         prompt,
		Autonomous:     opts.Autonomous,
		Model:          opts.Model,
		Effort:         opts.Effort,
		Dir:            opts.RepoPath,
		Stdin:          opts.Stdin,
		Stdout:         opts.Stdout,
		Stderr:         runStderr,
		OutputFilePath: opts.OutputFilePath,
		OutputKeysSpec: opts.OutputKeysSpec,
	})
	if runErr != nil {
		writeExitBanner(opts, 0, nowFn().Sub(startedAt), runResult.Usage, runErr)
		if classified := classifyRunError(stderrCapture.Bytes()); classified != nil {
			return runResult, fmt.Errorf("clauderun: %s: %w", opts.Agent, classified)
		}
		return runResult, fmt.Errorf("clauderun: %s exited non-zero: %w", opts.Agent, runErr)
	}

	postState, err := snapshotRepo(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		writeExitBanner(opts, 0, nowFn().Sub(startedAt), runResult.Usage, err)
		return runResult, fmt.Errorf("clauderun: snapshot after dispatch: %w", err)
	}

	if postState.branch != preState.branch {
		switchErr := fmt.Errorf("agent switched branches mid-run (was %q, now %q) — original-branch HEAD unchanged at %s",
			preState.branch, postState.branch, shortSHA(preState.head))
		writeExitBanner(opts, 0, nowFn().Sub(startedAt), runResult.Usage, switchErr)
		return runResult, fmt.Errorf("clauderun: %s: %w", opts.Agent, switchErr)
	}

	changed := diffDirty(preState.dirty, postState.dirty)
	if newUntracked := diffUntracked(preState.untracked, postState.untracked); len(newUntracked) > 0 {
		writeUntrackedWarning(opts, newUntracked)
	}

	writeExitBanner(opts, len(changed), nowFn().Sub(startedAt), runResult.Usage, nil)
	return runResult, nil
}

// nowFn is a package-level seam so tests can pin elapsed time in banner
// output. Production points at time.Now.
var nowFn = time.Now

// findUnfilledPlaceholders is the package-local alias for expand.FindUnfilled.
// Kept as an alias (rather than inlined at every call site) so the existing
// test suite that calls it directly continues to compile.
var findUnfilledPlaceholders = expand.FindUnfilled

// writePromptLog writes the rendered prompt to path, creating parent
// directories as needed. Used by Dispatch when Options.PromptLogPath is
// set so the operator has a persistent record of what the agent was
// asked to do — independent of materializePrompt's tempfile, which is
// deleted on dispatch exit.
func writePromptLog(path, prompt string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(prompt), 0o644)
}

// renderPrompt reads the embedded prompt for opts.Agent (or opts.PromptOverride
// when non-empty), expands ${name} placeholders against the ticket context,
// and appends opts.OverrideText (if any) as a trailing block. Public-ish for
// the test file; not exported. ${references-root} resolves to the user-global
// ~/.gh-optivem/references — callers wanting the project-local materialized
// copy route through Dispatch (which calls renderPromptWithReferencesRoot
// with a non-empty root).
func renderPrompt(opts Options) (string, error) {
	return renderPromptWithReferencesRoot(opts, "")
}

// renderPromptWithReferencesRoot is the worker behind renderPrompt.
// projectReferencesRoot, when non-empty, wins for the ${references-root}
// substitution — Dispatch passes the result of MaterializeProject so the
// agent reads the project-local references copy. Empty falls back to
// assetsync.ReferencesRoot() (the user-global path), which is what tests
// and CLI utilities use.
func renderPromptWithReferencesRoot(opts Options, projectReferencesRoot string) (string, error) {
	var body string
	if opts.PromptOverride != "" {
		body = opts.PromptOverride
	} else {
		var err error
		body, err = agents.Prompt(opts.Agent)
		if err != nil {
			return "", err
		}
	}
	params := map[string]string{}
	// Project-wide placeholders are seeded first so NodeParams and the
	// fixed-schema set can both override them on key collision. Empty
	// values are skipped — `findUnfilledPlaceholders` then catches a
	// missing-but-referenced key rather than silently substituting "".
	for k, v := range opts.Placeholders {
		if v != "" {
			params[k] = v
		}
	}
	for k, v := range opts.NodeParams {
		params[k] = v
	}
	// Fixed-schema placeholders win on key collision with NodeParams so a
	// node author can't accidentally shadow ticket context by reusing a
	// reserved name in YAML.
	referencesRoot := projectReferencesRoot
	if referencesRoot == "" {
		var err error
		referencesRoot, err = assetsync.ReferencesRoot()
		if err != nil {
			return "", fmt.Errorf("clauderun: resolve references root: %w", err)
		}
	}
	for k, v := range map[string]string{
		"issue-num":       strconv.Itoa(opts.IssueNum),
		"issue-title":     opts.IssueTitle,
		"phase":           opts.NodeDescription,
		"architecture":    opts.Architecture,
		"subtype":         opts.Subtype,
		"verify-results":  opts.VerifyResults,
		"changed-files":   opts.ChangedFiles,
		"references-root": referencesRoot,
	} {
		params[k] = v
	}
	// Per-phase scope block (plan 20260526-1448 Item 4). Rendered from the
	// BPMN node's read: / write: lists joined against the same path map
	// the inline ${key} annotations resolve through (opts.Placeholders —
	// produced by cfg.PlaceholderMap() in production). Only registered
	// when both lists are non-empty — `scope: none` phases and command-
	// only MIDs leave ${scope-block} unfilled, which
	// findUnfilledPlaceholders converts to a hard error if the prompt
	// body references the placeholder. Mirrors the Language /
	// AcceptanceCriteria load-bearing pattern.
	if len(opts.ScopeRead) > 0 && len(opts.ScopeWrite) > 0 {
		params["scope-block"] = renderScopeBlock(opts.ScopeRead, opts.ScopeWrite, opts.Placeholders)
	}
	// Language is load-bearing — only registered when the driver supplied
	// a value. An empty value would silently substitute, masking a config
	// bug; instead we leave `${language}` unfilled so findUnfilledPlaceholders
	// reports it after substitution.
	if opts.Language != "" {
		params["language"] = opts.Language
	}
	// TicketID is load-bearing for test-disabler / test-enabler (compose
	// the disable-reason string). Same registration shape as Language —
	// only registered when non-empty so an absent value surfaces via
	// findUnfilledPlaceholders rather than silently substituting "" into
	// a downstream startsWith filter.
	if opts.TicketID != "" {
		params["ticket-id"] = opts.TicketID
	}
	// Disable-marker examples (test-disabler / test-enabler). Composed
	// from Language + TicketID + the call-activity-pushed loop /
	// cycle_phase / prev_phase NodeParams. Same load-bearing pattern as
	// Language: register only when the helper produces a non-empty
	// string (all inputs present AND language is recognised); otherwise
	// findUnfilledPlaceholders fails the dispatch fast when the prompt
	// body references either placeholder. Inlining the marker shape
	// here replaces the previous "go read the language-equivalents row"
	// instruction in the agent body — the row is out of scope and the
	// agent was forced to grep in-scope tests to reverse-engineer the
	// syntax (3-5 wasted tool calls per dispatch).
	if ex := renderDisableMarkerExample(opts.Language, opts.TicketID, opts.NodeParams["loop"], opts.NodeParams["cycle_phase"]); ex != "" {
		params["disable-marker-example"] = ex
	}
	if ex := renderDisableMarkerRemovalExample(opts.Language, opts.TicketID, opts.NodeParams["prev_phase"]); ex != "" {
		params["disable-marker-removal-example"] = ex
	}
	// AcceptanceCriteria is load-bearing for write-acceptance-tests — same rationale
	// as Language. Only registered when non-empty so an absent value
	// surfaces via findUnfilledPlaceholders rather than substituting "".
	if opts.AcceptanceCriteria != "" {
		params["acceptance-criteria"] = opts.AcceptanceCriteria
	}
	// Checklist is load-bearing for the four task subtypes whose cycles
	// consume ${checklist} (system-redesign, external-system-redesign,
	// system-refactor, test-refactor). Same rationale as
	// AcceptanceCriteria — only registered when non-empty so an absent
	// value surfaces via findUnfilledPlaceholders rather than substituting
	// "". Catches the bug class where parse-ticket failed to populate or
	// the ticket-kind expected a Checklist but the body declared AC.
	if opts.Checklist != "" {
		params["checklist"] = opts.Checklist
	}
	// ParsedConcepts is load-bearing for refine-acceptance-criteria —
	// same rationale as AcceptanceCriteria. Only registered when
	// non-empty so an absent value surfaces via findUnfilledPlaceholders.
	if opts.ParsedConcepts != "" {
		params["parsed-concepts"] = opts.ParsedConcepts
	}
	// Command-failure payload — load-bearing for fix-command-failed.
	// CommandLine gates the trio: when present, all three placeholders
	// are registered together; when absent, the prompt body's
	// ${command} reference surfaces via findUnfilledPlaceholders.
	if opts.CommandLine != "" {
		params["command"] = opts.CommandLine
		params["command-exit-code"] = strconv.Itoa(opts.CommandExitCode)
		params["command-stderr-tail"] = opts.CommandStderrTail
	}
	// Validation-failure payload — load-bearing for fix-missing-output
	// and fix-scope-diff. FailingTaskName is shared by both prompts and
	// is registered whenever non-empty; MissingOutputs and
	// ViolatingPaths are mutually exclusive (missing-output wins over
	// scope-diff in validateOutputsAndScopes), so each is registered on
	// its own. Kebab-cased placeholder names mirror the kebab state keys
	// the action stashes (failing-task-name, missing-outputs,
	// scope-violating-paths).
	if opts.FailingTaskName != "" {
		params["failing-task-name"] = opts.FailingTaskName
	}
	if opts.MissingOutputs != "" {
		params["missing-outputs"] = opts.MissingOutputs
	}
	if opts.ViolatingPaths != "" {
		params["violating-paths"] = opts.ViolatingPaths
	}
	// Expected outputs (plan 20260526-2118). Always set — empty when the
	// MID declared none, so ${expected-outputs} in the prompt body
	// substitutes to an empty string rather than tripping
	// findUnfilledPlaceholders. The renderer formats per-spec entries as
	// `key: type`, grouped into Required / Optional blocks, then a single
	// "Emit:" line pointing at the CLI. Drift kill: prompt authors never
	// write this section by hand.
	params["expected-outputs"] = renderExpectedOutputs(opts.ExpectedOutputs)
	rendered, err := statemachine.ExpandParams(body, params, nil)
	if err != nil {
		return "", err
	}
	if opts.OverrideText != "" {
		rendered = strings.TrimRight(rendered, "\n") + "\n\n" + opts.OverrideText + "\n"
	}
	return rendered, nil
}

// RenderPrompt is the public counterpart to renderPrompt: it returns the
// prompt string Dispatch would hand to the subprocess, without invoking
// it. Kept as an exported test seam so the prompt-shape regression tests
// (see clauderun_test.go's TestRenderPrompt_* suite) can assert on the
// rendered text directly.
//
// If opts.RawPrompt is non-empty, it is returned verbatim — RenderPrompt
// mirrors Dispatch's "RawPrompt wins" rule.
func RenderPrompt(opts Options) (string, error) {
	if opts.RawPrompt != "" {
		return opts.RawPrompt, nil
	}
	return renderPrompt(opts)
}

// renderExpectedOutputs produces the body of the ${expected-outputs}
// substitution — a minimal contract table the writing-agent prompts
// embed so the agent knows which keys to emit via `gh optivem output
// write`. Per plan 20260526-2118, this is derived from the BPMN
// declaration (Engine.Outputs(taskName)) so prompt authors never write
// the table by hand — kills the prompt/BPMN drift permanently.
//
// Layout:
//
//	Required outputs:
//	  key-A: type
//
//	Optional outputs:
//	  key-B: type
//
//	Emit: gh optivem output write KEY=VAL [KEY=VAL...]
//
// When every output is required, the "Optional outputs:" block is
// omitted (and vice versa). When the list is empty, the substitution
// renders as empty — agents with no declared outputs simply have no
// table in their prompt.
func renderExpectedOutputs(specs []statemachine.OutputSpec) string {
	if len(specs) == 0 {
		return ""
	}
	var required, optional []statemachine.OutputSpec
	for _, s := range specs {
		if s.Optional {
			optional = append(optional, s)
		} else {
			required = append(required, s)
		}
	}
	var b strings.Builder
	if len(required) > 0 {
		b.WriteString("Required outputs:\n")
		for _, s := range required {
			fmt.Fprintf(&b, "  %s: %s\n", s.Key, s.Type)
		}
	}
	if len(optional) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Optional outputs:\n")
		for _, s := range optional {
			fmt.Fprintf(&b, "  %s: %s\n", s.Key, s.Type)
		}
	}
	b.WriteString("\nEmit: gh optivem output write KEY=VAL [KEY=VAL...]")
	return b.String()
}

// renderScopeBlock produces the body of the ${scope-block} substitution:
// two key-list blocks (read, then write) plus the scope_exception escape
// hatch line. The `### Scope` heading itself is owned by the prompt
// source (each in-scope prompt writes `### Scope\n\n${scope-block}` under
// `## Inputs`), so this helper renders only the body.
//
// Layer keys are resolved against the supplied paths map — same source
// the inline ${key} placeholders resolve through (opts.Placeholders, fed
// by cfg.PlaceholderMap()). Family B path keys (driver-port, dsl-core,
// …) plus the Family A `system-path` are all in scope. Unresolved keys
// (nil paths, or key not in the map) emit as `<key>: (unresolved)`
// rather than disappearing silently — the build-time canonical-keys
// guard catches drift before runtime, but if it slips through, the
// agent sees the gap.
func renderScopeBlock(read, write []string, paths map[string]string) string {
	resolve := func(key string) string {
		if paths == nil {
			return "(unresolved)"
		}
		if v, ok := paths[key]; ok && v != "" {
			return v
		}
		return "(unresolved)"
	}
	maxKeyLen := func(keys []string) int {
		n := 0
		for _, k := range keys {
			if len(k) > n {
				n = len(k)
			}
		}
		return n
	}
	writeBlock := func(b *strings.Builder, keys []string) {
		pad := maxKeyLen(keys)
		for _, k := range keys {
			fmt.Fprintf(b, "- `%s`:%s %s\n", k, strings.Repeat(" ", pad-len(k)), resolve(k))
		}
	}

	var b strings.Builder
	b.WriteString("You may **read** files under these paths:\n\n")
	writeBlock(&b, read)
	b.WriteString("\nYou may **modify** files under these paths:\n\n")
	writeBlock(&b, write)
	b.WriteString("\nReading or writing outside this set requires a `scope_exception` block.")
	return b.String()
}

// renderDisableMarkerExample inlines the per-language disable-marker
// emit-this snippet into the test-disabler prompt via
// ${disable-marker-example}. Previously the agent had to look up the
// syntax in the language-equivalents reference doc (which is out of
// scope under the agent's `read:` set) or grep the in-scope test tree
// to reverse-engineer it — 3-5 wasted tool calls per dispatch. With
// the snippet inlined the agent reads the target test file once and
// edits it.
//
// The reason string is composed inline (not via a nested placeholder)
// so the agent sees the literal marker it must emit. Returns "" when
// any required field is empty OR the language is unrecognised; the
// caller registers the placeholder only when non-empty so an absent
// value surfaces via findUnfilledPlaceholders rather than silently
// substituting "". Adding a new language requires touching this
// function AND the matching row in language-equivalents/<lang>.md.
func renderDisableMarkerExample(lang, ticketID, loop, cyclePhase string) string {
	if lang == "" || ticketID == "" || loop == "" || cyclePhase == "" {
		return ""
	}
	reason := fmt.Sprintf("%s - AT - %s - %s", ticketID, loop, cyclePhase)
	switch lang {
	case "java":
		return fmt.Sprintf("```java\n@Disabled(\"%s\")\n@Test\nvoid shouldXxx() { ... }\n```\n\nAdd `import org.junit.jupiter.api.Disabled;` next to the other JUnit imports if it's not already present.", reason)
	case "csharp":
		return fmt.Sprintf("```csharp\n[Fact(Skip = \"%s\")]\npublic void ShouldXxx() { ... }\n```\n\nKeep the `[Fact]` attribute; add only the `Skip = \"...\"` parameter. No import change.", reason)
	case "typescript":
		return fmt.Sprintf("```typescript\n// %s\ntest.skip('shouldXxx', async (...) => { ... });\n```\n\nPrepend the `// <reason>` comment line above the test, then change `test(` to `test.skip(`. Playwright's `test.skip(title, body)` overload defines a skipped test; the comment carries the reason because the definition-time skip overload has no reason parameter. No import change.", reason)
	}
	return ""
}

// renderDisableMarkerRemovalExample is the symmetric helper for the
// test-enabler — inlines the per-language transform that strips a
// disable marker via ${disable-marker-removal-example}. Same
// rationale as renderDisableMarkerExample. The reason-prefix is
// composed inline so the agent sees the exact startsWith filter it
// must match before stripping.
func renderDisableMarkerRemovalExample(lang, ticketID, prevPhase string) string {
	if lang == "" || ticketID == "" || prevPhase == "" {
		return ""
	}
	prefix := fmt.Sprintf("%s - AT - RED - %s", ticketID, prevPhase)
	switch lang {
	case "java":
		return fmt.Sprintf("Delete the `@Disabled(\"%s ...\")` annotation line above the test. If no `@Disabled` annotations remain in the file after stripping, also delete `import org.junit.jupiter.api.Disabled;`.", prefix)
	case "csharp":
		return fmt.Sprintf("Rewrite `[Fact(Skip = \"%s ...\")]` back to `[Fact]` — keep the attribute, drop only the `Skip` parameter. No import change.", prefix)
	case "typescript":
		return fmt.Sprintf("Delete the `// %s ...` comment line above the test, then change `test.skip(` back to `test(`. No import change.", prefix)
	}
	return ""
}

func (o Options) withDefaults() Options {
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

// ---------------------------------------------------------------------------
// Repo snapshot (pre/post) and HEAD detection
// ---------------------------------------------------------------------------

// repoState is the "before"/"after" snapshot Dispatch takes around the
// runner call. The diff catches two failure modes:
//   - Branch-switch: the agent runs `git checkout -b feature/foo`,
//     commits there, and never returns. HEAD on the original branch is
//     unchanged → without snapshot we'd halt with the misleading "no
//     commit produced"; with snapshot we say "switched branches".
//   - Stranded untracked files: the agent created files but never
//     `git add`ed them. The commit lands fine but the new files sit
//     outside it (silent data-loss class). Snapshot diff surfaces them
//     as a non-fatal warning.
type repoState struct {
	head      string
	branch    string
	untracked map[string]bool
	// dirty is the union of every path mentioned in `git status
	// --porcelain` (untracked, modified, deleted, staged, …). Used by
	// the CLICommits staging policy: the post-pre delta is exactly
	// "what the agent touched that wasn't already dirty," which is
	// what we stage and commit.
	dirty map[string]bool
}

func snapshotRepo(ctx context.Context, git GitRunner, dir string) (repoState, error) {
	headOut, err := git.Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return repoState{}, fmt.Errorf("rev-parse HEAD: %w", err)
	}
	branchOut, err := git.Run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return repoState{}, fmt.Errorf("rev-parse --abbrev-ref HEAD: %w", err)
	}
	statusOut, err := git.Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return repoState{}, fmt.Errorf("status --porcelain: %w", err)
	}
	return repoState{
		head:      strings.TrimSpace(string(headOut)),
		branch:    strings.TrimSpace(string(branchOut)),
		untracked: parseUntracked(statusOut),
		dirty:     parseDirty(statusOut),
	}, nil
}

// parseDirty returns every path mentioned in `git status --porcelain`
// output, regardless of status code. Renames (R old -> new) collapse
// to the new path — that is what we want to stage. Lines too short to
// hold "XY path" are skipped silently (defensive against trailing
// blank lines).
func parseDirty(porcelain []byte) map[string]bool {
	m := map[string]bool{}
	for line := range strings.SplitSeq(string(porcelain), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+len(" -> "):]
		}
		if path != "" {
			m[path] = true
		}
	}
	return m
}

// diffDirty returns paths present in post but not pre, sorted for
// stable test output and a deterministic `git add` argv.
func diffDirty(pre, post map[string]bool) []string {
	var out []string
	for p := range post {
		if !pre[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// parseUntracked picks out the `??<space><path>` rows from `git status
// --porcelain` output. Other status codes (modified, staged, etc.) are
// ignored — only untracked files are the silent-data-loss class we
// care about for the post-dispatch warning.
func parseUntracked(porcelain []byte) map[string]bool {
	m := map[string]bool{}
	for line := range strings.SplitSeq(string(porcelain), "\n") {
		if len(line) >= 4 && line[0] == '?' && line[1] == '?' {
			path := strings.TrimSpace(line[3:])
			if path != "" {
				m[path] = true
			}
		}
	}
	return m
}

// diffUntracked returns paths present in post but not pre, sorted for
// stable banner output.
func diffUntracked(pre, post map[string]bool) []string {
	var out []string
	for p := range post {
		if !pre[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// ---------------------------------------------------------------------------
// Stderr classification (rate limit / auth)
// ---------------------------------------------------------------------------

// rateLimitSignatures are case-insensitive substrings that mean "claude
// refused to dispatch because of a billing / rate limit". First match
// wins. Patterns are deliberately broad — false positives here only
// change the wording of an already-failing run.
var rateLimitSignatures = []string{
	"rate limit",
	"rate_limit_error",
	"weekly limit",
	"5-hour limit",
	"usage limit",
	"quota exceeded",
	"too many requests",
}

// authSignatures are case-insensitive substrings that mean "claude
// refused because credentials are missing or invalid". The pre-flight
// check at driver startup catches this before the first dispatch in
// the happy path; this branch covers credentials expiring mid-run.
var authSignatures = []string{
	"not authenticated",
	"auth required",
	"authentication required",
	"invalid api key",
	"please run /login",
	"please log in",
	"please login",
}

// classifyRunError inspects the captured stderr from a non-zero claude
// exit and returns a more actionable error when a known signature
// matches. Returns nil meaning "fall through to the generic wrapper".
func classifyRunError(stderr []byte) error {
	tail := lastLines(stderr, 20)
	lower := strings.ToLower(string(tail))
	for _, sig := range rateLimitSignatures {
		if strings.Contains(lower, sig) {
			return errors.New("rate limit hit on Claude subscription; weekly cap likely exhausted — re-run after the next reset window or upgrade your plan")
		}
	}
	for _, sig := range authSignatures {
		if strings.Contains(lower, sig) {
			return errors.New("claude CLI is not authenticated — run `claude /login` (credentials live in ~/.claude/) before re-dispatching")
		}
	}
	return nil
}

// lastLines returns the trailing n lines of b. Used to bound the
// classifier's scan to the most recent error output — if the runner
// printed a wall of progress text before failing, we want to look at
// the failure tail, not the noise above it.
func lastLines(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return b
	}
	count := 0
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			count++
			if count > n {
				return b[i+1:]
			}
		}
	}
	return b
}

// cappedBuffer is a write-only buffer that drops bytes past `cap`.
// Used to capture stderr for classification without unbounded memory
// growth on a runner that streams a lot of output before failing.
type cappedBuffer struct {
	buf bytes.Buffer
	cap int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.cap > 0 && c.buf.Len() >= c.cap {
		return len(p), nil
	}
	if c.cap > 0 && c.buf.Len()+len(p) > c.cap {
		p = p[:c.cap-c.buf.Len()]
	}
	return c.buf.Write(p)
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }

// ---------------------------------------------------------------------------
// Banners
// ---------------------------------------------------------------------------

const banner = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

// writePreparedPromptBanner prints a structured summary of the prompt
// the runner is about to receive. Always emitted (the noise cost is
// trivial vs. the bug class it catches: empty substitution fields like
// "architecture: " or "allowed roots: (empty)" become visible at a
// glance instead of bleeding into a multi-tree edit blowout).
//
// In RawPrompt mode every introspection field is meaningless (the
// operator deliberately swapped the templated body), so the banner
// degrades to a one-line `override mode — N bytes` notice.
func writePreparedPromptBanner(opts Options, prompt string) {
	cyan := color.New(color.FgCyan)
	w := opts.Stdout
	fmt.Fprintln(w, cyan.Sprint(banner))
	if opts.RawPrompt != "" {
		fmt.Fprintln(w, cyan.Sprintf("📋 PREPARED PROMPT for %s  (override mode — %s)",
			opts.Agent, formatPromptSize(len(prompt))))
		fmt.Fprintln(w, cyan.Sprint(banner))
		return
	}
	fmt.Fprintln(w, cyan.Sprintf("📋 PREPARED PROMPT for %s", opts.Agent))
	fmt.Fprintln(w, cyan.Sprintf("   size:           %s", formatPromptSize(len(prompt))))
	fmt.Fprintln(w, cyan.Sprintf("   architecture:   %s", orPlaceholderClauderun(opts.Architecture, "(empty)")))
	fmt.Fprintln(w, cyan.Sprintf("   scope:          %s", summarizeScope(opts.ScopeRead, opts.ScopeWrite)))
	fmt.Fprintln(w, cyan.Sprintf("   acceptance criteria: %s", summarizeAcceptanceCriteria(opts.AcceptanceCriteria)))
	writeIndentedBlock(w, cyan, opts.AcceptanceCriteria)
	fmt.Fprintln(w, cyan.Sprintf("   checklist:      %s", summarizeChecklist(opts.Checklist)))
	writeIndentedBlock(w, cyan, opts.Checklist)
	fmt.Fprintln(w, cyan.Sprintf("   override text:  %s", orPlaceholderClauderun(opts.OverrideText, "(none)")))
	fmt.Fprintln(w, cyan.Sprintf("   log:            %s", orPlaceholderClauderun(opts.PromptLogPath, "(none)")))
	fmt.Fprintln(w, cyan.Sprint(banner))
}

// writeIndentedBlock prints each non-empty line of s under the
// preceding summary line, indented to align beneath the field value.
// Skips blank lines so an embedded block with leading blanks doesn't
// leave a gap in the banner.
func writeIndentedBlock(w io.Writer, c *color.Color, s string) {
	for line := range strings.SplitSeq(s, "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		if trimmed == "" {
			continue
		}
		fmt.Fprintln(w, c.Sprintf("     %s", trimmed))
	}
}

func orPlaceholderClauderun(s, placeholder string) string {
	if s == "" {
		return placeholder
	}
	return s
}

func formatPromptSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

// summarizeScope reduces the per-phase read / write key lists to a
// one-line count for the prepared-prompt banner. Empty both → "(empty)"
// (which is the load-bearing path — see ScopeRead/ScopeWrite doc).
func summarizeScope(read, write []string) string {
	if len(read) == 0 && len(write) == 0 {
		return "(empty)"
	}
	return fmt.Sprintf("%d read / %d write", len(read), len(write))
}

// summarizeAcceptanceCriteria reduces the multi-line AC block to a one-
// line size hint for the banner. Counts non-blank lines; returns
// "(empty)" when no AC was set (the load-bearing placeholder catches
// dispatch-time misses).
func summarizeAcceptanceCriteria(s string) string {
	if s == "" {
		return "(empty)"
	}
	lines := 0
	for line := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines == 0 {
		return "(empty)"
	}
	return fmt.Sprintf("%d line(s)", lines)
}

// summarizeChecklist counts checklist items in the ${checklist} block.
// Recognises `- [ ]` and `- [x]` / `- [X]` rows; lines that are not
// markdown task rows are ignored. Empty input → "(empty)".
func summarizeChecklist(s string) string {
	if s == "" {
		return "(empty)"
	}
	total, checked := 0, 0
	for line := range strings.SplitSeq(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [") || len(trimmed) < 6 {
			continue
		}
		total++
		if trimmed[3] == 'x' || trimmed[3] == 'X' {
			checked++
		}
	}
	if total == 0 {
		return "(empty)"
	}
	return fmt.Sprintf("%d item(s) (%d already [x])", total, checked)
}

func writeEnterBanner(opts Options) {
	cyan := color.New(color.FgCyan, color.Bold)
	w := opts.Stdout
	fmt.Fprintln(w, cyan.Sprint(banner))
	mode := "interactive"
	if opts.Autonomous {
		mode = "autonomous"
	}
	fmt.Fprintln(w, cyan.Sprintf("🤖 ENTERING AGENT: %s  (%s)", opts.Agent, mode))
	if opts.IssueNum > 0 || opts.IssueTitle != "" {
		fmt.Fprintln(w, cyan.Sprintf("   Issue: #%d %q",
			opts.IssueNum, opts.IssueTitle))
	}
	fmt.Fprintln(w, cyan.Sprint(banner))
}

// writeExitBanner reports the agent's exit. changedFiles is the count of
// paths the agent touched (post-pre delta of `git status --porcelain`); the
// outer CLI is what stages and commits them, so the banner only signals
// the size of the work it has to act on. Zero changes is a legitimate
// no-op — still a successful exit, surfaced so the operator can tell at a
// glance there's nothing for the wrapper to commit.
func writeExitBanner(opts Options, changedFiles int, elapsed time.Duration, usage *TokenUsage, runErr error) {
	w := opts.Stdout
	if runErr != nil {
		red := color.New(color.FgRed, color.Bold)
		fmt.Fprintln(w, red.Sprint(banner))
		fmt.Fprintln(w, red.Sprintf("❌ AGENT FAILED: %s  (%s%s)", opts.Agent, elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
		fmt.Fprintln(w, red.Sprintf("   %s", runErr))
		fmt.Fprintln(w, red.Sprint(banner))
		return
	}
	green := color.New(color.FgGreen, color.Bold)
	fmt.Fprintln(w, green.Sprint(banner))
	if changedFiles == 0 {
		fmt.Fprintln(w, green.Sprintf("✅ EXITED AGENT: no changes  (%s%s)",
			elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
		fmt.Fprintln(w, green.Sprint(banner))
		return
	}
	fmt.Fprintln(w, green.Sprintf("✅ EXITED AGENT: %d file(s) changed  (%s%s)",
		changedFiles, elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
	fmt.Fprintln(w, green.Sprint(banner))
}

// formatUsageSuffix renders ", 12.4k in / 1.8k out, $0.18" if usage is non-nil
// and non-empty. Returns "" otherwise so the banner gracefully degrades to
// elapsed-time-only when the runner couldn't extract a JSON envelope (e.g.
// interactive mode, or an autonomous-mode parse failure).
func formatUsageSuffix(usage *TokenUsage) string {
	if usage == nil {
		return ""
	}
	in := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	out := usage.OutputTokens
	if in == 0 && out == 0 && usage.TotalCostUSD == 0 {
		return ""
	}
	return fmt.Sprintf(", %s in / %s out, $%.2f", formatTokens(in), formatTokens(out), usage.TotalCostUSD)
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

// writeUntrackedWarning surfaces files the agent created but never
// `git add`ed. Emitted after writeExitBanner on the success path so
// the operator sees the commit SHA first, then the warning. Non-fatal:
// the operator may have intended to leave them (e.g. ad-hoc scratch
// files) — but the typical case is they meant to commit them and the
// stranded files are silent data loss.
func writeUntrackedWarning(opts Options, paths []string) {
	yellow := color.New(color.FgYellow, color.Bold)
	w := opts.Stdout
	fmt.Fprintln(w, yellow.Sprintf("⚠  %s left %d untracked file(s) outside the commit:", opts.Agent, len(paths)))
	for _, p := range paths {
		fmt.Fprintln(w, yellow.Sprintf("    %s", p))
	}
}

const elapsedRound = time.Second

// ---------------------------------------------------------------------------
// Real subprocess implementations
// ---------------------------------------------------------------------------

// promptArgvLimit is the threshold above which materializePrompt spills
// the prompt to a tempfile instead of passing it as an argv argument.
// Windows' CreateProcess caps the full command line at ~32K chars; macOS
// and Linux ARG_MAX are higher (~256K and ~131K) but the same overflow
// is reachable as prompts grow. 8K leaves comfortable headroom under the
// strictest OS limit, including the executable path and any quoting the
// shell adds.
const promptArgvLimit = 8000

// materializePrompt returns the argv argument to hand to `claude` and a
// cleanup func. For single-line prompts under promptArgvLimit it returns
// the prompt verbatim with a no-op cleanup — the historical fast path.
// Otherwise it writes the prompt to a tempfile in dir and returns a short
// bootstrap message instructing the agent to read the file (the only
// viable path on Windows, where the OS argv limit is too low for large
// prompts and the `claude` CLI exposes no --prompt-file flag).
//
// Multi-line prompts are forced through the tempfile path regardless of
// size: on Windows `claude` is a `.cmd` shim, and Go's exec runs `.cmd`
// files via cmd.exe, which truncates argv at the first newline. That bug
// surfaced after the May 2026 ATDD prompt-optimization commits shrank the
// rendered WRITE prompt below promptArgvLimit — the agent then received
// only the first line of a multi-line prompt and reported "no task".
//
// The cleanup func is always safe to call and removes the tempfile if one
// was created. The dispatcher fires it via `defer`, so the agent does not
// need to know the tempfile exists past its first Read.
func materializePrompt(dir, prompt string) (string, func(), error) {
	if len(prompt) <= promptArgvLimit && !strings.ContainsRune(prompt, '\n') {
		return prompt, func() {}, nil
	}
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", func() {}, fmt.Errorf("materializePrompt: getwd: %w", err)
		}
	}
	f, err := os.CreateTemp(dir, ".atdd-prompt-*.tmp.md")
	if err != nil {
		return "", func() {}, fmt.Errorf("materializePrompt: create tempfile: %w", err)
	}
	if _, err := f.WriteString(prompt); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", func() {}, fmt.Errorf("materializePrompt: write tempfile: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", func() {}, fmt.Errorf("materializePrompt: close tempfile: %w", err)
	}
	base := filepath.Base(f.Name())
	bootstrap := fmt.Sprintf("Read `%s` and follow its instructions.", base)
	cleanup := func() { os.Remove(f.Name()) }
	return bootstrap, cleanup, nil
}

type execClaude struct{}

// Run invokes the `claude` CLI.
//
// Interactive mode → `claude <prompt>` with stdin/stdout/stderr connected
// directly so the operator sees the full Claude Code UI and can interject.
//
// Autonomous mode → `claude -p <prompt> --output-format json`:
//
//   - The prompt is the embedded agent's full instructions (rendered by
//     renderPrompt). v2 has no host/subagent split — `claude -p` IS the
//     agent and needs the default tool set (Read/Glob/Grep/Edit/Write/Bash)
//     to do real work.
//   - --output-format json buffers the run into a single JSON envelope
//     containing `total_cost_usd` and `usage.{input,output,cache_*}_tokens`
//     so we can surface cost/throughput in the exit banner. The trade-off
//     is no streaming output during the run.
//
// JSON parsing is best-effort — a future CLI version that changes the
// envelope shape leaves Usage nil and the banner falls back gracefully.
func (execClaude) Run(ctx context.Context, opts RunOpts) (RunResult, error) {
	if opts.Autonomous {
		return runAutonomous(ctx, opts)
	}
	return runInteractive(ctx, opts)
}

func runInteractive(ctx context.Context, opts RunOpts) (RunResult, error) {
	// Interactive: pass the prompt as a positional argument so it seeds
	// the first user turn. Subsequent turns come from the TTY.
	arg, cleanup, err := materializePrompt(opts.Dir, opts.Prompt)
	if err != nil {
		return RunResult{}, err
	}
	defer cleanup()
	args := claudeTuningArgs(opts)
	args = append(args, arg)
	cmd := exec.CommandContext(ctx, "claude", args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	cmd.Env = outputChannelEnv(opts)
	return RunResult{}, cmd.Run()
}

// outputChannelEnv composes the subprocess environment, prepending
// GH_OPTIVEM_OUTPUT_FILE / GH_OPTIVEM_OUTPUT_KEYS to the inherited parent
// env when set on opts. Returns nil when neither is set — leaving cmd.Env
// nil makes os/exec inherit the parent env unmodified (the pre-refactor
// behaviour). Both vars set together: this is the agent dispatch path.
// Neither set: utility runs of the runner (tests, scaffolding) that don't
// declare outputs.
//
// The vars are appended after os.Environ() so they overwrite any same-named
// inherited values — defensive against an operator who set them by hand.
func outputChannelEnv(opts RunOpts) []string {
	if opts.OutputFilePath == "" && opts.OutputKeysSpec == "" {
		return nil
	}
	env := os.Environ()
	if opts.OutputFilePath != "" {
		env = append(env, "GH_OPTIVEM_OUTPUT_FILE="+opts.OutputFilePath)
	}
	if opts.OutputKeysSpec != "" {
		env = append(env, "GH_OPTIVEM_OUTPUT_KEYS="+opts.OutputKeysSpec)
	}
	return env
}

// claudeTuningArgs returns the leading per-dispatch tuning flags
// (`--model`, `--effort`) for the claude CLI. Empty fields are skipped
// so the CLI inherits the user's session defaults — matching the
// behaviour before per-agent tuning was added.
func claudeTuningArgs(opts RunOpts) []string {
	var args []string
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	return args
}

func runAutonomous(ctx context.Context, opts RunOpts) (RunResult, error) {
	arg, cleanup, err := materializePrompt(opts.Dir, opts.Prompt)
	if err != nil {
		return RunResult{}, err
	}
	defer cleanup()
	args := claudeTuningArgs(opts)
	args = append(args,
		"-p", arg,
		"--output-format", "json",
	)
	cmd := exec.CommandContext(ctx, "claude", args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin

	// Capture stdout for JSON parsing. The buffered envelope is dumped to
	// opts.Stdout after the run so the operator still gets the host's
	// final result text, just not streaming.
	var captured bytes.Buffer
	cmd.Stdout = &captured
	cmd.Stderr = opts.Stderr
	cmd.Env = outputChannelEnv(opts)

	runErr := cmd.Run()

	usage, resultText := parseClaudeJSON(captured.Bytes())
	if resultText != "" {
		fmt.Fprintln(opts.Stdout, resultText)
	} else if runErr != nil && captured.Len() > 0 {
		// Run failed before the JSON envelope landed — surface the raw
		// bytes so the operator sees whatever claude did print.
		opts.Stdout.Write(captured.Bytes())
	}
	return RunResult{Usage: usage, ResultText: resultText}, runErr
}

// parseClaudeJSON decodes the `claude -p --output-format json` envelope.
// Returns (nil, "") when the bytes don't decode — callers treat that as
// "no usage data, fall back to elapsed-time-only banner".
func parseClaudeJSON(b []byte) (*TokenUsage, string) {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, ""
	}
	var env struct {
		Result       string     `json:"result"`
		TotalCostUSD float64    `json:"total_cost_usd"`
		Usage        TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, ""
	}
	usage := env.Usage
	usage.TotalCostUSD = env.TotalCostUSD
	return &usage, env.Result
}

type execGit struct{}

func (execGit) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("git %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
