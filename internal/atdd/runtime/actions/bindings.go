// Bindings — Go implementations of every service-task `action:` referenced
// in internal/atdd/runtime/statemachine/process-flow.yaml.
//
// Actions are the mechanical work of the pipeline: pick the top Ready
// ticket from the project board, enforce phase scope after the agent
// commits, and dispatch the BPMN Phase D LOW primitives (run-command,
// validate-outputs-and-scopes). Tracker-shaped work (PickReady) routes
// through the Tracker interface; everything else is implemented directly
// in this file using the same shell-out + dependency-injection pattern
// (Deps with Gh / Git / Shell / Prompter / Stdout, all defaulting to real
// implementations when nil).
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
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd"
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
	// Tracker is the seam pickTopReady's PickReady call goes through.
	// Optional — withDefaults constructs a github adapter from
	// ProjectURL + Gh when unset. Tests inject fakes either by setting
	// ProjectURL + a fake Gh (the constructed github tracker then routes
	// through the fake), or by setting Tracker directly for full control.
	Tracker tracker.Tracker
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
type ShellRunner interface {
	Run(ctx context.Context, commandLine string) ([]byte, error)
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
		// (driver.go) so PickReady / SetStatus / Verify resolve against a
		// real project; tests that don't exercise project ops can omit
		// ProjectURL and the placeholder below keeps github.New from
		// rejecting the call — issue-body ops (ReadSections /
		// MarkChecklistComplete / Classify) only need Issue.URL anyway.
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
	r.Register("pick-top-ready", a.pickTopReady)
	// Phase-scope enforcement Layer 2 (per plan 20260518-1144 item 5): runs
	// after the agent commits, diffs the working tree against the phase's
	// allowed paths joined from internal/atdd/phase-scopes.yaml +
	// gh-optivem.yaml paths:, and writes phase-scope-clean +
	// phase_scope_violating_paths to context. The downstream
	// phase-scope-clean gate consumes the boolean.
	r.Register("check-phase-scope", a.checkPhaseScope)
	// BPMN Phase D — LOW execute-command primitive. Reads ctx.Params["command"]
	// (the templated bash line, post-ExpandParams), appends --filter-type=
	// and --filter-value= flags only when those params are non-empty, shells
	// out, and writes ctx.State["command-succeeded"]. For the
	// `gh optivem run-tests` family it also stamps ctx.State["test-outcome"]
	// (pass|fail) so the verify-tests-pass / verify-tests-fail gateways can
	// route without a second shell-out.
	r.Register("run-command", a.runCommand)
	// BPMN Phase D — LOW execute-agent primitive's post-RUN_AGENT
	// validation step. Reads ctx.Params["outputs"] (comma-separated keys
	// the agent's `outputs:` YAML block must populate) + ctx.Params["scopes"]
	// (comma-separated Family B layer tokens defining the allowed
	// working-tree paths), and writes ctx.State["outputs-and-scopes-valid"]
	// + ctx.State["failure-kind"] (missing-output|scope-diff).
	r.Register("validate-outputs-and-scopes", a.validateOutputsAndScopes)
}

// Context keys consumed by the check-phase-scope action. Centralised so the
// downstream phase-scope-clean gate and the STOP_SCOPE_VIOLATION user-task
// reference one canonical declaration.
const (
	// CtxKeyPhaseScopeClean is the bool check-phase-scope writes to record
	// whether every modified path in the phase fell within the allowed-paths
	// join (phase-scopes.yaml ∘ gh-optivem.yaml paths:). Read by the
	// phase-scope-clean gate.
	CtxKeyPhaseScopeClean = "phase_scope_clean"

	// CtxKeyPhaseScopeViolatingPaths is the []string of modified paths
	// check-phase-scope found outside scope. Populated only on violations;
	// consumed by the STOP_SCOPE_VIOLATION user-task to render the
	// human-review payload.
	CtxKeyPhaseScopeViolatingPaths = "phase_scope_violating_paths"
)

type actions struct {
	deps Deps
}

// ---------------------------------------------------------------------------
// Board-backed actions
// ---------------------------------------------------------------------------

// pickTopReady reads the project's Ready column via Tracker.PickReady and
// writes the picked issue's number, URL, title, repo, and opaque handle
// into Context state. Downstream gates and actions read those keys; the
// engine itself does not interpret them.
//
// On an empty Ready column Tracker.PickReady returns tracker.ErrEmptyReady,
// which the action surfaces as Outcome.Err. The driver catches that
// specific sentinel and exits zero rather than crashing — a normal
// "nothing to do" outcome.
func (a actions) pickTopReady(ctx *statemachine.Context) statemachine.Outcome {
	issue, err := a.deps.Tracker.PickReady(context.Background())
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("pick-top-ready: %w", err)}
	}
	writeIssueToContext(ctx, issue)
	fmt.Fprintf(a.deps.Stdout, "Picked top Ready: #%s %s (%s)\n", issue.ID, issue.Title, issue.URL)
	return statemachine.Outcome{}
}

