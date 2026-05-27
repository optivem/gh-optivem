// Bindings — Go implementations of every service-task `action:` referenced
// in internal/atdd/runtime/statemachine/process-flow.yaml.
//
// Actions are the mechanical work of the pipeline: mark the picked ticket
// through its status columns, enforce phase scope after the agent commits,
// and dispatch the BPMN Phase D LOW primitives (run-command,
// validate-outputs-and-scopes). Tracker-shaped work (SetStatus,
// ReadSections, Classify, Subtypes, FindIssue) routes through the Tracker
// interface; everything else is implemented directly in this file using
// the same shell-out + dependency-injection pattern (Deps with Gh / Git /
// Shell / Prompter / Stdout, all defaulting to real implementations when
// nil).
//
// Every action returns `statemachine.Outcome` with Err set on hard
// failures. User-driven aborts also surface as Err so the engine halts
// the run — silent decline would route past a gate the user explicitly
// closed.
package actions

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/intake"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	trackergithub "github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/github"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Deps bundles the side-effecting collaborators every action may need. All
// fields are optional; a zero-value Deps falls back to real shell-outs and
// the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh         GhRunner
	Git        GitRunner
	Shell      ShellRunner // for the BPMN Phase D `run-command` primitive
	Prompter   Prompter
	Stdout     io.Writer
	Stderr     io.Writer
	ProjectURL string // optional — explicit override for tracker operations
	RepoPath   string // optional — defaults to current working directory
	// Config is the already-loaded gh-optivem.yaml. Threaded in by the
	// driver so scope-checking actions (check-phase-scope,
	// validate-outputs-and-scopes) read the same file the operator passed
	// via --config / $GH_OPTIVEM_CONFIG, not a hard-coded
	// <repoPath>/gh-optivem.yaml. nil is treated as a wiring bug — the
	// affected actions surface a hard error.
	Config *projectconfig.Config
	// Tracker is the seam tracker-shaped actions (SetStatus, ReadSections,
	// Classify, Subtypes, FindIssue) route through. Optional — withDefaults
	// constructs a github adapter from ProjectURL + Gh when unset. Tests
	// inject fakes either by setting ProjectURL + a fake Gh (the
	// constructed github tracker then routes through the fake), or by
	// setting Tracker directly for full control.
	Tracker tracker.Tracker
	// Engine is the loaded process-flow state machine. Scope-checking
	// actions (validate-outputs-and-scopes, check-phase-scope) call
	// Engine.Scope(processName) to resolve per-phase read / write lists
	// from the inline node scope in process-flow.yaml. nil is a wiring
	// bug — the affected actions surface a hard error.
	Engine *statemachine.Engine
	// Autonomous mirrors driver.Opts.Autonomous: when true, actions that
	// would prompt the operator instead emit a warning and proceed. No
	// surviving action currently reads this field; it is retained on
	// Deps so the driver can keep wiring it without a signature break
	// while the new agent-driven prompts are bound in.
	Autonomous bool
}

// Prompter is the same interface gates uses; redefined here so the actions
// package does not import gates (each registry stays self-contained).
type Prompter interface {
	Ask(prompt string) (string, error)
}

// GhRunner runs the `gh` CLI.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ShellRunner runs an arbitrary command. We use a bash-style CommandLine
// (no argv split) so the BPMN Phase D `run-command` primitive can pass
// any templated command line verbatim and tests can match against "the
// exact string you would type at a prompt".
//
// The returned ShellResult carries stdout, stderr, and the exit code so
// `runCommand` can surface a diagnostic payload (failure-kind +
// command-line + command-exit-code + command-stderr-tail) into ctx.State
// when the command fails, which the downstream `fix-command-failed`
// dispatch consumes via its prompt placeholders. Stderr is also embedded
// in the returned error for human-readable surfacing.
type ShellRunner interface {
	Run(ctx context.Context, commandLine string) (ShellResult, error)
}

// ShellResult is the rich return of a shell dispatch. Stdout / Stderr
// are populated for every run (success or failure); ExitCode is 0 on
// success and the OS-reported exit status on failure (or -1 when the
// process never started, e.g. command not found — Go's
// `*exec.ExitError` returns -1 in that case via ExitCode()).
type ShellResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

