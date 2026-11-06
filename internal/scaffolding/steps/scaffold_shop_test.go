package steps

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/scaffolding/files"
)

// TestScaffoldAgainstShopCheckout runs ApplyTemplate against a real shop
// checkout (GH_OPTIVEM_SHOP_PATH, e.g. a clone of shop main) for every
// monolith / multitier flavor and both repo strategies, with and without
// --no-legacy, then checks the scaffolded workflow set, the leftover-template
// guard over .github/ and (when actionlint is on PATH) actionlint. Skipped without the env
// var: it needs the network-free checkout but not GitHub.
func TestScaffoldAgainstShopCheckout(t *testing.T) {
	shop := os.Getenv("GH_OPTIVEM_SHOP_PATH")
	if shop == "" {
		t.Skip("GH_OPTIVEM_SHOP_PATH not set")
	}
	actionlint, _ := exec.LookPath("actionlint")

	langs := []string{"java", "dotnet", "typescript"}
	for _, arch := range []string{"monolith", "multitier"} {
		for _, lang := range langs {
			for _, strategy := range []string{"monorepo", "multirepo"} {
				for _, noLegacy := range []bool{false, true} {
					name := arch + "-" + lang + "-" + strategy
					if noLegacy {
						name += "-nolegacy"
					}
					t.Run(name, func(t *testing.T) {
						cfg := shopScaffoldConfig(t, shop, arch, lang, lang, strategy, noLegacy)
						ApplyTemplate(cfg)
						checkScaffoldedPipeline(t, cfg, actionlint)
					})
				}
			}
		}
	}
	// Polyglot: system tests in a different language than the system.
	t.Run("monolith-dotnet-tests-java-monorepo", func(t *testing.T) {
		cfg := shopScaffoldConfig(t, shop, "monolith", "dotnet", "java", "monorepo", false)
		ApplyTemplate(cfg)
		checkScaffoldedPipeline(t, cfg, actionlint)
	})
}

func shopScaffoldConfig(t *testing.T, shop, arch, lang, testLang, strategy string, noLegacy bool) *config.Config {
	root := t.TempDir()
	cfg := &config.Config{
		Arch: arch, RepoStrategy: strategy, TestLang: testLang,
		Owner: "acme", Repo: "shopx", FullRepo: "acme/shopx", SystemName: "shopx",
		Deploy: "docker", NoLegacy: noLegacy, ShopPath: shop,
		RepoDir: filepath.Join(root, "shopx"),
	}
	if arch == "monolith" {
		cfg.Lang = lang
		if strategy == "multirepo" {
			cfg.SystemRepo, cfg.SystemFullRepo = "shopx-system", "acme/shopx-system"
			cfg.SystemRepoDir = filepath.Join(root, "shopx-system")
		}
	} else {
		cfg.Lang, cfg.BackendLang, cfg.FrontendLang = lang, lang, "typescript"
		if strategy == "multirepo" {
			cfg.BackendRepo, cfg.BackendFullRepo = "shopx-backend", "acme/shopx-backend"
			cfg.FrontendRepo, cfg.FrontendFullRepo = "shopx-frontend", "acme/shopx-frontend"
			cfg.BackendRepoDir = filepath.Join(root, "shopx-backend")
			cfg.FrontendRepoDir = filepath.Join(root, "shopx-frontend")
		}
	}
	for _, d := range []string{cfg.RepoDir, cfg.SystemRepoDir, cfg.BackendRepoDir, cfg.FrontendRepoDir} {
		if d != "" {
			if err := os.MkdirAll(d, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return cfg
}

func checkScaffoldedPipeline(t *testing.T, cfg *config.Config, actionlint string) {
	t.Helper()
	want := []string{
		"_acceptance-stage-docker-compose.yml", "_acceptance-stage.yml", "_deploy-stage.yml",
		"_prod-stage.yml", "_qa-stage.yml",
		"acceptance-stage-docker-compose.yml", "acceptance-stage.yml",
		"bump-patch-version.yml", "cleanup.yml", "prod-stage.yml", "qa-signoff.yml", "qa-stage.yml",
	}
	if !cfg.NoLegacy {
		want = append(want, "acceptance-stage-docker-compose-legacy.yml")
	}
	if cfg.RepoStrategy == "monorepo" {
		if cfg.Arch == "monolith" {
			want = append(want, "commit-stage.yml")
		} else {
			want = append(want, "backend-commit-stage.yml", "frontend-commit-stage.yml")
		}
	}
	sort.Strings(want)
	got, err := listFilesWithExt(filepath.Join(cfg.RepoDir, ".github", "workflows"), ".yml")
	if err != nil {
		t.Fatal(err)
	}
	for i := range got {
		got[i] = filepath.Base(got[i])
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("workflows:\n got  %v\n want %v", got, want)
	}
	checkScaffoldedSpec(t, cfg.RepoDir, cfg.TestLang)

	legacyWf := filepath.Join(cfg.RepoDir, ".github", "workflows", Names.DestLegacyAcceptWf)
	if data, err := os.ReadFile(legacyWf); err == nil && !strings.Contains(string(data), "schedule:") {
		t.Errorf("legacy gate has no schedule")
	}

	refs := forbiddenTemplateRefs(cfg)
	for _, dir := range scaffoldRepoDirs(cfg) {
		// Only .github/: the leftover guard also flags shop-path comments in
		// source and test READMEs (pre-existing, not part of the pipeline).
		for _, needle := range refs {
			if hits := files.FindInTree(filepath.Join(dir, ".github"), needle); len(hits) > 0 {
				t.Errorf("leftover %q in %s: %v", needle, filepath.Base(dir), hits)
			}
		}
		if actionlint == "" {
			continue
		}
		// actionlint resolves ./.github/workflows/* and ./.github/actions/*
		// references only inside a project, which it finds by .git.
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		wfs, _ := listFilesWithExt(filepath.Join(dir, ".github", "workflows"), ".yml")
		cmd := exec.Command(actionlint, append([]string{"-no-color"}, wfs...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("actionlint in %s: %v\n%s", dir, err, out)
		}
	}
}
