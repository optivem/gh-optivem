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
	"fmt"
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
// Boolean prompt-driven gates
// ---------------------------------------------------------------------------

func TestDSLInterfaceChanged_Prompt(t *testing.T) {
	// Cases with multiple `answers` exercise the promptio reprompt loop:
	// the first answer is unrecognised so ConfirmYNVia loops and reads the
	// second. Bare Enter and "maybe" both reprompt — there is no Enter-
	// default any more, every gate needs an explicit y/n.
	for _, tc := range []struct {
		name    string
		answers []string
		want    bool
	}{
		{name: "yes_lower", answers: []string{"y"}, want: true},
		{name: "yes_word", answers: []string{"yes\n"}, want: true},
		{name: "no_lower", answers: []string{"n"}, want: false},
		{name: "no_word", answers: []string{"no\n"}, want: false},
		{name: "empty_reprompts_then_resolves", answers: []string{"\n", "n"}, want: false},
		{name: "garbage_reprompts_then_resolves", answers: []string{"maybe", "y"}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{answers: tc.answers}
			b := newBindings(t, Deps{Prompter: p})
			ctx := statemachine.NewContext()
			out := b.dslInterfaceChanged(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

func TestBoolGate_ReadsContextFirst(t *testing.T) {
	// When the binding key is pre-set in Context, the gate must NOT prompt.
	p := &fakePrompter{} // no answers; would error if Ask were called.
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("dsl_interface_changed", true)
	out := b.dslInterfaceChanged(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 0 {
		t.Fatalf("Ask was called %d times, expected 0", len(p.asked))
	}
}

func TestBoolGate_AcceptsStringTrueFalse(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
		want bool
	}{
		{name: "string_true", val: "true", want: true},
		{name: "string_false", val: "false", want: false},
		{name: "string_yes", val: "yes", want: true},
		{name: "bool_true", val: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set("system_driver_interface_changed", tc.val)
			out := b.systemDriverInterfaceChanged(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ticketType
// ---------------------------------------------------------------------------

func TestTicketType_FromContext(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("ticket_type", "story")
	out := b.ticketType(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Value != "story" {
		t.Fatalf("Value: got %q, want %q", out.Value, "story")
	}
}

func TestTicketType_PromptValid(t *testing.T) {
	for _, want := range []string{"story", "bug", "task"} {
		t.Run(want, func(t *testing.T) {
			p := &fakePrompter{answers: []string{want}}
			b := newBindings(t, Deps{Prompter: p})
			ctx := statemachine.NewContext()
			out := b.ticketType(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Value != want {
				t.Fatalf("Value: got %q, want %q", out.Value, want)
			}
		})
	}
}

func TestTicketType_PromptInvalid(t *testing.T) {
	p := &fakePrompter{answers: []string{"epic"}}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	out := b.ticketType(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for invalid ticket type, got Outcome %+v", out)
	}
	if !strings.Contains(out.Err.Error(), "unrecognised value") {
		t.Fatalf("unexpected error message: %v", out.Err)
	}
}

// ---------------------------------------------------------------------------
// subtype
// ---------------------------------------------------------------------------

func TestSubtype_FromContext(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("subtype", "system-interface-redesign")
	out := b.subtype(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Value != "system-interface-redesign" {
		t.Fatalf("Value: got %q", out.Value)
	}
}

func TestSubtype_PromptValid(t *testing.T) {
	for _, want := range []string{
		"system-interface-redesign",
		"external-system-interface-redesign",
		"system-implementation-refactoring",
	} {
		t.Run(want, func(t *testing.T) {
			p := &fakePrompter{answers: []string{want}}
			b := newBindings(t, Deps{Prompter: p})
			out := b.subtype(statemachine.NewContext())
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Value != want {
				t.Fatalf("Value: got %q, want %q", out.Value, want)
			}
		})
	}
}

func TestSubtype_PromptInvalid(t *testing.T) {
	p := &fakePrompter{answers: []string{"refactor"}}
	b := newBindings(t, Deps{Prompter: p})
	out := b.subtype(statemachine.NewContext())
	if out.Err == nil {
		t.Fatalf("expected error for invalid subtype")
	}
}

// ---------------------------------------------------------------------------
// parseOK
// ---------------------------------------------------------------------------

func TestParseOK_FromContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
		want bool
	}{
		{name: "true", val: true, want: true},
		{name: "false", val: false, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set("parse_ok", tc.val)
			out := b.parseOK(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// structuralTestMode
// ---------------------------------------------------------------------------

func TestStructuralTestMode_Defaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		want   string
	}{
		{name: "explicit_full", answer: "full", want: "full"},
		{name: "explicit_compile", answer: "compile", want: "compile"},
		{name: "explicit_skip", answer: "skip", want: "skip"},
		{name: "blank_defaults_to_compile", answer: "", want: "compile"},
		{name: "trims_and_lowercases", answer: "  FULL\n", want: "full"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{answers: []string{tc.answer}}
			b := newBindings(t, Deps{Prompter: p})
			ctx := statemachine.NewContext()
			out := b.structuralTestMode(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Value != tc.want {
				t.Fatalf("Value: got %q, want %q", out.Value, tc.want)
			}
		})
	}
}

func TestStructuralTestMode_Invalid(t *testing.T) {
	p := &fakePrompter{answers: []string{"sample"}}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	out := b.structuralTestMode(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for invalid mode, got %+v", out)
	}
}

// ---------------------------------------------------------------------------
// structuralVerifyOutcome — ok / red / red-after-retry / infra / unknown
//
// Item 3 of the verify-failure-dispatch-fix-agent plan: this gate routes
// the post-RUN_TESTS edge based on the failure class the verify action
// stamped into ctx.State["verify_class"], and is the only place the
// one-retry budget is enforced.
// ---------------------------------------------------------------------------

func TestStructuralVerifyOutcome_OkRoutesToReview(t *testing.T) {
	b := newBindings(t, Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("verify_class", "ok")
	out := b.structuralVerifyOutcome(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "ok" {
		t.Errorf("Value: got %q, want %q", out.Value, "ok")
	}
	if r, _ := ctx.Get("verify_retries").(int); r != 0 {
		t.Errorf("verify_retries should not increment on ok; got %d", r)
	}
}

func TestStructuralVerifyOutcome_EmptyClassTreatedAsOk(t *testing.T) {
	// Approve-without-running and "no driver-adapter changes" leave
	// verify_class empty. The gate should not invent a class — the human
	// review still owns the call.
	b := newBindings(t, Deps{})
	ctx := statemachine.NewContext()
	out := b.structuralVerifyOutcome(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "ok" {
		t.Errorf("Value: got %q, want %q", out.Value, "ok")
	}
}

func TestStructuralVerifyOutcome_FirstRedDispatchesFix(t *testing.T) {
	// First red: route to FIX_STRUCT_VERIFY and bump verify_retries to 1
	// so a second red after the agent's retry halts.
	b := newBindings(t, Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("verify_class", "red")
	out := b.structuralVerifyOutcome(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "red" {
		t.Errorf("Value: got %q, want %q", out.Value, "red")
	}
	if r, _ := ctx.Get("verify_retries").(int); r != 1 {
		t.Errorf("verify_retries: got %d, want 1", r)
	}
}

func TestStructuralVerifyOutcome_SecondRedHalts(t *testing.T) {
	// After one fix-agent retry: the gate halts. The cycle's contract is
	// "behaviour-preserving"; a second red after a fix attempt means the
	// agent could not restore green — surface to the human.
	b := newBindings(t, Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("verify_class", "red")
	ctx.Set("verify_retries", 1)
	out := b.structuralVerifyOutcome(ctx)
	if out.Err == nil {
		t.Fatalf("expected halt on second red; got Value=%q", out.Value)
	}
	if !strings.Contains(out.Err.Error(), "still RED") {
		t.Errorf("halt error should describe the state; got: %v", out.Err)
	}
}

func TestStructuralVerifyOutcome_InfraIsDefensiveError(t *testing.T) {
	// Item 5 halts at the action level on infra. Reaching the gate with
	// class=infra means that halt was bypassed — surface as a bug rather
	// than silently routing.
	b := newBindings(t, Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("verify_class", "infra")
	out := b.structuralVerifyOutcome(ctx)
	if out.Err == nil {
		t.Fatalf("expected defensive halt on infra; got Value=%q", out.Value)
	}
	if !strings.Contains(out.Err.Error(), "infra") {
		t.Errorf("err should mention infra; got: %v", out.Err)
	}
}

func TestStructuralVerifyOutcome_UnknownClassErrors(t *testing.T) {
	b := newBindings(t, Deps{})
	ctx := statemachine.NewContext()
	ctx.Set("verify_class", "warning")
	out := b.structuralVerifyOutcome(ctx)
	if out.Err == nil {
		t.Fatalf("expected error for unknown class; got Value=%q", out.Value)
	}
}

// ---------------------------------------------------------------------------
// legacyAcceptanceCriteriaSectionPresent
// ---------------------------------------------------------------------------

func TestLegacyAcceptanceCriteria_FromGhIssueBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want bool
	}{
		{
			name: "present_h2",
			body: "Description.\n\n## Legacy Acceptance Criteria\n\n- old test\n",
			want: true,
		},
		{
			name: "present_h3_case_insensitive",
			body: "## Story\n\n### legacy acceptance criteria\n\nfoo\n",
			want: true,
		},
		{
			name: "absent",
			body: "## Story\n\nNo coverage here.\n",
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gh := fakeGh{out: []byte(fmt.Sprintf(`{"body":%q}`, tc.body))}
			b := newBindings(t, Deps{Gh: gh, Prompter: &fakePrompter{}})
			ctx := statemachine.NewContext()
			ctx.Set("issue_num", "42")
			ctx.Set("issue_url", "https://github.com/optivem/shop/issues/42")
			out := b.legacyAcceptanceCriteriaSectionPresent(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

func TestLegacyAcceptanceCriteria_NoIssueNumPrompts(t *testing.T) {
	p := &fakePrompter{answers: []string{"y"}}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	out := b.legacyAcceptanceCriteriaSectionPresent(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 1 {
		t.Fatalf("expected one prompt, got %d", len(p.asked))
	}
}

// ---------------------------------------------------------------------------
// extractIssueBody
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// RegisterAll wiring
// ---------------------------------------------------------------------------

func TestRegisterAll_AllBindingsRegistered(t *testing.T) {
	r := New()
	RegisterAll(r, Deps{Prompter: &fakePrompter{}, Gh: fakeGh{}, Git: fakeGit{}})
	want := []string{
		"dsl_interface_changed",
		"external_system_driver_interface_changed",
		"system_driver_interface_changed",
		"ticket_type",
		"subtype",
		"change_type",
		"ticket_type_recognized",
		"subtype_ok",
		"parse_ok",
		"legacy_acceptance_criteria_section_present",
		"refine_requested",
		"external_system_driver_exists",
		"external_system_test_instance_accessible",
		"smoke_test_passes",
		"structural_test_mode",
		"compile_ok",
		"tests_failed_runtime",
		"tests_pass",
		"verify_real_required",
		"verify_real_pass",
		"structural_verify_outcome",
		"tests_selected",
		"scope_exception_requested",
		"phase_scope_clean",
		"dsl_flags_present",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("binding %q not registered", name)
		}
	}
}

// ---------------------------------------------------------------------------
// tests_selected — GATE_TESTS_SELECTED routing
// ---------------------------------------------------------------------------

func TestTestsSelected_ReadsSelectedCommands(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
		want bool
	}{
		{name: "non_empty_slice_true", val: []string{"gh optivem test run"}, want: true},
		{name: "empty_slice_false", val: []string{}, want: false},
		{name: "nil_slice_false", val: ([]string)(nil), want: false},
		{name: "unset_key_false", val: nil, want: false},
		{name: "wrong_type_false", val: "not a slice", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newBindings(t, Deps{})
			ctx := statemachine.NewContext()
			if tc.val != nil {
				ctx.Set("selected_test_commands", tc.val)
			}
			out := b.testsSelected(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected err: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Errorf("Outcome.Bool: got %v, want %v", out.Bool, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// compile_ok / tests_failed_runtime — red_phase_cycle gates
// ---------------------------------------------------------------------------

func TestCompileOK_ReadsContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		val  any
		want bool
	}{
		{name: "true_bool", val: true, want: true},
		{name: "false_bool", val: false, want: false},
		{name: "true_string", val: "true", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{}
			b := newBindings(t, Deps{Prompter: p})
			ctx := statemachine.NewContext()
			ctx.Set("compile_ok", tc.val)
			out := b.compileOK(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
			if len(p.asked) != 0 {
				t.Fatalf("Ask was called %d times, expected 0", len(p.asked))
			}
		})
	}
}

func TestCompileOK_PromptFallback(t *testing.T) {
	p := &fakePrompter{answers: []string{"y"}}
	b := newBindings(t, Deps{Prompter: p})
	out := b.compileOK(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 1 {
		t.Fatalf("Ask was called %d times, expected 1", len(p.asked))
	}
}

func TestTestsFailedRuntime_ReadsContext(t *testing.T) {
	p := &fakePrompter{}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("tests_failed_runtime", true)
	out := b.testsFailedRuntime(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 0 {
		t.Fatalf("Ask was called %d times, expected 0", len(p.asked))
	}
}

func TestTestsPass_ReadsContext(t *testing.T) {
	p := &fakePrompter{}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("tests_pass", true)
	out := b.testsPass(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 0 {
		t.Fatalf("Ask was called %d times, expected 0", len(p.asked))
	}
}

func TestTestsPass_PromptFallback(t *testing.T) {
	p := &fakePrompter{answers: []string{"y"}}
	b := newBindings(t, Deps{Prompter: p})
	out := b.testsPass(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 1 {
		t.Fatalf("Ask was called %d times, expected 1", len(p.asked))
	}
}

func TestTestsFailedRuntime_PromptFallback(t *testing.T) {
	p := &fakePrompter{answers: []string{"n"}}
	b := newBindings(t, Deps{Prompter: p})
	out := b.testsFailedRuntime(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false")
	}
	if len(p.asked) != 1 {
		t.Fatalf("Ask was called %d times, expected 1", len(p.asked))
	}
}

// ---------------------------------------------------------------------------
// verify_real_required / verify_real_pass — optional CT verify-real branch
// ---------------------------------------------------------------------------

func TestVerifyRealRequired_ReadsParamSet(t *testing.T) {
	for _, tc := range []struct {
		name  string
		suite string
		want  bool
	}{
		{name: "ct_red_test", suite: "<suite-contract-real>", want: true},
		{name: "at_unset", suite: "", want: false},
		{name: "whitespace_only", suite: "   ", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{}
			b := newBindings(t, Deps{Prompter: p})
			ctx := statemachine.NewContext()
			ctx.Params["verify_real_suite"] = tc.suite
			out := b.verifyRealRequired(ctx)
			if out.Err != nil {
				t.Fatalf("unexpected error: %v", out.Err)
			}
			if out.Bool != tc.want {
				t.Fatalf("Bool: got %v, want %v", out.Bool, tc.want)
			}
			if len(p.asked) != 0 {
				t.Fatalf("Ask was called %d times, expected 0 (param-only gate)", len(p.asked))
			}
		})
	}
}

func TestVerifyRealRequired_NilParams(t *testing.T) {
	p := &fakePrompter{}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Params = nil
	out := b.verifyRealRequired(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("nil Params should route as required=false, got true")
	}
}

func TestVerifyRealPass_ReadsContext(t *testing.T) {
	p := &fakePrompter{}
	b := newBindings(t, Deps{Prompter: p})
	ctx := statemachine.NewContext()
	ctx.Set("verify_real_pass", true)
	out := b.verifyRealPass(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if !out.Bool {
		t.Fatalf("Bool: got false, want true")
	}
	if len(p.asked) != 0 {
		t.Fatalf("Ask was called %d times, expected 0", len(p.asked))
	}
}

func TestVerifyRealPass_PromptFallback(t *testing.T) {
	p := &fakePrompter{answers: []string{"n"}}
	b := newBindings(t, Deps{Prompter: p})
	out := b.verifyRealPass(statemachine.NewContext())
	if out.Err != nil {
		t.Fatalf("unexpected error: %v", out.Err)
	}
	if out.Bool {
		t.Fatalf("Bool: got true, want false")
	}
	if len(p.asked) != 1 {
		t.Fatalf("Ask was called %d times, expected 1", len(p.asked))
	}
}

// ---------------------------------------------------------------------------
// dsl_flags_present — GATE_DSL_FLAGS_PRESENT (plan 20260518-1144 item 4)
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
// scope_exception_requested — Layer 1 (plan 20260518-1144 item 6)
// ---------------------------------------------------------------------------

func TestScopeExceptionRequested_NonEmptyFilesTrue(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("scope_exception_files", []string{"src/out-of-scope.go"})
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
				ctx.Set("scope_exception_files", tc.val)
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
// phase_scope_clean — Layer 2 (plan 20260518-1144 item 5)
// ---------------------------------------------------------------------------

func TestPhaseScopeClean_TrueRoutesContinue(t *testing.T) {
	b := newBindings(t, Deps{Prompter: &fakePrompter{}})
	ctx := statemachine.NewContext()
	ctx.Set("phase_scope_clean", true)
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
	ctx.Set("phase_scope_clean", false)
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
		t.Fatalf("expected error for unset phase_scope_clean, got nil")
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

func (f fakeTracker) PickReady(context.Context) (tracker.Issue, error) {
	panic("fakeTracker.PickReady: not implemented")
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
func (f fakeTracker) MarkChecklistComplete(context.Context, tracker.Issue) error {
	panic("fakeTracker.MarkChecklistComplete: not implemented")
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
// dsl-port-changed / system-driver-ports-changed / external-driver-ports-changed
// ---------------------------------------------------------------------------

func TestDriverPortChangedGates(t *testing.T) {
	type gateCase struct {
		key    string
		invoke func(b bindings, ctx *statemachine.Context) statemachine.Outcome
	}
	gates := []gateCase{
		{key: "dsl-port-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.dslPortChanged(ctx) }},
		{key: "system-driver-ports-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.systemDriverPortsChanged(ctx) }},
		{key: "external-driver-ports-changed", invoke: func(b bindings, ctx *statemachine.Context) statemachine.Outcome { return b.externalDriverPortsChanged(ctx) }},
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
	for _, ans := range []string{"refactor-system-structure", "refactor-test-structure", "redesign-system-structure", "none"} {
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
	ctx.Set("issue_url", "https://example/1")
	ctx.Set("ticket-kind", "task/cover-legacy")
	out := b.ticketKind(ctx)
	if out.Err != nil {
		t.Fatalf("unexpected err: %v", out.Err)
	}
	if out.Value != "task/cover-legacy" {
		t.Fatalf("Value: got %q, want %q", out.Value, "task/cover-legacy")
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
		{name: "task_cover_legacy", kind: "task", subtypes: []string{"cover-legacy"}, want: "task/cover-legacy"},
		{name: "task_redesign_system", kind: "task", subtypes: []string{"redesign-system"}, want: "task/redesign-system"},
		{name: "task_refactor_system", kind: "task", subtypes: []string{"refactor-system"}, want: "task/refactor-system"},
		{name: "task_refactor_tests", kind: "task", subtypes: []string{"refactor-tests"}, want: "task/refactor-tests"},
		{name: "task_onboard_external_system", kind: "task", subtypes: []string{"onboard-external-system"}, want: "task/onboard-external-system"},
		{name: "task_unrecognised_subtype_halts", kind: "task", subtypes: []string{"weird-subtype"}, expectErr: true},
		{name: "task_no_subtype_halts", kind: "task", subtypes: nil, expectErr: true},
		{name: "task_multiple_subtypes_halts", kind: "task", subtypes: []string{"cover-legacy", "refactor-system"}, expectErr: true},
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
			ctx.Set("issue_url", "https://example/1")
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
	ctx.Set("issue_url", "https://example/1")
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
		t.Fatalf("expected err for missing issue_url, got %+v", out)
	}
}
