package runner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Stack keys for the per-stack use-case filter defaults. They match the
// gh-optivem.yaml `system-test.lang` enum (projectconfig.Lang*); the runner
// keeps its own copy so it stays independent of the project-config package.
const (
	stackJava       = "java"
	stackDotnet     = "dotnet"
	stackTypescript = "typescript"
)

// UseCaseSelection narrows a run to the acceptance tests tagged with the given
// use cases (plan 20261111-1023, Q1). Tests carry one tag per criterion
// (`UC-006.9`); a selection of UC-006 runs every UC-006.* test.
//
// The filter is built per stack and merged into each selected suite's Command:
//
//	dotnet     — `&(tag~UC-001.|tag~UC-003.)` injected into `--filter '...'`.
//	java       — JUnit tag expression over the criterion IDs (no prefix
//	             wildcard): ANDed into `-DincludeTags=<x>`, or appended as
//	             `-DincludeTags=UC-001.1|UC-001.2`.
//	typescript — Playwright native tags: `--grep '@UC-001\.|@UC-003\.'`, or an
//	             existing `--grep '<x>'` becomes a lookahead AND of both.
//
// The zero value is inactive: the run is unchanged (whole suite).
type UseCaseSelection struct {
	// Lang is the system-test stack (gh-optivem.yaml system-test.lang).
	Lang string
	// UseCases are the UC IDs (`UC-001`), deduped, in request order.
	UseCases []string
	// Criteria maps each UC ID to its criterion IDs (`UC-001.1`). Java only:
	// JUnit tag expressions have no prefix match, so a UC must be expanded.
	Criteria map[string][]string
}

// Active reports whether the selection narrows the run.
func (s UseCaseSelection) Active() bool { return len(s.UseCases) > 0 }

// useCaseTokenRE accepts a use-case ID (`UC-001`) or a criterion ID
// (`UC-001.3`); group 1 is the use-case prefix.
var useCaseTokenRE = regexp.MustCompile(`^(UC-\d+)(?:\.\d+)?$`)

// ParseUseCaseIDs reduces raw `--use-case` tokens to their use-case IDs:
// criterion IDs are cut to the UC prefix, blanks are dropped, and duplicates
// are removed keeping first-seen order. An empty result means "whole suite".
// A token that is neither form is an error naming it.
func ParseUseCaseIDs(raw []string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	var bad []string
	for _, r := range raw {
		tok := strings.TrimSpace(r)
		if tok == "" {
			continue
		}
		m := useCaseTokenRE.FindStringSubmatch(tok)
		if m == nil {
			bad = append(bad, tok)
			continue
		}
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf("invalid --use-case value(s): %s — expected a use-case ID (UC-001) or criterion ID (UC-001.3)", strings.Join(bad, ", "))
	}
	return out, nil
}

// ResolveUseCaseSelection builds the selection for raw `--use-case` tokens.
// Every UC must be declared in the project spec: declared maps each spec
// use-case ID (from spec.yaml's specDir) to its spec criterion IDs, and a UC
// absent from it is an error naming it. Plain data keeps internal/build free of
// the spec package (TestBuildImportsKernelOnly). For Java, each UC is
// expanded to its criterion IDs: the union of the spec's criteria and the
// `"UC-NNN.M"` tag literals in the Java test sources under testsDir. The
// sources are needed because a done use case keeps no criteria in the spec
// (its tests are the spec), so a spec-only expansion would select nothing — or,
// worse, only part of the tests. A Java UC with no criterion ID anywhere is an
// error rather than a silently empty filter.
func ResolveUseCaseSelection(raw []string, lang string, declared map[string][]string, testsDir string) (UseCaseSelection, error) {
	ids, err := ParseUseCaseIDs(raw)
	if err != nil || len(ids) == 0 {
		return UseCaseSelection{}, err
	}
	switch lang {
	case stackJava, stackDotnet, stackTypescript:
	default:
		return UseCaseSelection{}, fmt.Errorf("--use-case: no use-case filter for system-test.lang %q (supported: java, dotnet, typescript)", lang)
	}
	var unknown []string
	for _, id := range ids {
		if _, ok := declared[id]; !ok {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		return UseCaseSelection{}, fmt.Errorf("--use-case: use case(s) not in the project spec: %s — check the ID against the spec dir named by spec.yaml", strings.Join(unknown, ", "))
	}
	sel := UseCaseSelection{Lang: lang, UseCases: ids}
	if lang != stackJava {
		return sel, nil
	}
	tagged, err := scanJavaCriterionTags(testsDir)
	if err != nil {
		return UseCaseSelection{}, err
	}
	sel.Criteria = map[string][]string{}
	var empty []string
	for _, id := range ids {
		set := map[string]bool{}
		for _, c := range declared[id] {
			set[c] = true
		}
		for c := range tagged {
			if strings.HasPrefix(c, id+".") {
				set[c] = true
			}
		}
		if len(set) == 0 {
			empty = append(empty, id)
			continue
		}
		sel.Criteria[id] = sortedCriterionIDs(set)
	}
	if len(empty) > 0 {
		return UseCaseSelection{}, fmt.Errorf("--use-case: no criterion IDs for %s in the spec or as @Tag(\"UC-NNN.M\") in the Java tests under %s — a JUnit tag filter needs them; run without --use-case", strings.Join(empty, ", "), testsDir)
	}
	return sel, nil
}

// javaCriterionTagRE matches a criterion ID string literal, the form
// `@Tag("UC-006.9")` uses.
var javaCriterionTagRE = regexp.MustCompile(`"(UC-\d+\.\d+)"`)

// scanJavaSkipDirs are build/tooling output directories never holding sources.
var scanJavaSkipDirs = map[string]bool{"build": true, ".gradle": true, ".git": true, "node_modules": true, "out": true, "bin": true}

// scanJavaCriterionTags returns every criterion ID string literal in the .java
// files under dir. A missing dir yields the empty set.
func scanJavaCriterionTags(dir string) (map[string]bool, error) {
	found := map[string]bool{}
	if dir == "" {
		return found, nil
	}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == dir {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() {
			if p != dir && scanJavaSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".java") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range javaCriterionTagRE.FindAllStringSubmatch(string(data), -1) {
			found[m[1]] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Java tests under %s for criterion tags: %w", dir, err)
	}
	return found, nil
}

// sortedCriterionIDs orders criterion IDs by their numeric suffix so the
// filter is deterministic (UC-001.2 before UC-001.10).
func sortedCriterionIDs(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return criterionOrdinal(out[i]) < criterionOrdinal(out[j])
	})
	return out
}

