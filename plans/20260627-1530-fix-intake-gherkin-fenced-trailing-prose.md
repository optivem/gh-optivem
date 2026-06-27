# 2026-06-27 15:30:12 UTC — Fix intake Gherkin validator crash on fenced AC/ESCC with trailing prose

## TL;DR

**Why:** ATDD rehearsal #76 ("Bug: Order cancellation blackout on Dec 31") crashed at `PARSE_TICKET` with `Acceptance Criteria: Parser errors: (14:0): unexpected end of file, expected: #DocStringSeparator`. The AC is a ` ```gherkin ` fenced block **followed by an explanatory paragraph**; `stripEnclosingCodeFence` only strips a fence that *wholly* encloses the body, so the trailing prose makes it no-op and the raw `` ``` `` fences reach the cucumber parser and are mis-read as DocString separators.
**End result:** The intake Gherkin syntax gate strips a leading fenced code block regardless of any prose that follows the closing fence, so a fenced AC/ESCC with a trailing note parses cleanly; the crash class fixed in `5907b564` is closed for the surrounding-prose case too.

## Outcomes

What we get out of this — the goals and deliverables:

- Shop issue #76's ticket body parses through `PARSE_TICKET` without crashing (reproduced locally; the fix was prototyped and confirmed to extract the inner Gherkin and validate clean).
- A fenced ` ```gherkin ` AC or ESCC block followed by an explanatory paragraph passes the intake syntax gate.
- A genuine error inside such a fenced block (e.g. a typo'd step keyword) still fails fast with an **author-relative** line number — error-location mapping is preserved.
- All existing intake-gherkin behaviors stay green: wholly-enclosed fence, bare scenarios, leading `Feature:`, `@isolated` tag/comment, `Rule:` nesting.
- `go test ./internal/atdd/runtime/intake -p 2` passes.

## ▶ Next executable step (resume here)

Edit `internal/atdd/runtime/intake/gherkin.go`: generalize `stripEnclosingCodeFence` (lines ~132-159) so that when the body's first non-blank line is a fence opener (`codeFenceRE`), it scans forward for the **next** matching closing fence (`isClosingFence`) and returns the content between opener and closer plus the leading-line count (`openerIndex+1`) — discarding any prose after the closer — falling back to `(body, 0)` when there is no opener or no matching closer. Update the doc comment (drop "wholly encloses … last non-blank line is a matching closer"). Then add the three regression tests in `gherkin_test.go` and run `go test ./internal/atdd/runtime/intake -p 2`.

## Steps

- [ ] Step 1: In `internal/atdd/runtime/intake/gherkin.go`, change `stripEnclosingCodeFence` so the closer is located by scanning forward from the opener for the first `isClosingFence` line (instead of requiring the *last* non-blank line to be the closer). Return `strings.Join(lines[first+1:closerIdx], "\n")` and `first+1`. Keep the fallback `(body, 0)` for: no non-blank line, first non-blank line is not a fence opener, or no matching closer found after the opener. Leave `codeFenceRE` and `isClosingFence` unchanged.
- [ ] Step 2: Update the `stripEnclosingCodeFence` doc comment to describe the new behavior (strips a leading fenced block up to its matching closer, discarding trailing prose) and drop the "wholly encloses … AND its last non-blank line is a matching closer" wording. Optional: rename to reflect the behavior (e.g. `stripLeadingCodeFence` / `extractFencedBlock`) and update both call sites (`acceptanceGherkinSource`, `esccGherkinSource`).
- [ ] Step 3: Add regression tests in `internal/atdd/runtime/intake/gherkin_test.go`:
  - (a) fenced ` ```gherkin ` AC followed by a trailing explanatory paragraph → passes.
  - (b) a typo'd step keyword inside a fenced-with-trailing-prose AC → fails with an author-relative line number quoting the offending line.
  - (c) fenced ESCC (`External System:` + register sub-headers) followed by trailing prose → passes.
- [ ] Step 4: Run `go test ./internal/atdd/runtime/intake -p 2` and confirm all tests (new + existing) pass.

## Notes / scope

- **Single function, both paths fixed:** `acceptanceGherkinSource` and `esccGherkinSource` both call `stripEnclosingCodeFence`, so AC and ESCC are fixed by the one change. The returned leading-line count is unchanged in meaning, so the existing `lineMap` offset correction keeps error locations author-relative — no change needed in `validateAcceptanceCriteriaGherkin` / `validateESCCGherkin`.
- **Deliberate non-goal:** prose *before* the fence (opener is not the first non-blank line) is not handled. Both #72 and #76 put the fence first; handling leading prose is out of scope for this fix.
- **Single language:** gh-optivem Go-only. The intake Gherkin gate has no Java/.NET/TypeScript twin, so there is no parallel implementation to mirror.
- **Windows test hazard:** never run unbounded `go test ./...`; scope to the package with `-p 2` as above.
