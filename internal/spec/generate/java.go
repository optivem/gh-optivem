package generate

import (
	"fmt"
	"slices"
)

// annotation is one Java annotation or C# attribute: its simple name and the
// tokens between its parentheses.
type annotation struct {
	name string
	args []token
	line int
}

var javaTestAnnotations = []string{"Test", "TestTemplate", "ParameterizedTest"}

// parseJava finds the JUnit test methods of a Java file.
func parseJava(file string, toks []token) ([]TestCase, []error) {
	var cases []TestCase
	var errs []error
	var pending, classAnnos []annotation
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.punct("@") && i+1 < len(toks) && toks[i+1].Kind == tokIdent && !toks[i+1].is(tokIdent, "interface"):
			a, next, err := readJavaAnnotation(toks, i)
			if err != nil {
				return nil, []error{ParseError{File: file, Line: t.Line, Reason: err.Error()}}
			}
			pending = append(pending, a)
			i = next - 1
		case t.is(tokIdent, "class"):
			classAnnos, pending = pending, nil
		case t.punct(";") || t.punct("{") || t.punct("}"):
			pending = nil
		case t.Kind == tokIdent && i+1 < len(toks) && toks[i+1].punct("(") && hasAnnotation(pending, javaTestAnnotations...):
			tc, end, err := parseJavaMethod(file, toks, i, append(slices.Clone(classAnnos), pending...))
			pending = nil
			if err != nil {
				errs = append(errs, err)
			} else {
				cases = append(cases, tc)
			}
			if end < 0 {
				return cases, errs
			}
			i = end
		}
	}
	return cases, errs
}

// readJavaAnnotation reads `@a.b.Name(args)` starting at toks[i] == "@" and
// returns the index after it.
func readJavaAnnotation(toks []token, i int) (annotation, int, error) {
	j := i + 1
	a := annotation{name: toks[j].Text, line: toks[i].Line}
	for j+2 < len(toks) && toks[j+1].punct(".") && toks[j+2].Kind == tokIdent {
		j += 2
		a.name = toks[j].Text
	}
	j++
	if j < len(toks) && toks[j].punct("(") {
		end := matching(toks, j)
		if end < 0 {
			return a, 0, fmt.Errorf("unbalanced parentheses in @%s", a.name)
		}
		a.args = toks[j+1 : end]
		j = end + 1
	}
	return a, j, nil
}

func hasAnnotation(annos []annotation, names ...string) bool {
	return slices.ContainsFunc(annos, func(a annotation) bool { return slices.Contains(names, a.name) })
}

// parseJavaMethod parses the method whose name is toks[i]. It returns the
// index of the body's closing brace (-1 when the file ends first).
func parseJavaMethod(file string, toks []token, i int, annos []annotation) (TestCase, int, error) {
	tc := TestCase{File: file, Line: toks[i].Line, Name: toks[i].Text}
	fail := func(end int, format string, a ...any) (TestCase, int, error) {
		return TestCase{}, end, ParseError{File: file, Line: tc.Line, Test: tc.Name, Reason: fmt.Sprintf(format, a...)}
	}
	closeParams := matching(toks, i+1)
	if closeParams < 0 {
		return fail(-1, "unbalanced parentheses in the parameter list")
	}
	tc.Params = paramNames(toks[i+2:closeParams], nil)
	open := closeParams + 1
	for open < len(toks) && !toks[open].punct("{") {
		open++
	}
	end := -1
	if open < len(toks) {
		end = matching(toks, open)
	}
	if end < 0 {
		return fail(-1, "unbalanced braces in the method body")
	}
	for _, a := range annos {
		if err := applyJavaAnnotation(&tc, a); err != nil {
			return fail(end, "%v", err)
		}
	}
	sections, wip, err := parseBody(toks[open+1:end], tc.Params, false)
	if err != nil {
		return fail(end, "%v", err)
	}
	tc.Sections, tc.Wip = sections, tc.Wip || wip
	if err := checkExamples(tc); err != nil {
		return fail(end, "%v", err)
	}
	return tc, end, nil
}

