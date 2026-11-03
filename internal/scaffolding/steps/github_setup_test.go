package steps

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// TestCreateRepos_SkipsPreExistingRepos pins that repos in PreExistingRepos
// are reused without any `gh` call: with every target pre-existing, a call to
// CreateRepo would shell out to gh (absent/unauthenticated in tests) and fatal.
func TestCreateRepos_SkipsPreExistingRepos(t *testing.T) {
	cfg := &config.Config{
		FullRepo:         "acme/shop",
		RepoStrategy:     "multirepo",
		Arch:             "multitier",
		FrontendFullRepo: "acme/shop-frontend",
		BackendFullRepo:  "acme/shop-backend",
		PreExistingRepos: map[string]bool{
			"acme/shop":          true,
			"acme/shop-frontend": true,
			"acme/shop-backend":  true,
		},
	}
	CreateRepos(cfg, shell.NewGitHub(cfg.FullRepo))
}
