package intake

import (
	"fmt"
	"regexp"
	"strings"
)

// Section is one extracted canonical section of the ticket body.
//
// Found is false when no heading matching Heading appears at H2-or-deeper
// depth, or when the heading exists but its body is empty after trimming.
// The latter case is treated identically to "absent" — an empty section
// carries no signal.
type Section struct {
	Heading string
	Body    string
	Found   bool
}

// ChecklistResult is the Checklist section with its `- [ ]` / `- [x]` items
// pre-parsed. Items is empty when the section is absent or has no checkbox
// lines; the embedded Section still carries the raw body so callers that
// only want the markdown (e.g. the prompt's ${checklist} substitution)
// keep working unchanged.
type ChecklistResult struct {
	Section
	Items []ChecklistItem
}

// ChecklistItem is one parsed `- [ ]` / `- [x]` line.
type ChecklistItem struct {
	Text    string
	Checked bool
}

// ESCCResult is the External System Contract Criteria section with its
// `External System: <name>` lines pre-parsed into Systems. The embedded
// Section carries the raw body verbatim so the contract-test writers receive
// the register bodies unaltered. The parser stays *semantically* dumb — it
// interprets only presence + the named systems, never the meaning of the
// Given/Then bodies — but it does validate the bodies' Gherkin *syntax* at
// intake (see gherkin.go: validateESCCGherkin), so a malformed step fails fast
// here rather than deep in a contract-test writer. Systems is empty when the
// section is absent or declares no `External System:` line.
type ESCCResult struct {
	Section
	Systems []string
}

// CheckedCount returns the number of items with Checked == true.
func (c ChecklistResult) CheckedCount() int {
	n := 0
	for _, it := range c.Items {
		if it.Checked {
			n++
		}
	}
	return n
}

// Result is the parsed shape of a ticket body. Every field corresponds to
// one canonical heading; absent or empty sections are reported with
// Found = false and an empty Body. The runtime decides which sections are
// load-bearing for a given dispatch via the load-bearing placeholder
// check in clauderun (e.g. write-acceptance-tests requires AC, the four
// structural cycles require Checklist) — the parser itself enforces only
// shape rules (see Parse / ParseSections).
type Result struct {
	Description                    Section
	AcceptanceCriteria             Section
	StepsToReproduce               Section
	Checklist                      ChecklistResult
	ExternalSystemContractCriteria ESCCResult
}

// CanonicalHeadings is the ordered list of section headings every ticket
// body may declare. Callers that source sections via
// tracker.Tracker.ReadSections pass this slice as the `headings` argument
// so the adapter returns every canonical section in one call; ParseSections
// then validates the result against the shape rule (AC XOR Checklist).
var CanonicalHeadings = []string{
	SectionDescription,
	SectionAcceptanceCriteria,
	SectionStepsToReproduce,
	SectionChecklist,
	SectionExternalSystemContractCriteria,
}

// Parse extracts canonical sections from issue-body markdown and runs
// shape-level validation. Returns an error when the body is malformed:
// declaring both Acceptance Criteria and Checklist (mutually exclusive at
// intake regardless of ticket-kind), or an Acceptance Criteria / External
// System Contract Criteria section whose step bodies are not well-formed
// Gherkin syntax (see gherkin.go). It validates only syntax, never the pinned
// vocabulary — that stays the test-writers' concern.
//
// Per-kind required-section enforcement (story → AC, the five task
// subtypes that consume ${checklist} → Checklist, etc.) does NOT live
// here. It is enforced by the load-bearing-placeholder check in
// clauderun.go: a prompt that references ${acceptance-criteria} or
// ${checklist} with no value fails dispatch fast. That keeps the parser
// ticket-kind-agnostic so a single PARSE_TICKET service-task can run
// before GATE_TICKET_KIND.
func Parse(body string) (*Result, error) {
	sections := map[string]string{
		SectionDescription:                    ExtractSection(body, SectionDescription).Body,
		SectionAcceptanceCriteria:             ExtractSection(body, SectionAcceptanceCriteria).Body,
		SectionStepsToReproduce:               ExtractSection(body, SectionStepsToReproduce).Body,
		SectionChecklist:                      ExtractSection(body, SectionChecklist).Body,
		SectionExternalSystemContractCriteria: ExtractSection(body, SectionExternalSystemContractCriteria).Body,
	}
	return ParseSections(sections)
}

