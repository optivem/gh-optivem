package spec

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func loadSpec(t *testing.T) []UseCase {
	t.Helper()
	data, err := os.ReadFile("testdata/UC-001-place-order.md")
	if err != nil {
		t.Fatal(err)
	}
	return []UseCase{Parse("UC-001-place-order.md", string(data))}
}

func fixtureReport(t *testing.T, cfg Config, fixtures ...string) Report {
	t.Helper()
	var tests []Test
	for _, f := range fixtures {
		tests = append(tests, readFixture(t, f)...)
	}
	return BuildReport(loadSpec(t), tests, cfg, Options{Now: testNow})
}

func states(r Report) map[string]State {
	m := map[string]State{}
	for _, uc := range r.UseCases {
		for _, c := range uc.Criteria {
			m[c.ID] = c.State
		}
	}
	return m
}

func TestAllPassFixture(t *testing.T) {
	r := fixtureReport(t, Config{}, "junit-all-pass.xml")
	s := states(r)
	if s["UC-001.1"] != StateVerified || s["UC-001.2"] != StateVerified || s["UC-001.14"] != StatePlanned {
		t.Fatalf("states: %v", s)
	}
	if r.Summary.Total != 14 || r.Summary.Verified != 2 || r.Summary.Planned != 12 {
		t.Fatalf("summary: %+v", r.Summary)
	}
	if r.Summary.CoveragePercent != 14.3 {
		t.Fatalf("coverage: %v", r.Summary.CoveragePercent)
	}
	if r.UseCases[0].State != StateInProgress {
		t.Fatalf("use case state: %s", r.UseCases[0].State)
	}
	if len(r.Orphans) != 0 || len(r.UnknownIDs) != 0 {
		t.Fatalf("unexpected orphans/unknown: %+v %+v", r.Orphans, r.UnknownIDs)
	}
}

func TestFailingFixture(t *testing.T) {
	r := fixtureReport(t, Config{}, "junit-one-red.xml")
	s := states(r)
	if s["UC-001.1"] != StateVerified || s["UC-001.2"] != StateFailing {
		t.Fatalf("states: %v", s)
	}
	if r.Summary.Failing != 1 {
		t.Fatalf("summary: %+v", r.Summary)
	}
}

func TestTemplatedFixtureFailingAndInProgress(t *testing.T) {
	r := fixtureReport(t, Config{}, "junit-templated.xml")
	s := states(r)
	// UI passes, API errors: one test with two results, so Failing.
	if s["UC-001.1"] != StateFailing {
		t.Fatalf("UC-001.1: %s", s["UC-001.1"])
	}
	if s["UC-001.3"] != StateInProgress {
		t.Fatalf("UC-001.3: %s", s["UC-001.3"])
	}
	c := r.UseCases[0].Criteria[0]
	if len(c.Tests) != 1 || len(c.Tests[0].Results) != 2 {
		t.Fatalf("tests: %+v", c.Tests)
	}
}

func TestSkippedIsNotVerified(t *testing.T) {
	r := fixtureReport(t, Config{}, "junit-skipped.xml")
	if got := states(r)["UC-001.1"]; got != StatePlanned {
		t.Fatalf("got %s", got)
	}
	if len(r.UseCases[0].Criteria[0].Tests) != 1 {
		t.Fatal("skipped test should still be linked")
	}
}

func TestOrphanAndUnknown(t *testing.T) {
	r := fixtureReport(t, Config{}, "junit-orphan.xml", "junit-unknown-id.xml")
	if len(r.Orphans) != 2 {
		t.Fatalf("orphans: %+v", r.Orphans)
	}
	reasons := r.Orphans[0].Reason + "|" + r.Orphans[1].Reason
	if !strings.Contains(reasons, "no criterion ID") || !strings.Contains(reasons, "no known criterion ID") {
		t.Fatalf("reasons: %s", reasons)
	}
	if len(r.UnknownIDs) != 1 || r.UnknownIDs[0].ID != "UC-999.1" {
		t.Fatalf("unknown: %+v", r.UnknownIDs)
	}
}

func TestSharedTestsAndUseCaseVerified(t *testing.T) {
	spec := loadSpec(t)
	pass := []Result{{"x", StatusPassed}}
	var tests []Test
	for i, c := range spec[0].Criteria {
		if c.ID == "UC-001.14" {
			continue
		}
		tests = append(tests, Test{Class: "A", Method: "m" + string(rune('a'+i)), IDs: []string{c.ID}, Results: pass})
	}
	// Criteria 3 and 4 share a test.
	tests = append(tests, Test{Class: "A", Method: "t3and4", IDs: []string{"UC-001.3", "UC-001.4"}, Results: pass})
	r := BuildReport(spec, tests, Config{}, Options{Now: testNow})
	if r.Summary.Verified != 13 || r.Summary.Planned != 1 || r.Summary.CoveragePercent != 92.9 {
		t.Fatalf("summary: %+v", r.Summary)
	}
	if r.UseCases[0].State != StateInProgress {
		t.Fatalf("state: %s", r.UseCases[0].State)
	}
	for _, c := range r.UseCases[0].Criteria {
		if (c.ID == "UC-001.3" || c.ID == "UC-001.4") && len(c.Tests) != 2 {
			t.Fatalf("%s tests: %d", c.ID, len(c.Tests))
		}
	}
	// Waiving the untested one does not verify it.
	cfg := Config{Waivers: []Waiver{{"UC-001.14", "r", "a", "2026-12-31"}}}
	r = BuildReport(spec, tests, cfg, Options{Now: testNow})
	if r.Summary.Waived != 1 || r.Summary.Verified != 13 || r.UseCases[0].State != StateInProgress {
		t.Fatalf("waived summary: %+v state %s", r.Summary, r.UseCases[0].State)
	}
	// Verify the last one: use case Verified.
	tests = append(tests, Test{Class: "A", Method: "m14", IDs: []string{"UC-001.14"}, Results: pass})
	r = BuildReport(spec, tests, Config{}, Options{Now: testNow})
	if r.UseCases[0].State != StateVerified || r.Summary.CoveragePercent != 100 {
		t.Fatalf("all verified: %+v %s", r.Summary, r.UseCases[0].State)
	}
}

