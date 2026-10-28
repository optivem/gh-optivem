package spec

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Fatalf("%s differs from golden (run with -update to refresh)", name)
	}
}

// mixedReport has a failing, a wip, a waived and a baselined criterion plus an
// orphan, an unknown ID and a validation error, gated strictly.
func mixedReport(t *testing.T) Report {
	t.Helper()
	cfg := Config{Waivers: []Waiver{{ID: "UC-001.5", Reason: "Payment <sandbox> down | later", Approver: "vj", Until: "2026-12-31"}}}
	rep := fixtureReport(t, cfg, "junit-templated.xml", "junit-one-red.xml", "junit-orphan.xml", "junit-unknown-id.xml")
	rep.Mode = ModeStrict
	rep.Threshold = 80
	rep.Validation.Errors = append(rep.Validation.Errors, ValidationError{"duplicate-id", "UC-001.9 is defined twice"})
	b := Baseline{SchemaVersion: BaselineSchemaVersion, Unverified: []string{"UC-001.14"}}
	if err := ApplyGate(&rep, &b); err != nil {
		t.Fatal(err)
	}
	return rep
}

func cleanReport(t *testing.T) Report {
	t.Helper()
	rep := fixtureReport(t, Config{}, "junit-all-pass.xml")
	if err := ApplyGate(&rep, nil); err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestRenderGolden(t *testing.T) {
	for name, rep := range map[string]Report{"mixed": mixedReport(t), "clean": cleanReport(t)} {
		golden(t, name+".txt", RenderText(rep))
		golden(t, name+".summary.md", RenderSummaryMarkdown(rep))
		golden(t, name+".comment.md", RenderPRComment(rep))
		golden(t, name+".html", RenderHTML(rep, testNow))
	}
}

func TestRenderDeterministic(t *testing.T) {
	rep := mixedReport(t)
	if RenderHTML(rep, testNow) != RenderHTML(rep, testNow) || RenderText(rep) != RenderText(rep) {
		t.Fatal("output not deterministic")
	}
}

func TestRenderContracts(t *testing.T) {
	rep := mixedReport(t)
	if !strings.HasPrefix(RenderPRComment(rep), PRCommentMarker+"\n") || PRCommentMarker != "<!-- gh-optivem-spec -->" {
		t.Fatal("PR comment must start with the marker")
	}
	for name, out := range map[string]string{
		"text": RenderText(rep), "summary": RenderSummaryMarkdown(rep),
		"comment": RenderPRComment(rep), "html": RenderHTML(rep, testNow),
	} {
		if !strings.Contains(out, "acceptance criteria verified") {
			t.Errorf("%s lacks the coverage definition", name)
		}
	}
}

func TestRenderEscapesSpecText(t *testing.T) {
	rep := mixedReport(t)
	rep.UseCases[0].Title = "<script>alert(1)</script>"
	rep.UseCases[0].Criteria[0].Text = `a <b>"x"</b> | y`
	h := RenderHTML(rep, testNow)
	if strings.Contains(h, "<script>alert") || strings.Contains(h, "<b>") {
		t.Fatal("html not escaped")
	}
	if strings.Contains(h, "http://") || strings.Contains(h, "https://") {
		t.Fatal("html must have no external assets")
	}
	if strings.Contains(RenderSummaryMarkdown(rep), "<b>") || strings.Contains(RenderPRComment(rep), "<script>") {
		t.Fatal("markdown not escaped")
	}
}
