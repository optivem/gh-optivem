package runner

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestExecutedTestNames covers the per-format name extractors, the de-dup of
// channel/parameter invocations back to a single method name, and the
// missing-file rule. Fixtures mirror the real report shapes verified against
// the shop system-test reports (JUnit `method [Channel: UI]`, TRX
// `Method(channel: UI)`, Playwright spec titles).
func TestExecutedTestNames(t *testing.T) {
	// Two channels of one method + a second method: de-dups to {one, two}.
	junitBare := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="com.example.CancelOrderNegativeIsolatedTest" tests="3" skipped="0">
  <testcase name="cannotCancelAnOrderAt2245OnDec31 [Channel: API]" classname="com.example.CancelOrderNegativeIsolatedTest"/>
  <testcase name="cannotCancelAnOrderAt2245OnDec31 [Channel: UI]" classname="com.example.CancelOrderNegativeIsolatedTest"/>
  <testcase name="placeOrder [Channel: API]" classname="com.example.CancelOrderNegativeIsolatedTest"/>
</testsuite>`
	junitWrapped := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="A"><testcase name="alpha [Channel: API]"/></testsuite>
  <testsuite name="B"><testcase name="beta [Channel: UI]"/><testcase name="beta [Channel: API]"/></testsuite>
</testsuites>`
	trx := `<?xml version="1.0" encoding="UTF-8"?>
<TestRun id="x" xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <Results>
    <UnitTestResult testName="ShouldPlaceOrder(channel: UI)" />
    <UnitTestResult testName="ShouldPlaceOrder(channel: API)" />
    <UnitTestResult testName="My.Ns.CannotCancel(channel: API)" />
  </Results>
</TestRun>`
	pwJSON := `{"suites":[{"specs":[{"title":"shouldPlaceOrder"}],"suites":[{"specs":[{"title":"cannotCancel"}]}]}]}`

	cases := []struct {
		name    string
		file    string // ext drives dispatch; "" => non-existent path
		content string
		want    []string
		wantErr bool
	}{
		{name: "junit dedups channels to methods", file: "r.xml", content: junitBare, want: []string{"cannotCancelAnOrderAt2245OnDec31", "placeOrder"}},
		{name: "junit wrapped suites unioned", file: "r.xml", content: junitWrapped, want: []string{"alpha", "beta"}},
		{name: "trx strips args and fqn", file: "r.trx", content: trx, want: []string{"CannotCancel", "ShouldPlaceOrder"}},
		{name: "playwright nested spec titles", file: "r.json", content: pwJSON, want: []string{"cannotCancel", "shouldPlaceOrder"}},
		{name: "missing file is empty set", file: "", content: "", want: nil},
		{name: "malformed xml errors", file: "bad.xml", content: "<testsuite><not-closed>", wantErr: true},
		{name: "unsupported extension errors", file: "r.txt", content: "whatever", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "does-not-exist.xml")
			if tc.file != "" {
				path = filepath.Join(dir, tc.file)
				if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := executedTestNames(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", keys(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g := keys(got); !equalSets(g, tc.want) {
				t.Fatalf("names: got %v, want %v", g, tc.want)
			}
		})
	}
}

// TestExecutedTestNames_ExcludesSkipped pins that skipped testcases are not
// counted as executed in any report format, while a test that executed (even a
// failing one) is. A method skipped on one channel but executed on another
// still lands in the set via the executed invocation.
func TestExecutedTestNames_ExcludesSkipped(t *testing.T) {
	junit := `<testsuite name="A">
  <testcase name="skippedOnUi [Channel: UI]"><skipped/></testcase>
  <testcase name="failingOnUi [Channel: UI]"><failure message="boom"/></testcase>
  <testcase name="mixed [Channel: UI]"><skipped/></testcase>
  <testcase name="mixed [Channel: API]"/>
</testsuite>`
	junitWrapped := `<testsuites><testsuite name="A"><testcase name="skippedOnUi [Channel: UI]"><skipped message="x"/></testcase><testcase name="ran"/></testsuite></testsuites>`
	trx := `<TestRun id="x" xmlns="http://microsoft.com/schemas/VisualStudio/TeamTest/2010">
  <Results>
    <UnitTestResult testName="SkippedOnUi(channel: UI)" outcome="NotExecuted" />
    <UnitTestResult testName="FailingOnUi(channel: UI)" outcome="Failed" />
    <UnitTestResult testName="Passing(channel: API)" outcome="Passed" />
  </Results>
</TestRun>`
	pw := `{"suites":[{"specs":[
  {"title":"skippedOnUi","tests":[{"status":"skipped"}]},
  {"title":"failingOnUi","tests":[{"status":"unexpected"}]},
  {"title":"mixed","tests":[{"status":"skipped"},{"status":"expected"}]}
]}]}`

	cases := []struct {
		file, content string
		want          []string
	}{
		{"r.xml", junit, []string{"failingOnUi", "mixed"}},
		{"w.xml", junitWrapped, []string{"ran"}},
		{"r.trx", trx, []string{"FailingOnUi", "Passing"}},
		{"r.json", pw, []string{"failingOnUi", "mixed"}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := executedTestNames(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g := keys(got); !equalSets(g, tc.want) {
				t.Fatalf("names: got %v, want %v", g, tc.want)
			}
		})
	}
}

// TestExecutedTestNames_JUnitDir exercises the directory path: gradle drops one
// TEST-*.xml per class, so a directory must union names across files.
func TestExecutedTestNames_JUnitDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"TEST-A.xml": `<testsuite name="A"><testcase name="cannotCancelAnOrderAt2245OnDec31 [Channel: API]"/></testsuite>`,
		"TEST-B.xml": `<testsuite name="B"><testcase name="placeOrder [Channel: UI]"/><testcase name="placeOrder [Channel: API]"/></testsuite>`,
		"ignore.txt": "not xml",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := executedTestNames(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g := keys(got); !equalSets(g, []string{"cannotCancelAnOrderAt2245OnDec31", "placeOrder"}) {
		t.Fatalf("names: got %v", g)
	}
}

func keys(m map[string]bool) []string {
	if m == nil {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	bs := append([]string(nil), b...)
	sort.Strings(bs)
	for i := range a {
		if a[i] != bs[i] {
			return false
		}
	}
	return true
}

// TestNameExecuted pins the requested-name match: exact method, substring
// tolerance for manual partial filters, and a genuine miss.
func TestNameExecuted(t *testing.T) {
	executed := map[string]bool{
		"cannotCancelAnOrderAt2245OnDec31": true,
		"placeOrder":                       true,
	}
	cases := []struct {
		req  string
		want bool
	}{
		{"cannotCancelAnOrderAt2245OnDec31", true}, // exact
		{"Cancel", true}, // substring (manual partial)
		{"placeOrder", true},
		{"shipOrder", false}, // genuine miss
	}
	for _, tc := range cases {
		if got := nameExecuted(tc.req, executed); got != tc.want {
			t.Errorf("nameExecuted(%q) = %v, want %v", tc.req, got, tc.want)
		}
	}
}
