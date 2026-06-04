// Package projectconfig loads the consumer repo's per-project configuration
// file at gh-optivem.yaml (project root). The file holds project-level facts
// — board URL, repo strategy, system architecture + per-component layout,
// system-test layout, and optional external-system stand-in declarations
// — that legitimately differ between consumer repos but stay stable across
// pipeline invocations within one repo.
//
// The file is optional; absence is not an error. Callers either consult
// it as the sole source of project URL (board.ResolveProjectURL — there
// is no discovery fallback) or thread its contents into agent prompt
// context (driver.Run via Context.Params).
//
// The package sits at internal/projectconfig (not internal/config) because
// the latter is taken by the scaffold runner CLI's flag parser, and the two
// have nothing in common beyond the word "config".
//
// The schema is intentionally small. New top-level groups can be added
// without breaking existing repos — each section is independently optional.
// All non-empty values are validated against the documented enums; absence
// is accepted everywhere so that a half-populated file (e.g. only project
// URL set) keeps working.
package projectconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// Path is the canonical relative location of the file inside a consumer
// repo (project root). Read from the consumer's CWD — gh-optivem is
// repo-agnostic by design.
const Path = "gh-optivem.yaml"

// EnvVar is the environment-variable equivalent of the persistent
// --config / -c flag. When set, every command resolves config lookups
// to its value (unless the flag is explicitly passed, which wins).
const EnvVar = "GH_OPTIVEM_CONFIG"

// MissingFileError returns the canonical "no gh-optivem.yaml at <path>"
// error. Single-sourced so every entry point (`gh optivem init`, the
// runner-tier commands, the configinit bridge used by `config validate`
// / `compile` / `implement`) produces identical wording.
// The message names all three ways an operator can resolve a config:
// create one (`config init`), pass --config, or set GH_OPTIVEM_CONFIG.
func MissingFileError(path string) error {
	return fmt.Errorf("no gh-optivem.yaml at %s; run `gh optivem config init` to create one, or pass --config <path> (or set %s) to point at an existing one", path, EnvVar)
}

// Repo strategy enum values, surfaced as YAML strings.
const (
	RepoStrategyMonoRepo  = "mono-repo"
	RepoStrategyMultiRepo = "multi-repo"
)

// Architecture enum values.
const (
	ArchMonolith  = "monolith"
	ArchMultitier = "multitier"
)

// Language enum values (shared by every TierSpec.Lang).
const (
	LangJava       = "java"
	LangDotnet     = "dotnet"
	LangTypescript = "typescript"
)

// Provider enum values for project.provider. Names the tracker backend
// the runtime opens against project.url. The set is intentionally small
// — github is the production path; markdown is the network-free escape
// hatch used for offline workshops or during GitHub outages. A future
// jira value would slot in alongside.
const (
	ProviderGitHub   = "github"
	ProviderMarkdown = "markdown"
)

// Config mirrors the YAML schema. Top-level keys:
//   - project:          GitHub identity (currently just board URL).
//   - repo-strategy:    mono-repo | multi-repo (top-level scalar, not a
//                       property of the project board).
//   - system:           the system being built — polymorphic by architecture.
//   - system-test:      the test suite that drives the system.
//   - external-systems: optional vendored stand-ins (stubs, simulators).
//   - system-name:      human-readable system label for templating.
//   - license:          license key for the scaffolded LICENSE file.
//   - deploy:           deployment target (docker | cloud-run).
type Config struct {
	Project         Project         `yaml:"project"`
	RepoStrategy    string          `yaml:"repo-strategy,omitempty"`
	Sonar           Sonar           `yaml:"sonar,omitempty"`
	System          System          `yaml:"system"`
	SystemTest      TierSpec        `yaml:"system-test"`
	ExternalSystems ExternalSystems `yaml:"external-systems,omitempty"`

	// LocalRepos declares the project's own constituent local repos —
	// multitier projects whose frontend, backend, and system-test live in
	// separate folders. The workspace scope cascade reads it as the
	// project-iteration row that sits between "workspace file reachable"
	// and "cwd repo only": when gh-optivem.yaml has a non-empty repos:,
	// `gh optivem commit` and the other cross-repo verbs iterate the
	// listed folders instead of falling back to the cwd repo.
	//
	// Each entry's Path is project-relative — resolved against the
	// directory containing gh-optivem.yaml. Empty or absent means
	// "single-repo project; use the cwd repo". Back-filled by
	// `gh optivem config migrate` for existing multitier configs.
	//
	// The Go field is named LocalRepos to avoid colliding with the
	// pre-existing (*Config).Repos() method that returns tier repo
	// slugs.
	LocalRepos []RepoEntry `yaml:"repos,omitempty"`

	// SystemName is the human-readable system label (e.g. "Page Turner")
	// that drives template substitution at `gh optivem init` — Java package
	// names, .NET namespaces, TypeScript package names, README headings.
	// Project-stable: an operator does not change a system's name across
	// init invocations. Validated against ValidateSystemName when set.
	SystemName string `yaml:"system-name,omitempty"`

	// License is the SPDX-like license key used to emit the scaffolded
	// repo's LICENSE file and README badge (mit, apache-2.0, gpl-3.0,
	// bsd-2-clause, bsd-3-clause, unlicense). Validated against the known
	// set when set.
	License string `yaml:"license,omitempty"`

	// Deploy is the deployment target the scaffolded repo's workflows
	// target (docker | cloud-run). Today only docker is production-ready;
	// cloud-run is reserved for the in-development path.
	Deploy string `yaml:"deploy,omitempty"`

	// Channels declares the project's delivery channels — the subset of the
	// supported channel set (api, ui) this project exposes and its
	// acceptance suite drives. shop is [api, ui]; an API-only project is
	// [api]. It is the single source of truth for the channel set: the
	// scaffold generates the per-language ChannelType constants from it, and
	// the runtime drives channel-by-channel system implementation off it.
	//
	// Tokens are lowercase canonical and drawn from the closed enum in
	// channels.go (Validate rejects unknown tokens, and casing slips on a
	// real channel get a did-you-mean) because the tokens are identity for
	// the selectors derived from them — acceptance-${channel} →
	// acceptance-api / acceptance-ui. List order is implementation order.
	// Scaffold-authoritative: `gh optivem init` writes the value matching
	// the scaffolded testkit; operator-owned afterwards. Optional/absent is
	// accepted (a pre-this-field config keeps working; no migrate or
	// validate-time back-fill).
	Channels []string `yaml:"channels,omitempty"`

	// ProcessFlow is the repo-relative path to a process-flow YAML override.
	// When empty, the driver falls back to the canonical embedded document.
	// Validated as repo-relative (no `..`); file existence is verified at
	// load-time, not validate-time.
	ProcessFlow string `yaml:"process-flow,omitempty"`

	// TaskPrompts maps MID task-name → repo-relative path to a prompt
	// body that replaces the embedded one. Partial overrides are allowed
	// (an entry per task the operator wants to customize). Keys are the
	// task-name verbs declared on each writing-agent MID's EXECUTE_AGENT
	// call-activity in the embedded process-flow.yaml; values pass
	// validatePath. The plan 20260526-1701 split separated task-name from
	// agent — keys here remain the task-name verb (operator-facing schema
	// is preserved), enumerated from the engine at Validate time.
	TaskPrompts map[string]string `yaml:"task-prompts,omitempty"`

	// NodeExtras maps process-flow node ID → literal text appended to that
	// node's prompt. Values are inline (project-stable advice such as
	// "prefer record types"). Node-ID keys are not validated here — the
	// driver re-validates them against the loaded process flow at startup.
	NodeExtras map[string]string `yaml:"node-extras,omitempty"`

	// NodeReplacements maps process-flow node ID → repo-relative path
	// whose file body replaces that node's prompt verbatim. Values pass
	// validatePath. Like NodeExtras, node-ID keys are validated at
	// startup, not here. A key appearing in both NodeExtras and
	// NodeReplacements is rejected — replacement strictly supersedes
	// extras, so the combination signals operator confusion.
	NodeReplacements map[string]string `yaml:"node-replacements,omitempty"`
}

