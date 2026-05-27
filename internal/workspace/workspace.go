// Package workspace resolves the cross-repo scope for a command invocation.
// It is the source of truth for "which repos should this verb iterate?":
//
//   - When a *.code-workspace file is reachable (via flag, env, or walking
//     up from CWD), the scope is the workspace and the folders enumerated
//     from its folders[] array.
//   - When no workspace file is reachable but CWD is inside a project
//     whose gh-optivem.yaml declares a non-empty repos:, the scope is
//     the project — the listed local repos are iterated.
//   - When neither of the above resolves but CWD is inside a git repo,
//     the scope shrinks to that single repo.
//   - When none of the above is true, the call errors.
//
// The project / single-repo fallbacks let the cross-repo verbs (commit,
// sync, …) Just Do The Right Thing whether the operator is inside a
// multi-repo workspace, a multitier project, or a standalone clone. The
// mode is surfaced to callers via Scope.Mode so the banner can announce
// which scope resolved.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// EnvVar names the environment variable that pins the workspace root when
// the operator does not pass --workspace.
const EnvVar = "GH_OPTIVEM_WORKSPACE"

// Mode discriminates the three possible resolved scopes.
type Mode int

const (
	// ModeWorkspace means Folders enumerates every repo declared in the
	// resolved *.code-workspace file.
	ModeWorkspace Mode = iota
	// ModeProject means Folders enumerates every repo declared in the
	// resolved gh-optivem.yaml's repos: field — a multitier project's
	// constituent local repos. Only reached by walk-up; the flag and
	// env-var cascade entries still target workspace files.
	ModeProject
	// ModeSingleRepo means Folders contains exactly one entry — the git
	// repo the operator is inside. Neither a workspace file nor a
	// project config with non-empty repos: was reachable.
	ModeSingleRepo
)

// Scope is the resolved cross-repo scope for one command invocation.
type Scope struct {
	Mode Mode
	// Root is the workspace root directory (ModeWorkspace) or the git
	// repo's root directory (ModeSingleRepo).
	Root string
	// Folders is every targeted repo. In ModeSingleRepo it has exactly
	// one entry equal to Root.
	Folders []string
	// SourceFile is the absolute path to the file the scope was derived
	// from — the resolved *.code-workspace in ModeWorkspace, the
	// resolved gh-optivem.yaml in ModeProject, or "" in ModeSingleRepo.
	SourceFile string
}

// Resolve returns the resolved scope for the current invocation. Cascade:
//
//  1. flagValue (if non-empty) — treated as a directory containing a
//     *.code-workspace file → ModeWorkspace.
//  2. $GH_OPTIVEM_WORKSPACE — same semantics as flagValue.
//  3. Walk up from CWD for a *.code-workspace file → ModeWorkspace,
//     BUT only if the CWD's git repo is one of the workspace's
//     folders[] entries. If the workspace file doesn't claim CWD's repo
//     as a member, the walk-up is treated as a non-match and the cascade
//     falls through to row 4. This prevents a git worktree (or any
//     standalone repo) placed inside a workspace tree from being
//     silently overridden by the surrounding workspace's folder list.
//  4. Walk up from CWD for a gh-optivem.yaml with non-empty repos: →
//     ModeProject, BUT only if the config "claims" CWD's repo: either
//     it sits at CWD's git repo root (monolith), or its repos: include
//     CWD's git repo (multitier subrepo). A stray outer gh-optivem.yaml
//     reached by walk-up but not claiming CWD's repo is silently
//     skipped — same protection as row 3 for *.code-workspace files.
//     Parse errors on such an outer file are also suppressed (it's not
//     our config; surfacing its problems would be noise — e.g. a stale
//     academy-level gh-optivem.yaml above an ATDD rehearsal worktree).
//  5. Walk up from CWD for a .git/ entry → ModeSingleRepo with Folders =
//     []string{<repo root>}.
//  6. Nothing → error.
//
// Within ModeWorkspace, repos declared in folders[] but missing on disk
// or without a .git/ subdir are silently filtered out (matches the
// commit.sh:183 behavior). Malformed JSON, missing/empty folders[], and
// flag/env paths that do not exist are errors. The flag/env entries do
// NOT apply the membership check from row 3 — those are explicit
// operator intent and are honored even when CWD is outside the
// workspace tree.
//
// Within ModeProject, the same filter applies to repos: entries —
// non-existent paths and non-git folders are skipped. If gh-optivem.yaml
// has no repos: field, an empty repos:, or every entry filters out, the
// cascade falls through to the .git walk-up rather than erroring (the
// project simply has no cross-repo scope to express). A malformed or
// schema-invalid gh-optivem.yaml is surfaced when it belongs to CWD
// (sits at CWD's repo root, or there is no git repo above CWD); when
// it's an outer file that doesn't claim CWD's repo, the error is
// suppressed and the cascade falls through.
func Resolve(flagValue string) (Scope, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Scope{}, fmt.Errorf("workspace: getwd: %w", err)
	}
	return resolveFrom(flagValue, os.Getenv(EnvVar), cwd)
}

