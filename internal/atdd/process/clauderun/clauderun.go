// Package clauderun shells out to the `claude` CLI to dispatch a named ATDD
// agent for the current phase, replacing v1's "pause and let the operator
// launch the agent in a second window" workflow.
//
// Dispatch reads the embedded per-agent prompt (see internal/atdd/runtime/
// agents/embed.go), substitutes ${name} placeholders against the live ticket
// context, invokes `claude` (interactive or `claude -p` headless), and
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
	"bufio"
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
	"github.com/optivem/gh-optivem/internal/atdd/assets"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/expand"
	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/userstate"
)

// Options bundles every input Dispatch needs to construct a prompt and run
// the subprocess. Zero values yield a usable configuration where it makes
// sense (Stdout/Stderr/Stdin default to the OS streams). Required fields
// (Agent, IssueNum, IssueTitle, NodeDescription) are
// not zero-defaulted because missing them yields a meaningless prompt.
type Options struct {
	// Agent is the subagent name to launch (e.g. "write-acceptance-tests").
	Agent string

	// AgentSet binds the prompt directory the agent body, tuning and the
	// two dispatch suffixes are resolved from. nil → agents.DefaultAgentSet()
	// (the built-in ATDD set), so production and existing tests need not set
	// it. An alternate set (e.g. a stub/fixture set) is bound here to swap
	// the agent layer without touching the process flow — the agent-axis
	// swap point.
	AgentSet *agents.AgentSet

	// NodeDescription is the YAML node's `name:` — surfaced in the
	// prompt so the agent has the same context the operator would have read.
	NodeDescription string

	// Ticket context — pulled from Context keys populated by preResolveIssue.
	IssueNum   int
	IssueTitle string

	// TicketID is the tracker-verbatim id (issue.ID), seeded by
	// writeResolvedIssue alongside IssueNum. Same value as IssueNum today,
	// but the dispatcher exposes it under the backend-agnostic ${ticket-id}
	// placeholder (consumed by the shared preamble) so prompts stay neutral
	// on whether the tracker is GitHub-numeric or Jira-prefixed.
	// Load-bearing: when empty AND the prompt references ${ticket-id},
	// findUnfilledPlaceholders fails the dispatch fast — same rationale as
	// Language / Checklist.
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

	// ScopeRationale is the optional free-form per-MID *why* sourced from
	// the BPMN node's `scope-rationale:` field (sibling to `read:` /
	// `write:`). The driver looks it up via engine.ScopeRationale at
	// dispatch time. Rendered by renderScopeBlock as a "Why: …" tail
	// under the auto-derived "Write-only paths" annotation when
	// `write \ read` is non-empty; empty / absent → not rendered.
	ScopeRationale string

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

	// VerifyFailureOutput is the captured stdout/stderr tail from the most
	// recent `gh optivem test run` invocation. Stamped by runCommand on
	// the `isTestRun && !succeeded` branch (see formatVerifyFailureOutput for
	// the stdout/stderr combination shape) and flows through
	// `verify_failure_output` in ctx.State to the driver's
	// cOpts.VerifyFailureOutput. Substituted into
	// fix-unexpected-passing-tests' / fix-unexpected-failing-tests'
	// ${verify-failure-output} placeholder so the fix agent reads the same
	// captured runner output the operator saw inline. Registered only
	// when non-empty so an absent value surfaces via
	// findUnfilledPlaceholders rather than silently substituting "" —
	// same rationale as CommandLine.
	VerifyFailureOutput string

	// ChangedFiles carries the working-tree path list the WRITE phase
	// produced, for substitution into fix-unexpected-passing-tests' /
	// fix-unexpected-failing-tests' / fix-scope-diff's /
	// fix-command-failed's / fix-missing-output's ${changed-files}
	// placeholder. For the three diagnostic fixers with a pre-WRITE
	// snapshot (fix-unexpected-passing-tests,
	// fix-unexpected-failing-tests, fix-scope-diff), the driver reads
	// the snapshot delta from ctx.State["phase-changed-files"] (always
	// stashed by validateOutputsAndScopes per plan 20260527-1536); the
	// live `git status --porcelain` fallback runs only when the stash
	// is absent. For fix-command-failed and fix-missing-output (no
	// pre-WRITE snapshot), the driver always uses the live shell-out.
	// Registered unconditionally — fix-command-failed may legitimately
	// dispatch with an empty working tree.
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
	CommandLine       string
	CommandExitCode   int
	CommandStderrTail string

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
	FailingTaskName string
	MissingOutputs  string
	ViolatingPaths  string

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
	// path keys (system-driver-port, system-driver-adapter, at-test, …) plus the
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

	// Headless — when true, run via `claude -p` (one-shot, headless).
	// When false, run interactively so the operator can observe / interject.
	Headless bool

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

	// EventsLogPath, when non-empty (headless mode only), is the file path
	// runHeadless tees the per-event `claude -p --output-format stream-json
	// --verbose` NDJSON output to (creating parent dirs as needed). One
	// JSON object per line; the terminal `type:"result"` line carries the
	// same `result` + `usage` + `total_cost_usd` fields the old single
	// envelope used to. The driver computes a per-run-and-dispatch path so
	// the audit log persists after the run — mirrors PromptLogPath. I/O
	// failure is a non-fatal warning to Stderr; diagnostics must not
	// break the dispatch (same policy as writePromptLog). Empty string
	// skips the audit log (interactive mode never writes one — ANSI
	// escapes make file-mirroring noisy).
	EventsLogPath string

	// EventsTextLogPath, when non-empty (headless mode only), is the file
	// path runHeadless tees a human-readable plain-text rendering of the
	// stream-json events to. Sibling of EventsLogPath: same stream, but
	// formatted to look like the interactive Claude Code transcript
	// (assistant text rendered inline, tool calls one-lined, tool results
	// summarised) so operators can read it without piping through `jq`.
	// I/O failure is a non-fatal warning to Stderr; same fail-soft policy
	// as EventsLogPath. Empty string skips the text log.
	EventsTextLogPath string

	// PidFilePath, when non-empty, is the file path runHeadless /
	// runInteractive writes a JSON marker (child_pid, parent_pid, cwd) to
	// immediately after cmd.Start() and removes on clean dispatch exit.
	// Used by `gh optivem doctor --orphans` to identify children that
	// survived a crash: the file's presence IS the crash signature, so
	// removal-on-defer only fires when the dispatch reaches the defer
	// (not when it panics or is force-killed). Empty path → no marker
	// file written (back-compat for non-dispatched callers, e.g. utility
	// runs). I/O failure is a non-fatal warning to Stderr; diagnostics
	// must not break the dispatch (same policy as openEventsLog).
	PidFilePath string

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

	// AttemptNumber / AttemptMax describe where this dispatch sits in a
	// max-visits loop, mapped by the driver from the engine's generic
	// per-node visit count (statemachine writes visit-count / visit-max
	// onto the run Context). AttemptNumber is 1-based; AttemptMax is the
	// loop cap. Only meaningful when AttemptMax > 0 (a looped node such as
	// a fixer): renderPrompt then fills ${attempt-block} with a pre-rendered
	// "Attempt #N of M" line. When AttemptMax == 0 (a non-looped, single-
	// pass node), ${attempt-block} is left unfilled — so a prompt that does
	// not reference it is unaffected, and one that does trips
	// findUnfilledPlaceholders (only the fixer preamble references it, and
	// fixers are always loop nodes, so it is always fillable there).
	AttemptNumber int
	AttemptMax    int

	// ShowPrompt, when true, dumps the full rendered prompt to Stdout
	// between the prepared-prompt summary banner and the `[agent] enter`
	// banner. Off by default; useful for debugging template edits or
	// auditing a new agent's body.
	ShowPrompt bool

	// RepoPath is the working directory the subprocess runs in (and the
	// directory git rev-parse / git log query). Empty → current cwd.
	RepoPath string

	// Stdout / Stderr targets. Stdout is the back-compat fallback when Out
	// is nil (driver populates Out via installLogFileMirror; callers that
	// bypass that path keep Stdout-only behaviour). Stderr is unaffected
	// by the level architecture — low-volume, no filter applied.
	// nil → os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Out routes Fprint sites by level (Phase / Detail). The driver
	// passes its own opts.Out through, so terminal and --log-file sinks
	// see the same level-tagged banners they get from trace and from
	// driver.go. nil → withDefaults builds outlog.Default(Stdout) so
	// tests that only set Stdout still see every banner.
	Out *outlog.Out

	// Stdin is the operator's TTY in interactive mode. nil → os.Stdin.
	Stdin io.Reader

	// Approval is the resolved auto-approve policy that gets forwarded
	// to the spawned claude subprocess via GH_OPTIVEM_AUTO and
	// GH_OPTIVEM_CONFIRM env vars. The cobra layer reads it off the
	// command context and the driver passes it through here. Without
	// this, nested `gh optivem` calls inside the agent fall back to
	// cautious mode and block on prompts the operator can't see.
	Approval approval.Resolved
}

