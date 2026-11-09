package generate

import (
	"fmt"
	"slices"
	"strings"
)

// tsFixtureParams are Playwright fixtures destructured by a test callback;
// they are not outline parameters.
var tsFixtureParams = []string{"scenario", "page", "request", "context", "browser"}

// tsLoop is a `xs.forEach((x) => { ... })` block: tests inside it are
// parameterised by x over xs.
type tsLoop struct {
	param string
	data  []token // the receiver: one identifier or an inline array
	end   int     // token index of the forEach call's closing parenthesis
}

// parseTypeScript finds the Playwright tests of a TS spec file.
func parseTypeScript(file string, toks []token) ([]TestCase, []error) {
	consts := tsArrayConsts(toks)
	var cases []TestCase
	var errs []error
	var loops []tsLoop
	for i := 0; i < len(toks); i++ {
		for len(loops) > 0 && i > loops[len(loops)-1].end {
			loops = loops[:len(loops)-1]
		}
		if l, ok := tsForEach(toks, i); ok {
			loops = append(loops, l)
			continue
		}
		t := toks[i]
		if !t.is(tokIdent, "test") || i > 0 && toks[i-1].punct(".") {
			continue
		}
		var data []token
		hasData := false
		open := i + 1
		if open+2 < len(toks) && toks[open].punct(".") && (toks[open+1].is(tokIdent, "each") || toks[open+1].is(tokIdent, "eachAlsoFirstRow")) && toks[open+2].punct("(") {
			end := matching(toks, open+2)
			if end < 0 {
				return cases, append(errs, ParseError{File: file, Line: t.Line, Reason: "unbalanced parentheses in test.each"})
			}
			data, hasData = toks[open+3:end], true
			open = end + 1
		}
		if open >= len(toks) || !toks[open].punct("(") {
			continue // test.describe, test.skip, test.extend, ...
		}
		end := matching(toks, open)
		if end < 0 {
			return cases, append(errs, ParseError{File: file, Line: t.Line, Reason: "unbalanced parentheses in test("})
		}
		var loop *tsLoop
		if len(loops) > 0 {
			loop = &loops[len(loops)-1]
		}
		tc, err := parseTSTest(file, t.Line, toks[open+1:end], data, hasData, loop, consts)
		if err != nil {
			errs = append(errs, err)
		} else {
			cases = append(cases, tc)
		}
		i = end
	}
	return cases, errs
}

// tsArrayConsts maps `const name = [ ... ]` declarations to their element tokens.
func tsArrayConsts(toks []token) map[string][]token {
	out := map[string][]token{}
	for i := 0; i+3 < len(toks); i++ {
		if toks[i].is(tokIdent, "const") && toks[i+1].Kind == tokIdent && toks[i+2].punct("=") && toks[i+3].punct("[") {
			if end := matching(toks, i+3); end > 0 {
				out[toks[i+1].Text] = toks[i+3 : end+1]
			}
		}
	}
	return out
}

// tsForEach matches `<recv>.forEach((x) => {` or `<recv>.forEach(x => {` with
// the "." at toks[i].
func tsForEach(toks []token, i int) (tsLoop, bool) {
	if i == 0 || i+5 >= len(toks) || !toks[i].punct(".") || !toks[i+1].is(tokIdent, "forEach") || !toks[i+2].punct("(") {
		return tsLoop{}, false
	}
	end := matching(toks, i+2)
	if end < 0 {
		return tsLoop{}, false
	}
	var param string
	switch {
	case toks[i+3].punct("(") && toks[i+4].Kind == tokIdent && toks[i+5].punct(")"):
		param = toks[i+4].Text
	case toks[i+3].Kind == tokIdent && toks[i+4].punct("=>"):
		param = toks[i+3].Text
	default:
		return tsLoop{}, false
	}
	var data []token
	switch prev := toks[i-1]; {
	case prev.Kind == tokIdent:
		data = toks[i-1 : i]
	case prev.punct("]"):
		for s := i - 2; s >= 0; s-- {
			if toks[s].punct("[") && matching(toks, s) == i-1 {
				data = toks[s:i]
				break
			}
		}
	}
	if data == nil {
		return tsLoop{}, false
	}
	return tsLoop{param: param, data: data, end: end}, true
}