func criterionOrdinal(id string) int {
	_, n, _ := strings.Cut(id, ".")
	v, _ := strconv.Atoi(n)
	return v
}

var (
	// dotnetFilterRE matches `--filter '<expr>'` (the single-quoted form the
	// suites use, same as filterInjectionRE); group 1 is the expression.
	dotnetFilterRE = regexp.MustCompile(`--filter\s+'([^']*)'`)
	// javaIncludeTagsRE matches a `-DincludeTags=<x>` token, optionally quoted.
	javaIncludeTagsRE = regexp.MustCompile(`(^|\s)(['"]?)-DincludeTags=([^\s'"]*)(['"]?)`)
	// tsGrepRE matches `--grep '<x>'` / `--grep "<x>"` / `--grep <x>` /
	// `--grep=<x>` but not `--grep-invert` (the flag must be followed by
	// whitespace or `=`).
	tsGrepRE = regexp.MustCompile(`(^|\s)--grep(?:\s+|=)(?:'([^']*)'|"([^"]*)"|([^\s'"]\S*))`)
)

// applyUseCaseFilter merges the selection's per-stack filter into command.
// An inactive selection returns command unchanged. A command the filter
// cannot be merged into safely is an error, never a silently unfiltered run.
func applyUseCaseFilter(command string, sel UseCaseSelection) (string, error) {
	if !sel.Active() {
		return command, nil
	}
	switch sel.Lang {
	case stackDotnet:
		return applyDotnetUseCaseFilter(command, sel.UseCases)
	case stackJava:
		return applyJavaUseCaseFilter(command, sel)
	case stackTypescript:
		return applyTypescriptUseCaseFilter(command, sel.UseCases), nil
	default:
		return "", fmt.Errorf("--use-case: no use-case filter for system-test.lang %q (supported: java, dotnet, typescript)", sel.Lang)
	}
}

func applyDotnetUseCaseFilter(command string, ids []string) (string, error) {
	terms := make([]string, len(ids))
	for i, id := range ids {
		terms[i] = "tag~" + id + "."
	}
	frag := "(" + strings.Join(terms, "|") + ")"
	loc := dotnetFilterRE.FindStringSubmatchIndex(command)
	if loc == nil {
		if strings.Contains(command, "--filter") {
			return "", fmt.Errorf("--use-case: cannot merge the use-case filter into %q — only a single-quoted --filter '...' is supported", command)
		}
		return command + " --filter '" + frag + "'", nil
	}
	existing := command[loc[2]:loc[3]]
	// `&` binds tighter than `|` in dotnet's filter grammar; parenthesise an
	// existing top-level OR so the use-case term ANDs with all of it.
	if strings.Contains(existing, "|") {
		existing = "(" + existing + ")"
	}
	return command[:loc[0]] + "--filter '" + existing + "&" + frag + "'" + command[loc[1]:], nil
}

func applyJavaUseCaseFilter(command string, sel UseCaseSelection) (string, error) {
	var ids []string
	for _, uc := range sel.UseCases {
		ids = append(ids, sel.Criteria[uc]...)
	}
	if len(ids) == 0 {
		return "", fmt.Errorf("--use-case: no criterion IDs resolved for %s", strings.Join(sel.UseCases, ", "))
	}
	expr := strings.Join(ids, "|")
	loc := javaIncludeTagsRE.FindStringSubmatchIndex(command)
	if loc == nil {
		return command + " -DincludeTags=" + expr, nil
	}
	existing := command[loc[6]:loc[7]]
	// Gradle splits includeTags on `,` (each part ORed); fold a list into one
	// expression so the AND applies to the whole include set.
	if strings.Contains(existing, ",") {
		existing = "(" + strings.ReplaceAll(existing, ",", "|") + ")"
	}
	merged := command[loc[2]:loc[3]] + command[loc[4]:loc[5]] + "-DincludeTags=" + existing + "&(" + expr + ")" + command[loc[8]:loc[9]]
	return command[:loc[0]] + merged + command[loc[1]:], nil
}

func applyTypescriptUseCaseFilter(command string, ids []string) string {
	alts := make([]string, len(ids))
	for i, id := range ids {
		alts[i] = "@" + id + `\.`
	}
	alt := strings.Join(alts, "|")
	if !tsGrepRE.MatchString(command) {
		return command + " --grep '" + alt + "'"
	}
	return tsGrepRE.ReplaceAllStringFunc(command, func(m string) string {
		sub := tsGrepRE.FindStringSubmatch(m)
		existing := sub[2] + sub[3] + sub[4]
		return sub[1] + "--grep '^(?=.*" + existing + ")(?=.*(" + alt + "))'"
	})
}
