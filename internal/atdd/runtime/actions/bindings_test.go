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
	"reflect"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd"
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
		"check-phase-scope",
		"run-command",
		"validate-outputs-and-scopes",
		"snapshot-working-tree",
		"identify-external-system",
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
		{"system-test/typescript/tests/latest/acceptance", true}, // exact match
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

// TestResolveLayerPaths_FamilyAKeysAllHaveAccessor is the drift guard
// between atdd.FamilyAPathKeysInScope and the switch in ResolveLayerPaths.
// Every key admitted to the allowlist must also be wired into the switch
// — otherwise the resolver returns "has no Config accessor" at runtime
// the first time a writing-agent MID references the layer, 4+ minutes
// into a ticket run.
func TestResolveLayerPaths_FamilyAKeysAllHaveAccessor(t *testing.T) {
	cfg := writePhaseScopeTestConfig(t, t.TempDir())
	for key := range atdd.FamilyAPathKeysInScope {
		got, err := ResolveLayerPaths([]string{key}, cfg)
		if err != nil {
			t.Errorf("ResolveLayerPaths(%q): %v — add a switch case for this key in ResolveLayerPaths (it is in FamilyAPathKeysInScope but has no Config accessor)", key, err)
			continue
		}
		if len(got) != 1 || got[0] == "" {
			t.Errorf("ResolveLayerPaths(%q): got %v, want a single non-empty path", key, got)
		}
	}
}

// TestMonolithOnlyPathKeysAreInScope is the drift guard between
// atdd.MonolithOnlyPathKeys and atdd.FamilyAPathKeysInScope: the skip
// gate in ResolveLayerPaths only fires for layers it already knows how to
// resolve, so every monolith-only key must also be an admitted Family A
// scope key.
func TestMonolithOnlyPathKeysAreInScope(t *testing.T) {
	for key := range atdd.MonolithOnlyPathKeys {
		if !atdd.FamilyAPathKeysInScope[key] {
			t.Errorf("MonolithOnlyPathKeys[%q] is not in FamilyAPathKeysInScope", key)
		}
	}
}