// ---------------------------------------------------------------------------
// Shell dispatch
// ---------------------------------------------------------------------------

// runShell prints the about-to-run command as a "$ <cmd>" banner so the
// operator can see which gh-optivem invocation the orchestrator is firing,
// then dispatches it. Used by the BPMN Phase D `run-command` primitive
// below; centralises the banner+run pair so any future shell-out action
// inherits the same trace shape.
func (a actions) runShell(cmdLine string) ([]byte, error) {
	fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmdLine)
	return a.deps.Shell.Run(context.Background(), cmdLine)
}

// ---------------------------------------------------------------------------
// Phase-scope enforcement Layer 2 (post-phase scripted check)
// ---------------------------------------------------------------------------

// checkPhaseScope is Layer 2 of phase-scope enforcement (plan
// 20260518-1144 item 5). After the agent commits, the action joins:
//
//   - internal/atdd/phase-scopes.yaml (SSoT: BPMN phase id → layer list)
//   - the project's gh-optivem.yaml paths: (layer name → resolved path)
//     plus Family A path-shaped keys in FamilyAPathKeysInScope (system-path
//     today).
//
// It then enumerates the working-tree changes via `git diff --name-only HEAD`
// + `git status --porcelain` (union covers staged, unstaged, and untracked
// paths since the last commit baseline) and checks each modified path
// against the allowed set with directory-aware prefix matching:
// diffPath ∈ scope iff diffPath == P || diffPath.startsWith(P + "/").
//
// Phase id source: the call-activity invoking red_phase_cycle /
// green_phase_cycle passes phase_id: <NODE_ID> in its params; this action
// reads ctx.Params["phase_id"].
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
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: phase_id not set in Params — the call-activity invoking red_phase_cycle / green_phase_cycle must pass phase_id")}
	}

	scopes, err := atdd.LoadPhaseScopes()
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: %w", err)}
	}
	layers, ok := scopes.Phases[phaseID]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf(
			"check_phase_scope: phase id %q not in internal/atdd/phase-scopes.yaml — add an entry", phaseID)}
	}

	cfg, err := projectconfig.Load(a.deps.RepoPath)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: load gh-optivem.yaml: %w", err)}
	}
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope: gh-optivem.yaml not found under %s", a.deps.RepoPath)}
	}

	allowed, err := resolveLayerPaths(layers, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("check_phase_scope (%s): %w", phaseID, err)}
	}

	modified, err := a.modifiedPathsSinceHead(context.Background())
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

// modifiedPathsSinceHead enumerates working-tree paths touched in the
// current phase by unioning `git diff --name-only HEAD` (tracked,
// staged + unstaged) with `git status --porcelain` (also covers untracked
// `??` and rename `R  old -> new` endpoints). Returns a sorted,
// de-duplicated slice so violating-paths reads deterministically.
func (a actions) modifiedPathsSinceHead(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	gitArgs := func(extra ...string) []string {
		if a.deps.RepoPath == "" {
			return extra
		}
		return append([]string{"-C", a.deps.RepoPath}, extra...)
	}

	diff, err := a.deps.Git.Run(ctx, gitArgs("diff", "--name-only", "HEAD")...)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-only HEAD: %w", err)
	}
	for _, line := range strings.Split(string(diff), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			seen[line] = true
		}
	}

	status, err := a.deps.Git.Run(ctx, gitArgs("status", "--porcelain")...)
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain: %w", err)
	}
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
//   - ctx.Params["command"]      — the fully-resolved bash command line
//                                   (e.g. "gh optivem run-tests")
//   - ctx.Params["filter-type"]  — optional; appended as --filter-type=…
//   - ctx.Params["filter-value"] — optional; appended as --filter-value=…
//
// Writes ctx.State["command-succeeded"] = (exit == 0). For the
// `gh optivem run-tests` family it additionally stamps
// ctx.State["test-outcome"] = "pass"|"fail" so the verify-tests-pass /
// verify-tests-fail gateways downstream of run-tests route without a
// second shell-out.
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
	isTestRun := strings.HasPrefix(cmd, "gh optivem run-tests")
	if filterType := strings.TrimSpace(ctx.Params["filter-type"]); filterType != "" {
		cmd += " --filter-type=" + shellEscape(filterType)
	}
	if filterValue := strings.TrimSpace(ctx.Params["filter-value"]); filterValue != "" {
		cmd += " --filter-value=" + shellEscape(filterValue)
	}
	_, err := a.runShell(cmd)
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
		fmt.Fprintf(a.deps.Stderr, "run-command: %v\n", err)
	}
	return statemachine.Outcome{}
}

