package testselect

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// TracerSelection identifies the single test, DSL helper, port method, and
// changed adapter method that together form a tracer-bullet trail through
// the layering. The tracer answers the WRITE-phase question "did I break
// the layering I just edited?" — it is intentionally narrower than Select,
// which answers the regression-safety question "is the world still
// correct?".
type TracerSelection struct {
	Suite         string // "acceptance-api" | "acceptance-ui" | "contract-stub"
	Test          string // class-qualified test name (Class.method or describe.it)
	DSLMethod     string // DSL method that bridges Test → Port
	PortMethod    string // resolved port method (post-bridge)
	AdapterFile   string // changed file (repo-relative)
	AdapterMethod string // changed method
	Stage         string // "when" | "given" | "then" | "" — chosen DSL form
}

// TracerResult is what SelectTracer returns. Selections enumerates one
// tracer per (changed adapter method × channel). Unmapped collects changes
// that could not be staged (no channel inference, no port bridge, no DSL
// caller, or no test caller); the action layer's contract is to fall back
// to a full-suite run when this is non-empty. Changed mirrors the same
// field on Result so callers can diff "what was edited" the same way they
// do for Select.
type TracerResult struct {
	Selections  []TracerSelection
	Unmapped    []ChangedMethod
	Changed     []ChangedMethod
	Diagnostics []string
}

// SelectTracer is the production entry point. See SelectTracerWithDeps for
// the testable form.
func SelectTracer(repoRoot, baseRef string) (TracerResult, error) {
	return SelectTracerWithDeps(repoRoot, baseRef, nil)
}

// SelectTracerWithDeps mirrors SelectWithDeps's plumbing — same Deps shape,
// same per-language layout walk, same diff parser — but applies the
// tracer-bullet pick rule (channel from path, WHEN/GIVEN/THEN-ranked DSL
// caller, alphabetical first test) instead of the affected-set traversal.
func SelectTracerWithDeps(repoRoot, baseRef string, deps *Deps) (TracerResult, error) {
	if deps == nil {
		deps = &Deps{}
	}
	if deps.Git == nil {
		deps.Git = realGitDiff
	}
	if deps.Read == nil {
		deps.Read = readRepoFile
	}
	if deps.Walk == nil {
		deps.Walk = defaultWalk
	}

	changed, err := parseChangedMethods(repoRoot, baseRef, deps)
	if err != nil {
		return TracerResult{}, err
	}
	if len(changed) == 0 {
		return TracerResult{}, nil
	}

	res := TracerResult{Changed: changed}

	byLang := map[string][]ChangedMethod{}
	for _, cm := range changed {
		byLang[cm.Lang] = append(byLang[cm.Lang], cm)
	}

	for lang, methods := range byLang {
		lay, ok := layouts[lang]
		if !ok {
			for _, cm := range methods {
				res.Unmapped = append(res.Unmapped, cm)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("tracer: unmapped — no layout for language %q (method %q)", lang, cm.Method))
			}
			continue
		}

		portFiles, err := deps.Walk(repoRoot, []string{lay.PortRoot(repoRoot)}, lay.SourceExts)
		if err != nil {
			return TracerResult{}, fmt.Errorf("walk %s ports: %w", lang, err)
		}
		portFiles = filterPaths(portFiles, lay.PortMatch)
		dslFiles, err := deps.Walk(repoRoot, []string{lay.DSLRoot(repoRoot)}, lay.SourceExts)
		if err != nil {
			return TracerResult{}, fmt.Errorf("walk %s dsl: %w", lang, err)
		}
		dslFiles = filterPaths(dslFiles, lay.DSLMatch)
		testFiles, err := deps.Walk(repoRoot, lay.TestRoots(repoRoot), lay.TestExts)
		if err != nil {
			return TracerResult{}, fmt.Errorf("walk %s tests: %w", lang, err)
		}
		testFiles = filterPaths(testFiles, lay.TestMatch)
		adapterFiles, err := deps.Walk(repoRoot, []string{lay.PortRoot(repoRoot)}, lay.SourceExts)
		if err != nil {
			return TracerResult{}, fmt.Errorf("walk %s adapters: %w", lang, err)
		}
		adapterFiles = filterPaths(adapterFiles, lay.AdapterMatch)

		portMethods := indexMethods(portFiles, lay, deps.Read)
		dslMethods := indexMethods(dslFiles, lay, deps.Read)
		testMethods := indexTestMethods(testFiles, lay, deps.Read)
		adapterMethods := indexMethods(adapterFiles, lay, deps.Read)

		for _, cm := range methods {
			channel := tracerChannelForPath(cm.File)
			if channel == "" {
				res.Unmapped = append(res.Unmapped, cm)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("tracer: unmapped — %s has no /ui/, /api/, or /external/ segment", cm.File))
				continue
			}

			effective := resolveAdapterToPortBackedMethods(
				cm.Method, portMethods, adapterFiles, adapterMethods, lay, deps.Read,
			)
			if len(effective) == 0 {
				res.Unmapped = append(res.Unmapped, cm)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("tracer: unmapped — %s::%s has no port bridge", cm.File, cm.Method))
				continue
			}

			anySelection := false
			for _, effName := range effective {
				dslPick, stage, dslDiag := pickTracerDSL(effName, dslFiles, dslMethods, lay, deps.Read, channel)
				res.Diagnostics = append(res.Diagnostics, dslDiag...)
				if dslPick == "" {
					res.Diagnostics = append(res.Diagnostics,
						fmt.Sprintf("tracer: port %q has no DSL caller", effName))
					continue
				}
				testPick := pickTracerTest(dslPick, testFiles, testMethods, lay, deps.Read)
				if testPick == "" {
					res.Diagnostics = append(res.Diagnostics,
						fmt.Sprintf("tracer: DSL %q has no test caller", dslPick))
					continue
				}
				res.Selections = append(res.Selections, TracerSelection{
					Suite:         channel,
					Test:          testPick,
					DSLMethod:     dslPick,
					PortMethod:    effName,
					AdapterFile:   cm.File,
					AdapterMethod: cm.Method,
					Stage:         stage,
				})
				stageNote := stage
				if stageNote == "" {
					stageNote = "(no stage)"
				}
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("tracer: %s::%s → port %q → DSL %q [%s] → test %q (suite %s)",
						cm.File, cm.Method, effName, dslPick, stageNote, testPick, channel))
				anySelection = true
			}
			if !anySelection {
				res.Unmapped = append(res.Unmapped, cm)
			}
		}
	}

	sort.SliceStable(res.Selections, func(i, j int) bool {
		a, b := res.Selections[i], res.Selections[j]
		if a.Suite != b.Suite {
			return a.Suite < b.Suite
		}
		if a.AdapterFile != b.AdapterFile {
			return a.AdapterFile < b.AdapterFile
		}
		if a.AdapterMethod != b.AdapterMethod {
			return a.AdapterMethod < b.AdapterMethod
		}
		return a.Test < b.Test
	})

	return res, nil
}