// Project carries the tracker backend identity: which Tracker adapter
// runs the pipeline (Provider) and where its board lives (URL). Repo
// organization (mono-repo vs multi-repo) lives at top level, not here,
// because it affects every tier's repo: field across system: and
// system-test:.
//
// Provider is mandatory — Validate rejects empty and unknown values
// and routes operators at `gh optivem config migrate` to add it
// idempotently from an existing url. URL may be empty at validate-time
// (`gh optivem init` Path A populates it on first run); when set, its
// shape is checked against Provider so a github URL paired with
// `provider: markdown` (or vice versa) errors loudly with both fields
// named.
type Project struct {
	Provider string `yaml:"provider,omitempty"`
	URL      string `yaml:"url,omitempty"`
}

// Sonar carries SonarCloud account identity. Singleton at the root because
// one scaffold maps to one SonarCloud organization regardless of how many
// code tiers it produces. Per-tier project keys live on the tier (see
// System.SonarProject and TierSpec.SonarProject) — that 1:1 placement
// mirrors how path, repo, and lang already sit on each tier.
//
// Organization is `strings.ToLower(owner)` by default, materialized by
// `gh optivem config init`. The field is its own slot (not derived at
// every read) so an operator whose SonarCloud org differs from their
// GitHub owner can override the value without a schema migration.
type Sonar struct {
	Organization string `yaml:"organization,omitempty"`
}

// System describes the system being built. Polymorphic by architecture:
//   - monolith:  Path/Repo/Lang are populated; Backend/Frontend are empty.
//   - multitier: Backend/Frontend are populated; Path/Repo/Lang are empty.
//
// Validate enforces exclusivity. Operators reading the file should see
// exactly one shape per architecture, never both.
type System struct {
	Architecture string `yaml:"architecture,omitempty"`

	// Config is the repo-relative path to systems.yaml (or legacy systems.json)
	// for `gh optivem system <verb>` and `gh optivem test run`. Optional; when empty, runner
	// commands fall back to ./systems.yaml or the explicit --system-config flag.
	// Free-form path (no enum/shape check) — file existence is verified at
	// load-time, not validate-time, matching the convention for the other
	// path fields in this schema. Belongs on System (not at top level)
	// because it's a property of the system being run — the runner
	// orchestration config, sibling to System.Path / System.Backend.Path.
	Config string `yaml:"config,omitempty"`

	// Monolith-only.
	Path         string `yaml:"path,omitempty"`
	Repo         string `yaml:"repo,omitempty"`
	Lang         string `yaml:"lang,omitempty"`
	SonarProject string `yaml:"sonar-project,omitempty"`

	// DbMigrationPath names the canonical Flyway-style migration set shared
	// across every SUT (3 languages × 2 architectures). Sibling of
	// system/monolith/ and system/multitier/, not a child of either: schema
	// migrations are architecture- and language-agnostic, one ordered set
	// consumed by all six SUT implementations via a Flyway sidecar.
	//
	// The field is **gh-optivem-owned in name** (Family A canonical key
	// `system-db-migration-path`) and **operator-owned in value** (paths can
	// point at a legacy SQL tree outside the gh-optivem-aware layout).
	// `gh optivem init` writes the default `system/db/migrations` matching
	// the shop template; `Validate` (Rule 22b) requires it when
	// architecture is set; `gh optivem config migrate` back-fills the
	// default exactly once for pre-this-plan configs.
	//
	// The scope-eligibility side of this lives in
	// `internal/atdd/phase_scopes.go` (`FamilyAPathKeysInScope`) — without
	// that entry the per-phase scope checker rejects the key.
	DbMigrationPath string `yaml:"db-migration-path,omitempty"`

	// Multitier-only.
	Backend  TierSpec `yaml:"backend,omitempty"`
	Frontend TierSpec `yaml:"frontend,omitempty"`
}

