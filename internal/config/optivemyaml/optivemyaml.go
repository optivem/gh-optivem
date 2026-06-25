// Package optivemyaml builds a projectconfig.Config (the gh-optivem.yaml
// schema) from an init config.Config, and writes it to disk.
package optivemyaml

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// WriteOptivemYAMLToFilePath renders cfg as a projectconfig.Config and writes
// it to an exact yaml file path. Used by `gh optivem config init` when the
// caller has chosen a non-default filename via the persistent --config flag
// (e.g. `gh-optivem.monolith-java.yaml`).
func WriteOptivemYAMLToFilePath(cfg *config.Config, yamlPath string) error {
	pc := BuildOptivemYAML(cfg)
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
	pc := BuildOptivemYAML(cfg)
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

// BuildOptivemYAML translates the init Config into the projectconfig schema.
// Kept as a pure function (no I/O) so tests can verify the translation
// independently of file writing, and so configinit.BuildConfig can
// reuse it without pulling in the disk-write tail of runWithBanner.
//
// Tier paths are read verbatim from cfg — BuildOptivemYAML does no path
// derivation. Each call site that produces a Config supplies the paths
// matching its own on-disk layout (the flat scaffold layout via
// config.resolvePathFlagsForYAML's defaults, or explicit --*-path flag
// overrides). This keeps the YAML emitter agnostic to whether it is
// writing for a scaffolded repo, shop's worktree, or a hand-rolled layout.
func BuildOptivemYAML(cfg *config.Config) *projectconfig.Config {
	pc := &projectconfig.Config{
		Project:      projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: cfg.ProjectURL},
		RepoStrategy: mapRepoStrategy(cfg.RepoStrategy),
		SystemName:   cfg.SystemName,
		License:      cfg.License,
		Deploy:       cfg.Deploy,
	}
	if cfg.Arch == "" {
		// Partial config (no architecture chosen yet) — emit just the
		// project + repo-strategy + identity keys; the rest stays empty
		// and Validate accepts that shape.
		return pc
	}
	derived := projectconfig.DeriveSonarProjects(cfg.Owner, cfg.Repo, cfg.Arch, cfg.RepoStrategy)
	pc.Sonar = projectconfig.Sonar{Organization: strings.ToLower(cfg.Owner)}
	// javaPackage is the resolved `com/<org>/<sut>` source package the Java
	// testkit/test trees live under. Derived from the SAME owner + system-name
	// the ReplaceNamespaces / ReplaceSystemName passes use (the `mycompany`→owner,
	// `myshop`→system-name lowercase forms), so the emitted `paths:` block matches
	// the just-renamed on-disk tree by construction. Derived from cfg.Owner /
	// cfg.SystemName directly (not the pre-computed *Casings fields) so it is
	// robust on call paths that populate the primitives but not the casings.
	// DefaultPaths consumes it only for Java (plan 20260526-1430).
	javaPackage := path.Join("com",
		config.OwnerCasings(cfg.Owner).Lower,
		config.SystemCasings(cfg.SystemName).Lower)
	pc.System = buildSystem(cfg, derived)
	pc.SystemTest = buildSystemTest(cfg, derived)
	// external-systems is intentionally NOT scaffolded (plan 20260606-1356,
	// option 1B): it is a per-system map keyed by external-system name, and
	// the flat scaffold flags carry no name to key on. Operators add entries
	// by hand (the Rule-22a "operator adds the lines" posture); `init` leaves
	// the block absent.
	// `init` writes `system-test.paths:` as the authoritative initial value
	// matching the directory tree this same scaffolder just created — not a
	// runtime default. The scaffolder owns both sides of the join (YAML +
	// tree), so the values are correct by construction here. After init the
	// paths block is operator-owned: Rule 22a in `projectconfig.Validate`
	// rejects missing/unknown keys, and `gh optivem config migrate` no
	// longer back-fills defaults. Do not generalise this `DefaultPaths`
	// call into a "default at validate-time" or "default at migrate-time"
	// helper — see `internal/projectconfig/path-keys.md` for the doctrine.
	pc.SystemTest.Paths = projectconfig.DefaultPaths(cfg.TestLang, cfg.SystemTestPath, javaPackage)
	// `init` writes channels: as the scaffold-authoritative initial value
	// matching the api+ui testkit copySystemTests just produced — the SSoT
	// the ChannelType codegen and the channel-by-channel runtime read.
	// Operator-owned afterwards (narrow to [api] for an API-only project);
	// no migrate-time or validate-time back-fill, same doctrine as the
	// DefaultPaths call above.
	pc.Channels = projectconfig.DefaultChannels()
	// Per-channel System Driver adapter members — the narrow write-scope +
	// resume footprint each channel team owns. Written for the same channel set
	// as channels: above, at the per-language casing the testkit just produced
	// (TS/Java `.../api`, .NET `.../Api`). Scaffold-authoritative; operator-owned
	// afterwards, no migrate-time or validate-time back-fill (same doctrine as
	// the DefaultPaths / DefaultChannels calls above).
	pc.SystemTest.SystemDriverAdapterChannels = projectconfig.DefaultSystemDriverAdapterChannels(cfg.TestLang, cfg.SystemTestPath, javaPackage, pc.Channels)
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
// and Frontend. Paths come from cfg verbatim — the resolution
// responsibility lives upstream in config.resolvePathFlagsForYAML, which
// fills empties with the flat scaffold defaults. sonar-project values
// come from the pre-computed DerivedSonar — the scaffolder seeds the
// default per-tier keys here; downstream consumers read them straight
// from the emitted YAML.
//
// The monolith `s.Path` is `cfg.SystemPath` verbatim — the system code
// root, with no sut-namespace segment baked in. This matches the flat
// scaffold layout (`system/`), the shop reference worktree
// (`system/monolith/java`), and multitier's un-baked Backend/Frontend
// paths. Resolution lives upstream in config.resolvePathFlagsForYAML;
// BuildOptivemYAML does no path derivation. The first consumer of
// `<system.path>/component-tests.yaml` (the component-test runner)
// depends on this verbatim value resolving to the real on-disk dir.
//
// `s.DbMigrationPath` is set unconditionally to the doctrinal default
// `system/db/migrations` whenever architecture is set — the migration
// set is architecture- and language-agnostic, one shared directory tree
// consumed by every SUT (3 langs × 2 archs) via a Flyway sidecar.
// Validate Rule 22b requires this field once architecture is set; the
// scaffolder owns the initial value, then the operator owns it.
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
	case "microservices":
		// Microservices is YAML-authored (D7): the per-service facts (path,
		// repo, lang, sonar-project) come straight from the loaded
		// backend-services: map — DeriveSonarProjects has no microservices
		// branch, so each service carries its own key rather than a
		// flag-derived one. Re-emit the map verbatim from the carrier (in the
		// sorted-name order FillRawFlagsFromYAML produced) so the round-trip is
		// faithful. The single frontend (D5) re-emits its authored repo slug.
		s.BackendServices = make(map[string]projectconfig.TierSpec, len(cfg.BackendServices))
		for _, svc := range cfg.BackendServices {
			s.BackendServices[svc.Name] = projectconfig.TierSpec{
				Path:         svc.Path,
				Repo:         svc.Repo,
				Lang:         svc.Lang,
				SonarProject: svc.SonarProject,
			}
		}
		s.Frontend = projectconfig.TierSpec{
			Path:         cfg.FrontendPath,
			Repo:         cfg.FrontendRepoSlug,
			Lang:         projectconfig.LangTypescript,
			SonarProject: derived.Frontend,
		}
	}
	s.DbMigrationPath = projectconfig.DefaultDbMigrationPath
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

// systemTestRepoSlug returns the slug for system-test. Defaults to the
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
