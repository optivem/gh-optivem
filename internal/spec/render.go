package spec

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// PRCommentMarker is the first line of the sticky pull-request comment, so that
// a later run can find and replace the comment it posted earlier.
const PRCommentMarker = "<!-- gh-optivem-spec -->"

// allStates lists the lifecycle states in display order.
var allStates = []State{StateVerified, StateInProgress, StateFailing, StatePlanned, StateWaived}

// stateRules describes each state for the lifecycle legend.
var stateRules = map[State]string{
	StateVerified:   "Every linked acceptance test passes and none is marked wip.",
	StateInProgress: "A linked acceptance test is marked wip: the feature is being built.",
	StateFailing:    "A linked acceptance test is red and not marked wip. Always blocks the gate.",
	StatePlanned:    "No passing acceptance test is linked yet.",
	StateWaived:     "Exempted by an approved, time-limited waiver. Counts as not verified.",
}

// stateColors are the pill and bar fill colours per state.
var stateColors = map[State]string{
	StateVerified:   "#2E7D4F",
	StateInProgress: "#2F6FDB",
	StateFailing:    "#C0392B",
	StatePlanned:    "#6B7580",
	StateWaived:     "#7E6BB8",
}

func stateCount(s Summary, st State) int {
	switch st {
	case StateVerified:
		return s.Verified
	case StateInProgress:
		return s.InProgress
	case StateFailing:
		return s.Failing
	case StatePlanned:
		return s.Planned
	case StateWaived:
		return s.Waived
	}
	return 0
}

// coverageLine is the definition-bearing statement of spec coverage.
func coverageLine(rep Report) string {
	return fmt.Sprintf("Spec coverage: %d of %d acceptance criteria verified (%.1f%%)",
		rep.Summary.Verified, rep.Summary.Total, rep.Summary.CoveragePercent)
}

func verdict(rep Report) string {
	switch {
	case rep.Gate == nil:
		return "NOT GATED"
	case rep.Gate.Passed:
		return "PASSED"
	}
	return "FAILED"
}

func useCasesVerified(rep Report) (verified int) {
	for _, uc := range rep.UseCases {
		if uc.State == StateVerified {
			verified++
		}
	}
	return verified
}

// criterionNotes returns the short annotations shown next to a criterion.
func criterionNotes(c ReportCriterion) []string {
	var notes []string
	if c.Flag != nil {
		if c.FlagOn {
			notes = append(notes, "flag "+*c.Flag+" on")
		} else {
			notes = append(notes, "flag "+*c.Flag+" off")
		}
	}
	if c.Baselined {
		notes = append(notes, "baselined")
	}
	if c.Waiver != nil {
		notes = append(notes, fmt.Sprintf("waived until %s by %s: %s", c.Waiver.Until, c.Waiver.Approver, c.Waiver.Reason))
	}
	return notes
}

