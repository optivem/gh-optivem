package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// dirtyTreePaths enumerates the paths `git status --porcelain` reports
// (tracked-modified + untracked + both endpoints of any rename row),
// sorted and de-duplicated. This is the path set both
// captureWorkingTreeFingerprint and modifiedPathsSinceFingerprint
// iterate over — `git status --porcelain` is the authoritative dirty
// set; clean tracked files are intentionally excluded (a file clean at
// snapshot time and dirty afterwards still surfaces in the post-state
// call).
func (a actions) dirtyTreePaths(ctx context.Context) ([]string, error) {
	gitArgs := func(extra ...string) []string {
		if a.deps.RepoPath == "" {
			return extra
		}
		return append([]string{"-C", a.deps.RepoPath}, extra...)
	}
	status, err := a.deps.Git.Run(ctx, gitArgs("status", "--porcelain")...)
	if err != nil {
		return nil, fmt.Errorf("git status --porcelain: %w", err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(status), "\n") {
		// porcelain v1 format: "XY path" or "XY old -> new"; X is the
		// staged status, Y the unstaged. Two status chars + one space
		// before the path.
		if len(line) < 4 {
			continue
		}
		rest := line[3:]
		if i := strings.Index(rest, " -> "); i >= 0 {
			oldPath := strings.TrimSpace(rest[:i])
			newPath := strings.TrimSpace(rest[i+4:])
			if oldPath != "" {
				seen[oldPath] = true
			}
			if newPath != "" {
				seen[newPath] = true
			}
			continue
		}
		path := strings.TrimSpace(rest)
		if path != "" {
			seen[path] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for p := range seen {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

// hashRepoFile returns the hex SHA-256 of <RepoPath>/<rel>. A missing
// or unreadable file returns "" — the delta comparator treats that as
// "absent on disk", which combined with "present in snapshot" surfaces
// as a delta (deleted by the phase).
func (a actions) hashRepoFile(rel string) string {
	full := rel
	if a.deps.RepoPath != "" {
		full = filepath.Join(a.deps.RepoPath, rel)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// captureWorkingTreeFingerprint takes a snapshot of every dirty path
// reported by `git status --porcelain`, hashing the bytes of each file
// via SHA-256. The resulting WorkingTreeFingerprint is the baseline a
// subsequent modifiedPathsSinceFingerprint call diffs against to
// compute *this phase's* edits — independent of upstream phases that
// have also left uncommitted changes in the working tree.
//
// Returns a hard error only when `git status` itself fails (genuine
// wiring problem); per-file read failures degrade gracefully to an
// empty hash entry (see hashRepoFile).
func (a actions) captureWorkingTreeFingerprint(ctx context.Context) (WorkingTreeFingerprint, error) {
	paths, err := a.dirtyTreePaths(ctx)
	if err != nil {
		return nil, err
	}
	fp := make(WorkingTreeFingerprint, len(paths))
	for _, p := range paths {
		fp[p] = a.hashRepoFile(p)
	}
	return fp, nil
}

// modifiedPathsSinceFingerprint returns the paths that changed between
// the supplied snapshot and the current working tree:
//
//   - present in snapshot, hash differs on disk → modified (or
//     deleted, in which case the on-disk hash is "")
//   - absent in snapshot, present in current `git status` → added by
//     this phase
//   - present in both with matching hashes → untouched (upstream-phase
//     residue, correctly excluded)
//
// Returns a sorted, de-duplicated slice — the same shape
// validateOutputsAndScopes and checkPhaseScope iterate over.
func (a actions) modifiedPathsSinceFingerprint(ctx context.Context, base WorkingTreeFingerprint) ([]string, error) {
	nowPaths, err := a.dirtyTreePaths(ctx)
	if err != nil {
		return nil, err
	}
	delta := map[string]bool{}
	for p, baseHash := range base {
		if a.hashRepoFile(p) != baseHash {
			delta[p] = true
		}
	}
	for _, p := range nowPaths {
		if _, inBase := base[p]; !inBase {
			delta[p] = true
		}
	}
	out := make([]string, 0, len(delta))
	for p := range delta {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