func (d Deps) withDefaults() Deps {
	if d.Gh == nil {
		d.Gh = realGh{}
	}
	if d.Git == nil {
		d.Git = realGit{}
	}
	if d.Shell == nil {
		d.Shell = realShell{}
	}
	if d.Prompter == nil {
		d.Prompter = stdinPrompter{}
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	if d.Tracker == nil {
		// Default to a github adapter wrapping the (possibly fake) Gh
		// runner. Production callers set ProjectURL from gh-optivem.yaml
		// (driver.go) so SetStatus / Verify / FindIssue resolve against a
		// real project; tests that don't exercise project ops can omit
		// ProjectURL and the placeholder below keeps github.New from
		// rejecting the call — issue-body ops (ReadSections / Classify)
		// only need Issue.URL anyway.
		url := d.ProjectURL
		if url == "" {
			url = "https://github.com/orgs/placeholder/projects/0"
		}
		if t, err := trackergithub.New(url, ghAdapter{d.Gh}); err == nil {
			d.Tracker = t
		}
	}
	return d
}

// RegisterAll wires every YAML action name to its implementation.
func RegisterAll(r *Registry, deps Deps) {
	deps = deps.withDefaults()
	a := actions{deps: deps}
	// Phase-scope enforcement Layer 2 (per plan 20260518-1144 item 5,
	// retargeted at process-flow.yaml node scope per plan 20260526-1536):
	// runs after the agent commits, diffs the working tree against the
	// writing-agent MID's allowed paths joined from
	// process-flow.yaml's inline node `write:` list + gh-optivem.yaml
	// paths:, and writes phase-scope-clean + phase_scope_violating_paths
	// to context. The downstream phase-scope-clean gate consumes the
	// boolean.
	r.Register("check-phase-scope", a.checkPhaseScope)
	// BPMN Phase D — LOW execute-command primitive. Reads ctx.Params["command"]
	// (the templated bash line, post-ExpandParams), appends --suite= and
	// --test= flags only when ctx.Params["suite"] / ctx.Params["test-names"]
	// are non-empty, shells out, and writes ctx.State["command-succeeded"].
	// For the `gh optivem test run` family it also stamps
	// ctx.State["test-outcome"] (pass|fail) so the verify-tests-pass /
	// verify-tests-fail gateways can route without a second shell-out.
	r.Register("run-command", a.runCommand)
	// BPMN Phase D — LOW execute-agent primitive's post-RUN_AGENT
	// validation step. Reads the per-dispatch JSONL outputs file (path
	// stashed at ctx.State["output_file_path"]; agent appends via `gh
	// optivem output write KEY=VAL`) and looks up the writing-agent
	// MID's declared OutputSpec list + write scope via Engine.Outputs /
	// Engine.Scope(task-name) (with originating-task-name fallback for
	// fix dispatches). Writes ctx.State["outputs-and-scopes-valid"] +
	// ctx.State["failure-kind"] (missing-output|scope-diff).
	r.Register("validate-outputs-and-scopes", a.validateOutputsAndScopes)
	// BPMN Phase D — LOW execute-agent primitive's pre-RUN_AGENT
	// baseline-capture step (per plan 20260526-1430). Snapshots the
	// dirty working tree into ctx.State[CtxKeyPreAgentFingerprint] so
	// the post-RUN_AGENT validate-outputs-and-scopes can diff against a
	// per-phase baseline instead of HEAD, eliminating cross-phase
	// false positives when several phases run back-to-back without an
	// intermediate commit. Action is wired into process-flow.yaml's
	// execute-agent subprocess (Item 4 in the same plan); the
	// dormant standalone action is registered here regardless so tests
	// and re-wirings find it.
	r.Register("snapshot-working-tree", a.snapshotWorkingTree)
	// MARK_* state-transition service tasks (per plan
	// 20260526-1220-fix-mark-ticket-state-transition-routing.md). Each
	// dispatches Tracker.SetStatus against the ticketing-system column the
	// canonical state maps to. Tracker.SetStatus is stringly-typed for
	// now; a typed state enum is separate future work.
	r.Register("move-to-in-refinement", a.moveToInRefinement)
	r.Register("move-to-ready", a.moveToReady)
	r.Register("move-to-in-progress", a.moveToInProgress)
	r.Register("move-to-in-acceptance", a.moveToInAcceptance)
	// PARSE_TICKET service task. Calls Tracker.ReadSections against
	// intake.CanonicalHeadings, runs intake.ParseSections (shape-level
	// validation — AC XOR Checklist), and stashes each section body into
	// ctx.State for downstream prompt substitution. Per-kind required-
	// section enforcement happens at dispatch time via the load-bearing
	// placeholder check in clauderun.go.
	r.Register("parse-ticket", a.parseTicket)
}

// Context keys consumed by the check-phase-scope action. Centralised so the
// downstream phase-scope-clean gate and the STOP_SCOPE_VIOLATION user-task
// reference one canonical declaration.
const (
	// CtxKeyPhaseScopeClean is the bool check-phase-scope writes to record
	// whether every modified path in the phase fell within the allowed-paths
	// join (process-flow.yaml MID `write:` scope ∘ gh-optivem.yaml paths:).
	// Read by the phase-scope-clean gate.
	CtxKeyPhaseScopeClean = "phase_scope_clean"

	// CtxKeyPhaseScopeViolatingPaths is the []string of modified paths
	// check-phase-scope found outside scope. Populated only on violations;
	// consumed by the STOP_SCOPE_VIOLATION user-task to render the
	// human-review payload.
	CtxKeyPhaseScopeViolatingPaths = "phase_scope_violating_paths"

	// CtxKeyPreAgentFingerprint is the snapshot of the working tree
	// captured by the snapshot-working-tree action immediately before
	// RUN_AGENT. It is the per-phase baseline downstream scope-checking
	// actions (validate-outputs-and-scopes, check-phase-scope) diff
	// against — replaces the previous HEAD-relative baseline, which
	// attributed upstream phases' uncommitted edits to whichever phase
	// happened to be running. Value type: WorkingTreeFingerprint.
	CtxKeyPreAgentFingerprint = "pre-agent-fingerprint"
)

// WorkingTreeFingerprint is a snapshot of dirty working-tree files
// captured immediately before an agent runs. Keys are repo-relative
// paths (the same paths `git status --porcelain` reports); values are
// hex-encoded SHA-256 hashes of the file bytes on disk at snapshot
// time, or "" for paths the snapshotter saw in `git status` but could
// not read (deleted between enumeration and read — equivalent to a
// post-snapshot delete).
//
// Clean tracked files are intentionally absent: a file clean at
// snapshot time and dirty afterwards appears in the post-state
// `git status` and is added to the delta as "absent in snapshot,
// present now".
type WorkingTreeFingerprint map[string]string

type actions struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// State-transition actions (MARK_* service tasks)
// ---------------------------------------------------------------------------

// moveToInRefinement flips the picked issue's status to "In refinement"
// via Tracker.SetStatus. Wired to the MARK_IN_REFINEMENT node at the
// start of refine-ticket.
func (a actions) moveToInRefinement(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue_handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-refinement: issue_handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In refinement"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-refinement: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In refinement.")
	return statemachine.Outcome{}
}

