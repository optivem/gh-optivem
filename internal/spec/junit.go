package spec

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Status is the outcome of a single test result.
type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

// WipTag is the tag value that marks a test as in progress.
const WipTag = "wip"

// Result is one executed result of a test, for example one channel of a templated test.
type Result struct {
	Name   string
	Status Status
}

// Test is one test method with all of its results aggregated.
type Test struct {
	Class   string
	Method  string
	IDs     []string
	WIP     bool
	Results []Result
}

// Status aggregates all results: failed if any failed, otherwise skipped if any
// was skipped or there are no results, otherwise passed.
func (t Test) Status() Status {
	if len(t.Results) == 0 {
		return StatusSkipped
	}
	status := StatusPassed
	for _, r := range t.Results {
		switch r.Status {
		case StatusFailed:
			return StatusFailed
		case StatusSkipped:
			status = StatusSkipped
		}
	}
	return status
}

var (
	tagIDPattern     = regexp.MustCompile(`^UC-\d+\.\d+$`)
	templateSuffixRe = regexp.MustCompile(`\s*\[[^\]]*\]\s*$`)
)

type xmlSuite struct {
	Suites []xmlSuite `xml:"testsuite"`
	Cases  []xmlCase  `xml:"testcase"`
}

type xmlCase struct {
	Name       string        `xml:"name,attr"`
	ClassName  string        `xml:"classname,attr"`
	Properties []xmlProperty `xml:"properties>property"`
	Failures   []struct{}    `xml:"failure"`
	Errors     []struct{}    `xml:"error"`
	Skipped    []struct{}    `xml:"skipped"`
}

type xmlProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// NormalizeTestName strips a trailing " [...]" suffix, as produced by
// templated or parameterised tests (for example "method [Channel: UI]").
func NormalizeTestName(name string) string {
	return strings.TrimSpace(templateSuffixRe.ReplaceAllString(name, ""))
}

// ParseJUnit parses one JUnit XML document (Gradle or Surefire; a single
// testsuite or a testsuites wrapper) into tests, aggregating results by
// class and normalised method name.
func ParseJUnit(r io.Reader) ([]Test, error) {
	var root xmlSuite
	if err := xml.NewDecoder(r).Decode(&root); err != nil {
		return nil, fmt.Errorf("parse JUnit XML: %w", err)
	}
	agg := newAggregator()
	agg.addSuite(root)
	return agg.tests(), nil
}

// ReadJUnitFiles reads and merges several JUnit XML files. A test appearing in
// more than one file has its results combined.
func ReadJUnitFiles(paths []string) ([]Test, error) {
	agg := newAggregator()
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", p, err)
		}
		var root xmlSuite
		err = xml.NewDecoder(f).Decode(&root)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("parse JUnit XML %s: %w", p, err)
		}
		agg.addSuite(root)
	}
	return agg.tests(), nil
}

type aggregator struct {
	byKey map[string]*Test
	order []string
}

func newAggregator() *aggregator {
	return &aggregator{byKey: map[string]*Test{}}
}

func (a *aggregator) addSuite(s xmlSuite) {
	for _, c := range s.Cases {
		a.addCase(c)
	}
	for _, child := range s.Suites {
		a.addSuite(child)
	}
}

func (a *aggregator) addCase(c xmlCase) {
	method := NormalizeTestName(c.Name)
	key := c.ClassName + "#" + method
	t, ok := a.byKey[key]
	if !ok {
		t = &Test{Class: c.ClassName, Method: method}
		a.byKey[key] = t
		a.order = append(a.order, key)
	}
	status := StatusPassed
	switch {
	case len(c.Failures) > 0 || len(c.Errors) > 0:
		status = StatusFailed
	case len(c.Skipped) > 0:
		status = StatusSkipped
	}
	t.Results = append(t.Results, Result{Name: c.Name, Status: status})

	for _, p := range c.Properties {
		if p.Name != "tag" && p.Name != "criterion" {
			continue
		}
		v := strings.TrimPrefix(strings.TrimSpace(p.Value), "@")
		switch {
		case v == WipTag:
			t.WIP = true
		case tagIDPattern.MatchString(v):
			t.IDs = appendUnique(t.IDs, v)
		}
	}
}

func (a *aggregator) tests() []Test {
	out := make([]Test, 0, len(a.order))
	for _, k := range a.order {
		t := *a.byKey[k]
		sort.Strings(t.IDs)
		out = append(out, t)
	}
	return out
}

func appendUnique(list []string, v string) []string {
	if slices.Contains(list, v) {
		return list
	}
	return append(list, v)
}