func testNames(c ReportCriterion) string {
	names := make([]string, 0, len(c.Tests))
	for _, t := range c.Tests {
		n := t.Name
		if t.WIP {
			n += " (wip)"
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}

// RenderText renders the terminal report.
func RenderText(rep Report) string {
	var b strings.Builder
	fmt.Fprintln(&b, coverageLine(rep))
	fmt.Fprintf(&b, "Mode: %s (threshold %.1f%%)   Gate: %s\n", rep.Mode, rep.Threshold, verdict(rep))
	parts := make([]string, 0, len(allStates))
	for _, st := range allStates {
		parts = append(parts, fmt.Sprintf("%s %d", st, stateCount(rep.Summary, st)))
	}
	fmt.Fprintln(&b, strings.Join(parts, " | "))
	for _, uc := range rep.UseCases {
		fmt.Fprintf(&b, "\n%s %s [%s]\n", uc.ID, uc.Title, uc.State)
		group := "\x00"
		for _, c := range uc.Criteria {
			if c.Group != group {
				group = c.Group
				if group != "" {
					fmt.Fprintf(&b, "  %s\n", group)
				}
			}
			fmt.Fprintf(&b, "    %-13s %-10s %s\n", "["+string(c.State)+"]", c.ID, c.Text)
			for _, n := range criterionNotes(c) {
				fmt.Fprintf(&b, "                  note: %s\n", n)
			}
		}
	}
	writeTextList(&b, "Orphan tests (not linked to any criterion)", len(rep.Orphans), func(i int) string {
		return fmt.Sprintf("%s: %s", rep.Orphans[i].Name, rep.Orphans[i].Reason)
	})
	writeTextList(&b, "Tests referencing unknown criterion IDs", len(rep.UnknownIDs), func(i int) string {
		return fmt.Sprintf("%s in %s", rep.UnknownIDs[i].ID, rep.UnknownIDs[i].Test)
	})
	writeTextList(&b, "Spec validation errors", len(rep.Validation.Errors), func(i int) string {
		return fmt.Sprintf("%s: %s", rep.Validation.Errors[i].Code, rep.Validation.Errors[i].Message)
	})
	if rep.Gate != nil {
		writeTextList(&b, "Gate reasons", len(rep.Gate.Reasons), func(i int) string { return rep.Gate.Reasons[i] })
	}
	return b.String()
}

func writeTextList(b *strings.Builder, title string, n int, item func(int) string) {
	if n == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n", title)
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "  - %s\n", item(i))
	}
}

// mdCell makes s safe inside a markdown table cell.
func mdCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\r", " ", "\n", " ", "<", "&lt;", ">", "&gt;").Replace(s)
}

// mdText makes s safe in running markdown text (no raw HTML).
func mdText(s string) string {
	return strings.NewReplacer("<", "&lt;", ">", "&gt;", "\r", " ", "\n", " ").Replace(s)
}

func mdVerdictLine(rep Report) string {
	switch {
	case rep.Gate == nil:
		return fmt.Sprintf("**Gate not evaluated** (mode %s, threshold %.1f%%)", rep.Mode, rep.Threshold)
	case rep.Gate.Passed:
		return fmt.Sprintf("**Gate passed** (mode %s, threshold %.1f%%)", rep.Mode, rep.Threshold)
	}
	return fmt.Sprintf("**Gate failed** (mode %s, threshold %.1f%%)", rep.Mode, rep.Threshold)
}

func writeMDCounts(b *strings.Builder, rep Report) {
	b.WriteString("| State | Criteria |\n|---|---:|\n")
	for _, st := range allStates {
		fmt.Fprintf(b, "| %s | %d |\n", st, stateCount(rep.Summary, st))
	}
}

func writeMDProblems(b *strings.Builder, rep Report) {
	if rep.Gate != nil && len(rep.Gate.Reasons) > 0 {
		b.WriteString("\n### Gate reasons\n\n")
		for _, r := range rep.Gate.Reasons {
			fmt.Fprintf(b, "- %s\n", mdText(r))
		}
	}
	if len(rep.Validation.Errors) > 0 {
		b.WriteString("\n### Spec validation errors\n\n")
		for _, e := range rep.Validation.Errors {
			fmt.Fprintf(b, "- `%s`: %s\n", e.Code, mdText(e.Message))
		}
	}
	if len(rep.Orphans) > 0 {
		b.WriteString("\n### Orphan tests\n\n")
		for _, o := range rep.Orphans {
			fmt.Fprintf(b, "- `%s`: %s\n", mdText(o.Name), mdText(o.Reason))
		}
	}
	if len(rep.UnknownIDs) > 0 {
		b.WriteString("\n### Tests referencing unknown criterion IDs\n\n")
		for _, u := range rep.UnknownIDs {
			fmt.Fprintf(b, "- `%s` in `%s`\n", mdText(u.ID), mdText(u.Test))
		}
	}
}

