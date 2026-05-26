package parse

import "testing"

func TestExtractSection(t *testing.T) {
	body := "Intro paragraph above.\n\n" +
		"## Description\n\nDesc body line 1.\nDesc body line 2.\n\n" +
		"## Acceptance Criteria\n\n- AC1\n- AC2\n\n" +
		"### Sub of AC\n\nnested.\n\n" +
		"## Checklist\n\n- [ ] One\n- [ ] Two\n"

	cases := []struct {
		name    string
		heading string
		want    string
	}{
		{"description", "Description", "Desc body line 1.\nDesc body line 2."},
		{"acceptance_with_h3_sub", "Acceptance Criteria", "- AC1\n- AC2\n\n### Sub of AC\n\nnested."},
		{"checklist_tail", "Checklist", "- [ ] One\n- [ ] Two"},
		{"case_insensitive", "checklist", "- [ ] One\n- [ ] Two"},
		{"missing", "Nope", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractSection(body, tc.heading)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractSection_IgnoresH1(t *testing.T) {
	// H1 is the document title; section names refer to H2/H3 only,
	// so a heading at depth 1 must not anchor an H2 lookup.
	body := "# Top\n\nbody\n\n## Real Section\n\nreal body\n"
	if got := ExtractSection(body, "Top"); got != "" {
		t.Errorf("H1 must not match a section lookup: got %q", got)
	}
	if got := ExtractSection(body, "Real Section"); got != "real body" {
		t.Errorf("H2 lookup: got %q", got)
	}
}

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
