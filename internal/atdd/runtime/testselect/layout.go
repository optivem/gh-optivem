package testselect

import (
	"path/filepath"
	"regexp"
	"strings"
)

// layout describes how to find driver-adapter, driver-port, dsl, and test
// sources for one language, plus the per-language indexer / caller-finder /
// class-extractor extension points the rest of the package routes through.
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

	// MethodIndexer returns every method-shaped region in `body` for this
	// language. Wired per language — currently to a regex-based scan that
	// recognises a class-body method declaration shape; will swap to a
	// tree-sitter walk per the migration plan.
	MethodIndexer func(body string) []methodRegion
	// CallerFinder returns the byte offsets in `body` where `methodName`
	// is invoked. Each offset is the start of the call expression. The
	// existing pipeline maps offsets to lines via byteOffsetToLine.
	CallerFinder func(body, methodName string) []int
	// ClassExtractor returns the class/interface names declared in `body`
	// and the parent type names in their extends/implements clauses, in
	// declaration order. Used by class-qualification to align port and
	// adapter contracts.
	ClassExtractor func(body string) (declared, parents []string)

	// IsTestAnnotation returns true when the given line declares the
	// method as a test (e.g. `@Test`, `@TestTemplate`, `[Fact]`, `it(` /
	// `test(`). Line-shape matcher; tree-sitter doesn't help here.
	IsTestAnnotation func(line string) bool
	// ChannelAnnotationRE matches the @Channel(...) annotation; group 1
	// is the parenthesised contents. Empty if the language has no
	// equivalent. Line-shape matcher; tree-sitter doesn't help here.
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
	classDeclRE := regexp.MustCompile(`\b(?:class|interface|record)\s+(\w+)\b`)
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
		MethodIndexer:  regexMethodIndexer(sig),
		CallerFinder:   regexCallerFinder,
		ClassExtractor: regexClassExtractor(classDeclRE),
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
	classDeclRE := regexp.MustCompile(`\b(?:class|interface|record|struct)\s+(\w+)\b`)
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
		MethodIndexer:  regexMethodIndexer(sig),
		CallerFinder:   regexCallerFinder,
		ClassExtractor: regexClassExtractor(classDeclRE),
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
	classDeclRE := regexp.MustCompile(`\b(?:class|interface)\s+(\w+)\b`)
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
		MethodIndexer:  regexMethodIndexer(sig),
		CallerFinder:   regexCallerFinder,
		ClassExtractor: regexClassExtractor(classDeclRE),
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

// regexCallerFinder is the shared regex implementation of CallerFinder.
// All three current languages (java, dotnet, typescript) use the same
// `\b<name>\s*\(` shape, so they share one implementation. The TypeScript
// slice will replace this with a tree-sitter query for `call_expression`
// nodes whose function ends in `property_identifier` or `identifier`.
func regexCallerFinder(body, methodName string) []int {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(methodName) + `\s*\(`)
	matches := re.FindAllStringIndex(body, -1)
	out := make([]int, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[0])
	}
	return out
}