// TierSpec describes one body of code: where it lives, in which repo,
// and what language it is written in. Used for backend, frontend, and
// system-test. Path/Repo/Lang are mandatory whenever the tier is set;
// no defaults, no inference. Config is only meaningful on system-test
// (the tests.yaml path) — Validate rejects it on backend/frontend.
// SonarProject carries the SonarCloud project key for the tier, required
// when system.architecture is set (system-test, backend, frontend); see
// Rules 18/19 in Validate.
//
// Paths is the user-owned map of named-location placeholders consumed by
// phase-doc substitution at sync time — the canonical Family B keys
// (system-driver-port, system-driver-adapter, at-test, …) describing the system-test
// tier's layered layout. Each key is a named location; each value is a
// repo-relative path. The scaffolder writes a default block matching
// glossary.md doctrine into every new project. Reserved keys that would
// shadow fixed-schema Family A placeholders (language, architecture,
// system-path, system-test-path, sut-namespace) are rejected by Validate.
// Meaningful only on system-test today — Validate rejects non-empty Paths
// on backend/frontend tiers, mirroring how Config is restricted.
type TierSpec struct {
	Path         string            `yaml:"path,omitempty"`
	Repo         string            `yaml:"repo,omitempty"`
	Lang         string            `yaml:"lang,omitempty"`
	Config       string            `yaml:"config,omitempty"`
	SonarProject string            `yaml:"sonar-project,omitempty"`
	Paths        map[string]string `yaml:"paths,omitempty"`
}

// ExternalSystems declares vendored stand-ins for third-party dependencies
// the system talks to during ATDD cycles. Both sub-fields are optional and
// independent — a project might use only stubs, only simulators, both, or
// neither. When a sub-field is non-empty, all of its inner fields must be
// set.
//
// Field order matches the ATDD cycle progression: Stubs (cycle 2,
// WireMock-style no-logic stand-in driven by JSON mappings) comes before
// Simulators (cycle 3, e.g. a node + json-server simulator with controlled
// state).
type ExternalSystems struct {
	Stubs      ExternalSpec `yaml:"stubs,omitempty"`
	Simulators ExternalSpec `yaml:"simulators,omitempty"`
}

// ExternalSpec describes one external-system tier. Two fields, both
// mandatory when the tier is set. No Lang field — externals are config and
// scaffolding (WireMock JSON, ad-hoc node simulators), not source code in
// the language enum sense.
type ExternalSpec struct {
	Path string `yaml:"path,omitempty"`
	Repo string `yaml:"repo,omitempty"`
}

// RepoEntry is one entry in Config.LocalRepos — a single local repo path
// the project iterates over for cross-repo verbs. Path is project-relative
// (resolved against the directory containing gh-optivem.yaml), validated
// against the same shape rules as every other path in this schema
// (no absolute paths, no `..` segments).
//
// The struct exists (rather than a plain []string) so future per-repo
// metadata — kind, optional, lint exclusions — can be added without a
// breaking schema change. Today only Path is set.
type RepoEntry struct {
	Path string `yaml:"path,omitempty"`
}

// Repos returns the union of every tier's Repo field, sorted. Used by
// repolocator and validation to know which repos participate, without
// requiring an explicit project.repos list.
func (c *Config) Repos() []string {
	if c == nil {
		return nil
	}
	set := map[string]struct{}{}
	add := func(r string) {
		if r != "" {
			set[r] = struct{}{}
		}
	}
	add(c.System.Repo)
	add(c.System.Backend.Repo)
	add(c.System.Frontend.Repo)
	add(c.SystemTest.Repo)
	add(c.ExternalSystems.Stubs.Repo)
	add(c.ExternalSystems.Simulators.Repo)
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// reservedPlaceholderKeys lists the Family A placeholder names that
// Paths entries (Family B) must not shadow. Names that come from existing
// top-level config fields belong here so a typo'd `paths.language: x`
// can't quietly override the canonical system.lang value.
//
// All keys are kebab — the project-wide YAML/identifier convention.
var reservedPlaceholderKeys = map[string]struct{}{
	"language":                 {},
	"architecture":             {},
	"system-path":              {},
	"system-db-migration-path": {},
	"system-test-path":         {},
	"sut-namespace":            {},
}

// SutNamespace returns the substitution value for ${sut-namespace}.
// Derived from the last path segment of System.Repo (e.g.
// "optivem/myShop" → "myShop"). Returns "" when System.Repo is empty —
// sync-time unfilled-placeholder checks surface the gap if a doc
// references the placeholder.
func (c *Config) SutNamespace() string {
	if c == nil {
		return ""
	}
	return lastPathSegment(c.System.Repo)
}

// PlaceholderMap returns the flat name → value map consumed by phase-doc
// substitution. Family A keys come from existing top-level config fields;
// Family B keys come from the user-owned SystemTest.Paths map. The output
// namespace stays flat — Family B keys emit at the top level alongside
// Family A. Family A wins on collision — Validate also rejects the
// collision at parse time, so the in-memory precedence is defensive cover
// for callers that mutate a Config without re-validating.
//
// `language` prefers System.Lang (monolith); when empty (multitier or a
// partial config without a system tier), falls back to SystemTest.Lang
// so a system-test-side doc reference resolves to the test suite's
// language even when there's no single SUT lang to name.
func (c *Config) PlaceholderMap() map[string]string {
	if c == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(c.SystemTest.Paths)+6)
	// Family B first; Family A overwrites on collision.
	maps.Copy(out, c.SystemTest.Paths)
	out["architecture"] = c.System.Architecture
	out["system-path"] = c.System.Path
	out["system-db-migration-path"] = c.System.DbMigrationPath
	out["system-test-path"] = c.SystemTest.Path
	out["sut-namespace"] = c.SutNamespace()
	if c.System.Lang != "" {
		out["language"] = c.System.Lang
	} else {
		out["language"] = c.SystemTest.Lang
	}
	return out
}

