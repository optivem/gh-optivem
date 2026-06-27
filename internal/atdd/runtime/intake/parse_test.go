package intake

import (
	"strings"
	"testing"
)

func TestParse_AcceptanceCriteriaExtracted(t *testing.T) {
	body := "## Description\n\nStuff.\n\n## Acceptance Criteria\n\nScenario: Foo\n  Given x\n  When y\n  Then z\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.AcceptanceCriteria.Found {
		t.Fatalf("AcceptanceCriteria.Found: got false, want true")
	}
	if r.Checklist.Found {
		t.Fatalf("Checklist.Found: got true, want false (body has no Checklist)")
	}
}

// A `Rule:`-grouped AC body must be extracted verbatim — the parser stays dumb,
// interpreting only section presence, never the Feature/Rule/Scenario structure.
func TestParse_AcceptanceCriteria_RuleBody_ExtractedVerbatim(t *testing.T) {
	acBody := "Feature: Checkout\n  Rule: Shipping is $0.10/kg per unit\n\n    Scenario: single item\n      Given a product weighing 2kg\n      When I check out 1\n      Then shipping is $0.20"
	body := "## Acceptance Criteria\n\n" + acBody + "\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.AcceptanceCriteria.Found {
		t.Fatalf("AcceptanceCriteria.Found: got false, want true")
	}
	if r.AcceptanceCriteria.Body != acBody {
		t.Fatalf("Body not verbatim:\n got %q\nwant %q", r.AcceptanceCriteria.Body, acBody)
	}
}

func TestParse_BugSectionsExtracted(t *testing.T) {
	body := "## Steps to Reproduce\n\n1. one\n\n## Acceptance Criteria\n\nScenario: ok\n  Given x\n  When y\n  Then z\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.StepsToReproduce.Found || !r.AcceptanceCriteria.Found {
		t.Fatalf("expected both sections found, got steps=%v ac=%v", r.StepsToReproduce.Found, r.AcceptanceCriteria.Found)
	}
}

func TestParse_ChecklistExtracted(t *testing.T) {
	body := "## Checklist\n\n- [ ] One\n- [ ] Two\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Checklist.Found {
		t.Fatalf("Checklist not found")
	}
	if got, want := len(r.Checklist.Items), 2; got != want {
		t.Fatalf("Items length: got %d, want %d", got, want)
	}
	if got := r.Checklist.CheckedCount(); got != 0 {
		t.Fatalf("CheckedCount: got %d, want 0", got)
	}
}

func TestParse_EmptyBodyHasNoFoundSections(t *testing.T) {
	body := "## Description\n\nNo AC or checklist here.\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.AcceptanceCriteria.Found || r.Checklist.Found || r.StepsToReproduce.Found {
		t.Fatalf("expected only Description found, got ac=%v cl=%v steps=%v", r.AcceptanceCriteria.Found, r.Checklist.Found, r.StepsToReproduce.Found)
	}
	if !r.Description.Found {
		t.Fatalf("Description.Found: got false, want true")
	}
}

// XOR rule: a body declaring both AC and Checklist is malformed regardless
// of ticket-kind. Per-kind required-section enforcement lives downstream
// in clauderun's load-bearing placeholder check.
func TestParse_ACAndChecklist_BothPresent_Rejected(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: x\n\n## Checklist\n\n- [ ] step\n"
	_, err := Parse(body)
	if err == nil {
		t.Fatalf("expected error for body declaring both AC and Checklist")
	}
	if !strings.Contains(err.Error(), "Acceptance Criteria") || !strings.Contains(err.Error(), "Checklist") {
		t.Fatalf("error should name both sections: %v", err)
	}
}