// moveToReady flips the picked issue's status to "Ready" via
// Tracker.SetStatus. Wired to the MARK_READY node at the end of
// refine-ticket.
func (a actions) moveToReady(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue_handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-ready: issue_handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "Ready"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-ready: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to Ready.")
	return statemachine.Outcome{}
}

// moveToInProgress sets the picked issue's status to "In progress" via
// Tracker.SetStatus. Reads issue_handle from Context — populated by the
// driver's issue-lookup path (preResolveIssue).
func (a actions) moveToInProgress(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue_handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-progress: issue_handle not in Context (requires explicit pre-resolution)")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In progress"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-progress: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In progress.")
	return statemachine.Outcome{}
}

// moveToInAcceptance sets the item status to "In acceptance" via
// Tracker.SetStatus. Errors out hard on failure — a missing Status
// option or a permission failure on edit is a misconfiguration the
// operator must fix before re-running.
func (a actions) moveToInAcceptance(ctx *statemachine.Context) statemachine.Outcome {
	handle := ctx.GetString("issue_handle")
	if handle == "" {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: issue_handle not in Context")}
	}
	if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In acceptance"); err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: %w", err)}
	}
	fmt.Fprintln(a.deps.Stdout, "Moved card to In acceptance.")
	return statemachine.Outcome{}
}

// parseTicket runs the deterministic markdown parser against the picked
// issue's body and stashes the canonical sections into Context state
// under four kebab-cased keys (description, acceptance-criteria,
// steps-to-reproduce, checklist). YAML placeholders consume these
// directly via ExpandParams's state-fallback path; agent prompt bodies
// consume them via the renderer's struct→params translation in
// clauderun.go, where the field→placeholder mapping currently still
// uses snake_case (${acceptance_criteria}, ${checklist}) pending the
// Path B kebab migration.
//
// Wired in implement-ticket between MARK_IN_PROGRESS and GATE_TICKET_KIND
// — runs once per ticket, before the gateway routes to the cycle. The
// parser is ticket-kind-agnostic (Decision 2 in plan
// 20260526-1300): it does shape-level validation only (AC XOR Checklist)
// and lets the load-bearing placeholder check in clauderun.go enforce
// per-kind required sections at dispatch time. That lets one PARSE_TICKET
// node serve all six branches off GATE_TICKET_KIND.
func (a actions) parseTicket(ctx *statemachine.Context) statemachine.Outcome {
	issue, err := issueFromContext(ctx)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse-ticket: %w", err)}
	}
	sections, err := a.deps.Tracker.ReadSections(context.Background(), issue, intake.CanonicalHeadings)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse-ticket: read sections: %w", err)}
	}
	r, err := intake.ParseSections(sections)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("parse-ticket: %w", err)}
	}
	ctx.Set("description", r.Description.Body)
	ctx.Set("acceptance-criteria", r.AcceptanceCriteria.Body)
	ctx.Set("steps-to-reproduce", r.StepsToReproduce.Body)
	ctx.Set("checklist", r.Checklist.Body)
	return statemachine.Outcome{}
}

// issueFromContext builds a tracker.Issue from the conventional Context
// keys preResolveIssue writes (issue_num, issue_url, issue_title,
// issue_handle). issue_url is the addressable form every Tracker call
// site needs; callers that don't seed it get a clear error rather than
// a downstream parse failure.
func issueFromContext(ctx *statemachine.Context) (tracker.Issue, error) {
	url := ctx.GetString("issue_url")
	if url == "" {
		return tracker.Issue{}, fmt.Errorf("issue_url not in Context")
	}
	return tracker.Issue{
		ID:     ctx.GetString("issue_num"),
		Title:  ctx.GetString("issue_title"),
		URL:    url,
		Handle: ctx.GetString("issue_handle"),
	}, nil
}

// ---------------------------------------------------------------------------
// Shell dispatch
// ---------------------------------------------------------------------------

// runShell prints the about-to-run command as a "$ <cmd>" banner so the
// operator can see which gh-optivem invocation the orchestrator is firing,
// then dispatches it. Used by the BPMN Phase D `run-command` primitive
// below; centralises the banner+run pair so any future shell-out action
// inherits the same trace shape.
func (a actions) runShell(cmdLine string) (ShellResult, error) {
	fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmdLine)
	return a.deps.Shell.Run(context.Background(), cmdLine)
}

// ---------------------------------------------------------------------------
// Phase-scope enforcement Layer 2 (post-phase scripted check)
// ---------------------------------------------------------------------------

