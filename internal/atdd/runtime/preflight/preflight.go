// Package preflight validates that the consumer's gh-optivem.yaml maps
// onto a real on-disk layout and a real remote setup before any ATDD
// agent or board work runs. It is the runtime backstop for "did the
// operator's directories actually match the schema, and do the services
// the schema names actually exist?" — the answer must be yes before
// implement-ticket dispatches anything.
//
// Three classes of check:
//
//  1. Repo-level local. Every slug in cfg.Repos() must resolve (via
//     repolocator) to a path that exists, is a directory, and contains
//     a `.git` entry.
//  2. Tier-level local. Every populated tier (system or backend+frontend,
//     plus system-test, plus declared external-systems) must have its
//     `path` join cleanly with its host repo's local clone.
//  3. Remote (optional). When the corresponding Options field is non-nil,
//     also verify that every repo slug exists on GitHub, every declared
//     sonar org + project key exists on SonarCloud, and the project board
//     URL resolves. nil fields mean "skip that class" — tests inject
//     fakes; production wires real clients in the cobra layer.
//
// Failures are aggregated — preflight does not return on the first error.
// The operator gets one multi-line error block listing every missing item.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/repolocator"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Options bundles the optional inputs to Run. All four remote-check
// fields default to nil = skip; production wires them via the cobra
// layer (see runImplementTicketPreflight, runConfigPreflight). Tests
// inject fakes per scenario without dragging in real network clients.
//
// Function-typed fields beat single-method interfaces here because each
// remote check is exactly one call — wrapping it in an interface adds
// boilerplate without buying polymorphism.
type Options struct {
	// Workspace is the operator-supplied workspace root (from the
	// --workspace flag). When empty, repolocator defaults to
	// filepath.Dir(cwd).
	Workspace string

	// Cwd is the working directory used by the default branch of the
	// resolver. When empty, the process CWD is used.
	Cwd string

	// RepoExists reports whether a GitHub repo slug (owner/name) is
	// visible to the authenticated gh CLI. nil = skip the repo-existence
	// remote check.
	RepoExists func(ctx context.Context, slug string) (bool, error)

	// SonarOrgExists reports whether a SonarCloud organization with the
	// given key exists. nil = skip the org check (and per-tier project
	// checks; without an org to anchor them they have no remote contract
	// to verify).
	SonarOrgExists func(ctx context.Context, key string) (bool, error)

	// SonarProjectExists reports whether a SonarCloud project with the
	// given key exists. nil = skip the project-existence remote check.
	SonarProjectExists func(ctx context.Context, key string) (bool, error)

	// BoardURLOK verifies that cfg.Project.URL resolves and is visible
	// to the authenticated gh CLI. nil = skip the board-URL check.
	BoardURLOK func(ctx context.Context, projectURL string) error

	// ClaudeCheck verifies the `claude` CLI is on PATH and runnable.
	// nil = skip (used by the v1 --manual-agents fallback that doesn't
	// need the CLI, and by config-flow callers that don't dispatch
	// agents). Production callers point at preflight.VerifyClaude;
	// tests inject canned-result stubs. Same nil=skip convention as
	// the remote-check fields above so a failed claude check folds
	// into the aggregated error block alongside any missing repos or
	// tier directories. Runs even on a nil cfg — claude readiness is
	// independent of project layout.
	ClaudeCheck func(ctx context.Context) error
}

// Run validates cfg's declared layout (local FS) and optionally its
// remote setup (GitHub repos, SonarCloud org + projects, project board
// URL). Returns nil when everything checks out. Otherwise returns a
// single error whose Error() lists every failure on its own line,
// prefixed with "  - ". Callers print it directly to stderr and exit
// non-zero.
//
// A nil cfg skips the structural checks (nothing to compare against)
// but still runs Options.ClaudeCheck when set, since claude readiness is
// independent of project layout.
func Run(ctx context.Context, cfg *projectconfig.Config, opts Options) error {
	var failures []string

	// Local-tool check: claude CLI presence. Runs before any cfg-dependent
	// work so its failure surfaces even on a nil cfg, and folds into the
	// same aggregated error block as the structural failures below.
	if opts.ClaudeCheck != nil {
		if err := opts.ClaudeCheck(ctx); err != nil {
			failures = append(failures, err.Error())
		}
	}

	if cfg == nil {
		return aggregateFailures(failures)
	}
	res, err := repolocator.Resolve(cfg, opts.Workspace, opts.Cwd)
	if err != nil {
		return fmt.Errorf("preflight: resolve repos: %w", err)
	}

	// Repo-level local checks.
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

	// Tier-level local checks. Each tier has a (field-name, repo-slug,
	// path) triple; we look up the host repo's local clone and join the
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

	// Remote checks (optional; only run when the corresponding Options
	// field is non-nil).
	failures = append(failures, runRepoExistsChecks(ctx, slugs, opts.RepoExists)...)
	failures = append(failures, runSonarChecks(ctx, cfg, opts)...)
	failures = append(failures, runBoardURLCheck(ctx, cfg, opts.BoardURLOK)...)

	return aggregateFailures(failures)
}

// aggregateFailures returns nil when failures is empty; otherwise a
// single error whose Error() lists every entry on its own bulleted line
// in sorted order. Centralised so the nil-cfg short-circuit path and
// the full structural path use one format.
func aggregateFailures(failures []string) error {
	if len(failures) == 0 {
		return nil
	}
	sort.Strings(failures)
	return fmt.Errorf("preflight failed:\n  - %s", strings.Join(failures, "\n  - "))
}

