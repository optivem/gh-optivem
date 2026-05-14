// Package workspace resolves the academy workspace root and enumerates the
// repo folders declared in its *.code-workspace file. It is the Go port of
// the bash load_workspace_folders helper in
// academy/github-utils/scripts/common.sh, but uses encoding/json directly
// rather than shelling out to node.
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

// Resolve returns the absolute path to the workspace root and the absolute
// paths of every repo declared in its *.code-workspace file that exists
// and contains a .git/ directory. The lookup cascades:
//
//  1. flagValue (if non-empty) — treated as the workspace ROOT, must contain
//     exactly one *.code-workspace file.
//  2. $GH_OPTIVEM_WORKSPACE — same semantics as flagValue.
//  3. Walk up from CWD looking for any directory containing a *.code-workspace
//     file; the first hit wins.
//
// Non-existent paths, paths without a .git/ subdir, and empty path entries
// are silently skipped — matching the filter in commit.sh:183. A malformed
// JSON file, missing folders[] section, or no discoverable workspace is an
// error.
func Resolve(flagValue string) (root string, folders []string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil, fmt.Errorf("workspace: getwd: %w", err)
	}
	return resolveFrom(flagValue, os.Getenv(EnvVar), cwd)
}

// resolveFrom is the testable core of Resolve. Tests inject cwd and the env
// var value directly rather than relying on process state.
func resolveFrom(flagValue, envValue, cwd string) (root string, folders []string, err error) {
	var wsFile string
	switch {
	case flagValue != "":
		wsFile, err = findWorkspaceFile(flagValue)
		if err != nil {
			return "", nil, fmt.Errorf("workspace: --workspace %s: %w", flagValue, err)
		}
	case envValue != "":
		wsFile, err = findWorkspaceFile(envValue)
		if err != nil {
			return "", nil, fmt.Errorf("workspace: $%s=%s: %w", EnvVar, envValue, err)
		}
	default:
		wsFile, err = walkUp(cwd)
		if err != nil {
			return "", nil, err
		}
	}

	root = filepath.Dir(wsFile)
	folders, err = parseWorkspaceFolders(wsFile, root)
	if err != nil {
		return "", nil, err
	}
	return root, folders, nil
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

// walkUp searches start and each parent directory for a *.code-workspace
// file, returning the first match. When several files exist in the same
// directory, the lexicographically smallest wins — parity with
// `ls *.code-workspace | head -1` in common.sh.
func walkUp(start string) (string, error) {
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
			return "", fmt.Errorf("workspace: no *.code-workspace file found by walking up from %s; set --workspace, $%s, or cd into the academy tree", start, EnvVar)
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
