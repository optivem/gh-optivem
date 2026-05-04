package intake

import (
	"strings"
	"testing"
)

func TestParse_StoryRequiresAcceptanceCriteria(t *testing.T) {
	body := "## Description\n\nStuff.\n\n## Acceptance Criteria\n\nScenario: Foo\n  Given x\n  When y\n  Then z\n"
	r, err := Parse(body, "story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.AcceptanceCriteria.Found {
		t.Fatalf("AcceptanceCriteria.Found: got false, want true")
	}
	if r.LegacyAcceptanceCriteria.Found {
		t.Fatalf("LegacyAcceptanceCriteria should be absent")
	}
}

func TestParse_StoryMissingACFails(t *testing.T) {
	body := "## Description\n\nNo AC.\n"
	_, err := Parse(body, "story")
	if err == nil {
		t.Fatalf("expected error for missing Acceptance Criteria")
	}
	if !strings.Contains(err.Error(), "Acceptance Criteria") {
		t.Fatalf("error should mention Acceptance Criteria: %v", err)
	}
}

func TestParse_BugRequiresStepsAndAC(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "missing_steps",
			body:    "## Acceptance Criteria\n\nScenario: x\n",
			wantErr: "Steps to Reproduce",
		},
		{
			name:    "missing_ac",
			body:    "## Steps to Reproduce\n\n1. step\n",
			wantErr: "Acceptance Criteria",
		},
		{
			name:    "missing_both",
			body:    "## Description\n\nfoo\n",
			wantErr: "Steps to Reproduce, Acceptance Criteria",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.body, "bug")
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestParse_BugWithBothSectionsPasses(t *testing.T) {
	body := "## Steps to Reproduce\n\n1. one\n\n## Acceptance Criteria\n\nScenario: ok\n"
	r, err := Parse(body, "bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.StepsToReproduce.Found || !r.AcceptanceCriteria.Found {
		t.Fatalf("expected both sections found, got steps=%v ac=%v", r.StepsToReproduce.Found, r.AcceptanceCriteria.Found)
	}
}

func TestParse_TaskRequiresChecklist(t *testing.T) {
	body := "## Description\n\nNo checklist.\n"
	_, err := Parse(body, "task")
	if err == nil {
		t.Fatalf("expected error for missing Checklist")
	}
	if !strings.Contains(err.Error(), "Checklist") {
		t.Fatalf("error should mention Checklist: %v", err)
	}
}

func TestParse_TaskWithChecklistPasses(t *testing.T) {
	body := "## Checklist\n\n- [ ] One\n- [ ] Two\n"
	r, err := Parse(body, "task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.Checklist.Found {
		t.Fatalf("Checklist not found")
	}
}

func TestParse_LegacyAcceptanceCriteriaOptional(t *testing.T) {
	withLegacy := "## Acceptance Criteria\n\nScenario: x\n\n## Legacy Acceptance Criteria\n\n- old\n"
	r, err := Parse(withLegacy, "story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !r.LegacyAcceptanceCriteria.Found {
		t.Fatalf("LegacyAcceptanceCriteria.Found should be true")
	}

	withoutLegacy := "## Acceptance Criteria\n\nScenario: x\n"
	r2, err := Parse(withoutLegacy, "story")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r2.LegacyAcceptanceCriteria.Found {
		t.Fatalf("LegacyAcceptanceCriteria.Found should be false when absent")
	}
}

func TestParse_RejectsEmptyTicketType(t *testing.T) {
	if _, err := Parse("## Acceptance Criteria\n\nx\n", ""); err == nil {
		t.Fatalf("expected error for empty ticket_type")
	}
}

func TestParse_RejectsUnknownTicketType(t *testing.T) {
	_, err := Parse("## Description\n\nx\n", "epic")
	if err == nil {
		t.Fatalf("expected error for unknown ticket_type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error should mention unsupported: %v", err)
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
