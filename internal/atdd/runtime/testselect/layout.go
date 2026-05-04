package testselect

import (
	"path/filepath"
	"regexp"
	"strings"
)

// layout describes how to find driver-adapter, driver-port, dsl, and test
// sources for one language, plus the regex shapes for method signatures,
// callers, test methods, and channel annotations.
//
// Path roots are *absolute* paths under repoRoot. We compute them lazily
// because the system-test root layout differs slightly between languages.
type layout struct {
	Lang       string
	SourceExts []string
	TestExts   []string

	// AdapterMatch returns true when the given repo-relative path is a
	// driver-adapter source file for this language. Used to filter the
	// `git diff` output.
	AdapterMatch func(relPath string) bool
	// PortMatch / DSLMatch / TestMatch identify files that belong to the
	// driver-port, dsl-core/usecase, and test trees respectively. The
	// path argument is whatever Walk returned (typically OS-native).
	PortMatch func(path string) bool
	DSLMatch  func(path string) bool
	TestMatch func(path string) bool
	// PortRoot returns the absolute path to the driver-port root.
	PortRoot func(repoRoot string) string
	// DSLRoot returns the absolute path to the DSL core/usecase root.
	DSLRoot func(repoRoot string) string
	// TestRoots returns the absolute paths to test source roots.
	TestRoots func(repoRoot string) []string

	// MethodSignatureRE captures group 1 = method name. Anchored to start
	// of line; conservative so it picks up real method declarations rather
	// than calls. Used both for adapters (during diff parsing) and for
	// ports/DSL (during caller resolution).
	MethodSignatureRE *regexp.Regexp
	// CallerRE captures `.<methodName>(` style invocations. Composed with
	// the target method name at use time.
	CallerREFor func(methodName string) *regexp.Regexp
	// TestMethodSignatureRE captures group 1 = test method name. Used to
	// determine the enclosing test method of any caller match.
	TestMethodSignatureRE *regexp.Regexp
	// TestAnnotationLines returns true when the given line declares the
	// method as a test (e.g. `@Test`, `@TestTemplate`, `[Fact]`, `it(` /
	// `test(`). Used to filter test methods from non-test methods inside
	// the same test file (e.g. helper methods).
	IsTestAnnotation func(line string) bool
	// ChannelAnnotationRE matches the @Channel(...) annotation; group 1
	// is the parenthesised contents. Empty if the language has no
	// equivalent.
	ChannelAnnotationRE *regexp.Regexp
	// ContractTestPathHint is a substring that, when present in a test
	// file path, marks the test as a contract test. (Falls back to suite
	// rules.)
	ContractTestPathHint string
}

// layouts is keyed by language code: "java" | "dotnet" | "typescript".
var layouts = map[string]*layout{
	"java":       javaLayout(),
	"dotnet":     dotnetLayout(),
	"typescript": typescriptLayout(),
}