// checkPhaseScope is Layer 2 of phase-scope enforcement (plan
// 20260518-1144 item 5, retargeted at process-flow.yaml node scope per
// plan 20260526-1536). After the agent commits, the action joins:
//
//   - process-flow.yaml's inline per-node scope (the writing-agent MID's
//     EXECUTE_AGENT carries `read:` / `write:` lists; this action checks
//     the write list — paths the agent may modify).
//   - the project's gh-optivem.yaml paths: (layer name → resolved path)
//     plus Family A path-shaped keys in FamilyAPathKeysInScope (system-path
//     today).
//
// It then enumerates the working-tree changes this phase produced and
// checks each modified path against the allowed set with directory-aware
// prefix matching: diffPath ∈ scope iff
// diffPath == P || diffPath.startsWith(P + "/").
//
// Baseline: prefers the per-phase snapshot stashed in
// ctx.State[CtxKeyPreAgentFingerprint] by an upstream snapshot-working-tree
// step (the same baseline validate-outputs-and-scopes uses), so an
// upstream phase's uncommitted edits are not re-attributed to whichever
// phase is currently running. When the snapshot is absent — check-phase-scope
// is dormant in process-flow.yaml today and may be re-wired in a context
// where no per-phase snapshot exists — the action falls back to the full
// dirty-tree set (`git status --porcelain`) and emits a debug line via
// a.deps.Stderr so the re-wiring is loud.
//
// Phase id source: ctx.Params["phase_id"] — the writing-agent MID's name
// (e.g. "write-acceptance-tests"). Resolved via Engine.Scope.
//
// Writes:
//   - CtxKeyPhaseScopeClean (bool)            — false on violation
//   - CtxKeyPhaseScopeViolatingPaths ([]string) — populated on violation
//
// The phase_scope_clean gate reads the boolean; the STOP_SCOPE_VIOLATION
// user-task reads the slice to render the human-review payload.
func (a actions) checkPhaseScope(ctx *statemachine.Context) statemachine.Outcome {
	phaseID := ctx.Params["phase_id"]
	if phaseID == "" {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: phase_id not set in Params — the call-activity invoking the writing-agent MID must pass phase_id")}
	}

	if a.deps.Engine == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: engine not loaded — driver must inject actions.Deps.Engine")}
	}
	_, write, ok := a.deps.Engine.Scope(phaseID)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf(
			"check_phase_scope: phase id %q not a writing-agent MID in process-flow.yaml — add inline read: / write: scope to its EXECUTE_AGENT node", phaseID)}
	}

	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}

	allowed, err := resolveLayerPaths(write, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope (%s): %w", phaseID, err)}
	}

	var modified []string
	if snapshot, ok := ctx.State[CtxKeyPreAgentFingerprint].(WorkingTreeFingerprint); ok {
		modified, err = a.modifiedPathsSinceFingerprint(context.Background(), snapshot)
	} else {
		// HEAD-equivalent fallback: this action is dormant in
		// process-flow.yaml today and may be re-wired in a context
		// without an upstream snapshot. Log loudly so the operator
		// notices, then enumerate the full dirty tree.
		fmt.Fprintln(a.deps.Stderr,
			"check_phase_scope: no pre-agent-fingerprint in state — falling back to current dirty tree; re-wire with snapshot-working-tree upstream for per-phase semantics")
		modified, err = a.dirtyTreePaths(context.Background())
	}
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: %w", err)}
	}

	var violating []string
	for _, m := range modified {
		if !pathInScope(m, allowed) {
			violating = append(violating, m)
		}
	}
	if len(violating) > 0 {
		ctx.Set(CtxKeyPhaseScopeClean, false)
		ctx.State[CtxKeyPhaseScopeViolatingPaths] = violating
		fmt.Fprintf(a.deps.Stderr,
			"check_phase_scope: %s scope violation — %d path(s) outside scope.\nResolve by: (1) accept the diff if intentional; (2) rewind to an upstream phase; (3) revert and rerun; (4) abort the cycle.\n",
			phaseID, len(violating))
		for _, v := range violating {
			fmt.Fprintf(a.deps.Stderr, "  out-of-scope: %s\n", v)
		}
		return statemachine.Outcome{Bool: false}
	}
	ctx.Set(CtxKeyPhaseScopeClean, true)
	return statemachine.Outcome{Bool: true}
}

// resolveLayerPaths joins a phase's layer list against the project's
// configured paths: Family A path-shaped keys go through their dedicated
// Config accessor (system-path → cfg.System.Path); everything else is a
// Family B key in cfg.SystemTest.Paths. Missing values surface as errors
// rather than silently shrinking the allowed set.
func resolveLayerPaths(layers []string, cfg *projectconfig.Config) ([]string, error) {
	out := make([]string, 0, len(layers))
	for _, layer := range layers {
		if atdd.FamilyAPathKeysInScope[layer] {
			switch layer {
			case "system-path":
				if cfg.System.Path == "" {
					return nil, fmt.Errorf("layer %q resolves to empty system.path in gh-optivem.yaml", layer)
				}
				out = append(out, cfg.System.Path)
			default:
				return nil, fmt.Errorf("layer %q is in FamilyAPathKeysInScope but has no Config accessor", layer)
			}
			continue
		}
		v, ok := cfg.SystemTest.Paths[layer]
		if !ok || v == "" {
			return nil, fmt.Errorf("layer %q not present in gh-optivem.yaml system-test.paths:", layer)
		}
		out = append(out, v)
	}
	return out, nil
}

// pathInScope returns true if diffPath is within any allowed path P
// with directory-aware prefix matching: diffPath == P, or diffPath
// starts with P + "/". Raw HasPrefix(P) is wrong — it would let
// ".../shop2/..." match ".../shop". This contract is shared with the
// `gh optivem process scope` CLI projection.
func pathInScope(diffPath string, allowed []string) bool {
	for _, p := range allowed {
		if diffPath == p || strings.HasPrefix(diffPath, p+"/") {
			return true
		}
	}
	return false
}

