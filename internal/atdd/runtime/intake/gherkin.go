package intake

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	gherkin "github.com/cucumber/gherkin/go/v33"
	messages "github.com/cucumber/messages/go/v28"
)

// Gherkin syntax validation for the Acceptance Criteria and External System
// Contract Criteria sections. The parser stays *semantically* dumb — it still
// interprets only section presence and the `External System:` names. These
// helpers add a pure *syntax* gate: they confirm the Given/Then step bodies are
// well-formed Gherkin so a typo'd keyword fails fast as STOP_PARSE_ERROR at
// intake instead of surfacing deep in a run, in the test-writer agents.
//
// Why parse-success alone is not enough: Gherkin treats any non-keyword line
// immediately after `Scenario:`/`Example:` as free-form *description* text, so a
// typo'd step (`Gven products Apple`) parses "successfully" — it lands in
// `Scenario.Description` and is silently dropped. assertGherkinSyntax therefore
// inspects the AST and rejects any non-empty scenario description (neither
// section authors prose descriptions — the `@isolated` reason is a `#` Gherkin
// comment, which the parser ignores, not a description).

// newGherkinID is the id generator passed to the parser. We never use the
// generated ids, so a constant empty string is fine and keeps parses
// deterministic.
func newGherkinID() string { return "" }

// parseErrLocRE matches the `(line:col)` location prefix the cucumber parser
// emits in its error strings (single error and the multi-line `Parser errors:`
// form). remapErrorLines rewrites the line number through a line map so the
// reported location points at the author's original text, not the synthetic
// document we feed the parser.
var parseErrLocRE = regexp.MustCompile(`\((\d+):(\d+)\)`)

// codeFenceRE matches a markdown code-fence run (three or more backticks or
// tildes) at the start of a trimmed line. The capture is the fence run itself;
// any info string (`gherkin`, `markdown`, …) follows it and is ignored.
var codeFenceRE = regexp.MustCompile("^(`{3,}|~{3,})")

// validateAcceptanceCriteriaGherkin validates that an Acceptance Criteria body
// is well-formed Gherkin. AC is authored as bare `Scenario:` blocks with no
// `Feature:` header, so a single synthetic `Feature: _` line is prepended
// (unless the body already starts with a `Feature:` line after optional leading
// tags/comments). Reported line numbers are offset-corrected back to the
// author's body.
func validateAcceptanceCriteriaGherkin(body string) error {
	src, prepended, stripped := acceptanceGherkinSource(body)
	lineMap := func(n int) int {
		if m := n - prepended; m >= 1 {
			return m + stripped
		}
		return n
	}
	doc, err := gherkin.ParseGherkinDocument(strings.NewReader(src), newGherkinID)
	if err != nil {
		return fmt.Errorf("%s: %s", SectionAcceptanceCriteria, remapErrorLines(err.Error(), lineMap))
	}
	return assertGherkinSyntax(doc, SectionAcceptanceCriteria, lineMap)
}

// validateESCCGherkin validates that an External System Contract Criteria body
// is well-formed Gherkin. The authored ESCC format is not Gherkin — it is
// translated internally (esccGherkinSource) into a single synthetic document:
// `Feature: _` → each `External System: X` → `Rule: X` → each register
// sub-header → `Scenario:`, then the verbatim step lines. A line map tracks each
// synthetic line back to its original ESCC line so every error reports the
// author's line number.
func validateESCCGherkin(body string) error {
	src, srcToOrig, err := esccGherkinSource(body)
	if err != nil {
		return err
	}
	lineMap := func(n int) int {
		if n >= 1 && n <= len(srcToOrig) {
			if o := srcToOrig[n-1]; o >= 1 {
				return o
			}
		}
		return n
	}
	doc, err := gherkin.ParseGherkinDocument(strings.NewReader(src), newGherkinID)
	if err != nil {
		return fmt.Errorf("%s: %s", SectionExternalSystemContractCriteria, remapErrorLines(err.Error(), lineMap))
	}
	return assertGherkinSyntax(doc, SectionExternalSystemContractCriteria, lineMap)
}

// acceptanceGherkinSource returns the source to feed the parser, the number of
// synthetic lines prepended (0 or 1), and the number of leading lines stripped
// from an enclosing markdown code fence. A `Feature: _` line is prepended
// unless the body already opens with a `Feature:` line (after optional leading
// tags / comments / blanks), so authors keep writing bare scenarios. An
// enclosing ```/~~~ fence (the story Issue Form's `render:` wrapper, or a
// hand-authored ```gherkin block) is stripped first so the fence lines are not
// mis-parsed as DocString separators; stripped is folded back into the caller's
// line map so reported locations stay author-relative.
func acceptanceGherkinSource(body string) (string, int, int) {
	inner, stripped := stripEnclosingCodeFence(body)
	if hasLeadingFeature(inner) {
		return inner, 0, stripped
	}
	return "Feature: _\n" + inner, 1, stripped
}

// hasLeadingFeature reports whether body's first significant line (skipping
// blank lines, `@tag` lines, and `#` comments) is a `Feature:` line.
func hasLeadingFeature(body string) bool {
	for _, raw := range strings.Split(body, "\n") {
		t := strings.TrimSpace(raw)
		if t == "" || strings.HasPrefix(t, "@") || strings.HasPrefix(t, "#") {
			continue
		}
		return strings.HasPrefix(t, "Feature:")
	}
	return false
}