func javaLayout() *layout {
	// Method signature regex — anchored, must end with `(`. Captures
	// the identifier preceding the open paren. Excludes lines that start
	// with control keywords (return, if, for, while) or assignments to
	// avoid call-site false positives.
	sig := regexp.MustCompile(
		`^\s*(?:@\w+(?:\([^)]*\))?\s+)*` + // any number of annotations on the same line (rare)
			`(?:public|protected|private|static|final|abstract|synchronized|default|@\w+\s*)*\s+` + // modifiers
			`[\w<>\[\],\s\?\.]+?\s+` + // return type (greedy-light)
			`(\w+)\s*\(`, // method name + open paren
	)
	return &layout{
		Lang:       "java",
		SourceExts: []string{".java"},
		TestExts:   []string{".java"},
		AdapterMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".java") && strings.Contains(p, "testkit/driver/adapter/")
		},
		PortMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".java") && strings.Contains(p, "testkit/driver/port/")
		},
		DSLMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".java") && strings.Contains(p, "testkit/dsl/")
		},
		TestMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".java") && strings.Contains(p, "/src/test/")
		},
		PortRoot: func(repoRoot string) string {
			return filepath.Join(repoRoot, "system-test", "java", "src", "main", "java")
		},
		DSLRoot: func(repoRoot string) string {
			return filepath.Join(repoRoot, "system-test", "java", "src", "main", "java")
		},
		TestRoots: func(repoRoot string) []string {
			return []string{filepath.Join(repoRoot, "system-test", "java", "src", "test", "java")}
		},
		MethodSignatureRE: sig,
		CallerREFor: func(name string) *regexp.Regexp {
			// Word boundary + dot/space-prefixed call; rules out partial-name
			// false positives (e.g. `.placeOrderEx(` not matching `placeOrder`).
			return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*\(`)
		},
		TestMethodSignatureRE: sig,
		IsTestAnnotation: func(line string) bool {
			t := strings.TrimSpace(line)
			return strings.HasPrefix(t, "@Test") ||
				strings.HasPrefix(t, "@TestTemplate") ||
				strings.HasPrefix(t, "@ParameterizedTest") ||
				strings.HasPrefix(t, "@RepeatedTest")
		},
		ChannelAnnotationRE:  regexp.MustCompile(`@Channel\s*\(([^)]*)\)`),
		ContractTestPathHint: "/contract/",
	}
}

func dotnetLayout() *layout {
	// C# is structurally similar to Java for method signatures.
	sig := regexp.MustCompile(
		`^\s*(?:\[[\w\(\)"' ,]*\]\s*)*` + // attributes inline
			`(?:public|protected|private|internal|static|virtual|override|async|sealed|abstract|new|\s)+\s+` +
			`[\w<>\[\],\s\?\.]+?\s+` +
			`(\w+)\s*\(`,
	)
	return &layout{
		Lang:       "dotnet",
		SourceExts: []string{".cs"},
		TestExts:   []string{".cs"},
		AdapterMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".cs") && strings.Contains(p, "/driver.adapter/")
		},
		PortMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".cs") && strings.Contains(p, "/driver.port/")
		},
		DSLMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".cs") && strings.Contains(p, "/dsl.")
		},
		TestMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".cs") &&
				(strings.Contains(p, "/tests/") || strings.Contains(p, "test.cs"))
		},
		PortRoot: func(repoRoot string) string {
			return filepath.Join(repoRoot, "system-test", "dotnet")
		},
		DSLRoot: func(repoRoot string) string {
			return filepath.Join(repoRoot, "system-test", "dotnet")
		},
		TestRoots: func(repoRoot string) []string {
			return []string{filepath.Join(repoRoot, "system-test", "dotnet")}
		},
		MethodSignatureRE: sig,
		CallerREFor: func(name string) *regexp.Regexp {
			return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*\(`)
		},
		TestMethodSignatureRE: sig,
		IsTestAnnotation: func(line string) bool {
			t := strings.TrimSpace(line)
			return strings.HasPrefix(t, "[Fact") ||
				strings.HasPrefix(t, "[Theory") ||
				strings.HasPrefix(t, "[Test") ||
				strings.HasPrefix(t, "[TestMethod")
		},
		ChannelAnnotationRE:  regexp.MustCompile(`\[Channel\s*\(([^)]*)\)\]`),
		ContractTestPathHint: "/contract/",
	}
}

func typescriptLayout() *layout {
	// Method declaration in a class body OR a function declaration.
	sig := regexp.MustCompile(
		`^\s*(?:public\s+|private\s+|protected\s+|static\s+|async\s+|export\s+|function\s+)*` +
			`(\w+)\s*` +
			`(?:<[^>]*>)?\s*\(`,
	)
	return &layout{
		Lang:       "typescript",
		SourceExts: []string{".ts"},
		// .spec.ts and .test.ts are both common.
		TestExts: []string{".ts"},
		AdapterMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".ts") && strings.Contains(p, "testkit/driver/adapter/")
		},
		PortMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".ts") && strings.Contains(p, "testkit/driver/port/")
		},
		DSLMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".ts") && strings.Contains(p, "testkit/dsl/")
		},
		TestMatch: func(p string) bool {
			p = filepath.ToSlash(strings.ToLower(p))
			return strings.HasSuffix(p, ".ts") &&
				(strings.Contains(p, "/tests/") ||
					strings.HasSuffix(p, ".spec.ts") ||
					strings.HasSuffix(p, ".test.ts"))
		},
		PortRoot: func(repoRoot string) string {
			return filepath.Join(repoRoot, "system-test", "typescript")
		},
		DSLRoot: func(repoRoot string) string {
			return filepath.Join(repoRoot, "system-test", "typescript")
		},
		TestRoots: func(repoRoot string) []string {
			return []string{filepath.Join(repoRoot, "system-test", "typescript")}
		},
		MethodSignatureRE: sig,
		CallerREFor: func(name string) *regexp.Regexp {
			return regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*\(`)
		},
		// In TS we don't use the same regex — tests are detected by
		// `it(` / `test(` blocks. testMethodIndex handles this specially
		// and ignores TestMethodSignatureRE for TS.
		TestMethodSignatureRE: sig,
		IsTestAnnotation: func(line string) bool {
			t := strings.TrimSpace(line)
			return strings.HasPrefix(t, "it(") ||
				strings.HasPrefix(t, "test(") ||
				strings.HasPrefix(t, "it.") ||
				strings.HasPrefix(t, "test.")
		},
		// TS shop uses // @channel(API) comment-style or describe block
		// names with " API " / " UI " — we'll match a relaxed comment hint
		// or a describe block tag.
		ChannelAnnotationRE:  regexp.MustCompile(`@channel\s*\(([^)]*)\)`),
		ContractTestPathHint: "/contract/",
	}
}
