package testselect

import (
	"strings"
)

// methodRegion records the line span (1-based, inclusive) of a method
// declaration in a single file. The body extends from the open `{` of
// the signature line (or the next `{` after the signature) to the
// matching `}` at depth zero.
type methodRegion struct {
	name      string
	startLine int
	endLine   int
}

// extractMethodRegions scans `body` once and returns every method-shaped
// region. Used both for adapters (intersect with diff hunks) and for
// ports / DSL / tests (find enclosing method of any caller hit).
//
// The brace tracker is plain — it ignores comments and strings. False
// matches inside string literals or block comments are tolerable: the
// downstream consumer only cares about the *name → file* mapping for
// callers, and a stray name in a comment is harmless.
func extractMethodRegions(body string, lay *layout) []methodRegion {
	lines := strings.Split(body, "\n")
	var regions []methodRegion

	depth := 0
	pending := -1 // line index where we last matched a signature; expecting `{`
	pendingName := ""
	pendingDepth := 0

	for i, raw := range lines {
		line := raw
		// Track every brace on this line in order so signatures that have
		// `(...){` on the same line still progress.
		startCol := 0
		for startCol < len(line) {
			// Try matching a signature once per line, on the leftmost
			// non-whitespace position. Once we've matched, we still need
			// to walk braces to advance depth.
			break
		}
		// Step 1: match a signature on this line if we are at depth 0 OR
		// inside a class body (depth 1 for top-level; deeper for nested).
		// We use the same regex everywhere — within the body the regex
		// is conservative enough to avoid false positives.
		if pending < 0 {
			if m := lay.MethodSignatureRE.FindStringSubmatchIndex(line); m != nil {
				name := line[m[2]:m[3]]
				if !isReservedKeyword(name) && !isControlBlockKeyword(name) {
					// If the line ends with `;` (after stripping trailing
					// whitespace and comments), treat it as an
					// interface/abstract declaration: record a single-line
					// region and skip the pending tracker.
					trimmed := strings.TrimRight(strings.TrimSpace(line), " \t")
					// Strip a trailing line comment.
					if idx := strings.Index(trimmed, "//"); idx >= 0 {
						trimmed = strings.TrimSpace(trimmed[:idx])
					}
					if strings.HasSuffix(trimmed, ";") {
						regions = append(regions, methodRegion{
							name:      name,
							startLine: i + 1,
							endLine:   i + 1,
						})
					} else {
						pending = i
						pendingName = name
						pendingDepth = depth
					}
				}
			}
		}
		// Step 2: walk braces on this line.
		for j := 0; j < len(line); j++ {
			c := line[j]
			switch c {
			case '{':
				depth++
				if pending >= 0 && depth == pendingDepth+1 {
					// Found the opening brace of the pending signature.
					startLine := pending + 1 // 1-based
					name := pendingName
					// Find the matching close brace.
					endLine := findMatchingClose(lines, i, j, depth)
					regions = append(regions, methodRegion{
						name:      name,
						startLine: startLine,
						endLine:   endLine + 1, // 1-based
					})
					pending = -1
					pendingName = ""
					pendingDepth = 0
				}
			case '}':
				depth--
			case '/':
				// Skip a `//` comment to end of line.
				if j+1 < len(line) && line[j+1] == '/' {
					j = len(line)
				}
			case '"':
				// Skip a string literal on the same line.
				k := j + 1
				for k < len(line) {
					if line[k] == '\\' && k+1 < len(line) {
						k += 2
						continue
					}
					if line[k] == '"' {
						break
					}
					k++
				}
				j = k
			}
		}
	}
	return regions
}

// findMatchingClose returns the 0-based line index of the `}` that closes
// the brace at lines[startLineIdx][startCol]. Depth is the depth *after*
// counting the open brace (i.e. depth==pendingDepth+1).
func findMatchingClose(lines []string, startLineIdx, startCol, depth int) int {
	d := depth
	for i := startLineIdx; i < len(lines); i++ {
		line := lines[i]
		startJ := 0
		if i == startLineIdx {
			startJ = startCol + 1
		}
		for j := startJ; j < len(line); j++ {
			c := line[j]
			switch c {
			case '{':
				d++
			case '}':
				d--
				if d == depth-1 {
					return i
				}
			case '/':
				if j+1 < len(line) && line[j+1] == '/' {
					j = len(line)
				}
			case '"':
				k := j + 1
				for k < len(line) {
					if line[k] == '\\' && k+1 < len(line) {
						k += 2
						continue
					}
					if line[k] == '"' {
						break
					}
					k++
				}
				j = k
			}
		}
	}
	return len(lines) - 1
}