// stripEnclosingCodeFence removes a single markdown code fence that wholly
// encloses body and returns the inner content plus the number of leading lines
// removed (the leading blanks before the opener plus the opener line itself).
// It fires only when body's first non-blank line is a fence opener (```/~~~,
// three or more, with or without an info string) AND its last non-blank line is
// a matching closer (same fence char, at least as long, no info string). On any
// other shape — no fence, an unterminated fence, or a fence wrapping only part
// of the body — it returns (body, 0) unchanged. The leading-line count lets
// callers keep Gherkin error locations author-relative.
func stripEnclosingCodeFence(body string) (string, int) {
	lines := strings.Split(body, "\n")
	first := -1
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			first = i
			break
		}
	}
	if first < 0 {
		return body, 0
	}
	fence := codeFenceRE.FindString(strings.TrimSpace(lines[first]))
	if fence == "" {
		return body, 0
	}
	last := -1
	for i := len(lines) - 1; i > first; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			last = i
			break
		}
	}
	if last < 0 || !isClosingFence(strings.TrimSpace(lines[last]), fence[0], len(fence)) {
		return body, 0
	}
	return strings.Join(lines[first+1:last], "\n"), first + 1
}

// isClosingFence reports whether s is a bare closing fence: only the opener's
// fence character, repeated at least as many times as the opener, with no
// trailing info string (CommonMark forbids an info string on the closer).
func isClosingFence(s string, fenceChar byte, minLen int) bool {
	if len(s) < minLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != fenceChar {
			return false
		}
	}
	return true
}

// esccRegisterLabel reports whether a trimmed line is one of the two ESCC
// register sub-headers and returns its canonical label. The match is
// case-insensitive on the label text but the canonical form is emitted into the
// synthetic Scenario name.
func esccRegisterLabel(trimmed string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(strings.TrimSuffix(trimmed, ":"))) {
	case "shared (stub + real)":
		return "Shared (stub + real)", true
	case "stub only":
		return "Stub only", true
	}
	return "", false
}

// esccGherkinSource translates an authored ESCC body into a synthetic Gherkin
// document and returns it alongside a slice mapping each synthetic line number
// (1-based) to its original ESCC line number (srcToOrig[i] is the origin of
// synthetic line i+1). Structural errors that the Gherkin parser cannot express
// — a register before any `External System:`, or a step line under no register
// — are returned directly with the author's line number.
func esccGherkinSource(body string) (string, []int, error) {
	var sb strings.Builder
	var srcToOrig []int
	emit := func(text string, origLine int) {
		sb.WriteString(text)
		sb.WriteByte('\n')
		srcToOrig = append(srcToOrig, origLine)
	}
	emit("Feature: _", 0)

	inner, stripped := stripEnclosingCodeFence(body)
	haveRule, haveRegister := false, false
	for i, raw := range strings.Split(inner, "\n") {
		origLine := i + 1 + stripped
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if m := externalSystemNamesRE.FindStringSubmatch(raw); m != nil {
			emit("  Rule: "+m[1], origLine)
			haveRule, haveRegister = true, false
			continue
		}
		if label, ok := esccRegisterLabel(trimmed); ok {
			if !haveRule {
				return "", nil, fmt.Errorf("%s: line %d: register %q appears before any 'External System:' line", SectionExternalSystemContractCriteria, origLine, label)
			}
			emit("    Scenario: "+label, origLine)
			haveRegister = true
			continue
		}
		// Any other non-blank line is a step body and must sit under a register.
		if !haveRegister {
			return "", nil, fmt.Errorf("%s: line %d: %q is not under a register sub-header (expected 'Shared (stub + real):' or 'Stub only:')", SectionExternalSystemContractCriteria, origLine, trimmed)
		}
		emit("      "+trimmed, origLine)
	}
	return sb.String(), srcToOrig, nil
}

// assertGherkinSyntax walks the parsed document and rejects the failure modes a
// successful parse leaves behind: a non-empty scenario description (a typo'd
// step keyword the parser absorbed as prose) and a scenario with no steps. It
// covers both top-level scenarios (AC) and Rule-nested scenarios (ESCC).
func assertGherkinSyntax(doc *messages.GherkinDocument, section string, lineMap func(int) int) error {
	if doc == nil || doc.Feature == nil {
		return nil
	}
	for _, child := range doc.Feature.Children {
		if child.Scenario != nil {
			if err := checkScenario(child.Scenario, section, lineMap); err != nil {
				return err
			}
		}
		if child.Rule != nil {
			for _, rc := range child.Rule.Children {
				if rc.Scenario != nil {
					if err := checkScenario(rc.Scenario, section, lineMap); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// checkScenario enforces the two AST post-checks for a single scenario: no
// description text (the typo-catching check) and at least one step.
func checkScenario(sc *messages.Scenario, section string, lineMap func(int) int) error {
	if desc := strings.TrimSpace(sc.Description); desc != "" {
		first := strings.TrimSpace(strings.SplitN(desc, "\n", 2)[0])
		line := lineMap(int(sc.Location.Line) + 1)
		return fmt.Errorf("%s: line %d: %q is not a valid Gherkin step (did you mistype Given/When/Then/And/But?)", section, line, first)
	}
	if len(sc.Steps) == 0 {
		return fmt.Errorf("%s: line %d: scenario %q has no steps", section, lineMap(int(sc.Location.Line)), sc.Name)
	}
	return nil
}

// remapErrorLines rewrites every `(line:col)` location in a parser error string
// through lineMap so reported locations point at the author's original text.
func remapErrorLines(msg string, lineMap func(int) int) string {
	return parseErrLocRE.ReplaceAllStringFunc(msg, func(loc string) string {
		sub := parseErrLocRE.FindStringSubmatch(loc)
		line, _ := strconv.Atoi(sub[1])
		return fmt.Sprintf("(%d:%s)", lineMap(line), sub[2])
	})
}
