# 2026-06-19 20:46:00 UTC — Fix no-progress guard: normalize the MM:SS.mmm suite-duration token

## TL;DR

**Why:** The verify-tests-pass fix loop's no-progress guard never fires when it should. `fixFailureSignature` (`internal/atdd/process/actions/fix_progress.go`) fails to strip the runner's `MM:SS.mmm` Suite-Results duration column (e.g. `FAILED 00:02.158`), so two byte-identical failures get different signatures and the guard always reports "progressing". In #65 this let an unwinnable loop run to the `max-visits:2` count cap instead of halting on attempt 1.
**End result:** A fix pass that changes nothing produces an equal signature on the next run, so the loop halts early via `FIX_LOOP_NO_PROGRESS_EXHAUSTED` — saving one opus·high pass on *every* stuck/never-green fix loop, not just the #65 contract case.

## Outcomes

What we get out of this — the goals and deliverables:

- `fixFailureSignature` treats two failing runs that differ **only** in the `FAILED 00:02.xxx` suite-duration column as identical (equal signature).
- The no-progress guard halts a spinning/never-green loop on the **first** re-run after a no-op fix pass (`FIX_LOOP_NO_PROGRESS_EXHAUSTED`), rather than deferring to the `max-visits:2` count cap (`FIX_LOOP_EXHAUSTED`).
- The conservative, one-sided error budget is preserved: genuinely-different failures still produce different signatures (no false "equal" that would wrongly halt a progressing loop).
- Regression coverage in `fix_progress_test.go` locks both directions in.
- Plan `20260619-1934-contract-test-system-driver-escape.md` can be deleted once this ships (this is its "3b" follow-up).

## ▶ Next executable step (resume here)

**First: the user must resolve the three Open questions (Q1–Q3) — `/execute-plan` should stop and ask, not auto-apply the recommendations.** Once decided, proceed:

Edit `internal/atdd/process/actions/fix_progress.go`: add a normalization pass to `fixFailureSignature` that replaces the `MM:SS(.mmm)?` suite-duration token with `<dur>`, applied **after** the existing `clockToken` (`HH:MM:SS`) pass so it can't clobber a full clock. Add a package var regex (e.g. `suiteDurationToken = regexp.MustCompile(\`\b\d{1,2}:\d{2}\.\d+\b\`)`) and apply it alongside `clockToken`/`durationToken`. Then add `fix_progress_test.go` cases: (a) two outputs differing only in `FAILED 00:02.xxx` → equal signature; (b) two genuinely-different failures → still differ. Validate with `go test ./internal/atdd/process/actions/`. Gate: stop at the review/commit gate before committing.

## Steps

- [ ] Step 1: In `internal/atdd/process/actions/fix_progress.go`, add a `suiteDurationToken` regex var for the `MM:SS(.mmm)?` format and apply it in `fixFailureSignature` after `clockToken`, normalizing to `<dur>`. Keep it conservative — anchor to the volatile duration shape, not arbitrary `\d:\d\d` text. (See Open question Q1 on bare `MM:SS` vs requiring the `.mmm` fraction.)
- [ ] Step 2: Add tests in `internal/atdd/process/actions/fix_progress_test.go`: (a) identical-failure-except-suite-duration → equal signature; (b) different-failure → different signature; reuse a realistic Suite-Results block (`latest - Contract (real)  FAILED  00:02.158`) from #65.
- [ ] Step 3: Run `go build ./...` and `go test ./internal/atdd/...`; confirm green (existing `fix_progress_test.go` + new cases).
- [ ] Step 4: Cross-language check — confirm whether the Java and .NET runners emit the same `MM:SS.mmm` Suite-Results duration format (TypeScript/Playwright is confirmed). If they use a different volatile duration shape, extend the regex to cover it so the guard works on every stack. (See Open question Q2.)
- [ ] Step 5: After this ships, delete `plans/20260619-1934-contract-test-system-driver-escape.md` (its only remaining content is this follow-up).

## Open questions

> **These are decisions for the user to make before execution.** Each carries my recommendation, but `/execute-plan` must stop and ask — do **not** auto-apply the recommendation.

- **Q1 (regex scope) — needs user decision:** Match only `MM:SS.mmm` (require the fractional part, `\b\d{1,2}:\d{2}\.\d+\b`), or also bare `MM:SS` (`\b\d{1,2}:\d{2}\b`)? Recommendation: **require the fraction** (`MM:SS.mmm`) — it's the observed Suite-Results format and is unambiguously a duration; a bare `\d{1,2}:\d{2}` risks eating a non-volatile `M:SS`-shaped substring elsewhere in output (false-equal — the one error the design forbids). Revisit only if a stack prints bare `MM:SS` durations.
- **Q2 (cross-language coverage) — needs user decision:** Is Step 4's verification in scope for this plan, or deferred? The #65 evidence is TypeScript-only. Recommendation: keep Step 4 in scope but treat a divergent Java/.NET format as a follow-up row rather than a blocker — the TS fix is correct and independently valuable.
- **Q3 (audit depth) — needs user decision:** Worth a `bpmn-logic-audit` pass to confirm nothing else consumes the un-normalized signature? `fix-prev-failure-signature` is written/read only by `checkFixProgress` and cleared by run-command on green — so likely no other consumer. Recommendation: a quick grep for `fix-prev-failure-signature` / `fixFailureSignature` suffices; skip the full audit unless the grep surprises.
