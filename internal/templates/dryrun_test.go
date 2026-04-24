package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStripSystemTestReadme_AgainstShop exercises the README rewrite against
// shop's real system-test/<lang>/README.md files. After stripping, the output
// must (a) contain no `-Architecture` flag, (b) contain no mention of the
// removed arch, and (c) still contain the other usage blocks (Legacy, Suite,
// Rebuild) exactly once.
func TestStripSystemTestReadme_AgainstShop(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(wd)))

	cases := []struct {
		testLang           string
		keepArch, rmArch   string
	}{
		{"typescript", "monolith", "multitier"},
		{"typescript", "multitier", "monolith"},
		{"java", "monolith", "multitier"},
		{"java", "multitier", "monolith"},
		{"dotnet", "monolith", "multitier"},
		{"dotnet", "multitier", "monolith"},
	}

	for _, tc := range cases {
		t.Run(tc.testLang+"_"+tc.keepArch, func(t *testing.T) {
			// Copy shop's README.md into a tmp testDst so stripSystemTestReadme
			// can write to it without mutating shop.
			shopReadme := filepath.Join(root, "shop", "system-test", tc.testLang, "README.md")
			data, err := os.ReadFile(shopReadme)
			if err != nil {
				t.Skipf("shop not available at %s", shopReadme)
			}
			testDst := t.TempDir()
			readmePath := filepath.Join(testDst, "README.md")
			if err := os.WriteFile(readmePath, data, 0644); err != nil {
				t.Fatal(err)
			}

			stripSystemTestReadme(testDst, tc.keepArch, tc.rmArch)

			got, err := os.ReadFile(readmePath)
			if err != nil {
				t.Fatal(err)
			}
			out := string(got)

			forbidden := []string{
				"-Architecture",
				"(" + tc.rmArch + ")",
				"(" + tc.keepArch + ")", // parenthetical on heading simplified away
				"./Run-SystemTests.ps1 -Architecture",
			}
			for _, f := range forbidden {
				if strings.Contains(out, f) {
					t.Errorf("output still contains forbidden %q", f)
				}
			}

			required := []string{
				"Run all latest test suites:",
				"./Run-SystemTests.ps1\n",
				"./Run-SystemTests.ps1 -Legacy\n",
				"./Run-SystemTests.ps1 -Suite acceptance-api",
				"./Run-SystemTests.ps1 -Rebuild",
			}
			for _, r := range required {
				if !strings.Contains(out, r) {
					t.Errorf("output missing required %q", r)
				}
			}

			if t.Failed() {
				t.Logf("--- rewritten README (%d bytes) ---\n%s", len(out), out)
			}
		})
	}
}

// TestStripRunSystemTestsPS1_AgainstShop exercises the full PS1 rewrite
// pipeline against shop's real Run-SystemTests.ps1 for each test-lang variant
// and arch choice (4 combinations), to catch any mismatch with shop's exact
// formatting. Skipped if shop isn't available (e.g. CI without sibling checkout).
func TestStripRunSystemTestsPS1_AgainstShop(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// wd is .../gh-optivem/internal/templates
	root := filepath.Dir(filepath.Dir(filepath.Dir(wd)))

	cases := []struct {
		testLang   string
		keepArch   string
		removeArch string
	}{
		{"typescript", "monolith", "multitier"},
		{"typescript", "multitier", "monolith"},
		{"java", "monolith", "multitier"},
		{"java", "multitier", "monolith"},
		{"dotnet", "monolith", "multitier"},
		{"dotnet", "multitier", "monolith"},
	}

	for _, tc := range cases {
		t.Run(tc.testLang+"_"+tc.keepArch, func(t *testing.T) {
			shopPS1 := filepath.Join(root, "shop", "system-test", tc.testLang, "Run-SystemTests.ps1")
			data, err := os.ReadFile(shopPS1)
			if err != nil {
				t.Skipf("shop not available at %s", shopPS1)
			}

			content := string(data)
			content = stripArchParameter(content)
			content = stripArchAutoDetect(content)
			content = stripArchFromComposeFilename(content)
			content = pruneAllSystemConfig(content, tc.keepArch, tc.removeArch)
			content = stripLangArchFromContainerNames(content, tc.keepArch, tc.testLang)
			content = stripArchFromSystemConfigLookup(content)
			content = stripArchFromDisplayHeading(content)

			forbidden := []string{
				`ValidateSet("multitier", "monolith")`,
				`$Architecture,`,
				`# Auto-detect architecture`,
				`"docker-compose.$Mode.$Architecture.$ExternalMode.yml"`,
				`"` + tc.removeArch + `" = @{`,
				`"` + tc.keepArch + `" = @{`,
				`shop-` + tc.testLang + `-` + tc.removeArch,
				`shop-` + tc.testLang + `-` + tc.keepArch,
				`$AllSystemConfig[$Architecture]`,
				`$Architecture /`, // display heading
			}
			for _, f := range forbidden {
				if strings.Contains(content, f) {
					t.Errorf("output still contains forbidden %q", f)
				}
			}

			required := []string{
				`$AllSystemConfig = @{`,
				`    "real" = @{`,
				`    "stub" = @{`,
				`ContainerName = "shop-real"`,
				`ContainerName = "shop-stub"`,
				`$SystemConfig = $AllSystemConfig`,
				`"docker-compose.$Mode.$ExternalMode.yml"`,
			}
			for _, r := range required {
				if !strings.Contains(content, r) {
					t.Errorf("output missing required %q", r)
				}
			}

			open := strings.Count(content, "{")
			close := strings.Count(content, "}")
			if open != close {
				t.Errorf("brace imbalance: %d open vs %d close", open, close)
			}

			if t.Failed() {
				t.Logf("--- rewritten PS1 (%d bytes) ---\n%s", len(content), content)
			}
		})
	}
}
