package version

import "testing"

func TestParseGhVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want [3]int
		err  bool
	}{
		{"typical", "gh version 2.92.0 (2026-04-28)\nhttps://github.com/cli/cli/releases/tag/v2.92.0\n", [3]int{2, 92, 0}, false},
		{"older", "gh version 2.80.0 (2025-09-23)\n", [3]int{2, 80, 0}, false},
		{"non gh tool", "git version 2.48.1\n", [3]int{}, true},
		{"empty", "", [3]int{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGhVersion(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b [3]int
		want int
	}{
		{[3]int{2, 92, 0}, [3]int{2, 92, 0}, 0},
		{[3]int{2, 80, 0}, [3]int{2, 92, 0}, -1},
		{[3]int{2, 92, 1}, [3]int{2, 92, 0}, 1},
		{[3]int{3, 0, 0}, [3]int{2, 92, 0}, 1},
		{[3]int{1, 99, 99}, [3]int{2, 0, 0}, -1},
	}
	for _, tc := range cases {
		got := compareSemver(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareSemver(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMinGhCLIVersionParses(t *testing.T) {
	if _, err := parseSemver(MinGhCLIVersion); err != nil {
		t.Fatalf("MinGhCLIVersion %q is not parseable: %v", MinGhCLIVersion, err)
	}
}
