# 2026-06-27 13:49:53 UTC — Strip enclosing code fence before intake Gherkin validation

## TL;DR

**Why:** The intake Gherkin *syntax* validator (added recently) feeds the raw extracted Acceptance Criteria / ESCC section body — including its surrounding markdown ` ```gherkin … ``` ` fence — to the cucumber parser, which reads the closing fence as an unclosed DocString and crashes PARSE_TICKET (`(22:0): unexpected end of file, expected: #DocStringSeparator`). Every fenced story ticket is affected (corpus tickets hand-fence; the story Issue Form's `render: markdown` auto-fences real submissions).
**End result:** The validator strips a single wholly-enclosing markdown code fence before parsing, so fenced AC/ESCC bodies validate as the Gherkin inside them. Error line numbers stay author-relative; unfenced bodies are unchanged; the raw fenced body still flows to `${acceptance-criteria}` for prompts.

## Outcomes

What we get out of this — the goals and deliverables:

- `gh optivem` PARSE_TICKET accepts an Acceptance Criteria wrapped in a ` ```gherkin `, ` ```markdown `, or bare ` ``` ` markdown code fence (and `~~~` fences), instead of crashing with `unexpected end of file, expected: #DocStringSeparator`.
- The same fix applies to a fenced External System Contract Criteria body.
- Gherkin syntax errors (e.g. a typo'd step keyword) inside a fenced body still fail fast at intake and report an author-relative line number.
- Unfenced AC/ESCC bodies behave exactly as before (no regression).
- The shop #72 rehearsal ("Charge shipping based on product weight from ERP") gets past PARSE_TICKET.

## ▶ Next executable step (resume here)

Step 1 — in `internal/atdd/runtime/intake/gherkin.go`, add `stripEnclosingCodeFence(body string) (string, int)` that, when the body's first non-blank line is a markdown fence opener (` ``` ` or `~~~`, with or without an info string like `gherkin`/`markdown`) **and** a matching closing fence is the last non-blank line, returns the inner content plus the count of leading lines removed; otherwise returns `(body, 0)`. No-op when the body isn't wholly fenced. This unblocks Steps 2–3 (wiring it into the two source builders + line maps).

## Steps

- [ ] Step 1: Add `stripEnclosingCodeFence(body string) (string, int)` helper in `internal/atdd/runtime/intake/gherkin.go`. Returns `(inner, leadingStripped)` when the body is wholly enclosed in one markdown fence (opener ` ``` `/`~~~` ± info string as first non-blank line, matching closer as last non-blank line); else `(body, 0)`. Strip only the fence lines, not inner content. Be conservative: require the closer to use the same fence char as the opener and only fire when the whole body is one block (don't strip a fence that wraps just part of the body).
- [ ] Step 2: In `acceptanceGherkinSource` (`gherkin.go:92`), call `stripEnclosingCodeFence` on `body` first, then run the existing `hasLeadingFeature` / `Feature: _` prepend logic on the de-fenced inner. Thread `leadingStripped` out to the caller.
- [ ] Step 3: In `validateAcceptanceCriteriaGherkin` (`gherkin.go:46`), fold the stripped-leading-line count into `lineMap`: `lineMap(n) = n - prepended + leadingStripped` so reported error locations stay relative to the author's fenced body.
- [ ] Step 4: In `esccGherkinSource` (`gherkin.go:132`), strip the enclosing fence before the per-line loop and offset the recorded origin line by the stripped count (`origLine = i + 1 + leadingStripped`) so `srcToOrig` stays author-relative. (ESCC is unfenced in current corpus tickets, so this is forward-looking but keeps the two paths symmetric.)
- [ ] Step 5: Confirm no de-fencing leaks into the body that reaches prompts — `ExtractSection`/`ParseSections` keep returning the raw (fenced) body for `${acceptance-criteria}`; only the syntax-validation source builders de-fence. Do not change `parse.go`'s data flow.
- [ ] Step 6: Add tests in `internal/atdd/runtime/intake/gherkin_test.go`:
  - fenced AC passes for ` ```gherkin `, ` ```markdown `, and bare ` ``` ` (Feature/Rule/Scenario body inside);
  - fenced AC with a typo'd step keyword still errors with a sensible author-relative line;
  - fenced ESCC body passes;
  - (regression) the existing unfenced AC/ESCC tests stay green.

## Verification

- [ ] `go test ./internal/atdd/runtime/intake/ -p 2` passes (Windows: never run unbounded `go test ./...`).
- [ ] Operator (not an agent step): re-run the #72 rehearsal loop and confirm PARSE_TICKET passes end-to-end.

## Notes

- Scope is Go-only: the intake parser has no Java/.NET/TS twin (those languages live in the shop *app*, not this orchestrator), so there is no parallel implementation to mirror this fix into.
- `story.yml`'s `render: markdown` on the acceptance-criteria textarea is what force-fences real form submissions; dropping it would stop the auto-fence but corpus tickets hand-fence anyway. The parser fix is the real solution — **leave the template as-is** (not a step in this plan).
