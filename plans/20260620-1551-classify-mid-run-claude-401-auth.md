# 2026-06-20 15:51:00 UTC — Classify mid-run Claude 401 as an auth failure

## TL;DR

**Why:** Rehearsal #65 crashed mid-run when the `claude` CLI hit a 401 ("Failed to authenticate. API Error: 401 Invalid authentication credentials"), but `clauderun`'s `authSignatures` set doesn't match that wording — so the failure fell through to a bare `exited non-zero: exit status 1` instead of the actionable "run `claude /login`" message the classifier promises. The operator had to hand-decode it.
**End result:** A mid-run Claude 401 is correctly classified as auth: the trace surfaces "claude CLI is not authenticated — run `claude /login` …" and fast-fails without a pointless retry. The classifier's documented "covers credentials expiring mid-run" claim becomes true.

## Outcomes

What we get out of this:

- A mid-run Claude 401 surfaces the actionable `run claude /login` message in the rehearsal trace, not a generic `exit status 1` the operator must decode by hand.
- The exact stderr line observed in #65 — `Failed to authenticate. API Error: 401 Invalid authentication credentials` — is matched by `authSignatures`, closing the gap between the comment at `clauderun.go:1437–1439` and the code.
- The 401 continues to fast-fail with no retry (correct — an invalid/expired token won't recover on retry), now via an explicit auth classification rather than by accidentally matching no signature.
- A unit test pins the observed stderr line to the auth verdict, so the precedent (`api error: 500`/`529` added from rehearsal-71/72 logs) extends to this observed 401.

## ▶ Next executable step (resume here)

Edit `internal/atdd/process/clauderun/clauderun.go` — extend the `authSignatures` slice (line ~1440) with the case-insensitive substrings the CLI emits on a 401: `"invalid authentication credentials"`, `"failed to authenticate"`, and `"api error: 401"`. These flow automatically into both `classifyRunError` (actionable message) and `hardFailStderrRegex` (`clauderun.go:1408`, fast-fail) — no other logic changes. Then add a `clauderun_test.go` case asserting the literal `#65` stderr line classifies as auth. Gate: `go test` the `clauderun` package (single package — safe on Windows). Unblocks: future rehearsals self-document a mid-run 401.

## Steps

- [ ] Step 1: In `internal/atdd/process/clauderun/clauderun.go`, add `"invalid authentication credentials"`, `"failed to authenticate"`, and `"api error: 401"` to the `authSignatures` slice (~line 1440), keeping them lowercase literals consistent with the existing entries (matched case-insensitively, regexp-quoted via `compileSignatureRegex`).
- [ ] Step 2: Add/extend a `classifyRunError` unit test in `clauderun_test.go` asserting the literal observed line `Failed to authenticate. API Error: 401 Invalid authentication credentials` returns the auth ("run `claude /login`") message; add a `hardFailStderrRegex` match assertion for the same line. (Coverage shape is executor's discretion.)
- [ ] Step 3: Run `go test` scoped to the `internal/atdd/process/clauderun` package only (never unbounded `go test ./...` on Windows) and confirm green.

## Out of scope (do not include)

- Rehearsal resume-from-commit (`scripts/atdd-rehearsal*.sh`) — deselected; tracked separately if ever wanted.
- Pre-dispatch auth re-check in `clauderun` — deselected.
- Any BPMN / `process-flow.yaml` change — the flow is correct; the 401 is environmental.
