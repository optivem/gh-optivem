// Tests for actions/bindings.go.
//
// Strategy: every action is exercised through fake Gh / Git / Shell /
// Prompter runners so the suite is hermetic. Each test seeds the Context
// inputs the action documents, runs the NodeFn, and asserts:
//   - the Outcome (Err on aborts; clean on success);
//   - the Context state mutated by the action; and
//   - the side-effecting calls observed by the fakes (argv shape).
//
// The board-backed and release-backed actions are tested via the same
// canned-response fakes; no real network or shell is invoked.
package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakePrompter struct {
	answers []string
	asked   []string
}

func (f *fakePrompter) Ask(prompt string) (string, error) {
	f.asked = append(f.asked, prompt)
	if len(f.answers) == 0 {
		return "", errors.New("fakePrompter: no answers left")
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}

type fakeRunner struct {
	t      *testing.T
	name   string
	calls  [][]string
	canned map[string]cannedResponse
}

type cannedResponse struct {
	out []byte
	err error
}

func newFakeRunner(t *testing.T, name string) *fakeRunner {
	return &fakeRunner{t: t, name: name, canned: map[string]cannedResponse{}}
}

func (f *fakeRunner) on(args []string, out []byte, err error) {
	f.canned[joinArgs(args)] = cannedResponse{out: out, err: err}
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	if r, ok := f.canned[joinArgs(args)]; ok {
		return r.out, r.err
	}
	f.t.Fatalf("%s: unexpected invocation %v (no canned response)", f.name, args)
	return nil, fmt.Errorf("unreachable")
}

func joinArgs(args []string) string {
	return strings.Join(args, "\x00")
}

type fakeShell struct {
	calls    []string
	out      []byte
	stderr   []byte
	exitCode int
	err      error
}

func (f *fakeShell) Run(_ context.Context, cmd string) (ShellResult, error) {
	f.calls = append(f.calls, cmd)
	return ShellResult{Stdout: f.out, Stderr: f.stderr, ExitCode: f.exitCode}, f.err
}

func newActions(deps Deps) actions {
	return actions{deps: deps.withDefaults()}
}

// loadTestEngine returns the canonical embedded process-flow Engine so
// scope-checking actions can resolve Engine.Scope(processName) against
// the same SSoT production code uses. Shared by every test that exercises
// checkPhaseScope / validateOutputsAndScopes.
func loadTestEngine(t *testing.T) *statemachine.Engine {
	t.Helper()
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("load embedded process-flow.yaml: %v", err)
	}
	return eng
}

// ---------------------------------------------------------------------------
// RegisterAll wiring
// ---------------------------------------------------------------------------

