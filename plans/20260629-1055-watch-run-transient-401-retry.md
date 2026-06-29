# 2026-06-29 10:55:00 UTC — Harden `watchRunID` against transient 401 "Bad credentials" from `gh run watch`

## TL;DR

**Why:** The end-to-end system test failed because `gh run watch` on the prod-stage run hit a transient `HTTP 401: Bad credentials` — GitHub's per-token throttle returning 401 (not 429/403) under concurrent matrix load. `watchRunID` only recovers from rate-limit errors, so the transient 401 returned immediately and failed the whole stage even though the token was valid (it succeeded on commit/acceptance/QA stages minutes earlier in the same run).
**End result:** `watchRunID` retries `gh run watch` on a transient 401 "Bad credentials" with bounded attempts + jittered backoff (mirroring the proven `githubUserAuthCheck` mitigation), so a single throttle miss no longer fails the stage. All stage watches (commit/acceptance/QA/prod) are hardened by the one change.

## Outcomes

What we get out of this — the goals and deliverables:

- A transient `HTTP 401: Bad credentials` from `gh run watch` is retried (bounded, jittered backoff) instead of failing the stage on the first miss.
- The fix lives in `watchRunID`, so it covers every stage watch — acceptance, QA, prod (via `RunWatchWorkflow`) and commit-stage (via `RunWatchPushWorkflow`) — through one code path.
- The existing `*RateLimitExceeded` → `pollRunUntilComplete` fallback is preserved unchanged.
- The global `retryTransient`/`retryHardFail` classifier is left untouched, so `check-*` probes keep failing loud on 4xx (per CLAUDE.md convention).
- Unit coverage proving: (a) a transient 401 is retried then succeeds, (b) a genuine non-401/non-rate-limit error still fails fast.

## ▶ Next executable step (resume here)

Implement Step 1: in `internal/kernel/shell/github.go`, add a transient-401 detector + bounded jittered-backoff retry loop around the `gh run watch` call in `watchRunID` (lines ~499-514). Keep the `*RateLimitExceeded` → poll fallback. Then add the unit test (Step 2) and run the scoped test (Step 3). A fresh agent can act directly — the root cause and approach are fully pinned below.

## Steps

- [ ] Step 1: In `internal/kernel/shell/github.go`, harden `watchRunID` (currently lines ~499-514):
  - After `gh run watch` fails and the error is **not** `*RateLimitExceeded`, check whether the combined output matches a transient-401 signature — case-insensitive match on `bad credentials` (optionally also requiring `http 401`), mirroring how `Run` detects `"rate limit"` substrings at `github.go:59`. Define the matcher as a small package-level helper/regex (e.g. `transientBadCredentials`) so the test can rely on a stable contract.
  - On a transient-401 match, retry the `gh run watch` call with **bounded attempts** and **jittered backoff**, mirroring the documented outer 401-retry in `internal/config/token_auth.go:156-222` (`githubUserAuthCheck`). Reuse the existing `sleepFn` seam for the backoff sleep so the test can fake it via `withFakeSleep`. Reuse existing retry-count/delay constants where they fit (e.g. `defaultRetryAttempts`/`defaultRetryDelays`) rather than inventing new ones unless a watch-specific bound reads clearer.
  - If retries are exhausted, surface the original error (fail loud — do not coerce to success).
  - Leave the `*RateLimitExceeded` → `pollRunUntilComplete` branch exactly as-is.
  - Do **not** touch `internal/kernel/shell/retry.go` — the global 4xx hard-fail policy must stay (probe fail-loud). This is a narrow watch-scoped override, analogous to `MustRunPostCreatePush`. Add a short comment on the new branch explaining why the watch path overrides the global hard-fail-on-4xx rule for the transient-401 case.
- [ ] Step 2: In `internal/kernel/shell/github_test.go`, add a unit test using the existing `runFn` stub seam + `withFakeSleep` (mirror `TestRunWatchWorkflow_AppearPollRetries504OnFirstAttempt` and the `pollRunUntilComplete` tests):
  - Script `runFn` so the `gh run watch` invocation returns a transient-401 output on the first attempt(s) then success, and assert the watch ultimately returns nil (retry worked) and that backoff sleeps were recorded.
  - Add a negative case: a non-401, non-rate-limit error (e.g. a plain `command failed`) is **not** retried and fails fast.
  - Note: `RunWatchWorkflow` first runs the appear-poll via `runCaptureFn`; either drive `watchRunID` directly with a known run ID, or stub both seams so the watch call is reached deterministically.
- [ ] Step 3: Run `go test ./internal/kernel/shell/...` (scoped — never `go test ./...` on Windows; freeze hazard). Confirm green.

## Verification

- `go test ./internal/kernel/shell/...` passes.
- Operator-only (not an agent step): re-run the `gh-acceptance-stage` rehearsal to confirm the transient-401 no longer fails the prod-stage watch end-to-end.

## Notes

- **Single-language:** Go CLI only — there is no parallel Java/.NET/TS implementation of this watch logic to fix.
- **Out of scope (follow-up only):** the bash mirror `optivem/actions/shared/retry.sh` in the separate `optivem/actions` repo could face the same transient-401 class in scaffolded-workflow `gh run watch` calls; consider a parallel hardening there in a separate change.

## Open questions

- None — root cause and approach are pinned. (Retry bound/backoff: default to reusing `defaultRetryAttempts`/`defaultRetryDelays`; executor may pick a watch-specific bound if it reads clearer.)
