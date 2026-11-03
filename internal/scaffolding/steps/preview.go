package steps

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PathClass says what applying the scaffold would do to one path of an
// existing repo.
type PathClass string

const (
	// ClassNew: in the scaffold, not in the repo. Would be added.
	ClassNew PathClass = "NEW"
	// ClassIdentical: same content (after line-ending normalisation for text
	// files). No-op.
	ClassIdentical PathClass = "IDENTICAL"
	// ClassOverwrite: in both, content differs. Would be replaced.
	ClassOverwrite PathClass = "OVERWRITE"
	// ClassKept: in the repo, not in the scaffold. Untouched.
	ClassKept PathClass = "KEPT"
)

// PreviewEntry is the classification of one path.
type PreviewEntry struct {
	// Path is slash-separated and relative to the repo root. For a
	// case-only match it is the scaffold's spelling.
	Path  string
	Class PathClass
	// ExistingPath is set only when the repo spells Path with different
	// letter case (a case-only rename). Case-insensitive filesystems treat the
	// two as the same file, so the entry is classified by content as if they
	// were one path, and the spelling change is surfaced here.
	ExistingPath string
	// Binary is true when either side is binary; line counts are then zero.
	Binary bool
	// Added and Removed are line counts for OVERWRITE text files.
	Added   int
	Removed int
}

// maxLCSCells bounds the line-diff work for a single file. Beyond it the
// differing middle section is counted as fully removed and fully added, which
// over-reports but never under-reports.
const maxLCSCells = 4_000_000

// BuildPreview classifies every path under scaffoldDir against repoDir (a clone
// of the existing repo). The .git directory on either side is ignored. It only
// reads; nothing is written. Entries are sorted by path.
//
// A rename (same content, different name) is reported as NEW plus KEPT: the
// engine does not detect renames, and the old file is never deleted.
func BuildPreview(scaffoldDir, repoDir string) ([]PreviewEntry, error) {
	scaffold, err := listFiles(scaffoldDir)
	if err != nil {
		return nil, fmt.Errorf("list scaffold dir %s: %w", scaffoldDir, err)
	}
	repo, err := listFiles(repoDir)
	if err != nil {
		return nil, fmt.Errorf("list repo dir %s: %w", repoDir, err)
	}

	repoByFold := make(map[string][]string, len(repo))
	for p := range repo {
		k := strings.ToLower(p)
		repoByFold[k] = append(repoByFold[k], p)
	}

	matched := make(map[string]bool, len(repo))
	var entries []PreviewEntry

	for p := range scaffold {
		existing := ""
		if _, ok := repo[p]; ok {
			existing = p
		} else if cands := repoByFold[strings.ToLower(p)]; len(cands) > 0 {
			sort.Strings(cands)
			existing = cands[0]
		}

		if existing == "" {
			entries = append(entries, PreviewEntry{Path: p, Class: ClassNew})
			continue
		}
		matched[existing] = true

		a, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(existing)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", existing, err)
		}
		b, err := os.ReadFile(filepath.Join(scaffoldDir, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("read scaffold %s: %w", p, err)
		}

		e := PreviewEntry{Path: p}
		if existing != p {
			e.ExistingPath = existing
		}
		classifyContent(&e, a, b)
		entries = append(entries, e)
	}

	for p := range repo {
		if !matched[p] {
			entries = append(entries, PreviewEntry{Path: p, Class: ClassKept})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// classifyContent fills Class, Binary, Added, Removed for a path present on
// both sides. old is the repo content, next is the scaffold content.
func classifyContent(e *PreviewEntry, old, next []byte) {
	if isBinary(old) || isBinary(next) {
		e.Binary = true
		if bytes.Equal(old, next) {
			e.Class = ClassIdentical
		} else {
			e.Class = ClassOverwrite
		}
		return
	}
	oldLines := splitLines(old)
	newLines := splitLines(next)
	added, removed := diffCounts(oldLines, newLines)
	if added == 0 && removed == 0 {
		e.Class = ClassIdentical
		return
	}
	e.Class = ClassOverwrite
	e.Added, e.Removed = added, removed
}

// isBinary reports whether data contains a NUL byte in its first 8000 bytes.
func isBinary(data []byte) bool {
	n := min(len(data), 8000)
	return bytes.IndexByte(data[:n], 0) >= 0
}

// splitLines normalises CRLF to LF, then splits on LF. A trailing newline
// does not produce an empty final line.
func splitLines(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// diffCounts returns the number of lines added and removed going from old to
// next, via a longest-common-subsequence over the section between the common
// prefix and suffix.
func diffCounts(old, next []string) (added, removed int) {
	start := 0
	for start < len(old) && start < len(next) && old[start] == next[start] {
		start++
	}
	endOld, endNew := len(old), len(next)
	for endOld > start && endNew > start && old[endOld-1] == next[endNew-1] {
		endOld--
		endNew--
	}
	a, b := old[start:endOld], next[start:endNew]
	if len(a) == 0 || len(b) == 0 || len(a)*len(b) > maxLCSCells {
		return len(b), len(a)
	}

	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
			} else if prev[j] >= cur[j-1] {
				cur[j] = prev[j]
			} else {
				cur[j] = cur[j-1]
			}
		}
		prev, cur = cur, prev
	}
	lcs := prev[len(b)]
	return len(b) - lcs, len(a) - lcs
}

// listFiles returns the set of slash-separated relative paths of regular files
// under root, skipping any .git directory.
func listFiles(root string) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	return out, err
}