// isReservedKeyword filters out signature regex hits that captured a
// language keyword (return, if, for, while, switch, ...) instead of a
// real method name.
func isReservedKeyword(name string) bool {
	switch name {
	case "return", "if", "for", "while", "switch", "do",
		"throw", "throws", "try", "catch", "finally", "new",
		"true", "false", "null", "void", "this", "super",
		"public", "private", "protected", "static", "final",
		"abstract", "synchronized", "import", "package",
		"class", "interface", "enum", "record":
		return true
	}
	return false
}

// isControlBlockKeyword catches `if(` / `for(` / `while(` style bodies
// that the open-paren regex may mistake for a method declaration.
func isControlBlockKeyword(name string) bool {
	switch name {
	case "if", "for", "while", "switch", "catch", "synchronized":
		return true
	}
	return false
}

// methodIndex maps method name → list of {file, region} so callersOf can
// find every place where a method is *declared* and its enclosing region.
type methodIndex struct {
	byName  map[string][]methodRecord // method name → declarations
	byFile  map[string][]methodRegion // file path → regions in that file (sorted by start)
}

type methodRecord struct {
	File   string
	Method string
	Region methodRegion
}

// indexMethods reads each file once and indexes every method declaration
// found by the layout's MethodSignatureRE.
func indexMethods(files []string, lay *layout, read func(string, string) ([]byte, error)) *methodIndex {
	idx := &methodIndex{
		byName: map[string][]methodRecord{},
		byFile: map[string][]methodRegion{},
	}
	for _, f := range files {
		body, err := read("", f) // f is already absolute when walked
		if err != nil {
			continue
		}
		regions := extractMethodRegions(string(body), lay)
		idx.byFile[f] = regions
		for _, r := range regions {
			idx.byName[r.name] = append(idx.byName[r.name], methodRecord{
				File:   f,
				Method: r.name,
				Region: r,
			})
		}
	}
	return idx
}

// testHit names a test method and the file it lives in (used for
// per-test suite tagging).
type testHit struct {
	Name     string // Class.method (Java/.NET) or describe-block.it (TS)
	File     string
	Channels []string // raw channel hints found near the declaration
	Class    string   // enclosing class name (for Java/.NET); "" otherwise
}

// indexTestMethods reads each test file once. For Java / .NET it records
// every method preceded by a test annotation; for TypeScript it records
// every `it(` / `test(` block. The test name format follows the runner's
// expectations: `Class.method` for Java / .NET, `describe > it` for TS
// (joined as `describe.it` for `--test` flag substitution).
func indexTestMethods(files []string, lay *layout, read func(string, string) ([]byte, error)) map[string][]testHit {
	out := map[string][]testHit{}
	for _, f := range files {
		body, err := read("", f)
		if err != nil {
			continue
		}
		switch lay.Lang {
		case "typescript":
			indexTSTests(f, string(body), lay, out)
		default:
			indexClassTests(f, string(body), lay, out)
		}
	}
	return out
}