// validateOutputsAndScopes is the LOW `execute-agent` primitive's
// post-RUN_AGENT validation step (BPMN Phase D Item 7, Q-D6). The
// agent's `outputs:` YAML block has already been flattened into
// ctx.State by clauderun.ParseOutputs (driver.go); this action checks
// (a) every key the caller declared in `outputs:` is present in
// ctx.State, and (b) every working-tree change since HEAD falls
// within at least one of the call-site's declared `scopes:` (joined
// against gh-optivem.yaml paths: via the same resolveLayerPaths the
// checkPhaseScope action uses).
//
// Reads:
//   - ctx.Params["outputs"]  — comma-separated list of expected output
//                              keys; empty → no output expectations.
//   - ctx.Params["scopes"]   — comma-separated Family B scope tokens
//                              (e.g. "at-test,dsl-port,dsl-core"); empty
//                              → skip scope check (update-ticket /
//                              refine-acceptance-criteria do not declare scopes).
//
// Writes:
//   - ctx.State["outputs-and-scopes-valid"] — bool.
//   - ctx.State["failure-kind"]             — set on false; one of
//                                             missing-output | scope-diff
//                                             (priority: missing-output
//                                             wins when both fail).
//
// Does NOT surface as Outcome.Err — the gateway's false branch
// dispatches `fix-${failure-kind}` per Q-late-5. Hard errors
// (gh-optivem.yaml missing, git unusable) DO surface as Err since
// they indicate a wiring/infra problem, not an agent-output problem.
func (a actions) validateOutputsAndScopes(ctx *statemachine.Context) statemachine.Outcome {
	// 1. Output presence check.
	var missing []string
	for _, key := range splitCSV(ctx.Params["outputs"]) {
		if _, ok := ctx.State[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		ctx.Set("outputs-and-scopes-valid", false)
		ctx.Set("failure-kind", "missing-output")
		fmt.Fprintf(a.deps.Stderr,
			"validate-outputs-and-scopes: agent did not emit expected outputs: %s\n",
			strings.Join(missing, ", "))
		return statemachine.Outcome{}
	}

	// 2. Scope check (no-op when the caller did not declare scopes).
	scopes := splitCSV(ctx.Params["scopes"])
	if len(scopes) == 0 {
		ctx.Set("outputs-and-scopes-valid", true)
		return statemachine.Outcome{}
	}

	cfg, err := projectconfig.Load(a.deps.RepoPath)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: load gh-optivem.yaml: %w", err)}
	}
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: gh-optivem.yaml not found under %s", a.deps.RepoPath)}
	}
	allowed, err := resolveLayerPaths(scopes, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}
	modified, err := a.modifiedPathsSinceHead(context.Background())
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("validate-outputs-and-scopes: %w", err)}
	}

	var violating []string
	for _, m := range modified {
		if !pathInScope(m, allowed) {
			violating = append(violating, m)
		}
	}
	if len(violating) > 0 {
		ctx.Set("outputs-and-scopes-valid", false)
		ctx.Set("failure-kind", "scope-diff")
		fmt.Fprintf(a.deps.Stderr,
			"validate-outputs-and-scopes: %d path(s) outside scope %v:\n",
			len(violating), scopes)
		for _, v := range violating {
			fmt.Fprintf(a.deps.Stderr, "  out-of-scope: %s\n", v)
		}
		return statemachine.Outcome{}
	}

	ctx.Set("outputs-and-scopes-valid", true)
	return statemachine.Outcome{}
}

// splitCSV trims and splits a comma-separated param value; empty or
// whitespace-only input returns nil so callers can use `len(...) == 0`
// as the "skip this dimension" predicate.
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// writeIssueToContext mirrors the Tracker.PickReady result into the
// conventional Context keys downstream actions read.
func writeIssueToContext(ctx *statemachine.Context, issue tracker.Issue) {
	ctx.Set("issue_num", issue.ID)
	ctx.Set("issue_url", issue.URL)
	ctx.Set("issue_title", issue.Title)
	ctx.Set("issue_handle", issue.Handle)
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

func (realShell) Run(ctx context.Context, commandLine string) ([]byte, error) {
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
	// returned []byte still carries stdout for callers that parse it
	// (e.g. `gh optivem test run --list`) and stderr is still inlined
	// into the error message on failure.
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdoutBuf.Bytes(), fmt.Errorf("shell %q: %w (stderr: %s)",
				commandLine, err, strings.TrimSpace(stderrBuf.String()))
		}
		return stdoutBuf.Bytes(), fmt.Errorf("shell %q: %w", commandLine, err)
	}
	return stdoutBuf.Bytes(), nil
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