// dirtyTreePaths enumerates the paths `git status --porcelain` reports
// (tracked-modified + untracked + both endpoints of any rename row),
// sorted and de-duplicated. This is the path set both
// captureWorkingTreeFingerprint and modifiedPathsSinceFingerprint
// iterate over — `git status --porcelain` is the authoritative dirty
// set; clean tracked files are intentionally excluded (a file clean at
// snapshot time and dirty afterwards still surfaces in the post-state
// call).
func (a actions) dirtyTreePaths(ctx context.Context) ([]string, error) {
	gitArgs := func(extra ...string) []string {
		if a.deps.RepoPath == "" {
			return extra
		}
		return append([]string{"-C", a.deps.RepoPath}, extra...)
	}
	status, err := a.deps.Git.Run(ctx, gitArgs("status", "--porcelain")...)
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain: %w", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(status), "\n") {
		// porcelain v1 format: "XY path" or "XY old -> new"; X is the
		// staged status, Y the unstaged. Two status chars + one space
		// before the path.
		if len(line) < 4 {
			continue
		}
		rest := line[3:]
		if i := strings.Index(rest, " -> "); i >= 0 {
			oldPath := strings.TrimSpace(rest[:i])
			newPath := strings.TrimSpace(rest[i+4:])
			if oldPath != "" {
				seen[oldPath] = true
			}
			if newPath != "" {
				seen[newPath] = true
			}
			continue
		}
		path := strings.TrimSpace(rest)
		if path != "" {
			seen[path] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// hashRepoFile returns the hex SHA-256 of <RepoPath>/<rel>. A missing
// or unreadable file returns "" — the delta comparator treats that as
// "absent on disk", which combined with "present in snapshot" surfaces
// as a delta (deleted by the phase).
func (a actions) hashRepoFile(rel string) string {
	full := rel
	if a.deps.RepoPath != "" {
		full = filepath.Join(a.deps.RepoPath, rel)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// captureWorkingTreeFingerprint takes a snapshot of every dirty path
// reported by `git status --porcelain`, hashing the bytes of each file
// via SHA-256. The resulting WorkingTreeFingerprint is the baseline a
// subsequent modifiedPathsSinceFingerprint call diffs against to
// compute *this phase's* edits — independent of upstream phases that
// have also left uncommitted changes in the working tree.
//
// Returns a hard error only when `git status` itself fails (genuine
// wiring problem); per-file read failures degrade gracefully to an
// empty hash entry (see hashRepoFile).
func (a actions) captureWorkingTreeFingerprint(ctx context.Context) (WorkingTreeFingerprint, error) {
	paths, err := a.dirtyTreePaths(ctx)
	if err != nil {
		return nil, err
	}
	fp := make(WorkingTreeFingerprint, len(paths))
	for _, p := range paths {
		fp[p] = a.hashRepoFile(p)
	}
	return fp, nil
}

// modifiedPathsSinceFingerprint returns the paths that changed between
// the supplied snapshot and the current working tree:
//
//   - present in snapshot, hash differs on disk → modified (or
//     deleted, in which case the on-disk hash is "")
//   - absent in snapshot, present in current `git status` → added by
//     this phase
//   - present in both with matching hashes → untouched (upstream-phase
//     residue, correctly excluded)
//
// Returns a sorted, de-duplicated slice — the same shape
// validateOutputsAndScopes and checkPhaseScope iterate over.
func (a actions) modifiedPathsSinceFingerprint(ctx context.Context, base WorkingTreeFingerprint) ([]string, error) {
	nowPaths, err := a.dirtyTreePaths(ctx)
	if err != nil {
		return nil, err
	}
	delta := map[string]bool{}
	for p, baseHash := range base {
		if a.hashRepoFile(p) != baseHash {
			delta[p] = true
		}
	}
	for _, p := range nowPaths {
		if _, inBase := base[p]; !inBase {
			delta[p] = true
		}
	}
	out := make([]string, 0, len(delta))
	for p := range delta {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// ---------------------------------------------------------------------------
// Shell-escape helper (used by the BPMN Phase D `run-command` primitive)
// ---------------------------------------------------------------------------

// shellEscape quotes a value for safe insertion into a bash command line.
// We use single quotes so `$`, backticks, and other meta-characters are
// taken literally; embedded single quotes are split-and-rejoined.
func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t'\"`$\\;|&<>(){}[]?*~#!") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ---------------------------------------------------------------------------
// BPMN Phase D — LOW execute-command + execute-agent primitives
// ---------------------------------------------------------------------------

// runCommand is the body of the LOW `execute-command` primitive (per
// plans/20260525-2348-bpmn-phase-d-bindings.md Item 1, Q-D5). The
// caller's `call-activity.params:` block is expanded against the parent
// scope before dispatch, so by the time the action fires:
//
//   - ctx.Params["command"]     — the fully-resolved bash command line
//                                  (e.g. "gh optivem test run")
//   - ctx.Params["suite"]       — optional; appended as --suite=…
//                                  pins the test category (acceptance,
//                                  contract-real, contract-stub)
//   - ctx.Params["test-names"]  — optional; appended as --test=…
//                                  comma-separated list of bare test
//                                  method names (the writer-agent's
//                                  emitted test-names, joined via
//                                  coerceStateValue's []string case)
//
// Writes ctx.State["command-succeeded"] = (exit == 0). For the
// `gh optivem test run` family it additionally stamps
// ctx.State["test-outcome"] = "pass"|"fail" so the verify-tests-pass /
// verify-tests-fail gateways downstream of run-tests route without a
// second shell-out. On failure it also stamps a diagnostic payload —
// failure-kind = "command-failed", command-line, command-exit-code,
// command-stderr-tail — which the `fix-command-failed` dispatch consumes
// via its prompt placeholders (see clauderun.Options.Command*).
//
// Does NOT surface command failure as Outcome.Err — the
// execute-command primitive's GATE_COMMAND_SUCCEEDED is the intended
// consumer of the false branch (it dispatches `fix` with
// failure-kind = "command-failed"). Empty `command` is a wiring bug, so
// surfaces as Err.
func (a actions) runCommand(ctx *statemachine.Context) statemachine.Outcome {
	cmd := strings.TrimSpace(ctx.Params["command"])
	if cmd == "" {
		return statemachine.Outcome{Err: fmt.Errorf("run-command: command param not set — call-activity must pass `command:`")}
	}
	isTestRun := strings.HasPrefix(cmd, "gh optivem test run")
	if suite := strings.TrimSpace(ctx.Params["suite"]); suite != "" {
		cmd += " --suite=" + shellEscape(suite)
	}
	if testNames := strings.TrimSpace(ctx.Params["test-names"]); testNames != "" {
		cmd += " --test=" + shellEscape(testNames)
	}
	result, err := a.runShell(cmd)
	succeeded := err == nil
	ctx.Set("command-succeeded", succeeded)
	if isTestRun {
		if succeeded {
			ctx.Set("test-outcome", "pass")
		} else {
			ctx.Set("test-outcome", "fail")
		}
	}
	if err != nil {
		ctx.Set("failure-kind", "command-failed")
		ctx.Set("command-line", cmd)
		ctx.Set("command-exit-code", result.ExitCode)
		ctx.Set("command-stderr-tail", lastNLines(string(result.Stderr), commandStderrTailLines))
		fmt.Fprintf(a.deps.Stderr, "run-command: %v\n", err)
	}
	return statemachine.Outcome{}
}

// commandStderrTailLines caps the stderr block stashed in ctx.State for
// the fix-command-failed prompt. 20 lines is enough to carry a typical
// stack trace tail without blowing the prompt size on a runaway log.
const commandStderrTailLines = 20

// lastNLines returns the trailing n non-empty-bounded lines of s, joined
// by "\n". When s has fewer than n lines, returns s with a single
// trailing newline trimmed (so the rendered ${command_stderr_tail}
// block doesn't gain an extra blank line). Used to bound the stderr
// payload fed to the fix-command-failed prompt.
func lastNLines(s string, n int) string {
	if s == "" || n <= 0 {
		return ""
	}
	trimmed := strings.TrimRight(s, "\n")
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= n {
		return trimmed
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// validateOutputsAndScopes is the LOW `execute-agent` primitive's
// post-RUN_AGENT validation step (BPMN Phase D Item 7, Q-D6).
//
// The agent emits structured outputs by invoking `gh optivem output
// write KEY=VAL` from its Bash tool; each call appends one JSON object
// to the per-dispatch JSONL file the driver pre-computes (path stashed
// in ctx.State["output_file_path"] before RUN_AGENT, env-exported as
// GH_OPTIVEM_OUTPUT_FILE). This action:
//
//  1. Walks that JSONL file (when present) applying last-write-wins per
//     key, and flattens the resulting map into ctx.State so downstream
//     actions and gates read the values the same way they always did.
//  2. Looks up the writing-agent MID's declared OutputSpec list via
//     Engine.Outputs(phaseTaskName) and presence-checks every
//     non-Optional key against the flattened state.
//  3. Diffs the working tree against the per-phase baseline stashed at
//     ctx.State[CtxKeyPreAgentFingerprint] by snapshot-working-tree and
//     flags any modified path outside the MID's `write:` scope.
//
// The JSONL channel replaces the older prose-YAML tail (parsed by the
// deleted clauderun.ParseOutputs). The new channel works uniformly in
// both interactive and autonomous modes — interactive mode used to
// silently fail every outputs validation because RunResult.ResultText
// was empty (claude-CLI prints to TTY, not envelope).
//
// Reads:
//   - ctx.State["output_file_path"]      — absolute path to the per-dispatch
//                                          JSONL file. Set by the driver
//                                          ONLY when the MID's BPMN
//                                          `outputs:` list is non-empty;
//                                          absent → no-op output read.
//   - ctx.Params["originating-task-name"] (preferred) or
//     ctx.Params["task-name"]            — the writing-agent MID name used
//                                          to look up scope (Engine.Scope)
//                                          AND outputs (Engine.Outputs).
//                                          The originating- prefix is set
//                                          by the `fix` LOW so the inner
//                                          execute-agent validation can
//                                          recover the outer MID's scope +
//                                          outputs after task-name is
//                                          shadowed to fix-${failure-kind}.
//   - ctx.State[CtxKeyPreAgentFingerprint] — WorkingTreeFingerprint
//                                          captured by snapshot-working-tree.
//                                          Required when the dispatching node
//                                          has a write scope; missing key is
//                                          a wiring bug and surfaces as
//                                          Outcome.Err.
//
// Writes:
//   - ctx.State["outputs-and-scopes-valid"]   — bool.
//   - ctx.State["failure-kind"]               — set on false; one of
//                                               missing-output | scope-diff
//                                               (priority: missing-output
//                                               wins when both fail).
//   - ctx.State["failing-task-name"]          — set on false; the OUTER
//                                               execute-agent's task-name
//                                               (e.g. "write-acceptance-tests")
//                                               captured from ctx.Params
//                                               before the inner `fix`
//                                               call-activity shadows it.
//                                               Consumed by the
//                                               fix-missing-output /
//                                               fix-scope-diff prompts.
//   - ctx.State["missing-outputs"]            — set on missing-output;
//                                               comma-separated list of
//                                               unemitted output keys.
//   - ctx.State["scope-violating-paths"]      — set on scope-diff;
//                                               comma-separated list of
//                                               working-tree paths
//                                               outside the declared
//                                               write scope.
//   - ctx.State["phase-changed-files"]        — set whenever the scope
//                                               check ran (success or
//                                               failure); newline-joined
//                                               sorted list of every path
//                                               in the snapshot delta
//                                               (in-scope + out-of-scope).
//                                               The fix-scope-diff prompt
//                                               reads this as ${changed_files}
//                                               so the diagnosing agent sees
//                                               only this phase's edits,
//                                               not the full git status dump.
//
// Does NOT surface as Outcome.Err — the gateway's false branch
// dispatches `fix-${failure-kind}` per Q-late-5. Hard errors
// (gh-optivem.yaml missing, git unusable, engine not wired, snapshot
// key missing) DO surface as Err since they indicate a wiring/infra
// problem, not an
// agent-output problem.
func (a actions) validateOutputsAndScopes(ctx *statemachine.Context) statemachine.Outcome {
	if a.deps.Engine == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: engine not loaded — driver must inject actions.Deps.Engine")}
	}

	// 1. Read the per-dispatch JSONL outputs file (when the driver
	// stashed a path), apply last-write-wins per key, coerce values per
	// the BPMN-declared types, and flatten into ctx.State. The driver
	// only stashes output_file_path when the MID declared outputs, so
	// an absent stash skips this block entirely.
	taskName := phaseTaskName(ctx)
	declared, _ := a.deps.Engine.Outputs(taskName)
	outputFile, _ := ctx.State["output_file_path"].(string)
	if outputFile != "" {
		flattened, err := readOutputsJSONL(outputFile, declared)
		if err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
		}
		for k, v := range flattened {
			ctx.Set(k, v)
		}
	}

	// 2. Output presence check — every non-Optional declared key must
	// be present in ctx.State after the flatten.
	var missing []string
	for _, spec := range declared {
		if spec.Optional {
			continue
		}
		if _, ok := ctx.State[spec.Key]; !ok {
			missing = append(missing, spec.Key)
		}
	}
	if len(missing) > 0 {
		ctx.Set("outputs-and-scopes-valid", false)
		ctx.Set("failure-kind", "missing-output")
		ctx.Set("failing-task-name", taskName)
		ctx.Set("missing-outputs", strings.Join(missing, ","))
		fmt.Fprintf(a.deps.Stderr,
			"validate-outputs-and-scopes: agent did not emit expected outputs: %s\n",
			strings.Join(missing, ", "))
		return statemachine.Outcome{}
	}

	// 3. Scope check (no-op when the dispatching node is not a
	// writing-agent MID — refine-acceptance-criteria declares
	// scope: none in prompt frontmatter and has no inline read/write,
	// so Engine.Scope returns ok=false and the scope check is skipped).
	_, write, ok := a.deps.Engine.Scope(taskName)
	if !ok {
		ctx.Set("outputs-and-scopes-valid", true)
		return statemachine.Outcome{}
	}

	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}
	allowed, err := resolveLayerPaths(write, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	snapshot, ok := ctx.State[CtxKeyPreAgentFingerprint].(WorkingTreeFingerprint)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: pre-agent-fingerprint not set — execute-agent must run snapshot-working-tree before RUN_AGENT")}
	}
	modified, err := a.modifiedPathsSinceFingerprint(context.Background(), snapshot)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	// Stash the full snapshot delta (in-scope + out-of-scope) so the
	// fix-scope-diff prompt's ${changed_files} renders this phase's
	// edits only — not the cross-phase `git status --porcelain` dump
	// the dispatcher would otherwise capture.
	ctx.Set("phase-changed-files", strings.Join(modified, "\n"))

	var violating []string
	for _, m := range modified {
		if !pathInScope(m, allowed) {
			violating = append(violating, m)
		}
	}
	if len(violating) > 0 {
		ctx.Set("outputs-and-scopes-valid", false)
		ctx.Set("failure-kind", "scope-diff")
		ctx.Set("failing-task-name", taskName)
		ctx.Set("scope-violating-paths", strings.Join(violating, ","))
		fmt.Fprintf(a.deps.Stderr,
			"validate-outputs-and-scopes: %d path(s) outside scope %v:\n",
			len(violating), write)
		for _, v := range violating {
			fmt.Fprintf(a.deps.Stderr, "  out-of-scope: %s\n", v)
		}
		return statemachine.Outcome{}
	}

	ctx.Set("outputs-and-scopes-valid", true)
	return statemachine.Outcome{}
}

// phaseTaskName returns the writing-agent MID name to look up scope by.
// Prefers ctx.Params["originating-task-name"] (set by the `fix` LOW so
// fix dispatches retain the outer MID's scope identity) and falls back
// to ctx.Params["task-name"] for the normal path.
func phaseTaskName(ctx *statemachine.Context) string {
	if orig := ctx.Params["originating-task-name"]; orig != "" {
		return orig
	}
	return ctx.Params["task-name"]
}

// snapshotWorkingTree is the body of the BPMN Phase D
// execute-agent.SNAPSHOT_WORKING_TREE service task. It captures a
// WorkingTreeFingerprint of every dirty path and stashes it in
// ctx.State[CtxKeyPreAgentFingerprint] for the post-RUN_AGENT
// validate-outputs-and-scopes step to diff against.
//
// Failure to enumerate the dirty set (e.g. `git` not on PATH, repo
// path invalid) is a wiring problem, not an agent-output problem, so
// it surfaces as Outcome.Err — same shape as
// validate-outputs-and-scopes' hard-error path.
func (a actions) snapshotWorkingTree(ctx *statemachine.Context) statemachine.Outcome {
	fp, err := a.captureWorkingTreeFingerprint(context.Background())
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("snapshot-working-tree: %w", err)}
	}
	ctx.State[CtxKeyPreAgentFingerprint] = fp
	return statemachine.Outcome{}
}

// readOutputsJSONL reads the per-dispatch JSONL outputs file the agent
// appended to via `gh optivem output write`, applies last-write-wins per
// key across lines, and coerces each value to the Go shape declared in
// the BPMN OutputSpec list (bool → bool, string → string, string-list →
// []string). Returns an empty map when the file is missing — the
// dispatcher stashed a path but the agent emitted no writes (or the run
// terminated early). Tolerates blank/whitespace lines but treats any
// malformed JSON line as a hard error so the cycle stops with a clear
// "agent emitted malformed output line" message rather than silently
// dropping the agent's intent.
//
// Unknown keys (not in declared) are tolerated and pass through
// unchanged — the CLI side already rejects them at write-time, so a key
// reaching this reader past the allow-list is either a test fixture or
// a deliberately-permissive caller. We don't double-enforce.
func readOutputsJSONL(path string, declared []statemachine.OutputSpec) (map[string]any, error) {
	out := map[string]any{}
	typeByKey := make(map[string]string, len(declared))
	for _, o := range declared {
		typeByKey[o.Key] = o.Type
	}

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("open outputs file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("agent emitted malformed output line: %q: %w", string(line), err)
		}
		if row == nil {
			return nil, fmt.Errorf("agent emitted malformed output line: %q (not a JSON object)", string(line))
		}
		for k, v := range row {
			coerced, err := coerceJSONOutputValue(k, v, typeByKey[k])
			if err != nil {
				return nil, err
			}
			out[k] = coerced
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read outputs file: %w", err)
	}
	return out, nil
}

// coerceJSONOutputValue normalises a value decoded from one JSONL line to
// the Go shape downstream consumers expect. The `output write` CLI
// already encodes values per the declared type, so the input is
// already-shaped JSON (bool / string / []string-as-JSON-array). The job
// here is to flatten the `[]any` that json.Unmarshal returns for arrays
// into a `[]string` so the existing []string-typed readers (e.g. the
// scope_exception_requested gate, the runCommand --test=… joiner) keep
// working unchanged.
//
// An undeclared key (declaredType == "") passes through untouched; the
// CLI rejects unknown keys at write time, so this branch is just
// defensive for test fixtures that hand-craft JSONL outside the CLI.
func coerceJSONOutputValue(key string, v any, declaredType string) (any, error) {
	switch declaredType {
	case statemachine.OutputTypeStringList:
		raw, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("output key %q: declared string-list but emitted %T", key, v)
		}
		out := make([]string, 0, len(raw))
		for i, e := range raw {
			s, ok := e.(string)
			if !ok {
				return nil, fmt.Errorf("output key %q: string-list element [%d] is %T, want string", key, i, e)
			}
			out = append(out, s)
		}
		return out, nil
	case statemachine.OutputTypeBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("output key %q: declared bool but emitted %T", key, v)
		}
		return b, nil
	case statemachine.OutputTypeString:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("output key %q: declared string but emitted %T", key, v)
		}
		return s, nil
	default:
		// Undeclared key — pass through as-is.
		return v, nil
	}
}

