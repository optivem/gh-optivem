package templates

import (
	"strings"
	"testing"
)

// shopStylePS1 mimics shop's Run-SystemTests.ps1 $AllSystemConfig block shape —
// same indentation (4 spaces for arch key, 8 for mode key, 12+ for contents).
// Kept minimal so the test stays readable.
const shopStylePS1 = `# prelude
$AllSystemConfig = @{
    "multitier" = @{
        "real" = @{
            ContainerName = "shop-typescript-multitier-real"
            Nested = @(
                @{ Name = "Frontend"; Port = 3311 }
            )
        }

        "stub" = @{
            ContainerName = "shop-typescript-multitier-stub"
        }
    }

    "monolith" = @{
        "real" = @{
            ContainerName = "shop-typescript-monolith-real"
            Nested = @(
                @{ Name = "Monolith"; Port = 3311 }
            )
        }

        "stub" = @{
            ContainerName = "shop-typescript-monolith-stub"
        }
    }
}

$SystemConfig = $AllSystemConfig[$Architecture]
`

func TestPruneAllSystemConfig_MonolithKept(t *testing.T) {
	got := pruneAllSystemConfig(shopStylePS1, "monolith", "multitier")

	mustNotContain(t, got, `"multitier"`, "multitier block should be deleted")
	mustNotContain(t, got, `shop-typescript-multitier-real`, "multitier contents should be gone")
	mustNotContain(t, got, `"monolith" = @{`, "monolith wrapper should be flattened out")

	mustContain(t, got, `    "real" = @{`, "real should be at 4-space indent (dedented from 8)")
	mustContain(t, got, `    "stub" = @{`, "stub should be at 4-space indent")
	mustContain(t, got, `shop-typescript-monolith-real`, "monolith contents should survive (rewrite is a later step)")

	// Structure sanity: the $AllSystemConfig opening brace must still be balanced
	// after pruning. Count braces; should remain equal.
	open := strings.Count(got, "{")
	close := strings.Count(got, "}")
	if open != close {
		t.Fatalf("brace imbalance after prune: %d open vs %d close\n%s", open, close, got)
	}

	// The trailing `$SystemConfig = $AllSystemConfig[$Architecture]` line must
	// still be present — stripArchFromSystemConfigLookup (separate step) handles it.
	mustContain(t, got, `$SystemConfig = $AllSystemConfig[$Architecture]`, "trailing lookup line preserved")
}

func TestPruneAllSystemConfig_MultitierKept(t *testing.T) {
	got := pruneAllSystemConfig(shopStylePS1, "multitier", "monolith")

	mustNotContain(t, got, `"monolith"`, "monolith block should be deleted")
	mustNotContain(t, got, `shop-typescript-monolith-real`, "monolith contents should be gone")
	mustContain(t, got, `shop-typescript-multitier-real`, "multitier contents should survive")
	mustContain(t, got, `    "real" = @{`, "real should be dedented")
}

func TestStripLangArchFromContainerNames(t *testing.T) {
	in := `            ContainerName = "shop-typescript-monolith-real"
            ContainerName = "shop-typescript-monolith-stub"
            ContainerName = "frontend"`
	got := stripLangArchFromContainerNames(in, "monolith", "typescript")

	mustContain(t, got, `ContainerName = "shop-real"`, "real rewritten")
	mustContain(t, got, `ContainerName = "shop-stub"`, "stub rewritten")
	mustContain(t, got, `ContainerName = "frontend"`, "unrelated ContainerName untouched")
	mustNotContain(t, got, `typescript-monolith`, "dim strings fully gone")
}

func TestStripArchParameter(t *testing.T) {
	in := `param(
    [ValidateSet("multitier", "monolith")]
    [string]$Architecture,

    [switch]$Legacy
)`
	got := stripArchParameter(in)
	mustNotContain(t, got, `ValidateSet`, "ValidateSet should be removed")
	mustNotContain(t, got, `$Architecture,`, "Architecture parameter should be removed")
	mustContain(t, got, `[switch]$Legacy`, "other parameters preserved")
}

func TestStripArchFromComposeFilename(t *testing.T) {
	in := `    $script:ComposeFile = "docker-compose.$Mode.$Architecture.$ExternalMode.yml"`
	got := stripArchFromComposeFilename(in)
	if !strings.Contains(got, `"docker-compose.$Mode.$ExternalMode.yml"`) {
		t.Fatalf("expected stripped filename template, got: %s", got)
	}
	if strings.Contains(got, "$Architecture") {
		t.Fatalf("$Architecture should be stripped, got: %s", got)
	}
}

func TestStripArchFromSystemConfigLookup(t *testing.T) {
	in := `$SystemConfig = $AllSystemConfig[$Architecture]`
	got := stripArchFromSystemConfigLookup(in)
	want := `$SystemConfig = $AllSystemConfig`
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

func mustContain(t *testing.T, s, substr, msg string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Fatalf("%s: missing %q\n--- content ---\n%s", msg, substr, s)
	}
}

func mustNotContain(t *testing.T, s, substr, msg string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Fatalf("%s: should not contain %q\n--- content ---\n%s", msg, substr, s)
	}
}
