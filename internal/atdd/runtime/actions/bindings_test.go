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
// the system.path + Family B `paths:` entries phase-scopes.yaml's
// AT_RED_TEST and AT_GREEN_SYSTEM rows reference. Used by the integration
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
	a := newActions(Deps{})
	ctx := statemachine.NewContext()
	out := a.checkPhaseScope(ctx)
	if out.Err == nil || !strings.Contains(out.Err.Error(), "phase_id") {
		t.Fatalf("expected phase_id error, got %v", out.Err)
	}
}

func TestCheckPhaseScope_UnknownPhaseIsHardError(t *testing.T) {
	a := newActions(Deps{})
	ctx := statemachine.NewContext()
	ctx.Params["phase_id"] = "NONEXISTENT_PHASE"
	out := a.checkPhaseScope(ctx)
	if out.Err == nil {
		t.Fatalf("expected error on unknown phase, got nil")
	}
	if !strings.Contains(out.Err.Error(), "NONEXISTENT_PHASE") {
		t.Errorf("error should name the phase: %v", out.Err)
	}
}

func TestCheckPhaseScope_CleanWhenAllModificationsInScope(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "diff", "--name-only", "HEAD"},
		[]byte("system-test/typescript/tests/latest/acceptance/foo.spec.ts\ndsl/typescript/src/core/Logic.ts\n"), nil)
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(" M system-test/typescript/tests/latest/acceptance/foo.spec.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Params["phase_id"] = "AT_RED_TEST"
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
	// AT_RED_TEST scope: at-test, dsl-port, dsl-core. The driver-port edit
	// is outside scope.
	git.on([]string{"-C", repoPath, "diff", "--name-only", "HEAD"},
		[]byte("driver/typescript/src/port/Driver.ts\nsystem-test/typescript/tests/latest/acceptance/foo.spec.ts\n"), nil)
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	var stderr bytes.Buffer
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Params["phase_id"] = "AT_RED_TEST"
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
	git.on([]string{"-C", repoPath, "diff", "--name-only", "HEAD"},
		[]byte("dsl/typescript/src/core/Old.ts\nsomewhere/else/New.ts\n"), nil)
	// Rename row: porcelain shape "R  old -> new". "somewhere/else/New.ts"
	// is outside scope; the action must surface it.
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("R  dsl/typescript/src/core/Old.ts -> somewhere/else/New.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["phase_id"] = "AT_RED_DSL" // scope: dsl-core, driver-port
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
	// refine-acceptance-criteria carries no outputs and no scopes — the
	// validation must pass trivially.
	a := newActions(Deps{Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
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
	a := newActions(Deps{Stderr: &stderr})
	ctx := statemachine.NewContext()
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

func TestValidateOutputsAndScopes_OutputsPresent_NoScopes_IsValid(t *testing.T) {
	a := newActions(Deps{Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
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
	git.on([]string{"-C", repoPath, "diff", "--name-only", "HEAD"},
		[]byte("dsl/typescript/src/core/Logic.ts\nsomewhere/else/Stray.ts\n"), nil)
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	var stderr bytes.Buffer
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &stderr})
	ctx := statemachine.NewContext()
	// declared scopes covers dsl-core but not "somewhere/else".
	ctx.Params["scopes"] = "dsl-core,driver-port"
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
}

func TestValidateOutputsAndScopes_AllClean_IsValid(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "diff", "--name-only", "HEAD"},
		[]byte("dsl/typescript/src/core/Logic.ts\ndriver/typescript/src/port/Port.ts\n"), nil)
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["outputs"] = "dsl-port-changed"
	ctx.Params["scopes"] = "dsl-core,driver-port"
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
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// modifiedPathsSinceHead is not consulted on the missing-output path,
	// but if it were, this would be a scope-diff candidate.
	git.on([]string{"-C", repoPath, "diff", "--name-only", "HEAD"},
		[]byte("somewhere/else/Stray.ts\n"), nil)
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["outputs"] = "dsl-port-changed"
	ctx.Params["scopes"] = "dsl-core"
	// dsl-port-changed not set in state.
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("failure-kind"); got != "missing-output" {
		t.Fatalf("failure-kind: got %q, want %q (missing-output must win)", got, "missing-output")
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
func (f *fakeTracker) MarkChecklistComplete(context.Context, tracker.Issue) error {
	panic("fakeTracker.MarkChecklistComplete: not implemented")
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
// checkChecklistProgress — partially-done detection + summary
// ---------------------------------------------------------------------------

func TestCheckChecklistProgress_AllUnchecked_NotPartial(t *testing.T) {
	a := newActions(Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_checklist", "- [ ] One\n- [ ] Two")

	if out := a.checkChecklistProgress(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got, _ := ctx.State["checklist-partially-done"].(bool); got {
		t.Errorf("checklist-partially-done: got true, want false")
	}
	if got := ctx.Params["checklist_progress_summary"]; got != "" {
		t.Errorf("summary: got %q, want empty when nothing checked", got)
	}
}

func TestCheckChecklistProgress_SomeChecked_IsPartial(t *testing.T) {
	a := newActions(Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_checklist", "- [x] Done\n- [ ] Pending")

	if out := a.checkChecklistProgress(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got, _ := ctx.State["checklist-partially-done"].(bool); !got {
		t.Errorf("checklist-partially-done: got false, want true")
	}
	got := ctx.Params["checklist_progress_summary"]
	if !strings.Contains(got, "1 of 2") {
		t.Errorf("summary should report 1 of 2: got %q", got)
	}
}

func TestCheckChecklistProgress_EmptyChecklist_NotPartial(t *testing.T) {
	a := newActions(Deps{})
	ctx := statemachine.NewContext()
	// No ticket_checklist set — story or bug or legacy-coverage path
	// where the cycle is never reached, but defensive coverage.

	if out := a.checkChecklistProgress(ctx); out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if got, _ := ctx.State["checklist-partially-done"].(bool); got {
		t.Errorf("checklist-partially-done: got true, want false")
	}
}