// tracerChannelForPath maps a driver-adapter file path to a suite name.
// `external` wins over `ui` / `api` because the external tree is rooted
// under a directory that may also carry an `api` segment. Returns "" when
// no segment matches — caller treats this as "unmapped, fall back".
func tracerChannelForPath(path string) string {
	low := filepath.ToSlash(strings.ToLower(path))
	if strings.Contains(low, "/external/") || strings.Contains(low, "/driver.adapter/external/") {
		return "contract-stub"
	}
	if strings.Contains(low, "/ui/") {
		return "acceptance-ui"
	}
	if strings.Contains(low, "/api/") {
		return "acceptance-api"
	}
	return ""
}

// dslHit names a DSL method that calls a port method, plus the file it
// was declared in. The file is needed for the alphabetical tie-break
// inside a stage and for stage detection (path matching).
type dslHit struct {
	File   string
	Method string
}

// pickTracerDSL returns the DSL method to use as a tracer (and its stage),
// chosen from the DSL methods that call portMethod. Stage ordering is
// when > given > then; alphabetical by (file, method) within a stage.
//
// `contract-stub` channel relaxes the stage requirement: contract tests
// don't necessarily route through scenario-stage DSL helpers, so an
// alphabetical fallback applies.
//
// Returns ("", "", diag) when no DSL method calls the port. The diag slice
// is appended unconditionally — callers fold it into TracerResult.Diagnostics.
func pickTracerDSL(
	portMethod string,
	dslFiles []string,
	dslMethods *methodIndex,
	lay *layout,
	read func(string, string) ([]byte, error),
	channel string,
) (string, string, []string) {
	hits := callersOfWithFiles(portMethod, dslFiles, dslMethods, lay, read)
	if len(hits) == 0 {
		return "", "", nil
	}

	buckets := map[string][]dslHit{
		"when":  nil,
		"given": nil,
		"then":  nil,
	}
	var unstaged []dslHit
	for _, h := range hits {
		s := stageOfDSLPath(h.File, h.Method)
		if _, ok := buckets[s]; ok {
			buckets[s] = append(buckets[s], h)
		} else {
			unstaged = append(unstaged, h)
		}
	}

	var diag []string
	for _, s := range []string{"when", "given", "then"} {
		if len(buckets[s]) == 0 {
			continue
		}
		pick := pickAlphabetical(buckets[s])
		if losers := otherStageHits(buckets, s, unstaged); len(losers) > 0 {
			diag = append(diag,
				fmt.Sprintf("tracer: DSL stage %s preferred over %v", s, losers))
		}
		return pick.Method, s, diag
	}
	if len(unstaged) > 0 {
		pick := pickAlphabetical(unstaged)
		return pick.Method, "", diag
	}
	_ = channel
	return "", "", diag
}

