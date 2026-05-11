package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Auto-populated path defaults written into the scaffolded gh-optivem.yaml's
// `system.config:` / `system_test.config:` fields. They mirror the layout that
// copySystemTests produces (docker/<arch>/<lang>/ collapses to docker/, and
// system-test/<lang>/ collapses to system-test/ — see apply_template.go's
// flattening rules). The scaffold writes a pair: gh-optivem.yaml points at
// tests.yaml (latest suites), gh-optivem.legacy.yaml points at
// tests.legacy.yaml (mod-numbered legacy suites). The acceptance-stage and
// acceptance-stage-legacy workflows pick between them via GH_OPTIVEM_CONFIG.
const (
	scaffoldedSystemConfigPath     = "docker/systems.yaml"
	scaffoldedTestConfigPath       = "system-test/tests.yaml"
	scaffoldedTestConfigPathLegacy = "system-test/tests.legacy.yaml"
	scaffoldedConfigFilenameLegacy = "gh-optivem.legacy.yaml"
)

// WriteOptivemYAML writes <repoRoot>/gh-optivem.yaml in the scaffolded repo(s),
// translating already-resolved init flags into the projectconfig.Config schema.
// The file is consumed by the ATDD pipeline at runtime (project URL, repo
// strategy, system architecture + per-component layout, system_test layout,
// and external-system stand-in declarations).
//
// Multi-repo: writes the same file to every per-tier repo so `gh optivem atdd
// implement-ticket` can be invoked from any of them.
//
// `cfg.RepoStrategy` arrives in the init flag's spelling (`monorepo` /
// `multirepo`); this function translates to the schema's spelling (`mono-repo`
// / `multi-repo`) at write-time so the two surfaces can evolve independently.
//
// System.Config / SystemTest.Config are auto-populated to the paths that
// copySystemTests just produced (`docker/systems.yaml`,
// `system-test/tests.yaml`). Without this, a freshly-scaffolded repo
// wouldn't work with `gh optivem run|test system` from repo root — the runner
// defaults are `./systems.yaml` / `./tests.yaml` in the working directory,
// which don't resolve from the scaffolded repo root.
func WriteOptivemYAML(cfg *config.Config) {
	log.Info("Writing gh-optivem.yaml + gh-optivem.legacy.yaml...")

	if cfg.DryRun {
		log.Info("[DRY RUN] Would write gh-optivem.yaml + gh-optivem.legacy.yaml")
		return
	}

	pc := buildOptivemYAML(cfg)
	pc.System.Config = scaffoldedSystemConfigPath
	pc.SystemTest.Config = scaffoldedTestConfigPath

	pcLegacy := buildOptivemYAML(cfg)
	pcLegacy.System.Config = scaffoldedSystemConfigPath
	pcLegacy.SystemTest.Config = scaffoldedTestConfigPathLegacy

	writePair := func(dir string) {
		writeOptivemYAMLToDirIfNotSource(cfg, dir, pc)
		writeLegacyOptivemYAMLToDir(dir, pcLegacy)
	}

	writePair(cfg.RepoDir)

	if cfg.RepoStrategy == "multirepo" {
		if cfg.Arch == "multitier" {
			writePair(cfg.BackendRepoDir)
			writePair(cfg.FrontendRepoDir)
		} else {
			writePair(cfg.SystemRepoDir)
		}
	}

	log.Success("Wrote gh-optivem.yaml + gh-optivem.legacy.yaml")
}

// writeLegacyOptivemYAMLToDir writes <dir>/gh-optivem.legacy.yaml. The
// legacy variant is never the source config (callers point --config at the
// latest file or at one of shop's per-flavor templates), so there's no
// source-overwrite guard — unlike the latest file.
func writeLegacyOptivemYAMLToDir(dir string, pc *projectconfig.Config) {
	if dir == "" {
		return
	}
	yamlPath := filepath.Join(dir, scaffoldedConfigFilenameLegacy)
	if err := projectconfig.WriteToPath(yamlPath, pc); err != nil {
		log.Fatalf("Write %s: %v", scaffoldedConfigFilenameLegacy, err)
	}
}