// ClaudeRunner runs the `claude` CLI. The default implementation is
// execClaude. RunOpts is a struct rather than a varargs slice because the
// runner has to choose between interactive and `-p` invocations and stream
// stdout/stderr back to the driver during long headless runs.
type ClaudeRunner interface {
	Run(ctx context.Context, opts RunOpts) (RunResult, error)
}

// RunOpts is the cross-cut between Options and the subprocess invocation.
// It hides the headless-vs-interactive flag-shape decision behind the
// runner so the production runner can evolve without touching Dispatch.
//
// OutputFilePath / OutputKeysSpec, when non-empty, are exported into the
// subprocess env as GH_OPTIVEM_OUTPUT_FILE / GH_OPTIVEM_OUTPUT_KEYS so the
// agent's `gh optivem output write` CLI calls land in the right file with
// the right allow-list. Mirrors the same fields on Options. Both modes
// (interactive and headless) honour the export uniformly — the JSONL
// channel is the cross-mode replacement for the prose-YAML tail that used
// to work only in headless mode.
type RunOpts struct {
	Prompt   string
	Headless bool
	Model    string
	Effort   string
	Dir      string
	Stdin    io.Reader
	// Stdout is the writer the interactive (claude TUI) subprocess pipes
	// its stdout into. For Detail-level routing in production, Dispatch
	// passes opts.Out.Detail through this field; tests may pass any
	// io.Writer directly.
	Stdout io.Writer
	Stderr io.Writer
	// Out is the level-tagged writer set, populated by Dispatch from the
	// caller's opts.Out. runHeadless uses it to direct the final
	// resultText to Phase (operator-facing summary) while keeping the
	// captured stream at Detail. nil → runHeadless treats both as the
	// same writer (back-compat with Stdout).
	Out            *outlog.Out
	OutputFilePath string
	OutputKeysSpec string

	// EventsLogPath mirrors clauderun.Options.EventsLogPath. Honoured by
	// runHeadless only; runInteractive ignores it. Empty → no audit log.
	EventsLogPath string

	// EventsTextLogPath mirrors clauderun.Options.EventsTextLogPath. The
	// sibling plain-text transcript next to the .events.jsonl audit log.
	// Honoured by runHeadless only; empty → no text log.
	EventsTextLogPath string

	// PidFilePath mirrors clauderun.Options.PidFilePath. Honoured by both
	// runHeadless and runInteractive (force-cancel can hit either mode).
	// Empty → no marker file written.
	PidFilePath string

	// Approval is the resolved auto-approve policy from the parent
	// process. Propagated into the child `claude` subprocess environment
	// as GH_OPTIVEM_AUTO and GH_OPTIVEM_CONFIRM so nested `gh optivem`
	// calls inside the agent see the same policy the operator chose at
	// the parent level. The child's own approval.Resolve reads those env
	// vars and emits a banner with auto-source: env — the documented
	// audit trail. Zero value leaves the env unset, so child inherits
	// only what was already in the parent env.
	Approval approval.Resolved
}

// RunResult is what the runner reports back to Dispatch. Usage is best-effort
// — populated only when the runner can parse a structured envelope (currently
// headless mode via `claude -p --output-format stream-json --verbose`, by
// reading the terminal `type:"result"` event from the stream). Interactive
// mode leaves it nil and the banner falls back to elapsed-time-only.
//
// ResultText is the agent's final response body — the `result` field from
// the terminal stream-json result event. Populated only in headless mode
// (interactive mode prints directly to the operator's TTY and has no
// envelope to parse). Used by the exit-banner result echo so the
// operator sees the agent's final message inline. Structured outputs
// flow through the per-dispatch JSONL channel
// (GH_OPTIVEM_OUTPUT_FILE) — not through ResultText anymore.
type RunResult struct {
	Usage      *TokenUsage
	ResultText string
}

