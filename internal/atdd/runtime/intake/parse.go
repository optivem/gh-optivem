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

// Result is the parsed shape of a ticket body. The fields a downstream
// cycle reads depend on ticket_type — story/bug consume AcceptanceCriteria,
// task consumes Checklist; LegacyAcceptanceCriteria is orthogonal and
// optional on every type.
type Result struct {
	Description              Section
	AcceptanceCriteria       Section
	StepsToReproduce         Section
	Checklist                ChecklistResult
	LegacyAcceptanceCriteria Section
}

// Parse extracts canonical sections from issue-body markdown for the given
// ticket type. Returns an error listing every required section that is
// missing or empty; callers surface that error as STOP_PARSE_ERROR with a
// "fix the body, re-run" resolution.
//
// Required sections by ticket type (matching the Issue Form templates):
//
//	story → Acceptance Criteria
//	bug   → Steps to Reproduce, Acceptance Criteria
//	task  → Checklist
//
// Description and Legacy Acceptance Criteria are optional for every type.
func Parse(body, ticketType string) (*Result, error) {
	r := &Result{
		Description:              ExtractSection(body, SectionDescription),
		AcceptanceCriteria:       ExtractSection(body, SectionAcceptanceCriteria),
		StepsToReproduce:         ExtractSection(body, SectionStepsToReproduce),
		Checklist:                ExtractChecklist(body),
		LegacyAcceptanceCriteria: ExtractSection(body, SectionLegacyAcceptanceCriteria),
	}

	var missing []string
	switch ticketType {
	case "story":
		if !r.AcceptanceCriteria.Found {
			missing = append(missing, SectionAcceptanceCriteria)
		}
	case "bug":
		if !r.StepsToReproduce.Found {
			missing = append(missing, SectionStepsToReproduce)
		}
		if !r.AcceptanceCriteria.Found {
			missing = append(missing, SectionAcceptanceCriteria)
		}
	case "task":
		if !r.Checklist.Found {
			missing = append(missing, SectionChecklist)
		}
	case "":
		return nil, fmt.Errorf("ticket_type is empty — classify_ticket_type must run before parse")
	default:
		return nil, fmt.Errorf("unsupported ticket_type %q (expected story | bug | task)", ticketType)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required section(s) for %s: %s", ticketType, strings.Join(missing, ", "))
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
