package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/spec"
)

const specTestUseCase = `---
id: UC-001
title: Place Order
---

# UC-001 Place Order

- UC-001.1: A valid order is placed.
- UC-001.2: Price is computed.
`

const specTestXML = `<testsuite name="s"><testcase name="shouldPlace" classname="c">
<properties><property name="tag" value="UC-001.1"/></properties></testcase></testsuite>`

func writeSpecFixture(t *testing.T, uc string) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "docs", "use-cases"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "build", "test-results"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "docs", "use-cases", "uc1.md"), []byte(uc), 0o644))
	must(os.WriteFile(filepath.Join(dir, "build", "test-results", "a.xml"), []byte(specTestXML), 0o644))
	must(os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte("results: \"**/test-results/*.xml\"\n"), 0o644))
	return filepath.Join(dir, "spec.yaml")
}

func TestSpecCheck_LenientPassesStrictFails(t *testing.T) {
	cfg := writeSpecFixture(t, specTestUseCase)
	var out bytes.Buffer
	code, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeLenient}, nil, &out, true)
	if err != nil || code != spec.ExitPass {
		t.Fatalf("lenient: code=%d err=%v out=%s", code, err, out.String())
	}
	out.Reset()
	code, err = runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeStrict}, nil, &out, true)
	if err != nil || code != spec.ExitGateFailed {
		t.Fatalf("strict: code=%d err=%v", code, err)
	}
	code, err = runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeStrict}, nil, &out, false)
	if err != nil || code != spec.ExitPass {
		t.Fatalf("status must not enforce: code=%d err=%v", code, err)
	}
}

