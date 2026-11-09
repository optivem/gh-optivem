package generate

import (
	"fmt"
	"slices"
	"strings"
)

// wipGateVar is the environment variable the ATDD WIP gate keys on, in every
// language's gate form.
const wipGateVar = "GH_OPTIVEM_RUN_WIP_TESTS"

// parseBody turns a test body into sections. The body holds exactly one
// scenario chain statement, optionally preceded by local constants bound to
// literals (inlined into the chain) and the WIP gate statement (which marks
// the test wip). Anything else is rejected. pascal converts PascalCase call
// names (C#) to lowerCamel.
func parseBody(body []token, params []string, pascal bool) ([]Section, bool, error) {
	locals := map[string]Arg{}
	var sections []Section
	wip, found := false, false
	for _, stmt := range splitTop(body, ";") {
		if len(stmt) == 0 {
			continue
		}
		if !parensBalanced(stmt) {
			return nil, false, fmt.Errorf("line %d: unbalanced parentheses", stmt[0].Line)
		}
		if mentionsWipGate(stmt) {
			wip = true
			continue
		}
		if name, arg, ok := localConstant(stmt, params, locals); ok {
			locals[name] = arg
			continue
		}
		if found {
			return nil, false, fmt.Errorf("line %d: unexpected second statement `%s`; the body must be a single scenario chain", stmt[0].Line, abbreviate(stmt))
		}
		s, err := parseChain(stmt, params, locals, pascal)
		if err != nil {
			return nil, false, err
		}
		sections, found = s, true
	}
	if !found {
		return nil, false, fmt.Errorf("body has no scenario chain")
	}
	return sections, wip, nil
}