// writeOptivemYAMLToDirIfNotSource writes the scaffolded gh-optivem.yaml
// unless its target path resolves to the same file as cfg.SourceConfigPath
// — the case where the operator ran init from inside cfg.RepoDir and the
// project-board step has already written the source-config form there.
// Overwriting in that case would replace the project-board write with the
// scaffolded-repo content (path-fields, etc.) and undo the persist step.
func writeOptivemYAMLToDirIfNotSource(cfg *config.Config, dir string, pc *projectconfig.Config) {
	if dir == "" {
		return
	}
	if cfg.SourceConfigPath != "" {
		target, terr := filepath.Abs(filepath.Join(dir, projectconfig.Path))
		source, serr := filepath.Abs(cfg.SourceConfigPath)
		if terr == nil && serr == nil && filepath.Clean(target) == filepath.Clean(source) {
			log.Infof("gh-optivem.yaml at %s is the source config — skipping scaffolded overwrite", target)
			return
		}
	}
	writeOptivemYAMLToDir(dir, pc)
}

// WriteOptivemYAMLToPath renders cfg as a projectconfig.Config and writes it to
// <dir>/gh-optivem.yaml. Single-target sibling of WriteOptivemYAML — used by
// `gh optivem config init` where the caller knows exactly one directory to
// write into (CWD or --dir), with no multirepo fan-out.
func WriteOptivemYAMLToPath(cfg *config.Config, dir string) error {
	pc := buildOptivemYAML(cfg)
	return projectconfig.Write(dir, pc)
}

// WriteOptivemYAMLToFilePath renders cfg as a projectconfig.Config and writes
// it to an exact yaml file path. Used by `gh optivem config init` when the
// caller has chosen a non-default filename via the persistent --config flag
// (e.g. `gh-optivem.monolith-java.yaml`).
func WriteOptivemYAMLToFilePath(cfg *config.Config, yamlPath string) error {
	pc := buildOptivemYAML(cfg)
	return projectconfig.WriteToPath(yamlPath, pc)
}

// WriteOptivemYAMLToFilePathWithBanner is the variant used by the
// interactive `config init` recovery flow: marshals the YAML and prepends
// a banner comment block so the operator sees which fields were
// defaulted before running `gh optivem config validate`. The non-
// interactive command keeps using WriteOptivemYAMLToFilePath (no banner)
// — operators running that one have supplied every flag and don't need
// a review checklist.
func WriteOptivemYAMLToFilePathWithBanner(cfg *config.Config, yamlPath, banner string) error {
	pc := buildOptivemYAML(cfg)
	data, err := projectconfig.Marshal(pc)
	if err != nil {
		return err
	}
	body := append([]byte(banner), data...)
	if err := os.WriteFile(yamlPath, body, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", yamlPath, err)
	}
	return nil
}

func writeOptivemYAMLToDir(dir string, pc *projectconfig.Config) {
	if err := projectconfig.Write(dir, pc); err != nil {
		log.Fatalf("Write gh-optivem.yaml: %v", err)
	}
}

