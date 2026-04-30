package main

import "testing"

func TestParseIssueArg(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		in      string
		want    int
		wantErr bool
	}{
		{"bare number", "42", 42, false},
		{"bare number with whitespace", "  42  ", 42, false},
		{"with hash prefix", "#42", 42, false},
		{"github url", "https://github.com/optivem/shop/issues/61", 61, false},
		{"github url trailing slash", "https://github.com/optivem/shop/issues/61/", 61, false},
		{"short repo path", "optivem/shop/issues/7", 7, false},
		{"empty", "", 0, true},
		{"whitespace only", "   ", 0, true},
		{"non-numeric tail", "https://github.com/optivem/shop/pulls/foo", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-1", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIssueArg(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseIssueArg(%q): want error, got %d", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseIssueArg(%q): unexpected error %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("parseIssueArg(%q): want %d, got %d", tc.in, tc.want, got)
			}
		})
	}
}
