package intake

import (
	"strings"
	"testing"
)

// --- Acceptance Criteria Gherkin syntax ---------------------------------------

func TestValidateAC_BareScenarioWithSteps_Passes(t *testing.T) {
	body := "Scenario: list products\n  Given products\n  When I view the list\n  Then I see them"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A typo'd step keyword is absorbed by the parser as scenario *description*
// text and silently dropped. The AST post-check must catch it and report the
// author-relative line (offset-corrected for the prepended synthetic Feature).
func TestValidateAC_TypoStepKeyword_FailsWithAuthorLine(t *testing.T) {
	body := "Scenario: Foo\n  Gven products Apple\n  Then I see them"
	err := validateAcceptanceCriteriaGherkin(body)
	if err == nil {
		t.Fatalf("expected error for typo'd step keyword, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error should report author-relative line 2: %v", err)
	}
	if !strings.Contains(err.Error(), "Gven products Apple") {
		t.Fatalf("error should quote the offending line: %v", err)
	}
	if !strings.Contains(err.Error(), SectionAcceptanceCriteria) {
		t.Fatalf("error should name the section: %v", err)
	}
}

func TestValidateAC_LeadingFeaturePresent_Passes(t *testing.T) {
	body := "Feature: ordering\n\nScenario: Foo\n  Given a\n  When b\n  Then c"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error (no double-Feature expected): %v", err)
	}
}

// The @isolated tag and a `# isolated: …` Gherkin comment must pass — neither
// is scenario description text.
func TestValidateAC_IsolatedTagAndComment_Passes(t *testing.T) {
	body := "@isolated\n# isolated: mutates the cancellation-blackout clock\nScenario: Foo\n  Given a\n  When b\n  Then c"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error for @isolated tag + comment: %v", err)
	}
}

// A `Feature:` → `Rule:` → `Scenario:` grouping is official Gherkin v6+ and the
// parser already descends into Rule children, so a Rule-nested AC body must pass
// the syntax gate unchanged — `Rule:` support is additive.
func TestValidateAC_RuleNesting_Passes(t *testing.T) {
	body := "Feature: Checkout\n  Rule: Shipping is $0.10/kg per unit\n\n    Scenario: single item\n      Given a product weighing 2kg\n      When I check out 1\n      Then shipping is $0.20\n\n    Scenario: scales with quantity\n      Given a product weighing 2kg\n      When I check out 3\n      Then shipping is $0.60"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error for Rule-nested AC: %v", err)
	}
}

// Story tickets wrap the AC in a markdown code fence — the story Issue Form's
// `render: markdown` wraps form submissions in ```markdown, and corpus tickets
// hand-author ```gherkin. The enclosing fence must be stripped before parsing
// so its lines are not mis-read as DocString separators (the closing ``` after
// the last step would otherwise open a DocString that never closes → EOF).
func TestValidateAC_FencedGherkin_Passes(t *testing.T) {
	body := "```gherkin\nFeature: Charge shipping based on product weight\n\n  Rule: Shipping fee is $0.10 per kg per unit\n\n    Scenario: derived from weight\n      Given a product weighing 2kg\n      When I check out 1\n      Then shipping is 0.20\n```"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error for ```gherkin-fenced AC: %v", err)
	}
}

func TestValidateAC_FencedMarkdown_Passes(t *testing.T) {
	body := "```markdown\nScenario: Foo\n  Given a\n  When b\n  Then c\n```"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error for ```markdown-fenced AC: %v", err)
	}
}

func TestValidateAC_FencedBare_Passes(t *testing.T) {
	body := "```\nScenario: Foo\n  Given a\n  When b\n  Then c\n```"
	if err := validateAcceptanceCriteriaGherkin(body); err != nil {
		t.Fatalf("unexpected error for bare-fenced AC: %v", err)
	}
}

// A typo'd step inside a fenced body must still fail, and the reported line must
// be author-relative — i.e. counted against the original fenced body (where the
// opening fence is line 1), not the de-fenced inner content.
func TestValidateAC_FencedTypoStep_FailsWithAuthorLine(t *testing.T) {
	// line 1: ```gherkin
	// line 2: Scenario: Foo
	// line 3:   Gven products Apple   <- typo
	// line 4:   Then I see them
	// line 5: ```
	body := "```gherkin\nScenario: Foo\n  Gven products Apple\n  Then I see them\n```"
	err := validateAcceptanceCriteriaGherkin(body)
	if err == nil {
		t.Fatalf("expected error for typo'd step in fenced AC, got nil")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error should report author-relative line 3: %v", err)
	}
	if !strings.Contains(err.Error(), "Gven products Apple") {
		t.Fatalf("error should quote the offending line: %v", err)
	}
}