// TokenUsage is the cost/throughput summary surfaced in the exit banner.
// Field names mirror the `usage` object inside the stream-json terminal
// `type:"result"` event so the JSON shape can be decoded directly into
// this struct.
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
// clauderun.
func Dispatch(ctx context.Context, deps Deps, opts Options) (RunResult, error) {
	deps = deps.withDefaults()
	opts = opts.withDefaults()

	prompt := opts.RawPrompt
	if prompt == "" {
		var err error
		prompt, err = renderPrompt(opts)
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
		// Full rendered prompt is a debugging artefact — Detail level.
		fmt.Fprintln(opts.Out.Detail, prompt)
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

	// Stdout routing differs by mode. Headless: the `claude -p` stream-json
	// is a captured byte stream — route it to Detail so it's teed to the
	// audit logs and filtered off the default (Phase-level) terminal.
	// Interactive: the Claude Code TUI must render to a real TTY and the
	// operator must see it, so hand the child the raw terminal stdout
	// (opts.Stdout, which installLogFileMirror never rewraps). opts.Out.Detail
	// is wrong for interactive on two counts: at the default terminal level
	// it collapses to io.Discard (nothing renders — the original bug), and
	// under --log-file it becomes an io.MultiWriter, i.e. a pipe rather than
	// a TTY. Stderr stays on runStderr in both modes so rate-limit / auth
	// classification keeps working; the TUI keys interactivity off stdout.
	runnerStdout := opts.Out.Detail
	if !opts.Headless {
		runnerStdout = opts.Stdout
	}

	runResult, runErr := deps.Claude.Run(ctx, RunOpts{
		Prompt:            prompt,
		Headless:          opts.Headless,
		Model:             opts.Model,
		Effort:            opts.Effort,
		Dir:               opts.RepoPath,
		Stdin:             opts.Stdin,
		Stdout:            runnerStdout,
		Stderr:            runStderr,
		Out:               opts.Out,
		OutputFilePath:    opts.OutputFilePath,
		OutputKeysSpec:    opts.OutputKeysSpec,
		EventsLogPath:     opts.EventsLogPath,
		EventsTextLogPath: opts.EventsTextLogPath,
		PidFilePath:       opts.PidFilePath,
		Approval:          opts.Approval,
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

	writeExitBanner(opts, len(changed), nowFn().Sub(startedAt), runResult.Usage, nil)
	return runResult, nil
}

// nowFn is a package-level seam so tests can pin elapsed time in banner
// output. Production points at time.Now.
var nowFn = time.Now

// rendererReEntryPolicy is the one-line "if your previous WRITE didn't
// compile, fix minimally" clause inlined into writing-implementer
// prompts via ${re-entry-policy}. Centralised here so a policy change
// (e.g. "re-runs always start fresh") needs only one edit. Per-agent
// specifics (which stub kind, agent-specific don't-touch clauses)
// stay in each prompt's body — this constant carries only the shared
// core.
const rendererReEntryPolicy = "If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten stub, signature mismatch, typo) and fix it minimally."

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
// the test file; not exported.
func renderPrompt(opts Options) (string, error) {
	set := opts.AgentSet
	if set == nil {
		set = agents.DefaultAgentSet()
	}
	var body string
	if opts.PromptOverride != "" {
		body = opts.PromptOverride
	} else {
		var err error
		body, err = set.Prompt(opts.Agent)
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
	for k, v := range map[string]string{
		"issue-num":       strconv.Itoa(opts.IssueNum),
		"issue-title":     opts.IssueTitle,
		"phase":           opts.NodeDescription,
		"architecture":    opts.Architecture,
		"subtype":         opts.Subtype,
		"changed-files":   opts.ChangedFiles,
		"re-entry-policy": rendererReEntryPolicy,
	} {
		params[k] = v
	}
	// VerifyFailureOutput is load-bearing for fix-unexpected-{failing,passing}-tests
	// — same rationale as CommandLine. Only registered when non-empty so
	// an absent value surfaces via findUnfilledPlaceholders rather than
	// silently substituting "" into the diagnostic prompt's
	// `### Verify results to address` block. ChangedFiles stays
	// unconditional because fix-command-failed legitimately dispatches
	// with an empty working tree.
	if opts.VerifyFailureOutput != "" {
		params["verify-failure-output"] = opts.VerifyFailureOutput
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
		params["scope-block"] = renderScopeBlock(opts.ScopeRead, opts.ScopeWrite, opts.Placeholders, opts.ScopeRationale)
	}
	// Loop-attempt block (plan 20260616-0649). Pre-rendered loop-level
	// caller context for looped nodes (fixers and any future max-visits
	// loop): a single ${attempt-block} string the prompt author drops in,
	// never raw ${attempt-number}/${attempt-max}. Registered only when
	// AttemptMax > 0 (the dispatch is inside a loop). For a single-pass
	// node AttemptMax == 0, so the key stays unfilled — a prompt that omits
	// ${attempt-block} is unaffected, and one that references it would trip
	// findUnfilledPlaceholders (by design: only loop nodes may reference
	// it, and there it is always fillable). Mirrors the scope-block
	// load-bearing registration pattern above.
	if opts.AttemptMax > 0 {
		params["attempt-block"] = renderAttemptBlock(opts.AttemptNumber, opts.AttemptMax)
	}
	// Language is load-bearing — only registered when the driver supplied
	// a value. An empty value would silently substitute, masking a config
	// bug; instead we leave `${language}` unfilled so findUnfilledPlaceholders
	// reports it after substitution.
	if opts.Language != "" {
		params["language"] = opts.Language
	}
	// TicketID feeds the ${ticket-id} placeholder consumed by the shared
	// preamble. Same registration shape as Language — only registered when
	// non-empty so an absent value surfaces via findUnfilledPlaceholders
	// rather than silently substituting "" when the prompt references it.
	if opts.TicketID != "" {
		params["ticket-id"] = opts.TicketID
	}
	// WIP-gate marker (acceptance-test-writer). The gate is permanent
	// and ticket-independent — it keys on the GH_OPTIVEM_RUN_WIP_TESTS
	// env var, not a per-ticket reason — so the renderer takes only the
	// language. Same load-bearing pattern as Language: register only
	// when the helper returns a non-empty string (language recognised);
	// otherwise findUnfilledPlaceholders fails the dispatch fast when
	// the prompt body references ${gate-marker-example}. Inlining the
	// marker shape here replaces the previous "go read the
	// language-equivalents row" instruction in the agent body — the row
	// is out of scope and the agent was forced to grep in-scope tests
	// to reverse-engineer the syntax (3-5 wasted tool calls per
	// dispatch).
	if ex := renderGateMarkerExample(opts.Language); ex != "" {
		params["gate-marker-example"] = ex
	}
	// Isolation marker (acceptance-test-writer). Mirrors the Gherkin
	// `@isolated` tag in the Acceptance Criteria onto the test's
	// language isolation shape (class-level @Isolated in Java, the
	// [Collection("Isolated")]+[Trait] pair in C#, the serial-mode
	// describe wrapper in TypeScript). Same load-bearing registration as
	// the WIP gate above: register only when the language is recognised,
	// else findUnfilledPlaceholders fails the dispatch fast when the
	// prompt references ${isolated-marker-example}. The per-language
	// shapes are non-obvious and live outside the agent's read scope, so
	// they are injected rather than reverse-engineered.
	if ex := renderIsolatedMarkerExample(opts.Language); ex != "" {
		params["isolated-marker-example"] = ex
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
	// Interactive dispatches keep the Claude Code REPL open after the
	// agent finishes; new operators don't always know to `/exit` to
	// continue the cycle (vs. typing redirect feedback). Appending the
	// operator-facing hint only when !Headless keeps headless prompts
	// uncluttered — the suffix would be a no-op without a REPL.
	if !opts.Headless {
		rendered = strings.TrimRight(rendered, "\n") + "\n\n" + set.InteractiveSuffix() + "\n"
	}
	// Headless dispatches (`claude -p`) have no operator to answer an
	// AskUserQuestion, so any such call only ever auto-rejects and burns
	// turns. Appending the no-ask clause tells the agent to resolve
	// ambiguity itself and proceed — the symmetric counterpart to the
	// interactive suffix above.
	if opts.Headless {
		rendered = strings.TrimRight(rendered, "\n") + "\n\n" + set.HeadlessSuffix() + "\n"
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
//	Emit each declared output by calling `gh optivem output write KEY=VAL`
//	from the `Bash` tool (multiple `KEY=VAL` allowed per call;
//	last-write-wins on re-call). The dispatcher reads the per-invocation
//	JSONL file after you exit.
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
	b.WriteString("Emit each declared output by calling `gh optivem output write KEY=VAL` from the `Bash` tool (multiple `KEY=VAL` allowed per call; last-write-wins on re-call). The dispatcher reads the per-invocation JSONL file after you exit.\n\n")
	if len(required) > 0 {
		b.WriteString("Required outputs:\n")
		for _, s := range required {
			fmt.Fprintf(&b, "  %s: %s\n", s.Key, s.Type)
		}
	}
	if len(optional) > 0 {
		if len(required) > 0 {
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

// renderAttemptBlock produces the body of the ${attempt-block}
// substitution: a one-line loop-level caller note telling a looped agent
// where it sits in its fix loop (e.g. "Attempt #2 of 2 — the loop halts
// after this pass."). number is 1-based; max is the loop cap. Only called
// when max > 0 (renderPrompt gates the registration), so the "last pass"
// clause is shown on the final attempt and a neutral "more attempts
// remain" framing otherwise. This is caller context, NOT permission to
// self-retry — the fixer preamble's "one attempt only" rule still governs
// the agent's own single pass; see fixer-preamble.md.
func renderAttemptBlock(number, max int) string {
	if number >= max {
		return fmt.Sprintf("Attempt #%d of %d — this is the final pass; the orchestrator's fix loop is exhausted after it and a human is asked to widen scope.", number, max)
	}
	return fmt.Sprintf("Attempt #%d of %d — the orchestrator re-validates after you exit and may dispatch one more pass before the loop is exhausted.", number, max)
}

// renderScopeBlock produces the body of the ${scope-block} substitution:
// two key-list blocks (read, then write) plus the scope_exception escape
// hatch line. The `### Scope` heading itself is owned by the prompt
// source (each in-scope prompt writes `### Scope\n\n${scope-block}` under
// `## Inputs`), so this helper renders only the body.
//
// Layer keys are resolved against the supplied paths map — same source
// the inline ${key} placeholders resolve through (opts.Placeholders, fed
// by cfg.PlaceholderMap()). Family B path keys (system-driver-port, dsl-core,
// …) plus the Family A `system-path` are all in scope. Unresolved keys
// (nil paths, or key not in the map) emit as `<key>: (unresolved)`
// rather than disappearing silently — the build-time canonical-keys
// guard catches drift before runtime, but if it slips through, the
// agent sees the gap.
func renderScopeBlock(read, write []string, paths map[string]string, rationale string) string {
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

	// Auto-derive the asymmetry annotation: any key in `write:` but not
	// `read:` is write-only. Surface this as a generic rule on the
	// rendered block so the agent sees the asymmetry at every dispatch
	// without each writer prompt having to restate it. When the MID
	// also declares scope-rationale:, tack the per-MID *why* on as a
	// "Why:" tail (the test-writer MIDs use this; others may but don't
	// today). Preserves write-order for stable output.
	readSet := map[string]bool{}
	for _, k := range read {
		readSet[k] = true
	}
	var writeOnly []string
	for _, k := range write {
		if !readSet[k] {
			writeOnly = append(writeOnly, k)
		}
	}
	if len(writeOnly) > 0 {
		fmt.Fprintf(&b, "\nWrite-only paths (in `write:` but not `read:`): %s. "+
			"Treat these as append-only or edit-by-location — do not read their "+
			"existing contents for context.\n", strings.Join(writeOnly, ", "))
		if rationale != "" {
			fmt.Fprintf(&b, "Why: %s\n", strings.TrimSpace(rationale))
		}
	}

	b.WriteString("\nReading or writing outside this set requires a `scope_exception` block.")
	return b.String()
}

// renderGateMarkerExample inlines the per-language WIP-gate snippet the
// acceptance-test-writer prepends to every AT method, via
// ${gate-marker-example}. Previously the agent had to look up the
// syntax in the language-equivalents reference doc (which is out of
// scope under the agent's `read:` set) or grep the in-scope test tree
// to reverse-engineer it — 3-5 wasted tool calls per dispatch. With
// the snippet inlined the agent reads the target test file once and
// edits it.
//
// The gate is PERMANENT and ticket-independent: it stays in the
// committed code for the test's whole lifetime, so it carries no
// per-ticket reason string (unlike the disable marker it replaces).
// The gate keys on the GH_OPTIVEM_RUN_WIP_TESTS env var, which only
// the ATDD orchestrator sets (=1) when it runs verify steps; regular
// CI, local `mvn test` / `dotnet test` / `npx playwright test`, and
// IDE runs leave it unset, so the AT is silently skipped. No enabler
// re-runs it, no disabler re-applies it.
//
// Returns "" when `lang` is empty, unrecognised, or its asset is
// missing; the caller registers the placeholder only when non-empty so
// an absent value surfaces via findUnfilledPlaceholders rather than
// silently substituting "". The snippet body lives in
// runtime/shared/wip-gate-<lang>.md (embedded via assets.FS) so it is
// visible/editable as prose rather than a buried Go literal, and is
// shaped as an *additive* gate layered onto the scaffold's
// channel-parameterized declaration — never a standalone test method
// (the standalone shape regressed rehearsal-71 by dropping
// @TestTemplate/@Channel). Adding a new language requires creating its
// wip-gate-<lang>.md asset AND adding the case to the switch below.
func renderGateMarkerExample(lang string) string {
	switch lang {
	case "java", "csharp", "typescript":
	default:
		return ""
	}
	data, err := assets.FS.ReadFile("runtime/shared/wip-gate-" + lang + ".md")
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
}

// renderIsolatedMarkerExample inlines the per-language isolation snippet
// the acceptance-test-writer applies to a test whose source scenario
// carries the Gherkin `@isolated` tag, via ${isolated-marker-example}.
// It is the isolation twin of renderGateMarkerExample: the writer is a
// mechanical 1:1 translator, so the per-language shape (class-level
// @Isolated in Java, the [Collection("Isolated")]+[Trait] pair in C#,
// the serial-mode describe wrapper in TypeScript) is injected rather
// than reverse-engineered from out-of-scope sibling tests.
//
// Unlike the WIP gate the isolation shape is class/describe-level, not
// per-method: an isolated scenario yields a dedicated isolated test
// class/describe block.
//
// Returns "" when `lang` is empty, unrecognised, or its asset is
// missing; the caller registers the placeholder only when non-empty so
// an absent value surfaces via findUnfilledPlaceholders rather than
// silently substituting "". The snippet body lives in
// runtime/shared/isolated-marker-<lang>.md (embedded via assets.FS) so
// it is visible/editable as prose. Adding a new language requires
// creating its isolated-marker-<lang>.md asset AND adding the case to
// the switch below.
func renderIsolatedMarkerExample(lang string) string {
	switch lang {
	case "java", "csharp", "typescript":
	default:
		return ""
	}
	data, err := assets.FS.ReadFile("runtime/shared/isolated-marker-" + lang + ".md")
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
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
	if o.Out == nil {
		o.Out = outlog.Default(o.Stdout)
	}
	return o
}

// ---------------------------------------------------------------------------
// Repo snapshot (pre/post) and HEAD detection
// ---------------------------------------------------------------------------

// repoState is the "before"/"after" snapshot Dispatch takes around the
// runner call. The diff catches branch-switch: the agent runs `git
// checkout -b feature/foo`, commits there, and never returns. HEAD on
// the original branch is unchanged → without snapshot we'd halt with
// the misleading "no commit produced"; with snapshot we say "switched
// branches".
type repoState struct {
	head   string
	branch string
	// dirty is the union of every path mentioned in `git status
	// --porcelain` (untracked, modified, deleted, staged, …). The
	// post-pre delta is "what the agent touched that wasn't already
	// dirty," used by the downstream COMMIT_* node (which runs
	// `gh optivem commit --yes --include-untracked`) to stage and
	// commit the working-tree delta.
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
		head:   strings.TrimSpace(string(headOut)),
		branch: strings.TrimSpace(string(branchOut)),
		dirty:  parseDirty(statusOut),
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
	// withDefaults is idempotent — safe to apply both here (for test
	// callers that bypass Dispatch) and inside Dispatch's pre-pipeline
	// step. Without it, tests that construct Options without an Out get
	// a nil-deref on opts.Out.Detail.
	opts = opts.withDefaults()
	cyan := color.New(color.FgCyan)
	// Detail-level: the prepared-prompt banner is a verbose summary used
	// for debugging; the headline `[agent] enter` line in writeEnterBanner
	// is what the operator needs to see by default. Not bold — quieter
	// than the Phase-sink banners by design.
	w := opts.Out.Detail
	if opts.RawPrompt != "" {
		fmt.Fprintln(w, cyan.Sprintf("[agent]  prep   %s  (override mode — %s)",
			opts.Agent, formatPromptSize(len(prompt))))
		return
	}
	fmt.Fprintln(w, cyan.Sprintf("[agent]  prep   %s", opts.Agent))
	fmt.Fprintln(w, cyan.Sprintf("         size:                %s", formatPromptSize(len(prompt))))
	fmt.Fprintln(w, cyan.Sprintf("         architecture:        %s", orPlaceholderClauderun(opts.Architecture, "(empty)")))
	fmt.Fprintln(w, cyan.Sprintf("         scope:               %s", summarizeScope(opts.ScopeRead, opts.ScopeWrite)))
	fmt.Fprintln(w, cyan.Sprintf("         acceptance criteria: %s", summarizeAcceptanceCriteria(opts.AcceptanceCriteria)))
	writeIndentedBlock(w, cyan, opts.AcceptanceCriteria)
	fmt.Fprintln(w, cyan.Sprintf("         checklist:           %s", summarizeChecklist(opts.Checklist)))
	writeIndentedBlock(w, cyan, opts.Checklist)
	fmt.Fprintln(w, cyan.Sprintf("         override text:       %s", orPlaceholderClauderun(opts.OverrideText, "(none)")))
	fmt.Fprintln(w, cyan.Sprintf("         log:                 %s", orPlaceholderClauderun(opts.PromptLogPath, "(none)")))
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
		fmt.Fprintln(w, c.Sprintf("           %s", trimmed))
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
	opts = opts.withDefaults()
	cyan := color.New(color.FgCyan, color.Bold)
	w := opts.Out.Phase
	mode := "interactive"
	if opts.Headless {
		mode = "headless"
	}
	fmt.Fprintln(w, cyan.Sprintf("[agent]  enter  %s  (%s)", opts.Agent, mode))
	if opts.IssueNum > 0 || opts.IssueTitle != "" {
		fmt.Fprintln(w, cyan.Sprintf("         Issue: #%d %q",
			opts.IssueNum, opts.IssueTitle))
	}
	// Surface --model / --effort so the operator can confirm which tuning
	// the dispatcher actually picked for this agent. Empty values fall
	// back to "(claude session default)" rather than being silently
	// elided — no silent inheritance.
	fmt.Fprintln(w, cyan.Sprintf("         Tuning: %s", formatTuning(opts.Model, opts.Effort)))
	// Surface the delivery channel for channel-split dispatches (system
	// driver adapter / system implementers, unrolled per project channel
	// by UnrollSystemChannels). The channel scopes WHAT the agent
	// implements — `api` vs `ui` — but lived only inside the substituted
	// prompt body, so the operator couldn't tell from the banner whether
	// the run was one channel or all. Gated on presence: channel-agnostic
	// dispatches (most phases) carry no `channel` node-param and emit no
	// line.
	if ch := opts.NodeParams["channel"]; ch != "" {
		fmt.Fprintln(w, cyan.Sprintf("         Channel: %s", ch))
	}
	// Surface the per-dispatch log paths so a headless operator —
	// who has no TTY to interject through — knows where to tail for
	// the rendered prompt, the claude event stream, and the agent's
	// declared-outputs JSONL. Each line is gated on a non-empty path:
	// MIDs with no declared outputs skip Outputs, and test paths that
	// don't set any of them simply emit none. The Events lines are
	// additionally gated on Headless: only runHeadless tees the
	// stream-json audit logs, so advertising those paths for an
	// interactive dispatch (where runInteractive never writes them)
	// would point the operator at files that never appear.
	if opts.PromptLogPath != "" {
		fmt.Fprintln(w, cyan.Sprintf("         Prompt log:  %s", opts.PromptLogPath))
	}
	if opts.Headless && opts.EventsLogPath != "" {
		fmt.Fprintln(w, cyan.Sprintf("         Events log:  %s", opts.EventsLogPath))
	}
	if opts.Headless && opts.EventsTextLogPath != "" {
		fmt.Fprintln(w, cyan.Sprintf("         Events text: %s", opts.EventsTextLogPath))
	}
	if opts.OutputFilePath != "" {
		fmt.Fprintln(w, cyan.Sprintf("         Outputs log: %s", opts.OutputFilePath))
	}
}

// writeExitBanner reports the agent's exit. changedFiles is the count of
// paths the agent touched (post-pre delta of `git status --porcelain`); the
// outer CLI is what stages and commits them, so the banner only signals
// the size of the work it has to act on. Zero changes is a legitimate
// no-op — still a successful exit, surfaced so the operator can tell at a
// glance there's nothing for the wrapper to commit.
func writeExitBanner(opts Options, changedFiles int, elapsed time.Duration, usage *TokenUsage, runErr error) {
	opts = opts.withDefaults()
	w := opts.Out.Phase
	if runErr != nil {
		red := color.New(color.FgRed, color.Bold)
		fmt.Fprintln(w, red.Sprintf("[agent]  FAIL   %s  (%s%s)", opts.Agent, elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
		fmt.Fprintln(w, red.Sprintf("         %s", runErr))
		return
	}
	green := color.New(color.FgGreen, color.Bold)
	if changedFiles == 0 {
		fmt.Fprintln(w, green.Sprintf("[agent]  exit   no changes  (%s%s)",
			elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
		return
	}
	fmt.Fprintln(w, green.Sprintf("[agent]  exit   %d files  (%s%s)",
		changedFiles, elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
}

// SplitInputTokens buckets a usage's input into fresh (paid-for this turn)
// and cached (cheap reuse). fresh = input_tokens + cache_creation: a cache
// write is billed at ≥ full rate this turn, so it belongs with genuinely
// fresh input, not with the reuse. cached = cache_read, strictly the cheap
// reuse. Single source of truth for the bucketing so the summary table and
// the per-dispatch banner can't drift.
func SplitInputTokens(usage *TokenUsage) (fresh, cached int) {
	if usage == nil {
		return 0, 0
	}
	return usage.InputTokens + usage.CacheCreationInputTokens, usage.CacheReadInputTokens
}

// formatUsageSuffix renders ", 7.0k fresh / 1355.6k cached / 1.8k out, $0.18"
// if usage is non-nil and non-empty. Returns "" otherwise so the banner
// gracefully degrades to elapsed-time-only when the runner couldn't extract
// a JSON envelope (e.g. interactive mode, or a headless-mode parse failure).
func formatUsageSuffix(usage *TokenUsage) string {
	if usage == nil {
		return ""
	}
	fresh, cached := SplitInputTokens(usage)
	out := usage.OutputTokens
	if fresh == 0 && cached == 0 && out == 0 && usage.TotalCostUSD == 0 {
		return ""
	}
	return fmt.Sprintf(", %s fresh / %s cached / %s out, $%.2f",
		formatTokens(fresh), formatTokens(cached), formatTokens(out), usage.TotalCostUSD)
}

// formatTuning renders the enter-banner Tuning: line. Both set →
// "model=sonnet, effort=high". One set → that key only. Neither set →
// "(claude session default)" so the operator sees an explicit signal
// that nothing was overridden, instead of silent inheritance.
func formatTuning(model, effort string) string {
	switch {
	case model != "" && effort != "":
		return fmt.Sprintf("model=%s, effort=%s", model, effort)
	case model != "":
		return fmt.Sprintf("model=%s", model)
	case effort != "":
		return fmt.Sprintf("effort=%s", effort)
	default:
		return "(claude session default)"
	}
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
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
// Headless mode → `claude -p <prompt> --output-format stream-json --verbose`:
//
//   - The prompt is the embedded agent's full instructions (rendered by
//     renderPrompt). v2 has no host/subagent split — `claude -p` IS the
//     agent and needs the default tool set (Read/Glob/Grep/Edit/Write/Bash)
//     to do real work.
//   - --output-format stream-json --verbose emits one JSON event per line
//     on stdout — assistant/user/system messages during the run, then a
//     terminal `type:"result"` line carrying `result` (final answer),
//     `usage.{input,output,cache_*}_tokens`, and `total_cost_usd`. The
//     stream is teed to the per-dispatch .events.jsonl audit log (when
//     opts.EventsLogPath is set) so unattended runs persist a full
//     replay of every tool call / message — closing the audit gap that
//     the previous single-envelope mode left for `--auto --headless`
//     rehearsals.
//   - parseClaudeStreamJSON scans the captured stdout for the terminal
//     result event and pulls the same usage/result/cost fields the
//     pre-stream-json banner used to print, so the exit banner shape is
//     unchanged from the operator's point of view.
//
// JSON parsing is best-effort — a future CLI version that changes the
// event shape leaves Usage nil and the banner falls back gracefully.
func (execClaude) Run(ctx context.Context, opts RunOpts) (RunResult, error) {
	if opts.Headless {
		return runHeadless(ctx, opts)
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
	cmd.Env = subprocessEnv(opts)
	// Split Run() into Start() + Wait() so we can capture the spawned
	// PID into the orphan-recovery marker file. Same pattern as
	// runHeadless — interactive dispatches can be Ctrl+C'd just as
	// easily and produce the same orphan-claude problem on Windows.
	if err := cmd.Start(); err != nil {
		return RunResult{}, err
	}
	if opts.PidFilePath != "" {
		writePidFile(opts.PidFilePath, userstate.PidMarker{
			ChildPid:  cmd.Process.Pid,
			ParentPid: os.Getpid(),
			Cwd:       dispatchCwd(opts.Dir),
		}, opts.Stderr)
	}
	runErr := cmd.Wait()
	if opts.PidFilePath != "" && runErr == nil {
		removePidFile(opts.PidFilePath, opts.Stderr)
	}
	return RunResult{}, runErr
}

// subprocessEnv composes the subprocess environment, appending the
// per-dispatch GH_OPTIVEM_OUTPUT_FILE / GH_OPTIVEM_OUTPUT_KEYS (when the
// dispatch declares outputs) and GH_OPTIVEM_AUTO / GH_OPTIVEM_CONFIRM
// (when the parent has Auto on) to the inherited parent env. Returns nil
// when none of the four apply — leaving cmd.Env nil makes os/exec inherit
// the parent env unmodified (the pre-refactor behaviour), preserving the
// "utility run, no extras" case.
//
// Vars are appended after os.Environ() so they overwrite any same-named
// inherited values — defensive against an operator who set them by hand.
// The forwarded confirm floor is the single tier the parent resolved; the
// child's approval.Resolve re-parses it.
func subprocessEnv(opts RunOpts) []string {
	envApproval := opts.Approval.Auto
	if !envApproval && opts.OutputFilePath == "" && opts.OutputKeysSpec == "" {
		return nil
	}
	env := os.Environ()
	if opts.OutputFilePath != "" {
		env = append(env, "GH_OPTIVEM_OUTPUT_FILE="+opts.OutputFilePath)
	}
	if opts.OutputKeysSpec != "" {
		env = append(env, "GH_OPTIVEM_OUTPUT_KEYS="+opts.OutputKeysSpec)
	}
	if envApproval {
		env = append(env, approval.EnvAuto+"=true")
		env = append(env, approval.EnvConfirm+"="+opts.Approval.ConfirmFloorString())
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

func runHeadless(ctx context.Context, opts RunOpts) (RunResult, error) {
	arg, cleanup, err := materializePrompt(opts.Dir, opts.Prompt)
	if err != nil {
		return RunResult{}, err
	}
	defer cleanup()
	args := claudeTuningArgs(opts)
	args = append(args,
		"-p", arg,
		"--output-format", "stream-json",
		"--verbose",
	)
	cmd := exec.CommandContext(ctx, "claude", args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin

	// Tee stdout to (a) an in-memory accumulator that parseClaudeStreamJSON
	// scans for the terminal `type:"result"` event after the run, (b) the
	// per-dispatch .events.jsonl audit log when EventsLogPath is set, and
	// (c) a sibling plain-text transcript when EventsTextLogPath is set.
	// Both file sinks are open-on-demand: a missing/unwritable path
	// downgrades to a non-fatal stderr warning so diagnostics never break
	// the dispatch (same policy as writePromptLog).
	var captured bytes.Buffer
	eventsSink, closeEvents := openEventsLog(opts.EventsLogPath, opts.Stderr)
	defer closeEvents()
	textSink, closeText := openEventsTextLog(opts.EventsTextLogPath, opts.Stderr)
	defer closeText()
	cmd.Stdout = io.MultiWriter(&captured, eventsSink, textSink)
	cmd.Stderr = opts.Stderr
	cmd.Env = subprocessEnv(opts)

	// Split Run() into Start() + Wait() so we can capture the spawned
	// PID into the orphan-recovery marker file between them. The PID
	// is undefined until Start() returns.
	if err := cmd.Start(); err != nil {
		return RunResult{}, err
	}
	if opts.PidFilePath != "" {
		writePidFile(opts.PidFilePath, userstate.PidMarker{
			ChildPid:  cmd.Process.Pid,
			ParentPid: os.Getpid(),
			Cwd:       dispatchCwd(opts.Dir),
		}, opts.Stderr)
	}
	runErr := cmd.Wait()
	// Remove the marker only on clean exit: a non-zero subprocess exit
	// IS a crash signature `doctor --orphans` may want to investigate,
	// so preserve the file then.
	if opts.PidFilePath != "" && runErr == nil {
		removePidFile(opts.PidFilePath, opts.Stderr)
	}

	usage, resultText := parseClaudeStreamJSON(captured.Bytes())
	if resultText != "" {
		// The agent's final assistant message is the operator-facing
		// summary of what the agent did — Phase level. Falls back to
		// the bare Stdout when Out is unset (back-compat).
		w := opts.Stdout
		if opts.Out != nil {
			w = opts.Out.Phase
		}
		fmt.Fprintln(w, resultText)
	} else if runErr != nil && captured.Len() > 0 {
		// Run failed before the result event landed — surface the raw
		// bytes so the operator sees whatever claude did print.
		opts.Stdout.Write(captured.Bytes())
	}
	return RunResult{Usage: usage, ResultText: resultText}, runErr
}

// dispatchCwd resolves the working directory recorded in the PID
// marker. Prefers the explicit opts.Dir the dispatch is running in;
// falls back to the process cwd when Dir is unset (ad-hoc invocations
// from tests / utility runs). A best-effort empty-string fallback is
// fine — the marker file's existence is what matters, the cwd field is
// human context only.
func dispatchCwd(dir string) string {
	if dir != "" {
		return dir
	}
	cwd, _ := os.Getwd()
	return cwd
}

// writePidFile serialises marker as JSON to path. Mirrors openEventsLog's
// fail-soft policy: a missing-dir or unwritable-path failure downgrades
// to a non-fatal stderr warning so diagnostics never break the dispatch.
// MkdirAll the parent dir first so the caller (driver) only has to
// compose the path.
func writePidFile(path string, marker userstate.PidMarker, stderr io.Writer) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to create pid file dir %s: %v\n", filepath.Dir(path), err)
		return
	}
	body, err := json.Marshal(marker)
	if err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to marshal pid marker for %s: %v\n", path, err)
		return
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to write pid file %s: %v\n", path, err)
	}
}

// removePidFile deletes the marker on clean dispatch exit. A missing
// file is silently ignored (the dispatch may have skipped the marker on
// the write side; either way removal succeeds vacuously). Any other
// error downgrades to a stderr warning.
func removePidFile(path string, stderr io.Writer) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "clauderun: warning: failed to remove pid file %s: %v\n", path, err)
	}
}

// openEventsLog returns a writer for the per-dispatch stream-json audit
// log and a cleanup that closes any underlying file. Behaviour:
//
//   - path == "": returns io.Discard + no-op cleanup. Headless dispatches
//     without an EventsLogPath (test fixtures, ad-hoc invocations) skip the
//     audit log entirely.
//   - path set, mkdir/open succeed: returns the *os.File + a closing
//     cleanup. Caller tees stdout into it.
//   - path set, mkdir/open fail: emits a single warning to stderr and
//     returns io.Discard + no-op cleanup. Audit-log diagnostics must not
//     break the dispatch (same non-fatal policy as writePromptLog).
//
// Extracted from runHeadless so tests can exercise the path-handling and
// error-recovery branches directly without spinning up a real subprocess.
func openEventsLog(path string, stderr io.Writer) (io.Writer, func()) {
	if path == "" {
		return io.Discard, func() {}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to create events log dir %s: %v\n", filepath.Dir(path), err)
		return io.Discard, func() {}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to open events log %s: %v\n", path, err)
		return io.Discard, func() {}
	}
	return f, func() { _ = f.Close() }
}

// openEventsTextLog mirrors openEventsLog but wraps the file in a
// streamTextWriter so the tee'd stream-json stdout lands on disk as a
// human-readable transcript (assistant text, tool calls, tool results)
// rather than raw NDJSON. The returned cleanup flushes any partial
// line still buffered before closing the file.
func openEventsTextLog(path string, stderr io.Writer) (io.Writer, func()) {
	if path == "" {
		return io.Discard, func() {}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to create events text log dir %s: %v\n", filepath.Dir(path), err)
		return io.Discard, func() {}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "clauderun: warning: failed to open events text log %s: %v\n", path, err)
		return io.Discard, func() {}
	}
	tw := &streamTextWriter{w: f}
	return tw, func() {
		tw.Flush()
		_ = f.Close()
	}
}

// streamTextWriter buffers bytes from the `claude -p --output-format
// stream-json` stdout pipe until newline boundaries, parses each
// complete JSON event line, and renders it as a human-readable line
// to the underlying writer. Designed to wrap an os.File for the
// per-dispatch .events.log audit log.
//
// Partial lines (chunks that don't end on '\n') accumulate in buf
// until the next Write completes them. Callers must invoke Flush()
// on cleanup to render any line the subprocess emitted without a
// trailing newline before exiting. Malformed JSON downgrades to a
// raw passthrough so a single CLI hiccup doesn't lose context.
type streamTextWriter struct {
	w   io.Writer
	buf []byte
}

func (s *streamTextWriter) Write(p []byte) (int, error) {
	s.buf = append(s.buf, p...)
	for {
		i := bytes.IndexByte(s.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		s.flushLine(s.buf[:i])
		s.buf = s.buf[i+1:]
	}
}

func (s *streamTextWriter) Flush() {
	if len(s.buf) > 0 {
		s.flushLine(s.buf)
		s.buf = nil
	}
}

func (s *streamTextWriter) flushLine(line []byte) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return
	}
	text, ok := formatStreamEvent(line)
	if !ok {
		_, _ = s.w.Write(line)
		_, _ = s.w.Write([]byte("\n"))
		return
	}
	if text != "" {
		_, _ = s.w.Write([]byte(text))
	}
}

// formatStreamEvent renders one stream-json line as plain text mimicking
// the interactive Claude Code transcript. Returns (text, true) on a
// recognised event type — including events that intentionally produce
// no output (e.g. rate_limit_event noise) — and ("", false) on a
// parse failure so the caller can fall back to raw passthrough.
func formatStreamEvent(line []byte) (string, bool) {
	var head struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype,omitempty"`
	}
	if err := json.Unmarshal(line, &head); err != nil {
		return "", false
	}
	switch head.Type {
	case "system":
		if head.Subtype == "init" {
			return "─── session started ───\n", true
		}
		return "", true
	case "rate_limit_event":
		return "", true
	case "assistant", "user":
		var env struct {
			Message struct {
				Role    string `json:"role"`
				Content []struct {
					Type     string          `json:"type"`
					Text     string          `json:"text,omitempty"`
					Thinking string          `json:"thinking,omitempty"`
					Name     string          `json:"name,omitempty"`
					Input    json.RawMessage `json:"input,omitempty"`
					Content  json.RawMessage `json:"content,omitempty"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			return "", false
		}
		var b strings.Builder
		for _, c := range env.Message.Content {
			switch c.Type {
			case "text":
				if c.Text != "" {
					b.WriteString(c.Text)
					if !strings.HasSuffix(c.Text, "\n") {
						b.WriteByte('\n')
					}
				}
			case "thinking":
				if c.Thinking != "" {
					b.WriteString("[thinking] ")
					b.WriteString(oneLine(c.Thinking, 400))
					b.WriteByte('\n')
				}
			case "tool_use":
				b.WriteString("[tool ")
				b.WriteString(c.Name)
				if summary := summarizeToolInput(c.Name, c.Input); summary != "" {
					b.WriteString("] ")
					b.WriteString(summary)
				} else {
					b.WriteByte(']')
				}
				b.WriteByte('\n')
			case "tool_result":
				if snippet := summarizeToolResult(c.Content); snippet != "" {
					b.WriteString("[tool_result] ")
					b.WriteString(snippet)
					b.WriteByte('\n')
				}
			}
		}
		return b.String(), true
	case "result":
		var env struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			return "", false
		}
		if env.Result == "" {
			return "─── result ───\n", true
		}
		if strings.HasSuffix(env.Result, "\n") {
			return "─── result ───\n" + env.Result, true
		}
		return "─── result ───\n" + env.Result + "\n", true
	}
	return "", true
}

// summarizeToolInput renders the tool_use input map as a one-line
// `key=value` snippet, preferring a tool-specific primary field (e.g.
// file_path for Read/Write/Edit, command for Bash) and falling back
// to compact JSON. Truncates to 200 characters to keep the transcript
// readable when a tool call carries large payloads.
func summarizeToolInput(toolName string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return oneLine(string(raw), 200)
	}
	if primary := primaryToolField(toolName); primary != "" {
		if v, ok := m[primary]; ok {
			return primary + "=" + oneLine(fmt.Sprint(v), 200)
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return oneLine(string(b), 200)
}

// summarizeToolResult condenses a tool_result content payload — which
// the CLI emits either as a plain string or as an array of content
// blocks — into a one-line snippet for the transcript.
func summarizeToolResult(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return oneLine(s, 200)
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		for _, b := range blocks {
			if b.Text != "" {
				return oneLine(b.Text, 200)
			}
		}
		return ""
	}
	return oneLine(string(raw), 200)
}

// primaryToolField returns the most operator-relevant input field for
// well-known tools so the transcript prints `Read file_path=…`
// instead of a full compact-JSON dump. Empty string means "no primary
// — fall back to compact JSON".
func primaryToolField(toolName string) string {
	switch toolName {
	case "Read", "Write", "Edit", "NotebookEdit":
		return "file_path"
	case "Bash", "PowerShell":
		return "command"
	case "Grep", "Glob":
		return "pattern"
	case "Task", "Agent":
		return "description"
	case "WebFetch":
		return "url"
	case "WebSearch":
		return "query"
	}
	return ""
}

// oneLine collapses newlines to spaces and truncates to max characters
// with a "…" ellipsis, so multi-line strings (file contents, command
// bodies) fit on a single transcript line.
func oneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	if max > 0 && len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// parseClaudeStreamJSON decodes the `claude -p --output-format stream-json
// --verbose` NDJSON stream: one JSON object per line, terminating in a
// `type:"result"` event that carries the same `result` + `usage` +
// `total_cost_usd` fields the single-envelope `--output-format json` mode
// used to. Only the terminal result event is decoded — earlier events
// (assistant / user / system / etc.) are inspected only for their `type`
// to find the result line.
//
// Returns (nil, "") on:
//   - Empty input (CLI died before emitting result — the surrounding
//     non-zero-exit error path surfaces the failure).
//   - No `type:"result"` line in the stream.
//
// Malformed mid-stream lines are skipped silently so a single CLI hiccup
// (truncated event, encoding glitch) doesn't poison the whole audit —
// callers downstream of the events log still see the raw bytes on disk.
func parseClaudeStreamJSON(b []byte) (*TokenUsage, string) {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, ""
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	// Stream events can be large (e.g. an assistant message containing a
	// long file read). Default Scanner cap is 64 KiB; lift to 1 MiB per
	// line so a chatty turn doesn't truncate the result event.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(line, &head); err != nil {
			continue
		}
		if head.Type != "result" {
			continue
		}
		var env struct {
			Result       string     `json:"result"`
			TotalCostUSD float64    `json:"total_cost_usd"`
			Usage        TokenUsage `json:"usage"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, ""
		}
		usage := env.Usage
		usage.TotalCostUSD = env.TotalCostUSD
		return &usage, env.Result
	}
	return nil, ""
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
