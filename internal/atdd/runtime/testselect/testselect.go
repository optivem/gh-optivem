// Package testselect computes the minimal-but-safe set of acceptance and
// contract tests to run after a driver-adapter change.
//
// The selector traverses the ATDD-enforced layering Test → DSL → DriverPort
// → DriverAdapter statically: it diffs driver-adapter sources against a
// base ref, walks each changed adapter method back through its port to the
// DSL methods that call it, transitively across DSL helpers, and forward
// to the test methods that exercise those DSL methods. Output is grouped
// by suite (acceptance-api / acceptance-ui / contract-stub / contract-real)
// so the orchestrator can invoke `gh optivem test system --suite <s>
// --test <t>` for each.
//
// The package is pure file-system reads + regex; it does not shell out for
// anything except `git diff`. All other I/O is direct file reading. This
// keeps it cheap, deterministic, and easy to test against a fixture repo.
//
// See plans/20260504-130000-minimal-test-set-after-driver-adapter-change.md
// for the original motivation and design.
package testselect

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Selection groups every test method that should run for a given suite.
// Tests are sorted and deduplicated.
type Selection struct {
	Suite string   // "acceptance-api" | "acceptance-ui" | "contract-stub" | "contract-real"
	Tests []string // class-qualified test names (e.g. "RegisterCustomerPositiveTest.shouldRegister")
}

// ChangedMethod identifies one adapter method whose body or signature
// changed between the base ref and HEAD.
type ChangedMethod struct {
	File   string // repo-relative path
	Method string // method name only (no parameter list)
	Layer  string // "shop" | "external" — drives suite candidacy
	Lang   string // "java" | "dotnet" | "typescript" — drives regex choice
}

// Result is what Select returns. Selections is the curated list to run.
// Unmapped enumerates changed methods we could not trace forward to a
// test — the caller (typically the verification node) is expected to fall
// back to a full-suite run when this is non-empty. Changed lists every
// adapter method the diff touched (mapped or not), useful for surfacing
// "what the agent edited" before the run/approve prompt. Diagnostics
// carries the human-readable trace of decisions, useful for verbose
// output.
type Result struct {
	Selections  []Selection
	Unmapped    []ChangedMethod
	Changed     []ChangedMethod
	Diagnostics []string
}

// Select runs the full pipeline and returns Result. baseRef is the git ref
// to diff against — `HEAD` for "since the last commit" (the default WRITE
// flow uses HEAD because the agent's changes are uncommitted), or a branch
// merge-base for multi-commit phases.
//
// All file paths in the result are repo-relative (forward-slash separated).
func Select(repoRoot, baseRef string) (Result, error) {
	return SelectWithDeps(repoRoot, baseRef, nil)
}

// Deps lets tests inject a fake git runner without touching the filesystem
// for the diff step. fileReader is optional; when nil, files are read from
// disk under repoRoot.
type Deps struct {
	Git  func(ctx context.Context, repoRoot string, args ...string) ([]byte, error)
	Read func(repoRoot, relPath string) ([]byte, error)
	// Walk is called once per language; it should yield repo-relative paths
	// for every file under each of the supplied roots whose extension matches.
	// When nil, defaultWalk walks the on-disk repoRoot.
	Walk func(repoRoot string, roots []string, exts []string) ([]string, error)
}

