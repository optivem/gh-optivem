# 2026-06-27 15:30:12 UTC — Fix intake Gherkin validator crash on fenced AC/ESCC with trailing prose

## TL;DR

**Why:** ATDD rehearsal #76 ("Bug: Order cancellation blackout on Dec 31") crashed at `PARSE_TICKET` with `Acceptance Criteria: Parser errors: (14:0): unexpected end of file, expected: #DocStringSeparator`. Two distinct defects: (1) the ticket is **mis-authored** — its Acceptance Criteria is a ` ```gherkin ` block followed by a paragraph that leaks the *implementation* ("the fix extends the blackout end from 22:30 to 23:00, turning it green"), which is test/solution commentary that has no place in behavior-only AC; (2) gh-optivem's intake gate reacts to that stray content with a **cryptic crash** instead of an actionable error — `stripEnclosingCodeFence` only strips a fence that *wholly* encloses the body, so the trailing prose makes it no-op and the raw `` ``` `` fences reach the cucumber parser and are mis-read as DocString separators.
**End result:** The intake Gherkin gate **fails loud with an actionable message** when an AC/ESCC section has non-blank content after its closing fence (or an unterminated fence) — telling the author to move notes to Description — instead of crashing cryptically or silently swallowing the prose; and shop issue #76's Acceptance Criteria is corrected to contain only the Gherkin block, so #76 proceeds past `PARSE_TICKET`.

## Outcomes

What we get out of this — the goals and deliverables:

