package parse

import "testing"

func TestFirstH1(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single h1", "# My Title\n\nbody\n", "My Title"},
		{"h1_after_text", "intro\n\n# Real\n", "Real"},
		{"no h1 only h2", "## Section\n\nbody\n", ""},
		{"empty", "", ""},
		{"h1_with_trailing_spaces", "#   spaced   \n", "spaced"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FirstH1(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
