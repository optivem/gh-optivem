package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeReport(t *testing.T, path, body string, mtime time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestFailuresInReports_ParsesEachFormat(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeReport(t, filepath.Join(dir, "java", "build", "test-results", "TEST-A.xml"), `<testsuite tests="2">
  <testcase name="shouldPass" classname="A"/>
  <testcase name="shouldPlaceOrder [Channel: API]" classname="A"><failure message="expected 1 but was 0" type="org.opentest4j.AssertionFailedError">stack</failure></testcase>
</testsuite>`, now)
	writeReport(t, filepath.Join(dir, "dotnet", "results.trx"), `<TestRun><Results>
  <UnitTestResult testName="ShouldPlaceOrder(channel: UI)" outcome="Failed"><Output><ErrorInfo><Message>Assert.Equal() Failure</Message><StackTrace>at X</StackTrace></ErrorInfo></Output></UnitTestResult>
  <UnitTestResult testName="Other" outcome="Passed"/>
</Results></TestRun>`, now)
	writeReport(t, filepath.Join(dir, "ts", "results.json"), `{"suites":[{"suites":[{"specs":[{"title":"shouldPlaceOrder","tests":[{"results":[{"status":"failed","error":{"message":"TargetClosedError: Target page, context or browser has been closed"}}]}]}]}]}],"errors":[]}`, now)
	cfg := &TestsConfig{Suites: []Suite{
		{ID: "java", Name: "java", Path: "java", TestCountPath: "build/test-results"},
		{ID: "dotnet", Name: "dotnet", Path: "dotnet", TestCountPath: "results.trx"},
		{ID: "ts", Name: "ts", Path: "ts", TestCountPath: "results.json"},
	}}
	ev, err := FailuresInReports(cfg, dir, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.Complete || ev.FreshReports != 3 || len(ev.Failures) != 3 {
		t.Fatalf("got %+v, want complete, 3 fresh reports, 3 failures", ev)
	}
	all := ""
	for _, f := range ev.Failures {
		all += f.Detail + "\n"
	}
	for _, want := range []string{"AssertionFailedError", "Assert.Equal() Failure", "TargetClosedError"} {
		if !strings.Contains(all, want) {
			t.Errorf("failure details missing %q:\n%s", want, all)
		}
	}
}

func TestFailuresInReports_IgnoresStaleReports(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeReport(t, filepath.Join(dir, "results.trx"), `<TestRun><Results><UnitTestResult testName="T" outcome="Failed"/></Results></TestRun>`, now.Add(-time.Hour))
	cfg := &TestsConfig{Suites: []Suite{{ID: "s", Name: "s", TestCountPath: "results.trx"}}}
	ev, err := FailuresInReports(cfg, dir, []string{"s"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if ev.FreshReports != 0 || len(ev.Failures) != 0 || !ev.Complete {
		t.Fatalf("stale report must be ignored: %+v", ev)
	}
}

func TestFailuresInReports_IncompleteWhenSuiteLacksCountPath(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "s", Name: "s"}}}
	ev, err := FailuresInReports(cfg, t.TempDir(), []string{"s"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if ev.Complete {
		t.Fatalf("suite without testCountPath must make evidence incomplete: %+v", ev)
	}
	if ev, _ := FailuresInReports(nil, "", nil, time.Now()); ev.Complete {
		t.Fatalf("nil config must be incomplete")
	}
}

func TestFailuresInReports_PlaywrightGlobalErrors(t *testing.T) {
	dir := t.TempDir()
	writeReport(t, filepath.Join(dir, "r.json"), `{"suites":[],"errors":[{"message":"browserType.launch: Host system is missing dependencies"}]}`, time.Now())
	cfg := &TestsConfig{Suites: []Suite{{ID: "s", Name: "s", TestCountPath: "r.json"}}}
	ev, err := FailuresInReports(cfg, dir, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.GlobalErrors) != 1 || len(ev.Failures) != 0 {
		t.Fatalf("got %+v", ev)
	}
}
