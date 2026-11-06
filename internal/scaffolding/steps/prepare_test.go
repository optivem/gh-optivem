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

// Shop refs up to meta-v1.0.223 have full per-flavor stage workflows and no
// Docker Compose callers or shared workflows: refused with a clear message.
func TestCheckShopWorkflowLayout_RejectsOldLayout(t *testing.T) {
	t.Parallel()
	shop := t.TempDir()
	writeShopWorkflow(t, shop, "monolith-java-acceptance-stage.yml", "jobs:\n  run:\n    runs-on: ubuntu-latest\n")
	writeShopWorkflow(t, shop, "monolith-java-acceptance-stage-legacy.yml", "jobs:\n  run:\n    runs-on: ubuntu-latest\n")

	for _, noLegacy := range []bool{false, true} {
		cfg := &config.Config{Arch: "monolith", Lang: "java", TestLang: "java", ShopPath: shop, NoLegacy: noLegacy}
		err := checkShopWorkflowLayout(cfg)
		if err == nil {
			t.Fatalf("noLegacy=%v: want error for the old layout, got nil", noLegacy)
		}
		for _, want := range []string{"monolith-java-acceptance-stage-docker-compose.yml", "_acceptance-stage-docker-compose.yml", "--shop-ref main"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("noLegacy=%v: error %q does not mention %s", noLegacy, err, want)
			}
		}
	}
}

// Shop main (single-pipeline layout): accepted, keyed by the test language.
func TestCheckShopWorkflowLayout_AcceptsSinglePipelineLayout(t *testing.T) {
	t.Parallel()
	shop := t.TempDir()
	writeShopWorkflow(t, shop, "_acceptance-stage-docker-compose.yml", "on:\n  workflow_call:\n")
	writeShopWorkflow(t, shop, "multitier-typescript-acceptance-stage-docker-compose.yml",
		"jobs:\n  acceptance-stage:\n    uses: ./.github/workflows/_acceptance-stage-docker-compose.yml\n")
	cfg := &config.Config{Arch: "multitier", BackendLang: "java", TestLang: "typescript", ShopPath: shop}
	if err := checkShopWorkflowLayout(cfg); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	// Microservices scaffolds copy no pipeline workflows: never checked.
	if err := checkShopWorkflowLayout(&config.Config{Arch: "microservices", ShopPath: t.TempDir()}); err != nil {
		t.Fatalf("microservices: want nil, got %v", err)
	}
}
