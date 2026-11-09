package generate

import (
	"fmt"
	"slices"
)

var csharpTestAttributes = []string{"Fact", "Theory", "SkippableFact", "SkippableTheory"}

// csharpSkipParamTypes are parameter types that carry execution context, not
// scenario data (the channel matrix is not rendered).
var csharpSkipParamTypes = []string{"Channel"}

// parseCSharp finds the xUnit test methods of a C# file.
func parseCSharp(file string, toks []token) ([]TestCase, []error) {
	var cases []TestCase
	var errs []error
	var pending, classAttrs []annotation
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch {
		case t.punct("[") && attributePosition(toks, i):
			end := matching(toks, i)
			if end < 0 {
				return nil, []error{ParseError{File: file, Line: t.Line, Reason: "unbalanced brackets in an attribute"}}
			}
			pending = append(pending, readCSharpAttributes(toks[i+1:end])...)
			i = end
		case t.is(tokIdent, "class") || t.is(tokIdent, "record"):
			classAttrs, pending = pending, nil
		case t.punct(";") || t.punct("{") || t.punct("}"):
			pending = nil
		case t.Kind == tokIdent && i+1 < len(toks) && toks[i+1].punct("(") && hasAnnotation(pending, csharpTestAttributes...):
			tc, end, err := parseCSharpMethod(file, toks, i, append(slices.Clone(classAttrs), pending...))
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

// attributePosition reports whether the "[" at toks[i] opens an attribute
// list: it starts a declaration (after ; { } ] or at the file start) and is
// followed by a name.
func attributePosition(toks []token, i int) bool {
	if i+1 >= len(toks) || toks[i+1].Kind != tokIdent {
		return false
	}
	if i == 0 {
		return true
	}
	p := toks[i-1]
	return p.punct(";") || p.punct("{") || p.punct("}") || p.punct("]")
}

// readCSharpAttributes splits `A(x), B` into attributes.
func readCSharpAttributes(toks []token) []annotation {
	var out []annotation
	for _, part := range splitTop(toks, ",") {
		if len(part) == 0 || part[0].Kind != tokIdent {
			continue
		}
		j := 0
		for j+2 < len(part) && part[j+1].punct(".") && part[j+2].Kind == tokIdent {
			j += 2
		}
		a := annotation{name: part[j].Text, line: part[0].Line}
		if j+1 < len(part) && part[j+1].punct("(") {
			if end := matching(part, j+1); end > 0 {
				a.args = part[j+2 : end]
			}
		}
		out = append(out, a)
	}
	return out
}

func parseCSharpMethod(file string, toks []token, i int, attrs []annotation) (TestCase, int, error) {
	tc := TestCase{File: file, Line: toks[i].Line, Name: toks[i].Text}
	fail := func(end int, format string, a ...any) (TestCase, int, error) {
		return TestCase{}, end, ParseError{File: file, Line: tc.Line, Test: tc.Name, Reason: fmt.Sprintf(format, a...)}
	}
	closeParams := matching(toks, i+1)
	if closeParams < 0 {
		return fail(-1, "unbalanced parentheses in the parameter list")
	}
	tc.Params = paramNames(toks[i+2:closeParams], csharpSkipParamTypes)
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
	for _, a := range attrs {
		if err := applyCSharpAttribute(&tc, a); err != nil {
			return fail(end, "%v", err)
		}
	}
	sections, wip, err := parseBody(toks[open+1:end], tc.Params, true)
	if err != nil {
		return fail(end, "%v", err)
	}
	tc.Sections, tc.Wip = sections, tc.Wip || wip
	if err := checkExamples(tc); err != nil {
		return fail(end, "%v", err)
	}
	return tc, end, nil
}

func applyCSharpAttribute(tc *TestCase, a annotation) error {
	switch a.name {
	case "Trait":
		lits := literals(a.args)
		if len(lits) != 2 {
			return fmt.Errorf("[Trait] must have a name and a value")
		}
		if lits[0] == "tag" || lits[0] == "criterion" {
			addTag(tc, lits[1])
		}
	case "InlineData", "ChannelInlineData":
		tc.Examples.Rows = append(tc.Examples.Rows, literals(a.args))
	case "ClassData", "ChannelClassData", "MemberData", "ChannelMemberData":
		src := ""
		for _, t := range a.args {
			if t.Kind == tokIdent && t.Text != "typeof" && t.Text != "nameof" || t.Kind == tokString {
				src = t.Text
				break
			}
		}
		if src == "" {
			return fmt.Errorf("[%s] without a source", a.name)
		}
		tc.Examples.Source = src
	}
	return nil
}