// otherStageHits returns the names of DSL methods that lost the stage
// tie-break — useful for verbose diagnostics. Caller-side gating keeps
// the message concise.
func otherStageHits(buckets map[string][]dslHit, picked string, unstaged []dslHit) []string {
	var names []string
	for s, hs := range buckets {
		if s == picked {
			continue
		}
		for _, h := range hs {
			names = append(names, fmt.Sprintf("%s[%s]", h.Method, s))
		}
	}
	for _, h := range unstaged {
		names = append(names, fmt.Sprintf("%s[(none)]", h.Method))
	}
	sort.Strings(names)
	return names
}

// callersOfWithFiles is like callersOf but returns each hit's file as well,
// so the tracer can stage-rank by path and tie-break alphabetically by
// (file, method).
func callersOfWithFiles(
	methodName string,
	dslFiles []string,
	idx *methodIndex,
	lay *layout,
	read func(string, string) ([]byte, error),
) []dslHit {
	hits := map[string]dslHit{}
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
					if r.name == methodName {
						break // ignore self-recursion
					}
					key := f + "\x00" + r.name
					hits[key] = dslHit{File: f, Method: r.name}
					break
				}
			}
		}
	}
	out := make([]dslHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, h)
	}
	return out
}

// stageOfDSLPath classifies a DSL caller as `when`, `given`, `then`, or
// "" (no stage). The classifier checks (in order):
//
//   - path segment `/when/` / `/given/` / `/then/`
//   - method-name prefix `when*` / `given*` / `then*` (case-insensitive)
//   - file basename prefix `When*` / `Given*` / `Then*`
//
// All three signal channels are present in the academy's existing fixtures
// (Java `WhenPlaceOrder.java`, TypeScript `whenPlacingOrder()` — different
// languages, different conventions, same intent).
func stageOfDSLPath(file, method string) string {
	low := filepath.ToSlash(strings.ToLower(file))
	switch {
	case strings.Contains(low, "/when/"):
		return "when"
	case strings.Contains(low, "/given/"):
		return "given"
	case strings.Contains(low, "/then/"):
		return "then"
	}
	mlow := strings.ToLower(method)
	switch {
	case strings.HasPrefix(mlow, "when"):
		return "when"
	case strings.HasPrefix(mlow, "given"):
		return "given"
	case strings.HasPrefix(mlow, "then"):
		return "then"
	}
	base := strings.ToLower(filepath.Base(file))
	switch {
	case strings.HasPrefix(base, "when"):
		return "when"
	case strings.HasPrefix(base, "given"):
		return "given"
	case strings.HasPrefix(base, "then"):
		return "then"
	}
	return ""
}

func pickAlphabetical(hits []dslHit) dslHit {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Method < hits[j].Method
	})
	return hits[0]
}

// pickTracerTest returns the alphabetically first test method that calls
// dslMethod (Class.method for Java/.NET, describe.it for TS). Empty when
// no test calls the DSL.
func pickTracerTest(
	dslMethod string,
	testFiles []string,
	testIdx map[string][]testHit,
	lay *layout,
	read func(string, string) ([]byte, error),
) string {
	hits := callersOfTest(dslMethod, testFiles, testIdx, lay, read)
	if len(hits) == 0 {
		return ""
	}
	names := make([]string, 0, len(hits))
	for _, h := range hits {
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names[0]
}
