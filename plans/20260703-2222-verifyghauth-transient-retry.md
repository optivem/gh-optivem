# 2026-07-03 22:22:00 UTC — Add a one-shot retry to `verifyGhAuth` for transient CI auth flakes

## TL;DR

**Why:** In run [28671927610](https://github.com/optivem/gh-optivem/actions/runs/28671927610), the `run (multitier, monorepo, dotnet, java)` combo failed at the `environment verify` "gh CLI auth" check with "The token in GH_TOKEN is invalid" — yet 17 of 18 combos in the same run passed with the identical `VERIFY_TOKEN`. The token is valid; this was a transient/spurious auth failure under concurrent matrix load. `verifyGhAuth` runs a single `gh auth status` with no retry, so one blip kills a whole combo.
**End result:** `verifyGhAuth` retries a failed `gh auth status` once (jittered 2–5s backoff), mirroring the documented one-shot retry the HTTP sibling `githubUserAuthCheck` already has. A truly-invalid token still fails loud after both attempts; a transient concurrency blip recovers silently.

## Outcomes

What we get out of this — the goals and deliverables:

- A single transient `gh auth status` failure under concurrent matrix load no longer fails an acceptance combo — `verifyGhAuth` retries once before surfacing an error.
- Fail-loud behavior is preserved: a genuinely unauthenticated/expired token fails **both** attempts and surfaces the unchanged `gh CLI is not authenticated. / Run: gh auth login / Output:<combined output>` error.
- `verifyGhAuth`'s resilience now matches its HTTP sibling `githubUserAuthCheck` (`token_auth.go:156-222`) — both auth checks that hit GitHub with the shared `VERIFY_TOKEN` guard against per-token-throttle transients.
- A unit test proves the retry recovers a fail-then-succeed `gh`, and the existing always-fail test proves the retry does not mask a bad token.

## ▶ Next executable step (resume here)

Edit `internal/config/tool_checks.go` → `verifyGhAuth` (currently lines 38-51): factor the `gh auth status` invocation into a one-shot-retry wrapper. On the first `CombinedOutput()` failure, sleep a jittered `2s + rand.IntN(3001)ms` (via a package-level, test-overridable `ghAuthRetrySleep` seam) and retry once; return the second attempt's error (unchanged wrapped message) only if it also fails. Copy the backoff shape verbatim from `githubUserAuthCheck` (`token_auth.go:209-221`); add `math/rand/v2` + `time` imports. Do **not** route through `shell.RetryWithPolicy` — see Step 1 note. This unblocks the test in Step 2.

## Steps

- [ ] **Step 1: Add the one-shot retry to `verifyGhAuth`** (`internal/config/tool_checks.go`).
  - Keep the `exec.LookPath("gh")` gate and the final wrapped error message exactly as-is.
  - Wrap the `gh auth status` call so that: attempt 1 runs `exec.Command("gh", "auth", "status").CombinedOutput()`; on error, sleep jittered backoff, then attempt 2; success on either attempt returns `nil`; failure on attempt 2 returns the existing `fmt.Errorf("gh CLI is not authenticated.\n    Run: gh auth login\n    Output:\n%s", out)` using attempt 2's output.
  - Backoff: `2*time.Second + time.Duration(rand.IntN(3001))*time.Millisecond`, mirroring `token_auth.go:219` (uses `math/rand/v2`).
  - Add a package-level seam `var ghAuthRetrySleep = time.Sleep` (function-typed) so the test overrides it to a no-op; call `ghAuthRetrySleep(delay)` instead of `time.Sleep(delay)` directly. (Check the package for an existing sleep seam to reuse before adding a new one.)
  - **Do NOT** use `shell.RetryWithPolicy(shell.RetryTransient(), shell.RetryHardFail(), ...)`: the `gh` failure text ("token ... is invalid", "not logged into any GitHub hosts") matches neither the transient nor the hard-fail regex (`internal/kernel/shell/retry.go:20-45`), so that path would classify it as non-retryable and never retry. An unconditional single retry on any failure is the correct shape here — same as the sibling's outer 401-retry, which is also unconditional-on-failure.
- [ ] **Step 2: Add a retry-recovers test** in `internal/config/verify_environment_tools_test.go`.
  - Override `ghAuthRetrySleep` to a no-op for the test (restore via `t.Cleanup`).
  - Use `mkPathDir`/`writeStub` to plant a `gh` stub that FAILS on its first invocation and SUCCEEDS on the second (counter file in the stub dir; increment on each call, `exit 1` while count==1 else `exit 0`). Plant a happy `actionlint` stub (`exit 0`), `setAllEnvTokens(t)`.
  - Assert `verifyEnvironmentWithClient(nil, "", happyAuthClient())` returns `nil` (the retry recovered).
- [ ] **Step 3: Confirm fail-loud is intact.** Verify the existing `TestVerifyEnvironment_GhAuthFails` (always-`exit 1` stub) still asserts the `gh CLI is not authenticated` / `gh auth login` / stub-output error surfaces — the retry must not mask a truly-bad token. (Override `ghAuthRetrySleep` to no-op there too so the always-fail path doesn't add real backoff to the test.)

## Verification

- `go test -p 2 ./internal/config/` passes. **Never** run unbounded `go test ./...` on Windows — it freezes the machine (use `-p 2` or `scripts/test.sh`, or scope to the one package as above).
- Operator (optional): re-dispatch the gh-acceptance pipeline and confirm no combo fails at the "gh CLI auth" check on a valid `VERIFY_TOKEN`.

## Notes

- Classification: **flake** (transient GitHub auth under concurrency), not a product bug and not an expired/bad token. Go-only — no parallel .NET/Java/TS implementations to mirror.
- Reference for the backoff shape and rationale: `internal/config/token_auth.go:156-222` (`githubUserAuthCheck`), especially the doc block at 156-166 and the outer retry at 209-221 — *"concurrent matrix jobs hit api.github.com with the same PAT, GitHub's per-token throttling can return a transient 401 even though the token is valid ... one retry makes that vanishingly rare."*
- In the failing job, `GH_TOKEN = secrets.VERIFY_TOKEN` (`.github/workflows/_gh-acceptance-pipeline.yml:538`); `max-parallel: 12` run jobs share that one PAT.