// indexClassTests walks a Java / .NET file. The class name comes from the
// nearest preceding `class Foo` declaration; method names come from the
// signature regex. A method is considered a test only when one of the
// preceding lines (up to a small window) carries a test annotation.
//
// Multiple test annotations stack — `@Test` plus `@Channel(API)` etc.
func indexClassTests(file, body string, lay *layout, out map[string][]testHit) {
	lines := strings.Split(body, "\n")
	regions := extractMethodRegions(body, lay)

	// Collect class names by line number.
	classByLine := classDeclarationLineMap(lines)

	for _, r := range regions {
		// Look at up to 5 lines before the start to find @Test / @Channel.
		isTest := false
		channels := []string{}
		for i := r.startLine - 2; i >= 0 && i >= r.startLine-7; i-- {
			line := lines[i]
			if lay.IsTestAnnotation(line) {
				isTest = true
			}
			if lay.ChannelAnnotationRE != nil {
				if m := lay.ChannelAnnotationRE.FindStringSubmatch(line); m != nil {
					for _, c := range parseChannelArgs(m[1]) {
						channels = append(channels, c)
					}
				}
			}
		}
		if !isTest {
			continue
		}
		class := nearestClassName(classByLine, r.startLine)
		name := class + "." + r.name
		if class == "" {
			name = r.name
		}
		out[r.name] = append(out[r.name], testHit{
			Name:     name,
			File:     file,
			Channels: channels,
			Class:    class,
		})
	}
}

// classDeclarationLineMap returns a map of 1-based line number → class
// name for every `class Foo` (Java/.NET) declaration.
func classDeclarationLineMap(lines []string) map[int]string {
	m := map[int]string{}
	for i, line := range lines {
		t := strings.TrimSpace(line)
		// Match `class Foo` (preceded by optional `public` etc.).
		idx := strings.Index(t, "class ")
		if idx < 0 {
			continue
		}
		// Filter out `// class` comments.
		head := strings.TrimSpace(t[:idx])
		if strings.HasSuffix(head, "//") || strings.HasSuffix(head, "*") {
			continue
		}
		rest := t[idx+len("class "):]
		// Pull the next identifier.
		name := ""
		for j := 0; j < len(rest); j++ {
			c := rest[j]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
				name += string(c)
				continue
			}
			break
		}
		if name == "" {
			continue
		}
		m[i+1] = name
	}
	return m
}

// nearestClassName returns the most-recent class name declared on or
// before `line`.
func nearestClassName(m map[int]string, line int) string {
	best := -1
	for k := range m {
		if k <= line && k > best {
			best = k
		}
	}
	if best < 0 {
		return ""
	}
	return m[best]
}

// parseChannelArgs splits the argument list of @Channel(args) / [Channel(args)]
// into individual channel labels. Accepts `API`, `UI`, `ChannelType.UI`,
// `{ChannelType.UI, ChannelType.API}` and similar.
func parseChannelArgs(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "{}")
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Strip a leading `ChannelType.` if present.
		if dot := strings.LastIndex(p, "."); dot >= 0 {
			p = p[dot+1:]
		}
		out = append(out, strings.ToUpper(p))
	}
	return out
}

// indexTSTests walks a TypeScript file looking for `describe(`/`it(`/`test(`
// blocks. For each `it(` we compose `<describe-stack>.<it-name>` (joined
// by `.`) — close enough to what jest / vitest accepts via --test for
// targeting. Channels are read from a `// @channel(API)` style comment
// on the line before the `it(` block.
func indexTSTests(file, body string, lay *layout, out map[string][]testHit) {
	lines := strings.Split(body, "\n")
	descStack := []string{}
	descBraceDepth := []int{}
	depth := 0

	for i, line := range lines {
		t := strings.TrimSpace(line)
		// Track describe entry.
		if strings.HasPrefix(t, "describe(") || strings.HasPrefix(t, "describe.") {
			if name, ok := readQuotedArg(t); ok {
				descStack = append(descStack, name)
				descBraceDepth = append(descBraceDepth, depth)
			}
		}
		// Track it / test entry.
		if strings.HasPrefix(t, "it(") || strings.HasPrefix(t, "test(") ||
			strings.HasPrefix(t, "it.") || strings.HasPrefix(t, "test.") {
			if name, ok := readQuotedArg(t); ok {
				prefix := strings.Join(descStack, ".")
				full := name
				if prefix != "" {
					full = prefix + "." + name
				}
				channels := []string{}
				if i > 0 && lay.ChannelAnnotationRE != nil {
					if m := lay.ChannelAnnotationRE.FindStringSubmatch(lines[i-1]); m != nil {
						channels = append(channels, parseChannelArgs(m[1])...)
					}
				}
				out[full] = append(out[full], testHit{
					Name:     full,
					File:     file,
					Channels: channels,
				})
			}
		}
		// Track braces to know when a describe block closes.
		for j := 0; j < len(line); j++ {
			c := line[j]
			switch c {
			case '{':
				depth++
			case '}':
				depth--
				// Pop describes whose depth reached this level.
				for len(descBraceDepth) > 0 && descBraceDepth[len(descBraceDepth)-1] >= depth {
					descStack = descStack[:len(descStack)-1]
					descBraceDepth = descBraceDepth[:len(descBraceDepth)-1]
				}
			case '/':
				if j+1 < len(line) && line[j+1] == '/' {
					j = len(line)
				}
			case '"', '\'', '`':
				quote := c
				k := j + 1
				for k < len(line) {
					if line[k] == '\\' && k+1 < len(line) {
						k += 2
						continue
					}
					if line[k] == quote {
						break
					}
					k++
				}
				j = k
			}
		}
	}
}

