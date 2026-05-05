// Package repolocator turns a parsed projectconfig.Config into a map from
// repo slug → absolute local clone path. It does not check that the
// resolved paths exist on disk — that is the runtime preflight's job.
//
// Three resolution strategies are tried per slug, first match wins:
//
//  1. Explicit --repo-dir flag (`slug=path`, repeatable on the implement-
//     ticket command). Bypasses the convention entirely.
//  2. $GH_OPTIVEM_WORKSPACE env var. If set, the local path for repo
//     `<owner>/<name>` is `$GH_OPTIVEM_WORKSPACE/<name>`.
//  3. Sibling-dir convention. For mono-repo, the sole repo's local path
//     is the CWD (already where implement-ticket runs). For multi-repo,
//     each repo's local path is `<dirname(cwd)>/<repo-name>`.
//
// Unresolved slugs (none of the three strategies produced a path) are
// returned to the caller — the locator does not treat them as errors.
package repolocator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// EnvWorkspace is the env var consulted for the second resolution
// strategy. Exported so tests can clear/set it deterministically.
const EnvWorkspace = "GH_OPTIVEM_WORKSPACE"

// Result bundles the resolved-slug map and any slugs that fell off all
// three strategies. Unresolved slugs are not an error here — the runtime
// preflight decides whether to surface them.
type Result struct {
	// Local maps repo slug (owner/name) → absolute local path.
	Local map[string]string
	// Unresolved lists slugs the locator couldn't place. Sorted.
	Unresolved []string
}

// Resolve walks the slugs in cfg.Repos() through the three strategies.
// repoDirs is the parsed --repo-dir flag map (slug → path); empty when
// the flag wasn't set. cwd is the working directory used by the sibling-
// convention strategy; when empty, the process CWD is used.
func Resolve(cfg *projectconfig.Config, repoDirs map[string]string, cwd string) (Result, error) {
	if cfg == nil {
		return Result{Local: map[string]string{}}, nil
	}
	slugs := cfg.Repos()
	if len(slugs) == 0 {
		return Result{Local: map[string]string{}}, nil
	}

	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return Result{}, fmt.Errorf("repolocator: getwd: %w", err)
		}
	}

	wsEnv := os.Getenv(EnvWorkspace)

	out := Result{Local: make(map[string]string, len(slugs))}
	for _, slug := range slugs {
		if path, ok := repoDirs[slug]; ok && path != "" {
			out.Local[slug] = path
			continue
		}
		if wsEnv != "" {
			out.Local[slug] = filepath.Join(wsEnv, repoName(slug))
			continue
		}
		// Sibling-dir convention.
		if cfg.RepoStrategy == projectconfig.RepoStrategyMonoRepo || len(slugs) == 1 {
			out.Local[slug] = cwd
			continue
		}
		// Multi-repo: sibling of CWD with the repo's name.
		parent := filepath.Dir(cwd)
		out.Local[slug] = filepath.Join(parent, repoName(slug))
	}

	// No "unresolved" slugs in the current rule set — every slug ends up
	// with a path under one of the three strategies. The Unresolved
	// field is reserved for future rules where a slug might fall off
	// (e.g. a future strategy that's allowed to refuse).
	return out, nil
}

// repoName extracts the right-hand component of `owner/name`, or returns
// the input verbatim when there is no slash.
func repoName(slug string) string {
	if idx := strings.LastIndex(slug, "/"); idx >= 0 && idx < len(slug)-1 {
		return slug[idx+1:]
	}
	return slug
}

// ParseRepoDirFlag parses a `--repo-dir slug=path` string slice into the
// map Resolve consumes. Returns an error on the first malformed entry.
// Empty input returns an empty map (not nil) so callers can call without
// a nil-check.
func ParseRepoDirFlag(pairs []string) (map[string]string, error) {
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		idx := strings.Index(p, "=")
		if idx <= 0 || idx == len(p)-1 {
			return nil, fmt.Errorf("repolocator: --repo-dir %q must be slug=path", p)
		}
		slug := strings.TrimSpace(p[:idx])
		path := strings.TrimSpace(p[idx+1:])
		if slug == "" || path == "" {
			return nil, fmt.Errorf("repolocator: --repo-dir %q must be slug=path", p)
		}
		out[slug] = path
	}
	return out, nil
}
