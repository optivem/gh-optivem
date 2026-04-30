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

// TestContentReplacementsRenamesBumpPatchVersion asserts that the per-flavor
// bump-patch-version workflow name and concurrency group get rewritten to the
// scaffolded name `bump-patch-version` (matching the renamed file). Regression
// guard for student repos receiving a `bump-patch-version.yml` whose internal
// `name:` and `concurrency.group:` still reference the shop flavor variant.
func TestContentReplacementsRenamesBumpPatchVersion(t *testing.T) {
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
		{
			name:     "multitier-dotnet name",
			in:       "name: multitier-dotnet-bump-patch-version\n",
			pairs:    multitierContentReplacements("dotnet", "react", "dotnet"),
			expected: "name: bump-patch-version\n",
		},
		{
			name:     "multitier-java concurrency group",
			in:       "  group: multitier-java-bump-patch-version\n",
			pairs:    multitierContentReplacements("java", "react", "java"),
			expected: "  group: bump-patch-version\n",
		},
		{
			name:     "multitier-typescript name",
			in:       "name: multitier-typescript-bump-patch-version\n",
			pairs:    multitierContentReplacements("typescript", "react", "typescript"),
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

// TestContentReplacementsRewritesBumpPatchVersionUsesReference is a regression
// guard for the filename form `bump-patch-version-<flavor>.yml` used inside
// `uses: ./.github/workflows/...` references in prod-stage. The previous rule
// only caught the `name:` / `concurrency.group:` form (`<flavor>-bump-patch-version`);
// without this reverse-order rule the scaffolded prod-stage kept the shop
// filename in its `uses:` line and `gh workflow run` failed at dispatch with
// HTTP 422 "workflow was not found". See gh-optivem run 25158207965 phase 8.
func TestContentReplacementsRewritesBumpPatchVersionUsesReference(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name:     "monolith-dotnet uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-monolith-dotnet.yml\n",
			pairs:    monolithContentReplacements("dotnet", "dotnet"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "monolith-java uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-monolith-java.yml\n",
			pairs:    monolithContentReplacements("java", "java"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "monolith-typescript uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-monolith-typescript.yml\n",
			pairs:    monolithContentReplacements("typescript", "typescript"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "multitier-dotnet uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-multitier-dotnet.yml\n",
			pairs:    multitierContentReplacements("dotnet", "react", "dotnet"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "multitier-java uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-multitier-java.yml\n",
			pairs:    multitierContentReplacements("java", "react", "java"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "multitier-typescript uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-multitier-typescript.yml\n",
			pairs:    multitierContentReplacements("typescript", "react", "typescript"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		// Polyglot cases (lang != testLang). Source prod-stage is the testLang flavor
		// and references `bump-patch-version-<arch>-<testLang>.yml`, so the rewrite
		// must key off testLang, not the system lang.
		{
			name:     "monolith polyglot java/typescript uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-monolith-typescript.yml\n",
			pairs:    monolithContentReplacements("java", "typescript"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "monolith polyglot dotnet/typescript uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-monolith-typescript.yml\n",
			pairs:    monolithContentReplacements("dotnet", "typescript"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "multitier polyglot java/typescript uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-multitier-typescript.yml\n",
			pairs:    multitierContentReplacements("java", "react", "typescript"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
		},
		{
			name:     "multitier polyglot dotnet/typescript uses reference",
			in:       "    uses: ./.github/workflows/bump-patch-version-multitier-typescript.yml\n",
			pairs:    multitierContentReplacements("dotnet", "react", "typescript"),
			expected: "    uses: ./.github/workflows/bump-patch-version.yml\n",
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

// TestContentReplacementsFlattensDockerCLIArgs asserts that unprefixed
// docker/<testLang>/<arch>/ paths embedded in `run:` CLI args (e.g. the
// per-suite acceptance-stage workflow's `gh optivem test system --system
// docker/<lang>/<arch>/system.json`) get flattened to `docker/`, matching the
// scaffolder's flattened on-disk layout. Regression guard for the per-suite
// migration in shop where these paths bypass the existing `working-directory:`
// rule.
func TestContentReplacementsFlattensDockerCLIArgs(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name:     "monolith-dotnet --system",
			in:       "    run: gh optivem test system --system docker/dotnet/monolith/system.json --tests system-test/dotnet/tests-latest.json\n",
			pairs:    monolithContentReplacements("dotnet", "dotnet"),
			expected: "    run: gh optivem test system --system docker/system.json --tests system-test/tests-latest.json\n",
		},
		{
			name:     "monolith-java --system",
			in:       "    run: gh optivem test system --system docker/java/monolith/system.json\n",
			pairs:    monolithContentReplacements("java", "java"),
			expected: "    run: gh optivem test system --system docker/system.json\n",
		},
		{
			name:     "monolith-typescript --system",
			in:       "    run: gh optivem test system --system docker/typescript/monolith/system.json\n",
			pairs:    monolithContentReplacements("typescript", "typescript"),
			expected: "    run: gh optivem test system --system docker/system.json\n",
		},
		{
			name:     "multitier-dotnet --system",
			in:       "    run: gh optivem test system --system docker/dotnet/multitier/system.json --tests system-test/dotnet/tests-latest.json\n",
			pairs:    multitierContentReplacements("dotnet", "react", "dotnet"),
			expected: "    run: gh optivem test system --system docker/system.json --tests system-test/tests-latest.json\n",
		},
		{
			name:     "multitier-java --system",
			in:       "    run: gh optivem test system --system docker/java/multitier/system.json\n",
			pairs:    multitierContentReplacements("java", "react", "java"),
			expected: "    run: gh optivem test system --system docker/system.json\n",
		},
		{
			name:     "multitier-typescript --system",
			in:       "    run: gh optivem test system --system docker/typescript/multitier/system.json\n",
			pairs:    multitierContentReplacements("typescript", "react", "typescript"),
			expected: "    run: gh optivem test system --system docker/system.json\n",
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

// TestMonolithCrossLangVersionPathCollapsesToRoot asserts that for cross-lang
// monolith scaffolds (lang != testLang) the testLang-flavored
// system/monolith/<testLang>/VERSION path embedded in the testLang pipeline
// stage workflows (acceptance/qa/prod) collapses to root VERSION. Regression
// guard for scaffolded acceptance-stage shipping with `file:
// system/monolith/typescript/VERSION` against a Java monolith repo (where
// CopyDir flattened system/monolith/java/ to system/ and CopyVersion put the
// VERSION at root).
func TestMonolithCrossLangVersionPathCollapsesToRoot(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name:     "java system, typescript tests",
			in:       "          file: system/monolith/typescript/VERSION\n",
			pairs:    monolithContentReplacements("java", "typescript"),
			expected: "          file: VERSION\n",
		},
		{
			name:     "dotnet system, java tests",
			in:       "          file: system/monolith/java/VERSION\n",
			pairs:    monolithContentReplacements("dotnet", "java"),
			expected: "          file: VERSION\n",
		},
		{
			name:     "typescript system, dotnet tests",
			in:       "          file: system/monolith/dotnet/VERSION\n",
			pairs:    monolithContentReplacements("typescript", "dotnet"),
			expected: "          file: VERSION\n",
		},
		{
			name:     "lang VERSION still collapses (commit-stage path) for cross-lang",
			in:       "          file: system/monolith/java/VERSION\n",
			pairs:    monolithContentReplacements("java", "typescript"),
			expected: "          file: VERSION\n",
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

// TestSystemPrefixDropReplacementsCollapsesPerComponent3SegmentTags asserts
// that systemPrefixDropReplacements collapses 3-segment per-component
// release-tag prefixes (introduced when multitier prod-stage started
// publishing per-component git tags alongside the flavor tag) — both in
// `tag:` step inputs (prod-stage publish-tag) and in `value:` JSON entries
// (bump-patch-version git-tag signal).
func TestSystemPrefixDropReplacementsCollapsesPerComponent3SegmentTags(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name:     "multitier-backend-dotnet tag in prod-stage",
			in:       "          tag: multitier-backend-dotnet-v${{ steps.x.outputs.backend }}\n",
			pairs:    systemPrefixDropReplacements("multitier-backend-dotnet"),
			expected: "          tag: v${{ steps.x.outputs.backend }}\n",
		},
		{
			name:     "multitier-frontend-react tag in prod-stage",
			in:       "          tag: multitier-frontend-react-v${{ steps.x.outputs.frontend }}\n",
			pairs:    systemPrefixDropReplacements("multitier-frontend-react"),
			expected: "          tag: v${{ steps.x.outputs.frontend }}\n",
		},
		{
			name:     "multitier-backend-java git-tag value in bump-patch-version",
			in:       `              {"path": "system/multitier/backend-java/VERSION", "value": "multitier-backend-java-v"}` + "\n",
			pairs:    systemPrefixDropReplacements("multitier-backend-java"),
			expected: `              {"path": "system/multitier/backend-java/VERSION", "value": "v"}` + "\n",
		},
		{
			name: "multi-prefix variadic call collapses both backend and frontend",
			in: `              {"path": "system/multitier/backend-typescript/VERSION", "value": "multitier-backend-typescript-v"},
              {"path": "system/multitier/frontend-react/VERSION",     "signal": "git-tag", "value": "multitier-frontend-react-v"}` + "\n",
			pairs: systemPrefixDropReplacements(
				"multitier-typescript",
				"multitier-backend-typescript",
				"multitier-frontend-react",
			),
			expected: `              {"path": "system/multitier/backend-typescript/VERSION", "value": "v"},
              {"path": "system/multitier/frontend-react/VERSION",     "signal": "git-tag", "value": "v"}` + "\n",
		},
		{
			name:     "2-segment monolith form unchanged (regression guard)",
			in:       "          tag: monolith-typescript-v${{ steps.x.outputs.version }}\n",
			pairs:    systemPrefixDropReplacements("monolith-typescript"),
			expected: "          tag: v${{ steps.x.outputs.version }}\n",
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

// TestMultitierPrefixDropAndContentReplacementsOrderingFixesPartialMatch
// asserts that prepending systemPrefixDropReplacements (with the 3-segment
// per-component prefixes) before multitierContentReplacements collapses
// "multitier-backend-{lang}-v" / "multitier-frontend-react-v" to "v" instead
// of letting the bare "multitier-backend-{lang}" → "backend" /
// "multitier-frontend-react" → "frontend" rules in
// multitierContentReplacements partial-match them into "backend-v" /
// "frontend-v". Mirrors the call-site composition in applyMultitierMonorepo
// and applyMultitierMultirepo (root repo).
func TestMultitierPrefixDropAndContentReplacementsOrderingFixesPartialMatch(t *testing.T) {
	cases := []struct {
		name         string
		in           string
		backendLang  string
		frontendLang string
		testLang     string
		expected     string
	}{
		{
			name:         "multitier-dotnet bump-patch-version git-tag values",
			in:           `              {"path": "system/multitier/backend-dotnet/VERSION", "value": "multitier-backend-dotnet-v"},` + "\n              " + `{"path": "system/multitier/frontend-react/VERSION",  "signal": "git-tag", "value": "multitier-frontend-react-v"}` + "\n",
			backendLang:  "dotnet",
			frontendLang: "react",
			testLang:     "dotnet",
			expected:     `              {"path": "backend/VERSION", "value": "v"},` + "\n              " + `{"path": "frontend/VERSION",  "signal": "git-tag", "value": "v"}` + "\n",
		},
		{
			name:         "multitier-typescript prod-stage per-component publish-tag",
			in:           "          tag: multitier-backend-typescript-v${{ steps.x.outputs.backend }}\n          tag: multitier-frontend-react-v${{ steps.x.outputs.frontend }}\n",
			backendLang:  "typescript",
			frontendLang: "react",
			testLang:     "typescript",
			expected:     "          tag: v${{ steps.x.outputs.backend }}\n          tag: v${{ steps.x.outputs.frontend }}\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Compose pairs in the same order as applyMultitierMonorepo /
			// applyMultitierMultirepo: prefix-drops first, then content.
			pairs := append(
				systemPrefixDropReplacements(
					"multitier-"+tc.testLang,
					"multitier-backend-"+tc.backendLang,
					"multitier-frontend-"+tc.frontendLang,
				),
				multitierContentReplacements(tc.backendLang, tc.frontendLang, tc.testLang)...,
			)
			got := tc.in
			for _, p := range pairs {
				got = strings.ReplaceAll(got, p[0], p[1])
			}
			if got != tc.expected {
				t.Errorf("got  %q\nwant %q", got, tc.expected)
			}
		})
	}
}

// TestMultirepoBackendAndFrontendReplacementsCollapseGitTagValue asserts
// that the backendReplacements / frontendReplacements lists composed in
// applyMultitierMultirepo (per-component repo path) collapse the 3-segment
// "multitier-backend-{lang}-v" / "multitier-frontend-react-v" git-tag value
// in bump-patch-version.yml to "v". Without the prepended
// systemPrefixDropReplacements call, the bare "multitier-backend-{lang}" →
// "backend" rule would partial-match and leave a "backend-v" fragment.
func TestMultirepoBackendAndFrontendReplacementsCollapseGitTagValue(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		pairs    [][2]string
		expected string
	}{
		{
			name: "backend repo (multitier-backend-dotnet)",
			in:   `              {"path": "system/multitier/backend-dotnet/VERSION", "value": "multitier-backend-dotnet-v"}` + "\n",
			// Mirrors backendReplacements composition in applyMultitierMultirepo.
			pairs: append(
				systemPrefixDropReplacements("multitier-backend-dotnet"),
				[2]string{"multitier-backend-dotnet-commit-stage", "backend-commit-stage"},
				[2]string{"system/multitier/backend-dotnet", "backend"},
				[2]string{"multitier-backend-dotnet", "backend"},
				[2]string{"backend-bump-patch-version", "bump-patch-version"},
			),
			expected: `              {"path": "backend/VERSION", "value": "v"}` + "\n",
		},
		{
			name: "frontend repo (multitier-frontend-react)",
			in:   `              {"path": "system/multitier/frontend-react/VERSION", "value": "multitier-frontend-react-v"}` + "\n",
			// Mirrors frontendReplacements composition in applyMultitierMultirepo.
			pairs: append(
				systemPrefixDropReplacements("multitier-frontend-react"),
				[2]string{"multitier-frontend-react-commit-stage", "frontend-commit-stage"},
				[2]string{"system/multitier/frontend-react", "frontend"},
				[2]string{"multitier-frontend-react", "frontend"},
				[2]string{"frontend-bump-patch-version", "bump-patch-version"},
			),
			expected: `              {"path": "frontend/VERSION", "value": "v"}` + "\n",
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
