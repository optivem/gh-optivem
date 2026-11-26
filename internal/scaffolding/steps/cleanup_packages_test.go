package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

// shopCleanupYML mirrors the two container-packages inputs in shop's cleanup.yml.
const shopCleanupYML = `jobs:
  cleanup:
    steps:
      - name: Cleanup Prereleases
        uses: optivem/actions/cleanup-prereleases@v1
        with:
          container-packages: shop/monolith-system-java,shop/multitier-backend-java,shop/multitier-frontend-react
      - name: Cleanup Orphan Manifests
        uses: optivem/actions/cleanup-ghcr-orphan-manifests@v1
        with:
          container-packages: shop/monolith-system-java,shop/multitier-backend-java,shop/multitier-frontend-react
`

func seedCleanupYML(t *testing.T, repoDir string) {
	t.Helper()
	dir := filepath.Join(repoDir, ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cleanup.yml"), []byte(shopCleanupYML), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCleanupPackages(t *testing.T, repoDir, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoDir, ".github", "workflows", "cleanup.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "shop/") {
		t.Errorf("%s: shop package names survived:\n%s", repoDir, content)
	}
	if got := strings.Count(content, "container-packages: "+want+"\n"); got != 2 {
		t.Errorf("%s: want both container-packages inputs = %q, got %d matches:\n%s", repoDir, want, got, content)
	}
}

func TestRewriteCleanupPackagesForAllRepos(t *testing.T) {
	tests := []struct {
		name  string
		cfg   func(root, sys, be, fe string) *config.Config
		wants map[string]string // dir key -> expected container-packages value
	}{
		{
			name: "monolith monorepo",
			cfg: func(root, _, _, _ string) *config.Config {
				return &config.Config{Arch: "monolith", RepoStrategy: "monorepo", Repo: "Sky-App", RepoDir: root}
			},
			wants: map[string]string{"root": "sky-app/system"},
		},
		{
			name: "monolith multirepo",
			cfg: func(root, sys, _, _ string) *config.Config {
				return &config.Config{Arch: "monolith", RepoStrategy: "multirepo", Repo: "app", RepoDir: root,
					SystemRepo: "app-system", SystemRepoDir: sys}
			},
			wants: map[string]string{"root": "''", "sys": "app-system/system"},
		},
		{
			name: "multitier monorepo",
			cfg: func(root, _, _, _ string) *config.Config {
				return &config.Config{Arch: "multitier", RepoStrategy: "monorepo", Repo: "app", RepoDir: root}
			},
			wants: map[string]string{"root": "app/backend,app/frontend"},
		},
		{
			name: "multitier multirepo",
			cfg: func(root, _, be, fe string) *config.Config {
				return &config.Config{Arch: "multitier", RepoStrategy: "multirepo", Repo: "app", RepoDir: root,
					BackendRepo: "app-backend", BackendRepoDir: be, FrontendRepo: "app-frontend", FrontendRepoDir: fe}
			},
			wants: map[string]string{"root": "''", "be": "app-backend/backend", "fe": "app-frontend/frontend"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dirs := map[string]string{"root": t.TempDir(), "sys": t.TempDir(), "be": t.TempDir(), "fe": t.TempDir()}
			for key := range tt.wants {
				seedCleanupYML(t, dirs[key])
			}
			rewriteCleanupPackagesForAllRepos(tt.cfg(dirs["root"], dirs["sys"], dirs["be"], dirs["fe"]))
			for key, want := range tt.wants {
				assertCleanupPackages(t, dirs[key], want)
			}
		})
	}
}
