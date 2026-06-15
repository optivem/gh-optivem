package steps

import (
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/config/optivemyaml"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Auto-populated path defaults written into the scaffolded gh-optivem.yaml's
// `system.config:` / `system-test.config:` fields. They mirror the layout that
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
// strategy, system architecture + per-component layout, system-test layout,
// and external-system stand-in declarations).
//
// Multi-repo: writes the same file to every per-tier repo so `gh optivem
// implement` can be invoked from any of them.
//
// `cfg.RepoStrategy` arrives in the init flag's spelling (`monorepo` /
// `multirepo`); this function translates to the schema's spelling (`mono-repo`
// / `multi-repo`) at write-time so the two surfaces can evolve independently.
//
// System.Config / SystemTest.Config are auto-populated to the paths that
// copySystemTests just produced (`docker/systems.yaml`,
// `system-test/tests.yaml`). Without this, a freshly-scaffolded repo
// wouldn't work with `gh optivem system start` / `gh optivem test run` from repo root — the runner
// defaults are `./systems.yaml` / `./tests.yaml` in the working directory,
// which don't resolve from the scaffolded repo root.
func WriteOptivemYAML(cfg *config.Config) {
	log.Info("Writing gh-optivem.yaml + gh-optivem.legacy.yaml...")

	pc := optivemyaml.BuildOptivemYAML(cfg)
	pc.System.Config = scaffoldedSystemConfigPath
	pc.SystemTest.Config = scaffoldedTestConfigPath

	pcLegacy := optivemyaml.BuildOptivemYAML(cfg)
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
	pc := optivemyaml.BuildOptivemYAML(cfg)
	return projectconfig.Write(dir, pc)
}

func writeOptivemYAMLToDir(dir string, pc *projectconfig.Config) {
	if err := projectconfig.Write(dir, pc); err != nil {
		log.Fatalf("Write gh-optivem.yaml: %v", err)
	}
}
