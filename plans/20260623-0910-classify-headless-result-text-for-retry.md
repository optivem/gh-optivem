# 2026-06-23 09:10:00 UTC — Retry transient API errors that surface in the headless stream-json result event

## TL;DR

**Why:** Rehearsal #69 hard-failed on the very first `system-implementer` request with `API Error: 529 Overloaded` (0 tokens, $0) — a textbook transient flake. The runner already has a 4-attempt backoff with `529`/`overloaded` in its transient signatures, but it never fired: in headless `claude -p --output-format stream-json` mode the API error lands on **stdout** (the terminal `result` event → `runResult.ResultText`), while the retry classifier only inspects the captured **stderr**, which was empty.

**End result:** The Dispatch retry classifier matches against the headless result-event text (and captured stdout tail) in addition to stderr, so a transient 5xx/overloaded on a headless dispatch is absorbed by the existing backoff instead of crashing the ticket on attempt 1. No new retry mechanism — only the classification haystack widens.

## Outcomes

What we get out of this — the goals and deliverables:

- A headless agent dispatch that hits a transient `529`/`5xx`/`overloaded` error reported in the stream-json `result` event is **retried** (up to the existing 4 attempts, 5s→15s→45s backoff) rather than hard-failing the rehearsal on the first attempt.
- The existing two-regex contract is preserved exactly: `hardFail` (rate-limit / auth) still wins over `transient`, and a real agent failure that matches neither regex is still **not** retried. Only the text being matched grows — the policy, backoff, and signature list are untouched.
- stderr remains part of the match string, so interactive-mode failures and CLI-level errors (which do print to stderr) keep retrying as they do today.
- A regression test pins the fix: a fake `ClaudeRunner` that returns a non-zero error with the 529 text **only** in `RunResult.ResultText` (empty stderr) drives more than one attempt, where today it fails after one.
- Scope held to exactly one file of production code (`internal/atdd/process/clauderun/clauderun.go`) plus its test — the single layer the user selected in triage. No BPMN, no agent-prompt, no rehearsal-wrapper changes.

## ▶ Next executable step (resume here)

In `internal/atdd/process/clauderun/clauderun.go`, edit the `Dispatch` `attempt` closure (≈ lines 667–703). On the failure branch (currently `return string(lastLines(lastStderr, 20)), runErr` at ≈ line 700), build the classification string from the **combination** of: the captured stderr tail (as today) **plus** `runResult.ResultText` **plus** the captured stdout tail. Hand that combined string to the existing `shell.RetryWithPolicy` call at ≈ line 705. Do not touch the regexes, the signature lists, the backoff, or the hardFail-wins ordering. Then add the regression test in Step 3 and run `go test ./internal/atdd/process/clauderun/...`.

## Steps

- [ ] **Step 1 — Capture the headless stdout for classification.** The `attempt` closure already tees stderr into a `cappedBuffer` (`stderrCapture`). The 529 lands on stdout, which `runHeadless` parses into `runResult.ResultText` and also accumulates in its internal `captured` buffer. `ResultText` is already returned to Dispatch, so it is available in the closure with no plumbing. Decide whether `ResultText` alone is sufficient (it carried the full "API Error: 529 Overloaded" in #69) or whether to also tee the runner stdout into a capped buffer for the cases where the error appears in an assistant/system event but the `result` event is empty. Default: use `ResultText` + a capped stdout tee for robustness, mirroring the existing stderr tee.
- [ ] **Step 2 — Widen the classification haystack.** On the failure branch of the `attempt` closure, compose the returned string as the concatenation of the stderr tail (`lastLines(lastStderr, 20)`), `runResult.ResultText`, and the stdout tail (if captured per Step 1). Keep the `lastLines`/cap discipline so the matched text stays bounded. Leave the `shell.RetryWithPolicy(transientStderrRegex, hardFailStderrRegex, "clauderun", attempt)` call (≈ line 705) and `classifyRunError` (≈ line 706) unchanged — they already read the right signals once the haystack includes the result text.
- [ ] **Step 3 — Regression test.** In `internal/atdd/process/clauderun/clauderun_test.go`, add a test using a fake `ClaudeRunner` that returns `(RunResult{ResultText: "API Error: 529 Overloaded."}, <non-nil error>)` with **empty stderr**. Assert the runner is invoked more than once (retry fired) using the `shell.SetSleepForTest` seam to no-op the backoff. Confirm the test fails against current `main` (one attempt) and passes after Steps 1–2. Add a sibling assertion that a hardFail signature (e.g. `rate limit`) appearing only in `ResultText` still fast-fails without retry, so the two-regex precedence is covered for the new haystack too.
- [ ] **Step 4 — Verify.** Run `go test ./internal/atdd/process/clauderun/...` and `go build ./...`. Confirm no behavior change for the existing stderr-based and generic-failure tests.

## Notes

- This is a **stream-wiring fix, not a new retry**. The 4-attempt policy, the 5s→15s→45s schedule, and the `transientSignatures` list (incl. `api error: 529`, `overloaded`) all already exist and are correct — the only defect is that the classifier read stderr while headless errors arrive on stdout. Frame any commit message and code comment accordingly.
- Aligns with the standing preference for native/consolidated retry over shell-loop retries — the rehearsal-wrapper "re-run whole ticket on rc=1" alternative was deliberately **not** chosen (it re-runs `acceptance-test-writer` and everything else, and would mask genuine runtime bugs).

## Open questions

- **Step 1 scope:** Is `runResult.ResultText` alone enough, or do we also tee+cap the runner stdout? Inference (not user-stated): include both for robustness, since an error could surface in an assistant/system event without a populated `result` event. Confirm or trim to `ResultText`-only to keep the diff minimal.
