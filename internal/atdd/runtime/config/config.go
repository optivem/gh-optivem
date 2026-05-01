// Package config loads the consumer repo's ATDD configuration file at
// docs/atdd/config.yaml. The file is optional; absence is not an error.
// Callers (currently only board.ResolveProjectURL) consult it before
// falling back to README scraping or git-remote-based project discovery.
//
// The schema is intentionally small. New top-level groups can be added
// without breaking existing repos — each section is independently optional
// and unmarshalled into its own struct.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Path is the canonical relative location inside a consumer repo. Read
// from the consumer's CWD — gh-optivem is repo-agnostic by design.
const Path = "docs/atdd/config.yaml"

// Config mirrors the YAML schema. Nested groups give room to add knobs
// without flattening the namespace.
type Config struct {
	Project Project `yaml:"project"`
}

// Project holds project-board configuration.
//
// URL is the canonical GitHub Project URL (org or user variant). When set,
// board.ResolveProjectURL returns it directly without scanning README or
// listing org projects.
//
// Name is the human-readable project title (e.g. "Shop Project"). When set,
// the driver displays it in the "Resolved issue" line without making an
// extra `gh project view` round-trip just for the title.
type Project struct {
	URL  string `yaml:"url"`
	Name string `yaml:"name"`
}

// Load reads <repoPath>/docs/atdd/config.yaml and returns the parsed
// Config. A missing file returns (nil, nil) — callers treat absence as
// "no config, fall through". I/O errors other than not-found are
// surfaced. YAML parse errors are surfaced.
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
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", full, err)
	}
	return &cfg, nil
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
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return &cfg, nil
}
