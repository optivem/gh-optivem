# Plan (deferred): revive the deterministic disable / enable test-marker implementation

**Date:** 2026-05-20
**Status:** Deferred reference. Do not execute until/unless the agent-driven approach proves untenable.

## Why this plan exists

On 2026-05-20, [plans/20260520-0001-switch-disable-enable-tests-to-agents.md](../20260520-0001-switch-disable-enable-tests-to-agents.md) replaced the deterministic Go disable/enable machinery with two Haiku-driven agent prompts (`prompts/atdd/disable-tests.md`, `prompts/atdd/enable-tests.md`) and `type: user_task / agent: <name>` BPMN nodes. The deterministic code was good code with passing tests; the pivot was about per-language extensibility (each new language was O(N) cost in `Pattern` + shell script + fixtures + edge cases) and the missing import-add behavior on the disable side.

This plan archives the deterministic implementation as fenced code blocks so a future revive plan can pick it up if the agent approach proves too costly, too slow, or too non-deterministic in production.

## When to revive

Revive this plan if:

- The Haiku agents repeatedly produce malformed reason strings (e.g. wrong separator, lowercase loop, missing import) that the downstream startsWith filter can't re-enable cleanly.
- Cost per loop iteration (across many tickets per day) makes the cumulative Haiku spend material.
- The agent latency adds enough delay to AT-cycle wall-clock time that it becomes the bottleneck.
- A specific language family (e.g. embedded systems with very strict syntax requirements) shows the agent unable to match the language-equivalents row reliably.

Do NOT revive simply because the agent occasionally fumbles an edge case — that's the expected cost of the trade. Revive only when the cost is structural and persistent.

## What was deleted, what to restore

| Asset | Path before deletion | Notes |
|---|---|---|
| `RemoveDisabledMarkers` + `Pattern` machinery | `internal/atdd/runtime/release/release.go` | Coexisted with `Commit`, `CloseIssue`, `Confirmer` — those were not deleted. Restore only the marker-removal portion. |
| Per-language `Pattern` defaults | `internal/atdd/runtime/release/patterns.go` | Entire file deleted. |
| Marker-removal tests | `internal/atdd/runtime/release/release_test.go` | Only the marker-removal tests were deleted; the Commit / CloseIssue / InteractiveConfirmer tests survived. |
| Testdata fixtures | `internal/atdd/runtime/release/testdata/{java,dotnet,typescript}/` | Six fixtures per language (input + expected pairs). |
| `disableChangeDriven` action | `internal/atdd/runtime/actions/bindings.go` | Shimmed out to a never-authored `./disable-test.sh`. |
| `enableChangeDriven` action | `internal/atdd/runtime/actions/bindings.go` | Shimmed out to a never-authored `./enable-test.sh`. |
| `assembleDisableReason` helper | `internal/atdd/runtime/actions/bindings.go` | Built and validated the reason string. |
| `allowedDisableLoops` / `allowedDisablePhases` | `internal/atdd/runtime/actions/bindings.go` | Validation domains. |
| `CtxKeyLanguage`, `CtxKeyTicketID`, `CtxKeyLoop`, `CtxKeyPhase`, `CtxKeyPrevPhase`, `CtxKeyDisableTargets` | `internal/atdd/runtime/actions/bindings.go` | Context keys used by the deleted actions. |
| Action registry entries | `internal/atdd/runtime/actions/bindings.go:158-159` | `r.Register("disable_change_driven", ...)` and `r.Register("enable_change_driven", ...)`. |
| Registry test expectations | `internal/atdd/runtime/actions/bindings_test.go:865-866` | `"disable_change_driven"` and `"enable_change_driven"` entries. |
| Doctrine docs | `internal/assets/global/docs/atdd/process/change/behavior/{disable,enable}-tests.md` | Inlined into the new agent prompts; standalone docs deleted. |
| BPMN nodes | `internal/atdd/runtime/statemachine/process-flow.yaml:427-430` (ENABLE_TESTS), `:919-922` (DISABLE) | Switched from `type: service_task / action: <name>` to `type: user_task / agent: <name>`. |

## How to revive