// ---------------------------------------------------------------------------
// Adapter shims (different runner interfaces across packages must not leak)
// ---------------------------------------------------------------------------

// ghAdapter exists because each underlying package (tracker) defines its
// own GhRunner interface — Go's structural typing means we can wrap once
// instead of teaching every package to depend on a shared runner type.
// The wrapper is zero-cost.
type ghAdapter struct{ inner GhRunner }

func (g ghAdapter) Run(ctx context.Context, args ...string) ([]byte, error) {
	return g.inner.Run(ctx, args...)
}

// ---------------------------------------------------------------------------
// Default exec runners + stdin prompter
// ---------------------------------------------------------------------------

type realGh struct{}

func (realGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("gh %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

type realGit struct{}

func (realGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
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

type realShell struct{}

func (realShell) Run(ctx context.Context, commandLine string) (ShellResult, error) {
	// We deliberately route through the user's shell so command lines like
	// `./test-all.sh --sample` and `bash -lc compile-all.sh` work uniformly.
	// On Windows, gh-optivem ships against bash via the Git Bash shim; if
	// that is missing the user gets a clear "executable file not found"
	// from os/exec.
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}
	cmd := exec.CommandContext(ctx, shell, "-c", commandLine)
	// Tee the child's stdio: stream live to the operator's terminal so
	// long-running commands (docker compose build, gradle, etc.) show
	// progress instead of looking hung, and capture into buffers so the
	// returned ShellResult still carries stdout for callers that parse it
	// (e.g. `gh optivem test run --list`) and stderr is still inlined
	// into the error message on failure.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Run()
	result := ShellResult{Stdout: stdoutBuf.Bytes(), Stderr: stderrBuf.Bytes()}
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			result.ExitCode = ee.ExitCode()
			return result, fmt.Errorf("shell %q: %w (stderr: %s)",
				commandLine, err, strings.TrimSpace(stderrBuf.String()))
		}
		// Process never started (binary not found, etc.) — exec.ExitError
		// is not in the chain, so we leave ExitCode at its zero value;
		// callers that surface command-exit-code into state still get a
		// stable int, just one that signals "no exit code observed".
		return result, fmt.Errorf("shell %q: %w", commandLine, err)
	}
	return result, nil
}

type stdinPrompter struct{}

func (stdinPrompter) Ask(prompt string) (string, error) {
	if _, err := fmt.Fprint(os.Stderr, prompt); err != nil {
		return "", err
	}
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}
