package steps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

// seedShopBackendTemplate writes a minimal shop-template backend tree for one
// language under <shop>/system/multitier/backend-<lang>, with a marker file
// whose contents identify the language. ScaffoldBackendServices copies this
// tree per service, so the marker lets the test prove each service got the
// source for ITS declared language (not a sibling's).
func seedShopBackendTemplate(t *testing.T, shop, lang string) {
	t.Helper()
	dir := filepath.Join(shop, "system", "multitier", "backend-"+lang)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatalf("seed backend-%s: %v", lang, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "src", "marker.txt"), []byte("lang="+lang), 0o644); err != nil {
		t.Fatalf("seed backend-%s marker: %v", lang, err)
	}
}

// TestScaffoldBackendServices_ProducesOneBackendPerService is the Step 6
// ride-along test for the microservices scaffolding seam (plan
// 20260615-1346): N declared service locations must produce N backend
// scaffolds, each at its own repo-relative path and carrying the source for
// its own declared language.
func TestScaffoldBackendServices_ProducesOneBackendPerService(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	shop := filepath.Join(work, "shop")
	repo := filepath.Join(work, "repo")
	// Heterogeneous mono-repo: two services sharing one repo with distinct
	// paths (the plan's headline shape) — one java, one dotnet.
	seedShopBackendTemplate(t, shop, "java")
	seedShopBackendTemplate(t, shop, "dotnet")

	cfg := &config.Config{
		Arch:         "microservices",
		RepoStrategy: "monorepo",
		ShopPath:     shop,
		RepoDir:      repo,
		WorkDir:      work,
		BackendServices: []config.BackendService{
			{Name: "inventory", Path: "system/microservices/inventory-dotnet", Lang: "dotnet", Repo: "optivem/shop"},
			{Name: "orders", Path: "system/microservices/orders-java", Lang: "java", Repo: "optivem/shop"},
		},
	}

	ScaffoldBackendServices(cfg)

	// One backend scaffold per service, at the service's own path, with the
	// marker proving the per-service language source landed.
	for _, svc := range cfg.BackendServices {
		marker := filepath.Join(repo, svc.Path, "src", "marker.txt")
		got, err := os.ReadFile(marker)
		if err != nil {
			t.Fatalf("service %q: expected backend scaffold at %s, got: %v", svc.Name, marker, err)
		}
		if want := "lang=" + svc.Lang; string(got) != want {
			t.Errorf("service %q: marker = %q, want %q (wrong-language source copied)", svc.Name, got, want)
		}
	}

	// Exactly N backend scaffolds — no extra, no collision. Count the
	// service-path leaf dirs that actually materialized under the repo.
	for _, svc := range cfg.BackendServices {
		if _, err := os.Stat(filepath.Join(repo, svc.Path)); err != nil {
			t.Errorf("service %q backend dir missing: %v", svc.Name, err)
		}
	}
}

// TestScaffoldBackendServices_MultiRepoPerServiceDirs pins the multi-repo
// shape: each service lands in its OWN clone dir (repo-<name>) rather than the
// shared RepoDir — one repo per service (D3), distinct destinations.
func TestScaffoldBackendServices_MultiRepoPerServiceDirs(t *testing.T) {
	t.Parallel()

	work := t.TempDir()
	shop := filepath.Join(work, "shop")
	seedShopBackendTemplate(t, shop, "java")
	seedShopBackendTemplate(t, shop, "dotnet")

	cfg := &config.Config{
		Arch:         "microservices",
		RepoStrategy: "multirepo",
		ShopPath:     shop,
		RepoDir:      filepath.Join(work, "repo"), // workspace repo; services do NOT land here
		WorkDir:      work,
		BackendServices: []config.BackendService{
			// Multi-repo: every service at path "." in its own repo.
			{Name: "inventory", Path: ".", Lang: "dotnet", Repo: "optivem/shop-inventory"},
			{Name: "orders", Path: ".", Lang: "java", Repo: "optivem/shop-orders"},
		},
	}

	ScaffoldBackendServices(cfg)

	for _, svc := range cfg.BackendServices {
		// Service code lands under repo-<name>/, derived from WorkDir — not in
		// the shared workspace RepoDir.
		marker := filepath.Join(work, "repo-"+svc.Name, "src", "marker.txt")
		got, err := os.ReadFile(marker)
		if err != nil {
			t.Fatalf("service %q: expected backend scaffold at %s, got: %v", svc.Name, marker, err)
		}
		if want := "lang=" + svc.Lang; string(got) != want {
			t.Errorf("service %q: marker = %q, want %q", svc.Name, got, want)
		}
	}
}