1. **Restore `release.go` marker-removal block.** Append the code from [§ release.go marker-removal block](#release-go-marker-removal-block) below to the existing `release.go`. Re-add the deleted imports (`io/fs`, `path/filepath`, `regexp`) if no longer present.
2. **Restore `patterns.go`.** Create `internal/atdd/runtime/release/patterns.go` from [§ patterns.go](#patterns-go) below.
3. **Restore the testdata fixtures.** Create the 18 fixture files from [§ testdata fixtures](#testdata-fixtures) below.
4. **Restore the tests.** Append the test functions from [§ release_test.go marker-removal tests](#release_test-go-marker-removal-tests) below to the existing `release_test.go`. Add the `bytes` import to that file if needed.
5. **Restore the bindings.** Append the code from [§ bindings.go disable/enable functions](#bindings-go-disable-enable-functions) below to `bindings.go`. Re-add the const block entries from [§ bindings.go context keys](#bindings-go-context-keys) above (or restore them within the existing const block). Re-add the action registrations from [§ bindings.go registry entries](#bindings-go-registry-entries).
6. **Restore the registry test.** Add `"disable_change_driven"` and `"enable_change_driven"` back to the want list in `bindings_test.go` around line 865.
7. **Flip the BPMN nodes back.** Edit `process-flow.yaml`:
   - ENABLE_TESTS: `type: user_task / agent: enable-tests` → `type: service_task / action: enable_change_driven`.
   - DISABLE: `type: user_task / agent: disable-tests` → `type: service_task / action: disable_change_driven`.
8. **Choose what to do with the agent prompts.** Either delete `prompts/atdd/{disable,enable}-tests.md` outright, or keep them as a fallback for languages the deterministic patterns don't yet cover (in which case the BPMN keeps the `agent:` form for those languages and uses `action:` for the covered ones — a hybrid that warrants its own design discussion).
9. **Choose what to do with the doctrine docs.** The doctrine content lives inside the agent prompts after the inline pass. If reviving deterministic-only, copy the doctrine text back to `internal/assets/global/docs/atdd/process/change/behavior/{disable,enable}-tests.md` from [§ doctrine docs](#doctrine-docs) below — or leave it inlined; the text is the same either way.
10. **Run tests.** `go test ./... -p 2` (Windows hazard rule) — expect green.

## Open questions for the revive author

- **Why is this being revived?** Cost? Non-determinism? Per-language coverage gap? The revive direction differs:
  - Cost-driven: full revert to deterministic-only.
  - Non-determinism-driven: full revert, or surgical reverts (e.g. deterministic for the format-strict reason string, agent for the import handling).
  - Coverage-driven: hybrid where deterministic patterns cover java/csharp/typescript and the agent serves new languages until a deterministic pattern is added.
- **Does `disable-test.sh` finally get authored?** The original deterministic disable code shelled out to a `disable-test.sh` script that was never written. If revived as-is, that script must be authored — or `disableChangeDriven` rewritten to call into `release.AddDisabledMarkers` (a new sibling of `RemoveDisabledMarkers` that this archive does not contain — it would have to be written from the doctrine spec).
- **Has the per-language coverage grown?** If new languages (Python, Go, Rust, ...) joined the academy stack since 2026-05-20, the `Pattern` slice in `patterns.go` needs new entries. The agent path absorbed those for free; the deterministic path does not.

---

## § release.go marker-removal block

The pre-deletion `release.go` carried `Commit`, `CloseIssue`, `Confirmer`, and the marker-removal block all in one file. Only the marker-removal block was deleted. Restore the block below by appending it to the surviving file (or by reverting the deletion commit).

```go
// RemoveDisabledMarkers walks a set of roots, applies one regex per
// language, and rewrites or deletes lines that match. Each language has
// a built-in default Pattern; callers can override the full set via
// RemoveOptions.Patterns.

// RemoveOptions controls a RemoveDisabledMarkers run.
type RemoveOptions struct {
	// Roots are directories to walk recursively, e.g.
	// ["system-test/java", "system-test/dotnet", "system-test/typescript"].
	// Each root is walked independently; missing roots are skipped (not an
	// error) so callers can pass all three language roots unconditionally.
	Roots []string

	// Patterns optionally overrides the built-in default patterns. Nil
	// (the common case) uses DefaultPatterns(). Callers passing a custom
	// slice replace the defaults entirely — there is no merge.
	Patterns []Pattern
}

// Pattern describes how to find and remove a disabled-marker in one
// language. A file is matched when its path satisfies Glob; once matched,
// every line matching Line is removed from the file. If LineRewrite is
// non-nil, matched lines are passed through it instead of removed (used by
// the .NET `[Fact(Skip = "…")]` rewrite, which keeps the attribute and
// drops only the Skip parameter).
//
// Imports is optional: if non-nil and the file has no remaining matches
// after removal, lines matching Imports are also removed (used to clean up
// `import org.junit.jupiter.api.Disabled;` once the last `@Disabled` is
// gone).
type Pattern struct {
	Name        string
	Glob        string
	Line        *regexp.Regexp
	LineRewrite func(string) string
	Imports     *regexp.Regexp
}

// FileChange records the per-file outcome of a RemoveDisabledMarkers pass.
type FileChange struct {
	Path         string
	LinesRemoved []int    // 1-indexed line numbers fully removed
	LinesEdited  []int    // 1-indexed line numbers rewritten in place (e.g. .NET Skip drop)
	PatternName  string
}

// Changes is the return type of RemoveDisabledMarkers.
type Changes struct {
	Files []FileChange
}

// RemoveDisabledMarkers walks each root, matches files against each
// pattern's glob, and removes (or rewrites) marker lines per the pattern.
// Returns a Changes slice describing every file modified.
//
// Ordering: roots are walked in the order given; within each root files
// are visited in lexical order (the default of filepath.WalkDir). Patterns
// are applied in the order given; within a single file every pattern that
// matches the glob runs, in order, so a file matching multiple patterns
// (rare) accumulates the union of edits.
func RemoveDisabledMarkers(ctx context.Context, opts RemoveOptions) (Changes, error) {
	patterns := opts.Patterns
	if patterns == nil {
		patterns = DefaultPatterns()
	}
	var out Changes
	for _, root := range opts.Roots {
		// Missing root is a soft skip — callers commonly pass all three
		// language roots even when only one language is in use.
		info, err := os.Stat(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Changes{}, fmt.Errorf("release: stat %s: %w", root, err)
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			for _, p := range patterns {
				ok, err := matchGlob(p.Glob, path)
				if err != nil {
					return fmt.Errorf("release: glob %q: %w", p.Glob, err)
				}
				if !ok {
					continue
				}
				change, err := applyPattern(path, p)
				if err != nil {
					return err
				}
				if change != nil {
					out.Files = append(out.Files, *change)
				}
			}
			return nil
		})
		if err != nil {
			return Changes{}, err
		}
		_ = ctx // ctx is reserved for future cancellation; walkDir is fast and local.
	}
	return out, nil
}

// applyPattern reads path, removes/rewrites marker lines per p, and writes
// the result back when there are changes. Returns nil when the file has no
// matches.
func applyPattern(path string, p Pattern) (*FileChange, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("release: read %s: %w", path, err)
	}
	// Preserve whether the file ended with a newline. After splitting on
	// '\n' a trailing newline produces a final empty element — track it so
	// we can re-emit identically on write-back.
	hadTrailingNewline := len(data) > 0 && data[len(data)-1] == '\n'
	body := string(data)
	if hadTrailingNewline {
		body = body[:len(body)-1]
	}
	lines := strings.Split(body, "\n")
	var (
		removed     []int
		edited      []int
		kept        = make([]string, 0, len(lines))
		matchCount  int
	)
	for i, line := range lines {
		if p.Line.MatchString(line) {
			matchCount++
			if p.LineRewrite != nil {
				rewritten := p.LineRewrite(line)
				if rewritten == line {
					// The pattern matched but the rewriter returned the
					// line unchanged (e.g. Skip parameter not actually
					// present). Treat as a no-op for this line.
					kept = append(kept, line)
					continue
				}
				if rewritten == "" {
					removed = append(removed, i+1)
					continue
				}
				edited = append(edited, i+1)
				kept = append(kept, rewritten)
				continue
			}
			removed = append(removed, i+1)
			continue
		}
		kept = append(kept, line)
	}

	// Import-line cleanup: only when at least one marker was removed AND
	// no Line matches remain in `kept`. matchCount > 0 above already means
	// the file had markers; we re-check kept to handle patterns that leave
	// markers behind (rare, but defensive).
	if p.Imports != nil && matchCount > 0 {
		stillHasMarker := false
		for _, line := range kept {
			if p.Line.MatchString(line) {
				stillHasMarker = true
				break
			}
		}
		if !stillHasMarker {
			cleaned := make([]string, 0, len(kept))
			for i, line := range kept {
				if p.Imports.MatchString(line) {
					removed = append(removed, originalLineNumber(lines, line, i, removed))
					continue
				}
				cleaned = append(cleaned, line)
			}
			kept = cleaned
		}
	}

	if len(removed) == 0 && len(edited) == 0 {
		return nil, nil
	}

	joined := strings.Join(kept, "\n")
	if hadTrailingNewline && len(kept) > 0 {
		joined += "\n"
	}
	if err := os.WriteFile(path, []byte(joined), 0o644); err != nil {
		return nil, fmt.Errorf("release: write %s: %w", path, err)
	}

	return &FileChange{
		Path:         path,
		LinesRemoved: removed,
		LinesEdited:  edited,
		PatternName:  p.Name,
	}, nil
}

// originalLineNumber best-effort maps a kept-array index back to the
// original-file line number. Imports cleanup is rare and the exact line
// number is informational, so this scans the original `lines` for the same
// text. If multiple originals match (unlikely for an import line), the
// first un-removed index wins.
func originalLineNumber(orig []string, target string, keptIdx int, alreadyRemoved []int) int {
	skip := make(map[int]bool, len(alreadyRemoved))
	for _, n := range alreadyRemoved {
		skip[n] = true
	}
	for i, line := range orig {
		if skip[i+1] {
			continue
		}
		if line == target {
			return i + 1
		}
	}
	return keptIdx + 1
}

// matchGlob applies a glob in the form "**/*.java" or "*.cs" against a
// file path. The "**" segment matches any depth; everything else is
// delegated to filepath.Match on the basename.
//
// Supported forms (sufficient for default patterns):
//
//	"**/*.java"    → any .java file at any depth
//	"**/*.cs"      → any .cs file at any depth
//	"**/*.spec.ts" → any .spec.ts file at any depth
//	"*.go"         → any .go file (basename only)
//
// Anything more elaborate (literal directory prefixes, multi-segment
// glob bodies) would require a real glob library; we deliberately stay
// simple and let callers pass exact globs.
func matchGlob(glob, path string) (bool, error) {
	// Normalize separators so windows paths work with forward-slash globs.
	p := filepath.ToSlash(path)
	g := strings.TrimSpace(glob)
	if strings.HasPrefix(g, "**/") {
		g = g[len("**/"):]
		base := filepath.Base(p)
		return filepath.Match(g, base)
	}
	return filepath.Match(g, filepath.Base(p))
}
```

Imports needed by this block: `context`, `errors`, `fmt`, `io/fs`, `os`, `path/filepath`, `regexp`, `strings`. The surviving `release.go` already has `context`, `errors`, `fmt`, `os`, `strings`; add `io/fs`, `path/filepath`, `regexp`.

## § patterns.go

`internal/atdd/runtime/release/patterns.go` (entire file):

```go
package release

import (
	"regexp"
	"strings"
)

// DefaultPatterns returns the built-in marker patterns for Java, .NET, and
// TypeScript, anchored to the canonical forms in
// shop/docs/atdd/code/language-equivalents.md:
//
//   - Java:       `@Disabled("reason")` standalone line, plus `import org.junit.jupiter.api.Disabled;`
//                 cleanup once the last `@Disabled` is gone.
//   - .NET (C#):  `[Fact(Skip = "reason")]` / `[Theory(Skip = "reason")]` — keep the attribute,
//                 drop just the `Skip = "…"` parameter (and the resulting empty `()` or stray
//                 leading `, `). The standalone form `[Skip("…")]` is removed entirely.
//   - TypeScript: `test.skip(true, "reason")` standalone line — removed entirely.
//
// Callers that need to test a non-default form should construct their own
// Pattern slice and pass it via RemoveOptions.Patterns.
func DefaultPatterns() []Pattern {
	return []Pattern{
		javaDisabledPattern(),
		dotnetSkipParamPattern(),
		dotnetSkipAttributePattern(),
		typescriptTestSkipPattern(),
	}
}

// -------------------------------------------------------------------------
// Java
// -------------------------------------------------------------------------

// javaDisabledPattern matches a whole line consisting of `@Disabled` with
// optional `("…")` argument and any surrounding whitespace. The whole line
// is removed; if the file no longer has any `@Disabled` references, the
// JUnit Disabled import is also removed.
//
// Regex: `^\s*@Disabled(\(.*\))?\s*$`
func javaDisabledPattern() Pattern {
	return Pattern{
		Name:    "java-disabled",
		Glob:    "**/*.java",
		Line:    regexp.MustCompile(`^\s*@Disabled(\(.*\))?\s*$`),
		Imports: regexp.MustCompile(`^\s*import\s+org\.junit\.jupiter\.api\.Disabled\s*;\s*$`),
	}
}

// -------------------------------------------------------------------------
// .NET — Skip parameter on existing [Fact(...)] / [Theory(...)]
// -------------------------------------------------------------------------

// dotnetSkipParamRe matches a line whose only attribute is [Fact(...)] or
// [Theory(...)] and whose argument list contains a `Skip = "…"` parameter.
// The attribute is preserved; the Skip parameter is dropped. Other params
// (e.g. `DisplayName = "…"`) are kept.
//
// Regex: `^(?P<lead>\s*)\[(?P<attr>Fact|Theory)\((?P<args>.*Skip\s*=\s*"[^"]*".*)\)\](?P<trail>\s*)$`
var dotnetSkipParamRe = regexp.MustCompile(
	`^(?P<lead>\s*)\[(?P<attr>Fact|Theory)\((?P<args>.*Skip\s*=\s*"[^"]*".*)\)\](?P<trail>\s*)$`,
)

// dotnetSkipParamRewrite returns a function that, given a matched line,
// rewrites it to drop the `Skip = "…"` named argument while preserving the
// rest of the attribute. Rules:
//
//   - `[Fact(Skip = "x")]`                     → `[Fact]`
//   - `[Fact(Skip = "x", DisplayName = "y")]`  → `[Fact(DisplayName = "y")]`
//   - `[Fact(DisplayName = "y", Skip = "x")]`  → `[Fact(DisplayName = "y")]`
//
// We strip the trailing `, ` left over by removing a non-final argument or
// the leading `, ` left over by removing a non-first argument, and collapse
// `()` (no remaining args) to no parens at all.
func dotnetSkipParamPattern() Pattern {
	skipKV := regexp.MustCompile(`Skip\s*=\s*"[^"]*"`)
	commaCleanup := regexp.MustCompile(`,\s*,`)
	return Pattern{
		Name: "dotnet-fact-theory-skip-param",
		Glob: "**/*.cs",
		Line: dotnetSkipParamRe,
		LineRewrite: func(line string) string {
			m := dotnetSkipParamRe.FindStringSubmatch(line)
			if m == nil {
				return line
			}
			lead := m[dotnetSkipParamRe.SubexpIndex("lead")]
			attr := m[dotnetSkipParamRe.SubexpIndex("attr")]
			args := m[dotnetSkipParamRe.SubexpIndex("args")]
			trail := m[dotnetSkipParamRe.SubexpIndex("trail")]

			args = skipKV.ReplaceAllString(args, "")
			// Collapse `,  ,` → `,` left by mid-list removal.
			for commaCleanup.MatchString(args) {
				args = commaCleanup.ReplaceAllString(args, ",")
			}
			// Trim leading `, ` (Skip was first arg) and trailing `, `
			// (Skip was last arg of two+).
			args = strings.TrimSpace(args)
			args = strings.TrimPrefix(args, ",")
			args = strings.TrimSuffix(args, ",")
			args = strings.TrimSpace(args)

			if args == "" {
				return lead + "[" + attr + "]" + trail
			}
			return lead + "[" + attr + "(" + args + ")]" + trail
		},
	}
}

// -------------------------------------------------------------------------
// .NET — Standalone [Skip("…")] attribute
// -------------------------------------------------------------------------

// dotnetSkipAttributePattern matches a whole-line `[Skip("…")]` (xunit v3
// standalone attribute form). The line is removed entirely.
func dotnetSkipAttributePattern() Pattern {
	return Pattern{
		Name: "dotnet-skip-attribute",
		Glob: "**/*.cs",
		Line: regexp.MustCompile(`^\s*\[\s*Skip\s*\(\s*"[^"]*"\s*\)\s*\]\s*$`),
	}
}

// -------------------------------------------------------------------------
// TypeScript — test.skip(true, "…")
// -------------------------------------------------------------------------

// typescriptTestSkipPattern matches a standalone-statement line of the form
// `test.skip(true, "reason");` (semicolon optional) per the shop spec
// (docs/atdd/code/language-equivalents.md). The whole line is removed.
//
// Regex: `^\s*test\.skip\s*\(\s*true\s*,\s*"[^"]*"\s*\)\s*;?\s*$`
func typescriptTestSkipPattern() Pattern {
	return Pattern{
		Name: "typescript-test-skip",
		Glob: "**/*.spec.ts",
		Line: regexp.MustCompile(`^\s*test\.skip\s*\(\s*true\s*,\s*"[^"]*"\s*\)\s*;?\s*$`),
	}
}
```

## § release_test.go marker-removal tests

Append these to the surviving `release_test.go`. The Commit / CloseIssue / InteractiveConfirmer tests were not deleted; only the block below was.

```go
// -------------------------------------------------------------------------
// Golden-file fixture tests
// -------------------------------------------------------------------------

// fixtureCase pairs an input file (under testdata/) with its expected
// output. The Name is used as the t.Run subtest name.
type fixtureCase struct {
	Name     string
	Input    string // path under testdata/
	Expected string // path under testdata/
}

func TestRemoveDisabledMarkers_Java(t *testing.T) {
	cases := []fixtureCase{
		{
			Name:     "single_disabled_with_import_cleanup",
			Input:    "java/disabled-with-import-input.java",
			Expected: "java/disabled-with-import-expected.java",
		},
		{
			Name:     "multiple_disabled_markers",
			Input:    "java/multi-disabled-input.java",
			Expected: "java/multi-disabled-expected.java",
		},
		{
			Name:     "no_marker_is_noop",
			Input:    "java/no-marker-input.java",
			Expected: "java/no-marker-expected.java",
		},
	}
	runFixtureCases(t, cases)
}

func TestRemoveDisabledMarkers_DotNet(t *testing.T) {
	cases := []fixtureCase{
		{
			Name:     "fact_theory_skip_param_drops_skip_keeps_attribute",
			Input:    "dotnet/fact-skip-input.cs",
			Expected: "dotnet/fact-skip-expected.cs",
		},
		{
			Name:     "standalone_skip_attribute_removed",
			Input:    "dotnet/skip-attribute-input.cs",
			Expected: "dotnet/skip-attribute-expected.cs",
		},
		{
			Name:     "no_marker_is_noop",
			Input:    "dotnet/no-marker-input.cs",
			Expected: "dotnet/no-marker-expected.cs",
		},
	}
	runFixtureCases(t, cases)
}

func TestRemoveDisabledMarkers_TypeScript(t *testing.T) {
	cases := []fixtureCase{
		{
			Name:     "single_test_skip",
			Input:    "typescript/test-skip-input.spec.ts",
			Expected: "typescript/test-skip-expected.spec.ts",
		},
		{
			Name:     "multiple_test_skip",
			Input:    "typescript/multi-skip-input.spec.ts",
			Expected: "typescript/multi-skip-expected.spec.ts",
		},
		{
			Name:     "no_marker_is_noop",
			Input:    "typescript/no-marker-input.spec.ts",
			Expected: "typescript/no-marker-expected.spec.ts",
		},
	}
	runFixtureCases(t, cases)
}

func runFixtureCases(t *testing.T, cases []fixtureCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			tmp := t.TempDir()
			// Stage the fixture under tmp/<basename> so the walker uses
			// the same extension as production. We strip the "-input"
			// suffix so the file looks like a real test file.
			inputBytes, err := os.ReadFile(filepath.Join("testdata", tc.Input))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			expectedBytes, err := os.ReadFile(filepath.Join("testdata", tc.Expected))
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			stagedName := stripInputSuffix(filepath.Base(tc.Input))
			stagedPath := filepath.Join(tmp, stagedName)
			if err := os.WriteFile(stagedPath, inputBytes, 0o644); err != nil {
				t.Fatalf("write staged: %v", err)
			}

			changes, err := RemoveDisabledMarkers(context.Background(), RemoveOptions{
				Roots: []string{tmp},
			})
			if err != nil {
				t.Fatalf("RemoveDisabledMarkers: %v", err)
			}

			gotBytes, err := os.ReadFile(stagedPath)
			if err != nil {
				t.Fatalf("read staged after run: %v", err)
			}
			if string(gotBytes) != string(expectedBytes) {
				t.Errorf("output mismatch for %s\n--- got ---\n%s\n--- want ---\n%s",
					tc.Input, string(gotBytes), string(expectedBytes))
			}

			// Sanity: a noop fixture (input == expected) must produce zero
			// FileChange entries; everything else must produce exactly one.
			isNoop := string(inputBytes) == string(expectedBytes)
			if isNoop && len(changes.Files) != 0 {
				t.Errorf("expected no FileChange for noop fixture, got %d", len(changes.Files))
			}
			if !isNoop && len(changes.Files) == 0 {
				t.Errorf("expected at least one FileChange for non-noop fixture, got 0")
			}
		})
	}
}

func stripInputSuffix(name string) string {
	// "disabled-with-import-input.java" → "disabled-with-import.java"
	// "multi-skip-input.spec.ts"        → "multi-skip.spec.ts"
	const marker = "-input"
	idx := strings.Index(name, marker)
	if idx < 0 {
		return name
	}
	return name[:idx] + name[idx+len(marker):]
}

// TestRemoveDisabledMarkers_MissingRoot asserts that a non-existent root is
// silently skipped (per the soft-skip contract for callers passing all
// three language roots unconditionally).
func TestRemoveDisabledMarkers_MissingRoot(t *testing.T) {
	tmp := t.TempDir()
	missing := filepath.Join(tmp, "does-not-exist")
	changes, err := RemoveDisabledMarkers(context.Background(), RemoveOptions{
		Roots: []string{missing},
	})
	if err != nil {
		t.Fatalf("RemoveDisabledMarkers on missing root: %v", err)
	}
	if len(changes.Files) != 0 {
		t.Errorf("expected zero FileChanges for missing root, got %d", len(changes.Files))
	}
}
```

Additional imports needed when re-pasting: `context`, `path/filepath`. The surviving test file already has `bytes`, `errors`, `fmt`, `io`, `os`, `strings`, `testing`.

## § testdata fixtures

### Java

`testdata/java/disabled-with-import-input.java`:

```java
package shop.systemtest;

import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

class RegisterCustomerTest {

    @Test
    @Disabled("AT - RED - DSL")
    void shouldRegisterCustomer() {
        // ...
    }
}
```

`testdata/java/disabled-with-import-expected.java`:

```java
package shop.systemtest;

import org.junit.jupiter.api.Test;

class RegisterCustomerTest {

    @Test
    void shouldRegisterCustomer() {
        // ...
    }
}
```

`testdata/java/multi-disabled-input.java`:

```java
package shop.systemtest;

import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;

class CheckoutTest {

    @Test
    @Disabled("AT - RED - DSL")
    void shouldCheckoutSuccessfully() {
    }

    @Test
    @Disabled
    void shouldRejectInvalidCart() {
    }

    @Test
    @Disabled("AT - RED - SYSTEM DRIVER")
    void shouldHandleEmptyCart() {
    }
}
```

`testdata/java/multi-disabled-expected.java`:

```java
package shop.systemtest;

import org.junit.jupiter.api.Test;

class CheckoutTest {

    @Test
    void shouldCheckoutSuccessfully() {
    }

    @Test
    void shouldRejectInvalidCart() {
    }

    @Test
    void shouldHandleEmptyCart() {
    }
}
```

`testdata/java/no-marker-input.java` (and `-expected.java` — same content; noop fixture):

```java
package shop.systemtest;

import org.junit.jupiter.api.Test;

class HappyPathTest {

    @Test
    void shouldDoNothingDisabled() {
        // No @Disabled here.
    }
}
```

### .NET

`testdata/dotnet/fact-skip-input.cs`:

```csharp
namespace Shop.SystemTest;

public class RegisterCustomerTest
{
    [Fact(Skip = "AT - RED - DSL")]
    public void ShouldRegisterCustomer()
    {
    }

    [Theory(Skip = "AT - RED - SYSTEM DRIVER")]
    [InlineData("foo")]
    public void ShouldRejectInvalid(string input)
    {
    }

    [Fact(DisplayName = "kept", Skip = "AT - RED - DSL")]
    public void ShouldKeepDisplayName()
    {
    }

    [Fact(Skip = "AT - RED - DSL", DisplayName = "kept2")]
    public void ShouldKeepDisplayName2()
    {
    }
}
```

`testdata/dotnet/fact-skip-expected.cs`:

```csharp
namespace Shop.SystemTest;

public class RegisterCustomerTest
{
    [Fact]
    public void ShouldRegisterCustomer()
    {
    }

    [Theory]
    [InlineData("foo")]
    public void ShouldRejectInvalid(string input)
    {
    }

    [Fact(DisplayName = "kept")]
    public void ShouldKeepDisplayName()
    {
    }

    [Fact(DisplayName = "kept2")]
    public void ShouldKeepDisplayName2()
    {
    }
}
```

`testdata/dotnet/skip-attribute-input.cs`:

```csharp
namespace Shop.SystemTest;

public class CheckoutTest
{
    [Fact]
    [Skip("AT - RED - DSL")]
    public void ShouldCheckoutSuccessfully()
    {
    }
}
```

`testdata/dotnet/skip-attribute-expected.cs`:

```csharp
namespace Shop.SystemTest;

public class CheckoutTest
{
    [Fact]
    public void ShouldCheckoutSuccessfully()
    {
    }
}
```

`testdata/dotnet/no-marker-input.cs` (and `-expected.cs` — same content; noop fixture):

```csharp
namespace Shop.SystemTest;

public class HappyPathTest
{
    [Fact]
    public void ShouldDoNothingDisabled()
    {
    }

    [Theory]
    [InlineData("foo")]
    public void ShouldStayEnabled(string input)
    {
    }
}
```

### TypeScript

`testdata/typescript/test-skip-input.spec.ts`:

```typescript
import { test, expect } from "vitest";

test("should register customer", async () => {
    test.skip(true, "AT - RED - DSL");
    expect(true).toBe(true);
});
```

`testdata/typescript/test-skip-expected.spec.ts`:

```typescript
import { test, expect } from "vitest";

test("should register customer", async () => {
    expect(true).toBe(true);
});
```

`testdata/typescript/multi-skip-input.spec.ts`:

```typescript
import { test, expect } from "vitest";

test("should checkout", async () => {
    test.skip(true, "AT - RED - DSL");
    expect(true).toBe(true);
});

test("should reject invalid cart", async () => {
    test.skip(true, "AT - RED - SYSTEM DRIVER");
});

test("should handle empty cart", async () => {
    test.skip(true, "AT - RED - DSL");
});
```

`testdata/typescript/multi-skip-expected.spec.ts`:

```typescript
import { test, expect } from "vitest";

test("should checkout", async () => {
    expect(true).toBe(true);
});

test("should reject invalid cart", async () => {
});

test("should handle empty cart", async () => {
});
```

`testdata/typescript/no-marker-input.spec.ts` (and `-expected.spec.ts` — same content; noop fixture):

```typescript
import { test, expect } from "vitest";

test("should run normally", async () => {
    expect(true).toBe(true);
});
```

## § bindings.go context keys

These const definitions belong inside the `Context keys consumed by the red_phase_cycle actions` const block in `bindings.go`. Their position relative to `CtxKeySuite` and `CtxKeyTestNames` (which were not deleted) is shown by their original order.

```go
// CtxKeyLanguage is the language disable_change_driven hands to
// ./disable-test.sh: java | csharp | typescript. The script owns the
// per-language `@Disabled` / `Skip = "..."` / `test.skip(true, "...")`
// edit syntax.
CtxKeyLanguage = "language"

// CtxKeyTicketID is the tracker-verbatim ticket identifier
// (e.g. "OPV-123", "#42", "SHOP-7"). disable_change_driven and
// enable_change_driven combine it with cycle (hard-coded "AT"),
// loop, and phase to produce the §Conventions disable-reason
// "<TICKET-ID> - AT - <LOOP> - <PHASE>" per
// docs/atdd/process/change/behavior/disable-tests.md.
CtxKeyTicketID = "ticket_id"

// CtxKeyLoop is the loop slot of the §Conventions disable-reason:
// RED | GREEN. Only RED uses disable today (GREEN never disables),
// but the schema reserves both for symmetry.
CtxKeyLoop = "loop"

// CtxKeyPhase is the phase slot of the §Conventions disable-reason:
// TEST | DSL | SYSTEM DRIVER (uppercase; internal space allowed).
CtxKeyPhase = "phase"

// CtxKeyPrevPhase is the phase whose @Disabled annotations
// enable_change_driven strips. Same domain as CtxKeyPhase. Loop is
// implicitly RED (the only loop that disables).
CtxKeyPrevPhase = "prev_phase"

// CtxKeyDisableTargets is the []string of "<file>:<method>" pairs
// disable_change_driven applies the disable markup to.
CtxKeyDisableTargets = "disable_targets"
```

## § bindings.go disable/enable functions

```go
// allowedDisableLoops is the loop slot domain per §Conventions disable-reason:
// docs/atdd/process/change/behavior/disable-tests.md.
var allowedDisableLoops = map[string]bool{"RED": true, "GREEN": true}

// allowedDisablePhases is the phase slot domain per §Conventions
// disable-reason: docs/atdd/process/change/behavior/disable-tests.md.
var allowedDisablePhases = map[string]bool{"TEST": true, "DSL": true, "SYSTEM DRIVER": true}

// assembleDisableReason builds the §Conventions disable-reason string
//
//	"<TICKET-ID> - AT - <LOOP> - <PHASE>"
//
// from constituents read from context. Cycle is hard-coded "AT" (CT slot
// reserved for symmetry, not yet used). Validates loop and phase against
// the published domains so the action fails fast on a malformed input
// rather than producing an un-strippable annotation downstream.
func assembleDisableReason(actionName, ticketID, loop, phase string) (string, error) {
	if ticketID == "" {
		return "", fmt.Errorf("%s: %s not set in Context", actionName, CtxKeyTicketID)
	}
	if !allowedDisableLoops[loop] {
		return "", fmt.Errorf("%s: %s %q not in {RED, GREEN}", actionName, CtxKeyLoop, loop)
	}
	if !allowedDisablePhases[phase] {
		return "", fmt.Errorf("%s: %s %q not in {TEST, DSL, SYSTEM DRIVER}", actionName, CtxKeyPhase, phase)
	}
	return fmt.Sprintf("%s - AT - %s - %s", ticketID, loop, phase), nil
}

// disableChangeDriven applies per-language disable markup
// (`@Disabled("reason")` / `[Fact(Skip = "reason")]` / `test.skip(true, "reason")`)
// to the change-driven test methods identified at WRITE. v1 shells out to
// ./disable-test.sh once per target — the script owns the language-specific
// edit syntax (the language-equivalents.md table).
//
// The reason string is assembled in-action from constituents per the
// §Conventions schema "<TICKET-ID> - AT - <LOOP> - <PHASE>"
// (see docs/atdd/process/change/behavior/disable-tests.md). This keeps the
// format-of-record next to the action that emits it; callers populate the
// four inputs separately rather than pre-formatting a brittle string.
//
// Reads:
//   - CtxKeyLanguage (string)         — required; java | csharp | typescript
//   - CtxKeyTicketID (string)         — required; tracker-verbatim id
//   - CtxKeyLoop (string)             — required; RED | GREEN
//   - CtxKeyPhase (string)            — required; TEST | DSL | SYSTEM DRIVER
//   - CtxKeyDisableTargets ([]string) — required; one entry per test,
//     formatted "<file>:<method>"
//
// Each target produces:
//
//	./disable-test.sh <language> "<reason>" <file>:<method>
//
// First failure halts the action with Outcome.Err — committing a partially
// disabled test set would leave the repo in an inconsistent state.
//
// Legacy-skip rule: the disable convention applies only to change-driven
// scenarios; legacy tests must never be annotated. The skip is owned by
// the caller (which selects disable_targets), since the legacy marker
// convention itself is designed by the legacy-coverage-cycle plan
// (plans/20260518-1116-legacy-coverage-cycle.md). This action assumes
// targets are pre-filtered.
func (a actions) disableChangeDriven(ctx *statemachine.Context) statemachine.Outcome {
	lang := ctx.GetString(CtxKeyLanguage)
	if lang == "" {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s not set in Context", CtxKeyLanguage)}
	}
	reason, err := assembleDisableReason("disable_change_driven",
		ctx.GetString(CtxKeyTicketID), ctx.GetString(CtxKeyLoop), ctx.GetString(CtxKeyPhase))
	if err != nil {
		return statemachine.Outcome{Err: err}
	}
	rawTargets, ok := ctx.State[CtxKeyDisableTargets]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s not set in Context", CtxKeyDisableTargets)}
	}
	targets, ok := rawTargets.([]string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s is %T, want []string", CtxKeyDisableTargets, rawTargets)}
	}
	if len(targets) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven: %s is empty", CtxKeyDisableTargets)}
	}
	for _, target := range targets {
		cmd := fmt.Sprintf("./disable-test.sh %s %s %s",
			shellEscape(lang), shellEscape(reason), shellEscape(target))
		if _, err := a.deps.Shell.Run(context.Background(), cmd); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("disable_change_driven (%s): %w", target, err)}
		}
	}
	return statemachine.Outcome{}
}

// enableChangeDriven inverts disableChangeDriven: it strips the per-language
// disable markup whose reason starts with the §Conventions re-enable filter
//
//	"<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>"
//
// from the named test methods. v1 shells out to ./enable-test.sh once per
// target — the script owns the per-language edit syntax and the
// startsWith match (mirroring disable-test.sh). Used at the start of
// at_green_system to re-enable tests the prior RED phase disabled.
//
// Loop is implicit RED — GREEN never disables, so re-enable always strips
// the prior RED annotation. The action passes the filter prefix to the
// shell script; the script must never strip annotations whose ticket
// prefix belongs to a different ticket, and must never strip legacy
// markers.
//
// Reads:
//   - CtxKeyLanguage (string)         — required; java | csharp | typescript
//   - CtxKeyTicketID (string)         — required; tracker-verbatim id
//   - CtxKeyPrevPhase (string)        — required; TEST | DSL | SYSTEM DRIVER
//   - CtxKeyDisableTargets ([]string) — required; one entry per test,
//     formatted "<file>:<method>"
//
// First failure halts the action with Outcome.Err — committing a partially
// re-enabled test set would leave the repo in an inconsistent state.
func (a actions) enableChangeDriven(ctx *statemachine.Context) statemachine.Outcome {
	lang := ctx.GetString(CtxKeyLanguage)
	if lang == "" {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s not set in Context", CtxKeyLanguage)}
	}
	filterPrefix, err := assembleDisableReason("enable_change_driven",
		ctx.GetString(CtxKeyTicketID), "RED", ctx.GetString(CtxKeyPrevPhase))
	if err != nil {
		// assembleDisableReason emits "phase" in its error for the phase slot;
		// enable's slot is conventionally prev_phase, so rewrite for clarity.
		return statemachine.Outcome{Err: fmt.Errorf("%s", strings.Replace(err.Error(), CtxKeyPhase, CtxKeyPrevPhase, 1))}
	}
	rawTargets, ok := ctx.State[CtxKeyDisableTargets]
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s not set in Context", CtxKeyDisableTargets)}
	}
	targets, ok := rawTargets.([]string)
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s is %T, want []string", CtxKeyDisableTargets, rawTargets)}
	}
	if len(targets) == 0 {
		return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven: %s is empty", CtxKeyDisableTargets)}
	}
	for _, target := range targets {
		cmd := fmt.Sprintf("./enable-test.sh %s %s %s",
			shellEscape(lang), shellEscape(filterPrefix), shellEscape(target))
		if _, err := a.deps.Shell.Run(context.Background(), cmd); err != nil {
			return statemachine.Outcome{Err: fmt.Errorf("enable_change_driven (%s): %w", target, err)}
		}
	}
	return statemachine.Outcome{}
}
```

## § bindings.go registry entries

These two registration lines belong in the action registry function (the one that wires `r.Register(...)` calls), originally placed near the `run_targeted_tests` registration around lines 158-159:

```go
r.Register("disable_change_driven", a.disableChangeDriven)
r.Register("enable_change_driven", a.enableChangeDriven)
```

The corresponding `bindings_test.go` registry test (around line 865 in the want list) also needs:

```go
"disable_change_driven",
"enable_change_driven",
```

## § doctrine docs

### `internal/assets/global/docs/atdd/process/change/behavior/disable-tests.md`

```markdown
# Disable change-driven tests