func TestSpecCheck_JSONAndPRComment(t *testing.T) {
	cfg := writeSpecFixture(t, specTestUseCase)
	var out bytes.Buffer
	if _, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeLenient, JSON: true}, nil, &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"schemaVersion": 1`) {
		t.Errorf("missing JSON: %s", out.String())
	}
	out.Reset()
	if _, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeLenient, PRComment: true}, nil, &out, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), spec.PRCommentMarker) {
		t.Errorf("missing marker: %s", out.String())
	}
}

func TestSpecCheck_StepSummaryAndHTML(t *testing.T) {
	cfg := writeSpecFixture(t, specTestUseCase)
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	html := filepath.Join(t.TempDir(), "r.html")
	var out bytes.Buffer
	if _, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeLenient, HTML: html}, nil, &out, true); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{summary, html} {
		if fi, err := os.Stat(f); err != nil || fi.Size() == 0 {
			t.Errorf("%s not written: %v", f, err)
		}
	}
}

func TestSpecCheck_ConfigErrorsAreLoud(t *testing.T) {
	var out bytes.Buffer
	if _, err := runSpecCheck(specOptions{Config: filepath.Join(t.TempDir(), "spec.yaml"), Mode: spec.ModeLenient}, nil, &out, true); err == nil {
		t.Error("missing spec.yaml must error")
	}
	cfg := writeSpecFixture(t, specTestUseCase)
	if _, err := runSpecCheck(specOptions{Config: cfg, Mode: "bogus"}, nil, &out, true); err == nil {
		t.Error("unknown mode must error")
	}
	if err := os.Remove(filepath.Join(filepath.Dir(cfg), "build", "test-results", "a.xml")); err != nil {
		t.Fatal(err)
	}
	if _, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeLenient}, nil, &out, true); err == nil || !strings.Contains(err.Error(), "no JUnit XML files matched") {
		t.Errorf("no results must error, got %v", err)
	}
}

func TestSpecValidate(t *testing.T) {
	var out bytes.Buffer
	code, err := runSpecValidate(specOptions{Config: writeSpecFixture(t, specTestUseCase)}, &out)
	if err != nil || code != 0 {
		t.Fatalf("valid: code=%d err=%v", code, err)
	}
	bad := strings.Replace(specTestUseCase, "UC-001.2", "UC-001.1", 1)
	code, err = runSpecValidate(specOptions{Config: writeSpecFixture(t, bad)}, &out)
	if err != nil || code != spec.ExitGateFailed {
		t.Fatalf("duplicate id: code=%d err=%v", code, err)
	}
}

func TestSpecAdopt_BaselineMakesStrictPass(t *testing.T) {
	cfg := writeSpecFixture(t, specTestUseCase)
	var out bytes.Buffer
	if code, err := runSpecAdopt(specOptions{Config: cfg}, &out); err != nil || code != 0 {
		t.Fatalf("adopt: code=%d err=%v", code, err)
	}
	baseline := filepath.Join(filepath.Dir(cfg), "docs", "use-cases", spec.BaselineFileName)
	if _, err := os.Stat(baseline); err != nil {
		t.Fatalf("baseline not written: %v", err)
	}
	code, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeStrict}, nil, &out, true)
	if err != nil || code != spec.ExitPass {
		t.Fatalf("strict after adopt: code=%d err=%v out=%s", code, err, out.String())
	}
}

func TestSpecGlobMatch(t *testing.T) {
	cases := []struct {
		pat, rel string
		want     bool
	}{
		{"**/build/test-results/**/*.xml", "a/build/test-results/x/y.xml", true},
		{"**/build/test-results/**/*.xml", "build/test-results/y.xml", true},
		{"**/build/test-results/**/*.xml", "build/other/y.xml", false},
		{"*.xml", "a/b.xml", false},
	}
	for _, c := range cases {
		if got := matchesAny([]string{c.pat}, c.rel); got != c.want {
			t.Errorf("%s vs %s = %v", c.pat, c.rel, got)
		}
	}
}

func TestSpecRecord_ChangedCriterionStaysUnverifiedUntilRefreshed(t *testing.T) {
	cfg := writeSpecFixture(t, specTestUseCase)
	ucFile := filepath.Join(filepath.Dir(cfg), "docs", "use-cases", "uc1.md")
	var out bytes.Buffer

	if _, err := runSpecRecord(specOptions{Config: cfg}, &out); err == nil || !strings.Contains(err.Error(), "spec adopt") {
		t.Fatalf("record without a state file must point at adopt, got %v", err)
	}
	if _, err := runSpecAdopt(specOptions{Config: cfg}, &out); err != nil {
		t.Fatal(err)
	}
	state, err := spec.LoadBaseline(filepath.Join(filepath.Dir(cfg), "docs", "use-cases", spec.BaselineFileName))
	if err != nil || state.Fingerprints["UC-001.1"] == "" {
		t.Fatalf("adopt must fingerprint the verified criterion: %+v err=%v", state, err)
	}

	edited := strings.Replace(specTestUseCase, "A valid order is placed.", "A valid order is placed within five minutes.", 1)
	if err := os.WriteFile(ucFile, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeStrict}, nil, &out, true)
	if err != nil || code != spec.ExitGateFailed || !strings.Contains(out.String(), "[Changed]") {
		t.Fatalf("edited criterion must show Changed and block strict: code=%d err=%v out=%s", code, err, out.String())
	}

	out.Reset()
	if _, err := runSpecRecord(specOptions{Config: cfg}, &out); err != nil || !strings.Contains(out.String(), "--refresh UC-001.1") {
		t.Fatalf("record must not silently accept the change: err=%v out=%s", err, out.String())
	}
	if _, err := runSpecRecord(specOptions{Config: cfg, Refresh: []string{"UC-009.9"}}, &out); err == nil {
		t.Fatal("refreshing an unknown criterion must error")
	}
	if _, err := runSpecRecord(specOptions{Config: cfg, Refresh: []string{"UC-001.1"}}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if _, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeLenient}, nil, &out, true); err != nil || strings.Contains(out.String(), "[Changed]") {
		t.Fatalf("refreshed criterion must be Verified again: err=%v out=%s", err, out.String())
	}
}

func TestSpecRecord_CreateAndCriteriaScope(t *testing.T) {
	cfg := writeSpecFixture(t, specTestUseCase)
	stateFile := filepath.Join(filepath.Dir(cfg), "docs", "use-cases", spec.BaselineFileName)
	var out bytes.Buffer

	if _, err := runSpecRecord(specOptions{Config: cfg, Create: true, Criteria: []string{"UC-009.9"}}, &out); err == nil {
		t.Fatal("an unknown --criteria id must error")
	}
	if _, err := os.Stat(stateFile); !os.IsNotExist(err) {
		t.Fatalf("a failed record must not create the state file: %v", err)
	}
	if _, err := runSpecRecord(specOptions{Config: cfg, Create: true, Criteria: []string{""}}, &out); err == nil {
		t.Fatal("an empty --criteria must error")
	}

	// --create starts a missing state file; the scope records only the named
	// Verified criterion and reports the named unverified one.
	if _, err := runSpecRecord(specOptions{Config: cfg, Create: true, Criteria: []string{"UC-001.1", " UC-001.2"}}, &out); err != nil {
		t.Fatal(err)
	}
	state, err := spec.LoadBaseline(stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Unverified) != 0 || len(state.Fingerprints) != 1 || state.Fingerprints["UC-001.1"] == "" {
		t.Fatalf("created state must hold only the scoped fingerprint and no baselined gap: %+v", state)
	}
	if !strings.Contains(out.String(), "UC-001.2 is not Verified") {
		t.Fatalf("a scoped unverified criterion must be reported: %s", out.String())
	}

	// The created file gates like no file: strict still blocks on UC-001.2.
	out.Reset()
	if code, err := runSpecCheck(specOptions{Config: cfg, Mode: spec.ModeStrict}, nil, &out, true); err != nil || code != spec.ExitGateFailed {
		t.Fatalf("created state must not baseline unverified criteria: code=%d err=%v", code, err)
	}
}
