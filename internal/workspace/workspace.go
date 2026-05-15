// Package workspace resolves the cross-repo scope for a command invocation.
// It is the source of truth for "which repos should this verb iterate?":
//
//   - When a *.code-workspace file is reachable (via flag, env, or walking
//     up from CWD), the scope is the workspace and the folders enumerated
//     from its folders[] array.
//   - When no workspace file is reachable but CWD is inside a git repo,
//     the scope shrinks to that single repo.
//   - When neither is true, the call errors.
//
// The single-repo fallback lets the cross-repo verbs (commit, sync, …)
// Just Do The Right Thing whether the operator is inside a multi-repo
// workspace or a standalone clone. The mode is surfaced to callers via
// Scope.Mode so the banner can announce which scope resolved.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// EnvVar names the environment variable that pins the workspace root when
// the operator does not pass --workspace.
const EnvVar = "GH_OPTIVEM_WORKSPACE"

// Mode discriminates the two possible resolved scopes.
type Mode int

const (
	// ModeWorkspace means Folders enumerates every repo declared in the
	// resolved *.code-workspace file.
	ModeWorkspace Mode = iota
	// ModeSingleRepo means Folders contains exactly one entry — the git
	// repo the operator is inside. No workspace file was reachable.
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
	// SourceFile is the absolute path to the resolved *.code-workspace
	// file in ModeWorkspace, or "" in ModeSingleRepo.
	SourceFile string
}

// Resolve returns the resolved scope for the current invocation. Cascade:
//
//  1. flagValue (if non-empty) — treated as a directory containing a
//     *.code-workspace file → ModeWorkspace.
//  2. $GH_OPTIVEM_WORKSPACE — same semantics as flagValue.
//  3. Walk up from CWD for a *.code-workspace file → ModeWorkspace.
//  4. Walk up from CWD for a .git/ entry → ModeSingleRepo with Folders =
//     []string{<repo root>}.
//  5. Nothing → error.
//
// Within ModeWorkspace, repos declared in folders[] but missing on disk
// or without a .git/ subdir are silently filtered out (matches the
// commit.sh:183 behavior). Malformed JSON, missing/empty folders[], and
// flag/env paths that do not exist are errors.
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

	switch {
	case flagValue != "":
		wsFile, err = findWorkspaceFile(flagValue)
		if err != nil {
			return Scope{}, fmt.Errorf("workspace: --workspace %s: %w", flagValue, err)
		}
	case envValue != "":
		wsFile, err = findWorkspaceFile(envValue)
		if err != nil {
			return Scope{}, fmt.Errorf("workspace: $%s=%s: %w", EnvVar, envValue, err)
		}
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
		return Scope{
			Mode:       ModeWorkspace,
			Root:       root,
			Folders:    folders,
			SourceFile: wsFile,
		}, nil
	}

	// No workspace file found by walk-up. Try single-repo fallback.
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