// lastPathSegment returns the substring after the last "/" in s. Used by
// SutNamespace to derive ${sut-namespace} from a "owner/repo" slug; an
// absent slash returns s itself.
func lastPathSegment(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 && i < len(s)-1 {
		return s[i+1:]
	}
	return s
}

// Validate enforces the enum and cross-field rules. Empty values are
// accepted — only non-empty invalid values are errors when scope is not
// set; when scope (system.architecture) IS set, the architecture-specific
// shape is required. project.url may also be empty: `gh optivem init`'s
// EnsureProjectBoard step auto-creates a board and writes the URL back
// into gh-optivem.yaml (Path A in internal/steps/project.go), so an empty
// project.url is a valid intermediate state between `config init` and
// the first `init` run. The ATDD runtime still requires a non-empty
// project.url (board.ResolveProjectURL), which is enforced there.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}

	// Rule 0b: Config on backend/frontend tiers is meaningless — the runner
	// systems.yaml path lives on the System block, not on per-tier code
	// specs. Reject early so a typo'd `system.backend.config:` doesn't
	// silently parse as a no-op.
	if c.System.Backend.Config != "" {
		return fmt.Errorf("config: system.backend.config is not a supported field (use system.config for the runner systems.yaml)")
	}
	if c.System.Frontend.Config != "" {
		return fmt.Errorf("config: system.frontend.config is not a supported field (use system.config for the runner systems.yaml)")
	}

	// Rule 1: repo strategy enum.
	switch c.RepoStrategy {
	case "", RepoStrategyMonoRepo, RepoStrategyMultiRepo:
	default:
		return fmt.Errorf("config: repo-strategy %q must be one of %q, %q",
			c.RepoStrategy, RepoStrategyMonoRepo, RepoStrategyMultiRepo)
	}

	// Rule 2: architecture enum.
	switch c.System.Architecture {
	case "", ArchMonolith, ArchMultitier:
	default:
		return fmt.Errorf("config: system.architecture %q must be one of %q, %q",
			c.System.Architecture, ArchMonolith, ArchMultitier)
	}

	// Rule 3: lang enum on every tier where it is set.
	for _, tl := range []struct {
		field string
		val   string
	}{
		{"system.lang", c.System.Lang},
		{"system.backend.lang", c.System.Backend.Lang},
		{"system.frontend.lang", c.System.Frontend.Lang},
		{"system-test.lang", c.SystemTest.Lang},
	} {
		if err := validateLang(tl.field, tl.val); err != nil {
			return err
		}
	}

	// Rule 4: path validity on every tier where it is set.
	for _, tp := range []struct {
		field string
		val   string
	}{
		{"system.path", c.System.Path},
		{"system.backend.path", c.System.Backend.Path},
		{"system.frontend.path", c.System.Frontend.Path},
		{"system.db-migration-path", c.System.DbMigrationPath},
		{"system-test.path", c.SystemTest.Path},
		{"external-systems.stubs.path", c.ExternalSystems.Stubs.Path},
		{"external-systems.simulators.path", c.ExternalSystems.Simulators.Path},
	} {
		if err := validatePath(tp.field, tp.val); err != nil {
			return err
		}
	}

	// Rule 5: architecture exclusivity.
	switch c.System.Architecture {
	case ArchMonolith:
		if c.System.Path == "" || c.System.Repo == "" || c.System.Lang == "" {
			return fmt.Errorf("config: system.architecture=%s requires system.{path,repo,lang} all set",
				ArchMonolith)
		}
		if !c.System.Backend.IsEmpty() {
			return fmt.Errorf("config: system.architecture=%s incompatible with system.backend (multitier-only field)",
				ArchMonolith)
		}
		if !c.System.Frontend.IsEmpty() {
			return fmt.Errorf("config: system.architecture=%s incompatible with system.frontend (multitier-only field)",
				ArchMonolith)
		}
	case ArchMultitier:
		if c.System.Path != "" || c.System.Repo != "" || c.System.Lang != "" {
			return fmt.Errorf("config: system.architecture=%s incompatible with system.{path,repo,lang} (monolith-only fields); use system.backend / system.frontend",
				ArchMultitier)
		}
		if c.System.Backend.IsEmpty() {
			return fmt.Errorf("config: system.architecture=%s requires system.backend",
				ArchMultitier)
		}
		if c.System.Frontend.IsEmpty() {
			return fmt.Errorf("config: system.architecture=%s requires system.frontend",
				ArchMultitier)
		}
	}

	// Rule 6: tier completeness — within any non-empty TierSpec, all three
	// fields must be set. (Architecture exclusivity above already enforces
	// this for the system-tier shape; this rule covers system-test, backend,
	// frontend, plus external-systems specs.)
	if err := requireFullTier("system-test", c.SystemTest); err != nil {
		return err
	}
	if err := requireFullTier("system.backend", c.System.Backend); err != nil {
		return err
	}
	if err := requireFullTier("system.frontend", c.System.Frontend); err != nil {
		return err
	}
	if err := requireFullExternal("external-systems.stubs", c.ExternalSystems.Stubs); err != nil {
		return err
	}
	if err := requireFullExternal("external-systems.simulators", c.ExternalSystems.Simulators); err != nil {
		return err
	}

	// Rule 7: system-test presence when architecture is set.
	if c.System.Architecture != "" && c.SystemTest.IsEmpty() {
		return fmt.Errorf("config: system.architecture is set; system-test.{path,repo,lang} are required")
	}

	// Rule 8: repo-strategy consistency.
	if c.RepoStrategy != "" {
		repos := c.Repos()
		switch c.RepoStrategy {
		case RepoStrategyMonoRepo:
			if len(repos) > 1 {
				return fmt.Errorf("config: repo-strategy=%s but tiers point at multiple repos %v",
					RepoStrategyMonoRepo, repos)
			}
			if c.System.Architecture != "" && len(repos) != 1 {
				return fmt.Errorf("config: repo-strategy=%s with system.architecture set requires exactly one repo across all tiers (got %d)",
					RepoStrategyMonoRepo, len(repos))
			}
		case RepoStrategyMultiRepo:
			if c.System.Architecture != "" && len(repos) == 0 {
				return fmt.Errorf("config: repo-strategy=%s with system.architecture set requires at least one tier with a non-empty repo:",
					RepoStrategyMultiRepo)
			}
		}
	}

	// Rule 9 (formerly project.url required): empty project.url is now
	// accepted at validate-time. `gh optivem init` Path A auto-creates a
	// board and persists the URL on its first run; the ATDD runtime
	// re-checks presence at board-resolution time, so an empty value here
	// is not silently absorbed downstream.

	// Rule 10: process-flow path validity (when set).
	if err := validatePath("process-flow", c.ProcessFlow); err != nil {
		return err
	}

	// Rule 11: task-prompts. Keys must be known embedded MID task names
	// (typos surface at config-load, not deep inside the pipeline); values
	// pass validatePath. Sorted iteration so errors are deterministic.
	// Post-1701 split: enumerate task-names from the embedded process-flow
	// YAML rather than agents.Names() (which now returns agent nouns).
	if len(c.TaskPrompts) > 0 {
		known, err := knownTaskNames()
		if err != nil {
			return fmt.Errorf("config: task-prompts: enumerate MID task names: %w", err)
		}
		for _, name := range sortedKeys(c.TaskPrompts) {
			if !known[name] {
				return fmt.Errorf("config: task-prompts: %q is not a known embedded MID task", name)
			}
			if err := validatePath("task-prompts."+name, c.TaskPrompts[name]); err != nil {
				return err
			}
		}
	}

	// Rule 12: node-replacements paths pass validatePath. Sorted
	// iteration for deterministic errors.
	for _, node := range sortedKeys(c.NodeReplacements) {
		if err := validatePath("node-replacements."+node, c.NodeReplacements[node]); err != nil {
			return err
		}
	}

	// Rule 13: a node ID may not appear in both node-extras and
	// node-replacements — replace strictly supersedes extras, so the
	// combination signals operator confusion. Node-ID validity against
	// the resolved process flow is deferred to the driver (the process
	// flow is itself configurable, so the node-ID set isn't known here).
	for _, node := range sortedKeys(c.NodeExtras) {
		if _, dup := c.NodeReplacements[node]; dup {
			return fmt.Errorf("config: node %q appears in both node-extras and node-replacements (replace supersedes extras)", node)
		}
	}

	// Rule 14: system-name format. Empty is accepted at the schema layer
	// so partial configs (project URL + repo-strategy only, no system
	// scope yet) keep working; `gh optivem init` re-checks presence at
	// invocation time. When set, the value must match the full naming
	// rule used by the templating pipeline.
	if c.SystemName != "" {
		if msg := ValidateSystemName(c.SystemName); msg != "" {
			return fmt.Errorf("config: system-name: %s", msg)
		}
	}

	// Rule 15: license enum. Empty is accepted (init layers a default
	// when reading); a non-empty value must be a known key so the
	// scaffolded LICENSE file isn't silently wrong.
	if c.License != "" && !IsValidLicense(c.License) {
		return fmt.Errorf("config: license %q must be one of mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense", c.License)
	}

	// Rule 16: deploy enum. Empty is accepted (init layers `docker` as
	// the default); a non-empty value must be `docker` or `cloud-run`.
	if c.Deploy != "" && !IsValidDeploy(c.Deploy) {
		return fmt.Errorf("config: deploy %q must be one of %q, %q",
			c.Deploy, DeployDocker, DeployCloudRun)
	}

	// Rules 17/18: SonarCloud presence. The block is optional when no
	// architecture is set (a partial config — project URL + repo-strategy
	// only — has no Sonar identities to express). Once architecture is
	// set, gh-optivem.yaml must carry the org + per-code-tier project
	// keys so runtime consumers (run-sonar, finalize) can resolve them
	// from the YAML without re-deriving. The YAML is the source of truth
	// for these keys — `gh optivem init` seeds them via
	// DeriveSonarProjects but the values may be hand-edited afterwards
	// (e.g. multi-stack reference repos that need per-variant SonarCloud
	// projects the single-stack deriver cannot express).
	if c.System.Architecture != "" {
		if err := c.validateSonar(); err != nil {
			return err
		}
	}

	// Rule 19: project.provider is mandatory and must name a known
	// tracker backend. The empty case names the migrate command so an
	// operator with a pre-provider config has a one-shot fix path.
	switch c.Project.Provider {
	case ProviderGitHub, ProviderMarkdown:
		// ok
	case "":
		return fmt.Errorf("config: project.provider is required (one of %q, %q); run `gh optivem config migrate` to add it from an existing project.url",
			ProviderGitHub, ProviderMarkdown)
	default:
		return fmt.Errorf("config: project.provider %q must be one of %q, %q",
			c.Project.Provider, ProviderGitHub, ProviderMarkdown)
	}

	// Rule 20: provider/url shape consistency. A github provider paired
	// with a non-github URL (or a markdown provider paired with an
	// HTTP URL) is operator confusion — name both fields so the fix is
	// unambiguous. Empty URL is accepted (init Path A populates it on
	// first run; runtime re-checks at board-resolution time).
	if c.Project.URL != "" {
		switch c.Project.Provider {
		case ProviderGitHub:
			if !strings.HasPrefix(c.Project.URL, "https://github.com/") {
				return fmt.Errorf("config: project.provider=%q but project.url=%q is not a github URL",
					c.Project.Provider, c.Project.URL)
			}
		case ProviderMarkdown:
			if strings.HasPrefix(c.Project.URL, "http://") || strings.HasPrefix(c.Project.URL, "https://") {
				return fmt.Errorf("config: project.provider=%q but project.url=%q is an HTTP URL, not a directory path",
					c.Project.Provider, c.Project.URL)
			}
		}
	}

	// Rule 21: repos[] path shape. Each entry's path must be set and
	// repo-relative — validateRepoPath rejects absolute paths but
	// (unlike validatePath, which polices intra-repo tier paths)
	// permits `..` segments because the sibling-folder layout is the
	// normal multi-repo convention (`../page-turner-backend` reaches a
	// sibling clone of the gh-optivem.yaml directory). Empty repos: is
	// accepted — the scope cascade falls back to the cwd repo.
	// Duplicate paths (after filepath.Clean) are rejected so the
	// cascade can't iterate the same folder twice.
	seenRepoPaths := map[string]struct{}{}
	for i, r := range c.LocalRepos {
		if r.Path == "" {
			return fmt.Errorf("config: repos[%d].path is required", i)
		}
		if err := validateRepoPath(fmt.Sprintf("repos[%d].path", i), r.Path); err != nil {
			return err
		}
		normalized := filepath.ToSlash(filepath.Clean(r.Path))
		if _, dup := seenRepoPaths[normalized]; dup {
			return fmt.Errorf("config: repos[%d].path %q appears more than once", i, r.Path)
		}
		seenRepoPaths[normalized] = struct{}{}
	}

	// Rule 22a: when system.architecture is set, every canonical Family B
	// key must be present in paths: with a non-empty value. The contract
	// is "explicit only — no defaults anywhere": the scaffolder writes
	// these at `gh optivem init` and the operator owns subsequent edits;
	// runtime never falls back to derived values. A missing key here
	// would otherwise reach the ATDD dispatcher as an unfilled
	// ${system-driver-adapter} (etc.) placeholder, failing deep inside a
	// per-ticket agent dispatch instead of at config load.
	//
	// Gating on architecture matches the scaffolder/migrate: partial
	// configs (project URL + repo-strategy only, no system scope yet)
	// legitimately have no paths: block.
	if c.System.Architecture != "" {
		var missing []string
		for _, k := range CanonicalPathKeys() {
			if c.SystemTest.Paths[k] == "" {
				missing = append(missing, "system-test.paths."+k)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("config: system.architecture is set; %s required (canonical Family B keys must be explicitly configured — see internal/projectconfig/path-keys.md for the supported set)",
				strings.Join(missing, ", "))
		}
	}

	// Rule 22b: when system.architecture is set, system.db-migration-path
	// must be set. The migration set is shared infrastructure consumed by
	// every SUT (3 langs × 2 archs); the gh-optivem-owned name +
	// operator-owned value contract treats it as Family A path-shaped key
	// system-db-migration-path. `gh optivem init` writes the
	// shop-template default (`system/db/migrations`); `gh optivem config
	// migrate` back-fills the same default exactly once for pre-this-plan
	// configs. After that, operators own the value.
	if c.System.Architecture != "" && c.System.DbMigrationPath == "" {
		return fmt.Errorf("config: system.architecture is set; system.db-migration-path is required (run `gh optivem config migrate` to back-fill the default %q for a pre-this-plan config)",
			DefaultDbMigrationPath)
	}

	// Rule 22: paths.* keys must (a) not shadow reserved Family A
	// placeholder names, (b) be in the canonical Family B key set
	// (CanonicalPathKeys — eight names today), and (c) carry fully-
	// resolved values with no `${...}` substitution markers. (a) is the
	// historical wires-crossed guard (a typo'd `paths.language:
	// typescript` would otherwise quietly override system.lang). (b) is
	// the SSoT typo / stale-key catch (plan 20260518-1530 item 5) — see
	// internal/projectconfig/path-keys.md for the
	// canonical-key vocabulary. (c) is the SSoT no-substitution rule —
	// values must be fully resolved at scaffold time (plan item 5);
	// runtime substitution is retired (locked δ).
	canonical := map[string]struct{}{}
	for _, k := range CanonicalPathKeys() {
		canonical[k] = struct{}{}
	}
	// Sorted iteration so errors are deterministic when multiple keys
	// could trip the rule.
	for _, k := range sortedKeys(c.SystemTest.Paths) {
		if _, reserved := reservedPlaceholderKeys[k]; reserved {
			return fmt.Errorf("config: system-test.paths.%s shadows a reserved fixed-schema placeholder name; rename it", k)
		}
		if _, ok := canonical[k]; !ok {
			return fmt.Errorf("config: system-test.paths.%s is not a canonical Family B key; see internal/projectconfig/path-keys.md for the supported set", k)
		}
		if strings.Contains(c.SystemTest.Paths[k], "${") {
			return fmt.Errorf("config: system-test.paths.%s %q contains a ${...} marker; under SSoT, paths must be fully resolved (substitution is scaffold-time-only)", k, c.SystemTest.Paths[k])
		}
		if err := validatePath("system-test.paths."+k, c.SystemTest.Paths[k]); err != nil {
			return err
		}
	}

	// Rule 22c: paths: is system-test-only today. Reject non-empty Paths on
	// backend/frontend tiers — they have no canonical Family B vocabulary
	// to anchor against, and silently accepting the field would let a typo
	// like `system.backend.paths:` parse as a no-op. Same shape as Rule 0b
	// for TierSpec.Config (also system-test-only).
	if len(c.System.Backend.Paths) > 0 {
		return fmt.Errorf("config: system.backend.paths is not a supported field (paths: is system-test-only; use system-test.paths)")
	}
	if len(c.System.Frontend.Paths) > 0 {
		return fmt.Errorf("config: system.frontend.paths is not a supported field (paths: is system-test-only; use system-test.paths)")
	}

	// Rule 23: channels: tokens are lowercase canonical members of the
	// supported channel enum (single canon, no fold layer — see
	// channels.go). Independent of architecture: the field is optional, but
	// any value present must be well-formed.
	if err := validateChannels(c.Channels); err != nil {
		return err
	}

	return nil
}