// RenderSummaryMarkdown renders the GitHub job summary (GITHUB_STEP_SUMMARY).
func RenderSummaryMarkdown(rep Report) string {
	var b strings.Builder
	b.WriteString("## Spec coverage\n\n")
	fmt.Fprintf(&b, "%s\n\n%s\n\n", coverageLine(rep), mdVerdictLine(rep))
	writeMDCounts(&b, rep)
	for _, uc := range rep.UseCases {
		fmt.Fprintf(&b, "\n### %s %s (%s)\n\n", uc.ID, mdText(uc.Title), uc.State)
		b.WriteString("| Criterion | State | Text | Tests |\n|---|---|---|---|\n")
		for _, c := range uc.Criteria {
			text := mdCell(c.Text)
			if notes := criterionNotes(c); len(notes) > 0 {
				text += " _(" + mdCell(strings.Join(notes, "; ")) + ")_"
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.ID, c.State, text, mdCell(testNames(c)))
		}
	}
	writeMDProblems(&b, rep)
	return b.String()
}

// RenderPRComment renders the sticky pull-request comment. It starts with
// PRCommentMarker and is meant to be printed, never posted by gh-optivem.
func RenderPRComment(rep Report) string {
	var b strings.Builder
	b.WriteString(PRCommentMarker + "\n")
	b.WriteString("### Spec coverage\n\n")
	fmt.Fprintf(&b, "%s\n\n%s\n\n", coverageLine(rep), mdVerdictLine(rep))
	writeMDCounts(&b, rep)
	var gaps []string
	for _, uc := range rep.UseCases {
		for _, c := range uc.Criteria {
			if c.State == StateVerified {
				continue
			}
			line := fmt.Sprintf("- `%s` %s: %s", c.ID, c.State, mdText(c.Text))
			if notes := criterionNotes(c); len(notes) > 0 {
				line += " _(" + mdText(strings.Join(notes, "; ")) + ")_"
			}
			gaps = append(gaps, line)
		}
	}
	if len(gaps) > 0 {
		b.WriteString("\n<details><summary>Criteria not yet verified</summary>\n\n")
		b.WriteString(strings.Join(gaps, "\n") + "\n\n</details>\n")
	}
	writeMDProblems(&b, rep)
	return b.String()
}

func stateClass(st State) string {
	return "s-" + strings.ReplaceAll(strings.ToLower(string(st)), " ", "-")
}

const htmlCSS = `
:root{color-scheme:light dark;--bg:#FAFAF8;--card:#FFFFFF;--line:#E3E3DE;--fg:#1B1F23;--muted:#5B6470;--soft:#3A424C;--ok:#1E6B3F;--bad:#B3261E}
@media (prefers-color-scheme:dark){:root{--bg:#14171A;--card:#1D2125;--line:#2E343A;--fg:#E8EAEC;--muted:#9AA3AD;--soft:#C3C9CF;--ok:#58C285;--bad:#FF8A80}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font:14px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
code,.mono{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px}
header{border-bottom:1px solid var(--line);background:var(--card)}
.wrap{max-width:1120px;margin:0 auto;padding:16px 24px}
main.wrap{padding-top:28px;padding-bottom:48px;display:flex;flex-direction:column;gap:24px}
h1{margin:0;font-size:26px;letter-spacing:-0.02em}
h2{margin:0;font-size:16px}
.muted{color:var(--muted)}
.banner{border-radius:10px;padding:14px 18px;border:1px solid var(--line);background:var(--card);border-left:6px solid var(--ok)}
.banner.fail{border-left-color:var(--bad)}
.banner.none{border-left-color:var(--muted)}
.banner h2{font-size:18px}
.banner ul{margin:8px 0 0;padding-left:20px}
.tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(150px,1fr));gap:16px}
.card{background:var(--card);border:1px solid var(--line);border-radius:10px;padding:16px 18px}
.tile .v{font-size:32px;font-weight:700;line-height:1.15;display:block}
.tile .l{font-size:13px;color:var(--muted);font-weight:500}
.bar{display:flex;gap:3px;height:22px;margin:12px 0}
.bar div{border-radius:4px;min-width:3px}
.legend{display:flex;flex-wrap:wrap;gap:18px;font-size:13px;color:var(--soft)}
.dot{display:inline-block;width:10px;height:10px;border-radius:2px;margin-right:6px}
.life{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:14px;margin-top:12px}
.life .item{border:1px solid var(--line);border-radius:10px;padding:12px 14px;background:var(--bg)}
.pill{display:inline-block;color:#fff;border-radius:999px;padding:1px 10px;font-size:12px;font-weight:600;white-space:nowrap}
.tbl{overflow-x:auto}
table{border-collapse:collapse;width:100%}
th,td{text-align:left;padding:8px 10px;border-bottom:1px solid var(--line);vertical-align:top}
th{font-size:12px;color:var(--muted);text-transform:uppercase;letter-spacing:.04em}
.note{font-size:12px;color:var(--muted)}
section.uc{display:flex;flex-direction:column;gap:8px}
.uchead{display:flex;flex-wrap:wrap;gap:10px;align-items:center}
@media (max-width:600px){.wrap{padding-left:16px;padding-right:16px}}
`