// resolveFrom is the testable core of Resolve. Tests inject cwd and the
// env var value directly rather than relying on process state.
func resolveFrom(flagValue, envValue, cwd string) (Scope, error) {
	var wsFile string
	var err error
	// explicit tracks whether wsFile came from --workspace or
	// $GH_OPTIVEM_WORKSPACE (true) versus walk-up (false). Only the
	// walk-up path applies the CWD-membership check below; explicit
	// paths are honored as-is.
	var explicit bool

	switch {
	case flagValue != "":
		wsFile, err = findWorkspaceFile(flagValue)
		if err != nil {
			return Scope{}, fmt.Errorf("workspace: --workspace %s: %w", flagValue, err)
		}
		explicit = true
	case envValue != "":
		wsFile, err = findWorkspaceFile(envValue)
		if err != nil {
			return Scope{}, fmt.Errorf("workspace: $%s=%s: %w", EnvVar, envValue, err)
		}
		explicit = true
	default:
		// Walk up looking for a workspace file. If none is found, fall
		// through to the single-repo cascade rather than erroring — the
		// operator may simply be inside a standalone clone.
		wsFile, _ = walkUpForWorkspace(cwd)
	}

	if wsFile != "" {
		root := filepath.Dir(wsFile)
		folders, err := parseWorkspaceFolders(wsFile, root)
		if err != nil {
			return Scope{}, err
		}
		if explicit || cwdInFolders(cwd, folders) {
			return Scope{
				Mode:       ModeWorkspace,
				Root:       root,
				Folders:    folders,
				SourceFile: wsFile,
			}, nil
		}
		// Walk-up found a workspace file, but CWD's repo is not one of
		// its folders[]. Fall through to ModeProject / ModeSingleRepo so
		// the operator's actual repo wins instead of being silently
		// overridden by the surrounding workspace's folder list.
	}

	// No workspace file found (or walk-up rejected by CWD-membership).
	// Try project-iteration: a gh-optivem.yaml above CWD with a
	// non-empty repos: list whose declared repos include CWD's git repo
	// (or, for monolith configs without repos:, that sit at CWD's repo
	// root).
	cfgFile, _ := walkUpForProjectConfig(cwd)
	if cfgFile != "" {
		cfgRoot := filepath.Dir(cfgFile)
		folders, parseErr := parseProjectRepos(cfgFile, cfgRoot)
		switch {
		case !cfgAppliesToCwd(cwd, cfgFile, folders):
			// Stray outer gh-optivem.yaml — found by walk-up but does
			// not describe CWD's repo. Mirrors the row-3 membership
			// guard for *.code-workspace files. Silently fall through
			// (parse errors included) rather than letting an unrelated
			// outer file hijack or break this invocation.
		case parseErr != nil:
			return Scope{}, parseErr
		case len(folders) > 0:
			return Scope{
				Mode:       ModeProject,
				Root:       cfgRoot,
				Folders:    folders,
				SourceFile: cfgFile,
			}, nil
		}
		// Applies, parsed cleanly, but no usable repos: — fall through
		// to single-repo, the documented behavior for monolith projects.
	}

	// Final fallback: cwd is inside a git repo.
	repoRoot, err := walkUpForGitRepo(cwd)
	if err != nil {
		return Scope{}, fmt.Errorf("workspace: no *.code-workspace file or git repo found by walking up from %s; set --workspace, $%s, or cd into a git repo (or a directory below a *.code-workspace file)", cwd, EnvVar)
	}
	return Scope{
		Mode:    ModeSingleRepo,
		Root:    repoRoot,
		Folders: []string{repoRoot},
	}, nil
}

