package shell

import (
	"reflect"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{
			name: "simple words",
			in:   "git status",
			want: []string{"git", "status"},
		},
		{
			name: "double-quoted message",
			in:   `git commit -m "hello world"`,
			want: []string{"git", "commit", "-m", "hello world"},
		},
		{
			name: "single-quoted literal",
			in:   `echo 'a b c'`,
			want: []string{"echo", "a b c"},
		},
		{
			// Regression: fmt.Sprintf("git commit -m %q", msg) emits \" for
			// embedded quotes; without escape handling, splitCommand used to
			// terminate the quoted run early and git received the rest as
			// pathspecs, failing with "pathspec did not match any file(s)".
			name: "double-quoted with escaped quote",
			in:   `git commit -m "msg with \"inner\" quotes"`,
			want: []string{"git", "commit", "-m", `msg with "inner" quotes`},
		},
		{
			name: "double-quoted with escaped backslash",
			in:   `cmd "a\\b"`,
			want: []string{"cmd", `a\b`},
		},
		{
			name:    "unterminated double quote",
			in:      `cmd "oops`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitCommand(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil; parts=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