Annotation reason format:

\`\`\`
@Disabled("<TICKET-ID> - AT - <LOOP> - <PHASE>")
\`\`\`

- **Separator:** ` - ` (space-hyphen-space) between every segment.
- **`<TICKET-ID>`:** verbatim from the tracker (e.g. `OPV-123`, `#42`, `SHOP-7`).
- **`AT`:** the cycle (Acceptance Test).
- **`<LOOP>`:** `RED` | `GREEN`.
- **`<PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

Examples:

- `@Disabled("OPV-123 - AT - RED - TEST")`
- `@Disabled("OPV-123 - AT - RED - DSL")`
- `@Disabled("OPV-123 - AT - RED - SYSTEM DRIVER")`
```

### `internal/assets/global/docs/atdd/process/change/behavior/enable-tests.md`

```markdown
# Enable change-driven tests

Strip `@Disabled` annotations matching this filter:

\`\`\`
startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")
\`\`\`

- **`<PREV-PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

**Never strip annotations whose prefix belongs to a different ticket.**
```

Note: the inner fenced code blocks in these doctrine docs are escaped with backslashes (`\`\`\``) here so that this deferred plan, which itself uses Markdown fenced blocks, doesn't terminate prematurely. When restoring, replace each `\`\`\`` with a real triple-backtick.