func applyJavaAnnotation(tc *TestCase, a annotation) error {
	switch a.name {
	case "Tag":
		lits := literals(a.args)
		if len(lits) != 1 {
			return fmt.Errorf("@Tag must have one string value")
		}
		addTag(tc, lits[0])
	case "EnabledIfEnvironmentVariable", "DisabledIfEnvironmentVariable":
		if mentionsWipGate(a.args) {
			tc.Wip = true
		}
	case "DataSource":
		tc.Examples.Rows = append(tc.Examples.Rows, literals(a.args))
	case "ValueSource":
		for _, v := range literals(a.args) {
			tc.Examples.Rows = append(tc.Examples.Rows, []string{v})
		}
	case "CsvSource":
		return fmt.Errorf("@CsvSource is not supported; use @DataSource rows")
	case "ArgumentsSource", "MethodSource":
		if len(a.args) == 0 {
			return fmt.Errorf("@%s without a source", a.name)
		}
		tc.Examples.Source = a.args[0].Text
	}
	return nil
}

// addTag records a criterion ID or the wip tag; other tags are ignored.
func addTag(tc *TestCase, tag string) {
	switch {
	case tag == "wip":
		tc.Wip = true
	case criterionIDPattern.MatchString(tag) && !slices.Contains(tc.Tags, tag):
		tc.Tags = append(tc.Tags, tag)
	}
}

// literals returns the cell text of every literal value among toks (brackets,
// commas and `name =` keys are skipped). A leading minus is folded in.
func literals(toks []token) []string {
	var out []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if i+1 < len(toks) && toks[i+1].punct("=") && t.Kind == tokIdent {
			i++
			continue
		}
		switch {
		case t.punct("-") && i+1 < len(toks) && toks[i+1].Kind == tokNumber:
			out = append(out, "-"+stripNumberSuffix(toks[i+1].Text))
			i++
		case t.Kind == tokString || t.Kind == tokTemplate:
			out = append(out, unescape(t.Text))
		case t.Kind == tokNumber:
			out = append(out, stripNumberSuffix(t.Text))
		case t.Kind == tokIdent && (t.Text == "null" || t.Text == "undefined"):
			out = append(out, "none")
		case t.Kind == tokIdent && (t.Text == "true" || t.Text == "false"):
			out = append(out, t.Text)
		case t.Kind == tokIdent && i+1 < len(toks) && toks[i+1].punct("."):
			// enum path: keep its last segment
			for i+2 < len(toks) && toks[i+1].punct(".") && toks[i+2].Kind == tokIdent {
				i += 2
			}
			out = append(out, toks[i].Text)
		}
	}
	return out
}

// paramNames returns the parameter names of a Java/C# parameter list (the last
// identifier of each comma part), skipping parts whose type is in skipTypes.
func paramNames(toks []token, skipTypes []string) []string {
	var names []string
	for _, part := range splitTop(toks, ",") {
		var idents []string
		for k := 0; k < len(part); k++ {
			if part[k].punct("@") || part[k].punct("[") {
				// annotation/attribute on the parameter: skip it
				if part[k].punct("[") {
					if e := matching(part, k); e > 0 {
						k = e
					}
				}
				continue
			}
			if part[k].Kind == tokIdent {
				idents = append(idents, part[k].Text)
			}
		}
		if len(idents) == 0 {
			continue
		}
		if len(idents) >= 2 && slices.Contains(skipTypes, idents[len(idents)-2]) {
			continue
		}
		names = append(names, idents[len(idents)-1])
	}
	return names
}

// checkExamples rejects a parameterised test whose rows don't match its
// parameters, and rows without parameters.
func checkExamples(tc TestCase) error {
	if len(tc.Params) == 0 {
		if len(tc.Examples.Rows) > 0 || tc.Examples.Source != "" {
			return fmt.Errorf("data rows on a test without parameters")
		}
		return nil
	}
	if len(tc.Examples.Rows) == 0 && tc.Examples.Source == "" {
		return fmt.Errorf("parameters %v but no data rows or source", tc.Params)
	}
	for _, r := range tc.Examples.Rows {
		if len(r) != len(tc.Params) {
			return fmt.Errorf("data row %v has %d values for %d parameters", r, len(r), len(tc.Params))
		}
	}
	return nil
}
