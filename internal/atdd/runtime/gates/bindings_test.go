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
	for _, tc := range []struct {
		name    string
		answer  string
		want    bool
		wantErr bool
	}{
		{name: "yes_lower", answer: "y", want: true},
		{name: "yes_word", answer: "yes\n", want: true},
		{name: "no_lower", answer: "n", want: false},
		{name: "empty_is_no", answer: "\n", want: false},
		{name: "garbage", answer: "maybe", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakePrompter{answers: []string{tc.answer}}
			b := newBindings(t, Deps{Prompter: p})
			ctx := statemachine.NewContext()
			out := b.dslInterfaceChanged(ctx)
			if (out.Err != nil) != tc.wantErr {
				t.Fatalf("err: got %v, wantErr=%v", out.Err, tc.wantErr)
			}
			if out.Err != nil {
				return
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
		"system-implementation-change",
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
// the post-VERIFY_STRUCT_DRIVER edge based on the failure class the
// verify action stamped into ctx.State["verify_class"], and is the only
// place the one-retry budget is enforced.
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
			ctx.Set("issue_repo", "optivem/shop")
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

func TestExtractIssueBody_Variants(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "simple", raw: `{"body":"hello"}`, want: "hello"},
		{name: "with_newline", raw: `{"body":"line1\nline2"}`, want: "line1\nline2"},
		{name: "escaped_quote", raw: `{"body":"say \"hi\""}`, want: `say "hi"`},
		{name: "no_body_key", raw: `{"title":"x"}`, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := extractIssueBody([]byte(tc.raw))
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

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
		"external_system_driver_exists",
		"external_system_test_instance_accessible",
		"smoke_test_passes",
		"structural_test_mode",
		"compile_ok",
		"tests_failed_runtime",
		"verify_real_required",
		"verify_real_pass",
		"structural_verify_outcome",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("binding %q not registered", name)
		}
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
