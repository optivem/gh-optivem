// Package repolocator turns a parsed projectconfig.Config into a map from
// repo slug → absolute local clone path. It does not check that the
// resolved paths exist on disk — that is the runtime preflight's job.
//
// Two resolution rules, branched on the project's declared repo-strategy:
//
//  1. mono-repo (cfg.RepoStrategy == "mono-repo", no --workspace pinned):
//     walk up from CWD for a .git entry (directory or file — worktrees
//     use a .git file) and use that directory as the single clone path.
//     Robust to invocations from a subdirectory of the clone and to git
//     worktrees, where the parent(CWD) heuristic gets the wrong answer.
//
//  2. multi-repo (everything else: multi-repo strategy, empty strategy,
//     or any strategy with an explicit --workspace):
//
//	workspace        = --workspace flag value, or parent(CWD) if unset
//	clone_path(slug) = <workspace>/<repo-name(slug)>
//
//     The clone directory's name must match the repo-name component of
//     the slug — the sibling-dir convention. Operators with outlier
//     clones symlink them into the workspace dir.
//
// When the mono-repo branch's walk-up finds no .git on the way to the
// filesystem root, the locator falls through to rule 2 so configurations
// that invoke from outside a clone still resolve (preflight then surfaces
// any missing-clone failure with a clear message).
package repolocator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Result bundles the resolved-slug map and any slugs that fell off the
// formula. Unresolved slugs are not an error here — the runtime preflight
// decides whether to surface them.
type Result struct {
	// Local maps repo slug (owner/name) → absolute local path.
	Local map[string]string
	// Unresolved lists slugs the locator couldn't place. Sorted.
	Unresolved []string
}

// Resolve places every slug in cfg.Repos() under its resolved clone path.
// See the package doc for the two-branch resolution rule. workspace is
// the operator-supplied workspace root (from the --workspace flag); when
// empty and the mono-repo branch does not match, the formula defaults to
// filepath.Dir(cwd). cwd is the working directory used by the default
// branch; when empty, the process CWD is used.
func Resolve(cfg *projectconfig.Config, workspace string, cwd string) (Result, error) {
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

	// Mono-repo branch: when the project declares a single clone and the
	// operator did not pin --workspace, use the git-resolved repo root
	// instead of parent(cwd)/<repo-name>. Robust to invocations from a
	// subdirectory of the clone and to git worktrees, where the
	// parent-of-cwd heuristic gets the wrong answer.
	if cfg.RepoStrategy == projectconfig.RepoStrategyMonoRepo && workspace == "" {
		if root := walkUpForGitRoot(cwd); root != "" {
			return Result{Local: map[string]string{slugs[0]: root}}, nil
		}
		// Fall through if cwd is not inside a git repo — preflight will
		// surface the missing-clone failure with a clear message.
	}

	if workspace == "" {
		workspace = filepath.Dir(cwd)
	}

	out := Result{Local: make(map[string]string, len(slugs))}
	for _, slug := range slugs {
		out.Local[slug] = filepath.Join(workspace, repoName(slug))
	}
	return out, nil
}

// walkUpForGitRoot searches start and each parent for a .git entry
// (directory or file — git worktrees use a .git file). Returns the
// absolute path of the containing directory, or "" when none is found
// on the way to the filesystem root.
func walkUpForGitRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// repoName extracts the right-hand component of `owner/name`, or returns
// the input verbatim when there is no slash.
func repoName(slug string) string {
	if idx := strings.LastIndex(slug, "/"); idx >= 0 && idx < len(slug)-1 {
		return slug[idx+1:]
	}
	return slug
}