- gh-optivem **rejects** an AC/ESCC section that has non-blank content after its closing code fence, with a clear, author-relative error that names the offending line and says to move notes to Description — replacing today's cryptic `unexpected end of file, expected: #DocStringSeparator` crash. (Aligns with the repo's fail-loud-with-actionable-message rule; a teaching repo should reject implementation-leaking AC, not normalize it by stripping.)
- An **unterminated** fence (opener with no matching closer) is likewise rejected with a clear message instead of a cryptic parser crash.
- A genuine error inside a wholly-enclosed fenced block (e.g. a typo'd step keyword) still fails fast with an **author-relative** line number — error-location mapping is preserved.
- All existing intake-gherkin behaviors stay green: **wholly-enclosed fence** (closer is the last non-blank line), bare scenarios, leading `Feature:`, `@isolated` tag/comment, `Rule:` nesting.
- Shop issue **#76's Acceptance Criteria** contains only the Gherkin block (the implementation-leaking paragraph removed, or a behavior-only sentence relocated to Description), so the ticket parses through `PARSE_TICKET`.
- `go test ./internal/atdd/runtime/intake -p 2` passes.

## ▶ Next executable step (resume here)

Edit `internal/atdd/runtime/intake/gherkin.go`: rework `stripEnclosingCodeFence` (lines ~132-159) so that when the body's first non-blank line is a fence opener (`codeFenceRE`) it scans forward for the matching closer (`isClosingFence`) and then **distinguishes three cases**: (a) no closer found → return an error "unterminated code fence"; (b) closer found but a non-blank line exists after it → return an error naming that line and saying to move notes to Description; (c) closer is the last non-blank line → return inner content + leading-line count, no error. Give it an `error` return and thread it through `acceptanceGherkinSource` → `validateAcceptanceCriteriaGherkin` and through `esccGherkinSource` (already returns `error`). Then add the regression tests in `gherkin_test.go` and run `go test ./internal/atdd/runtime/intake -p 2`.

## Steps

- [ ] Step 1: In `internal/atdd/runtime/intake/gherkin.go`, change the de-fence helper (`stripEnclosingCodeFence`) to **reject stray content** rather than strip it. When the first non-blank line is a fence opener (`codeFenceRE`), scan forward for the first `isClosingFence` line and branch:
  - **no closer** → return an error: `<section>: line N: unterminated code fence` (N = opener line, section-body-relative, 1-based).
  - **closer found, non-blank content after it** → return an error: `<section>: line M: content after the closing code fence — the Acceptance Criteria / External System Contract Criteria section must contain only the Gherkin block; move notes to Description` (M = first non-blank line after the closer).
  - **closer is the last non-blank line** → return `(inner, first+1, nil)` as today (wholly-enclosed happy path).
  - first non-blank line is **not** a fence opener (bare-scenario AC), or body is empty → return `(body, 0, nil)` unchanged.
  The helper gains an `error` return; the section-name prefix is passed in (or the callers wrap the error) so messages name AC vs ESCC. Leave `codeFenceRE` and `isClosingFence` unchanged.
- [ ] Step 2: Thread the error out. `acceptanceGherkinSource` propagates the helper's error (becomes `(string, int, int, error)` or de-fences in `validateAcceptanceCriteriaGherkin` directly); `validateAcceptanceCriteriaGherkin` returns it. `esccGherkinSource` already returns `error` — return the helper's error from there. Update the helper's doc comment to describe the reject-not-strip behavior (drop "wholly encloses … last non-blank line is a matching closer"); optionally rename (e.g. `defenceGherkinBody` / `stripLeadingCodeFence`).
- [ ] Step 3: Add regression tests in `internal/atdd/runtime/intake/gherkin_test.go`:
  - (a) fenced ` ```gherkin ` AC **followed by a trailing explanatory paragraph** → **rejected** with an error that names the offending line and the section (and ideally mentions "after the closing code fence").
  - (b) fenced ` ```gherkin ` AC with **no** trailing prose (closer is last non-blank) → passes (wholly-enclosed regression).
  - (c) fenced AC with an **unterminated** fence (opener, no closer) → rejected with the unterminated-fence error.
  - (d) fenced ESCC (`External System:` + register sub-headers) **followed by trailing prose** → rejected.
  - Keep existing tests green: wholly-enclosed fence, bare scenarios, leading `Feature:`, `@isolated` tag/comment, `Rule:` nesting, and the typo'd-step author-relative-line test.
- [ ] Step 4: Run `go test ./internal/atdd/runtime/intake -p 2` and confirm all tests (new + existing) pass.
- [ ] Step 5: **(shop repo — separate from gh-optivem)** Correct ticket `optivem/shop#76`: edit the issue body (`gh issue edit 76 --repo optivem/shop`) to remove the implementation-leaking paragraph from **Acceptance Criteria** ("This scenario is **red** … the fix extends the blackout end from 22:30 to 23:00, turning it green"). Drop it entirely, or relocate a behavior-only sentence to **Description** — leave Acceptance Criteria as the Gherkin block alone. This is user-owned content; confirm wording before saving.

## Notes / scope

- **Both paths fixed by one helper:** `acceptanceGherkinSource` and `esccGherkinSource` both call the de-fence helper, so AC and ESCC are covered. The leading-line count is unchanged in meaning, so the existing `lineMap` offset correction keeps error locations author-relative.
- **Why reject, not strip:** silently stripping the trailing prose would let implementation-leaking AC pass unnoticed — wrong for a teaching repo. Failing loud with an actionable message teaches the author to keep AC behavior-only and matches the repo's fail-loud-with-actionable-message rule. The ticket fix (Step 5) is what actually makes #76 green; gh-optivem's job is only to fail *clearly*.
- **Deliberate non-goal:** prose *before* the fence (opener is not the first non-blank line) is not handled. Both #72 and #76 put the fence first; handling leading prose is out of scope.
- **Single language:** gh-optivem Go-only. The intake Gherkin gate has no Java/.NET/TypeScript twin — no parallel implementation to mirror.
- **Windows test hazard:** never run unbounded `go test ./...`; scope to the package with `-p 2`.

## Verification

- After Steps 1–4: `go test ./internal/atdd/runtime/intake -p 2` green.
- After Step 5: re-run the #76 rehearsal — it should pass `PARSE_TICKET` and proceed.
- Operator: grep other open `optivem/shop` issues for the same "prose after a ` ``` ` fence in Acceptance Criteria" pattern so they're corrected before they hit a rehearsal (each will now fail loud rather than crash cryptically).