func TestValidateAC_ScenarioWithoutSteps_Fails(t *testing.T) {
	body := "Scenario: empty"
	err := validateAcceptanceCriteriaGherkin(body)
	if err == nil {
		t.Fatalf("expected error for stepless scenario, got nil")
	}
	if !strings.Contains(err.Error(), "no steps") {
		t.Fatalf("error should mention no steps: %v", err)
	}
}

// --- ESCC Gherkin syntax ------------------------------------------------------

func TestValidateESCC_ValidMultiSystem_Passes(t *testing.T) {
	body := "External System: ERP\n  Shared (stub + real):\n    Given products Apple (1.00)\n    Then ERP has products Apple (1.00)\n  Stub only:\n    Given no products\n    Then ERP has no products\nExternal System: Tax\n  Shared (stub + real):\n    Given rate 0.2\n    Then Tax has rate 0.2"
	if err := validateESCCGherkin(body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A typo'd ESCC step must fail and report the *original* ESCC line number, not
// the synthetic translated-document line.
func TestValidateESCC_TypoStep_FailsWithOriginalLine(t *testing.T) {
	// line 1: External System: ERP
	// line 2:   Stub only:
	// line 3:     Gven no products   <- typo
	// line 4:     Then ERP has no products
	body := "External System: ERP\n  Stub only:\n    Gven no products\n    Then ERP has no products"
	err := validateESCCGherkin(body)
	if err == nil {
		t.Fatalf("expected error for typo'd ESCC step, got nil")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("error should report original ESCC line 3: %v", err)
	}
	if !strings.Contains(err.Error(), "Gven no products") {
		t.Fatalf("error should quote the offending line: %v", err)
	}
}

func TestValidateESCC_StepOutsideRegister_Fails(t *testing.T) {
	// A step line directly under External System with no register sub-header.
	body := "External System: ERP\n    Given stray step"
	err := validateESCCGherkin(body)
	if err == nil {
		t.Fatalf("expected error for step outside any register, got nil")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error should report line 2: %v", err)
	}
	if !strings.Contains(err.Error(), "register") {
		t.Fatalf("error should mention the missing register: %v", err)
	}
}

// A fenced ESCC body must strip its enclosing fence before translation, the
// same as AC.
func TestValidateESCC_Fenced_Passes(t *testing.T) {
	body := "```gherkin\nExternal System: ERP\n  Shared (stub + real):\n    Given products Apple (1.00)\n    Then ERP has products Apple (1.00)\n```"
	if err := validateESCCGherkin(body); err != nil {
		t.Fatalf("unexpected error for fenced ESCC: %v", err)
	}
}

// --- Integration through Parse: absent sections are no-ops --------------------

func TestParse_AbsentSections_NoGherkinValidation(t *testing.T) {
	body := "## Description\n\nJust prose, no AC and no ESCC.\n"
	if _, err := Parse(body); err != nil {
		t.Fatalf("unexpected error for body with no AC/ESCC: %v", err)
	}
}

func TestParse_MalformedAC_SurfacesAtParse(t *testing.T) {
	body := "## Acceptance Criteria\n\nScenario: Foo\n  Gven products Apple\n  Then I see them\n"
	_, err := Parse(body)
	if err == nil {
		t.Fatalf("expected Parse error for malformed AC, got nil")
	}
	if !strings.Contains(err.Error(), SectionAcceptanceCriteria) {
		t.Fatalf("error should name the AC section: %v", err)
	}
}

func TestParse_MalformedESCC_SurfacesAtParse(t *testing.T) {
	escc := "External System: ERP\n  Stub only:\n    Gven no products\n    Then ERP has no products"
	body := "## Acceptance Criteria\n\nScenario: x\n  Given a\n  Then b\n\n## External System Contract Criteria\n\n" + escc + "\n"
	_, err := Parse(body)
	if err == nil {
		t.Fatalf("expected Parse error for malformed ESCC, got nil")
	}
	if !strings.Contains(err.Error(), SectionExternalSystemContractCriteria) {
		t.Fatalf("error should name the ESCC section: %v", err)
	}
}
