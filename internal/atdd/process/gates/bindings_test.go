// Tests for gates/bindings.go.
//
// Strategy: drive each gate through a fakePrompter / fakeGh / fakeGit so the
// suite is hermetic (no real `gh` / `git` calls, no real stdin), and assert
// the resulting Outcome.Bool / Outcome.Value matches the binding contract
// described in process-flow.yaml. Context-pre-set paths are exercised
// alongside the prompt fallback path so the "skip the prompt when state is
// already known" optimisation is locked in.
package gates

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakePrompter struct {
	answers []string
	err     error
	asked   []string
}

func (f *fakePrompter) Ask(prompt string) (string, error) {
	f.asked = append(f.asked, prompt)
	if f.err != nil {
		return "", f.err
	}
	if len(f.answers) == 0 {
		return "", errors.New("fakePrompter: no answers left")
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}

type fakeGh struct {
	out []byte
	err error
}

func (f fakeGh) Run(_ context.Context, _ ...string) ([]byte, error) {
	return f.out, f.err
}

type fakeGit struct{ out []byte }

func (f fakeGit) Run(_ context.Context, _ ...string) ([]byte, error) {
	return f.out, nil
}

func newBindings(t *testing.T, deps Deps) bindings {
	t.Helper()
	return bindings{deps: deps.withDefaults()}
}

// ---------------------------------------------------------------------------
// RegisterAll wiring
// ---------------------------------------------------------------------------

func TestRegisterAll_AllBindingsRegistered(t *testing.T) {
	r := New()
	RegisterAll(r, Deps{Prompter: &fakePrompter{}, Gh: fakeGh{}, Git: fakeGit{}})
	want := []string{
		"scope-exception-requested",
		"phase-scope-clean",
		"dsl-flags-present",
		"command-succeeded",
		"test-outcome",
		"fix-loop-progressing",
		"expected-test-result",
		"fix-on-failure-enabled",
		"at-dsl-port-changed",
		"at-system-driver-port-changed",
		"at-external-driver-port-changed",
		"ct-dsl-port-changed",
		"ticket-has-escc",
		"real-kind",
		"external-system-touched",
		"channel-touched",
		"refactor-type-choice",
		"approval-outcome",
		"outputs-and-scopes-valid",
		"ticket-kind",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("binding %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// dsl-flags-present — GATE_DSL_FLAGS_PRESENT (plan 20260518-1144 item 4)
// ---------------------------------------------------------------------------

func TestDSLFlagsPresent_BothSetTrue(t *testing.T) {
	p := &fakePrompter{}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("system_driver_interface_changed", false)
	ctx.Set("external_system_driver_interface_changed", true)
	out := b.dslFlagsPresent(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true (both flags set)")
	}
	if len(p.asked) != 0 {
		t.Fatalf("Ask was called %d times, expected 0 (no prompt fallback)", len(p.asked))
	}
}

func TestDSLFlagsPresent_MissingSystemFalse(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("external_system_driver_interface_changed", false)
	out := b.dslFlagsPresent(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false (system flag unset)")
	}
}

func TestDSLFlagsPresent_MissingExternalFalse(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("system_driver_interface_changed", false)
	out := b.dslFlagsPresent(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false (external flag unset)")
	}
}

func TestDSLFlagsPresent_BothUnsetFalse(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	out := b.dslFlagsPresent(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false (both flags unset)")
	}
}

// ---------------------------------------------------------------------------
// scope-exception-requested — Layer 1 (plan 20260518-1144 item 6)
// ---------------------------------------------------------------------------

func TestScopeExceptionRequested_NonEmptyFilesTrue(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("scope-exception-files", []string{"src/out-of-scope.go"})
	out := b.scopeExceptionRequested(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
}

func TestScopeExceptionRequested_EmptyFalse(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	cases := []struct {
		name string
		val  any
	}{
		{name: "nil_slice", val: ([]string)(nil)},
		{name: "empty_slice", val: []string{}},
		{name: "unset", val: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := statemachine.NewContext()
			if tc.val != nil {
				ctx.Set("scope-exception-files", tc.val)
			}
			out := b.scopeExceptionRequested(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool {
				t.Fatalf("Bool: got true, want false")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// phase-scope-clean — Layer 2 (plan 20260518-1144 item 5)
// ---------------------------------------------------------------------------

func TestPhaseScopeClean_TrueRoutesContinue(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("phase-scope-clean", true)
	out := b.phaseScopeClean(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
}

func TestPhaseScopeClean_FalseRoutesStop(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("phase-scope-clean", false)
	out := b.phaseScopeClean(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false")
	}
}

func TestPhaseScopeClean_UnsetErrors(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	out := b.phaseScopeClean(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected error for unset phase-scope-clean, got nil")
	}
	if !strings.Contains(out.Err.Error(), "not set in Context") {
		t.Fatalf("error %q does not mention 'not set in Context'", out.Err)
	}
}

// ---------------------------------------------------------------------------
// BPMN Phase D bindings (plans/20260525-2348-bpmn-phase-d-bindings.md)
// ---------------------------------------------------------------------------

// fakeTracker is the test-side Tracker implementation for BPMN Phase D
// gates (ticketKind). Only Classify + Subtypes are exercised by the
// bindings under test; the other methods panic so a future binding
// that adds a new tracker call fails loud.
type fakeTracker struct {
	classifyKind      string
	classifyConfident bool
	classifyErr       error
	subtypes          []string
	subtypesErr       error
}

func (f fakeTracker) FindIssue(context.Context, string) (tracker.Issue, error) {
	panic("fakeTracker.FindIssue: not implemented")
}
func (f fakeTracker) SetStatus(context.Context, string, string) error {
	panic("fakeTracker.SetStatus: not implemented")
}
func (f fakeTracker) Verify(context.Context) error {
	panic("fakeTracker.Verify: not implemented")
}
func (f fakeTracker) Classify(_ context.Context, _ tracker.Issue) (string, bool, error) {
	return f.classifyKind, f.classifyConfident, f.classifyErr
}
func (f fakeTracker) Subtypes(_ context.Context, _ tracker.Issue) ([]string, error) {
	return f.subtypes, f.subtypesErr
}
func (f fakeTracker) ReadSections(context.Context, tracker.Issue, []string) (map[string]string, error) {
	panic("fakeTracker.ReadSections: not implemented")
}

// ---------------------------------------------------------------------------
// command-succeeded
// ---------------------------------------------------------------------------

func TestCommandSucceeded(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   any
		seeded bool
		want   bool
		err    bool
	}{
		{name: "true_bool", seed: true, seeded: true, want: true},
		{name: "false_bool", seed: false, seeded: true, want: false},
		{name: "true_string", seed: "true", seeded: true, want: true},
		{name: "false_string", seed: "false", seeded: true, want: false},
		{name: "unset_halts", seeded: false, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seeded {
				ctx.Set("command-succeeded", tc.seed)
			}
			out := b.commandSucceeded(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// test-outcome
// ---------------------------------------------------------------------------

func TestTestOutcome(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   any
		seeded bool
		want   string
		err    bool
	}{
		{name: "pass", seed: "pass", seeded: true, want: "pass"},
		{name: "fail", seed: "fail", seeded: true, want: "fail"},
		{name: "infra", seed: "infra", seeded: true, want: "infra"},
		{name: "unset_halts", seeded: false, err: true},
		{name: "wrong_type_halts", seed: 42, seeded: true, err: true},
		{name: "unrecognised_value_halts", seed: "skipped", seeded: true, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seeded {
				ctx.Set("test-outcome", tc.seed)
			}
			out := b.testOutcome(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fix-loop-progressing — GATE_FIX_PROGRESSING (plan 20260615-1845 Step 4)
// ---------------------------------------------------------------------------

func TestFixLoopProgressing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   any
		seeded bool
		want   bool
		err    bool
	}{
		{name: "true_continues", seed: true, seeded: true, want: true},
		{name: "false_halts", seed: false, seeded: true, want: false},
		{name: "string_true", seed: "true", seeded: true, want: true},
		{name: "string_false", seed: "false", seeded: true, want: false},
		{name: "unset_halts", seeded: false, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seeded {
				ctx.Set("fix-loop-progressing", tc.seed)
			}
			out := b.fixLoopProgressing(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// real-kind — GATE_CONTRACT_REAL_RED_KIND (plans 20260606-1356, -1943)
// ---------------------------------------------------------------------------

func TestRealKind(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   any
		seeded bool
		want   string
		err    bool
	}{
		{name: "test-instance", seed: "test-instance", seeded: true, want: "test-instance"},
		{name: "simulator", seed: "simulator", seeded: true, want: "simulator"},
		{name: "unset_halts", seeded: false, err: true},
		{name: "wrong_type_halts", seed: 42, seeded: true, err: true},
		{name: "unrecognised_value_halts", seed: "stub", seeded: true, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seeded {
				ctx.Set("real-kind", tc.seed)
			}
			out := b.realKind(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// external-system-touched — GATE_EXTERNAL_SYSTEM_TOUCHED (plan 20260615-0755)
// ---------------------------------------------------------------------------

func TestExternalSystemTouched(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   any
		seeded bool
		want   bool
		err    bool
	}{
		{name: "touched", seed: true, seeded: true, want: true},
		{name: "untouched", seed: false, seeded: true, want: false},
		{name: "unset_halts", seeded: false, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seeded {
				ctx.Set("external-system-touched", tc.seed)
			}
			out := b.externalSystemTouched(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// expected-test-result
// ---------------------------------------------------------------------------

func TestExpectedTestResult(t *testing.T) {
	for _, tc := range []struct {
		name  string
		param string
		want  string
		err   bool
	}{
		{name: "success", param: "success", want: "success"},
		{name: "failure", param: "failure", want: "failure"},
		{name: "whitespace_trimmed", param: "  success  ", want: "success"},
		{name: "empty_halts", param: "", err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.param != "" || tc.name == "empty_halts" {
				if tc.param != "" {
					ctx.Params["expected-test-result"] = tc.param
				}
			}
			out := b.expectedTestResult(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// fix-on-failure-enabled
// ---------------------------------------------------------------------------

func TestFixOnFailureEnabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		param   string
		wantBool bool
		err     bool
	}{
		{name: "empty_defaults_true", param: "", wantBool: true},
		{name: "true", param: "true", wantBool: true},
		{name: "false", param: "false", wantBool: false},
		{name: "yes", param: "yes", wantBool: true},
		{name: "no", param: "no", wantBool: false},
		{name: "garbage_halts", param: "maybe", err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.param != "" {
				ctx.Params["fix-on-failure"] = tc.param
			}
			out := b.fixOnFailureEnabled(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.wantBool {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.wantBool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// at-/ct- cascade-namespaced port-changed verdicts (plan 20260606-1525)
// ---------------------------------------------------------------------------

func TestDriverPortChangedGates(t *testing.T) {
	type gateCase struct {
		key    string
		invoke func(b bindings, ctx *statemachine.Context) statemachine.Outcome
	}
	gates := []gateCase{
		{key: "at-dsl-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.atDslPortChanged(ctx) }},
		{key: "at-system-driver-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.atSystemDriverPortChanged(ctx) }},
		{key: "at-external-driver-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.atExternalDriverPortChanged(ctx) }},
		{key: "ct-dsl-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.ctDslPortChanged(ctx) }},
	}
	for _, g := range gates {
		t.Run(g.key+"/true", func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set(g.key, true)
			out := g.invoke(b, ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if !out.Bool {
				t.Fatalf("Bool: got false, want true")
			}
		})
		t.Run(g.key+"/false", func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set(g.key, false)
			out := g.invoke(b, ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool {
				t.Fatalf("Bool: got true, want false")
			}
		})
		t.Run(g.key+"/string_yes_coerced", func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set(g.key, "yes")
			out := g.invoke(b, ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if !out.Bool {
				t.Fatalf("Bool: got false, want true")
			}
		})
		t.Run(g.key+"/unset_halts", func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			out := g.invoke(b, statemachine.NewContext())
			if out.Err == nil {
				t.Fatalf("expected err, got %+v", out)
			}
			if !strings.Contains(out.Err.Error(), "outputs:") {
				t.Fatalf("err %q does not mention 'outputs:'", out.Err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// cover-path mode-aware verify gate (plan 20260606-1518)
// ---------------------------------------------------------------------------

// TestAtVerifyExpectation is the verify-polarity matrix for the four cover-path
// rows of the plan's table plus the change-path regression. It exercises the
// binding directly: the change path (verify-mode red/unset) must echo the
// pinned expected-test-result verbatim, and the cover path (green-when-complete)
// must return success exactly when this layer's plumbing scope reports nothing
// pending.
func TestAtVerifyExpectation(t *testing.T) {
	cases := []struct {
		name      string
		mode      string         // verify-mode param ("" = unset)
		pendingOn string         // verify-pending-on param ("" = unset)
		expected  string         // expected-test-result param ("" = unset)
		state     map[string]any // preseeded at-* flags
		wantValue string
		wantErr   bool
	}{
		// change path (red / unset) — echoes expected-test-result verbatim.
		{name: "red/success", mode: "red", expected: "success", wantValue: "success"},
		{name: "red/failure", mode: "red", expected: "failure", wantValue: "failure"},
		{name: "unset_defaults_red", expected: "failure", wantValue: "failure"},
		{name: "red/empty_halts", mode: "red", wantErr: true},
		// compile-only path (plan 20260606-2330) — short-circuits to "none"
		// before consulting plumbingPending or expected-test-result. The
		// contradictory expected-test-result (would yield "failure" on the
		// change path) and the unscoped verify-pending-on (would HALT on the
		// green path) both prove the early return fires first.
		{name: "none/short-circuits-expected", mode: "none", expected: "failure", wantValue: "none"},
		{name: "none/ignores-pending-scope", mode: "none", pendingOn: "drivers", wantValue: "none"},
		// case A — test-code layer, no DSL change: greens at the code layer.
		{name: "green/dsl/not-pending", mode: "green-when-complete", pendingOn: "dsl",
			state: map[string]any{"at-dsl-port-changed": false}, wantValue: "success"},
		// cases B/C/D at the code layer — DSL changed, still red here.
		{name: "green/dsl/pending", mode: "green-when-complete", pendingOn: "dsl",
			state: map[string]any{"at-dsl-port-changed": true}, wantValue: "failure"},
		// case B — DSL layer, no driver ports change: greens at the DSL layer.
		{name: "green/drivers/none-pending", mode: "green-when-complete", pendingOn: "drivers",
			state: map[string]any{"at-system-driver-port-changed": false, "at-external-driver-port-changed": false}, wantValue: "success"},
		// case C — DSL layer, system-driver port changed: still red (adapters pending).
		{name: "green/drivers/system-pending", mode: "green-when-complete", pendingOn: "drivers",
			state: map[string]any{"at-system-driver-port-changed": true, "at-external-driver-port-changed": false}, wantValue: "failure"},
		// case D — DSL layer, external-driver port changed only: still red (external CT-HIGH pending).
		{name: "green/drivers/external-pending", mode: "green-when-complete", pendingOn: "drivers",
			state: map[string]any{"at-system-driver-port-changed": false, "at-external-driver-port-changed": true}, wantValue: "failure"},
		// case C terminal — system-driver adapter layer: always greens (nothing after).
		{name: "green/none/terminal", mode: "green-when-complete", pendingOn: "none", wantValue: "success"},
		// strictness: unset/unknown scope and unset flags halt rather than mis-route.
		{name: "green/pending-on-unset_halts", mode: "green-when-complete", wantErr: true},
		{name: "green/pending-on-unknown_halts", mode: "green-when-complete", pendingOn: "bogus", wantErr: true},
		{name: "green/drivers/flag-unset_halts", mode: "green-when-complete", pendingOn: "drivers",
			state: map[string]any{"at-system-driver-port-changed": false}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.mode != "" {
				ctx.Params["verify-mode"] = tc.mode
			}
			if tc.pendingOn != "" {
				ctx.Params["verify-pending-on"] = tc.pendingOn
			}
			if tc.expected != "" {
				ctx.Params["expected-test-result"] = tc.expected
			}
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			out := b.atVerifyExpectation(ctx)
			if tc.wantErr {
				if out.Err == nil {
					t.Fatalf("want err, got Value=%q", out.Value)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.wantValue {
				t.Fatalf("Value = %q, want %q", out.Value, tc.wantValue)
			}
		})
	}
}

// TestAtExternalTerminalVerifyNeeded covers the case-D terminal-verify gate: it
// fires only on the cover path and only when no system-driver adapter step
// follows (else that step owns the terminal PASS).
func TestAtExternalTerminalVerifyNeeded(t *testing.T) {
	cases := []struct {
		name     string
		mode     string
		state    map[string]any
		wantBool bool
		wantErr  bool
	}{
		{name: "red_never", mode: "red", wantBool: false},
		{name: "unset_never", wantBool: false},
		{name: "green/no-system-driver/needs-terminal", mode: "green-when-complete",
			state: map[string]any{"at-system-driver-port-changed": false}, wantBool: true},
		{name: "green/system-driver-follows/no-terminal", mode: "green-when-complete",
			state: map[string]any{"at-system-driver-port-changed": true}, wantBool: false},
		{name: "green/system-flag-unset_halts", mode: "green-when-complete", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.mode != "" {
				ctx.Params["verify-mode"] = tc.mode
			}
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			out := b.atExternalTerminalVerifyNeeded(ctx)
			if tc.wantErr {
				if out.Err == nil {
					t.Fatalf("want err, got Bool=%v", out.Bool)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.wantBool {
				t.Fatalf("Bool = %v, want %v", out.Bool, tc.wantBool)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// refactor-type-choice
// ---------------------------------------------------------------------------

func TestRefactorTypeChoice(t *testing.T) {
	t.Run("preseeded_state_short_circuits", func(t *testing.T) {
		p := &fakePrompter{} // no answers; would error if asked.
		b := newBindings(t, Deps{Prompter: p})
		ctx := statemachine.NewContext()
		ctx.Set("refactor-type-choice", "refactor-system-structure")
		out := b.refactorTypeChoice(ctx)
		if out.Err != nil {
			t.Fatalf("unexpected err: %v", out.Err)
		}
		if out.Value != "refactor-system-structure" {
			t.Fatalf("Value: got %q, want %q", out.Value, "refactor-system-structure")
		}
		if len(p.asked) != 0 {
			t.Fatalf("prompter should not have been asked: %v", p.asked)
		}
	})
	for _, ans := range []string{"refactor-system-structure", "refactor-test-structure", "redesign-system-structure", "redesign-external-system-structure", "none"} {
		t.Run("prompt/"+ans, func(t *testing.T) {
			p := &fakePrompter{answers: []string{ans}}
			b := newBindings(t, Deps{Prompter: p})
			out := b.refactorTypeChoice(statemachine.NewContext())
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != ans {
				t.Fatalf("Value: got %q, want %q", out.Value, ans)
			}
		})
	}
	t.Run("empty_reply_defaults_to_none", func(t *testing.T) {
		p := &fakePrompter{answers: []string{""}}
		b := newBindings(t, Deps{Prompter: p})
		out := b.refactorTypeChoice(statemachine.NewContext())
		if out.Err != nil {
			t.Fatalf("unexpected err: %v", out.Err)
		}
		if out.Value != "none" {
			t.Fatalf("Value: got %q, want %q", out.Value, "none")
		}
	})
	t.Run("unrecognised_reprompts_until_valid", func(t *testing.T) {
		// Mirrors promptio.ConfirmYNVia's loop semantics: an unrecognised
		// reply ("n", "refactor-the-world") prints the reminder and asks
		// again, instead of halting the entire BPMN cycle.
		p := &fakePrompter{answers: []string{"n", "refactor-the-world", "none"}}
		b := newBindings(t, Deps{Prompter: p})
		out := b.refactorTypeChoice(statemachine.NewContext())
		if out.Err != nil {
			t.Fatalf("unexpected err: %v", out.Err)
		}
		if out.Value != "none" {
			t.Fatalf("Value: got %q, want %q", out.Value, "none")
		}
		if len(p.asked) != 3 {
			t.Fatalf("expected 3 prompts (two reprompts + valid), got %d", len(p.asked))
		}
	})
	t.Run("auto_skips_prompt_and_returns_none", func(t *testing.T) {
		// Under --auto the menu is operator-skippable: the opportunistic
		// refactor branch is never entered, the default "none" is taken
		// without prompting so an autonomous run does not stall on stdin.
		p := &fakePrompter{} // no answers; would error if asked.
		b := newBindings(t, Deps{Prompter: p, Approval: approval.Resolved{Auto: true}})
		out := b.refactorTypeChoice(statemachine.NewContext())
		if out.Err != nil {
			t.Fatalf("unexpected err: %v", out.Err)
		}
		if out.Value != "none" {
			t.Fatalf("Value: got %q, want %q", out.Value, "none")
		}
		if len(p.asked) != 0 {
			t.Fatalf("prompter should not have been asked under --auto: %v", p.asked)
		}
	})
}

// ---------------------------------------------------------------------------
// approval-outcome
// ---------------------------------------------------------------------------

func TestApprovalOutcome(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed string
		want string
		err  bool
	}{
		{name: "approved", seed: "approved", want: "approved"},
		{name: "rejected", seed: "rejected", want: "rejected"},
		{name: "unset_halts", seed: "", err: true},
		{name: "unrecognised_halts", seed: "maybe", err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seed != "" {
				ctx.Set("approval-outcome", tc.seed)
			}
			out := b.approvalOutcome(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// outputs-and-scopes-valid
// ---------------------------------------------------------------------------

func TestOutputsAndScopesValid(t *testing.T) {
	for _, tc := range []struct {
		name   string
		seed   any
		seeded bool
		want   bool
		err    bool
	}{
		{name: "true", seed: true, seeded: true, want: true},
		{name: "false", seed: false, seeded: true, want: false},
		{name: "unset_halts", seeded: false, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			if tc.seeded {
				ctx.Set("outputs-and-scopes-valid", tc.seed)
			}
			out := b.outputsAndScopesValid(ctx)
			if tc.err {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ticket-kind (Q-D3 lookup table)
// ---------------------------------------------------------------------------

func TestTicketKind_PreseededShortCircuits(t *testing.T) {
	// fakeTracker with both methods panicking — proves the binding
	// short-circuits on a preseeded ticket-kind without touching the
	// tracker.
	tk := fakeTracker{}
	b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
	ctx := statemachine.NewContext()
	ctx.Set("issue-url", "https://example/1")
	ctx.Set("ticket-kind", "task")
	out := b.ticketKind(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "task" {
		t.Fatalf("Value: got %q, want %q", out.Value, "task")
	}
}

func TestTicketKind_LookupTable(t *testing.T) {
	// ticketKind resolves only the kind axis — subtype resolution moved
	// down to taskSubtype (TestTaskSubtype_LookupTable). Every task,
	// regardless of its subtype labels, resolves to bare "task".
	for _, tc := range []struct {
		name      string
		kind      string
		want      string
		expectErr bool
	}{
		{name: "story", kind: "story", want: "story"},
		{name: "bug", kind: "bug", want: "bug"},
		{name: "feature_aliased_to_story", kind: "feature", want: "story"},
		{name: "task", kind: "task", want: "task"},
		{name: "unsupported_ticket_type_halts", kind: "spike", expectErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tk := fakeTracker{
				classifyKind:      tc.kind,
				classifyConfident: true,
				// subtypes intentionally left unset: ticketKind no longer
				// reads them (that is taskSubtype's job now).
			}
			b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
			ctx := statemachine.NewContext()
			ctx.Set("issue-url", "https://example/1")
			out := b.ticketKind(ctx)
			if tc.expectErr {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

func TestTicketKind_NoConfidence_Halts(t *testing.T) {
	tk := fakeTracker{classifyKind: "", classifyConfident: false}
	b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
	ctx := statemachine.NewContext()
	ctx.Set("issue-url", "https://example/1")
	out := b.ticketKind(ctx)
	if out.Err == nil {
		t.Fatalf("expected err for no-confidence classification, got %+v", out)
	}
}

func TestTicketKind_NoIssueURL_Halts(t *testing.T) {
	tk := fakeTracker{}
	b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
	out := b.ticketKind(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected err for missing issue-url, got %+v", out)
	}
}

// ---------------------------------------------------------------------------
// task-subtype (second-level GATE_TASK_SUBTYPE axis)
// ---------------------------------------------------------------------------

func TestTaskSubtype_PreseededShortCircuits(t *testing.T) {
	// fakeTracker.Subtypes would return nil; the preseed must short-
	// circuit before the binding reaches the tracker at all.
	tk := fakeTracker{}
	b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
	ctx := statemachine.NewContext()
	ctx.Set("issue-url", "https://example/1")
	ctx.Set("task-subtype", "legacy-coverage")
	out := b.taskSubtype(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "legacy-coverage" {
		t.Fatalf("Value: got %q, want %q", out.Value, "legacy-coverage")
	}
}

func TestTaskSubtype_LookupTable(t *testing.T) {
	// Mirror of ticketKind's old task branch, lifted down one axis: the
	// subtype resolves from Tracker.Subtypes, exactly one label expected.
	for _, tc := range []struct {
		name      string
		subtypes  []string
		want      string
		expectErr bool
	}{
		{name: "legacy_coverage", subtypes: []string{"legacy-coverage"}, want: "legacy-coverage"},
		{name: "system_redesign", subtypes: []string{"system-redesign"}, want: "system-redesign"},
		{name: "external_system_redesign", subtypes: []string{"external-system-redesign"}, want: "external-system-redesign"},
		{name: "system_refactor", subtypes: []string{"system-refactor"}, want: "system-refactor"},
		{name: "test_refactor", subtypes: []string{"test-refactor"}, want: "test-refactor"},
		{name: "unrecognised_subtype_halts", subtypes: []string{"weird-subtype"}, expectErr: true},
		{name: "no_subtype_halts", subtypes: nil, expectErr: true},
		{name: "multiple_subtypes_halts", subtypes: []string{"legacy-coverage", "system-refactor"}, expectErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tk := fakeTracker{subtypes: tc.subtypes}
			b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
			ctx := statemachine.NewContext()
			ctx.Set("issue-url", "https://example/1")
			out := b.taskSubtype(ctx)
			if tc.expectErr {
				if out.Err == nil {
					t.Fatalf("expected err, got %+v", out)
				}
				return
			}
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

func TestTaskSubtype_NoIssueURL_Halts(t *testing.T) {
	tk := fakeTracker{}
	b := newBindings(t, Deps{Prompter: &fakePrompter{}, Tracker: tk})
	out := b.taskSubtype(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected err for missing issue-url, got %+v", out)
	}
}

