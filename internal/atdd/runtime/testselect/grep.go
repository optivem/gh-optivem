package testselect

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// callersOf returns the names of methods (in dslFiles) that contain at
// least one call-site matching `methodName`. The returned set is sorted
// and deduplicated.
func callersOf(methodName string, dslFiles []string, idx *methodIndex, lay *layout, read func(string, string) ([]byte, error)) []string {
	hits := map[string]bool{}
	for _, f := range dslFiles {
		body, err := read("", f)
		if err != nil {
			continue
		}
		offsets := lay.CallerFinder(string(body), methodName)
		if len(offsets) == 0 {
			continue
		}
		regions := idx.byFile[f]
		for _, off := range offsets {
			line := byteOffsetToLine(string(body), off)
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
	hits := map[string]testHit{}
	for _, f := range testFiles {
		body, err := read("", f)
		if err != nil {
			continue
		}
		offsets := lay.CallerFinder(string(body), methodName)
		if len(offsets) == 0 {
			continue
		}
		// TS test-region resolution is file-granular: tree-sitter doesn't recognise it()/describe() arrow callbacks as methods.
		if lay.Lang == "typescript" {
			for _, candidates := range testIdx {
				for _, h := range candidates {
					if h.File == f {
						hits[h.Name] = h
					}
				}
			}
			continue
		}
		regions := lay.MethodIndexer(string(body))
		for _, off := range offsets {
			line := byteOffsetToLine(string(body), off)
			for _, r := range regions {
				if line < r.startLine || line > r.endLine {
					continue
				}
				for _, h := range testIdx[r.name] {
					if h.File == f {
						hits[h.Name] = h
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

// resolveAdapterToPortBackedMethods bridges a changed adapter method up
// to whichever adapter method actually fulfils a port contract. Page
// Object helpers (e.g. `NewOrderPage.inputSku`) live under the adapter
// tree but have no corresponding port method — the port is named after
// the driver-level method (e.g. `placeOrder`) that calls them. The
// selector therefore walks adapter callers transitively and stops at
// the first port-backed ancestor on each branch.
//
// Returns []string{name} when `name` itself is port-backed; the bridged
// set when it is not; nil when no resolution is possible. Empty result
// means "still unmapped — fall back to full suite".
func resolveAdapterToPortBackedMethods(
	name string,
	portMethods *methodIndex,
	adapterFiles []string,
	adapterMethods *methodIndex,
	lay *layout,
	read func(string, string) ([]byte, error),
) []string {
	if _, ok := portMethods.byName[name]; ok {
		return []string{name}
	}

	visited := map[string]bool{name: true}
	frontier := []string{name}
	resolved := map[string]bool{}

	for len(frontier) > 0 {
		var next []string
		for _, n := range frontier {
			callers := callersOf(n, adapterFiles, adapterMethods, lay, read)
			for _, c := range callers {
				if visited[c] {
					continue
				}
				visited[c] = true
				if _, ok := portMethods.byName[c]; ok {
					resolved[c] = true
					continue
				}
				next = append(next, c)
			}
		}
		frontier = next
	}

	if len(resolved) == 0 {
		return nil
	}
	out := make([]string, 0, len(resolved))
	for n := range resolved {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// classQualifyPortCandidates narrows port matches by interface
// compatibility. A port record survives only if its declaring file's
// interface name is in the implements / extends / `:` parents list of
// some adapter file that declares `methodName`. Returns nil when no
// candidate survives — the caller falls back to the unfiltered list so
// fixtures and corner cases without parseable class info keep working.
func classQualifyPortCandidates(
	methodName string,
	candidates []methodRecord,
	adapterMethods *methodIndex,
	adapterParentsByFile map[string][]string,
	portDeclaredByFile map[string][]string,
) []methodRecord {
	parentSet := map[string]bool{}
	for _, rec := range adapterMethods.byName[methodName] {
		for _, p := range adapterParentsByFile[rec.File] {
			parentSet[p] = true
		}
	}
	if len(parentSet) == 0 {
		return nil
	}
	var out []methodRecord
	for _, c := range candidates {
		for _, decl := range portDeclaredByFile[c.File] {
			if parentSet[decl] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// narrowDSLByPortType returns the subset of `dslFiles` that mention any
// of the given port type names. Used to skip DSLs that don't actually
// inject the port whose method we're chasing — without this, a regex
// match on `port.foo(` in an unrelated DSL fans out into tests that have
// nothing to do with the change. Falls back to the full list when no DSL
// mentions any port type (treats as "type info unparseable, behave as
// before").
func narrowDSLByPortType(dslFiles []string, dslByPortType map[string]map[string]bool, portTypes []string) []string {
	if len(portTypes) == 0 {
		return dslFiles
	}
	union := map[string]bool{}
	for _, t := range portTypes {
		for f := range dslByPortType[t] {
			union[f] = true
		}
	}
	if len(union) == 0 {
		return dslFiles
	}
	var out []string
	for _, f := range dslFiles {
		if union[f] {
			out = append(out, f)
		}
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
