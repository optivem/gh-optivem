package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

func writeShopWorkflow(t *testing.T, shop, name, body string) {
	t.Helper()
	dir := filepath.Join(shop, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Shop main (post single-pipeline restructure) makes the stage workflows thin
// callers of _*-stage.yml, which the scaffolder never copies: refuse the ref.
func TestCheckShopWorkflowLayout_RejectsSharedReusableCallers(t *testing.T) {
	t.Parallel()
	shop := t.TempDir()
	writeShopWorkflow(t, shop, "monolith-java-acceptance-stage.yml",
		"jobs:\n  acceptance-stage:\n    uses: ./.github/workflows/_acceptance-stage.yml\n")
	writeShopWorkflow(t, shop, "monolith-java-qa-stage.yml",
		"jobs:\n  qa-stage:\n    uses: ./.github/workflows/_qa-stage.yml\n")
	writeShopWorkflow(t, shop, "monolith-java-prod-stage.yml",
		"jobs:\n  prod-stage:\n    uses: ./.github/workflows/_prod-stage.yml\n  bump:\n    uses: ./.github/workflows/monolith-java-bump-patch-version.yml\n")

	for _, noLegacy := range []bool{false, true} {
		cfg := &config.Config{Arch: "monolith", Lang: "java", TestLang: "java", Deploy: "docker", ShopPath: shop, NoLegacy: noLegacy}
		err := checkShopWorkflowLayout(cfg)
		if err == nil {
			t.Fatalf("noLegacy=%v: want error for shared reusable callers, got nil", noLegacy)
		}
		for _, want := range []string{"_acceptance-stage.yml", "_qa-stage.yml", "_prod-stage.yml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("noLegacy=%v: error %q does not name %s", noLegacy, err, want)
			}
		}
		if strings.Contains(err.Error(), "bump-patch-version") {
			t.Errorf("bump-patch-version is copied by the scaffolder and must not be flagged: %v", err)
		}
	}
}

// Shop refs up to meta-v1.0.223 inline the stage jobs: accepted.
func TestCheckShopWorkflowLayout_AcceptsSelfContainedStages(t *testing.T) {
	t.Parallel()
	shop := t.TempDir()
	writeShopWorkflow(t, shop, "monolith-java-acceptance-stage.yml",
		"jobs:\n  run:\n    steps:\n      - uses: ./.github/actions/compile\n")
	writeShopWorkflow(t, shop, "monolith-java-prod-stage.yml",
		"jobs:\n  bump:\n    uses: ./.github/workflows/monolith-java-bump-patch-version.yml\n")
	cfg := &config.Config{Arch: "monolith", Lang: "java", TestLang: "java", Deploy: "docker", ShopPath: shop}
	if err := checkShopWorkflowLayout(cfg); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
