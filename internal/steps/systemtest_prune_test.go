package steps

import (
	"os"
	"strings"
	"testing"
)

const fencePowerShell = "```powershell"

// Test against the actual scaffolded Run-SystemTests.ps1 we just inspected.
func TestStripPowerShellArchBlockRealScaffold(t *testing.T) {
	path := `C:\Users\valen_4rjvn9e\AppData\Local\Temp\scaffold-3771067308\repo\system-test\Run-SystemTests.ps1`
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("scaffold file not present: %v", err)
	}
	stripped := stripPowerShellArchBlock(string(data), "multitier")

	// Should still contain the monolith block, ValidateSet, and have dropped
	// the multitier hashtable entry.
	if !strings.Contains(stripped, `"monolith" = @{`) {
		t.Error("monolith block was dropped too")
	}
	if strings.Contains(stripped, `"multitier" = @{`) {
		t.Error("multitier block was not stripped")
	}
	if strings.Contains(stripped, "shop-typescript-multitier-real") {
		t.Error("multitier internals still present")
	}
	// Balance: equal number of { and } after stripping.
	opens := strings.Count(stripped, "{")
	closes := strings.Count(stripped, "}")
	if opens != closes {
		t.Errorf("brace imbalance after strip: %d { vs %d }", opens, closes)
	}
}

// applyReplacements mimics templates.FixupWorkflowContent: applies replacements
// in order to a single string. Kept here so the prefix-drop logic can be unit
// tested without touching files.
func applyReplacements(s string, replacements [][2]string) string {
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r[0], r[1])
	}
	return s
}

func TestSystemPrefixDropMonolith(t *testing.T) {
	// System-level references that must lose the `monolith-typescript-` prefix.
	cases := []struct {
		in, want string
	}{
		{"          tag: monolith-typescript-v${{ steps.read-base-version.outputs.base-version }}",
			"          tag: v${{ steps.read-base-version.outputs.base-version }}"},
		{"          tag-prefix: monolith-typescript-v", "          tag-prefix: v"},
		{"          prefix: monolith-typescript", "          prefix: ''"},
		{"description: 'Prerelease version to deploy (e.g., monolith-typescript-v1.0.0-rc.1).'",
			"description: 'Prerelease version to deploy (e.g., v1.0.0-rc.1).'"},
	}
	reps := systemPrefixDropReplacements("monolith-typescript")
	for _, c := range cases {
		got := applyReplacements(c.in, reps)
		if got != c.want {
			t.Errorf("\n  in:   %q\n  got:  %q\n  want: %q", c.in, got, c.want)
		}
	}
}

// Component prefixes must survive the system-prefix drop pass.
func TestSystemPrefixDropLeavesComponentPrefixes(t *testing.T) {
	reps := systemPrefixDropReplacements("multitier-typescript")
	// Pretend the earlier multitier replacements have already rewritten
	// `backend-<lang>` → `backend` and `frontend-<lang>`
	// → `frontend`. The remaining text should be untouched.
	in := []string{
		"          tag: backend-v1.0.0",
		"          tag-prefix: frontend-v",
		"ghcr.io/foo/bar/backend:latest",
		"ghcr.io/foo/bar/frontend:latest",
	}
	for _, line := range in {
		got := applyReplacements(line, reps)
		if got != line {
			t.Errorf("component prefix got mangled:\n  in:  %q\n  got: %q", line, got)
		}
	}
}

func TestPruneReadmeLinesMonolithStripsMultitier(t *testing.T) {
	in := []string{
		"# System Test (TypeScript)",
		"",
		"## Running Tests",
		"",
		"Run all latest test suites (multitier):",
		"",
		fencePowerShell,
		"./Run-SystemTests.ps1 -Architecture multitier",
		"```",
		"",
		"Run all latest test suites (monolith):",
		"",
		fencePowerShell,
		"./Run-SystemTests.ps1 -Architecture monolith",
		"```",
		"",
		"Run legacy test suites:",
		"",
		fencePowerShell,
		"./Run-SystemTests.ps1 -Architecture multitier -Legacy",
		"./Run-SystemTests.ps1 -Architecture monolith -Legacy",
		"```",
		"",
		"Run a specific suite by ID:",
		"",
		fencePowerShell,
		"./Run-SystemTests.ps1 -Architecture multitier -Suite acceptance-api",
		"```",
	}
	out := strings.Join(pruneReadmeLines(in, "monolith", "multitier"), "\n")

	// Must strip the "(multitier):" example block entirely.
	if strings.Contains(out, "(multitier)") {
		t.Error("(multitier) heading still present")
	}
	if strings.Contains(out, "-Architecture multitier -Legacy") {
		t.Error("multitier -Legacy line still present")
	}
	// Remaining "-Architecture multitier" references must be retargeted.
	if strings.Contains(out, "-Architecture multitier") {
		t.Error("-Architecture multitier should be rewritten to monolith")
	}
	// Monolith example must survive.
	if !strings.Contains(out, "(monolith):") {
		t.Error("(monolith) heading should remain")
	}
	if !strings.Contains(out, "./Run-SystemTests.ps1 -Architecture monolith") {
		t.Error("monolith example command should remain")
	}
	// Specific-suite example: originally -Architecture multitier, must become monolith.
	if !strings.Contains(out, "-Architecture monolith -Suite acceptance-api") {
		t.Error("specific-suite example should be retargeted to monolith")
	}
}
