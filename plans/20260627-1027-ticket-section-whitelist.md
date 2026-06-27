# 2026-06-27 10:27:00 UTC — Enforce the canonical-section whitelist at ticket intake

## TL;DR

**Why:** Today `internal/atdd/runtime/intake` extracts the 5 canonical sections and
**silently ignores everything else** — an unknown heading, a renamed section, or
prose dumped before the first heading all pass without complaint and are dropped.
A ticket can drift arbitrarily far from the Issue-Form shape and still parse.
**End result:** `ParseSections` enforces an explicit whitelist: a ticket body may
contain **only** the canonical sections, each in its required format, with no
unknown headings and no stray content outside an allowed section body. Anything
else fails fast as `STOP_PARSE_ERROR` with an actionable, line-anchored message.

## Outcomes

What we get out of this — observable results:

- A ticket with a heading that isn't one of the canonical 5 (`Description`,
  `Acceptance Criteria`, `Steps to Reproduce`, `Checklist`,
  `External System Contract Criteria`) fails at intake naming the offending heading.
- A ticket with non-empty content **outside** any allowed section body — preamble
  before the first heading, or text under no recognized heading — fails at intake
  naming where the stray content starts.
- The **Checklist** section must be a bulleted or numbered list; a Checklist whose
  body has non-list content fails at intake.
- Acceptance Criteria and External System Contract Criteria still required to be
  well-formed Gherkin — but that check is owned by the sibling plan
  `20260627-1207-gherkin-validation-ac-escc.md`, not re-implemented here.
- All existing valid tickets (Issue-Form-generated bodies) continue to parse
  unchanged — the whitelist is exactly the set the forms already produce.

## Design constraint — whitelist is the UNION, not per-kind

`PARSE_TICKET` runs **before** `GATE_TICKET_KIND`, so the parser cannot know the
ticket kind/subtype and must stay kind-agnostic. The section whitelist this plan
adds is therefore the **union across all kinds — exactly the 5 canonical
sections**; it must NOT be made per-kind. Per-kind "which section is *required*"
stays where it lives today (the load-bearing `${acceptance-criteria}` /
`${checklist}` placeholder check in `clauderun.go`, plus the AC-XOR-Checklist
mutual-exclusion rule already in `ParseSections`). For reference, the per-kind
usage this plan does NOT re-implement:

- **story** → Acceptance Criteria (ESCC optional); **bug** → Acceptance Criteria +
  Steps to Reproduce; **task/legacy-coverage** → Acceptance Criteria.
- **task/{system-redesign, external-system-redesign, system-refactor,
  test-refactor}** → Checklist.
- **Description** optional on every kind. AC and Checklist are mutually exclusive.

## ▶ Next executable step (resume here)

Design/encoding is not finished — resolve the Open Questions below first (they
change the validator's behavior), then this becomes mechanical. Once settled, the
first executable unit is: **add a whitelist + stray-content check to
`ParseSections` in `internal/atdd/runtime/intake/parse.go`** — walk the raw body's
headings, reject any heading whose text isn't in `CanonicalHeadings`, and reject
any non-empty line that falls outside a recognized section body. This needs a
small change to `ExtractSection`'s sibling logic (it currently only locates the
*wanted* heading; the whitelist needs to enumerate *all* headings + the gaps
between them). Stop at: `go build ./...` + intake tests green.

## Steps

- [ ] Step 1: **Enumerate-all-sections helper.** Today `ExtractSection` finds one
  named heading. Add a helper that walks the body once and returns every
  H2-or-deeper heading (text + line span) plus the line spans of content that sits
  outside any heading body (the gaps). Reuse `headingDepthAndText`. This is the
  data the whitelist + stray-content checks consume.

- [ ] Step 2: **Whitelist check.** In `ParseSections` (or a new
  `validateAllowedSections`), reject any enumerated heading whose text doesn't
  match one of `CanonicalHeadings` (case-insensitive, matching `ExtractSection`'s
  comparison). Error names the offending heading and lists the allowed set.

- [ ] Step 3: **Stray-content check.** Reject any line that falls outside every
  canonical section body — preamble before the first canonical heading, or lines
  under a disallowed/unknown heading — **except** blank/whitespace-only lines and
  HTML comments (`<!-- ... -->`, including multi-line ones), which are tolerated.
  Error names the line number where stray content begins. (A disallowed heading
  already fails in Step 2; this catches headingless prose.)

- [ ] Step 4: **Checklist-is-a-list check.** When `Checklist.Found`, assert every
  non-blank line of its body is a list item — **any** bulleted (`-`, `*`, `+`) or
  numbered (`1.`, `1)`) marker, with or without a `[ ]`/`[x]` checkbox. Allow
  continuation/indented sub-lines under an item. Error names the first non-list
  line. NOTE the downstream consequence: `parseChecklistLine`/`checklistLineRE`
  today only recognize `- [ ]` **checkbox** items, so a plain-bullet Checklist now
  passes the format gate but yields zero `ChecklistResult.Items`
  (`CheckedCount() == 0`). The raw body still flows to the agent via
  `${checklist}`, so the structural cycles are unaffected; but if any consumer
  relies on `Items`/`CheckedCount` for progress, broaden `checklistLineRE` to make
  the checkbox optional so plain bullets parse into items too. Audit `Items`
  consumers in this step and decide.

- [ ] Step 5: **Wire into the existing error path.** These checks return errors
  the same way the AC-XOR-Checklist and Gherkin checks do, so they ride the
  existing `STOP_PARSE_ERROR` path out of `Parse`/`ParseSections`. Order them so
  the most structural failure (unknown section) reports before format failures.

- [ ] Step 6: **Tests** in `internal/atdd/runtime/intake/`. Cover: unknown heading
  fails; preamble-before-first-heading fails; Checklist with prose fails; a valid
  Issue-Form body (each canonical section, correct format) passes; an empty body
  and single-section bodies behave as expected; blank lines / trailing whitespace
  between sections are tolerated.

- [ ] Step 7: **Update the dumb-parser contract docs/comments** in `parse.go` and
  `sections.go`: the parser now enforces a closed section whitelist + no stray
  content, in addition to presence + shape. Keep it consistent with the wording the
  sibling Gherkin plan lands.

## Sequencing

Land the in-flight Gherkin plan `20260627-1207-gherkin-validation-ac-escc.md`
**first** (it already has uncommitted code in `parse.go` + `gherkin.go`), then
layer this whitelist check above it. Both touch `ParseSections` and both rewrite
the "parser stays dumb" doc comments, so executing this plan second avoids a
conflict and lets Step 7 here align with the wording the Gherkin plan lands.