func parseTSTest(file string, line int, args, data []token, hasData bool, loop *tsLoop, consts map[string][]token) (TestCase, error) {
	tc := TestCase{File: file, Line: line}
	fail := func(format string, a ...any) (TestCase, error) {
		return TestCase{}, ParseError{File: file, Line: line, Test: tc.Name, Reason: fmt.Sprintf(format, a...)}
	}
	parts := splitTop(args, ",")
	if len(parts) < 2 || len(parts[0]) != 1 || parts[0][0].Kind != tokString && parts[0][0].Kind != tokTemplate {
		return fail("test( must start with a title string and end with the test callback")
	}
	tc.Name = stripTitleParams(parts[0][0].Text)
	for _, d := range parts[1 : len(parts)-1] {
		if len(d) < 3 || !d[0].is(tokIdent, "criteria") || !d[1].punct("(") {
			return fail("unsupported test detail `%s`; tag criteria with criteria('UC-N.M', ...)", abbreviate(d))
		}
		for _, id := range literals(d[2 : len(d)-1]) {
			addTag(&tc, id)
		}
	}
	params, body, err := tsCallback(parts[len(parts)-1])
	if err != nil {
		return fail("%v", err)
	}
	tc.Params = params
	switch {
	case hasData:
		tc.Examples, err = tsExamples(data, tc.Params, consts)
	case loop != nil:
		tc.Params = append(tc.Params, loop.param)
		tc.Examples, err = tsExamples(loop.data, []string{loop.param}, consts)
	}
	if err != nil {
		return fail("%v", err)
	}
	sections, wip, err := parseBody(body, tc.Params, false)
	if err != nil {
		return fail("%v", err)
	}
	tc.Sections, tc.Wip = sections, tc.Wip || wip
	if err := checkExamples(tc); err != nil {
		return fail("%v", err)
	}
	return tc, nil
}

// stripTitleParams removes a `_$param` / `_"${param}"` suffix from a
// parameterised title: everything from the last "_" before the first "$".
func stripTitleParams(title string) string {
	d := strings.IndexByte(title, '$')
	if d < 0 {
		return title
	}
	if u := strings.LastIndexByte(title[:d], '_'); u > 0 {
		return title[:u]
	}
	return title[:d]
}

// tsCallback parses `async ({ scenario, a }) => { body }` into the non-fixture
// parameter names and the body tokens.
func tsCallback(toks []token) ([]string, []token, error) {
	arrow := slices.IndexFunc(toks, func(t token) bool { return t.punct("=>") })
	if arrow < 0 || arrow+1 >= len(toks) || !toks[arrow+1].punct("{") || !toks[len(toks)-1].punct("}") {
		return nil, nil, fmt.Errorf("the last test( argument must be an arrow function with a block body")
	}
	if matching(toks, arrow+1) != len(toks)-1 {
		return nil, nil, fmt.Errorf("unbalanced braces in the test body")
	}
	var params []string
	for _, t := range toks[:arrow] {
		if t.Kind == tokIdent && t.Text != "async" && !slices.Contains(tsFixtureParams, t.Text) {
			params = append(params, t.Text)
		}
	}
	return params, toks[arrow+2 : len(toks)-1], nil
}

// tsExamples resolves test.each data: an inline array, or the name of a
// same-file const array; any other name is an outside source.
func tsExamples(data []token, params []string, consts map[string][]token) (Examples, error) {
	if len(data) == 1 && data[0].Kind == tokIdent {
		arr, ok := consts[data[0].Text]
		if !ok {
			return Examples{Source: data[0].Text}, nil
		}
		data = arr
	}
	if len(data) < 2 || !data[0].punct("[") || matching(data, 0) != len(data)-1 {
		return Examples{}, fmt.Errorf("unsupported test data `%s`; use an array literal or a same-file const array", abbreviate(data))
	}
	var ex Examples
	for _, el := range splitTop(data[1:len(data)-1], ",") {
		if len(el) > 0 && el[0].punct("{") {
			row, err := tsObjectRow(el, params)
			if err != nil {
				return Examples{}, err
			}
			ex.Rows = append(ex.Rows, row)
			continue
		}
		if len(params) != 1 {
			return Examples{}, fmt.Errorf("scalar test data needs exactly one parameter, got %v", params)
		}
		cells := literals(el)
		if len(cells) != 1 {
			return Examples{}, fmt.Errorf("unsupported test data value `%s`", abbreviate(el))
		}
		ex.Rows = append(ex.Rows, cells)
	}
	return ex, nil
}

// tsObjectRow reads `{ k: v, ... }` into cells ordered by params.
func tsObjectRow(el []token, params []string) ([]string, error) {
	if matching(el, 0) != len(el)-1 {
		return nil, fmt.Errorf("unsupported test data value `%s`", abbreviate(el))
	}
	vals := map[string]string{}
	for _, kv := range splitTop(el[1:len(el)-1], ",") {
		if len(kv) < 3 || kv[0].Kind != tokIdent && kv[0].Kind != tokString || !kv[1].punct(":") {
			return nil, fmt.Errorf("unsupported test data entry `%s`", abbreviate(kv))
		}
		cells := literals(kv[2:])
		if len(cells) != 1 {
			return nil, fmt.Errorf("unsupported test data value `%s`", abbreviate(kv[2:]))
		}
		vals[kv[0].Text] = cells[0]
	}
	row := make([]string, len(params))
	for i, p := range params {
		v, ok := vals[p]
		if !ok {
			return nil, fmt.Errorf("test data row has no value for parameter %q", p)
		}
		row[i] = v
	}
	return row, nil
}
