package testselect

import (
	"sync"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// Tree-sitter TypeScript implementation of MethodIndexer / CallerFinder /
// ClassExtractor. Parity with the regex layer: every existing TS testselect
// and tracer test must pass without modification when this is wired into
// typescriptLayout (step 4 of the migration).
//
// The implementation is shape-aware (not type-aware): it parses each body
// once with tree-sitter and walks the AST. Recognises declaration shapes
// the regex layer missed — arrow-property class fields
// (`inputSku = async (sku: string) => {…}`), multi-line signatures,
// getters/setters, decorated methods, generics with line breaks.
//
// Byte offsets feed into the existing byteOffsetToLine plumbing, so the
// downstream caller-attribution logic (callersOf / callersOfTest) is
// unchanged.

var (
	tsLangOnce sync.Once
	tsLang     *tree_sitter.Language

	tsMethodQueryOnce sync.Once
	tsMethodQuery     *tree_sitter.Query
	tsMethodCapName   uint
	tsMethodCapMethod uint

	tsCallerQueryOnce sync.Once
	tsCallerQuery     *tree_sitter.Query
	tsCallerCapCallee uint

	tsClassQueryOnce       sync.Once
	tsClassDeclaredQuery   *tree_sitter.Query
	tsClassParentsQuery    *tree_sitter.Query
	tsClassDeclaredCapName uint
	tsClassParentsCapName  uint
)

func tsLanguage() *tree_sitter.Language {
	tsLangOnce.Do(func() {
		tsLang = tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	})
	return tsLang
}

// treesitterMethodIndexer parses body as TypeScript and emits methodRegion
// entries for every method-shaped declaration. The struct shape and span
// semantics match regexMethodIndexer's output: 1-based startLine (signature
// line), 1-based endLine (closing-brace line for concrete methods, same as
// startLine for abstract / interface signatures).
func treesitterMethodIndexer(body string) []methodRegion {
	if body == "" {
		return nil
	}
	src := []byte(body)
	tree := parseTypeScript(src)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	q := methodQuery()
	if q == nil {
		return nil
	}
	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	matches := cursor.Matches(q, tree.RootNode(), src)
	var regions []methodRegion
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		var (
			nameText  string
			startByte uint
			endByte   uint
			haveSpan  bool
			haveName  bool
		)
		for _, cap := range m.Captures {
			switch uint(cap.Index) {
			case tsMethodCapName:
				nameText = cap.Node.Utf8Text(src)
				haveName = true
			case tsMethodCapMethod:
				startByte = cap.Node.StartByte()
				endByte = cap.Node.EndByte()
				haveSpan = true
			}
		}
		if !haveName || !haveSpan || nameText == "" {
			continue
		}
		startLine := byteOffsetToLine(body, int(startByte))
		endLine := byteOffsetToLine(body, int(endByte)-1)
		if endLine < startLine {
			endLine = startLine
		}
		regions = append(regions, methodRegion{
			name:      nameText,
			startLine: startLine,
			endLine:   endLine,
		})
	}
	return regions
}

// treesitterCallerFinder returns the byte offsets of every call expression
// in body whose callee identifier (or member-expression property) is
// methodName. Covers `methodName(...)`, `obj.methodName(...)`, and
// `this.methodName(...)`. The offset returned is the start of the callee
// identifier — same shape as regexCallerFinder, so byteOffsetToLine maps it
// to the same line.
func treesitterCallerFinder(body, methodName string) []int {
	if body == "" || methodName == "" {
		return nil
	}
	src := []byte(body)
	tree := parseTypeScript(src)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	q := callerQuery()
	if q == nil {
		return nil
	}
	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	matches := cursor.Matches(q, tree.RootNode(), src)
	var offsets []int
	for {
		m := matches.Next()
		if m == nil {
			break
		}
		for _, cap := range m.Captures {
			if uint(cap.Index) != tsCallerCapCallee {
				continue
			}
			if cap.Node.Utf8Text(src) != methodName {
				continue
			}
			offsets = append(offsets, int(cap.Node.StartByte()))
		}
	}
	return offsets
}