// validateSonar enforces Rules 17/18. Split out to keep Validate's body
// linear and to give the Sonar block one cohesive failure point.
func (c *Config) validateSonar() error {
	// Rule 17: sonar.organization required when architecture is set.
	if c.Sonar.Organization == "" {
		return fmt.Errorf("config: system.architecture is set; sonar.organization is required")
	}

	// Rule 18: per-code-tier sonar-project required.
	switch c.System.Architecture {
	case ArchMonolith:
		if c.System.SonarProject == "" {
			return fmt.Errorf("config: system.architecture=%s requires system.sonar-project",
				ArchMonolith)
		}
		if c.System.Backend.SonarProject != "" {
			return fmt.Errorf("config: system.architecture=%s incompatible with system.backend.sonar-project (multitier-only)",
				ArchMonolith)
		}
		if c.System.Frontend.SonarProject != "" {
			return fmt.Errorf("config: system.architecture=%s incompatible with system.frontend.sonar-project (multitier-only)",
				ArchMonolith)
		}
	case ArchMultitier:
		if c.System.Backend.SonarProject == "" {
			return fmt.Errorf("config: system.architecture=%s requires system.backend.sonar-project",
				ArchMultitier)
		}
		if c.System.Frontend.SonarProject == "" {
			return fmt.Errorf("config: system.architecture=%s requires system.frontend.sonar-project",
				ArchMultitier)
		}
		if c.System.SonarProject != "" {
			return fmt.Errorf("config: system.architecture=%s incompatible with system.sonar-project (monolith-only)",
				ArchMultitier)
		}
	}
	if c.SystemTest.SonarProject == "" {
		return fmt.Errorf("config: system.architecture is set; system-test.sonar-project is required")
	}
	return nil
}

