package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

// TestReplaceNamespacesAndSystemNameGeneric asserts that the generic
// placeholder passes rewrite content in all text files, filenames, and
// directories for every casing variant, without needing any language-specific
// rules.
func TestReplaceNamespacesAndSystemNameGeneric(t *testing.T) {
	const owner = "sky-traveler"
	repoDir := t.TempDir()
	seedGenericFixtures(t, repoDir)

	cfg := &config.Config{
		Owner:          owner,
		OwnerLower:     owner,
		Repo:           "sky-travel",
		FullRepo:       owner + "/sky-travel",
		Arch:           "monolith",
		RepoStrategy:   "monorepo",
		RepoDir:        repoDir,
		OwnerCasings:   config.OwnerCasings(owner),
		SysNameCasings: config.SystemCasings("Sky Travel"),
	}

	ReplaceNamespaces(cfg)
	ReplaceSystemName(cfg)

	assertNoLiteralSurvives(t, repoDir, []string{
		"MyCompany", "myCompany", "my-company", "mycompany", "my_company", "MY_COMPANY",
		"MyShop", "myShop", "my-shop", "myshop", "my_shop", "MY_SHOP",
	})

	// Spot-check a handful of the rewritten values.
	expect := map[string]string{
		filepath.Join("system", "Program.cs"):                       "SkyTraveler.SkyTravel.Monolith",
		filepath.Join("src", "main", "java", "com", "skytraveler", "skytravel", "Main.java"): "package com.skytraveler.skytravel;",
		filepath.Join("system-test", "src", "skyTravel", "dsl.ts"):  "class SkyTravelDsl",
		filepath.Join("SkyTraveler.SkyTravel.Monolith.sln"):         "SkyTraveler.SkyTravel.Monolith",
		filepath.Join("sky-travel-api-driver.ts"):                   "const skyTravelUiBaseUrl",
	}
	for rel, substr := range expect {
		full := filepath.Join(repoDir, rel)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Errorf("expected file %s to exist: %v", rel, err)
			continue
		}
		if !strings.Contains(string(data), substr) {
			t.Errorf("file %s missing expected substring %q; got:\n%s", rel, substr, string(data))
		}
	}
}

func seedGenericFixtures(t *testing.T, repoDir string) {
	t.Helper()
	fixtures := map[string]string{
		// Content: .NET namespace, Java package, TS identifier.
		"system/Program.cs":                                        "namespace MyCompany.MyShop.Monolith;",
		"src/main/java/com/mycompany/myshop/Main.java":             "package com.mycompany.myshop;",
		"system-test/src/myShop/dsl.ts":                            "class MyShopDsl {}",
		// Filenames to be renamed.
		"MyCompany.MyShop.Monolith.sln":                            "MyCompany.MyShop.Monolith contents",
		"my-shop-api-driver.ts":                                    "const myShopUiBaseUrl = '/';",
		// Env-style and snake-style variants.
		".env":                                                     "DB_PREFIX=my_shop\nAPP=MY_SHOP",
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

func assertNoLiteralSurvives(t *testing.T, repoDir string, literals []string) {
	t.Helper()
	filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(repoDir, path)
		content := string(data)
		for _, lit := range literals {
			if strings.Contains(content, lit) {
				t.Errorf("leftover placeholder %q in file %s", lit, rel)
			}
			if strings.Contains(rel, lit) {
				t.Errorf("leftover placeholder %q in path %s", lit, rel)
			}
		}
		return nil
	})
}

// TestRewritePublisherRefsSonar asserts the hardcoded publisher-real passes
// rewrite optivem/shop, optivem_shop, and sonar.organization for a fresh
// scaffold (owner != optivem). Covers the three Apr-23 failure fixtures plus
// the 2026-04-24 revert state where Sonar identifiers stay optivem_shop-*.
func TestRewritePublisherRefsSonar(t *testing.T) {
	cases := []struct {
		name string
		repo string
	}{
		{"single word", "horizon"},
		{"hyphenated two words", "blue-horizon"},
		{"multirepo backend suffix", "blue-horizon-backend"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoDir := t.TempDir()
			seedPublisherFixtures(t, repoDir)

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

			assertNoLiteralSurvives(t, repoDir, []string{
				"optivem/shop",
				"optivem_shop",
				"api.optivem.com",
				"@optivem/shop-system-test",
			})
		})
	}
}

func seedPublisherFixtures(t *testing.T, repoDir string) {
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
            -Dsonar.projectName=shop-system \
            -Dsonar.organization=optivem
`,
		"docker/docker-compose.pipeline.monolith.real.yml": `services:
  system:
    image: ghcr.io/optivem/shop/system:latest
`,
		"system-test/typescript/package-lock.json": `{"name": "@optivem/shop-system-test", "version": "1.0.0"}`,
		"README.md": `See https://api.optivem.com/errors/validation`,
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

// TestContentReplacementsRenamesAutoBumpPatch asserts that the per-flavor
// bump-patch-version workflow name and concurrency group get rewritten to the
// scaffolded name `bump-patch-version` (matching the renamed file). Regression
// guard for student repos receiving an `bump-patch-version.yml` whose internal
// `name:` and `concurrency.group:` still reference the shop flavor variant.
func TestContentReplacementsRenamesAutoBumpPatch(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name:     "monolith-dotnet name",
			in:       "name: monolith-dotnet-bump-patch-version\n",
			pairs:    monolithContentReplacements("dotnet", "dotnet"),
			expected: "name: bump-patch-version\n",
		},
		{
			name:     "monolith-java concurrency group",
			in:       "  group: monolith-java-bump-patch-version\n",
			pairs:    monolithContentReplacements("java", "java"),
			expected: "  group: bump-patch-version\n",
		},
		{
			name:     "monolith-typescript name",
			in:       "name: monolith-typescript-bump-patch-version\n",
			pairs:    monolithContentReplacements("typescript", "typescript"),
			expected: "name: bump-patch-version\n",
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
