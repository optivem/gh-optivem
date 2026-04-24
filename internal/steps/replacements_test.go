package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

// TestReplaceRepoReferencesNoLeftoverShop asserts that after
// ReplaceRepoReferences runs over a repo seeded with the three patterns that
// survived replacement on run 2026-04-23 (optivem/gh-optivem#29), no "shop"
// substring remains anywhere in the tree.
//
// The failure surface is the state AFTER ApplyTemplate — i.e. after the
// sysapp-<lang> → system transform — fed into ReplaceRepoReferences.
func TestReplaceRepoReferencesNoLeftoverShop(t *testing.T) {
	cases := []struct {
		name string
		repo string
	}{
		{"single word", "horizon"},
		{"hyphenated two words", "blue-horizon"},
		{"long hyphenated with numeric suffix", "course-tester-atdd-typescript-20260423-192402"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoDir := t.TempDir()
			seedFailureFixtures(t, repoDir)

			cfg := &config.Config{
				Owner:        "valentinajemuovic",
				OwnerLower:   "valentinajemuovic",
				Repo:         tc.repo,
				FullRepo:     "valentinajemuovic/" + tc.repo,
				Arch:         "monolith",
				RepoStrategy: "monorepo",
				RepoDir:      repoDir,
			}

			ReplaceRepoReferences(cfg)

			leftovers := findShopLeftovers(t, repoDir)
			if len(leftovers) > 0 {
				t.Errorf("Leftover shop references after ReplaceRepoReferences:\n  %s",
					strings.Join(leftovers, "\n  "))
			}
		})
	}
}

// seedFailureFixtures writes the three files from the Apr 23 failure into
// repoDir. Each file also contains an optivem/actions reference so
// verifyActionsReferencesIntact's safety check passes.
func seedFailureFixtures(t *testing.T, repoDir string) {
	t.Helper()
	fixtures := map[string]string{
		".github/workflows/commit-stage.yml": `name: commit-stage
jobs:
  check:
    steps:
      - uses: optivem/actions/validate-env-vars-defined@v1
      - run: |
          sonar-scanner \
            -Dsonar.projectKey=optivem_shop-system \
            -Dsonar.projectName=shop-system
`,
		"system-test/docker-compose.pipeline.monolith.real.yml": `services:
  system:
    image: ghcr.io/optivem/shop/system:latest
`,
		"system-test/docker-compose.pipeline.monolith.stub.yml": `services:
  system:
    image: ghcr.io/optivem/shop/system:latest
`,
	}
	for rel, content := range fixtures {
		p := filepath.Join(repoDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
}

// findShopLeftovers returns paths (relative to repoDir) of files that still
// contain "shop", "Shop", or "SHOP" as a word (not preceded by a lowercase
// letter, to avoid "eshop" / "workshop" false positives).
func findShopLeftovers(t *testing.T, repoDir string) []string {
	t.Helper()
	var leftovers []string
	_ = filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := string(data)
		for _, needle := range []string{"shop", "Shop", "SHOP"} {
			if hasWordBoundaryMatch(content, needle) {
				rel, _ := filepath.Rel(repoDir, path)
				leftovers = append(leftovers, rel+": contains "+needle)
				break
			}
		}
		return nil
	})
	return leftovers
}

func hasWordBoundaryMatch(content, needle string) bool {
	idx := 0
	for {
		pos := strings.Index(content[idx:], needle)
		if pos < 0 {
			return false
		}
		absPos := idx + pos
		if absPos == 0 || !isLowerLetter(content[absPos-1]) {
			return true
		}
		idx = absPos + len(needle)
	}
}

func isLowerLetter(c byte) bool {
	return c >= 'a' && c <= 'z'
}

// TestContentReplacementsStripsEnvPrefix asserts that monolith/multitier
// content replacements produce pairs that strip the arch-lang prefix from
// `environment: <name>` references. Regression guard for the Apr 17 scaffolds
// where workflows shipped with `environment: monolith-dotnet-acceptance`
// instead of the bare `acceptance` expected by GitHub's environment config.
func TestContentReplacementsStripsEnvPrefix(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name:     "monolith-dotnet acceptance",
			in:       "    environment: monolith-dotnet-acceptance\n",
			pairs:    monolithContentReplacements("dotnet", "dotnet"),
			expected: "    environment: acceptance\n",
		},
		{
			name:     "monolith-java qa",
			in:       "    environment: monolith-java-qa\n",
			pairs:    monolithContentReplacements("java", "java"),
			expected: "    environment: qa\n",
		},
		{
			name:     "monolith-typescript production",
			in:       "    environment: monolith-typescript-production\n",
			pairs:    monolithContentReplacements("typescript", "typescript"),
			expected: "    environment: production\n",
		},
		{
			name:     "multitier-dotnet acceptance",
			in:       "    environment: multitier-dotnet-acceptance\n",
			pairs:    multitierContentReplacements("dotnet", "react", "dotnet"),
			expected: "    environment: acceptance\n",
		},
		{
			name:     "multitier-java qa",
			in:       "    environment: multitier-java-qa\n",
			pairs:    multitierContentReplacements("java", "react", "java"),
			expected: "    environment: qa\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in
			for _, p := range tc.pairs {
				got = strings.ReplaceAll(got, p[0], p[1])
			}
			if got != tc.expected {
				t.Errorf("got  %q\nwant %q", got, tc.expected)
			}
		})
	}
}
