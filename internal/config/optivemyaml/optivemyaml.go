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
	// SSoT (plan 20260518-1530 item 3): sutNamespace is a scaffold-time
	// input — derived from the system repo slug's last segment, mirroring
	// the pre-SSoT runtime rule in `projectconfig.Config.SutNamespace`.
	// Baked into `System.Path` (monolith) and the `paths:` testkit-key
	// values so the resulting gh-optivem.yaml carries fully-resolved
	// paths and the `system.sut-namespace` field is no longer persisted.
	sutNamespace := lastSlashSegment(systemRepoSlug(cfg))
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
	pc.System = buildSystem(cfg, derived, sutNamespace)
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

// lastSlashSegment returns the substring after the final "/" in s, or s
// itself if there is no "/". Used to derive sutNamespace from
// systemRepoSlug(cfg) at scaffold time (e.g. "x/shop" → "shop").
// Mirrors projectconfig.Config.SutNamespace's repo-derivation rule.
func lastSlashSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return s
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
// SSoT (plan 20260518-1530 item 3): the monolith `s.Path` is fully
// resolved at scaffold time by joining `cfg.SystemPath` with
// `sutNamespace` (when non-empty). An empty `sutNamespace` reproduces
// the pre-SSoT shape (just `cfg.SystemPath`) — used for partial configs
// and for multirepo-multitier (where systemRepoSlug returns "").
// Multitier's nested Backend/Frontend Paths are not resolved here:
// multitier scope is deferred per plan item 11's allowlist.
//
// `s.DbMigrationPath` is set unconditionally to the doctrinal default
// `system/db/migrations` whenever architecture is set — the migration
// set is architecture- and language-agnostic, one shared directory tree
// consumed by every SUT (3 langs × 2 archs) via a Flyway sidecar.
// Validate Rule 22b requires this field once architecture is set; the
// scaffolder owns the initial value, then the operator owns it.
func buildSystem(cfg *config.Config, derived projectconfig.DerivedSonar, sutNamespace string) projectconfig.System {
	s := projectconfig.System{Architecture: cfg.Arch}
	switch cfg.Arch {
	case "monolith":
		s.Path = cfg.SystemPath
		if sutNamespace != "" {
			s.Path = path.Join(cfg.SystemPath, sutNamespace)
		}
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