// treesitterClassExtractor returns the names of class/interface declarations
// in body and the parent type names referenced by their extends/implements
// clauses, in declaration order. Matches regexClassExtractor's flat-list
// shape so classQualifyPortCandidates can consume it unchanged.
func treesitterClassExtractor(body string) (declared, parents []string) {
	if body == "" {
		return nil, nil
	}
	src := []byte(body)
	tree := parseTypeScript(src)
	if tree == nil {
		return nil, nil
	}
	defer tree.Close()

	declQ, parentQ := classQueries()
	if declQ == nil || parentQ == nil {
		return nil, nil
	}
	cursor := tree_sitter.NewQueryCursor()
	defer cursor.Close()

	declMatches := cursor.Matches(declQ, tree.RootNode(), src)
	for {
		m := declMatches.Next()
		if m == nil {
			break
		}
		for _, cap := range m.Captures {
			if uint(cap.Index) != tsClassDeclaredCapName {
				continue
			}
			declared = append(declared, cap.Node.Utf8Text(src))
		}
	}

	parentCursor := tree_sitter.NewQueryCursor()
	defer parentCursor.Close()
	parentMatches := parentCursor.Matches(parentQ, tree.RootNode(), src)
	for {
		m := parentMatches.Next()
		if m == nil {
			break
		}
		for _, cap := range m.Captures {
			if uint(cap.Index) != tsClassParentsCapName {
				continue
			}
			parents = append(parents, cap.Node.Utf8Text(src))
		}
	}
	return declared, parents
}

func parseTypeScript(src []byte) *tree_sitter.Tree {
	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(tsLanguage()); err != nil {
		return nil
	}
	return parser.Parse(src, nil)
}

func methodQuery() *tree_sitter.Query {
	tsMethodQueryOnce.Do(func() {
		const src = `
(method_definition
  name: (property_identifier) @name) @method

(abstract_method_signature
  name: (property_identifier) @name) @method

(method_signature
  name: (property_identifier) @name) @method

(public_field_definition
  name: (property_identifier) @name
  value: (arrow_function)) @method

(public_field_definition
  name: (property_identifier) @name
  value: (function_expression)) @method

(function_declaration
  name: (identifier) @name) @method

(generator_function_declaration
  name: (identifier) @name) @method

(function_signature
  name: (identifier) @name) @method
`
		q, qerr := tree_sitter.NewQuery(tsLanguage(), src)
		if qerr != nil {
			return
		}
		nameIdx, ok1 := q.CaptureIndexForName("name")
		methodIdx, ok2 := q.CaptureIndexForName("method")
		if !ok1 || !ok2 {
			q.Close()
			return
		}
		tsMethodQuery = q
		tsMethodCapName = nameIdx
		tsMethodCapMethod = methodIdx
	})
	return tsMethodQuery
}

func callerQuery() *tree_sitter.Query {
	tsCallerQueryOnce.Do(func() {
		const src = `
(call_expression
  function: (identifier) @callee)

(call_expression
  function: (member_expression
    property: (property_identifier) @callee))
`
		q, qerr := tree_sitter.NewQuery(tsLanguage(), src)
		if qerr != nil {
			return
		}
		idx, ok := q.CaptureIndexForName("callee")
		if !ok {
			q.Close()
			return
		}
		tsCallerQuery = q
		tsCallerCapCallee = idx
	})
	return tsCallerQuery
}

func classQueries() (declared, parents *tree_sitter.Query) {
	tsClassQueryOnce.Do(func() {
		const declSrc = `
(class_declaration
  name: (type_identifier) @declared)

(abstract_class_declaration
  name: (type_identifier) @declared)

(interface_declaration
  name: (type_identifier) @declared)
`
		// Parents: extract identifier-shaped names from extends / implements
		// subtrees. Covers bare identifiers, qualified member-expression heads
		// (rare but possible: `extends ns.Foo`), and generic types
		// (`implements Foo<T>` → `Foo`).
		const parentsSrc = `
(extends_clause (identifier) @parent)
(extends_clause (member_expression property: (property_identifier) @parent))
(extends_type_clause (type_identifier) @parent)
(extends_type_clause (generic_type name: (type_identifier) @parent))
(extends_type_clause (generic_type name: (nested_type_identifier name: (type_identifier) @parent)))
(implements_clause (type_identifier) @parent)
(implements_clause (generic_type name: (type_identifier) @parent))
(implements_clause (generic_type name: (nested_type_identifier name: (type_identifier) @parent)))
`
		dq, derr := tree_sitter.NewQuery(tsLanguage(), declSrc)
		if derr != nil {
			return
		}
		pq, perr := tree_sitter.NewQuery(tsLanguage(), parentsSrc)
		if perr != nil {
			dq.Close()
			return
		}
		didx, ok1 := dq.CaptureIndexForName("declared")
		pidx, ok2 := pq.CaptureIndexForName("parent")
		if !ok1 || !ok2 {
			dq.Close()
			pq.Close()
			return
		}
		tsClassDeclaredQuery = dq
		tsClassParentsQuery = pq
		tsClassDeclaredCapName = didx
		tsClassParentsCapName = pidx
	})
	return tsClassDeclaredQuery, tsClassParentsQuery
}
