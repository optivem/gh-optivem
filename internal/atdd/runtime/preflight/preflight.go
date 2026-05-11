// Package preflight validates that the consumer's gh-optivem.yaml maps
// onto a real on-disk layout before any ATDD agent or board work runs.
// It is the runtime backstop for "did the operator's directories actually
// match the schema?" — the answer must be yes before implement-ticket
// dispatches anything.
//
// Two classes of check:
//
//  1. Repo-level. Every slug in cfg.Repos() must resolve (via repolocator)
//     to a path that exists, is a directory, and contains a `.git` entry.
//  2. Tier-level. Every populated tier (system or backend+frontend, plus
//     system_test, plus declared external_systems) must have its `path`
//     join cleanly with its host repo's local clone.
//
// Failures are aggregated — Preflight does not return on the first error.
// The operator gets one multi-line error block listing every missing item.
package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/repolocator"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Run validates that cfg's declared layout exists on disk. workspace is
// the operator-supplied workspace root (from the --workspace flag);
// when empty, the formula defaults to filepath.Dir(cwd). cwd is the
// working directory used by the default branch; when empty, the process
// CWD is used.
//
// Returns nil when everything checks out. Otherwise returns a single
// error whose Error() lists every failure on its own line, prefixed with
// "  - ". Callers (the implement-ticket cobra command) print it directly
// to stderr and exit non-zero.
func Run(cfg *projectconfig.Config, workspace string, cwd string) error {
	if cfg == nil {
		return nil
	}
	res, err := repolocator.Resolve(cfg, workspace, cwd)
	if err != nil {
		return fmt.Errorf("preflight: resolve repos: %w", err)
	}

	var failures []string

	// Repo-level checks.
	slugs := cfg.Repos()
	for _, slug := range slugs {
		path, ok := res.Local[slug]
		if !ok || path == "" {
			failures = append(failures, fmt.Sprintf("repo %s: no local path resolved", slug))
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Sprintf("repo %s: local clone %s does not exist", slug, path))
			} else {
				failures = append(failures, fmt.Sprintf("repo %s: stat %s: %v", slug, path, err))
			}
			continue
		}
		if !info.IsDir() {
			failures = append(failures, fmt.Sprintf("repo %s: %s is not a directory", slug, path))
			continue
		}
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			failures = append(failures, fmt.Sprintf("repo %s: %s is not a git repository (no .git entry)", slug, path))
		}
	}

	// Tier-level checks. Each tier has a (field-name, repo-slug, path)
	// triple; we look up the host repo's local clone and join the
	// tier's path onto it.
	tiers := collectTiers(cfg)
	for _, tier := range tiers {
		hostPath, ok := res.Local[tier.repo]
		if !ok || hostPath == "" {
			// Already reported above as a repo-level failure; skip
			// the tier-level check to avoid double-counting.
			continue
		}
		full := filepath.Join(hostPath, tier.path)
		info, err := os.Stat(full)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				failures = append(failures, fmt.Sprintf("%s: %s does not exist", tier.field, full))
			} else {
				failures = append(failures, fmt.Sprintf("%s: stat %s: %v", tier.field, full, err))
			}
			continue
		}
		if !info.IsDir() {
			failures = append(failures, fmt.Sprintf("%s: %s is not a directory", tier.field, full))
		}
	}

	if len(failures) == 0 {
		return nil
	}
	sort.Strings(failures)
	return fmt.Errorf("preflight failed:\n  - %s", strings.Join(failures, "\n  - "))
}

// tierCheck packages one populated tier for the per-tier loop. Field is
// the YAML path used in error messages (e.g. "system.backend.path");
// repo is the slug whose local clone hosts the tier; path is the
// repo-relative directory.
type tierCheck struct {
	field string
	repo  string
	path  string
}

// collectTiers returns every populated tier in cfg, in a deterministic
// order suitable for stable error output.
func collectTiers(cfg *projectconfig.Config) []tierCheck {
	var out []tierCheck
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		if cfg.System.Path != "" || cfg.System.Repo != "" {
			out = append(out, tierCheck{
				field: "system.path",
				repo:  cfg.System.Repo,
				path:  cfg.System.Path,
			})
		}
	case projectconfig.ArchMultitier:
		if !cfg.System.Backend.IsEmpty() {
			out = append(out, tierCheck{
				field: "system.backend.path",
				repo:  cfg.System.Backend.Repo,
				path:  cfg.System.Backend.Path,
			})
		}
		if !cfg.System.Frontend.IsEmpty() {
			out = append(out, tierCheck{
				field: "system.frontend.path",
				repo:  cfg.System.Frontend.Repo,
				path:  cfg.System.Frontend.Path,
			})
		}
	}
	if !cfg.SystemTest.IsEmpty() {
		out = append(out, tierCheck{
			field: "system_test.path",
			repo:  cfg.SystemTest.Repo,
			path:  cfg.SystemTest.Path,
		})
	}
	// External systems — stubs first (cycle 2), simulators second (cycle 3).
	if !cfg.ExternalSystems.Stubs.IsEmpty() {
		out = append(out, tierCheck{
			field: "external_systems.stubs.path",
			repo:  cfg.ExternalSystems.Stubs.Repo,
			path:  cfg.ExternalSystems.Stubs.Path,
		})
	}
	if !cfg.ExternalSystems.Simulators.IsEmpty() {
		out = append(out, tierCheck{
			field: "external_systems.simulators.path",
			repo:  cfg.ExternalSystems.Simulators.Repo,
			path:  cfg.ExternalSystems.Simulators.Path,
		})
	}
	return out
}