// knownTaskNames returns the set of MID task-name verbs declared on every
// writing-agent EXECUTE_AGENT call-activity in the embedded process-flow
// YAML. The validator for `task-prompts:` uses this to reject typo'd keys
// at config-load. Post-plan-1701 split, task-names (verbs) and agent names
// (nouns) diverged — the schema field is keyed by task-name, so this is
// the right source. Templated task-names (e.g. "fix-${failure-kind}" on
// the `fix` LOW process) are skipped: they resolve at runtime to a
// concrete MID that already appears in this set via its own entry.
func knownTaskNames() (map[string]bool, error) {
	eng, err := statemachine.LoadDefault()
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			name := node.Raw.Params["task-name"]
			if name == "" || strings.Contains(name, "${") {
				continue
			}
			out[name] = true
		}
	}
	return out, nil
}

// sortedKeys returns the keys of m in lexicographic order. Used by
// Validate to produce deterministic error messages when multiple entries
// could trip the same rule.
func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// IsEmpty reports whether t has no tier identity set (Path/Repo/Lang all
// empty). Config alone does not make a tier — `system-test: { config: x }`
// is still IsEmpty so the architecture-presence rule fires the same way
// it always did.
func (t TierSpec) IsEmpty() bool {
	return t.Path == "" && t.Repo == "" && t.Lang == ""
}