func TestParse_ACOnly_Passes(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParse_ChecklistOnly_Passes(t *testing.T) {
	body := "## Checklist\n\n- [ ] step\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParse_NeitherACNorChecklist_Passes(t *testing.T) {
	// Parser is shape-only; missing required sections are caught downstream
	// at dispatch time via load-bearing placeholders.
	body := "## Description\n\nJust prose.\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractChecklist_CountsCheckedAndUnchecked(t *testing.T) {
	body := "## Checklist\n\n- [x] Done one\n- [ ] Pending two\n- [X] Done three (capital X)\n"
	got := ExtractChecklist(body)
	if !got.Found {
		t.Fatalf("Found: got false, want true")
	}
	if n := len(got.Items); n != 3 {
		t.Fatalf("Items length: got %d, want 3", n)
	}
	if c := got.CheckedCount(); c != 2 {
		t.Fatalf("CheckedCount: got %d, want 2", c)
	}
	wants := []ChecklistItem{
		{Text: "Done one", Checked: true},
		{Text: "Pending two", Checked: false},
		{Text: "Done three (capital X)", Checked: true},
	}
	for i, want := range wants {
		if got.Items[i] != want {
			t.Fatalf("item %d: got %+v, want %+v", i, got.Items[i], want)
		}
	}
}

func TestExtractChecklist_PreservesRawBodyForPromptSubstitution(t *testing.T) {
	raw := "- [x] Item one\n- [ ] Item two"
	body := "## Checklist\n\n" + raw + "\n"
	got := ExtractChecklist(body)
	if got.Body != raw {
		t.Fatalf("Body got %q, want %q", got.Body, raw)
	}
}

func TestExtractChecklist_AbsentReturnsEmpty(t *testing.T) {
	got := ExtractChecklist("## Description\n\nno checklist here\n")
	if got.Found {
		t.Fatalf("Found: got true, want false")
	}
	if len(got.Items) != 0 {
		t.Fatalf("Items: got %d, want 0", len(got.Items))
	}
	if got.CheckedCount() != 0 {
		t.Fatalf("CheckedCount: want 0")
	}
}

func TestExtractChecklist_IgnoresNonCheckboxBullets(t *testing.T) {
	// Plain bullets and prose should not be counted as checklist items.
	body := "## Checklist\n\nIntro line.\n\n- [x] Real item\n- A plain bullet\n* Another bullet\n"
	got := ExtractChecklist(body)
	if n := len(got.Items); n != 1 {
		t.Fatalf("Items length: got %d, want 1", n)
	}
	if got.Items[0].Text != "Real item" || !got.Items[0].Checked {
		t.Fatalf("got %+v, want {Text:'Real item', Checked:true}", got.Items[0])
	}
}

// --- External System Contract Criteria ---------------------------------------

func TestParse_ESCCExtracted_NamesAndVerbatimBody(t *testing.T) {
	escBody := "External System: ERP\n  Shared (stub + real):\n    Given products Apple (1.00)\n    Then ERP has products Apple (1.00)\n  Stub only:\n    Given no products\n    Then ERP has no products"
	body := "## Acceptance Criteria\n\nScenario: list\n  Given a\n  When b\n  Then c\n\n## External System Contract Criteria\n\n" + escBody + "\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	escc := r.ExternalSystemContractCriteria
	if !escc.Found {
		t.Fatalf("ESCC.Found: got false, want true")
	}
	if got, want := escc.Systems, []string{"ERP"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("Systems: got %v, want %v", got, want)
	}
	// Body passes through verbatim — the parser does not interpret the registers.
	if escc.Body != escBody {
		t.Fatalf("Body not verbatim:\n got %q\nwant %q", escc.Body, escBody)
	}
}

func TestParse_ESCCMultipleSystems_InOrder(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n\n## External System Contract Criteria\n\nExternal System: ERP\n  Shared (stub + real):\n    Given products Apple (1.00)\n    Then ERP has products Apple (1.00)\nExternal System: Tax\n  Shared (stub + real):\n    Given rate 0.2\n    Then Tax has rate 0.2\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := r.ExternalSystemContractCriteria.Systems
	want := []string{"ERP", "Tax"}
	if len(got) != len(want) {
		t.Fatalf("Systems length: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Systems[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParse_ESCCAbsent_NoSystems(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	escc := r.ExternalSystemContractCriteria
	if escc.Found {
		t.Fatalf("ESCC.Found: got true, want false")
	}
	if len(escc.Systems) != 0 {
		t.Fatalf("Systems: got %v, want none", escc.Systems)
	}
}

func TestExtractESCC_CaseInsensitiveLabel(t *testing.T) {
	body := "## External System Contract Criteria\n\nexternal system:  ERP \n  Stub only:\n    Given no products\n    Then ERP has no products\n"
	got := ExtractESCC(body)
	if !got.Found {
		t.Fatalf("Found: got false, want true")
	}
	// Name is trimmed even with odd spacing / casing on the label.
	if len(got.Systems) != 1 || got.Systems[0] != "ERP" {
		t.Fatalf("Systems: got %v, want [ERP]", got.Systems)
	}
}

func TestExtractESCC_AbsentReturnsEmpty(t *testing.T) {
	got := ExtractESCC("## Description\n\nno escc here\n")
	if got.Found {
		t.Fatalf("Found: got true, want false")
	}
	if len(got.Systems) != 0 {
		t.Fatalf("Systems: got %d, want 0", len(got.Systems))
	}
}

func TestExtractSection_NestedSubheading(t *testing.T) {
	body := "## Acceptance Criteria\n\n- a\n- b\n\n### Notes\n\nnested\n\n## Checklist\n\n- [ ] step\n"
	got := ExtractSection(body, "Acceptance Criteria")
	if !got.Found {
		t.Fatalf("section not found")
	}
	want := "- a\n- b\n\n### Notes\n\nnested"
	if got.Body != want {
		t.Fatalf("body got %q, want %q", got.Body, want)
	}
}

func TestExtractSection_CaseInsensitive(t *testing.T) {
	body := "## acceptance criteria\n\nx\n"
	got := ExtractSection(body, "Acceptance Criteria")
	if !got.Found {
		t.Fatalf("expected case-insensitive match")
	}
}

func TestExtractSection_EmptySectionTreatedAsAbsent(t *testing.T) {
	body := "## Acceptance Criteria\n\n## Checklist\n\n- [ ] step\n"
	got := ExtractSection(body, "Acceptance Criteria")
	if got.Found {
		t.Fatalf("empty section should report Found=false")
	}
}

func TestExtractSection_AbsentReturnsNotFound(t *testing.T) {
	body := "## Description\n\nx\n"
	got := ExtractSection(body, "Acceptance Criteria")
	if got.Found {
		t.Fatalf("absent section should report Found=false")
	}
	if got.Heading != "Acceptance Criteria" {
		t.Fatalf("Heading should still be set")
	}
}
