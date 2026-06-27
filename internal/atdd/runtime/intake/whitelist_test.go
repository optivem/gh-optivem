package intake

import (
	"strings"
	"testing"
)

// --- Section whitelist --------------------------------------------------------

func TestParse_UnknownHeading_Rejected(t *testing.T) {
	body := "## Description\n\nok\n\n## Random Section\n\nstuff\n"
	_, err := Parse(body)
	if err == nil {
		t.Fatalf("expected error for unknown heading, got nil")
	}
	if !strings.Contains(err.Error(), "Random Section") {
		t.Fatalf("error should name the offending heading: %v", err)
	}
	if !strings.Contains(err.Error(), SectionAcceptanceCriteria) {
		t.Fatalf("error should list the allowed sections: %v", err)
	}
}

func TestParse_UnknownHeadingBeforeCanonical_Rejected(t *testing.T) {
	body := "## Notes\n\nfree text\n\n## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n"
	_, err := Parse(body)
	if err == nil || !strings.Contains(err.Error(), "Notes") {
		t.Fatalf("expected unknown-heading error naming Notes, got: %v", err)
	}
}

func TestParse_AllCanonicalSections_Pass(t *testing.T) {
	// A bug-shaped body exercising every section a single ticket can legally
	// carry alongside AC (Description + Steps + AC + ESCC). Checklist is XOR
	// with AC so it is intentionally absent here.
	escc := "External System: ERP\n  Stub only:\n    Given no products\n    Then ERP has no products"
	body := "## Description\n\nProse.\n\n" +
		"## Steps to Reproduce\n\n1. do a thing\n\n" +
		"## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n\n" +
		"## External System Contract Criteria\n\n" + escc + "\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("valid all-canonical body should parse, got: %v", err)
	}
}

func TestParse_NestedSubheadingUnderCanonical_Tolerated(t *testing.T) {
	// A deeper heading nested inside a canonical section body is part of that
	// body (ExtractSection's depth model), not a separate whitelisted heading.
	// Description carries no format gate, so this isolates the whitelist rule
	// (an AC body would additionally be Gherkin-validated, including the nest).
	body := "## Description\n\nintro prose\n\n### Notes\n\nnested prose\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("nested subheading under Description should be tolerated, got: %v", err)
	}
}

// --- Stray content ------------------------------------------------------------

func TestParse_PreambleBeforeFirstHeading_Rejected(t *testing.T) {
	body := "Some stray preamble.\n\n## Description\n\nok\n"
	_, err := Parse(body)
	if err == nil {
		t.Fatalf("expected error for preamble before first heading, got nil")
	}
	if !strings.Contains(err.Error(), "line 1") || !strings.Contains(err.Error(), "stray") {
		t.Fatalf("error should flag stray content at line 1: %v", err)
	}
}

func TestParse_BlankLinesAndTrailingWhitespace_Tolerated(t *testing.T) {
	body := "\n\n## Description\n\nProse.\n   \n\n## Acceptance Criteria\n\nScenario: x\n  Given a\n  When b\n  Then c\n\n   \n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("blank lines / trailing whitespace should be tolerated, got: %v", err)
	}
}

func TestParse_HTMLComment_Tolerated(t *testing.T) {
	// A comment outside any section — including a multi-line one whose content
	// looks like a heading — must not be flagged as stray or whitelisted.
	body := "<!-- a hidden form hint\n## Not A Real Heading\n-->\n## Description\n\nok\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("HTML comment outside sections should be tolerated, got: %v", err)
	}
}

func TestParse_LeadingH1Title_Tolerated(t *testing.T) {
	// The markdown ticket backend prepends an H1 title; it is depth-1, below the
	// whitelist, and must not read as stray preamble.
	body := "# A Ticket Title\n\n## Description\n\nok\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("leading H1 title should be tolerated, got: %v", err)
	}
}

func TestParse_EmptyBody_Passes(t *testing.T) {
	if _, err := Parse(""); err != nil {
		t.Fatalf("empty body should parse (no sections), got: %v", err)
	}
}

func TestParse_SingleSectionBody_Passes(t *testing.T) {
	if _, err := Parse("## Description\n\njust a description\n"); err != nil {
		t.Fatalf("single-section body should parse, got: %v", err)
	}
}

// --- Checklist is a list ------------------------------------------------------

func TestParse_ChecklistWithProse_Rejected(t *testing.T) {
	body := "## Checklist\n\nThis is prose, not a list.\n"
	_, err := Parse(body)
	if err == nil {
		t.Fatalf("expected error for prose under Checklist, got nil")
	}
	if !strings.Contains(err.Error(), SectionChecklist) || !strings.Contains(err.Error(), "list") {
		t.Fatalf("error should name the Checklist and the list rule: %v", err)
	}
}

func TestParse_ChecklistPlainBullets_Pass(t *testing.T) {
	// Any bullet/number marker is a valid list — the checkbox is optional for
	// the format gate (item parsing into Items stays checkbox-only).
	body := "## Checklist\n\n- bare bullet\n* star bullet\n+ plus bullet\n1. numbered\n2) paren-numbered\n"
	r, err := Parse(body)
	if err != nil {
		t.Fatalf("plain-bullet / numbered Checklist should pass the format gate, got: %v", err)
	}
	if len(r.Checklist.Items) != 0 {
		t.Fatalf("plain bullets must not parse into checkbox Items: got %d", len(r.Checklist.Items))
	}
}

func TestParse_ChecklistContinuationLines_Tolerated(t *testing.T) {
	body := "## Checklist\n\n- [ ] An item\n  with a wrapped continuation line\n- [ ] Another item\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("indented continuation under an item should be tolerated, got: %v", err)
	}
}
