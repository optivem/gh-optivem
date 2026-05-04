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
	for _, want := range []string{
		"story", "bug", "chore",
		"system-api-task", "system-ui-task", "external-api-task",
	} {
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
		"legacy_acceptance_criteria_section_present",
		"external_system_driver_exists",
		"external_system_test_instance_accessible",
		"smoke_test_passes",
		"structural_test_mode",
	}
	for _, name := range want {
		if r.Lookup(name) == nil {
			t.Errorf("binding %q not registered", name)
		}
	}
}
