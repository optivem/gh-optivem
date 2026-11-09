package spec

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestParse_GherkinShopUC001 pins shop's real UC-001 file: 22 tagged
// scenarios over 14 distinct criterion IDs, so scenarios sharing a tag merge.
func TestParse_GherkinShopUC001(t *testing.T) {
	data, err := os.ReadFile("testdata/gherkin/UC-001-acceptance-criteria.md")
	if err != nil {
		t.Fatal(err)
	}
	uc := Parse("UC-001-place-order/acceptance-criteria.md", string(data))
	if uc.FrontErr != "" || len(uc.ParseErrs) > 0 || len(uc.Warnings) > 0 {
		t.Fatalf("unexpected problems: front=%q parse=%v warn=%v", uc.FrontErr, uc.ParseErrs, uc.Warnings)
	}
	if uc.Status != StatusImplemented {
		t.Errorf("status = %q, want %q", uc.Status, StatusImplemented)
	}
	if len(uc.Criteria) != 14 {
		t.Fatalf("got %d criteria, want 14: %+v", len(uc.Criteria), uc.Criteria)
	}
	for i, c := range uc.Criteria {
		if want := "UC-001." + strconv.Itoa(i+1); c.ID != want {
			t.Errorf("criterion %d id = %s, want %s", i, c.ID, want)
		}
		if c.Text == "" {
			t.Errorf("%s has empty text", c.ID)
		}
	}
	if errs := Validate([]UseCase{uc}); len(errs) > 0 {
		t.Errorf("Validate: %v", errs)
	}
}

const gherkinHeader = "---\nid: UC-009\ntitle: Demo\n---\n\n"

func TestParse_GherkinEdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantIDs   []string
		wantText  string
		wantParse string
		wantWarn  string
	}{
		{
			name:     "shared tag merges titles",
			body:     "```gherkin\n@UC-009.1\nScenario: A\n  When x\n@UC-009.1 @narrative-2a\nScenario Outline: B\n  When y\n```\n",
			wantIDs:  []string{"UC-009.1"},
			wantText: "A; B",
		},
		{
			name:     "untagged scenario warns",
			body:     "```gherkin\nScenario: Loose\n  When x\n```\n",
			wantWarn: `scenario "Loose" has no @UC-N.M tag`,
		},
		{
			name:      "two criterion tags is an error",
			body:      "```gherkin\n@UC-009.1 @UC-009.2\nScenario: Both\n```\n",
			wantParse: "tagged with 2 criterion IDs",
		},
		{
			name:    "rule becomes the group, tags reset by steps",
			body:    "```gherkin\nFeature: F\n  Rule: R1\n    @UC-009.1\n    Scenario: A\n      Then ok\n    Scenario: Untagged after steps\n```\n",
			wantIDs: []string{"UC-009.1"}, wantWarn: "Untagged after steps",
		},
		{
			name:    "scenarios outside a gherkin fence are ignored",
			body:    "```\n@UC-009.1\nScenario: Not gherkin fence\n```\n- UC-009.2: bullet\n",
			wantIDs: []string{"UC-009.2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uc := Parse("x.md", gherkinHeader+tc.body)
			var ids []string
			for _, c := range uc.Criteria {
				ids = append(ids, c.ID)
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantIDs, ",") {
				t.Errorf("ids = %v, want %v", ids, tc.wantIDs)
			}
			if tc.wantText != "" && (len(uc.Criteria) == 0 || uc.Criteria[0].Text != tc.wantText) {
				t.Errorf("text = %+v, want %q", uc.Criteria, tc.wantText)
			}
			if got := strings.Join(uc.ParseErrs, "|"); (tc.wantParse == "") != (got == "") || !strings.Contains(got, tc.wantParse) {
				t.Errorf("parse errs = %q, want containing %q", got, tc.wantParse)
			}
			if got := strings.Join(uc.Warnings, "|"); (tc.wantWarn == "") != (got == "") || !strings.Contains(got, tc.wantWarn) {
				t.Errorf("warnings = %q, want containing %q", got, tc.wantWarn)
			}
		})
	}
}

func TestParse_GherkinGroupFromRule(t *testing.T) {
	uc := Parse("x.md", gherkinHeader+"```gherkin\nRule: Pricing\n  @UC-009.1\n  Scenario: A\n```\n")
	if len(uc.Criteria) != 1 || uc.Criteria[0].Group != "Pricing" {
		t.Fatalf("got %+v, want group Pricing", uc.Criteria)
	}
}

func TestValidate_BulletAndScenarioSameIDIsDuplicate(t *testing.T) {
	uc := Parse("x.md", gherkinHeader+"- UC-009.1: bullet\n\n```gherkin\n@UC-009.1\nScenario: Same id\n```\n")
	errs := Validate([]UseCase{uc})
	if len(errs) != 1 || errs[0].Code != "duplicate-id" {
		t.Fatalf("got %v, want one duplicate-id", errs)
	}
}