// ParseSections is the section-keyed counterpart to Parse. It takes the
// already-resolved sections (typically produced by tracker.Tracker.ReadSections
// against CanonicalHeadings) and runs the same shape validation Parse
// runs. Missing keys, empty values, and absent headings are treated
// identically as "section not present" — the same as Parse's body-input
// path, where an empty extracted body collapses to Section.Found = false.
func ParseSections(sections map[string]string) (*Result, error) {
	section := func(name string) Section {
		body := strings.Trim(sections[name], "\n")
		return Section{Heading: name, Body: body, Found: body != ""}
	}
	checklistSec := section(SectionChecklist)
	checklist := ChecklistResult{Section: checklistSec}
	if checklistSec.Found {
		for line := range strings.SplitSeq(checklistSec.Body, "\n") {
			if it, ok := parseChecklistLine(line); ok {
				checklist.Items = append(checklist.Items, it)
			}
		}
	}
	esccSec := section(SectionExternalSystemContractCriteria)
	escc := ESCCResult{Section: esccSec, Systems: externalSystemNames(esccSec.Body)}
	r := &Result{
		Description:                    section(SectionDescription),
		AcceptanceCriteria:             section(SectionAcceptanceCriteria),
		StepsToReproduce:               section(SectionStepsToReproduce),
		Checklist:                      checklist,
		ExternalSystemContractCriteria: escc,
	}
	if r.AcceptanceCriteria.Found && r.Checklist.Found {
		return nil, fmt.Errorf("ticket body declares both Acceptance Criteria and Checklist; pick one matching the ticket-kind")
	}
	// Syntax-validate the Gherkin step bodies (presence only is checked above;
	// these gate well-formedness so a typo'd keyword fails fast here rather than
	// deep in a run, in the test-writer agents). See gherkin.go.
	if r.AcceptanceCriteria.Found {
		if err := validateAcceptanceCriteriaGherkin(r.AcceptanceCriteria.Body); err != nil {
			return nil, err
		}
	}
	if r.ExternalSystemContractCriteria.Found {
		if err := validateESCCGherkin(r.ExternalSystemContractCriteria.Body); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// ExtractSection scans body for an H2-or-deeper markdown heading whose
// text matches name (case-insensitive, exact after dropping leading hashes
// and surrounding whitespace), and returns the section body — every line
// from the next line to (but not including) the next heading at the same
// or shallower depth, with surrounding blank lines trimmed.
//
// Found is true only when the heading is present AND its body is non-empty
// after trimming. An empty section is reported the same as a missing one
// because a downstream consumer cannot do anything with an empty body.
func ExtractSection(body, name string) Section {
	lines := strings.Split(body, "\n")
	startIdx, startDepth := -1, 0
	for i, line := range lines {
		depth, text, ok := headingDepthAndText(line)
		if !ok || depth < 2 {
			continue
		}
		if strings.EqualFold(text, name) {
			startIdx = i + 1
			startDepth = depth
			break
		}
	}
	if startIdx < 0 {
		return Section{Heading: name}
	}
	endIdx := len(lines)
	for i := startIdx; i < len(lines); i++ {
		depth, _, ok := headingDepthAndText(lines[i])
		if !ok {
			continue
		}
		if depth <= startDepth {
			endIdx = i
			break
		}
	}
	body2 := strings.Trim(strings.Join(lines[startIdx:endIdx], "\n"), "\n")
	return Section{Heading: name, Body: body2, Found: body2 != ""}
}

// ExtractChecklist extracts the Checklist section and pre-parses every
// `- [ ]` / `- [x]` line into a typed item. The embedded Section still
// carries the raw body so callers that only need the markdown (e.g. the
// prompt's ${checklist} substitution) are unaffected.
func ExtractChecklist(body string) ChecklistResult {
	sec := ExtractSection(body, SectionChecklist)
	res := ChecklistResult{Section: sec}
	if !sec.Found {
		return res
	}
	for line := range strings.SplitSeq(sec.Body, "\n") {
		if it, ok := parseChecklistLine(line); ok {
			res.Items = append(res.Items, it)
		}
	}
	return res
}

// ExtractESCC extracts the External System Contract Criteria section and
// pre-parses every `External System: <name>` line into Systems. The embedded
// Section carries the raw body verbatim so the contract-test writers receive
// the register bodies unaltered. The parser interprets only presence + the
// named systems — never the *meaning* of the Given/Then register bodies —
// though ParseSections does syntax-validate those bodies as Gherkin (gherkin.go).
func ExtractESCC(body string) ESCCResult {
	sec := ExtractSection(body, SectionExternalSystemContractCriteria)
	return ESCCResult{Section: sec, Systems: externalSystemNames(sec.Body)}
}

// externalSystemNamesRE matches an `External System: <name>` line (optional
// leading indent, case-insensitive label). The captured name is trimmed; the
// register bodies on surrounding lines are ignored — the parser stays dumb.
var externalSystemNamesRE = regexp.MustCompile(`(?i)^\s*External System:\s*(.+?)\s*$`)

// externalSystemNames returns the names declared by every `External System:`
// line in an ESCC body, in declaration order. Returns nil when the body
// declares none (an absent or register-only section).
func externalSystemNames(body string) []string {
	if body == "" {
		return nil
	}
	var names []string
	for line := range strings.SplitSeq(body, "\n") {
		if m := externalSystemNamesRE.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
		}
	}
	return names
}

// checklistLineRE matches a markdown task-list line: optional indent, a
// list marker (`-`, `*`, or `+`), `[ ]` / `[x]` / `[X]`, and the text.
var checklistLineRE = regexp.MustCompile(`^\s*[-*+]\s+\[([ xX])\]\s*(.*)$`)

func parseChecklistLine(line string) (ChecklistItem, bool) {
	m := checklistLineRE.FindStringSubmatch(line)
	if m == nil {
		return ChecklistItem{}, false
	}
	return ChecklistItem{
		Text:    strings.TrimRight(m[2], " \t"),
		Checked: m[1] == "x" || m[1] == "X",
	}, true
}

// headingDepthAndText returns (depth, text, true) when line is a markdown
// heading (`#` followed optionally by more `#` then whitespace then text).
// Depth is the count of leading `#` characters; text is the trimmed
// remainder. Non-heading lines return ok=false.
func headingDepthAndText(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return 0, "", false
	}
	depth := 0
	for depth < len(trimmed) && trimmed[depth] == '#' {
		depth++
	}
	return depth, strings.TrimSpace(trimmed[depth:]), true
}