// RenderHTML renders the self-contained static HTML report for product owners.
// generated is the timestamp shown in the header; the output is otherwise
// deterministic.
func RenderHTML(rep Report, generated time.Time) string {
	esc := html.EscapeString
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n")
	w("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	w("<title>Spec coverage report</title>\n<style>%s</style>\n</head>\n<body>\n", htmlCSS)
	w("<header><div class=\"wrap\"><strong>gh optivem spec</strong> <span class=\"muted\">Report generated %s</span></div></header>\n",
		esc(generated.UTC().Format("2006-01-02 15:04 UTC")))
	w("<main class=\"wrap\">\n<h1>Spec coverage report</h1>\n")

	switch {
	case rep.Gate == nil:
		w("<section class=\"banner none\" role=\"status\"><h2>Gate not evaluated</h2><p class=\"muted\">Mode %s, threshold %.1f%%.</p></section>\n", esc(rep.Mode), rep.Threshold)
	case rep.Gate.Passed:
		w("<section class=\"banner\" role=\"status\"><h2>Gate passed</h2><p class=\"muted\">Mode %s, threshold %.1f%%.</p></section>\n", esc(rep.Mode), rep.Threshold)
	default:
		w("<section class=\"banner fail\" role=\"status\"><h2>Gate failed</h2><p class=\"muted\">Mode %s, threshold %.1f%%.</p><ul>", esc(rep.Mode), rep.Threshold)
		for _, r := range rep.Gate.Reasons {
			w("<li>%s</li>", esc(r))
		}
		w("</ul></section>\n")
	}

	s := rep.Summary
	w("<section class=\"tiles\" aria-label=\"Summary\">\n")
	tile := func(label, value string) {
		w("<div class=\"card tile\"><span class=\"l\">%s</span><span class=\"v\">%s</span></div>\n", esc(label), esc(value))
	}
	tile("Spec coverage", fmt.Sprintf("%.1f%%", s.CoveragePercent))
	tile("Criteria verified", fmt.Sprintf("%d of %d", s.Verified, s.Total))
	tile("Use cases verified", fmt.Sprintf("%d of %d", useCasesVerified(rep), len(rep.UseCases)))
	for _, st := range allStates[1:] {
		tile(string(st), fmt.Sprint(stateCount(s, st)))
	}
	w("</section>\n")

	w("<section class=\"card\">\n<h2>%s</h2>\n", esc(coverageLine(rep)))
	w("<div class=\"bar\" role=\"img\" aria-label=\"%d verified, %d in progress, %d failing, %d planned, %d waived criteria\">",
		s.Verified, s.InProgress, s.Failing, s.Planned, s.Waived)
	for _, st := range allStates {
		if n := stateCount(s, st); n > 0 {
			w("<div style=\"flex:%d 1 0;background:%s\" title=\"%s (%d)\"></div>", n, stateColors[st], esc(string(st)), n)
		}
	}
	w("</div>\n<div class=\"legend\">")
	for _, st := range allStates {
		w("<span><span class=\"dot\" style=\"background:%s\"></span>%s (%d)</span>", stateColors[st], esc(string(st)), stateCount(s, st))
	}
	w("</div>\n<p class=\"note\">Threshold %.1f%% applies in strict mode.</p>\n</section>\n", rep.Threshold)

	w("<section class=\"card\">\n<h2>Criterion lifecycle</h2>\n")
	w("<p class=\"muted\">Each acceptance criterion moves through these states as its acceptance test is added and the feature is built. A use case is Verified when all its criteria are.</p>\n<div class=\"life\">\n")
	for _, st := range allStates {
		w("<div class=\"item\"><span class=\"pill\" style=\"background:%s\">%s</span> <strong>%d</strong><br><span class=\"note\">%s</span></div>\n",
			stateColors[st], esc(string(st)), stateCount(s, st), esc(stateRules[st]))
	}
	w("</div>\n</section>\n")

	for _, uc := range rep.UseCases {
		v := 0
		for _, c := range uc.Criteria {
			if c.State == StateVerified {
				v++
			}
		}
		w("<section class=\"card uc\">\n<div class=\"uchead\"><h2><span class=\"mono\">%s</span> %s</h2><span class=\"pill\" style=\"background:%s\">%s</span><span class=\"note\">%d of %d criteria verified</span></div>\n",
			esc(uc.ID), esc(uc.Title), stateColors[uc.State], esc(string(uc.State)), v, len(uc.Criteria))
		w("<div class=\"tbl\"><table>\n<thead><tr><th>Criterion</th><th>State</th><th>Description</th><th>Tests</th></tr></thead>\n<tbody>\n")
		for _, c := range uc.Criteria {
			desc := esc(c.Text)
			if c.Group != "" {
				desc = "<span class=\"note\">" + esc(c.Group) + "</span><br>" + desc
			}
			if notes := criterionNotes(c); len(notes) > 0 {
				desc += "<br><span class=\"note\">" + esc(strings.Join(notes, "; ")) + "</span>"
			}
			w("<tr class=\"%s\"><td class=\"mono\">%s</td><td><span class=\"pill\" style=\"background:%s\">%s</span></td><td>%s</td><td class=\"mono\">%s</td></tr>\n",
				stateClass(c.State), esc(c.ID), stateColors[c.State], esc(string(c.State)), desc, esc(testNames(c)))
		}
		w("</tbody></table></div>\n</section>\n")
	}

	list := func(title string, n int, item func(int) string) {
		if n == 0 {
			return
		}
		w("<section class=\"card\">\n<h2>%s</h2>\n<ul>", esc(title))
		for i := 0; i < n; i++ {
			w("<li>%s</li>", item(i))
		}
		w("</ul>\n</section>\n")
	}
	list("Orphan tests (not linked to any criterion)", len(rep.Orphans), func(i int) string {
		return "<code>" + esc(rep.Orphans[i].Name) + "</code>: " + esc(rep.Orphans[i].Reason)
	})
	list("Tests referencing unknown criterion IDs", len(rep.UnknownIDs), func(i int) string {
		return "<code>" + esc(rep.UnknownIDs[i].ID) + "</code> in <code>" + esc(rep.UnknownIDs[i].Test) + "</code>"
	})
	list("Spec validation errors", len(rep.Validation.Errors), func(i int) string {
		return "<code>" + esc(rep.Validation.Errors[i].Code) + "</code>: " + esc(rep.Validation.Errors[i].Message)
	})

	w("</main>\n</body>\n</html>\n")
	return b.String()
}
