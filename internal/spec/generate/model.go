// Package generate turns tagged acceptance tests (Java, .NET, TypeScript)
// into readable Gherkin: one Feature per use case, one scenario per test
// and criterion ID. Each language front-end fills the language-neutral
// model in this file; one renderer turns the model into Gherkin.
package generate

import "fmt"

// Section keywords.
const (
	KeywordGiven = "given"
	KeywordWhen  = "when"
	KeywordThen  = "then"
)

// Argument kinds.
const (
	ArgString     = "string"
	ArgNumber     = "number"
	ArgBool       = "bool"
	ArgNull       = "null"
	ArgEnum       = "enum"
	ArgIdentifier = "identifier"
	ArgParam      = "param"
	ArgTemplate   = "template"
)

// TestCase is one acceptance test method, normalised across languages.
type TestCase struct {
	File string
	Line int
	// Name is the test name as written (method name, or the TS title with
	// its `_$param` suffix stripped).
	Name string
	// Tags are the criterion IDs (UC-NNN.M) the test carries, in source order.
	Tags []string
	Wip  bool
	// Params are the outline parameter names (empty for a plain scenario).
	Params   []string
	Examples Examples
	Sections []Section
}

// Examples are the rows of a parameterised test. Source names a provider
// (class, method or imported value) whose rows live outside the file; it is
// rendered as a comment instead of an Examples table.
type Examples struct {
	Rows   [][]string
	Source string
}

// Section is one given/when/then step group. Continuation marks a section
// opened by `.and()` inside the previous keyword's group.
type Section struct {
	Keyword      string
	Continuation bool
	Subject      Call
	Modifiers    []Call
}

// Call is one DSL method call; Name is lowerCamelCase.
type Call struct {
	Name string
	Args []Arg
}

// Arg is one literal-ish call argument. Text is the value as it is rendered:
// string and template contents without quotes, numbers without type
// suffixes, an enum's last segment, a parameter or identifier name.
type Arg struct {
	Kind string
	Text string
}

// ParseError is one test the grammar rejects.
type ParseError struct {
	File   string
	Line   int
	Test   string
	Reason string
}

func (e ParseError) Error() string {
	if e.Test == "" {
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Reason)
	}
	return fmt.Sprintf("%s:%d %s: %s", e.File, e.Line, e.Test, e.Reason)
}