// IsEmpty reports whether e is the zero-value ExternalSpec.
func (e ExternalSpec) IsEmpty() bool {
	return e.Path == "" && e.Repo == ""
}

// requireFullTier enforces that any TierSpec with at least one field set
// has all three fields set.
func requireFullTier(field string, t TierSpec) error {
	if t.IsEmpty() {
		return nil
	}
	if t.Path == "" || t.Repo == "" || t.Lang == "" {
		return fmt.Errorf("config: %s requires path, repo, and lang all set when any is set",
			field)
	}
	return nil
}

// requireFullExternal enforces that any ExternalSpec with at least one
// field set has both fields set.
func requireFullExternal(field string, e ExternalSpec) error {
	if e.IsEmpty() {
		return nil
	}
	if e.Path == "" || e.Repo == "" {
		return fmt.Errorf("config: %s requires path and repo both set when either is set",
			field)
	}
	return nil
}

// validateLang accepts empty strings and the three enum members. Any other
// value (e.g. "react" — a framework, not a language; "rust" — unsupported)
// is rejected.
func validateLang(field, value string) error {
	switch value {
	case "", LangJava, LangDotnet, LangTypescript:
		return nil
	default:
		return fmt.Errorf("config: %s %q must be one of %q, %q, %q",
			field, value, LangJava, LangDotnet, LangTypescript)
	}
}

