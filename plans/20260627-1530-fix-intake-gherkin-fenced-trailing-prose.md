# 2026-06-27 15:30:12 UTC — Fix intake Gherkin validator crash on fenced AC/ESCC with trailing prose

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-27T17:31:53Z`

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

**(shop repo — user-owned content; confirm wording before saving.)** Correct ticket `optivem/shop#76`: edit the issue body (`gh issue edit 76 --repo optivem/shop`) to remove the implementation-leaking paragraph from **Acceptance Criteria** ("This scenario is **red** … the fix extends the blackout end from 22:30 to 23:00, turning it green") — drop it entirely, or relocate a behavior-only sentence to **Description**, leaving Acceptance Criteria as the Gherkin block alone. Then re-run the #76 rehearsal; it should pass `PARSE_TICKET`. The gh-optivem fix (Steps 1–4) is already committed and is what makes the gate fail *loud* rather than crash; Step 5 is what makes #76 itself green.

## Steps

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
