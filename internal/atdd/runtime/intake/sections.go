// Package intake holds the deterministic markdown parser that replaces the
// three LLM-driven intake agents (atdd-story / atdd-bug / task).
// Issue Forms (.github/ISSUE_TEMPLATE/*.yml) enforce the
// canonical heading shape, so the runtime can extract sections without an
// LLM. The constants in this file are the single source of truth for those
// headings; both the form labels and the parser key off them.
package intake

// Canonical issue-body section headings. The Issue Forms render each
// textarea as a markdown heading whose text equals the form's `label:`
// attribute. These constants must match those labels exactly, case-
// sensitive — a drift between the form YAML and these strings will surface
// as STOP_PARSE_ERROR at runtime, which is the intended behavior.
const (
	SectionDescription              = "Description"
	SectionAcceptanceCriteria       = "Acceptance Criteria"
	SectionLegacyAcceptanceCriteria = "Legacy Acceptance Criteria"
	SectionChecklist                = "Checklist"
	SectionStepsToReproduce         = "Steps to Reproduce"
)