func TestRegisterAll_AllActionsRegistered(t *testing.T) {
	r := New()
	RegisterAll(r, Deps{
		Prompter: &fakePrompter{},
		Gh:       newFakeRunner(t, "gh"),
		Git:      newFakeRunner(t, "git"),
		Shell:    &fakeShell{},
	})
	want := []string{
		"pick-top-ready",
		"check-phase-scope",
		"run-command",
		"validate-outputs-and-scopes",
		"snapshot-working-tree",
		"move-to-in-refinement",
		"move-to-ready",
		"move-to-in-progress",
		"move-to-in-acceptance",
		"parse-ticket",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("action %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// checkPhaseScope — Layer 2 phase-scope enforcement
// ---------------------------------------------------------------------------

func TestPathInScope(t *testing.T) {
	allowed := []string{"system-test/typescript/tests/latest/acceptance", "dsl/typescript/src"}
	cases := []struct {
		path string
		want bool
	}{
		{"system-test/typescript/tests/latest/acceptance/foo.spec.ts", true},
		{"system-test/typescript/tests/latest/acceptance", true},  // exact match
		{"dsl/typescript/src/Driver.ts", true},
		{"dsl/typescript/srcOther/Driver.ts", false}, // directory-aware: no false-prefix match
		{"system-test/typescript/tests/latest/acceptanceX", false},
		{"system/monolith/typescript/src/Server.ts", false},
	}
	for _, tc := range cases {
		if got := pathInScope(tc.path, allowed); got != tc.want {
			t.Errorf("pathInScope(%q): got %v, want %v", tc.path, got, tc.want)
		}
	}
}

// writePhaseScopeTestConfig writes a minimal gh-optivem.yaml containing
// the system.path + Family B `paths:` entries process-flow.yaml's MID
// `read:` / `write:` scope lists reference. Used by the integration
// tests below to exercise the layer-name → resolved-path join without
// shelling out to `gh optivem config init`.
func writePhaseScopeTestConfig(t *testing.T, repoPath string) *projectconfig.Config {
	t.Helper()
	body := `project:
  provider: github
  url: https://github.com/orgs/acme/projects/1

repo-strategy: mono-repo

sonar:
  organization: acme

system:
  architecture: monolith
  path: system/monolith/typescript
  repo: acme/shop
  lang: typescript
  sonar-project: acme_shop-system

system-test:
  path: system-test/typescript
  repo: acme/shop
  lang: typescript
  sonar-project: acme_shop-system-test
  paths:
    at-test: system-test/typescript/tests/latest/acceptance
    dsl-port: dsl/typescript/src/port
    dsl-core: dsl/typescript/src/core
    driver-port: driver/typescript/src/port
    driver-adapter: driver/typescript/src/adapter
    ct-test: system-test/typescript/tests/latest/contract
    external-system-driver-port: driver/typescript/src/external-port
    external-system-driver-adapter: driver/typescript/src/external-adapter
`
	if err := os.WriteFile(filepath.Join(repoPath, "gh-optivem.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write gh-optivem.yaml: %v", err)
	}
	cfg, err := projectconfig.Load(repoPath)
	if err != nil {
		t.Fatalf("load gh-optivem.yaml: %v", err)
	}
	if cfg == nil {
		t.Fatalf("load gh-optivem.yaml: cfg is nil")
	}
	return cfg
}

func TestCheckPhaseScope_RequiresPhaseID(t *testing.T) {
	a := newActions(Deps{Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	out := a.checkPhaseScope(ctx)
	if out.Err == nil || !strings.Contains(out.Err.Error(), "phase_id") {
		t.Fatalf("expected phase_id error, got %v", out.Err)
	}
}

func TestCheckPhaseScope_UnknownPhaseIsHardError(t *testing.T) {
	a := newActions(Deps{Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["phase_id"] = "nonexistent-phase"
	out := a.checkPhaseScope(ctx)
	if out.Err == nil {
		t.Fatalf("expected error on unknown phase, got nil")
	}
	if !strings.Contains(out.Err.Error(), "nonexistent-phase") {
		t.Errorf("error should name the phase: %v", out.Err)
	}
}

func TestCheckPhaseScope_CleanWhenAllModificationsInScope(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// HEAD-equivalent fallback path: no pre-agent-fingerprint in state,
	// so checkPhaseScope enumerates the full dirty tree via `git status`.
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(" M system-test/typescript/tests/latest/acceptance/foo.spec.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// write-acceptance-tests scope: at-test, dsl-port, dsl-core.
	ctx.Params["phase_id"] = "write-acceptance-tests"
	out := a.checkPhaseScope(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get(CtxKeyPhaseScopeClean); got != true {
		t.Fatalf("phase_scope_clean: got %v, want true", got)
	}
	if v := ctx.Get(CtxKeyPhaseScopeViolatingPaths); v != nil {
		t.Errorf("violating_paths: got %v, want nil", v)
	}
}

func TestCheckPhaseScope_ViolationPopulatesContext(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// write-acceptance-tests scope: at-test, dsl-port, dsl-core. The
	// driver-port edit is outside scope. HEAD-equivalent fallback (no
	// snapshot pre-seeded), so checkPhaseScope enumerates the full dirty
	// tree via `git status`.
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(" M driver/typescript/src/port/Driver.ts\n M system-test/typescript/tests/latest/acceptance/foo.spec.ts\n"), nil)
	var stderr bytes.Buffer
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &stderr, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["phase_id"] = "write-acceptance-tests"
	out := a.checkPhaseScope(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get(CtxKeyPhaseScopeClean); got != false {
		t.Fatalf("phase_scope_clean: got %v, want false", got)
	}
	violating, ok := ctx.State[CtxKeyPhaseScopeViolatingPaths].([]string)
	if !ok {
		t.Fatalf("violating_paths: not set or wrong type")
	}
	if len(violating) != 1 || violating[0] != "driver/typescript/src/port/Driver.ts" {
		t.Fatalf("violating: got %v, want [driver/typescript/src/port/Driver.ts]", violating)
	}
	if !strings.Contains(stderr.String(), "scope violation") {
		t.Errorf("expected scope-violation banner in stderr, got %q", stderr.String())
	}
}

func TestCheckPhaseScope_RenameTracksBothEndpoints(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// Rename row: porcelain shape "R  old -> new". "somewhere/else/New.ts"
	// is outside scope; the action must surface it. HEAD-equivalent
	// fallback (no snapshot pre-seeded).
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("R  dsl/typescript/src/core/Old.ts -> somewhere/else/New.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// implement-dsl scope: dsl-core, driver-port, external-system-driver-port.
	ctx.Params["phase_id"] = "implement-dsl"
	out := a.checkPhaseScope(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get(CtxKeyPhaseScopeClean); got != false {
		t.Fatalf("phase_scope_clean: got %v, want false (rename target is outside scope)", got)
	}
	violating, _ := ctx.State[CtxKeyPhaseScopeViolatingPaths].([]string)
	found := false
	for _, v := range violating {
		if v == "somewhere/else/New.ts" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("violating slice missing rename target: %v", violating)
	}
}

// ---------------------------------------------------------------------------
// run-command (BPMN Phase D Item 1, Q-D5)
// ---------------------------------------------------------------------------

func TestRunCommand_HappyPath(t *testing.T) {
	sh := &fakeShell{out: []byte("OK")}
	var stdout, stderr bytes.Buffer
	a := newActions(Deps{Shell: sh, Stdout: &stdout, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem compile"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("command-succeeded"); got != true {
		t.Fatalf("command-succeeded: got %v, want true", got)
	}
	if _, set := ctx.State["test-outcome"]; set {
		t.Fatalf("test-outcome should NOT be set for non-run-tests commands: got %v", ctx.Get("test-outcome"))
	}
	// The diagnostic payload (failure-kind + command-* keys) is a
	// failure-only signal — the fix-command-failed dispatch consumes it.
	// On the happy path it must be absent so a downstream gateway can
	// safely treat "failure-kind set" as the routing condition.
	if _, set := ctx.State["failure-kind"]; set {
		t.Fatalf("failure-kind should NOT be set on success: got %v", ctx.Get("failure-kind"))
	}
	if _, set := ctx.State["command-line"]; set {
		t.Fatalf("command-line should NOT be set on success: got %v", ctx.Get("command-line"))
	}
	if _, set := ctx.State["command-exit-code"]; set {
		t.Fatalf("command-exit-code should NOT be set on success: got %v", ctx.Get("command-exit-code"))
	}
	if _, set := ctx.State["command-stderr-tail"]; set {
		t.Fatalf("command-stderr-tail should NOT be set on success: got %v", ctx.Get("command-stderr-tail"))
	}
	if len(sh.calls) != 1 || sh.calls[0] != "gh optivem compile" {
		t.Fatalf("shell calls: got %v, want [\"gh optivem compile\"]", sh.calls)
	}
}

func TestRunCommand_FailureRoutes_NotErrors(t *testing.T) {
	sh := &fakeShell{
		out:      []byte("fail"),
		stderr:   []byte("boom: file not found\ntraceback line 1\ntraceback line 2\n"),
		exitCode: 7,
		err:      errors.New("exit 7"),
	}
	var stdout, stderr bytes.Buffer
	a := newActions(Deps{Shell: sh, Stdout: &stdout, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem commit"
	out := a.runCommand(ctx)
	// Failure must route, not halt — the cycle's GATE_COMMAND_SUCCEEDED
	// dispatches `fix` on the false branch.
	if out.Err != nil {
		t.Fatalf("command failure should route, not halt: %v", out.Err)
	}
	if got := ctx.Get("command-succeeded"); got != false {
		t.Fatalf("command-succeeded: got %v, want false", got)
	}
	// failure-kind + diagnostic payload are the signal the downstream
	// `fix-command-failed` dispatch consumes. The Q-late-5 β-convention
	// resolves task-name as "fix-" + failure-kind, so the literal value
	// here is load-bearing — a rename here breaks the prompt lookup.
	if got := ctx.GetString("failure-kind"); got != "command-failed" {
		t.Fatalf("failure-kind: got %q, want %q", got, "command-failed")
	}
	if got := ctx.GetString("command-line"); got != "gh optivem commit" {
		t.Fatalf("command-line: got %q, want %q", got, "gh optivem commit")
	}
	if got := ctx.Get("command-exit-code"); got != 7 {
		t.Fatalf("command-exit-code: got %v, want 7", got)
	}
	wantTail := "boom: file not found\ntraceback line 1\ntraceback line 2"
	if got := ctx.GetString("command-stderr-tail"); got != wantTail {
		t.Fatalf("command-stderr-tail:\n got: %q\nwant: %q", got, wantTail)
	}
}

func TestRunCommand_StderrTailTruncatesToLastNLines(t *testing.T) {
	// Stash the stderr payload feeds into the fix-command-failed prompt;
	// commandStderrTailLines caps the block at 20 lines so a chatty runner
	// (e.g. a docker pull stream) doesn't blow out the prompt budget.
	var b strings.Builder
	for i := 0; i < commandStderrTailLines+5; i++ {
		fmt.Fprintf(&b, "line %d\n", i+1)
	}
	sh := &fakeShell{
		out:      nil,
		stderr:   []byte(b.String()),
		exitCode: 1,
		err:      errors.New("exit 1"),
	}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "noisy-runner"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	tail := ctx.GetString("command-stderr-tail")
	tailLines := strings.Split(tail, "\n")
	if len(tailLines) != commandStderrTailLines {
		t.Fatalf("tail line count: got %d, want %d (commandStderrTailLines)", len(tailLines), commandStderrTailLines)
	}
	// The first line of the tail should be line 6 (we wrote 25 lines, kept
	// the last 20, so the surviving first line is 25 - 20 + 1 = 6).
	if tailLines[0] != "line 6" {
		t.Errorf("tail[0]: got %q, want %q", tailLines[0], "line 6")
	}
	if tailLines[len(tailLines)-1] != "line 25" {
		t.Errorf("tail[last]: got %q, want %q", tailLines[len(tailLines)-1], "line 25")
	}
}

func TestRunCommand_RunTestsStampsTestOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "pass", err: nil, want: "pass"},
		{name: "fail", err: errors.New("exit 1"), want: "fail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := &fakeShell{out: []byte(""), err: tc.err}
			var stderr bytes.Buffer
			a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &stderr})
			ctx := statemachine.NewContext()
			ctx.Params["command"] = "gh optivem test run"
			out := a.runCommand(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if got := ctx.GetString("test-outcome"); got != tc.want {
				t.Fatalf("test-outcome: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunCommand_FilterFlagsAppendedOnlyWhenSet(t *testing.T) {
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem test run"
	ctx.Params["filter-type"] = "test-type"
	ctx.Params["filter-value"] = "at-test"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if len(sh.calls) != 1 {
		t.Fatalf("expected 1 shell call, got %d: %v", len(sh.calls), sh.calls)
	}
	if !strings.Contains(sh.calls[0], "--filter-type=test-type") {
		t.Fatalf("shell call missing --filter-type=: %q", sh.calls[0])
	}
	if !strings.Contains(sh.calls[0], "--filter-value=at-test") {
		t.Fatalf("shell call missing --filter-value=: %q", sh.calls[0])
	}
}

func TestRunCommand_NoFilterFlagsWhenEmpty(t *testing.T) {
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem commit"
	// filter-type / filter-value left empty
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if strings.Contains(sh.calls[0], "--filter-") {
		t.Fatalf("shell call should not carry filter flags: %q", sh.calls[0])
	}
}

func TestRunCommand_EmptyCommandHalts(t *testing.T) {
	sh := &fakeShell{}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	// no command param set
	out := a.runCommand(ctx)
	if out.Err == nil {
		t.Fatalf("expected err for missing command param, got %+v", out)
	}
	if len(sh.calls) != 0 {
		t.Fatalf("shell should not be called when command param is empty: %v", sh.calls)
	}
}

// ---------------------------------------------------------------------------
// validate-outputs-and-scopes (BPMN Phase D Item 7, Q-D6)
// ---------------------------------------------------------------------------

func TestValidateOutputsAndScopes_NoOutputs_NoScopes_IsValid(t *testing.T) {
	// refine-acceptance-criteria carries no outputs and no inline scope
	// (declares scope: none in prompt frontmatter) — the validation must
	// pass trivially.
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
	if _, set := ctx.State["failure-kind"]; set {
		t.Fatalf("failure-kind should not be set on success: got %v", ctx.Get("failure-kind"))
	}
}

func TestValidateOutputsAndScopes_MissingOutput_FlagsAndKind(t *testing.T) {
	var stderr bytes.Buffer
	a := newActions(Deps{Stderr: &stderr, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["outputs"] = "dsl-port-changed,system-driver-ports-changed"
	// Only one of the two declared outputs is present in state.
	ctx.Set("dsl-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != false {
		t.Fatalf("outputs-and-scopes-valid: got %v, want false", got)
	}
	if got := ctx.GetString("failure-kind"); got != "missing-output" {
		t.Fatalf("failure-kind: got %q, want %q", got, "missing-output")
	}
	if !strings.Contains(stderr.String(), "system-driver-ports-changed") {
		t.Fatalf("stderr missing output name: %q", stderr.String())
	}
}

func TestValidateOutputsAndScopes_OutputsPresent_NoScope_IsValid(t *testing.T) {
	// A task-name with no engine entry (scope: none / unknown) skips the
	// scope check; outputs-only validation succeeds when every key is
	// present in state.
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.Params["outputs"] = "dsl-port-changed"
	ctx.Set("dsl-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
}

func TestValidateOutputsAndScopes_ScopeDiff_FlagsAndKind(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// Post-state dirty tree: two untracked paths the failing agent added.
	// One in scope (dsl-core), one not (somewhere/else).
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? dsl/typescript/src/core/Logic.ts\n?? somewhere/else/Stray.ts\n"), nil)
	var stderr bytes.Buffer
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &stderr, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// implement-dsl scope (write list) covers dsl-core, driver-port,
	// external-system-driver-port — but not "somewhere/else".
	ctx.Params["task-name"] = "implement-dsl"
	// Empty snapshot → every dirty path is "added by this phase".
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != false {
		t.Fatalf("outputs-and-scopes-valid: got %v, want false", got)
	}
	if got := ctx.GetString("failure-kind"); got != "scope-diff" {
		t.Fatalf("failure-kind: got %q, want %q", got, "scope-diff")
	}
	if !strings.Contains(stderr.String(), "somewhere/else/Stray.ts") {
		t.Fatalf("stderr should name the out-of-scope file: %q", stderr.String())
	}
	// phase-changed-files carries the FULL snapshot delta (both files),
	// so fix-scope-diff's ${changed_files} sees what the agent did.
	gotChanged := ctx.GetString("phase-changed-files")
	wantChanged := "dsl/typescript/src/core/Logic.ts\nsomewhere/else/Stray.ts"
	if gotChanged != wantChanged {
		t.Errorf("phase-changed-files: got %q, want %q", gotChanged, wantChanged)
	}
}

func TestValidateOutputsAndScopes_AllClean_IsValid(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	// Empty snapshot + empty dirty tree → no delta, scope check passes.
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.Params["outputs"] = "dsl-port-changed"
	ctx.Set("dsl-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
}

func TestValidateOutputsAndScopes_MissingOutputWins_OverScopeDiff(t *testing.T) {
	// Both failure conditions hold; missing-output is prioritised (the
	// agent must first emit the flag — there is nothing to validate
	// scope-wise if the agent didn't even claim to have done the work).
	// The snapshot baseline is not consulted on the missing-output path,
	// so we deliberately leave pre-agent-fingerprint unset to confirm
	// missing-output short-circuits before the snapshot lookup.
	cfg := writePhaseScopeTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["outputs"] = "dsl-port-changed"
	// dsl-port-changed not set in state.
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("failure-kind"); got != "missing-output" {
		t.Fatalf("failure-kind: got %q, want %q (missing-output must win)", got, "missing-output")
	}
}

func TestValidateOutputsAndScopes_FixPath_UsesOriginatingTaskName(t *testing.T) {
	// When a fix dispatch is in flight, task-name is fix-${failure-kind}
	// (no MID entry) but originating-task-name carries the outer MID's
	// name so scope resolution stays on the original phase's write list.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// Empty snapshot + dirty tree with one in-scope (dsl-core) and one
	// out-of-scope path; the scope-diff branch must fire against
	// implement-dsl's write list (looked up via originating-task-name).
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? dsl/typescript/src/core/Logic.ts\n?? somewhere/else/Stray.ts\n"), nil)
	var stderr bytes.Buffer
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &stderr, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "fix-scope-diff"
	ctx.Params["originating-task-name"] = "implement-dsl"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != false {
		t.Fatalf("outputs-and-scopes-valid: got %v, want false (scope-diff via originating-task-name)", got)
	}
	if got := ctx.GetString("failure-kind"); got != "scope-diff" {
		t.Fatalf("failure-kind: got %q, want %q", got, "scope-diff")
	}
	if got := ctx.GetString("failing-task-name"); got != "implement-dsl" {
		t.Errorf("failing-task-name: got %q, want %q (originating MID name)", got, "implement-dsl")
	}
}

func TestValidateOutputsAndScopes_MissingSnapshot_HardErrors(t *testing.T) {
	// Scope check is required (task-name resolves to a writing-agent MID
	// with a non-empty write scope, output is present), but the upstream
	// snapshot-working-tree step never ran. This is a wiring bug:
	// validate-outputs-and-scopes must surface it as Outcome.Err, not
	// silently report "valid" or "no changes".
	cfg := writePhaseScopeTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["outputs"] = "dsl-port-changed"
	ctx.Set("dsl-port-changed", true)
	// Deliberately do NOT seed CtxKeyPreAgentFingerprint.
	out := a.validateOutputsAndScopes(ctx)
	if out.Err == nil {
		t.Fatalf("expected hard error on missing pre-agent-fingerprint, got nil")
	}
	if !strings.Contains(out.Err.Error(), "pre-agent-fingerprint not set") {
		t.Errorf("error should explain the missing snapshot: %v", out.Err)
	}
}

func TestValidateOutputsAndScopes_UpstreamPhaseResidue_DoesNotViolate(t *testing.T) {
	// Rehearsal-#61 shape: an upstream phase legitimately edited
	// upstream-edit.ts before this validator ran. The pre-agent snapshot
	// already records that file's bytes; the current dirty tree still
	// shows it (no commit between phases). This phase's agent made no
	// edits, so the delta against the snapshot is empty even though the
	// raw `git status` would surface upstream-edit.ts. The validator
	// must not flag it.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	writeRepoFile(t, repoPath, "system-test/typescript/upstream-edit.ts", "edited by upstream phase")
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? system-test/typescript/upstream-edit.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// Snapshot captures the upstream path at its current hash — this
	// phase therefore has zero delta against it.
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{
		"system-test/typescript/upstream-edit.ts": a.hashRepoFile("system-test/typescript/upstream-edit.ts"),
	}
	// Current phase's scope (external-system-driver-port +
	// external-system-driver-adapter) intentionally excludes the upstream
	// system-test path. Pre-snapshot code would have reported it as a
	// violation; the snapshot baseline correctly attributes it to the
	// upstream phase.
	ctx.Params["task-name"] = "implement-external-system-driver-adapters"
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true (upstream residue must not count)", got)
	}
	if got := ctx.GetString("phase-changed-files"); got != "" {
		t.Errorf("phase-changed-files: got %q, want empty (this phase made no edits)", got)
	}
}

// ---------------------------------------------------------------------------
// parseTicket — body parsing + state population
// ---------------------------------------------------------------------------

// fakeTracker is a test-side tracker.Tracker that returns canned sections
// from ReadSections. Other methods panic — only parseTicket exercises
// ReadSections.
type fakeTracker struct {
	sections   map[string]string
	readErr    error
	readCalled bool
}

func (f *fakeTracker) PickReady(context.Context) (tracker.Issue, error) {
	panic("fakeTracker.PickReady: not implemented")
}
func (f *fakeTracker) FindIssue(context.Context, string) (tracker.Issue, error) {
	panic("fakeTracker.FindIssue: not implemented")
}
func (f *fakeTracker) SetStatus(context.Context, string, string) error {
	panic("fakeTracker.SetStatus: not implemented")
}
func (f *fakeTracker) Verify(context.Context) error {
	panic("fakeTracker.Verify: not implemented")
}
func (f *fakeTracker) Classify(context.Context, tracker.Issue) (string, bool, error) {
	panic("fakeTracker.Classify: not implemented")
}
func (f *fakeTracker) Subtypes(context.Context, tracker.Issue) ([]string, error) {
	panic("fakeTracker.Subtypes: not implemented")
}
func (f *fakeTracker) ReadSections(_ context.Context, _ tracker.Issue, _ []string) (map[string]string, error) {
	f.readCalled = true
	return f.sections, f.readErr
}

func seedIssue(ctx *statemachine.Context) {
	ctx.Set("issue_num", "42")
	ctx.Set("issue_url", "https://github.com/example/example/issues/42")
	ctx.Set("issue_title", "Test")
	ctx.Set("issue_handle", "PROJID:ITEMID")
}

func TestParseTicket_PopulatesStateOnHappyPath(t *testing.T) {
	tk := &fakeTracker{sections: map[string]string{
		"Description":         "Some prose.",
		"Acceptance Criteria": "Scenario: x\n  Given y\n  When z\n  Then w",
		"Steps to Reproduce":  "",
		"Checklist":           "",
	}}
	a := newActions(Deps{Tracker: tk})
	ctx := statemachine.NewContext()
	seedIssue(ctx)

	out := a.parseTicket(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !tk.readCalled {
		t.Fatalf("expected ReadSections to be called")
	}
	if got := ctx.GetString("ticket_description"); got != "Some prose." {
		t.Errorf("ticket_description: got %q", got)
	}
	if got := ctx.GetString("ticket_acceptance_criteria"); !strings.Contains(got, "Scenario: x") {
		t.Errorf("ticket_acceptance_criteria: got %q", got)
	}
	if got := ctx.GetString("ticket_checklist"); got != "" {
		t.Errorf("ticket_checklist: got %q, want empty (no Checklist in body)", got)
	}
}

func TestParseTicket_ChecklistSectionStashed(t *testing.T) {
	tk := &fakeTracker{sections: map[string]string{
		"Checklist": "- [x] One done\n- [ ] Two pending",
	}}
	a := newActions(Deps{Tracker: tk})
	ctx := statemachine.NewContext()
	seedIssue(ctx)

	if out := a.parseTicket(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	got := ctx.GetString("ticket_checklist")
	if !strings.Contains(got, "- [x] One done") || !strings.Contains(got, "- [ ] Two pending") {
		t.Fatalf("ticket_checklist body lost: got %q", got)
	}
}

func TestParseTicket_BothACAndChecklist_XORViolation(t *testing.T) {
	tk := &fakeTracker{sections: map[string]string{
		"Acceptance Criteria": "Scenario: x",
		"Checklist":           "- [ ] step",
	}}
	a := newActions(Deps{Tracker: tk})
	ctx := statemachine.NewContext()
	seedIssue(ctx)

	out := a.parseTicket(ctx)
	if out.Err == nil {
		t.Fatalf("expected XOR-violation error, got nil")
	}
	if !strings.Contains(out.Err.Error(), "Acceptance Criteria") || !strings.Contains(out.Err.Error(), "Checklist") {
		t.Errorf("error should name both sections: %v", out.Err)
	}
}

func TestParseTicket_TrackerReadError_Surfaces(t *testing.T) {
	tk := &fakeTracker{readErr: errors.New("tracker boom")}
	a := newActions(Deps{Tracker: tk})
	ctx := statemachine.NewContext()
	seedIssue(ctx)

	out := a.parseTicket(ctx)
	if out.Err == nil {
		t.Fatalf("expected error to propagate")
	}
	if !strings.Contains(out.Err.Error(), "tracker boom") {
		t.Errorf("error should wrap tracker error: %v", out.Err)
	}
}

func TestParseTicket_NoIssueURL_Fails(t *testing.T) {
	a := newActions(Deps{Tracker: &fakeTracker{}})
	ctx := statemachine.NewContext()
	// No seedIssue — issue_url missing.

	out := a.parseTicket(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for missing issue_url")
	}
}

// ---------------------------------------------------------------------------
// Working-tree fingerprint helpers (per plan 20260526-1430 Items 1, 6)
// ---------------------------------------------------------------------------

// writeRepoFile is a tiny helper for the fingerprint tests: writes a
// file under repo at the given relative path (creating parents) so
// captureWorkingTreeFingerprint / modifiedPathsSinceFingerprint have
// real bytes to hash.
func writeRepoFile(t *testing.T, repo, rel, content string) {
	t.Helper()
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", rel, err)
	}
}

func TestCaptureWorkingTreeFingerprint(t *testing.T) {
	tcs := []struct {
		name     string
		status   string
		files    map[string]string // repo-relative path → content
		wantKeys []string
	}{
		{
			name:     "clean working tree → empty fingerprint",
			status:   "",
			files:    nil,
			wantKeys: nil,
		},
		{
			name:     "one modified tracked file",
			status:   " M src/foo.go\n",
			files:    map[string]string{"src/foo.go": "package foo\n"},
			wantKeys: []string{"src/foo.go"},
		},
		{
			name:     "one untracked file",
			status:   "?? new.txt\n",
			files:    map[string]string{"new.txt": "hello"},
			wantKeys: []string{"new.txt"},
		},
		{
			name:     "rename — both endpoints fingerprinted",
			status:   "R  old.txt -> new/path.txt\n",
			files:    map[string]string{"new/path.txt": "renamed"},
			wantKeys: []string{"new/path.txt", "old.txt"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			for p, c := range tc.files {
				writeRepoFile(t, repo, p, c)
			}
			git := newFakeRunner(t, "git")
			git.on([]string{"-C", repo, "status", "--porcelain"}, []byte(tc.status), nil)

			a := newActions(Deps{Git: git, RepoPath: repo})
			fp, err := a.captureWorkingTreeFingerprint(context.Background())
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(fp) != len(tc.wantKeys) {
				t.Fatalf("fingerprint size: got %d (%v), want %d (%v)", len(fp), fp, len(tc.wantKeys), tc.wantKeys)
			}
			for _, k := range tc.wantKeys {
				if _, ok := fp[k]; !ok {
					t.Errorf("missing key %q (fp = %v)", k, fp)
				}
			}
			// Every recorded hash must match a fresh read from disk.
			// "" is allowed for paths whose file does not exist
			// (e.g. the rename's old endpoint, which has no bytes
			// to hash on the new tree).
			for k, v := range fp {
				if want := a.hashRepoFile(k); want != v {
					t.Errorf("hash mismatch for %q: stored=%q, disk=%q", k, v, want)
				}
			}
		})
	}
}

func TestCaptureWorkingTreeFingerprint_GitStatusError(t *testing.T) {
	// Genuine wiring failure (git missing, repo invalid) must surface
	// as an error — the snapshot action turns this into Outcome.Err.
	repo := t.TempDir()
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repo, "status", "--porcelain"}, nil, errors.New("git: not found"))
	a := newActions(Deps{Git: git, RepoPath: repo})
	_, err := a.captureWorkingTreeFingerprint(context.Background())
	if err == nil || !strings.Contains(err.Error(), "git status --porcelain") {
		t.Fatalf("expected git-status error, got %v", err)
	}
}

func TestModifiedPathsSinceFingerprint_AddDeleteModifyNoOp(t *testing.T) {
	// Each subtest stages a snapshot + a post-state working tree, then
	// asserts the delta. The "no-op" case is the bug-fix scenario:
	// upstream phase already dirtied a file, this phase does not touch
	// it, and the validator must not flag it.
	t.Run("add — file absent in snapshot, present in current status", func(t *testing.T) {
		repo := t.TempDir()
		writeRepoFile(t, repo, "added.txt", "new")
		git := newFakeRunner(t, "git")
		git.on([]string{"-C", repo, "status", "--porcelain"}, []byte("?? added.txt\n"), nil)
		a := newActions(Deps{Git: git, RepoPath: repo})
		got, err := a.modifiedPathsSinceFingerprint(context.Background(), WorkingTreeFingerprint{})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0] != "added.txt" {
			t.Fatalf("delta: got %v, want [added.txt]", got)
		}
	})
	t.Run("delete — file in snapshot, missing on disk", func(t *testing.T) {
		repo := t.TempDir()
		// File was tracked-modified at snapshot, deleted by the phase.
		// `git status --porcelain` reports " D removed.txt" (no file
		// on disk).
		git := newFakeRunner(t, "git")
		git.on([]string{"-C", repo, "status", "--porcelain"}, []byte(" D removed.txt\n"), nil)
		a := newActions(Deps{Git: git, RepoPath: repo})
		base := WorkingTreeFingerprint{"removed.txt": "deadbeef"} // non-empty pre-state hash
		got, err := a.modifiedPathsSinceFingerprint(context.Background(), base)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0] != "removed.txt" {
			t.Fatalf("delta: got %v, want [removed.txt]", got)
		}
	})
	t.Run("modify — file in snapshot, different bytes on disk", func(t *testing.T) {
		repo := t.TempDir()
		writeRepoFile(t, repo, "edited.txt", "new content")
		git := newFakeRunner(t, "git")
		git.on([]string{"-C", repo, "status", "--porcelain"}, []byte(" M edited.txt\n"), nil)
		a := newActions(Deps{Git: git, RepoPath: repo})
		base := WorkingTreeFingerprint{"edited.txt": "old-hash-value"} // does not match disk
		got, err := a.modifiedPathsSinceFingerprint(context.Background(), base)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0] != "edited.txt" {
			t.Fatalf("delta: got %v, want [edited.txt]", got)
		}
	})
	t.Run("no-op — upstream-phase residue not attributed to this phase", func(t *testing.T) {
		// Bug-fix scenario from plan 20260526-1430: Phase 1 edited
		// upstream.txt; Phase 2's snapshot already records the
		// upstream-edited content. Phase 2 makes no edits. The delta
		// must be empty.
		repo := t.TempDir()
		writeRepoFile(t, repo, "upstream.txt", "edited-by-phase-1")
		git := newFakeRunner(t, "git")
		git.on([]string{"-C", repo, "status", "--porcelain"}, []byte(" M upstream.txt\n"), nil)
		a := newActions(Deps{Git: git, RepoPath: repo})
		base := WorkingTreeFingerprint{"upstream.txt": a.hashRepoFile("upstream.txt")}
		got, err := a.modifiedPathsSinceFingerprint(context.Background(), base)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("delta: got %v, want []", got)
		}
	})
	t.Run("delta is sorted and de-duplicated", func(t *testing.T) {
		repo := t.TempDir()
		writeRepoFile(t, repo, "a.txt", "a")
		writeRepoFile(t, repo, "b.txt", "b")
		writeRepoFile(t, repo, "c.txt", "c")
		git := newFakeRunner(t, "git")
		// Status reports them out of order; the delta must come back sorted.
		git.on([]string{"-C", repo, "status", "--porcelain"},
			[]byte("?? c.txt\n?? a.txt\n?? b.txt\n"), nil)
		a := newActions(Deps{Git: git, RepoPath: repo})
		got, err := a.modifiedPathsSinceFingerprint(context.Background(), WorkingTreeFingerprint{})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		want := []string{"a.txt", "b.txt", "c.txt"}
		if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
			t.Fatalf("delta order: got %v, want %v", got, want)
		}
	})
}

func TestExecuteAgentPipeline_PerPhaseBaseline_NoFalsePositiveOnUpstreamResidue(t *testing.T) {
	// Action-bindings-level smoke for the execute-agent pipeline's
	// per-phase baseline contract (plan 20260526-1430). Mirrors the
	// rehearsal-#61 shape end-to-end across the two actions
	// process-flow.yaml wires sequentially:
	//   1. snapshot-working-tree (before RUN_AGENT)
	//   2. validate-outputs-and-scopes (after RUN_AGENT)
	// Phase 1 (different scope) legitimately edited upstream.ts; Phase 2
	// runs immediately after with a narrower scope that excludes
	// upstream.ts and its agent makes no edits. Pre-snapshot wiring
	// flagged upstream.ts against Phase 2's scope; the snapshot baseline
	// must report valid=true with no phase-changed-files.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	// Phase 1's residual edit: file exists on disk when Phase 2 starts.
	writeRepoFile(t, repoPath, "system-test/typescript/upstream.ts", "edited by phase 1")
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? system-test/typescript/upstream.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()

	// Step 1: SNAPSHOT_WORKING_TREE before this phase's agent runs.
	if out := a.snapshotWorkingTree(ctx); out.Err != nil {
		t.Fatalf("snapshotWorkingTree: %v", out.Err)
	}

	// Step 2: agent runs and makes NO edits — working tree is unchanged
	// between the snapshot and the post-RUN validator. The fake git
	// stub returns the same `git status` response on the second poll.

	// Step 3: VALIDATE_OUTPUTS_AND_SCOPES — task-name resolves to the
	// MID whose write scope is external-system-driver-port +
	// external-system-driver-adapter (excludes the upstream system-test
	// path).
	ctx.Params["task-name"] = "implement-external-system-driver-adapters"
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("validateOutputsAndScopes: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true (upstream residue must not count)", got)
	}
	if got := ctx.GetString("phase-changed-files"); got != "" {
		t.Errorf("phase-changed-files: got %q, want empty (no edits in this phase)", got)
	}
	if _, set := ctx.State["failure-kind"]; set {
		t.Errorf("failure-kind set on a clean phase: %v", ctx.Get("failure-kind"))
	}
}

func TestSnapshotWorkingTreeAction_StashesFingerprint(t *testing.T) {
	repo := t.TempDir()
	writeRepoFile(t, repo, "dirty.txt", "x")
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repo, "status", "--porcelain"}, []byte("?? dirty.txt\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repo})
	ctx := statemachine.NewContext()

	out := a.snapshotWorkingTree(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	fp, ok := ctx.State[CtxKeyPreAgentFingerprint].(WorkingTreeFingerprint)
	if !ok {
		t.Fatalf("CtxKeyPreAgentFingerprint not set or wrong type: %T", ctx.State[CtxKeyPreAgentFingerprint])
	}
	if _, ok := fp["dirty.txt"]; !ok {
		t.Errorf("snapshot missing dirty.txt: got %v", fp)
	}
}

func TestSnapshotWorkingTreeAction_GitFailureIsHardError(t *testing.T) {
	repo := t.TempDir()
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repo, "status", "--porcelain"}, nil, errors.New("boom"))
	a := newActions(Deps{Git: git, RepoPath: repo})
	ctx := statemachine.NewContext()

	out := a.snapshotWorkingTree(ctx)
	if out.Err == nil || !strings.Contains(out.Err.Error(), "snapshot-working-tree") {
		t.Fatalf("expected snapshot-working-tree error, got %v", out.Err)
	}
	if _, set := ctx.State[CtxKeyPreAgentFingerprint]; set {
		t.Errorf("fingerprint must not be stashed on error")
	}
}