// findWorkspaceFile expects dir to be a directory containing exactly one
// *.code-workspace file and returns its absolute path.
func findWorkspaceFile(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("directory does not exist")
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	matches, err := filepath.Glob(filepath.Join(abs, "*.code-workspace"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no *.code-workspace file in %s", abs)
	}
	if len(matches) > 1 {
		sort.Strings(matches)
		return "", fmt.Errorf("multiple *.code-workspace files in %s: %v", abs, matches)
	}
	return matches[0], nil
}

// walkUpForWorkspace searches start and each parent directory for a
// *.code-workspace file, returning the first match. When several files
// exist in the same directory, the lexicographically smallest wins —
// parity with `ls *.code-workspace | head -1` in common.sh. Returns "" if
// none is found on the way to the filesystem root.
func walkUpForWorkspace(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.code-workspace"))
		if len(matches) > 0 {
			sort.Strings(matches)
			return matches[0], nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no *.code-workspace file found above %s", start)
		}
		dir = parent
	}
}

// walkUpForProjectConfig searches start and each parent directory for a
// gh-optivem.yaml file, returning the absolute path of the first match.
// Mirrors walkUpForWorkspace's contract: returns ("", error) when no
// file is found on the way to the filesystem root. The error is
// informational — callers fall through to the single-repo cascade row
// rather than surfacing it.
func walkUpForProjectConfig(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, projectconfig.Path)
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found above %s", projectconfig.Path, start)
		}
		dir = parent
	}
}

// cwdInFolders reports whether the git repo containing cwd is one of
// the resolved workspace folders. Used by the walk-up branch to detect
// the "workspace file found, but CWD's repo is not declared" case — a
// git worktree placed inside a workspace tree (e.g. an ATDD rehearsal
// worktree under academy/worktrees/) must not be silently overridden
// by the surrounding workspace.code-workspace.
//
// Returns true when:
//   - cwd is inside a git repo, AND
//   - that repo's root matches one of folders (case-insensitive compare
//     on Windows where path casing varies between sources).
//
// Returns false when cwd is not in any git repo, or its repo root is
// not in folders. The caller falls through to the project/single-repo
// cascade rows on false.
func cwdInFolders(cwd string, folders []string) bool {
	repoRoot, err := walkUpForGitRepo(cwd)
	if err != nil {
		return false
	}
	repoRoot = filepath.Clean(repoRoot)
	for _, f := range folders {
		if pathsEqual(filepath.Clean(f), repoRoot) {
			return true
		}
	}
	return false
}

