// Package projectconfig loads the consumer repo's per-project configuration
// file at optivem.yaml (project root). The file holds project-level facts
// — board URL, repo strategy, and the scope axes the ATDD pipeline runs
// against — that legitimately differ between consumer repos but stay
// stable across pipeline invocations within one repo.
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
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Path is the canonical relative location of the file inside a consumer
// repo (project root). Read from the consumer's CWD — gh-optivem is
// repo-agnostic by design.
const Path = "optivem.yaml"

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

// Language enum values (shared by SystemLang and TestLang).
const (
	LangJava       = "java"
	LangDotnet     = "dotnet"
	LangTypescript = "typescript"
)

// Config mirrors the YAML schema. Nested groups give room to add knobs
// without flattening the namespace.
type Config struct {
	Project Project `yaml:"project"`
	Scope   Scope   `yaml:"scope"`
}

// Project holds project-board configuration and repo-layout facts.
//
// URL is the canonical GitHub Project URL (org or user variant). When
// unset, board.ResolveProjectURL fails — there is no README scrape or
// `gh project list` fallback. Operators must configure project.url
// explicitly (or pass the alternate file via --config).
//
// Name is the human-readable project title (e.g. "Shop Project"). When set,
// the driver displays it in the "Resolved issue" line without making an
// extra `gh project view` round-trip just for the title.
//
// RepoStrategy declares whether the system lives in one repo (mono-repo)
// or is split across several (multi-repo). Repos lists the participating
// repos when RepoStrategy is multi-repo; for mono-repo the field is
// optional and defaults to the implicit-self repo.
type Project struct {
	URL          string   `yaml:"url,omitempty"`
	Name         string   `yaml:"name,omitempty"`
	RepoStrategy string   `yaml:"repo_strategy,omitempty"`
	Repos        []string `yaml:"repos,omitempty"`
}

// Scope declares which architecture and which languages the pipeline
// runs against for this repo. Each axis takes a single value; the runtime
// substitutes them into agent prompts as ${architecture}, ${system_lang},
// and ${test_lang}. Empty values are accepted (the corresponding template
// var expands to the empty string); invalid non-empty values fail
// validation at load time.
type Scope struct {
	Architecture string `yaml:"architecture,omitempty"`
	SystemLang   string `yaml:"system_lang,omitempty"`
	TestLang     string `yaml:"test_lang,omitempty"`
}

// Validate enforces the enum and cross-field rules. Empty values are
// accepted — only non-empty invalid values are errors. Cross-field rules:
//
//   - RepoStrategy=multi-repo with empty Repos is an error (the list is
//     required to disambiguate which repos participate).
//   - RepoStrategy=mono-repo with len(Repos) > 1 is an error (contradictory:
//     "one repo" plus several listed almost certainly means a typo, and
//     silently ignoring extras would lead to "why is the pipeline only
//     touching one repo?" debugging).
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	switch c.Project.RepoStrategy {
	case "", RepoStrategyMonoRepo, RepoStrategyMultiRepo:
	default:
		return fmt.Errorf("config: project.repo_strategy %q must be one of %q, %q",
			c.Project.RepoStrategy, RepoStrategyMonoRepo, RepoStrategyMultiRepo)
	}
	if c.Project.RepoStrategy == RepoStrategyMultiRepo && len(c.Project.Repos) == 0 {
		return fmt.Errorf("config: project.repo_strategy=%s requires a non-empty project.repos list",
			RepoStrategyMultiRepo)
	}
	if c.Project.RepoStrategy == RepoStrategyMonoRepo && len(c.Project.Repos) > 1 {
		return fmt.Errorf("config: project.repo_strategy=%s is incompatible with multiple entries in project.repos (got %d)",
			RepoStrategyMonoRepo, len(c.Project.Repos))
	}

	switch c.Scope.Architecture {
	case "", ArchMonolith, ArchMultitier:
	default:
		return fmt.Errorf("config: scope.architecture %q must be one of %q, %q",
			c.Scope.Architecture, ArchMonolith, ArchMultitier)
	}
	if err := validateLang("scope.system_lang", c.Scope.SystemLang); err != nil {
		return err
	}
	if err := validateLang("scope.test_lang", c.Scope.TestLang); err != nil {
		return err
	}
	return nil
}

func validateLang(field, value string) error {
	switch value {
	case "", LangJava, LangDotnet, LangTypescript:
		return nil
	default:
		return fmt.Errorf("config: %s %q must be one of %q, %q, %q",
			field, value, LangJava, LangDotnet, LangTypescript)
	}
}

// Load reads <repoPath>/optivem.yaml and returns the parsed Config. A
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
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", source, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", source, err)
	}
	return &cfg, nil
}

// Write marshals cfg to <repoPath>/optivem.yaml (0644). Validates first so a
// caller can't accidentally persist a config that Load would reject — the
// round-trip Write→Load must always succeed for the same value.
func Write(repoPath string, cfg *Config) error {
	if repoPath == "" {
		return fmt.Errorf("config: repoPath is required")
	}
	if cfg == nil {
		return fmt.Errorf("config: cfg is required")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	full := filepath.Join(repoPath, Path)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", full, err)
	}
	return nil
}