// TestResolveLayerPaths_MonolithOnlyKeysSkippedOnMultitier covers the
// architecture polymorphism: on a multitier config system.path is empty
// by construction, so the monolith-only system-path layer is not
// applicable and is dropped from the resolved scope rather than surfaced
// as a phantom "resolves to empty system.path" failure (the 6 multitier
// preflight failures this fix targets). Layers that DO apply to multitier
// still resolve alongside it.
func TestResolveLayerPaths_MonolithOnlyKeysSkippedOnMultitier(t *testing.T) {
	cfg := &projectconfig.Config{}
	cfg.System.Architecture = projectconfig.ArchMultitier // system.path left empty, as multitier configs do
	cfg.SystemTest.Paths = map[string]string{"at-test": "system-test/typescript/tests/acceptance"}

	t.Run("system-path alone resolves to no paths, no error", func(t *testing.T) {
		got, err := ResolveLayerPaths([]string{"system-path"}, cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %v, want empty (system-path not applicable on multitier)", got)
		}
	})

	t.Run("applicable layers still resolve alongside the skipped system-path", func(t *testing.T) {
		got, err := ResolveLayerPaths([]string{"at-test", "system-path"}, cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := []string{"system-test/typescript/tests/acceptance"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
}

// TestResolveLayerPaths_EmptySystemPathStillErrorsOnMonolith guards the
// other half of the polymorphism: on a monolith config an empty
// system.path is genuine misconfiguration and must still fail loudly —
// the architecture-aware skip must not weaken drift detection where the
// key actually applies.
func TestResolveLayerPaths_EmptySystemPathStillErrorsOnMonolith(t *testing.T) {
	cfg := &projectconfig.Config{}
	cfg.System.Architecture = projectconfig.ArchMonolith // system.path required but left empty
	if _, err := ResolveLayerPaths([]string{"system-path"}, cfg); err == nil {
		t.Fatal("want error for empty system.path on monolith, got nil")
	}
}

// TestNarrowAdapterScopeByChannel covers the per-channel write-scope
// narrowing the shared implement-system-driver-adapters node relies on:
// (a) a channel param replaces the whole-layer system-driver-adapter
// entry with that channel's configured member; (b) no channel param
// leaves the resolved scope untouched; (c) a channel with no configured
// member is a hard error, never a silent widen to the whole layer.
// Fixture shape mirrors scoped_test.go (SystemDriverAdapterChannels
// populated per channel).
func TestNarrowAdapterScopeByChannel(t *testing.T) {
	cfg := &projectconfig.Config{}
	cfg.SystemTest.SystemDriverAdapterChannels = map[string]string{
		"api": "system-test/driver/adapter/api",
		"ui":  "system-test/driver/adapter/ui",
	}
	// write and allowed are index-aligned, exactly as ResolveLayerPaths
	// produces them — system-driver-port read-only sits alongside the
	// system-driver-adapter write entry the narrowing targets.
	write := []string{"system-driver-port", "system-driver-adapter"}
	whole := []string{"driver/port", "driver/adapter"}

	t.Run("channel param narrows adapter entry to the member", func(t *testing.T) {
		got, err := narrowAdapterScopeByChannel(write, whole, "api", cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := []string{"driver/port", "system-test/driver/adapter/api"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
		// The caller's resolved slice must not be mutated in place.
		if whole[1] != "driver/adapter" {
			t.Errorf("input allowed mutated: %v", whole)
		}
	})

	t.Run("no channel param leaves the whole layer unchanged", func(t *testing.T) {
		got, err := narrowAdapterScopeByChannel(write, whole, "", cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !reflect.DeepEqual(got, whole) {
			t.Errorf("got %v, want whole layer %v", got, whole)
		}
	})

	t.Run("channel set but member missing errors, no silent widen", func(t *testing.T) {
		got, err := narrowAdapterScopeByChannel(write, whole, "grpc", cfg)
		if err == nil {
			t.Fatalf("expected error for channel with no configured member, got %v", got)
		}
		if got != nil {
			t.Errorf("on error the scope must be nil (no whole-layer widen), got %v", got)
		}
		if !strings.Contains(err.Error(), "grpc") {
			t.Errorf("error should name the offending channel: %v", err)
		}
	})

	// The gift-wrap repro: a per-channel (ui) dispatch carries both
	// system-driver-adapter (narrowed to the channel member) and
	// system-driver-adapter-shared (the test-transport foundation). The
	// narrower rewrites ONLY the exact system-driver-adapter entry, so the
	// shared entry survives untouched — a write under driver/adapter/shared
	// (e.g. PageClient.setChecked) passes scope for a channel-ui dispatch
	// without a halt.
	t.Run("shared key is never narrowed by a channel param", func(t *testing.T) {
		writeWithShared := []string{"system-driver-port", "system-driver-adapter", "system-driver-adapter-shared"}
		resolved := []string{"driver/port", "driver/adapter", "driver/adapter/shared"}
		got, err := narrowAdapterScopeByChannel(writeWithShared, resolved, "ui", cfg)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		want := []string{"driver/port", "system-test/driver/adapter/ui", "driver/adapter/shared"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v (shared entry must be untouched)", got, want)
		}
		// A gift-wrap edit under the shared foundation is in scope for the
		// narrowed ui dispatch.
		if !pathInScope("driver/adapter/shared/client/playwright/PageClient.java", got) {
			t.Errorf("a write under driver/adapter/shared must pass scope for a channel-ui dispatch; allowed=%v", got)
		}
	})
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
  db-migration-path: system/db/migrations

system-test:
  path: system-test/typescript
  repo: acme/shop
  lang: typescript
  sonar-project: acme_shop-system-test
  paths:
    at-test: system-test/typescript/tests/latest/acceptance
    dsl-port: dsl/typescript/src/port
    dsl-core: dsl/typescript/src/core
    system-driver-port: driver/typescript/src/port
    system-driver-adapter: driver/typescript/src/adapter
    ct-test: system-test/typescript/tests/latest/contract
    external-system-driver-port: driver/typescript/src/external-port
    external-system-driver-adapter: driver/typescript/src/external-adapter
    system-driver-adapter-shared: driver/typescript/src/adapter/shared
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
	if out.Err == nil || !strings.Contains(out.Err.Error(), "phase-id") {
		t.Fatalf("expected phase-id error, got %v", out.Err)
	}
}

func TestCheckPhaseScope_UnknownPhaseIsHardError(t *testing.T) {
	a := newActions(Deps{Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["phase-id"] = "nonexistent-phase"
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
	ctx.Params["phase-id"] = "write-acceptance-tests"
	out := a.checkPhaseScope(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get(CtxKeyPhaseScopeClean); got != true {
		t.Fatalf("phase-scope-clean: got %v, want true", got)
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
	// system-driver-port edit is outside scope. HEAD-equivalent fallback (no
	// snapshot pre-seeded), so checkPhaseScope enumerates the full dirty
	// tree via `git status`.
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(" M driver/typescript/src/port/Driver.ts\n M system-test/typescript/tests/latest/acceptance/foo.spec.ts\n"), nil)
	var stderr bytes.Buffer
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &stderr, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["phase-id"] = "write-acceptance-tests"
	out := a.checkPhaseScope(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get(CtxKeyPhaseScopeClean); got != false {
		t.Fatalf("phase-scope-clean: got %v, want false", got)
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
	// implement-dsl scope: dsl-core, system-driver-port, external-system-driver-port.
	ctx.Params["phase-id"] = "implement-dsl"
	out := a.checkPhaseScope(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get(CtxKeyPhaseScopeClean); got != false {
		t.Fatalf("phase-scope-clean: got %v, want false (rename target is outside scope)", got)
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

// envCapturingShell records the value of a named env var at the moment
// Run is invoked, so a test can assert what the child shell-out would
// inherit from the orchestrator process.
type envCapturingShell struct {
	envVar   string
	captured *string
}

func (s *envCapturingShell) Run(_ context.Context, _ string) (ShellResult, error) {
	*s.captured = os.Getenv(s.envVar)
	return ShellResult{Stdout: []byte("OK")}, nil
}

// TestRunCommand_TestRunLiftsWipGate pins the env-var gating mechanism:
// a `gh optivem test run` dispatch sets GH_OPTIVEM_RUN_WIP_TESTS=1 for
// the duration of the shell-out (so the child runner and its mvn /
// dotnet / playwright invocation inherit it and the WIP acceptance
// tests run), then restores the prior state so the var never leaks into
// a later non-test dispatch in the same process.
func TestRunCommand_TestRunLiftsWipGate(t *testing.T) {
	os.Unsetenv(wipTestsEnvVar)
	var during string
	sh := &envCapturingShell{envVar: wipTestsEnvVar, captured: &during}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem test run"
	a.runCommand(ctx)
	if during != "1" {
		t.Errorf("during test-run dispatch: %s = %q, want %q", wipTestsEnvVar, during, "1")
	}
	if v, had := os.LookupEnv(wipTestsEnvVar); had {
		t.Errorf("after dispatch: %s still set to %q, want unset (no leak)", wipTestsEnvVar, v)
	}
}

// TestRunCommand_NonTestRunLeavesWipGateUnset is the negative-space
// counterpart: a non-test dispatch must not set the gate var, so an
// operator/CI/IDE-style `gh optivem` shell-out stays unaffected.
func TestRunCommand_NonTestRunLeavesWipGateUnset(t *testing.T) {
	os.Unsetenv(wipTestsEnvVar)
	var during string
	sh := &envCapturingShell{envVar: wipTestsEnvVar, captured: &during}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem compile"
	a.runCommand(ctx)
	if during != "" {
		t.Errorf("non-test dispatch set %s = %q, want empty", wipTestsEnvVar, during)
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

// TestRunCommand_RunTestsClassifiesInfraFailure exercises the wiring
// of verify_classify.classifyShellErr into runCommand. When the
// test-runner shell-out fails with stderr matching one of the infra
// patterns ("is not recognized", "command not found", docker daemon
// unreachable, etc.), runCommand must stamp test-outcome="infra" and
// surface the matching label under test-infra-label — overriding the
// default "fail" stamp set on every non-zero exit. The infra branch
// is what TESTS_INFRA_HALT routes off in the verify-tests-* processes,
// so the pre-classifier behaviour of treating runner-not-started as
// "test red" (silently advancing verify-tests-fail) is what this test
// guards against.
func TestRunCommand_RunTestsClassifiesInfraFailure(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stderr    string
		wantLabel string
	}{
		{
			name:      "windows cmd not recognized (the #71 rehearsal stderr)",
			stderr:    "'C:\\Program' is not recognized as an internal or external command,\noperable program or batch file.",
			wantLabel: "missing executable",
		},
		{
			name:      "bash command not found",
			stderr:    "bash: npx: command not found",
			wantLabel: "missing executable",
		},
		{
			name:      "docker daemon unreachable",
			stderr:    "error during connect: this error may indicate that the docker daemon is not running",
			wantLabel: "docker daemon unreachable",
		},
		{
			// The suite/test-names filter selected zero tests (the shop #72
			// rehearsal: a contract method under --suite=acceptance matched
			// nothing). Gradle's "No tests found …" arrives in result.Stderr
			// via the runner's wrapped error tail; classifying it infra (not
			// the default "fail") is what stops a fail-expecting verify from
			// greening without exercising a single test (plan 20260608-1240).
			name:      "empty test selection (gradle no tests found)",
			stderr:    "No tests found for given includes: [com.example.AcceptanceTest.method](filter.includeTestsMatching)",
			wantLabel: "empty test selection",
		},
		{
			// The runner's own zero-executed guard (plan 20260608-1502) catches
			// dotnet, which exits 0 on a zero-match --filter — so the per-tool
			// "no tests …" patterns never fire. RunTests fails the run with this
			// marker instead, dropping the empty case into the same infra halt.
			name:      "empty test selection (runner zero-executed marker)",
			stderr:    "Error: 0 tests executed for the given selection — the suite/test filter matched nothing on any selected suite",
			wantLabel: "empty test selection",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := &fakeShell{stderr: []byte(tc.stderr), exitCode: 1, err: errors.New("exit 1")}
			var stderr bytes.Buffer
			a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &stderr})
			ctx := statemachine.NewContext()
			ctx.Params["command"] = "gh optivem test run"
			out := a.runCommand(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if got := ctx.GetString("test-outcome"); got != "infra" {
				t.Fatalf("test-outcome: got %q, want \"infra\" (classifier should override the default \"fail\")", got)
			}
			if got := ctx.GetString("test-infra-label"); got != tc.wantLabel {
				t.Fatalf("test-infra-label: got %q, want %q", got, tc.wantLabel)
			}
			// verify_failure_output still gets stamped so the halt banner /
			// trace can quote the runner output.
			if got := ctx.GetString("verify_failure_output"); got == "" {
				t.Fatalf("verify_failure_output should still be stamped on infra failure")
			}
		})
	}
}

// TestRunCommand_RunTestsRedFailureStaysFail is the negative-space
// guard for the infra wiring: a shell-out failure whose stderr does
// NOT match any infra pattern must remain test-outcome="fail", not
// drift to "infra". Otherwise every actual red test would halt the
// pipeline instead of routing to fix-unexpected-failing-tests.
func TestRunCommand_RunTestsRedFailureStaysFail(t *testing.T) {
	sh := &fakeShell{
		stderr:   []byte("Expected 'gift-wrapped: true' but got 'gift-wrapped: false'\nat tests/gift-wrap.spec.ts:42"),
		exitCode: 1,
		err:      errors.New("exit 1"),
	}
	var stderr bytes.Buffer
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &stderr})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem test run"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("test-outcome"); got != "fail" {
		t.Fatalf("test-outcome: got %q, want \"fail\" (a real assertion failure must not be classified as infra)", got)
	}
	if _, set := ctx.State["test-infra-label"]; set {
		t.Fatalf("test-infra-label should not be set on a non-infra failure: got %v", ctx.Get("test-infra-label"))
	}
}

func TestRunCommand_SuiteAndTestFlagsAppendedOnlyWhenSet(t *testing.T) {
	cases := []struct {
		name      string
		suite     string
		testNames string
		wantHas   []string
		wantMiss  []string
	}{
		{
			name:     "both unset",
			wantMiss: []string{"--suite=", "--test="},
		},
		{
			name:     "suite only",
			suite:    "acceptance",
			wantHas:  []string{"--suite=acceptance"},
			wantMiss: []string{"--test="},
		},
		{
			name:      "test-names only",
			testNames: "foo,bar",
			wantHas:   []string{"--test=foo,bar"},
			wantMiss:  []string{"--suite="},
		},
		{
			name:      "both set",
			suite:     "acceptance",
			testNames: "foo,bar",
			wantHas:   []string{"--suite=acceptance", "--test=foo,bar"},
		},
		{
			name:      "test-name with whitespace is shell-quoted",
			suite:     "acceptance",
			testNames: "shouldHandle whitespace",
			wantHas:   []string{"--suite=acceptance", "'shouldHandle whitespace'"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := &fakeShell{out: []byte("OK")}
			a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			ctx := statemachine.NewContext()
			ctx.Params["command"] = "gh optivem test run"
			if tc.suite != "" {
				ctx.Params["suite"] = tc.suite
			}
			if tc.testNames != "" {
				ctx.Params["test-names"] = tc.testNames
			}
			out := a.runCommand(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if len(sh.calls) != 1 {
				t.Fatalf("expected 1 shell call, got %d: %v", len(sh.calls), sh.calls)
			}
			for _, want := range tc.wantHas {
				if !strings.Contains(sh.calls[0], want) {
					t.Errorf("shell call missing %q: %q", want, sh.calls[0])
				}
			}
			for _, miss := range tc.wantMiss {
				if strings.Contains(sh.calls[0], miss) {
					t.Errorf("shell call should not contain %q: %q", miss, sh.calls[0])
				}
			}
		})
	}
}

func TestRunCommand_NoFilterFlagsForNonTestCommand(t *testing.T) {
	// Covers two related cases:
	//   (a) suite / test-names unset — flags must not appear (baseline).
	//   (b) suite / test-names SET, but command is not `gh optivem test run`
	//       — flags must STILL not appear. Caller-scope inheritance
	//       (run.go:168-180) propagates outer `suite`/`test-names`
	//       bindings into every nested call-activity; without this guard
	//       `gh optivem system build` and `gh optivem commit` receive
	//       `--suite=…`/`--test=…` and the CLI rejects them.
	cases := []struct {
		name      string
		command   string
		suite     string
		testNames string
	}{
		{name: "commit, both unset", command: "gh optivem commit"},
		{name: "system build, both unset", command: "gh optivem system build"},
		{
			name:      "system build with inherited suite + test-names",
			command:   "gh optivem system build",
			suite:     "acceptance",
			testNames: "shouldRejectOrderWithQuantityOf100",
		},
		{
			name:      "commit with inherited suite + test-names",
			command:   "gh optivem commit",
			suite:     "acceptance",
			testNames: "foo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sh := &fakeShell{out: []byte("OK")}
			a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
			ctx := statemachine.NewContext()
			ctx.Params["command"] = tc.command
			if tc.suite != "" {
				ctx.Params["suite"] = tc.suite
			}
			if tc.testNames != "" {
				ctx.Params["test-names"] = tc.testNames
			}
			out := a.runCommand(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if strings.Contains(sh.calls[0], "--suite=") || strings.Contains(sh.calls[0], "--test=") {
				t.Fatalf("non-test-run shell call should not carry suite/test flags: %q", sh.calls[0])
			}
		})
	}
}

func TestRunCommand_CommitAppendsMessageAsPositional(t *testing.T) {
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem commit"
	ctx.Params["message"] = "[69] Add product search"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	want := `gh optivem commit '[69] Add product search'`
	if sh.calls[0] != want {
		t.Fatalf("shell call: got %q, want %q", sh.calls[0], want)
	}
}

func TestRunCommand_CommitMessageEscapesShellMetacharacters(t *testing.T) {
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem commit"
	ctx.Params["message"] = "msg with 'quote' and $var"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	// shellEscape wraps in single quotes and escapes embedded single quotes
	// as '\'' — the result is bash-safe regardless of $var / backtick / etc.
	want := `gh optivem commit 'msg with '\''quote'\'' and $var'`
	if sh.calls[0] != want {
		t.Fatalf("shell call: got %q, want %q", sh.calls[0], want)
	}
}

func TestRunCommand_CommitWithoutMessageDoesNotSplice(t *testing.T) {
	// Strict-mode YAML wiring blocks unbound `${message}` upstream of this
	// action; at the action layer, an absent `message` param produces a
	// bare `gh optivem commit` command line (matches the existing failure-
	// routing fixture at TestRunCommand_RouteFailureViaState).
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem commit"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if sh.calls[0] != "gh optivem commit" {
		t.Fatalf("shell call: got %q, want %q", sh.calls[0], "gh optivem commit")
	}
}

func TestRunCommand_MessageIgnoredForNonCommitCommand(t *testing.T) {
	// The isCommit prefix guard means a stray `message:` binding on a
	// non-commit command is a silent no-op, matching the suite/test-names
	// non-test-run behaviour.
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem test run"
	ctx.Params["message"] = "should not appear"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if strings.Contains(sh.calls[0], "should not appear") {
		t.Fatalf("shell call should not contain message for non-commit command: %q", sh.calls[0])
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

func TestRunCommand_TestRunFailure_StampsVerifyFailureOutput(t *testing.T) {
	// On the isTestRun && !succeeded branch, runCommand stashes the
	// captured stdout/stderr tails into verify_failure_output so the
	// downstream fix-unexpected-{failing,passing}-tests prompt's
	// ${verify-failure-output} placeholder renders the runner output the
	// operator saw inline. Both streams are individually capped by
	// lastNLines(s, commandStderrTailLines).
	var stdoutLines, stderrLines strings.Builder
	for i := 0; i < commandStderrTailLines+5; i++ {
		fmt.Fprintf(&stdoutLines, "stdout line %d\n", i+1)
		fmt.Fprintf(&stderrLines, "stderr line %d\n", i+1)
	}
	sh := &fakeShell{
		out:      []byte(stdoutLines.String()),
		stderr:   []byte(stderrLines.String()),
		exitCode: 1,
		err:      errors.New("exit 1"),
	}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem test run"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	got := ctx.GetString("verify_failure_output")
	if got == "" {
		t.Fatalf("verify_failure_output must be stamped on test-run failure")
	}
	// Both streams must be present, separated by the documented marker.
	if !strings.Contains(got, "--- stderr ---") {
		t.Fatalf("verify_failure_output missing stderr separator:\n%s", got)
	}
	parts := strings.SplitN(got, "\n--- stderr ---\n", 2)
	if len(parts) != 2 {
		t.Fatalf("verify_failure_output not split into stdout/stderr sections:\n%s", got)
	}
	stdoutTailLines := strings.Split(parts[0], "\n")
	stderrTailLines := strings.Split(parts[1], "\n")
	if len(stdoutTailLines) != commandStderrTailLines {
		t.Errorf("stdout tail lines: got %d, want %d", len(stdoutTailLines), commandStderrTailLines)
	}
	if len(stderrTailLines) != commandStderrTailLines {
		t.Errorf("stderr tail lines: got %d, want %d", len(stderrTailLines), commandStderrTailLines)
	}
	// Each tail should preserve the trailing window — last entries reach
	// the max line we wrote.
	if stdoutTailLines[len(stdoutTailLines)-1] != fmt.Sprintf("stdout line %d", commandStderrTailLines+5) {
		t.Errorf("stdout tail last line: got %q", stdoutTailLines[len(stdoutTailLines)-1])
	}
	if stderrTailLines[len(stderrTailLines)-1] != fmt.Sprintf("stderr line %d", commandStderrTailLines+5) {
		t.Errorf("stderr tail last line: got %q", stderrTailLines[len(stderrTailLines)-1])
	}
}

func TestRunCommand_TestRunSuccess_DoesNotStampVerifyFailureOutput(t *testing.T) {
	// A clean test-run must NOT stash verify_failure_output so a later
	// fix dispatch via an unrelated failure-kind cannot inherit a stale
	// value. The placeholder is failure-only.
	sh := &fakeShell{out: []byte("PASS: 12 tests"), stderr: nil}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem test run"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if _, set := ctx.State["verify_failure_output"]; set {
		t.Fatalf("verify_failure_output must NOT be set on test-run success: got %q", ctx.GetString("verify_failure_output"))
	}
}

func TestRunCommand_NonTestRunFailure_DoesNotStampVerifyFailureOutput(t *testing.T) {
	// A non-test-run failure (e.g. `gh optivem commit` exit 7) routes
	// through the command-failed payload (command-line / command-exit-code
	// / command-stderr-tail) — verify_failure_output is fixer-only, so
	// non-test-run dispatches must not register a placeholder the
	// fix-command-failed prompt body doesn't reference.
	sh := &fakeShell{
		stderr:   []byte("commit refused"),
		exitCode: 7,
		err:      errors.New("exit 7"),
	}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem commit"
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if _, set := ctx.State["verify_failure_output"]; set {
		t.Fatalf("verify_failure_output must NOT be set on non-test-run failure: got %q", ctx.GetString("verify_failure_output"))
	}
	// Sanity: the command-failed payload IS set (so the test is exercising
	// a real failure, not a no-op).
	if got := ctx.GetString("failure-kind"); got != "command-failed" {
		t.Fatalf("failure-kind: got %q, want %q", got, "command-failed")
	}
}

func TestRunCommand_Success_ClearsPriorFailureDiagnostics(t *testing.T) {
	// Within a single run, ctx.State persists across call-activities (the
	// state-fallback path documented at run.go:308). If a prior dispatch
	// failed and stamped failure-kind / command-* / verify_failure_output,
	// a later success on the same producer must wipe those keys — otherwise
	// the trace's state-delta hoists stale values onto the success banner
	// and a downstream fix-* dispatch's ExpandParams substitutes them into
	// a recovery prompt for a failure that's already been resolved.
	sh := &fakeShell{out: []byte("OK")}
	a := newActions(Deps{Shell: sh, Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}})
	ctx := statemachine.NewContext()
	ctx.Params["command"] = "gh optivem compile"
	// Seed every diagnostic key runCommand owns, simulating residue from
	// a prior failed dispatch in the same run.
	ctx.Set("failure-kind", "command-failed")
	ctx.Set("command-line", "previous failing cmd")
	ctx.Set("command-exit-code", 42)
	ctx.Set("command-stderr-tail", "old stderr blob")
	ctx.Set("verify_failure_output", "old verify blob")
	out := a.runCommand(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	for _, k := range []string{
		"failure-kind",
		"command-line",
		"command-exit-code",
		"command-stderr-tail",
		"verify_failure_output",
	} {
		if _, set := ctx.State[k]; set {
			t.Errorf("%s must be cleared on success: still set to %v", k, ctx.Get(k))
		}
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
	// implement-dsl declares two required keys in BPMN:
	// system-driver-port-changed + external-driver-port-changed.
	ctx.Params["task-name"] = "implement-dsl"
	// Only one of the two required outputs is present in state.
	ctx.Set("system-driver-port-changed", true)
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
	if !strings.Contains(stderr.String(), "external-driver-port-changed") {
		t.Fatalf("stderr missing output name: %q", stderr.String())
	}
}

func TestValidateOutputsAndScopes_OutputsPresent_NoScope_IsValid(t *testing.T) {
	// A task-name with no outputs declared and no inline scope (e.g.
	// refine-acceptance-criteria) is the trivial-pass path: nothing to
	// check, validation succeeds.
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
	// implement-dsl scope (write list) covers dsl-core, system-driver-port,
	// external-system-driver-port — but not "somewhere/else".
	ctx.Params["task-name"] = "implement-dsl"
	// Required outputs must be present for the scope check to fire (the
	// new presence-check from BPMN OutputSpec runs first; missing keys
	// short-circuit to failure-kind=missing-output, not scope-diff).
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed", false)
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
	// so fix-scope-diff's ${changed-files} sees what the agent did.
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
	// Both required outputs present so the presence-check passes.
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
	// phase-changed-files is stashed on every dispatch (per plan
	// 20260527-1536), including the all-clean success path. Empty delta
	// → empty string, but the key must be present so a downstream
	// fix-unexpected-{failing,passing}-tests dispatch (on the
	// verify-tests-fail branch) reads from the stash, not the
	// live git-status fallback.
	if _, set := ctx.State["phase-changed-files"]; !set {
		t.Errorf("phase-changed-files must be stashed on the success path (empty delta = empty string), key missing entirely")
	}
	if got := ctx.GetString("phase-changed-files"); got != "" {
		t.Errorf("phase-changed-files on empty delta: got %q, want %q", got, "")
	}
}

// CT-path System-Driver fence (plan 20260527-1147 Item 4). A dsl-implementer
// dispatched with test-category=contract that emits system-driver-port-changed=true
// must HALT — the flag would otherwise leak up into the AT cycle's
// system-driver-adapter gate. The fence is a hard Outcome.Err (structural
// invariant, no fix-* recovery), and it fires before the presence/scope
// checks, so no git/config/fingerprint wiring is needed to trip it.
func TestValidateOutputsAndScopes_CTPath_SystemDriverPortChanged_Halts(t *testing.T) {
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["test-category"] = "contract"
	// On the contract cascade the verdict lands namespaced (plan 20260606-1525);
	// the fence reads the ct- key.
	ctx.Set("ct-system-driver-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err == nil {
		t.Fatalf("CT-path dsl-implementer emitting system-driver-port-changed=true must halt, got nil err")
	}
	if !strings.Contains(out.Err.Error(), "System Driver port") {
		t.Fatalf("diagnostic should name the System Driver port: %q", out.Err.Error())
	}
}

// The AT path (test-category=acceptance) emitting system-driver-port-changed=true
// is correct and must pass cleanly — the fence is CT-only.
func TestValidateOutputsAndScopes_ATPath_SystemDriverPortChanged_Allowed(t *testing.T) {
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["test-category"] = "acceptance"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	// On the acceptance cascade the verdicts land namespaced (plan 20260606-1525).
	ctx.Set("at-system-driver-port-changed", true)
	ctx.Set("at-external-driver-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("AT-path system-driver-port-changed=true must be allowed, got err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
}

// landingStateKey namespaces the port-changed verdicts AND test-names by
// cascade so the nested contract excursion can't clobber the acceptance
// cascade's value (plan 20260606-1525; test-names added by plan 20260608-1231).
// Every other output, and any unrecognised cascade, is the identity.
func TestLandingStateKey_CascadeNamespacing(t *testing.T) {
	cases := []struct {
		key, testCategory, want string
	}{
		{"dsl-port-changed", "acceptance", "at-dsl-port-changed"},
		{"dsl-port-changed", "contract", "ct-dsl-port-changed"},
		{"system-driver-port-changed", "acceptance", "at-system-driver-port-changed"},
		{"system-driver-port-changed", "contract", "ct-system-driver-port-changed"},
		{"external-driver-port-changed", "acceptance", "at-external-driver-port-changed"},
		{"external-driver-port-changed", "contract", "ct-external-driver-port-changed"},
		// test-names is namespaced too (plan 20260608-1231).
		{"test-names", "acceptance", "at-test-names"},
		{"test-names", "contract", "ct-test-names"},
		// Genuinely non-namespaced outputs are the identity.
		{"scope-exception-reason", "contract", "scope-exception-reason"},
		// Unrecognised / absent cascade falls back to the bare key (the
		// downstream gate's strict "not set" check surfaces the wiring bug).
		{"dsl-port-changed", "", "dsl-port-changed"},
		{"system-driver-port-changed", "nonsense", "system-driver-port-changed"},
	}
	for _, c := range cases {
		if got := landingStateKey(c.key, c.testCategory); got != c.want {
			t.Errorf("landingStateKey(%q, %q) = %q, want %q", c.key, c.testCategory, got, c.want)
		}
	}
}

// The anti-clobber property at the landing layer: a port-changed verdict
// flattened on the acceptance cascade lands under its `at-` key and NOT the
// bare key, so a subsequent contract excursion (which writes only `ct-*`)
// leaves it intact for the parent re-gate (plan 20260606-1525).
func TestValidateOutputsAndScopes_PortChangedVerdicts_CascadeNamespaced(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-dsl-implementer.outputs.jsonl")
	writeJSONL(t, jsonl, `{"system-driver-port-changed":true,"external-driver-port-changed":false}`)

	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"}, []byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["test-category"] = "acceptance"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.State["output-file-path"] = jsonl

	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("at-system-driver-port-changed"); got != true {
		t.Errorf("at-system-driver-port-changed: got %v, want true", got)
	}
	if got := ctx.Get("at-external-driver-port-changed"); got != false {
		t.Errorf("at-external-driver-port-changed: got %v, want false", got)
	}
	// The bare keys must NOT be written — that is the whole point of the
	// namespacing (a CT excursion writing bare/ct- can't touch at-*).
	if _, set := ctx.State["system-driver-port-changed"]; set {
		t.Errorf("bare system-driver-port-changed must not be set; namespacing is single-backend")
	}
	if _, set := ctx.State["external-driver-port-changed"]; set {
		t.Errorf("bare external-driver-port-changed must not be set; namespacing is single-backend")
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
}

func TestValidateOutputsAndScopes_Success_ClearsPriorFailureDiagnostics(t *testing.T) {
	// Within a single run, ctx.State persists across call-activities (the
	// state-fallback path documented at run.go:308). If a prior dispatch
	// failed and stamped failure-kind / failing-task-name / missing-outputs /
	// scope-violating-paths, a later success on this producer must wipe
	// those keys — otherwise the trace's state-delta hoists stale values
	// onto the success banner and a downstream fix-* dispatch's
	// ExpandParams substitutes them into a recovery prompt for a failure
	// that's already been resolved.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed", true)
	// Seed every diagnostic key validateOutputsAndScopes owns, simulating
	// residue from a prior failed dispatch in the same run.
	ctx.Set("failure-kind", "missing-output")
	ctx.Set("failing-task-name", "some-prior-task")
	ctx.Set("missing-outputs", "old,missing,keys")
	ctx.Set("scope-violating-paths", "old/stray/path.ts")
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
	for _, k := range []string{
		"failure-kind",
		"failing-task-name",
		"missing-outputs",
		"scope-violating-paths",
	} {
		if _, set := ctx.State[k]; set {
			t.Errorf("%s must be cleared on success: still set to %v", k, ctx.Get(k))
		}
	}
}

func TestValidateOutputsAndScopes_AllInScope_StashesChangedFiles(t *testing.T) {
	// Validation passes (every modified path is in scope), but the delta
	// is non-empty. The phase-changed-files stash must capture it so the
	// downstream fix-unexpected-{failing,passing}-tests dispatch on a
	// later verify-tests-fail can read it without re-shelling
	// `git status --porcelain`.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	// One in-scope dirty path (dsl-core).
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? dsl/typescript/src/core/Logic.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed", true)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true (all in scope)", got)
	}
	wantChanged := "dsl/typescript/src/core/Logic.ts"
	if got := ctx.GetString("phase-changed-files"); got != wantChanged {
		t.Errorf("phase-changed-files: got %q, want %q", got, wantChanged)
	}
}

func TestValidateOutputsAndScopes_ExternalDriverPortChange_PreservesPortPaths(t *testing.T) {
	// Plan 20260613-1835: on a true external-driver-port-changed verdict, the
	// under-port-root subset of the delta is preserved into
	// external-driver-port-changed-paths so IDENTIFY_EXTERNAL_SYSTEM resolves the
	// system name SOLELY from the port change — covering a DTO-only change that
	// writes ZERO adapter files (the #65 crash). Only the port-root path is
	// preserved; the unrelated in-scope dsl-core edit is excluded.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? driver/typescript/src/external-port/warehouse/dtos/ReturnsProductRequest.ts\n?? dsl/typescript/src/core/Logic.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["test-category"] = "acceptance"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	// AT-cascade landed verdict (the meaningful port change).
	ctx.Set("at-external-driver-port-changed", true)
	ctx.Set("at-system-driver-port-changed", false)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	want := "driver/typescript/src/external-port/warehouse/dtos/ReturnsProductRequest.ts"
	if got := ctx.GetString("external-driver-port-changed-paths"); got != want {
		t.Errorf("external-driver-port-changed-paths: got %q, want %q (only the under-port-root subset)", got, want)
	}
}

func TestValidateOutputsAndScopes_NoPortChange_DoesNotPreservePortPaths(t *testing.T) {
	// The guard: when the external-driver-port-changed verdict is false, the
	// preserved key must NOT be written — so a compile-only CT DSL phase (which
	// does not re-change the port) cannot clobber the AT phase's list.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"},
		[]byte("?? dsl/typescript/src/core/Logic.ts\n"), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "implement-dsl"
	ctx.Params["test-category"] = "acceptance"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.Set("at-external-driver-port-changed", false)
	ctx.Set("at-system-driver-port-changed", false)
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if _, set := ctx.State["external-driver-port-changed-paths"]; set {
		t.Errorf("external-driver-port-changed-paths must not be set when the verdict is false")
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
	// No outputs set — system-driver-port-changed +
	// external-driver-port-changed are both missing per implement-dsl's
	// BPMN declaration, so missing-output must fire before any scope check.
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
	// Outputs declared on the originating MID (implement-dsl) must be
	// present so the presence-check clears and the scope-diff branch
	// can fire.
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed", false)
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
	// Both required outputs present so the presence-check clears and the
	// scope check (which depends on the snapshot) is what fires next.
	ctx.Set("system-driver-port-changed", true)
	ctx.Set("external-driver-port-changed", true)
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
// validate-outputs-and-scopes: JSONL output channel (plan 20260526-2118)
// ---------------------------------------------------------------------------

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestValidateOutputsAndScopes_JSONL_FlattenedIntoState(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-acceptance-test-writer.outputs.jsonl")
	writeJSONL(t, jsonl,
		`{"dsl-port-changed":true,"test-names":["shouldA","shouldB"]}`,
	)
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// write-acceptance-tests has scope read/write declared but the test
	// fixture doesn't seed gh-optivem.yaml or the snapshot, so we use
	// refine-acceptance-criteria's no-scope path. That MID has no
	// outputs declared either — but the reader still flattens whatever
	// the JSONL holds (the presence-check is empty, so anything
	// undeclared passes through as-is).
	//
	// To exercise the full flatten + presence-check happy path we point
	// at write-acceptance-tests but use a no-scope task by overriding
	// outputs-source: we set the engine entry indirectly. Instead, the
	// cleanest setup uses refine-acceptance-criteria (no presence-check
	// pressure) and verifies the flatten worked end-to-end.
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.State["output-file-path"] = jsonl
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("dsl-port-changed"); got != true {
		t.Errorf("dsl-port-changed: got %v, want true", got)
	}
	gotNames, ok := ctx.Get("test-names").([]any)
	if !ok {
		// declared-type-aware coercion only applies to keys in the MID's
		// outputs list. refine-acceptance-criteria has none, so the
		// reader passes []any through. That's acceptable for this
		// fixture; a declared key gets the typed shape, which the next
		// test covers.
		if v := ctx.Get("test-names"); v == nil {
			t.Errorf("test-names: not set")
		}
		return
	}
	if len(gotNames) != 2 || gotNames[0] != "shouldA" || gotNames[1] != "shouldB" {
		t.Errorf("test-names: got %v, want [shouldA shouldB]", gotNames)
	}
}

func TestValidateOutputsAndScopes_JSONL_TypedCoercionForDeclaredKeys(t *testing.T) {
	// write-acceptance-tests declares test-names: string-list, so the
	// reader must coerce JSON `[...]` (decoded as []any) into a real
	// []string — matching the shape downstream readers (runCommand
	// --test=…, scope_exception_requested gate) already cast to.
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-acceptance-test-writer.outputs.jsonl")
	writeJSONL(t, jsonl,
		`{"dsl-port-changed":true,"test-names":["shouldA","shouldB"]}`,
	)
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// Use originating-task-name to point at write-acceptance-tests
	// without triggering its scope check (no snapshot seeded). The
	// scope check fires after the output presence-check, but write-
	// acceptance-tests' Scope returns ok=true so we need to seed
	// pre-agent-fingerprint + config. Simpler: use a fix-shape so the
	// originating MID provides outputs while the inner runs scope: none.
	//
	// Actually the validator runs presence-check via phaseTaskName,
	// then scope check via the same key. If write-acceptance-tests is
	// the task, both fire. Side-step by skipping the scope test
	// requirements: set fix dispatch with originating-task-name pointing
	// at write-acceptance-tests but task-name pointing at a no-MID name
	// — phaseTaskName prefers originating-task-name, so outputs resolve
	// against write-acceptance-tests but Scope(originating-task-name)
	// returns ok=true too. We just need a Config + snapshot for the
	// scope check to clear.
	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"}, []byte(""), nil)
	a = newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx.Params["task-name"] = "write-acceptance-tests"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.State["output-file-path"] = jsonl

	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got, ok := ctx.Get("test-names").([]string); !ok {
		t.Fatalf("test-names: want []string, got %T (%v)", ctx.Get("test-names"), ctx.Get("test-names"))
	} else if len(got) != 2 || got[0] != "shouldA" || got[1] != "shouldB" {
		t.Errorf("test-names: got %v, want [shouldA shouldB]", got)
	}
	if got := ctx.Get("dsl-port-changed"); got != true {
		t.Errorf("dsl-port-changed: got %v, want true", got)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Fatalf("outputs-and-scopes-valid: got %v, want true", got)
	}
}

func TestValidateOutputsAndScopes_JSONL_EnvelopeKeysCoercedForNoOutputsMID(t *testing.T) {
	// Plan 20260528-1150: prod-agent MIDs that declare no `outputs:` block
	// (implement-system, update-system, the adapter implementers/updaters)
	// can still emit the universal scope-exception envelope via the
	// runtime-seeded channel. The reader must coerce the envelope keys
	// using the built-in envelope contract (statemachine.EnvelopeOutputSpecs),
	// so scope-exception-files lands in ctx.State as []string and the
	// scope_exception_requested gate's type-assertion succeeds — not as
	// the raw []any that JSON.Unmarshal would otherwise produce.
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-system-implementer.outputs.jsonl")
	writeJSONL(t, jsonl,
		`{"scope-exception-files":["system/db/migrations/V_x.sql"],"scope-exception-reason":"gift_wrap column required by AT"}`,
	)
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	// refine-acceptance-criteria carries no outputs and no inline scope —
	// stand-in for any no-outputs MID at the validator layer; the
	// envelope-coercion behaviour is MID-agnostic. Using refine-* skips
	// the scope check (Engine.Scope returns ok=false) so the test pins
	// only the coercer behaviour.
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.State["output-file-path"] = jsonl
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	files, ok := ctx.Get("scope-exception-files").([]string)
	if !ok {
		t.Fatalf("scope-exception-files: want []string, got %T (%v)",
			ctx.Get("scope-exception-files"), ctx.Get("scope-exception-files"))
	}
	if len(files) != 1 || files[0] != "system/db/migrations/V_x.sql" {
		t.Errorf("scope-exception-files: got %v, want [system/db/migrations/V_x.sql]", files)
	}
	if got, ok := ctx.Get("scope-exception-reason").(string); !ok {
		t.Fatalf("scope-exception-reason: want string, got %T", ctx.Get("scope-exception-reason"))
	} else if got != "gift_wrap column required by AT" {
		t.Errorf("scope-exception-reason: got %q, want %q", got, "gift_wrap column required by AT")
	}
}

func TestValidateOutputsAndScopes_JSONL_LastWriteWinsAcrossLines(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-writer.outputs.jsonl")
	writeJSONL(t, jsonl,
		`{"dsl-port-changed":false}`,
		`{"dsl-port-changed":true,"test-names":["a"]}`,
	)
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.State["output-file-path"] = jsonl
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("dsl-port-changed"); got != true {
		t.Errorf("dsl-port-changed: got %v, want true (last write wins)", got)
	}
}

func TestValidateOutputsAndScopes_JSONL_BlankLinesTolerated(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-writer.outputs.jsonl")
	writeJSONL(t, jsonl,
		``,
		`{"dsl-port-changed":true}`,
		`   `,
		``,
	)
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.State["output-file-path"] = jsonl
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("dsl-port-changed"); got != true {
		t.Errorf("dsl-port-changed: got %v, want true", got)
	}
}

func TestValidateOutputsAndScopes_JSONL_MalformedLineHardErrors(t *testing.T) {
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-writer.outputs.jsonl")
	writeJSONL(t, jsonl,
		`{"dsl-port-changed":true}`,
		`{ this is not json`,
	)
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.State["output-file-path"] = jsonl
	out := a.validateOutputsAndScopes(ctx)
	if out.Err == nil {
		t.Fatalf("expected hard error on malformed JSONL line, got nil")
	}
	if !strings.Contains(out.Err.Error(), "malformed output line") {
		t.Errorf("error should mention malformed line: %v", out.Err)
	}
}

func TestValidateOutputsAndScopes_JSONL_MissingFileIsNoOp(t *testing.T) {
	// Path stashed but the agent emitted nothing — file simply does not
	// exist. The reader treats this as an empty result; the
	// presence-check still fires for any required outputs.
	a := newActions(Deps{Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "refine-acceptance-criteria"
	ctx.State["output-file-path"] = filepath.Join(t.TempDir(), "does-not-exist.jsonl")
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Errorf("outputs-and-scopes-valid: got %v, want true (no outputs declared, missing file is fine)", got)
	}
}

func TestValidateOutputsAndScopes_JSONL_OptionalAbsenceTolerated(t *testing.T) {
	// write-acceptance-tests declares test-names + scope-exception-* as
	// optional; only dsl-port-changed is required. An emit of just the
	// required key passes validation.
	dir := t.TempDir()
	jsonl := filepath.Join(dir, "001-writer.outputs.jsonl")
	writeJSONL(t, jsonl, `{"dsl-port-changed":true}`)

	repoPath := t.TempDir()
	cfg := writePhaseScopeTestConfig(t, repoPath)
	git := newFakeRunner(t, "git")
	git.on([]string{"-C", repoPath, "status", "--porcelain"}, []byte(""), nil)
	a := newActions(Deps{Git: git, RepoPath: repoPath, Config: cfg, Stderr: &bytes.Buffer{}, Engine: loadTestEngine(t)})
	ctx := statemachine.NewContext()
	ctx.Params["task-name"] = "write-acceptance-tests"
	ctx.State[CtxKeyPreAgentFingerprint] = WorkingTreeFingerprint{}
	ctx.State["output-file-path"] = jsonl
	out := a.validateOutputsAndScopes(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.Get("outputs-and-scopes-valid"); got != true {
		t.Errorf("outputs-and-scopes-valid: got %v, want true (optional keys absent is fine)", got)
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
	ctx.Set("issue-num", "42")
	ctx.Set("issue-url", "https://github.com/example/example/issues/42")
	ctx.Set("issue-title", "Test")
	ctx.Set("issue-handle", "PROJID:ITEMID")
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
	if got := ctx.GetString("description"); got != "Some prose." {
		t.Errorf("description: got %q", got)
	}
	if got := ctx.GetString("acceptance-criteria"); !strings.Contains(got, "Scenario: x") {
		t.Errorf("acceptance-criteria: got %q", got)
	}
	if got := ctx.GetString("checklist"); got != "" {
		t.Errorf("checklist: got %q, want empty (no Checklist in body)", got)
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
	got := ctx.GetString("checklist")
	if !strings.Contains(got, "- [x] One done") || !strings.Contains(got, "- [ ] Two pending") {
		t.Fatalf("checklist body lost: got %q", got)
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
	// No seedIssue — issue-url missing.

	out := a.parseTicket(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for missing issue-url")
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

// ---------------------------------------------------------------------------
// identifyExternalSystem — CT-HIGH real-side identity (plan 20260606-1356)
// ---------------------------------------------------------------------------

// writeIdentifyTestConfig writes a config whose external-system-driver-PORT
// root is `driver/typescript/src/external-port` and whose external-systems
// registry holds `warehouse` (simulator) + `payments` (test-instance), so the
// IDENTIFY tests can exercise both real-kind values plus the unknown/ambiguous
// error paths. Identity is resolved from the external-driver-port change
// (plan 20260613-1835), so the tests seed external-driver-port-changed-paths —
// never the adapter files.
func writeIdentifyTestConfig(t *testing.T, repoPath string) *projectconfig.Config {
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
  db-migration-path: system/db/migrations

system-test:
  path: system-test/typescript
  repo: acme/shop
  lang: typescript
  sonar-project: acme_shop-system-test
  paths:
    at-test: system-test/typescript/tests/latest/acceptance
    dsl-port: dsl/typescript/src/port
    dsl-core: dsl/typescript/src/core
    system-driver-port: driver/typescript/src/port
    system-driver-adapter: driver/typescript/src/adapter
    ct-test: system-test/typescript/tests/latest/contract
    external-system-driver-port: driver/typescript/src/external-port
    external-system-driver-adapter: driver/typescript/src/external-adapter
    system-driver-adapter-shared: driver/typescript/src/adapter/shared

external-systems:
  warehouse:
    real-kind: simulator
    stub:
      path: stubs/warehouse
      repo: acme/shop
    simulator:
      path: simulators/warehouse
      repo: acme/shop
  payments:
    real-kind: test-instance
    stub:
      path: stubs/payments
      repo: acme/shop
`
	if err := os.WriteFile(filepath.Join(repoPath, "gh-optivem.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write gh-optivem.yaml: %v", err)
	}
	cfg, err := projectconfig.Load(repoPath)
	if err != nil {
		t.Fatalf("load gh-optivem.yaml: %v", err)
	}
	return cfg
}

func TestIdentifyExternalSystem_DTOOnlyPortChange_StampsNameAndRealKind_Simulator(t *testing.T) {
	// The #65 regression: a DTO-only external-driver-port change (a DTO under
	// the port root, ZERO adapter files) must still resolve identity. Identity
	// comes from the preserved external-driver-port-changed-paths, so a DTO
	// file under `<port-root>/warehouse/dtos/...` is sufficient.
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths", "driver/typescript/src/external-port/warehouse/dtos/ReturnsProductRequest.ts")
	if out := a.identifyExternalSystem(ctx); out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("external-system-name"); got != "warehouse" {
		t.Errorf("external-system-name: got %q, want warehouse", got)
	}
	if got := ctx.GetString("real-kind"); got != "simulator" {
		t.Errorf("real-kind: got %q, want simulator", got)
	}
}

func TestIdentifyExternalSystem_MethodPortChange_NoAdapterFiles_StampsName(t *testing.T) {
	// A port-interface (method) change with NO adapter files present must
	// resolve identically — proving the adapter scan is gone and identity
	// depends only on the port change.
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths", "driver/typescript/src/external-port/warehouse/WarehousePort.ts")
	if out := a.identifyExternalSystem(ctx); out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("external-system-name"); got != "warehouse" {
		t.Errorf("external-system-name: got %q, want warehouse", got)
	}
	if got := ctx.GetString("real-kind"); got != "simulator" {
		t.Errorf("real-kind: got %q, want simulator", got)
	}
}

func TestIdentifyExternalSystem_StampsRealKind_TestInstance(t *testing.T) {
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths", "driver/typescript/src/external-port/payments/dtos/ChargeRequest.ts")
	if out := a.identifyExternalSystem(ctx); out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("external-system-name"); got != "payments" {
		t.Errorf("external-system-name: got %q, want payments", got)
	}
	if got := ctx.GetString("real-kind"); got != "test-instance" {
		t.Errorf("real-kind: got %q, want test-instance", got)
	}
}

func TestIdentifyExternalSystem_IgnoresSharedResidual(t *testing.T) {
	// A file directly under the port root (no `<name>/` segment) is residual
	// `shared` code and must not be mistaken for a system name; the per-system
	// warehouse file is the only identifiable system.
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths",
		"driver/typescript/src/external-port/SharedBase.ts\n"+
			"driver/typescript/src/external-port/warehouse/dtos/ReturnsProductRequest.ts")
	if out := a.identifyExternalSystem(ctx); out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if got := ctx.GetString("external-system-name"); got != "warehouse" {
		t.Errorf("external-system-name: got %q, want warehouse (shared residual ignored)", got)
	}
}

func TestIdentifyExternalSystem_UnknownName_HardErrors(t *testing.T) {
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths", "driver/typescript/src/external-port/shipping/dtos/ShipRequest.ts")
	out := a.identifyExternalSystem(ctx)
	if out.Err == nil {
		t.Fatalf("expected hard error on unregistered system, got nil")
	}
	if !strings.Contains(out.Err.Error(), "shipping") || !strings.Contains(out.Err.Error(), "onboard") {
		t.Errorf("error should name the unknown system and point at onboarding: %v", out.Err)
	}
	if _, set := ctx.State["real-kind"]; set {
		t.Errorf("real-kind must not be stamped on an unrecognised system")
	}
}

func TestIdentifyExternalSystem_NoExternalSystemTouched_HardErrors(t *testing.T) {
	// The preserved port-path set is empty (no external-driver-port change at
	// all) — the genuine "no external system touched" stop.
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	// Nothing seeded into external-driver-port-changed-paths; a stray shared /
	// out-of-root path must not identify a system either.
	ctx.Set("external-driver-port-changed-paths",
		"driver/typescript/src/external-port/SharedBase.ts\n"+
			"some/other/layer/File.ts")
	out := a.identifyExternalSystem(ctx)
	if out.Err == nil {
		t.Fatalf("expected hard error when no external system was touched, got nil")
	}
	if !strings.Contains(out.Err.Error(), "no external system identifiable") {
		t.Errorf("error should explain nothing was identifiable: %v", out.Err)
	}
}

func TestIdentifyExternalSystem_TwoSystems_HardErrors(t *testing.T) {
	// A ticket touching two external systems' PORTS (e.g. a method on one and
	// a DTO on the other) is the >1 case — a CT-HIGH cycle targets exactly one.
	cfg := writeIdentifyTestConfig(t, t.TempDir())
	a := newActions(Deps{Config: cfg})
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths",
		"driver/typescript/src/external-port/warehouse/WarehousePort.ts\n"+
			"driver/typescript/src/external-port/payments/dtos/ChargeRequest.ts")
	out := a.identifyExternalSystem(ctx)
	if out.Err == nil {
		t.Fatalf("expected hard error when the port change spans two systems, got nil")
	}
	// Both names surface, deterministically ordered.
	if !strings.Contains(out.Err.Error(), "payments, warehouse") {
		t.Errorf("error should list both systems in sorted order: %v", out.Err)
	}
}

func TestIdentifyExternalSystem_NilConfig_HardErrors(t *testing.T) {
	a := newActions(Deps{}) // no Config
	ctx := statemachine.NewContext()
	ctx.Set("external-driver-port-changed-paths", "driver/typescript/src/external-port/warehouse/dtos/ReturnsProductRequest.ts")
	out := a.identifyExternalSystem(ctx)
	if out.Err == nil || !strings.Contains(out.Err.Error(), "gh-optivem.yaml not loaded") {
		t.Fatalf("expected nil-config wiring error, got %v", out.Err)
	}
}
