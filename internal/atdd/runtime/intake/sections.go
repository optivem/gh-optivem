// Package intake holds the deterministic markdown parser that replaces the
// three LLM-driven intake agents (atdd-story / atdd-bug / task).
// Issue Forms (.github/ISSUE_TEMPLATE/*.yml) enforce the
// canonical heading shape, so the runtime can extract sections without an
// LLM. The constants in this file are the single source of truth for those
// headings; both the form labels and the parser key off them.
//
// The parser enforces a *closed* contract on the body (see Parse): it may
// contain only these canonical sections, with no unknown headings and no
// content outside a recognized section body. The whitelist is the union across
// all ticket kinds — exactly the sections below — never per-kind, because
// PARSE_TICKET runs before the ticket kind is known.
package intake

// Canonical issue-body section headings. The Issue Forms render each
// textarea as a markdown heading whose text equals the form's `label:`
// attribute. These constants must match those labels exactly, case-
// sensitive — a drift between the form YAML and these strings will surface
// as STOP_PARSE_ERROR at runtime, which is the intended behavior.
const (
	SectionDescription        = "Description"
	SectionAcceptanceCriteria = "Acceptance Criteria"
	SectionChecklist          = "Checklist"
	SectionStepsToReproduce   = "Steps to Reproduce"
	// SectionExternalSystemContractCriteria is the optional spec for the
	// inner / contract loop — present only on stories that cross an external
	// system boundary. Its presence opens the contract/stub room and each
	// `External System: <name>` line names a boundary. The register bodies
	// pass through verbatim to the contract-test writers; the parser stays
	// *semantically* dumb (it interprets only presence + names) but does
	// syntax-validate the bodies as Gherkin at intake — see gherkin.go and
	// internal/atdd/assets/runtime/shared/escc-format.md.
	SectionExternalSystemContractCriteria = "External System Contract Criteria"
)
