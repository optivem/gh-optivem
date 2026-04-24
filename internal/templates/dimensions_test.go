package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStripArchParameter verifies that the [ValidateSet] + $Architecture
// parameter is removed cleanly without touching other params.
func TestStripArchParameter(t *testing.T) {
	in := `param(
    [ValidateSet("multitier", "monolith")]
    [string]$Architecture,

    [switch]$Legacy
)`
	got := stripArchParameter(in)
	if strings.Contains(got, "ValidateSet") {
		t.Errorf("ValidateSet not removed: %q", got)
	}
	if strings.Contains(got, "$Architecture,") {
		t.Errorf("$Architecture parameter not removed: %q", got)
	}
	if !strings.Contains(got, "[switch]$Legacy") {
		t.Errorf("unrelated parameter was affected: %q", got)
	}
}

func TestStripArchFromComposeFilename(t *testing.T) {
	in := `    $script:ComposeFile = "$Architecture/docker-compose.$Mode.$ExternalMode.yml"`
	got := stripArchFromComposeFilename(in)
	if !strings.Contains(got, `"docker-compose.$Mode.$ExternalMode.yml"`) {
		t.Fatalf("expected arch-less filename template, got: %s", got)
	}
	if strings.Contains(got, "$Architecture") {
		t.Fatalf("$Architecture should be stripped, got: %s", got)
	}
}

func TestStripArchFromDotSource(t *testing.T) {
	in := `$SystemConfig = . "$PSScriptRoot/$Architecture/Run-SystemTests.Config.Architecture.ps1"`
	got := stripArchFromDotSource(in)
	want := `$SystemConfig = . "$PSScriptRoot/Run-SystemTests.Config.Architecture.ps1"`
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func TestStripArchFromDisplayHeading(t *testing.T) {
	in := `        Write-Heading -Text "System: $Architecture / $($externalMode.ToUpper())"`
	got := stripArchFromDisplayHeading(in)
	if !strings.Contains(got, `"System: $($externalMode.ToUpper())"`) {
		t.Fatalf("expected stripped heading, got: %s", got)
	}
}

// TestStripRunSystemTestsPS1_AgainstShop exercises the full scaffolded-PS1
// rewrite against shop's real Run-SystemTests.ps1 for each test-lang. After
// stripping, the scaffolded PS1 must (a) no longer mention $Architecture,
// (b) dot-source the arch config at the sibling level, (c) reference compose
// files at the sibling level. Skipped if shop isn't available.
func TestStripRunSystemTestsPS1_AgainstShop(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(wd))) // …/academy
	for _, lang := range []string{"typescript", "java", "dotnet"} {
		t.Run(lang, func(t *testing.T) {
			shopPS1 := filepath.Join(root, "shop", "system-test", lang, "Run-SystemTests.ps1")
			data, err := os.ReadFile(shopPS1)
			if err != nil {
				t.Skipf("shop not available at %s", shopPS1)
			}

			// Stage shop's PS1 into a tmp testDst and run the file-level function
			// (which handles CRLF normalization — shop ships PS1 files as CRLF).
			testDst := t.TempDir()
			psPath := filepath.Join(testDst, "Run-SystemTests.ps1")
			if err := os.WriteFile(psPath, data, 0644); err != nil {
				t.Fatal(err)
			}
			stripRunSystemTestsPS1(testDst)
			out, err := os.ReadFile(psPath)
			if err != nil {
				t.Fatal(err)
			}
			content := string(out)

			forbidden := []string{
				`ValidateSet("multitier", "monolith")`,
				`$Architecture,`,
				`# Auto-detect architecture`,
				`"$Architecture/docker-compose.$Mode.$ExternalMode.yml"`,
				`"$PSScriptRoot/$Architecture/Run-SystemTests.Config.Architecture.ps1"`,
				`$Architecture /`,
			}
			for _, f := range forbidden {
				if strings.Contains(content, f) {
					t.Errorf("output still contains forbidden %q", f)
				}
			}

			required := []string{
				`"docker-compose.$Mode.$ExternalMode.yml"`,
				`"$PSScriptRoot/Run-SystemTests.Config.Architecture.ps1"`,
				`"System: $($externalMode.ToUpper())"`,
			}
			for _, r := range required {
				if !strings.Contains(content, r) {
					t.Errorf("output missing required %q", r)
				}
			}

			// Brace balance must stay intact.
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

// TestStripSystemTestReadme_AgainstShop verifies the "## Architectures" section
// is cleanly excised from shop's top-level system-test/<lang>/README.md.
func TestStripSystemTestReadme_AgainstShop(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(wd)))
	for _, lang := range []string{"typescript", "java", "dotnet"} {
		t.Run(lang, func(t *testing.T) {
			shopReadme := filepath.Join(root, "shop", "system-test", lang, "README.md")
			data, err := os.ReadFile(shopReadme)
			if err != nil {
				t.Skipf("shop not available at %s", shopReadme)
			}

			// Simulate by staging shop's README into a tmp testDst and running
			// stripSystemTestReadme against it.
			testDst := t.TempDir()
			path := filepath.Join(testDst, "README.md")
			if err := os.WriteFile(path, data, 0644); err != nil {
				t.Fatal(err)
			}
			stripSystemTestReadme(testDst)

			out, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			s := string(out)
			forbidden := []string{
				"## Architectures",
				"monolith/README.md",
				"multitier/README.md",
				"-Architecture monolith|multitier",
			}
			for _, f := range forbidden {
				if strings.Contains(s, f) {
					t.Errorf("output still contains forbidden %q", f)
				}
			}
			required := []string{
				"## Prerequisites",
				"## Available Suite IDs",
			}
			for _, r := range required {
				if !strings.Contains(s, r) {
					t.Errorf("output missing required %q", r)
				}
			}
		})
	}
}