// buildOptivemYAML translates the init Config into the projectconfig schema.
// Kept as a pure function (no I/O) so tests can verify the translation
// independently of file writing.
//
// Tier paths are read verbatim from cfg — buildOptivemYAML does no path
// derivation. Each call site that produces a Config supplies the paths
// matching its own on-disk layout (the scaffolder via resolveScaffoldPaths,
// `config init` via explicit --*-path flags). This keeps the YAML emitter
// agnostic to whether it is writing for a scaffolded repo, shop's worktree,
// or a hand-rolled layout.
func buildOptivemYAML(cfg *config.Config) *projectconfig.Config {
	pc := &projectconfig.Config{
		Project:      projectconfig.Project{URL: cfg.ProjectURL},
		RepoStrategy: mapRepoStrategy(cfg.RepoStrategy),
		SystemName:   cfg.SystemName,
		License:      cfg.License,
		Deploy:       cfg.Deploy,
	}
	if cfg.Arch == "" {
		// Partial config (no architecture chosen yet) — emit just the
		// project + repo_strategy + identity keys; the rest stays empty
		// and Validate accepts that shape.
		return pc
	}
	derived := projectconfig.DeriveSonarProjects(cfg.Owner, cfg.Repo, cfg.Arch, cfg.RepoStrategy)
	pc.Sonar = projectconfig.Sonar{Organization: strings.ToLower(cfg.Owner)}
	pc.System = buildSystem(cfg, derived)
	pc.SystemTest = buildSystemTest(cfg, derived)
	pc.ExternalSystems = buildExternals(cfg)
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

// buildSystem populates the System block from cfg. Polymorphic by
// architecture: monolith uses flat Path/Repo/Lang; multitier nests Backend
// and Frontend. Paths come from cfg verbatim — the resolution responsibility
// lives at the call site (resolveScaffoldPaths for `init`, --*-path flags
// for `config init`). sonar_project values come from the pre-computed
// DerivedSonar so the per-tier key matches projectconfig.Validate's
// Rule 19 derivation.
func buildSystem(cfg *config.Config, derived projectconfig.DerivedSonar) projectconfig.System {
	s := projectconfig.System{Architecture: cfg.Arch}
	switch cfg.Arch {
	case "monolith":
		s.Path = cfg.SystemPath
		s.Repo = systemRepoSlug(cfg)
		s.Lang = cfg.Lang
		s.SonarProject = derived.System
	case "multitier":
		s.Backend = projectconfig.TierSpec{
			Path:         cfg.BackendPath,
			Repo:         backendRepoSlug(cfg),
			Lang:         cfg.BackendLang,
			SonarProject: derived.Backend,
		}
		// The scaffold currently emits a single React+TypeScript frontend
		// regardless of system_lang. Lang is the underlying source
		// language (typescript), not the framework — adding more
		// frontend frameworks later is out of scope.
		s.Frontend = projectconfig.TierSpec{
			Path:         cfg.FrontendPath,
			Repo:         frontendRepoSlug(cfg),
			Lang:         projectconfig.LangTypescript,
			SonarProject: derived.Frontend,
		}
	}
	return s
}

// buildSystemTest populates the SystemTest tier from cfg.SystemTestPath.
func buildSystemTest(cfg *config.Config, derived projectconfig.DerivedSonar) projectconfig.TierSpec {
	return projectconfig.TierSpec{
		Path:         cfg.SystemTestPath,
		Repo:         systemTestRepoSlug(cfg),
		Lang:         cfg.TestLang,
		SonarProject: derived.SystemTest,
	}
}

// buildExternals populates the ExternalSystems block from cfg.StubsPath and
// cfg.SimulatorsPath.
func buildExternals(cfg *config.Config) projectconfig.ExternalSystems {
	repo := externalsRepoSlug(cfg)
	return projectconfig.ExternalSystems{
		Stubs:      projectconfig.ExternalSpec{Path: cfg.StubsPath, Repo: repo},
		Simulators: projectconfig.ExternalSpec{Path: cfg.SimulatorsPath, Repo: repo},
	}
}

// systemRepoSlug returns the slug for the monolith system tier:
//   - mono-repo: cfg.FullRepo (the workspace repo)
//   - multi-repo: cfg.SystemFullRepo (the per-system repo)
func systemRepoSlug(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return cfg.SystemFullRepo
	}
	return cfg.FullRepo
}

// backendRepoSlug returns the slug for the multitier backend tier.
func backendRepoSlug(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return cfg.BackendFullRepo
	}
	return cfg.FullRepo
}

// frontendRepoSlug returns the slug for the multitier frontend tier.
func frontendRepoSlug(cfg *config.Config) string {
	if cfg.RepoStrategy == "multirepo" {
		return cfg.FrontendFullRepo
	}
	return cfg.FullRepo
}

// systemTestRepoSlug returns the slug for system_test. Defaults to the
// system repo (mono-repo or multi-repo monolith) or the backend repo
// (multi-repo multitier) — the operator can override post-scaffold.
func systemTestRepoSlug(cfg *config.Config) string {
	if cfg.RepoStrategy != "multirepo" {
		return cfg.FullRepo
	}
	if cfg.Arch == "multitier" {
		return cfg.BackendFullRepo
	}
	return cfg.SystemFullRepo
}

// externalsRepoSlug returns the slug for external-system tiers, mirroring
// the system-test default (operator can override post-scaffold).
func externalsRepoSlug(cfg *config.Config) string {
	return systemTestRepoSlug(cfg)
}
