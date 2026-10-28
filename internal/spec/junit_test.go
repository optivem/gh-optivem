package spec

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []Test {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tests, err := ParseJUnit(f)
	if err != nil {
		t.Fatal(err)
	}
	return tests
}

func TestParseJUnitFixtures(t *testing.T) {
	cases := []struct {
		file    string
		method  string
		ids     []string
		results int
		status  Status
		count   int
	}{
		{"junit-all-pass.xml", "shouldPlaceOrder", []string{"UC-001.1"}, 1, StatusPassed, 2},
		{"junit-one-red.xml", "shouldRejectEmpty", []string{"UC-001.2"}, 1, StatusFailed, 2},
		{"junit-skipped.xml", "shouldPlaceOrder", []string{"UC-001.1"}, 1, StatusSkipped, 1},
		{"junit-templated.xml", "shouldPlaceOrder", []string{"UC-001.1"}, 2, StatusFailed, 2},
		{"junit-unknown-id.xml", "shouldDoGhost", []string{"UC-999.1"}, 1, StatusPassed, 1},
		{"junit-orphan.xml", "shouldHaveNoTag", nil, 1, StatusPassed, 1},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			tests := readFixture(t, c.file)
			if len(tests) != c.count {
				t.Fatalf("got %d tests, want %d", len(tests), c.count)
			}
			var got *Test
			for i := range tests {
				if tests[i].Method == c.method {
					got = &tests[i]
				}
			}
			if got == nil {
				t.Fatalf("method %q not found in %+v", c.method, tests)
			}
			if !reflect.DeepEqual(got.IDs, c.ids) {
				t.Errorf("ids = %v, want %v", got.IDs, c.ids)
			}
			if got.WIP {
				t.Error("unexpected wip")
			}
			if len(got.Results) != c.results {
				t.Errorf("results = %d, want %d", len(got.Results), c.results)
			}
			if got.Status() != c.status {
				t.Errorf("status = %s, want %s", got.Status(), c.status)
			}
		})
	}
}

func TestTemplatedWipTag(t *testing.T) {
	for _, tc := range readFixture(t, "junit-templated.xml") {
		if tc.Method == "shouldWip" {
			if !tc.WIP || !reflect.DeepEqual(tc.IDs, []string{"UC-001.3"}) {
				t.Errorf("got %+v", tc)
			}
			return
		}
	}
	t.Fatal("shouldWip not found")
}

func TestNormalizeTestName(t *testing.T) {
	cases := map[string]string{
		"method [Channel: UI]":     "method",
		"method":                   "method",
		"method [Channel: UI] [x]": "method [Channel: UI]",
		"method(String) [1]":       "method",
		"method()":                 "method",
		"method() [Skipped: UI channel was not specified]": "method",
		"  spaced name  [Channel: A]":                      "spaced name",
	}
	for in, want := range cases {
		if got := NormalizeTestName(in); got != want {
			t.Errorf("NormalizeTestName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseJUnitTagVariants(t *testing.T) {
	doc := `<testsuites><testsuite><testcase name="a" classname="C"><properties>
<property name="tag" value="@UC-002.5"/><property name="criterion" value="UC-002.6"/>
<property name="tag" value="other"/><property name="tag" value="@wip"/><property name="color" value="UC-009.9"/>
</properties></testcase></testsuite></testsuites>`
	tests, err := ParseJUnit(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 1 || !tests[0].WIP || !reflect.DeepEqual(tests[0].IDs, []string{"UC-002.5", "UC-002.6"}) {
		t.Errorf("got %+v", tests)
	}
}

func TestParseJUnitMalformed(t *testing.T) {
	if _, err := ParseJUnit(strings.NewReader("<testsuite><oops")); err == nil {
		t.Fatal("expected error")
	}
}

func TestReadJUnitFilesMergesResults(t *testing.T) {
	tests, err := ReadJUnitFiles([]string{"testdata/junit-all-pass.xml", "testdata/junit-one-red.xml"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 2 {
		t.Fatalf("got %d tests, want 2", len(tests))
	}
	for _, tc := range tests {
		if len(tc.Results) != 2 {
			t.Errorf("%s: %d results, want 2", tc.Method, len(tc.Results))
		}
	}
	if _, err := ReadJUnitFiles([]string{"testdata/missing.xml"}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestTestStatusEmpty(t *testing.T) {
	if (Test{}).Status() != StatusSkipped {
		t.Error("no results should not count as passed")
	}
}

func TestParseJUnitNotApplicablePlaceholder(t *testing.T) {
	xml := `<testsuite>
  <testcase name="admin [Channel: API]" classname="C"><properties><property name="tag" value="UC-001.12"/></properties></testcase>
  <testcase name="admin() [Skipped: UI channel was not specified]" classname="C"><properties><property name="tag" value="UC-001.12"/></properties><skipped/></testcase>
</testsuite>`
	tests, err := ParseJUnit(strings.NewReader(xml))
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 1 || tests[0].Method != "admin" {
		t.Fatalf("want one test named admin, got %+v", tests)
	}
	if got := tests[0].Status(); got != StatusPassed {
		t.Errorf("Status() = %q, want passed (placeholder must not count as skipped)", got)
	}
}