// SelectWithDeps is the testable form of Select; production callers use
// Select.
func SelectWithDeps(repoRoot, baseRef string, deps *Deps) (Result, error) {
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
		return Result{}, err
	}
	if len(changed) == 0 {
		return Result{}, nil
	}

	res := Result{Changed: changed}

	// Per-language pass — each language has its own port/dsl/test roots,
	// regex shapes, and test-annotation conventions. Group changed methods
	// by language so we only walk each language tree once.
	byLang := map[string][]ChangedMethod{}
	for _, cm := range changed {
		byLang[cm.Lang] = append(byLang[cm.Lang], cm)
	}

	suiteTests := map[string]map[string]struct{}{} // suite → set of test names

	for lang, methods := range byLang {
		lay, ok := layouts[lang]
		if !ok {
			for _, cm := range methods {
				res.Unmapped = append(res.Unmapped, cm)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("unmapped: %s — no layout for language %q", cm.Method, lang))
			}
			continue
		}

		// 1. Resolve port methods for each changed adapter method.
		portFiles, err := deps.Walk(repoRoot, []string{lay.PortRoot(repoRoot)}, lay.SourceExts)
		if err != nil {
			return Result{}, fmt.Errorf("walk %s ports: %w", lang, err)
		}
		portFiles = filterPaths(portFiles, lay.PortMatch)
		dslFiles, err := deps.Walk(repoRoot, []string{lay.DSLRoot(repoRoot)}, lay.SourceExts)
		if err != nil {
			return Result{}, fmt.Errorf("walk %s dsl: %w", lang, err)
		}
		dslFiles = filterPaths(dslFiles, lay.DSLMatch)
		testFiles, err := deps.Walk(repoRoot, lay.TestRoots(repoRoot), lay.TestExts)
		if err != nil {
			return Result{}, fmt.Errorf("walk %s tests: %w", lang, err)
		}
		testFiles = filterPaths(testFiles, lay.TestMatch)
		// Adapter tree — needed to bridge a changed method that is not
		// itself port-backed (e.g. a Page Object helper) up to the
		// adapter method that fulfils the port. Reuses PortRoot since
		// adapters and ports share the language root in every layout.
		adapterFiles, err := deps.Walk(repoRoot, []string{lay.PortRoot(repoRoot)}, lay.SourceExts)
		if err != nil {
			return Result{}, fmt.Errorf("walk %s adapters: %w", lang, err)
		}
		adapterFiles = filterPaths(adapterFiles, lay.AdapterMatch)

		// 2. Build name → file caches once.
		portMethods := indexMethods(portFiles, lay, deps.Read)
		dslMethods := indexMethods(dslFiles, lay, deps.Read)
		testMethods := indexTestMethods(testFiles, lay, deps.Read)
		adapterMethods := indexMethods(adapterFiles, lay, deps.Read)

		// 2b. Class-qualification indices. Per-file parent-type lists for
		// adapters and declared-type lists for ports let us narrow port
		// candidates: a port method matches only when its declaring
		// interface is in some adapter's implements list. dslFilesByPortType
		// then narrows callersOf so a same-named method on an unrelated
		// port doesn't fan out across DSLs that don't actually inject it.
		adapterParentsByFile := map[string][]string{}
		for _, f := range adapterFiles {
			body, err := deps.Read("", f)
			if err != nil {
				continue
			}
			_, parents := lay.ClassExtractor(string(body))
			if len(parents) > 0 {
				adapterParentsByFile[f] = parents
			}
		}
		portDeclaredByFile := map[string][]string{}
		for _, f := range portFiles {
			body, err := deps.Read("", f)
			if err != nil {
				continue
			}
			declared, _ := lay.ClassExtractor(string(body))
			if len(declared) > 0 {
				portDeclaredByFile[f] = declared
			}
		}
		dslFilesByPortType := map[string]map[string]bool{}
		for _, f := range dslFiles {
			body, err := deps.Read("", f)
			if err != nil {
				continue
			}
			text := string(body)
			for _, types := range portDeclaredByFile {
				for _, portType := range types {
					if !strings.Contains(text, portType) {
						continue
					}
					if dslFilesByPortType[portType] == nil {
						dslFilesByPortType[portType] = map[string]bool{}
					}
					dslFilesByPortType[portType][f] = true
				}
			}
		}

		// 3. For each changed adapter method, find DSL methods reachable
		//    through any port that names that method.
		dslSet := map[string]bool{} // DSL method names that any changed adapter is reachable through

		for _, cm := range methods {
			effective := resolveAdapterToPortBackedMethods(
				cm.Method, portMethods, adapterFiles, adapterMethods, lay, deps.Read,
			)
			if len(effective) == 0 {
				res.Unmapped = append(res.Unmapped, cm)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("unmapped: %s — no port method named %q and no adapter caller resolves to one", cm.File, cm.Method))
				continue
			}
			if len(effective) == 1 && effective[0] == cm.Method {
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("port method %q matched %d port file(s)", cm.Method, len(portMethods.byName[cm.Method])))
			} else {
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("adapter method %q has no port; bridged via adapter callers to %v", cm.Method, effective))
			}

			anyDSL := false
			for _, effName := range effective {
				candidates := portMethods.byName[effName]
				ports := classQualifyPortCandidates(
					effName, candidates, adapterMethods, adapterParentsByFile, portDeclaredByFile,
				)
				if len(ports) > 0 && len(ports) < len(candidates) {
					res.Diagnostics = append(res.Diagnostics,
						fmt.Sprintf("port method %q narrowed by class qualification: %d → %d candidate(s)",
							effName, len(candidates), len(ports)))
				}
				if len(ports) == 0 {
					ports = candidates
				}
				for _, p := range ports {
					narrowedDSL := narrowDSLByPortType(dslFiles, dslFilesByPortType, portDeclaredByFile[p.File])
					callers := callersOf(p.Method, narrowedDSL, dslMethods, lay, deps.Read)
					if len(callers) == 0 {
						continue
					}
					anyDSL = true
					for _, c := range callers {
						dslSet[c] = true
					}
					res.Diagnostics = append(res.Diagnostics,
						fmt.Sprintf("port method %q → DSL methods %v", p.Method, callers))
				}
			}
			if !anyDSL {
				res.Unmapped = append(res.Unmapped, cm)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("unmapped: %s — port for %q has no DSL caller", cm.File, cm.Method))
			}
		}

		if len(dslSet) == 0 {
			continue
		}

		// 4. Transitive DSL closure — any DSL method that calls a method in
		//    dslSet is also affected.
		expanded := transitiveDSLClosure(dslSet, dslFiles, dslMethods, lay, deps.Read)
		for name := range expanded {
			if !dslSet[name] {
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("DSL helper %q transitively reaches a changed port", name))
			}
			dslSet[name] = true
		}

		// 5. For each affected DSL method, find tests that call it.
		testHits := map[string]testHit{} // testName → metadata
		for dslName := range dslSet {
			callers := callersOfTest(dslName, testFiles, testMethods, lay, deps.Read)
			for _, c := range callers {
				testHits[c.Name] = c
			}
			if len(callers) > 0 {
				names := make([]string, 0, len(callers))
				for _, c := range callers {
					names = append(names, c.Name)
				}
				sort.Strings(names)
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("DSL method %q → tests %v", dslName, names))
			}
		}

		// 6. Tag suites for each test hit.
		for _, h := range testHits {
			suites := tagSuites(h, methods[0].Layer, lay)
			if len(suites) == 0 {
				res.Diagnostics = append(res.Diagnostics,
					fmt.Sprintf("test %q has no @Channel and no contract tag — falling back per layer", h.Name))
				suites = fallbackSuitesForLayer(methods[0].Layer)
			}
			for _, s := range suites {
				if suiteTests[s] == nil {
					suiteTests[s] = map[string]struct{}{}
				}
				suiteTests[s][h.Name] = struct{}{}
			}
		}
	}

	// Materialise Selection list.
	suites := make([]string, 0, len(suiteTests))
	for s := range suiteTests {
		suites = append(suites, s)
	}
	sort.Strings(suites)
	for _, s := range suites {
		names := make([]string, 0, len(suiteTests[s]))
		for n := range suiteTests[s] {
			names = append(names, n)
		}
		sort.Strings(names)
		res.Selections = append(res.Selections, Selection{Suite: s, Tests: names})
	}

	return res, nil
}

// realGitDiff invokes the system `git` binary with --unified=0 and returns
// the raw diff output. Stderr is folded into the error on failure so the
// caller sees it.
func realGitDiff(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
	full := append([]string{"-C", repoRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s: %w (output: %s)",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

