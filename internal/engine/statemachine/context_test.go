package statemachine

import "testing"

// TestGetString_CoercionParity locks the invariant behind rehearsal #72: the
// predicate-read path (Context.GetString) and the substitution path
// (coerceStateValue / ExpandParams) must render a value of a given type
// identically. They now share coerceValueToString, so for the same value
// GetString and coerceStateValue can never silently diverge — the divergence
// that rendered a `[]string` as "[a b c]" on the GetString side but "a,b,c" on
// the substitution side, collapsing a comma-split read into one junk token.
func TestGetString_CoercionParity(t *testing.T) {
	cases := []struct {
		name string
		val  any
		want string
	}{
		{"string", "hello", "hello"},
		{"bool-true", true, "true"},
		{"bool-false", false, "false"},
		{"string-slice", []string{"a", "b", "c"}, "a,b,c"},
		{"string-slice-single", []string{"shouldRejectQty100"}, "shouldRejectQty100"},
		{"any-slice-of-strings", []any{"a", "b", "c"}, "a,b,c"},
		{"int", 7, "7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := NewContext()
			ctx.Set("k", tc.val)
			got := ctx.GetString("k")
			if got != tc.want {
				t.Errorf("GetString(%v) = %q, want %q", tc.val, got, tc.want)
			}
			// The two paths must agree for the same value.
			if sub := coerceStateValue(tc.val); sub != got {
				t.Errorf("divergence: GetString=%q but coerceStateValue=%q for %v", got, sub, tc.val)
			}
		})
	}
}

// TestGetString_MissingKey returns the empty string for an unset key.
func TestGetString_MissingKey(t *testing.T) {
	ctx := NewContext()
	if got := ctx.GetString("absent"); got != "" {
		t.Errorf("GetString(absent) = %q, want \"\"", got)
	}
}
