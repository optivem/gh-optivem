package files

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindInTreeWordBoundaryExcept verifies that legitimate occurrences of a
// needle inside known surrounding patterns are ignored, while truly leftover
// occurrences are still reported.
func TestFindInTreeWordBoundaryExcept(t *testing.T) {
	type file struct {
		rel, body string
	}
	allowed := []string{
		"optivem/actions",
		"@optivem/",
		"optivem-testing",
		".optivem",
		"api.optivem.com",
	}

	cases := []struct {
		name       string
		files      []file
		wantHit    bool
		wantInPath string
	}{
		{
			name:    "optivem/actions ref is allowed",
			files:   []file{{"wf.yml", "uses: optivem/actions/foo@v1\n"}},
			wantHit: false,
		},
		{
			name:    "@optivem/ npm scope is allowed",
			files:   []file{{"a.ts", "import {x} from '@optivem/optivem-testing';\n"}},
			wantHit: false,
		},
		{
			name:    "registry URL form is allowed",
			files:   []file{{"pkg-lock.json", `"resolved": "https://registry.npmjs.org/@optivem/optivem-testing/-/optivem-testing-1.1.8.tgz"`}},
			wantHit: false,
		},
		{
			name:    ".optivem metadata dir is allowed",
			files:   []file{{"run.ps1", `$cfg = Join-Path $root ".optivem" "config.json"`}},
			wantHit: false,
		},
		{
			name:    "api.optivem.com branded URL is allowed",
			files:   []file{{"errors.ts", "const BASE = 'https://api.optivem.com/errors';\n"}},
			wantHit: false,
		},
		{
			name:    "word-boundary still excludes eshop",
			files:   []file{{"notes.md", "deprecated eshop-tests project\n"}},
			wantHit: false,
		},
		{
			name:       "bare optivem in unrecognized context is flagged",
			files:      []file{{"stale.md", "owner: optivem\n"}},
			wantHit:    true,
			wantInPath: "stale.md",
		},
		{
			name: "mix: allowed refs plus one real leftover → flag only the real one",
			files: []file{
				{"wf.yml", "uses: optivem/actions/foo@v1\n"},
				{"bad.md", "copyright optivem, 2025\n"},
			},
			wantHit:    true,
			wantInPath: "bad.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				full := filepath.Join(dir, f.rel)
				if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(full, []byte(f.body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			got := FindInTreeWordBoundaryExcept(dir, "optivem", allowed)
			if tc.wantHit {
				if len(got) == 0 {
					t.Fatalf("expected a match in %q, got none", tc.wantInPath)
				}
				found := false
				for _, m := range got {
					if filepath.ToSlash(m) == tc.wantInPath {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected match in %q, got %v", tc.wantInPath, got)
				}
			} else if len(got) != 0 {
				t.Fatalf("expected no matches, got %v", got)
			}
		})
	}
}