// cfgAppliesToCwd reports whether cfgFile (a gh-optivem.yaml found by
// walk-up from cwd) actually describes CWD's repo. Used by the
// project-cascade branch to skip a stray outer file that walk-up
// reached but that has nothing to do with CWD — mirrors cwdInFolders
// for *.code-workspace files.
//
// Returns true when:
//   - cwd is not in any git repo (cannot disqualify the file — preserve
//     legacy behavior so misconfiguration in unusual setups is still
//     surfaced), OR
//   - cfgFile sits at CWD's git repo root (the config IS this repo's
//     config — monolith case), OR
//   - folders is non-empty AND one of its entries equals CWD's git
//     repo root (multitier subrepo case).
//
// Returns false otherwise — the caller silently falls through to the
// single-repo cascade row. folders may be nil (parse failed); in that
// case the function still returns true for the first two rules, so a
// broken gh-optivem.yaml inside CWD's repo continues to surface its
// parse error rather than being silently ignored.
func cfgAppliesToCwd(cwd, cfgFile string, folders []string) bool {
	repoRoot, err := walkUpForGitRepo(cwd)
	if err != nil {
		return true
	}
	repoRoot = filepath.Clean(repoRoot)
	cfgDir := filepath.Clean(filepath.Dir(cfgFile))
	if pathsEqual(cfgDir, repoRoot) {
		return true
	}
	for _, f := range folders {
		if pathsEqual(filepath.Clean(f), repoRoot) {
			return true
		}
	}
	return false
}

// pathsEqual compares two filesystem paths. On Windows, where the same
// path can appear in mixed casing or with different drive-letter casing
// depending on its source (env var vs. os.Getwd vs. filepath.Join), we
// fold case to avoid spurious mismatches.
func pathsEqual(a, b string) bool {
	if a == b {
		return true
	}
	return filepath.ToSlash(strings.ToLower(a)) == filepath.ToSlash(strings.ToLower(b))
}

// walkUpForGitRepo searches start and each parent for a .git entry
// (directory or file — git worktrees and submodules use .git as a file),
// returning the absolute path of the containing directory.
func walkUpForGitRepo(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		gitEntry := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitEntry); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .git found above %s", start)
		}
		dir = parent
	}
}

// parseProjectRepos reads cfgPath (a gh-optivem.yaml) and returns the
// absolute paths of every repos: entry that exists and contains .git/.
// Each entry's Path is resolved against root, which is the directory
// holding cfgPath. Non-existent paths and non-git folders are silently
// filtered — parity with parseWorkspaceFolders for *.code-workspace
// entries — so a half-scaffolded multitier project (one tier cloned,
// the others not yet) still iterates the tier that's present.
//
// An empty / missing repos: returns (nil, nil) so the caller can fall
// through to the single-repo row. A parse or schema-validation error on
// the YAML file is surfaced — the file exists but is broken, and the
// operator wants to know rather than silently scoping to one repo.
func parseProjectRepos(cfgPath, root string) ([]string, error) {
	cfg, err := projectconfig.LoadFromPath(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	if cfg == nil || len(cfg.LocalRepos) == 0 {
		return nil, nil
	}
	var folders []string
	for _, r := range cfg.LocalRepos {
		abs := r.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, r.Path)
		}
		abs = filepath.Clean(abs)
		gitDir := filepath.Join(abs, ".git")
		info, statErr := os.Stat(gitDir)
		if statErr != nil || !info.IsDir() {
			continue
		}
		folders = append(folders, abs)
	}
	return folders, nil
}

// parseWorkspaceFolders reads wsFile, decodes its folders[] array, and
// returns the absolute paths of folders that exist and contain .git/.
// Non-existent paths and non-git folders are filtered out. An empty or
// missing folders[] is an error — that indicates the wrong file.
func parseWorkspaceFolders(wsFile, root string) ([]string, error) {
	data, err := os.ReadFile(wsFile)
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", wsFile, err)
	}
	var raw struct {
		Folders []struct {
			Path string `json:"path"`
		} `json:"folders"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("workspace: could not parse %s as JSON: %w", wsFile, err)
	}
	if len(raw.Folders) == 0 {
		return nil, fmt.Errorf("workspace: %s has no folders[] entries", wsFile)
	}
	var folders []string
	for _, f := range raw.Folders {
		if f.Path == "" {
			continue
		}
		abs := f.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(root, f.Path)
		}
		abs = filepath.Clean(abs)
		gitDir := filepath.Join(abs, ".git")
		info, err := os.Stat(gitDir)
		if err != nil || !info.IsDir() {
			continue
		}
		folders = append(folders, abs)
	}
	return folders, nil
}