func TestValidate_TwoTagsReportedAsCriteriaError(t *testing.T) {
	uc := Parse("x.md", gherkinHeader+"```gherkin\n@UC-009.1\nScenario: ok\n@UC-009.2 @UC-009.3\nScenario: Both\n```\n")
	errs := Validate([]UseCase{uc})
	if len(errs) != 1 || errs[0].Code != "criteria" {
		t.Fatalf("got %v, want one criteria error", errs)
	}
}

func TestValidate_NoCriteria(t *testing.T) {
	implemented := Parse("a.md", gherkinHeader+"Just prose.\n")
	planned := Parse("b.md", "---\nid: UC-010\ntitle: Later\nstatus: planned\n---\nNothing yet.\n")
	errs := Validate([]UseCase{implemented, planned})
	if len(errs) != 1 || errs[0].Code != "no-criteria" || !strings.Contains(errs[0].Message, "a.md") {
		t.Fatalf("got %v, want one no-criteria error for a.md only", errs)
	}
}

func TestParse_StatusValues(t *testing.T) {
	uc := Parse("x.md", "---\nid: UC-009\ntitle: T\nstatus: someday\n---\n- UC-009.1: a\n")
	if !strings.Contains(uc.FrontErr, `status "someday"`) {
		t.Fatalf("FrontErr = %q, want status error", uc.FrontErr)
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadDir_FolderPerUseCase(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"README.md": "# Use cases\n\nNo front matter here.\n",
		"UC-001-place-order/use-case-narrative.md":  "# Narrative\n",
		"UC-001-place-order/business-rules.md":      "# Rules\n",
		"UC-001-place-order/acceptance-criteria.md": "---\nid: UC-001\ntitle: Place Order\n---\n```gherkin\n@UC-001.1\nScenario: A\n```\n",
		"UC-002-view-order/use-case-narrative.md":   "# Narrative\n",
		"UC-002-view-order/acceptance-criteria.md":  "---\nid: UC-002\ntitle: View Order\nstatus: planned\n---\n```gherkin\n@UC-002.1\nScenario: B\n@UC-002.2\nScenario: C\n```\n",
		"UC-003-misnamed/acceptance-criteria.md":    "---\nid: UC-004\ntitle: Odd\n---\n- UC-004.1: x\n",
		"notes/scratch.md":                          "ignored: no acceptance-criteria.md here\n",
		"UC-005-flat.md":                            "---\nid: UC-005\ntitle: Flat\n---\n- UC-005.1: flat bullet\n",
	})
	ucs, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, uc := range ucs {
		files = append(files, uc.File)
	}
	want := "UC-001-place-order/acceptance-criteria.md,UC-002-view-order/acceptance-criteria.md,UC-003-misnamed/acceptance-criteria.md,UC-005-flat.md"
	if strings.Join(files, ",") != want {
		t.Fatalf("files = %v\nwant %s", files, want)
	}
	if errs := Validate(ucs); len(errs) > 0 {
		t.Errorf("Validate: %v", errs)
	}
	warns := Warnings(ucs)
	if len(warns) != 1 || !strings.Contains(warns[0], "named for UC-003 but its front matter id is UC-004") {
		t.Errorf("warnings = %v, want the folder/id mismatch only", warns)
	}

	rep := BuildReport(ucs, nil, Config{}, Options{})
	if rep.Summary.Total != 3 {
		t.Errorf("gated criteria = %d, want 3 (UC-001.1, UC-004.1, UC-005.1)", rep.Summary.Total)
	}
	if len(rep.PlannedUseCases) != 1 || rep.PlannedUseCases[0].ID != "UC-002" || rep.PlannedUseCases[0].Criteria != 2 {
		t.Errorf("planned = %+v, want UC-002 with 2 criteria", rep.PlannedUseCases)
	}
	for _, uc := range rep.UseCases {
		if uc.ID == "UC-002" {
			t.Errorf("planned UC-002 must not be gated")
		}
	}
	if !strings.Contains(RenderText(rep), "UC-002 View Order: planned, 2 criteria, not gated") {
		t.Errorf("text report doesn't list the planned use case:\n%s", RenderText(rep))
	}
}

// TestBuildReport_PlannedCriterionTagIsNotUnknown: a test tagged with a
// planned use case's criterion links to a known ID, not an unknown one.
func TestBuildReport_PlannedCriterionTagIsNotUnknown(t *testing.T) {
	planned := Parse("p.md", "---\nid: UC-010\ntitle: Later\nstatus: planned\n---\n- UC-010.1: a\n")
	tests := []Test{{Class: "C", Method: "m", IDs: []string{"UC-010.1"}, Results: []Result{{Name: "m", Status: StatusPassed}}}}
	rep := BuildReport([]UseCase{planned}, tests, Config{}, Options{})
	if len(rep.UnknownIDs) != 0 {
		t.Errorf("unknown IDs = %v, want none", rep.UnknownIDs)
	}
}