func TestUseCasePlannedWithoutTests(t *testing.T) {
	r := BuildReport(loadSpec(t), nil, Config{}, Options{Now: testNow})
	if r.UseCases[0].State != StatePlanned || r.Summary.Planned != 14 || r.Summary.CoveragePercent != 0 {
		t.Fatalf("%+v %s", r.Summary, r.UseCases[0].State)
	}
}

func TestWaiverExpiry(t *testing.T) {
	cfg := Config{Waivers: []Waiver{{"UC-001.2", "sandbox", "jane", "2026-10-28"}}}
	// Failing underneath; waiver valid through its date inclusive.
	r := fixtureReport(t, cfg, "junit-one-red.xml")
	c := r.UseCases[0].Criteria[1]
	if c.State != StateWaived || c.Waiver == nil || c.Waiver.Approver != "jane" {
		t.Fatalf("criterion: %+v", c)
	}
	if r.Summary.Failing != 0 || r.Summary.Waived != 1 {
		t.Fatalf("summary: %+v", r.Summary)
	}
	// Expired the next day: reverts to Failing.
	r = BuildReport(loadSpec(t), readFixture(t, "junit-one-red.xml"), cfg, Options{Now: testNow.AddDate(0, 0, 1)})
	c = r.UseCases[0].Criteria[1]
	if c.State != StateFailing || c.Waiver != nil {
		t.Fatalf("expired: %+v", c)
	}
}

func TestWipBeatsFailing(t *testing.T) {
	tests := []Test{{Method: "m", IDs: []string{"UC-001.1"}, WIP: true, Results: []Result{{"m", StatusFailed}}}}
	r := BuildReport(loadSpec(t), tests, Config{}, Options{Now: testNow})
	if got := states(r)["UC-001.1"]; got != StateInProgress {
		t.Fatalf("got %s", got)
	}
}

func TestFlag(t *testing.T) {
	uc := Parse("f.md", "---\nid: UC-002\ntitle: T\n---\n- UC-002.1 [flag: dark]: x\n- UC-002.2: y\n")
	r := BuildReport([]UseCase{uc}, nil, Config{}, Options{Now: testNow})
	cs := r.UseCases[0].Criteria
	if cs[0].Flag == nil || *cs[0].Flag != "dark" || cs[0].FlagOn || cs[1].Flag != nil || !cs[1].FlagOn {
		t.Fatalf("flags: %+v", cs)
	}
	r = BuildReport([]UseCase{uc}, nil, Config{}, Options{Now: testNow, Flags: []string{"dark"}})
	if !r.UseCases[0].Criteria[0].FlagOn {
		t.Fatal("flag from options should be on")
	}
	r = BuildReport([]UseCase{uc}, nil, Config{Flags: StringList{"dark"}}, Options{Now: testNow})
	if !r.UseCases[0].Criteria[0].FlagOn {
		t.Fatal("flag from config should be on")
	}
}

func TestValidationErrorsInReport(t *testing.T) {
	uc := Parse("bad.md", "no front matter\n")
	r := BuildReport([]UseCase{uc}, nil, Config{}, Options{Now: testNow})
	if len(r.Validation.Errors) != 1 || r.Validation.Errors[0].Code != "front-matter" {
		t.Fatalf("%+v", r.Validation)
	}
}

func TestJSONContract(t *testing.T) {
	r := fixtureReport(t, Config{}, "junit-all-pass.xml")
	data, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m["schemaVersion"].(float64) != 1 || m["mode"] != "lenient" || m["threshold"].(float64) != 100 {
		t.Fatalf("header: %v", m)
	}
	for _, k := range []string{"summary", "useCases", "orphans", "unknownIds", "validation"} {
		if m[k] == nil {
			t.Fatalf("missing %s in %s", k, data)
		}
	}
	if _, ok := m["gate"]; ok {
		t.Fatal("gate must be absent until gating fills it")
	}
	crit := m["useCases"].([]any)[0].(map[string]any)["criteria"].([]any)[0].(map[string]any)
	if crit["state"] != "Verified" || crit["flag"] != nil || crit["waiver"] != nil || crit["baselined"] != false {
		t.Fatalf("criterion: %v", crit)
	}
	res := crit["tests"].([]any)[0].(map[string]any)["results"].([]any)[0].(map[string]any)
	if res["status"] != "passed" {
		t.Fatalf("result: %v", res)
	}
	if !strings.Contains(string(data), `"orphans": []`) {
		t.Fatal("empty lists must render as []")
	}
}

func TestThresholdOverride(t *testing.T) {
	th := 80.0
	r := BuildReport(nil, nil, Config{}, Options{Now: testNow, Threshold: &th, Mode: "strict"})
	if r.Threshold != 80 || r.Mode != "strict" {
		t.Fatalf("%+v", r)
	}
}