// extractDeclaredAndParentTypes scans a class/interface declaration in
// `body` and returns:
//
//	declared — every type name introduced (class Foo, interface Foo, …)
//	parents  — every type name appearing in an extends / implements / `:`
//	           clause (with package prefixes stripped)
//
// Used by class-qualification: a port whose declaring file's interface
// name is in some adapter file's parents list is the same logical
// contract; a port whose declared types do not overlap is a same-named
// method on an unrelated port and should be filtered out.
//
// The parser is permissive: any non-keyword identifier between the type
// name and the body's opening brace is treated as a parent. Generic
// blocks (`<T>`) are stripped first so `Foo<T> implements Bar<T>` yields
// a single `Bar` parent rather than `Bar` plus `T`.
func extractDeclaredAndParentTypes(body string, lay *layout) (declared, parents []string) {
	if lay.ClassDeclRE == nil {
		return nil, nil
	}
	matches := lay.ClassDeclRE.FindAllStringSubmatchIndex(body, -1)
	for _, m := range matches {
		name := body[m[2]:m[3]]
		declared = append(declared, name)
		rest := body[m[3]:]
		braceIdx := strings.IndexByte(rest, '{')
		if braceIdx < 0 {
			continue
		}
		parents = append(parents, parseParentNames(rest[:braceIdx])...)
	}
	return declared, parents
}

// parseParentNames extracts identifier tokens from a class/interface
// header (the text between the type name and the opening brace),
// dropping structural keywords like `extends` / `implements` / `where`.
// Generics are stripped before tokenizing.
func parseParentNames(header string) []string {
	header = stripGenerics(header)
	var names []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		tok := cur.String()
		cur.Reset()
		if isParentSeparatorKeyword(tok) {
			return
		}
		if dot := strings.LastIndex(tok, "."); dot >= 0 {
			tok = tok[dot+1:]
		}
		names = append(names, tok)
	}
	for i := 0; i < len(header); i++ {
		c := header[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.' {
			cur.WriteByte(c)
			continue
		}
		flush()
	}
	flush()
	return names
}

func isParentSeparatorKeyword(tok string) bool {
	switch tok {
	case "extends", "implements", "where", "by":
		return true
	}
	return false
}

// stripGenerics removes balanced `<…>` blocks from a string. Non-generic
// `<` / `>` tokens (e.g. arithmetic) are extremely unlikely in a class
// header, so this is conservative enough for our use.
func stripGenerics(s string) string {
	var out strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteByte(c)
			}
		}
	}
	return out.String()
}

// readQuotedArg reads the first quoted argument from a call expression
// like `it("foo bar", () => { ... })`. Supports single, double, and
// backtick quotes.
func readQuotedArg(s string) (string, bool) {
	open := strings.Index(s, "(")
	if open < 0 {
		return "", false
	}
	rest := strings.TrimSpace(s[open+1:])
	if rest == "" {
		return "", false
	}
	q := rest[0]
	if q != '"' && q != '\'' && q != '`' {
		return "", false
	}
	end := -1
	for i := 1; i < len(rest); i++ {
		if rest[i] == '\\' && i+1 < len(rest) {
			i++
			continue
		}
		if rest[i] == q {
			end = i
			break
		}
	}
	if end < 0 {
		return "", false
	}
	return rest[1:end], true
}
