package steps

import (
	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// WriteOptivemYAML writes <repoRoot>/gh-optivem.yaml in the scaffolded repo(s),
// translating already-resolved init flags into the projectconfig.Config schema.
// The file is consumed by the ATDD pipeline at runtime (project URL, repo
// strategy, scope axes). It is the single config the gh-optivem binary reads.
//
// Multi-repo: writes the same file to every per-tier repo so `gh optivem atdd
// implement-ticket` can be invoked from any of them.
//
// `cfg.RepoStrategy` arrives in the init flag's spelling (`monorepo` /
// `multirepo`); this function translates to the schema's spelling (`mono-repo`
// / `multi-repo`) at write-time so the two surfaces can evolve independently.
func WriteOptivemYAML(cfg *config.Config) {
	log.Info("Writing gh-optivem.yaml...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would write gh-optivem.yaml")
		return
	}

	pc := buildOptivemYAML(cfg)

	writeOptivemYAMLToDir(cfg.RepoDir, pc)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			writeOptivemYAMLToDir(cfg.BackendRepoDir, pc)
			writeOptivemYAMLToDir(cfg.FrontendRepoDir, pc)
		} else {
			writeOptivemYAMLToDir(cfg.SystemRepoDir, pc)
		}
	}

	log.Success("Wrote gh-optivem.yaml")
}

// WriteOptivemYAMLToPath renders cfg as a projectconfig.Config and writes it to
// <dir>/gh-optivem.yaml. Single-target sibling of WriteOptivemYAML — used by
// `gh optivem config init` where the caller knows exactly one directory to
// write into (CWD or --dir), with no multirepo fan-out.
func WriteOptivemYAMLToPath(cfg *config.Config, dir string) error {
	pc := buildOptivemYAML(cfg)
	return projectconfig.Write(dir, pc)
}

func writeOptivemYAMLToDir(dir string, pc *projectconfig.Config) {
	if err := projectconfig.Write(dir, pc); err != nil {
		log.Fatalf("Write gh-optivem.yaml: %v", err)
	}
}

// buildOptivemYAML translates the init Config into the projectconfig schema.
// Kept as a pure function (no I/O) so tests can verify the translation
// independently of file writing.
func buildOptivemYAML(cfg *config.Config) *projectconfig.Config {
	pc := &projectconfig.Config{
		Project: projectconfig.Project{
			URL:          cfg.ProjectURL,
			RepoStrategy: mapRepoStrategy(cfg.RepoStrategy),
			Repos:        repoSlugs(cfg),
		},
		Scope: projectconfig.Scope{
			Architecture: cfg.Arch,
			SystemLang:   systemLangFor(cfg),
			TestLang:     cfg.TestLang,
		},
	}
	return pc
}

// mapRepoStrategy converts the init flag's spelling to the projectconfig
// schema's spelling. Empty input → empty output (the schema accepts absent).
func mapRepoStrategy(s string) string {
	switch s {
	case "monorepo":
		return projectconfig.RepoStrategyMonoRepo
	case "multirepo":
		return projectconfig.RepoStrategyMultiRepo
	default:
		return s
	}
}

// systemLangFor resolves the single system_lang value for the YAML. For
// monolith the system language IS Lang; for multitier we surface the backend
// language, since system tests run end-to-end against the backend service
// (the frontend tier is captured separately in scope or out of scope).
func systemLangFor(cfg *config.Config) string {
	if cfg.Arch == "multitier" {
		return cfg.BackendLang
	}
	return cfg.Lang
}

// repoSlugs returns the owner/repo list to write to project.repos. Empty for
// mono-repo (the schema accepts mono-repo with empty repos as the implicit-
// self case). Multi-repo enumerates the per-tier slugs so the field matches
// the schema's "non-empty repos required" rule.
func repoSlugs(cfg *config.Config) []string {
	if cfg.RepoStrategy != "multirepo" {
		return nil
	}
	if cfg.Arch == "multitier" {
		return []string{cfg.BackendFullRepo, cfg.FrontendFullRepo}
	}
	return []string{cfg.SystemFullRepo}
}