func parensBalanced(stmt []token) bool {
	depth := 0
	for _, t := range stmt {
		switch {
		case t.punct("("):
			depth++
		case t.punct(")"):
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func mentionsWipGate(stmt []token) bool {
	for _, t := range stmt {
		if t.Text == wipGateVar || t.Kind == tokString && strings.Contains(t.Text, wipGateVar) {
			return true
		}
	}
	return false
}

// localConstant matches `[const|let|var|final|Type ...] name = <literal>`.
func localConstant(stmt []token, params []string, locals map[string]Arg) (string, Arg, bool) {
	eq := slices.IndexFunc(stmt, func(t token) bool { return t.punct("=") })
	if eq < 1 {
		return "", Arg{}, false
	}
	for _, t := range stmt[:eq] {
		if t.Kind != tokIdent {
			return "", Arg{}, false
		}
	}
	arg, err := parseArg(stmt[eq+1:], params, locals)
	if err != nil || arg.Kind == ArgParam {
		return "", Arg{}, false
	}
	return stmt[eq-1].Text, arg, true
}

type rawCall struct {
	name string
	args []Arg
	line int
}

// parseChain parses `[await] scenario[(...)] .x(...) .y(...) ...`.
func parseChain(stmt []token, params []string, locals map[string]Arg, pascal bool) ([]Section, error) {
	i := 0
	if stmt[i].is(tokIdent, "await") {
		i++
	}
	if i >= len(stmt) || stmt[i].Kind != tokIdent || !strings.EqualFold(stmt[i].Text, "scenario") {
		return nil, fmt.Errorf("line %d: statement `%s` is not a scenario chain", stmt[0].Line, abbreviate(stmt))
	}
	i++
	if i < len(stmt) && stmt[i].punct("(") {
		end := matching(stmt, i)
		if end < 0 {
			return nil, fmt.Errorf("line %d: unbalanced parentheses", stmt[i].Line)
		}
		i = end + 1
	}
	var calls []rawCall
	for i < len(stmt) {
		if !stmt[i].punct(".") || i+2 >= len(stmt) || stmt[i+1].Kind != tokIdent || !stmt[i+2].punct("(") {
			return nil, fmt.Errorf("line %d: unexpected `%s` in scenario chain", stmt[i].Line, stmt[i])
		}
		name, open := stmt[i+1].Text, i+2
		end := matching(stmt, open)
		if end < 0 {
			return nil, fmt.Errorf("line %d: unbalanced parentheses in `.%s(`", stmt[open].Line, name)
		}
		var args []Arg
		for _, a := range splitTop(stmt[open+1:end], ",") {
			arg, err := parseArg(a, params, locals)
			if err != nil {
				return nil, fmt.Errorf("line %d: `.%s(...)`: %w", stmt[open].Line, name, err)
			}
			args = append(args, arg)
		}
		if pascal {
			name = lowerFirst(name)
		}
		calls = append(calls, rawCall{name, args, stmt[i+1].Line})
		i = end + 1
	}
	return buildSections(calls)
}

var keywordRank = map[string]int{KeywordGiven: 0, KeywordWhen: 1, KeywordThen: 2}

func buildSections(calls []rawCall) ([]Section, error) {
	var sections []Section
	needSubject := false
	hasThen := false
	for _, c := range calls {
		_, keyword := keywordRank[c.name]
		if len(c.args) == 0 && (c.name == "and" || keyword) {
			if needSubject {
				return nil, fmt.Errorf("line %d: `.%s()` directly after a section keyword", c.line, c.name)
			}
			if c.name == "and" {
				if len(sections) == 0 {
					return nil, fmt.Errorf("line %d: `.and()` before any section", c.line)
				}
				sections = append(sections, Section{Keyword: sections[len(sections)-1].Keyword, Continuation: true})
			} else {
				if len(sections) > 0 && keywordRank[c.name] < keywordRank[sections[len(sections)-1].Keyword] {
					return nil, fmt.Errorf("line %d: `.%s()` after `.%s()`", c.line, c.name, sections[len(sections)-1].Keyword)
				}
				hasThen = hasThen || c.name == KeywordThen
				sections = append(sections, Section{Keyword: c.name})
			}
			needSubject = true
			continue
		}
		if len(sections) == 0 {
			return nil, fmt.Errorf("line %d: unknown section keyword `.%s()`; a chain starts with .given(), .when() or .then()", c.line, c.name)
		}
		s := &sections[len(sections)-1]
		call := Call{Name: c.name, Args: c.args}
		if needSubject {
			s.Subject, needSubject = call, false
		} else {
			s.Modifiers = append(s.Modifiers, call)
		}
	}
	if needSubject {
		return nil, fmt.Errorf("chain ends with a section keyword that has no step")
	}
	if !hasThen {
		return nil, fmt.Errorf("chain has no .then()")
	}
	return sections, nil
}

// parseArg accepts a literal, a parameter or local name, a bare identifier
// (an imported constant) or a dotted path (an enum value or constant).
func parseArg(toks []token, params []string, locals map[string]Arg) (Arg, error) {
	if len(toks) == 0 {
		return Arg{}, fmt.Errorf("empty argument")
	}
	if len(toks) == 1 {
		t := toks[0]
		switch t.Kind {
		case tokString:
			return Arg{ArgString, unescape(t.Text)}, nil
		case tokTemplate:
			if !strings.Contains(t.Text, "${") && !strings.Contains(t.Text, "{") {
				return Arg{ArgString, unescape(t.Text)}, nil
			}
			return Arg{ArgTemplate, interpolate(t.Text, params, locals)}, nil
		case tokNumber:
			return Arg{ArgNumber, stripNumberSuffix(t.Text)}, nil
		case tokIdent:
			switch t.Text {
			case "null", "undefined":
				return Arg{ArgNull, "none"}, nil
			case "true", "false":
				return Arg{ArgBool, t.Text}, nil
			}
			if slices.Contains(params, t.Text) {
				return Arg{ArgParam, t.Text}, nil
			}
			if a, ok := locals[t.Text]; ok {
				return a, nil
			}
			return Arg{ArgIdentifier, t.Text}, nil
		}
	}
	if len(toks) == 2 && toks[0].punct("-") && toks[1].Kind == tokNumber {
		return Arg{ArgNumber, "-" + stripNumberSuffix(toks[1].Text)}, nil
	}
	if dotted(toks) {
		return Arg{ArgEnum, toks[len(toks)-1].Text}, nil
	}
	if a, ok := valueExpression(toks, params, locals); ok {
		return a, nil
	}
	reason := "unsupported argument"
	switch {
	case toks[0].is(tokIdent, "new"):
		reason = "unsupported `new` argument"
	case slices.ContainsFunc(toks, func(t token) bool { return t.punct("=>") || t.punct("->") }):
		reason = "unsupported lambda argument"
	}
	return Arg{}, fmt.Errorf("%s `%s`", reason, abbreviate(toks))
}

func dotted(toks []token) bool {
	if len(toks) < 3 || len(toks)%2 == 0 {
		return false
	}
	for i, t := range toks {
		if i%2 == 0 && t.Kind != tokIdent || i%2 == 1 && !t.punct(".") {
			return false
		}
	}
	return true
}

// interpolate rewrites `${x}` (TS) and `{x}` (C#) placeholders: a parameter
// becomes <x>, a local constant its value; anything else is kept as written.
func interpolate(text string, params []string, locals map[string]Arg) string {
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		start := -1
		switch {
		case strings.HasPrefix(text[i:], "${"):
			start = i + 2
		case text[i] == '{':
			start = i + 1
		}
		if start < 0 {
			b.WriteByte(text[i])
			continue
		}
		end := strings.IndexByte(text[start:], '}')
		if end < 0 {
			b.WriteString(text[i:])
			break
		}
		expr := strings.TrimSpace(text[start : start+end])
		switch a, ok := locals[expr]; {
		case slices.Contains(params, expr):
			b.WriteString("<" + expr + ">")
		case ok:
			b.WriteString(a.Text)
		default:
			b.WriteString(text[i : start+end+1])
		}
		i = start + end
	}
	return unescape(b.String())
}

func stripNumberSuffix(s string) string {
	return strings.TrimRight(s, "mMlLfFdDuU")
}

// unescape resolves the escapes that matter for prose (quotes, backslash);
// others are kept as written.
func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\'`, `'`, `\"`, `"`, "\\`", "`", `\\`, `\`).Replace(s)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func abbreviate(toks []token) string {
	s := joinTokens(toks)
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}

// conversionCalls are the standard-library value conversions a test wraps
// around a value to satisfy a parameter type (OrderStatus.valueOf(s),
// Number.parseFloat(s), decimal.Parse(s, ...)). They render as the value.
var conversionCalls = []string{"valueOf", "parse", "Parse", "parseFloat", "parseInt", "Number", "String"}

// valueExpression accepts the expression forms that only restate a value:
// a C# null-forgiving `x!`, a cast `(T) x`, parentheses, a conversion call,
// and string concatenation (rendered as one template, identifiers as ${x}).
func valueExpression(toks []token, params []string, locals map[string]Arg) (Arg, bool) {
	if n := len(toks); n > 1 && toks[n-1].punct("!") {
		a, err := parseArg(toks[:n-1], params, locals)
		return a, err == nil
	}
	if toks[0].punct("(") {
		end := matching(toks, 0)
		switch {
		case end == len(toks)-1:
			a, err := parseArg(toks[1:end], params, locals)
			return a, err == nil
		case end == 2 && toks[1].Kind == tokIdent:
			a, err := parseArg(toks[3:], params, locals)
			return a, err == nil
		}
	}
	if open := slices.IndexFunc(toks, func(t token) bool { return t.punct("(") }); open > 0 &&
		matching(toks, open) == len(toks)-1 && (open == 1 || dotted(toks[:open])) &&
		slices.Contains(conversionCalls, toks[open-1].Text) {
		inner := splitTop(toks[open+1:len(toks)-1], ",")
		if len(inner) == 0 {
			return Arg{}, false
		}
		a, err := parseArg(inner[0], params, locals)
		return a, err == nil
	}
	parts := splitTop(toks, "+")
	if len(parts) < 2 {
		return Arg{}, false
	}
	var b strings.Builder
	for _, p := range parts {
		a, err := parseArg(p, params, locals)
		if err != nil {
			return Arg{}, false
		}
		switch a.Kind {
		case ArgString, ArgTemplate, ArgNumber, ArgBool:
			b.WriteString(a.Text)
		case ArgParam:
			b.WriteString("<" + a.Text + ">")
		case ArgIdentifier, ArgEnum:
			b.WriteString("${" + a.Text + "}")
		default:
			return Arg{}, false
		}
	}
	return Arg{ArgTemplate, b.String()}, true
}
