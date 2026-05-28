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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
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
		"expected-test-result",
		"fix-on-failure-enabled",
		"dsl-port-changed",
		"system-driver-port-changed",
		"external-driver-port-changed",
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
// dsl-port-changed / system-driver-port-changed / external-driver-port-changed
// ---------------------------------------------------------------------------

func TestDriverPortChangedGates(t *testing.T) {
	type gateCase struct {
		key    string
		invoke func(b bindings, ctx *statemachine.Context) statemachine.Outcome
	}
	gates := []gateCase{
		{key: "dsl-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.dslPortChanged(ctx) }},
		{key: "system-driver-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.systemDriverPortChanged(ctx) }},
		{key: "external-driver-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.externalDriverPortChanged(ctx) }},
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
	t.Run("unrecognised_halts", func(t *testing.T) {
		p := &fakePrompter{answers: []string{"refactor-the-world"}}
		b := newBindings(t, Deps{Prompter: p})
		out := b.refactorTypeChoice(statemachine.NewContext())
		if out.Err == nil {
			t.Fatalf("expected err, got %+v", out)
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
	ctx.Set("ticket-kind", "task/legacy-coverage")
	out := b.ticketKind(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "task/legacy-coverage" {
		t.Fatalf("Value: got %q, want %q", out.Value, "task/legacy-coverage")
	}
}

func TestTicketKind_LookupTable(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      string
		subtypes  []string
		want      string
		expectErr bool
	}{
		{name: "story", kind: "story", want: "story"},
		{name: "bug", kind: "bug", want: "bug"},
		{name: "feature_aliased_to_story", kind: "feature", want: "story"},
		{name: "task_legacy_coverage", kind: "task", subtypes: []string{"legacy-coverage"}, want: "task/legacy-coverage"},
		{name: "task_system_redesign", kind: "task", subtypes: []string{"system-redesign"}, want: "task/system-redesign"},
		{name: "task_external_system_redesign", kind: "task", subtypes: []string{"external-system-redesign"}, want: "task/external-system-redesign"},
		{name: "task_system_refactor", kind: "task", subtypes: []string{"system-refactor"}, want: "task/system-refactor"},
		{name: "task_test_refactor", kind: "task", subtypes: []string{"test-refactor"}, want: "task/test-refactor"},
		{name: "task_unrecognised_subtype_halts", kind: "task", subtypes: []string{"weird-subtype"}, expectErr: true},
		{name: "task_no_subtype_halts", kind: "task", subtypes: nil, expectErr: true},
		{name: "task_multiple_subtypes_halts", kind: "task", subtypes: []string{"legacy-coverage", "system-refactor"}, expectErr: true},
		{name: "unsupported_ticket_type_halts", kind: "spike", expectErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tk := fakeTracker{
				classifyKind:      tc.kind,
				classifyConfident: true,
				subtypes:          tc.subtypes,
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

