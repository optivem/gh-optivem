// Package repolocator turns a parsed projectconfig.Config into a map from
// repo slug → absolute local clone path. It does not check that the
// resolved paths exist on disk — that is the runtime preflight's job.
//
// Resolution rule (one formula for both mono- and multi-repo layouts):
//
//	workspace        = --workspace flag value, or parent(CWD) if unset
//	clone_path(slug) = <workspace>/<repo-name(slug)>
//
// The clone directory's name must therefore match the repo-name component
// of the slug. For mono-repo this is no constraint because the operator
// runs from inside the clone (`parent(CWD)/<repo-name>` == CWD when CWD
// is `<workspace>/<repo-name>`); for multi-repo it was already the
// sibling-dir convention. Operators with outlier clones symlink them
// into the workspace dir.
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

// Resolve places every slug in cfg.Repos() under <workspace>/<repo-name>.
// workspace is the operator-supplied workspace root (from the --workspace
// flag); when empty, the formula defaults to filepath.Dir(cwd). cwd is
// the working directory used by the default branch; when empty, the
// process CWD is used.
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
	if workspace == "" {
		workspace = filepath.Dir(cwd)
	}

	out := Result{Local: make(map[string]string, len(slugs))}
	for _, slug := range slugs {
		out.Local[slug] = filepath.Join(workspace, repoName(slug))
	}
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
