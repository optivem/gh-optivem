package generate

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/spec"
)

var criterionIDPattern = regexp.MustCompile(`^UC-\d+\.\d+$`)

// ParseFile parses one test source; the language comes from the extension
// (.java, .cs, .ts). ok is false for any other file.
func ParseFile(path string, src []byte) (cases []TestCase, errs []error, ok bool) {
	var parse func(string, []token) ([]TestCase, []error)
	switch strings.ToLower(filepath.Ext(path)) {
	case ".java":
		parse = parseJava
	case ".cs":
		parse = parseCSharp
	case ".ts":
		parse = parseTypeScript
	default:
		return nil, nil, false
	}
	toks, err := lex(string(src))
	if err != nil {
		return nil, []error{ParseError{File: path, Line: 0, Reason: err.Error()}}, true
	}
	cases, errs = parse(path, toks)
	return cases, errs, true
}

// Options configure Generate.
type Options struct {
	// TestDirs are the directories (or files) whose test sources are read.
	TestDirs []string
	// UseCases supply the Feature titles; a criterion whose use case is not
	// among them is an error.
	UseCases []spec.UseCase
	// Only keeps these use cases (UC-NNN); empty keeps all, plus the
	// untagged block.
	Only []string
	// Exclude are spec.yaml exclude globs, matched against slash paths.
	Exclude []string
}

// Result is a generated document.
type Result struct {
	Gherkin string
	// Untagged are the tests that carry no criterion ID (rendered in the
	// trailing block), as file:line paths.
	Untagged []string
}

// Generate reads every test source under opts.TestDirs and renders the
// Gherkin document. It is all or nothing: any test the grammar rejects, and
// any criterion whose use case is unknown, is returned as an error (all of
// them, joined) and no document is produced.
func Generate(opts Options) (Result, error) {
	files, err := collectFiles(opts.TestDirs, opts.Exclude)
	if err != nil {
		return Result{}, err
	}
	titles := map[string]string{}
	for _, uc := range opts.UseCases {
		titles[uc.ID] = uc.Title
	}
	var cases []TestCase
	var errs []error
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", f, err)
		}
		cs, es, _ := ParseFile(filepath.ToSlash(f), src)
		errs = append(errs, es...)
		for _, tc := range cs {
			for _, id := range tc.Tags {
				if _, ok := titles[useCaseOf(id)]; !ok {
					errs = append(errs, ParseError{File: tc.File, Line: tc.Line, Test: tc.Name, Reason: fmt.Sprintf("criterion %s: use case %s is not in the spec dir", id, useCaseOf(id))})
				}
			}
		}
		cases = append(cases, cs...)
	}
	if len(errs) > 0 {
		return Result{}, errors.Join(errs...)
	}
	cases = filterUseCases(cases, opts.Only)
	var res Result
	for _, tc := range cases {
		if len(tc.Tags) == 0 {
			res.Untagged = append(res.Untagged, fmt.Sprintf("%s:%d", tc.File, tc.Line))
		}
	}
	sources := make([]string, len(opts.TestDirs))
	for i, d := range opts.TestDirs {
		sources[i] = filepath.ToSlash(d)
	}
	res.Gherkin = Render(cases, titles, sources)
	return res, nil
}

// TaggedTestNames reads every test source under dirs (the files Generate
// reads) and returns, in file and line order without duplicates, the names of
// the tests tagged with a criterion of one of useCases (UC-NNN ids). A test
// the grammar rejects has no readable tags, so a rejected test in a file that
// mentions one of the use cases ("UC-NNN.") is an error, never silently left
// out; rejected tests in other files are ignored.
func TaggedTestNames(dirs, useCases []string) ([]string, error) {
	if len(useCases) == 0 {
		return nil, nil // filterUseCases keeps everything for an empty filter
	}
	files, err := collectFiles(dirs, nil)
	if err != nil {
		return nil, err
	}
	var names []string
	var errs []error
	seen := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		cases, es, _ := ParseFile(filepath.ToSlash(f), src)
		if len(es) > 0 && slices.ContainsFunc(useCases, func(uc string) bool { return strings.Contains(string(src), uc+".") }) {
			errs = append(errs, es...)
		}
		for _, tc := range filterUseCases(cases, useCases) {
			if !seen[tc.Name] {
				seen[tc.Name] = true
				names = append(names, tc.Name)
			}
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return names, nil
}

// filterUseCases keeps tags of the selected use cases and drops tests left
// with none (untagged tests are dropped whenever a filter is set).
func filterUseCases(cases []TestCase, only []string) []TestCase {
	if len(only) == 0 {
		return cases
	}
	var out []TestCase
	for _, tc := range cases {
		var tags []string
		for _, id := range tc.Tags {
			if slices.Contains(only, useCaseOf(id)) {
				tags = append(tags, id)
			}
		}
		if len(tags) > 0 {
			tc.Tags = tags
			out = append(out, tc)
		}
	}
	return out
}

// collectFiles walks dirs for .java, .cs and .ts test sources (TypeScript:
// *.spec.ts and *.test.ts only), sorted, skipping excluded and
// node_modules paths.
func collectFiles(dirs, exclude []string) ([]string, error) {
	var files []string
	for _, d := range dirs {
		err := filepath.WalkDir(d, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(p)
			if e.IsDir() {
				if e.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if excluded(slash, exclude) || !isTestSource(e.Name()) {
				return nil
			}
			files = append(files, p)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("read test dir %s: %w", d, err)
		}
	}
	sort.Strings(files)
	return files, nil
}

func isTestSource(name string) bool {
	lower := strings.ToLower(name)
	switch filepath.Ext(lower) {
	case ".java", ".cs":
		return true
	case ".ts":
		return strings.HasSuffix(lower, ".spec.ts") || strings.HasSuffix(lower, ".test.ts")
	}
	return false
}

// excluded matches spec.yaml exclude globs against the slash path; a "**"
// segment matches zero or more directories.
func excluded(p string, globs []string) bool {
	segs := strings.Split(strings.TrimPrefix(p, "./"), "/")
	for _, g := range globs {
		if globMatch(strings.Split(strings.TrimPrefix(filepath.ToSlash(g), "./"), "/"), segs) {
			return true
		}
	}
	return false
}

func globMatch(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globMatch(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	if ok, err := path.Match(pat[0], segs[0]); err != nil || !ok {
		return false
	}
	return globMatch(pat[1:], segs[1:])
}
