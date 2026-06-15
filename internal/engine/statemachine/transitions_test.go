// Predicate-evaluator unit tests for the generic statemachine engine.
//
// These exercise evalPredicate directly against an in-memory Context and do
// not load any concrete process document, so they live in the engine package.
package statemachine

import (
	"testing"
)

func TestPredicate_EmptyAlwaysTrue(t *testing.T) {
	ctx := NewContext()
	got, err := evalPredicate("", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("empty predicate: got false, want true")
	}
}

func TestPredicate_Equality(t *testing.T) {
	cases := []struct {
		state    map[string]any
		expr     string
		want     bool
		wantErr  bool
		caseName string
	}{
		{map[string]any{"ticket_type": "story"}, "ticket_type == story", true, false, "bare value matches"},
		{map[string]any{"ticket_type": "story"}, `ticket_type == "story"`, true, false, "quoted value matches"},
		{map[string]any{"ticket_type": "bug"}, "ticket_type == story", false, false, "mismatch returns false"},
		{map[string]any{}, "ticket_type == story", false, false, "missing key treated as empty string"},
	}
	for _, tc := range cases {
		t.Run(tc.caseName, func(t *testing.T) {
			ctx := NewContext()
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			got, err := evalPredicate(tc.expr, ctx)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("evalPredicate(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestPredicate_BoolEquality(t *testing.T) {
	ctx := NewContext()
	ctx.Set("approval-outcome", true)
	got, err := evalPredicate("approval-outcome == true", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("bool equality: got false, want true")
	}
}

func TestPredicate_InList(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "bug")
	got, err := evalPredicate("ticket_type in [story, bug]", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("`in` membership: got false, want true")
	}
}

func TestPredicate_InListNegative(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "spike")
	got, err := evalPredicate("ticket_type in [story, bug]", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if got {
		t.Errorf("`in` non-membership: got true, want false")
	}
}
