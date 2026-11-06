package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/spec"
)

// copySpec ships shop's spec.yaml (results flattened to system-test/) and the
// use-case markdown, so `gh optivem spec check` and `spec record` find their
// config in a scaffolded repo.
func TestCopySpec(t *testing.T) {
	t.Parallel()
	shop, repo := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(shop, "spec.yaml"), `specDir: docs/use-cases
results:
  - "system-test/java/build/test-results/**/*.xml"
  - "system-test/dotnet/TestResults/**/*.xml"
  - "system-test/typescript/test-results/**/*.xml"
exclude:
  - "**/legacy/**"
threshold: 90
`)
	if err := os.MkdirAll(filepath.Join(shop, "docs", "use-cases"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(shop, "docs", "use-cases", "UC-001-place-order.md"), "---\nid: UC-001\ntitle: Place Order\n---\n\n## Acceptance criteria\n- UC-001.1: myshop places an order.\n")

	copySpec(shop, repo)

	cfg, err := spec.LoadConfig(filepath.Join(repo, "spec.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"system-test/build/test-results/**/*.xml",
		"system-test/TestResults/**/*.xml",
		"system-test/test-results/**/*.xml",
	}
	if strings.Join(cfg.Results, " ") != strings.Join(want, " ") {
		t.Errorf("results = %v, want %v", cfg.Results, want)
	}
	if cfg.SpecDir != "docs/use-cases" || strings.Join(cfg.Exclude, " ") != "**/legacy/**" || cfg.ThresholdValue() != 90 {
		t.Errorf("spec.yaml not carried over: %+v", cfg)
	}
	ucs, err := spec.LoadDir(filepath.Join(repo, cfg.SpecDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(ucs) != 1 || ucs[0].ID != "UC-001" {
		t.Errorf("use cases = %+v", ucs)
	}
}

// A shop ref without spec.yaml / docs/use-cases is skipped, not fatal.
func TestCopySpec_AbsentInShop(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	copySpec(t.TempDir(), repo)
	if _, err := os.Stat(filepath.Join(repo, "spec.yaml")); !os.IsNotExist(err) {
		t.Errorf("spec.yaml created without a shop source: %v", err)
	}
	if dirExists(filepath.Join(repo, "docs", "use-cases")) {
		t.Error("docs/use-cases created without a shop source")
	}
}

// checkScaffoldedSpec asserts the scaffolded repo carries a spec.yaml whose
// results point into the flat system-test/ dir and whose specDir resolves to
// valid use cases, and that the lenient gate passes once every criterion has
// a passing tagged test (what the java acceptance suites produce).
func checkScaffoldedSpec(t *testing.T, repoDir string) {
	t.Helper()
	cfg, err := spec.LoadConfig(filepath.Join(repoDir, "spec.yaml"))
	if err != nil {
		t.Fatalf("spec.yaml: %v", err)
	}
	if err := cfg.RequireResults(); err != nil {
		t.Fatal(err)
	}
	for _, r := range cfg.Results {
		if !strings.HasPrefix(r, Names.TargetSystemTestDir+"/") {
			t.Errorf("results %q not under %s/", r, Names.TargetSystemTestDir)
		}
		for _, l := range specTestLangs {
			if strings.Contains(r, dirSystemTest+"/"+l+"/") {
				t.Errorf("results %q still names shop's per-language dir", r)
			}
		}
	}
	ucs, err := spec.LoadDir(filepath.Join(repoDir, cfg.SpecDir))
	if err != nil {
		t.Fatalf("specDir %s: %v", cfg.SpecDir, err)
	}
	if len(ucs) == 0 {
		t.Fatalf("no use cases under %s", cfg.SpecDir)
	}
	if errs := spec.Validate(ucs); len(errs) > 0 {
		t.Errorf("use-case validation: %v", errs)
	}
	var tests []spec.Test
	for _, uc := range ucs {
		for _, c := range uc.Criteria {
			tests = append(tests, spec.Test{Class: "AcceptanceTest", Method: c.ID, IDs: []string{c.ID},
				Results: []spec.Result{{Name: c.ID, Status: spec.StatusPassed}}})
		}
	}
	rep := spec.BuildReport(ucs, tests, cfg, spec.Options{Mode: spec.ModeLenient, Now: time.Now()})
	if err := spec.ApplyGate(&rep, nil); err != nil {
		t.Fatal(err)
	}
	if !rep.Gate.Passed {
		t.Errorf("lenient spec gate failed: %v", rep.Gate.Reasons)
	}
}
