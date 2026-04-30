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