// validatePath rejects absolute paths and paths containing `..` segments.
// Empty values are accepted (the per-tier presence rules in Validate cover
// the "must be set" cases). FS existence is not checked here — that lives
// in the runtime preflight.
//
// Absolute-path detection is cross-platform: filepath.IsAbs uses host
// rules (so on Windows "/abs/path" is not absolute, on Linux "C:\foo" is
// not absolute), but `gh-optivem.yaml` is committed across platforms — a
// path that's relative on the developer's machine but absolute on the
// CI runner is still wrong. The explicit `/` / `\` / drive-letter checks
// catch every form regardless of host.
func validatePath(field, value string) error {
	if value == "" {
		return nil
	}
	if filepath.IsAbs(value) ||
		strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "\\") ||
		looksLikeWindowsDriveAbsolute(value) {
		return fmt.Errorf("config: %s %q must be repo-relative, not absolute",
			field, value)
	}
	// Scan the raw value for `..` segments — do NOT use filepath.Clean
	// here, since Clean collapses `foo/../bar` to `bar` and would let
	// the escape attempt through.
	posix := filepath.ToSlash(value)
	for _, seg := range strings.Split(posix, "/") {
		if seg == ".." {
			return fmt.Errorf("config: %s %q must not contain a .. segment",
				field, value)
		}
	}
	return nil
}

// looksLikeWindowsDriveAbsolute reports whether v starts with a drive
// letter and colon (e.g. "C:foo" or "C:\foo"). filepath.IsAbs handles
// this on Windows but not on Linux/macOS, so we add an explicit cross-
// platform check.
func looksLikeWindowsDriveAbsolute(v string) bool {
	return len(v) >= 2 && v[1] == ':' &&
		((v[0] >= 'a' && v[0] <= 'z') || (v[0] >= 'A' && v[0] <= 'Z'))
}

// validateRepoPath enforces the path shape for entries in repos[]. Like
// validatePath it rejects absolute paths (host-rule plus explicit
// leading-slash and Windows-drive checks) so a path that's relative on
// one OS but absolute on another never slips into a committed file.
// Unlike validatePath it permits `..` segments — repos[] declares the
// project's own clones, and the sibling-folder layout used by
// multi-repo projects (`../page-turner-backend`) is a legitimate
// expression of that. FS existence is not checked here; the workspace
// cascade silently filters non-existent / non-git entries at runtime.
func validateRepoPath(field, value string) error {
	if value == "" {
		return nil
	}
	if filepath.IsAbs(value) ||
		strings.HasPrefix(value, "/") ||
		strings.HasPrefix(value, "\\") ||
		looksLikeWindowsDriveAbsolute(value) {
		return fmt.Errorf("config: %s %q must be repo-relative, not absolute",
			field, value)
	}
	return nil
}

// ResolvePath produces the gh-optivem.yaml path every command should
// read, applying flag > env > default precedence:
//
//   - flagVal non-empty (operator passed --config / -c): that path, explicit=true.
//   - $GH_OPTIVEM_CONFIG set: that path, explicit=true.
//   - otherwise: <cwd>/gh-optivem.yaml, explicit=false.
//
// The explicit return tells callers whether the path was operator-chosen
// (a typo'd path should hard-error, not silently fall back) vs the
// default (where future UX work can offer to scaffold the file in-place).
func ResolvePath(flagVal string) (path string, explicit bool) {
	if flagVal != "" {
		return flagVal, true
	}
	if v := os.Getenv(EnvVar); v != "" {
		return v, true
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, Path), false
}

// Load reads <repoPath>/gh-optivem.yaml and returns the parsed Config. A
// missing file returns (nil, nil) — callers treat absence as "no config,
// fall through". I/O errors other than not-found are surfaced. YAML parse
// errors and validation errors are surfaced.
func Load(repoPath string) (*Config, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("config: repoPath is required")
	}
	full := filepath.Join(repoPath, Path)
	data, err := os.ReadFile(full)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", full, err)
	}
	return parse(data, full)
}

// LoadFromPath reads and parses a config file at the given absolute or
// relative path. Unlike Load, missing-file is an error — when the operator
// passes an explicit path (e.g. via `--config`), they expect the file to
// exist; silently falling through to defaults would mask a typo.
func LoadFromPath(path string) (*Config, error) {
	if path == "" {
		return nil, fmt.Errorf("config: path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return parse(data, path)
}

func parse(data []byte, source string) (*Config, error) {
	var cfg Config
	if len(bytes.TrimSpace(data)) > 0 {
		dec := yaml.NewDecoder(bytes.NewReader(data))
		// KnownFields(true) rejects any field not declared on the target
		// struct at every nesting level — uniform strictness so a typo like
		// `system.namepsace` errors with the same hard-fail as a root typo
		// (per plan 20260518-1530 item 5). Skipped for empty/whitespace-only
		// input because the decoder returns io.EOF in that case; Validate
		// produces the canonical "project.provider is required" error
		// against the zero-value Config below.
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", source, err)
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", source, err)
	}
	return &cfg, nil
}

// Write marshals cfg to <repoPath>/gh-optivem.yaml (0644). Validates first so a
// caller can't accidentally persist a config that Load would reject — the
// round-trip Write→Load must always succeed for the same value.
func Write(repoPath string, cfg *Config) error {
	if repoPath == "" {
		return fmt.Errorf("config: repoPath is required")
	}
	return WriteToPath(filepath.Join(repoPath, Path), cfg)
}

// WriteToPath marshals cfg to the given exact file path (0644), allowing
// callers to choose a non-canonical filename (e.g. `gh-optivem.shop.yaml`)
// when --config points at one. Validates first.
func WriteToPath(yamlPath string, cfg *Config) error {
	if yamlPath == "" {
		return fmt.Errorf("config: yamlPath is required")
	}
	data, err := Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(yamlPath, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", yamlPath, err)
	}
	return nil
}

// Marshal validates cfg and returns its YAML byte representation. Exposed
// so callers that need to layer additional bytes onto the YAML (e.g.
// prepend a banner comment in the interactive `config init` recovery
// flow) don't have to round-trip through a file. Mirrors WriteToPath's
// validate-then-marshal contract — what comes back is guaranteed to be
// the same bytes WriteToPath would have written.
func Marshal(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config: cfg is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("config: marshal: %w", err)
	}
	return data, nil
}
