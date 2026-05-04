package testselect

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// callersOf returns the names of methods (in dslFiles) that contain at
// least one call-site matching `methodName`. The returned set is sorted
// and deduplicated.
func callersOf(methodName string, dslFiles []string, idx *methodIndex, lay *layout, read func(string, string) ([]byte, error)) []string {
	re := lay.CallerREFor(methodName)
	hits := map[string]bool{}
	for _, f := range dslFiles {
		body, err := read("", f)
		if err != nil {
			continue
		}
		matches := re.FindAllStringIndex(string(body), -1)
		if len(matches) == 0 {
			continue
		}
		regions := idx.byFile[f]
		for _, m := range matches {
			line := byteOffsetToLine(string(body), m[0])
			for _, r := range regions {
				if line >= r.startLine && line <= r.endLine {
					if r.name != methodName { // ignore self-recursion
						hits[r.name] = true
					}
					break
				}
			}
		}
	}
	out := make([]string, 0, len(hits))
	for k := range hits {
		out = append(out, k)
	}
	return out
}

// callersOfTest is callersOf for test files: each hit is a testHit (we
// already indexed annotations) so the caller can read channels directly.
func callersOfTest(methodName string, testFiles []string, testIdx map[string][]testHit, lay *layout, read func(string, string) ([]byte, error)) []testHit {
	re := lay.CallerREFor(methodName)
	hits := map[string]testHit{}
	for _, f := range testFiles {
		body, err := read("", f)
		if err != nil {
			continue
		}
		matches := re.FindAllStringIndex(string(body), -1)
		if len(matches) == 0 {
			continue
		}
		// For each match, find the enclosing test method using the
		// existing test index plus extract regions for the file.
		regions := extractMethodRegions(string(body), lay)
		for _, m := range matches {
			line := byteOffsetToLine(string(body), m[0])
			for _, r := range regions {
				if line < r.startLine || line > r.endLine {
					continue
				}
				// Find the testHit (if any) for this method name in this file.
				for _, h := range testIdx[r.name] {
					if h.File != f {
						continue
					}
					hits[h.Name] = h
				}
				// TS path: hits keyed differently — search testIdx full
				// names for ones whose file matches and whose enclosing
				// region covers `line`.
				if lay.Lang == "typescript" {
					for _, candidates := range testIdx {
						for _, h := range candidates {
							if h.File != f {
								continue
							}
							hits[h.Name] = h
						}
					}
				}
				break
			}
		}
	}
	out := make([]testHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, h)
	}
	return out
}

// transitiveDSLClosure expands the DSL set to include any DSL method that
// (transitively) calls a method already in the set. Returns the expanded
// set. The input set is not mutated.
func transitiveDSLClosure(seed map[string]bool, dslFiles []string, idx *methodIndex, lay *layout, read func(string, string) ([]byte, error)) map[string]bool {
	out := map[string]bool{}
	for k, v := range seed {
		out[k] = v
	}
	frontier := map[string]bool{}
	for k := range seed {
		frontier[k] = true
	}
	for len(frontier) > 0 {
		next := map[string]bool{}
		for name := range frontier {
			callers := callersOf(name, dslFiles, idx, lay, read)
			for _, c := range callers {
				if !out[c] {
					out[c] = true
					next[c] = true
				}
			}
		}
		frontier = next
	}
	return out
}

// byteOffsetToLine returns the 1-based line number of byte offset `off`
// in `body`. Counts '\n' before `off`.
func byteOffsetToLine(body string, off int) int {
	if off > len(body) {
		off = len(body)
	}
	return 1 + strings.Count(body[:off], "\n")
}

// filterPaths returns the subset of paths for which match returns true.
// A nil match function passes everything through.
func filterPaths(paths []string, match func(string) bool) []string {
	if match == nil {
		return paths
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if match(p) {
			out = append(out, p)
		}
	}
	return out
}

// readRepoFile reads a file at relPath under repoRoot. When repoRoot is
// empty (used by the index helpers, which receive absolute paths), the
// path is treated as absolute.
func readRepoFile(repoRoot, relPath string) ([]byte, error) {
	if repoRoot == "" {
		return os.ReadFile(relPath)
	}
	return os.ReadFile(filepath.Join(repoRoot, relPath))
}

// defaultWalk walks every directory in `roots`, returning files whose
// extension matches one of `exts`. Directories that don't exist are
// skipped silently — a project that doesn't have a Java tree just
// produces no Java hits.
func defaultWalk(repoRoot string, roots []string, exts []string) ([]string, error) {
	var paths []string
	extSet := map[string]bool{}
	for _, e := range exts {
		extSet[strings.ToLower(e)] = true
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				// Skip build / vendor noise.
				if name == "node_modules" || name == "bin" || name == "obj" ||
					name == "build" || name == ".gradle" || name == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if !extSet[ext] {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
	}
	return paths, nil
}