// runRepoExistsChecks calls RepoExists once per slug. A nil checker
// means the operator opted out; the call is a no-op.
func runRepoExistsChecks(ctx context.Context, slugs []string, check func(context.Context, string) (bool, error)) []string {
	if check == nil {
		return nil
	}
	var failures []string
	for _, slug := range slugs {
		ok, err := check(ctx, slug)
		if err != nil {
			failures = append(failures, fmt.Sprintf("repo %s: remote check failed: %v", slug, err))
			continue
		}
		if !ok {
			failures = append(failures, fmt.Sprintf("repo %s: does not exist on GitHub (or not visible to your gh auth)", slug))
		}
	}
	return failures
}

// runSonarChecks verifies the SonarCloud org + every declared per-tier
// project key. Org check gates the per-tier checks: if the org isn't
// there, the project keys can't exist either and the per-tier failures
// would just be noise.
func runSonarChecks(ctx context.Context, cfg *projectconfig.Config, opts Options) []string {
	if opts.SonarOrgExists == nil && opts.SonarProjectExists == nil {
		return nil
	}
	if cfg.Sonar.Organization == "" {
		// Validate already rejects empty org when arch is set, so the
		// only way to land here is a partial config where there's
		// nothing meaningful to check remotely.
		return nil
	}
	var failures []string
	orgOK := true
	if opts.SonarOrgExists != nil {
		ok, err := opts.SonarOrgExists(ctx, cfg.Sonar.Organization)
		if err != nil {
			failures = append(failures, fmt.Sprintf("sonar.organization %s: remote check failed: %v", cfg.Sonar.Organization, err))
			orgOK = false
		} else if !ok {
			failures = append(failures, fmt.Sprintf("sonar.organization %s: does not exist on SonarCloud", cfg.Sonar.Organization))
			orgOK = false
		}
	}
	if !orgOK || opts.SonarProjectExists == nil {
		return failures
	}
	for _, sp := range collectSonarProjects(cfg) {
		ok, err := opts.SonarProjectExists(ctx, sp.key)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s %s: remote check failed: %v", sp.field, sp.key, err))
			continue
		}
		if !ok {
			failures = append(failures, fmt.Sprintf("%s %s: does not exist on SonarCloud", sp.field, sp.key))
		}
	}
	return failures
}

// runBoardURLCheck verifies cfg.Project.URL resolves via the gh CLI.
// Empty project.url is accepted at validate-time (Rule 9 in
// projectconfig.Validate) — preflight respects that: with no URL set,
// there's nothing to verify here. The ATDD runtime still re-checks
// presence at board-resolution time.
func runBoardURLCheck(ctx context.Context, cfg *projectconfig.Config, check func(context.Context, string) error) []string {
	if check == nil || cfg.Project.URL == "" {
		return nil
	}
	if err := check(ctx, cfg.Project.URL); err != nil {
		return []string{fmt.Sprintf("project.url %s: %v", cfg.Project.URL, err)}
	}
	return nil
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
			field: "system-test.path",
			repo:  cfg.SystemTest.Repo,
			path:  cfg.SystemTest.Path,
		})
		// Family B sub-paths under system-test.paths.* live in the same
		// host repo as system-test.path. Sorted iteration keeps the
		// aggregated error output deterministic.
		keys := make([]string, 0, len(cfg.SystemTest.Paths))
		for k := range cfg.SystemTest.Paths {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			out = append(out, tierCheck{
				field: "system-test.paths." + k,
				repo:  cfg.SystemTest.Repo,
				path:  cfg.SystemTest.Paths[k],
			})
		}
	}
	// External systems — stubs first (cycle 2), simulators second (cycle 3).
	if !cfg.ExternalSystems.Stubs.IsEmpty() {
		out = append(out, tierCheck{
			field: "external-systems.stubs.path",
			repo:  cfg.ExternalSystems.Stubs.Repo,
			path:  cfg.ExternalSystems.Stubs.Path,
		})
	}
	if !cfg.ExternalSystems.Simulators.IsEmpty() {
		out = append(out, tierCheck{
			field: "external-systems.simulators.path",
			repo:  cfg.ExternalSystems.Simulators.Repo,
			path:  cfg.ExternalSystems.Simulators.Path,
		})
	}
	return out
}

// sonarProjectCheck pairs a YAML field-name with the sonar-project key
// declared at that field. Field is used purely for error messages.
type sonarProjectCheck struct {
	field string
	key   string
}

// collectSonarProjects returns every populated sonar-project entry in
// cfg, in the same deterministic order collectTiers uses for the local
// pass.
func collectSonarProjects(cfg *projectconfig.Config) []sonarProjectCheck {
	var out []sonarProjectCheck
	switch cfg.System.Architecture {
	case projectconfig.ArchMonolith:
		if cfg.System.SonarProject != "" {
			out = append(out, sonarProjectCheck{
				field: "system.sonar-project",
				key:   cfg.System.SonarProject,
			})
		}
	case projectconfig.ArchMultitier:
		if cfg.System.Backend.SonarProject != "" {
			out = append(out, sonarProjectCheck{
				field: "system.backend.sonar-project",
				key:   cfg.System.Backend.SonarProject,
			})
		}
		if cfg.System.Frontend.SonarProject != "" {
			out = append(out, sonarProjectCheck{
				field: "system.frontend.sonar-project",
				key:   cfg.System.Frontend.SonarProject,
			})
		}
	}
	if cfg.SystemTest.SonarProject != "" {
		out = append(out, sonarProjectCheck{
			field: "system-test.sonar-project",
			key:   cfg.SystemTest.SonarProject,
		})
	}
	return out
}
