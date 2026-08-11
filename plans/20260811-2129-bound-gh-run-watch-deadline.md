# 2026-08-11 19:29:00 UTC — Bound the `gh run watch` path so a stalled GitHub run can't hang the verifier

## TL;DR

**Why:** `watchRunID` shells out to `gh run watch` with no deadline and no heartbeat. On 2026-08-11 GitHub held two workflow-run objects in a non-terminal state for ~49m and ~40m after all their jobs had finished; the CLI sat silent through both and the enclosing `go test -timeout 2h` killed the smoke job with an unreadable goroutine stack dump. Every *other* wait in the same chain is already bounded — this one isn't.
**End result:** `watchRunID` has a hard deadline, degrades to the already-bounded polling fallback when it expires, fails loud (naming the run URL and elapsed time) if that also expires, and prints a heartbeat while waiting — so a GitHub-side stall surfaces in minutes as an actionable error instead of as a two-hour silence.

## Diagnosis (for context — the fix is Steps 1–4)

Failure: <https://github.com/optivem/gh-optivem/actions/runs/31499562567/job/93806562815> — `run / Smoke (ubuntu-latest, multitier, multirepo, java)`, 2h1m7s.

```
panic: test timed out after 2h0m0s
	running tests:
		TestValidMultitierConfigurations (2h0m0s)
		TestValidMultitierConfigurations/multitier_multirepo_java_ts_typescript (2h0m0s)
```

Blocked in `runCLI`'s `cmd.Run` at `internal/config/config_system_test.go:200` (subtest launched at `config_system_test.go:289`). The three sibling smoke jobs passed in ~35m.

**Root cause, pinned:** `internal/kernel/shell/github.go:522` — `watchRunID` runs `gh run watch <id> --exit-status --interval <n>` wrapped only in `runWithRetryLoop`, with **no deadline**. Every sibling wait in the same chain *is* bounded:

- appear-poll — `runAppearAttempts × runAppearPollSecs` (`github.go:443-445`)
- fallback poll — `pollMaxDuration = 60 * time.Minute` (`github.go:24`), deadline enforced at `github.go:609`, whose own doc comment reads *"Bounded by pollMaxDuration so a stuck run / permanently malformed output cannot loop forever."*

The primary watch path is the only unbounded wait, and it's the one that hung.

**Evidence** — scaffolded repo `valentinajemuovic/test-app-50f5938d-e8056014eb846a98`:

| stage | run | last job completed | run object settled (`updated_at`) | CLI unblocked | wall |
|---|---|---|---|---|---|
| qa-stage | 31501646364 | 14:28:52 | 15:17:00 | 15:18:00 | 49m20s (for ~54s of job time) |
| prod-stage | 31506724695 | 15:25:05 | 16:03:43 | never | 40m32s+ |

Both downstream runs **succeeded**. No environment protection rules and no pending deployments (verified via the environments and deployments APIs) — GitHub simply held the run objects in a non-terminal state long after their last job finished, and `gh run watch` returned only when they flipped.

Because `watchRunID` emits no heartbeat, the CLI produced **zero log output** from 14:27:45 to 15:18:00, then again from 15:23:17 to the timeout. The only line it prints is the one-shot `log.Successf("Watching workflow run (polling every %ds): ...")` at `github.go:523`. The enclosing `go test -tags=system ./internal/config/ -v -timeout 2h` at `.github/actions/acceptance-test/action.yml:174` is what finally killed it at 16:08:17.

**Classification:** not a product bug in the scaffolder and not a test-authoring error — a GitHub-side stall that a missing bound converted into a silent 2-hour hang. Go only; gh-optivem is single-language for this code path, so there are no parallel implementations to mirror.

**Secondary cost (motivates Step 4):** the 300s watch interval inflates every stage. The passing sibling jobs spent 10m20s on Phase 9 for two runs that each took ~1 minute of real job time — the time went to poll granularity, not to work.

## Outcomes

- A stalled `gh run watch` can no longer block the scaffold verifier indefinitely — it hits a deadline, falls back to bounded polling, and then fails loud.
- A watch-path timeout produces a specific, actionable error naming the run URL and elapsed time, never a silent success and never a bare goroutine stack dump.
- The CI log shows periodic progress while a run is being watched, so a stall is visible as it happens instead of only in hindsight.
- Watched stages no longer pay ~5 minutes of pure poll latency each; the smoke jobs reclaim roughly 20 minutes of the 2-hour budget.
- Regression coverage exists for the deadline → fallback → loud-failure sequence, driven through the existing `runFn` / `sleepFn` test seams.

## ▶ Next executable step (resume here)

**Step 1** — add a bounded watch deadline to `watchRunID` in `internal/kernel/shell/github.go:522-547`.

Introduce a `watchMaxDuration` constant next to `pollMaxDuration` (`github.go:24`) and enforce it around the `runWithRetryLoop` call so the `gh run watch` subprocess cannot block past it. The existing structure already has the right shape to extend: on rate-limit the function falls through to `g.pollRunUntilComplete(runID)` — route deadline expiry into that same fallback, logging the switch the way the rate-limit path does at `github.go:545` (`log.Infof("Rate limit hit while watching run %s — switching to polling mode...", runID)`).

Termination matters: `gh run watch` runs via the `runFn` seam, so the deadline must actually kill the subprocess, not just abandon the goroutine — otherwise the orphaned `gh` process survives (the failing run's log ends with `Terminate orphan process: pid (23650) (gh)`).

Stop at a compiling change with no behavior change on the happy path; Step 2 handles what happens when the fallback *also* expires. Unblocks Steps 2 and 3.

## Steps

- [ ] **Step 1: Add a hard deadline to `watchRunID`.** In `internal/kernel/shell/github.go:522-547`, bound the `gh run watch` invocation with a new `watchMaxDuration` constant declared alongside `pollMaxDuration` (`github.go:24`). On expiry, terminate the subprocess and fall through to `g.pollRunUntilComplete(runID)`, logging the transition. Pick the constant's value so watch-deadline + poll-deadline together stay well inside the outer `-timeout 2h` backstop, and state the arithmetic in a code comment.
- [ ] **Step 2: Fail loud when the fallback also expires.** `pollRunUntilComplete` already returns `polling run %s timed out after %s` (`github.go:617`). Make the watch path wrap that into a message naming the run URL (`https://github.com/<repo>/actions/runs/<id>`) and total elapsed time across both phases, so the operator gets the link without grepping. Per the repo's fail-loud convention, a timed-out watch must **never** be reported as success — verify no caller coerces this error into a pass.
- [ ] **Step 3: Emit a heartbeat while watching.** Add periodic progress output to `watchRunID` (run URL + elapsed) so a stall is visible in the CI log. Today `github.go:523` prints once and then nothing. Match the existing progress idiom — `waitForRunToAppear` already uses `spinner.Start` / `sp.Update` (`github.go:469-478`) — and keep it legible in non-TTY GitHub Actions logs, which is where the silence actually hurt.
- [ ] **Step 4: Lower the watch interval from 300s to 60s.** Change the five call sites in `internal/scaffolding/steps/verify.go` — lines 328 and 336 (`RunWatchWorkflow` for `acceptance-stage.yml` / `acceptance-stage-legacy.yml`) and lines 379, 386, 393 (`verifyWorkflow` for `qa-stage.yml`, `qa-signoff.yml`, `prod-stage.yml`). This matches what `verifyCleanupIn` already does at `verify.go:423` (*"Cleanup is short-lived; poll every 60s instead of the 300s default"*). Confirm the increased poll rate doesn't push these paths into GitHub API rate limiting — `CheckRateLimit()` is already called before each watch.
- [ ] **Step 5: Cover the new path in tests.** Extend `internal/kernel/shell/github_test.go` using the existing `runFn` / `sleepFn` seams (see `internal/kernel/shell/testing_seam.go`): a stalled watch hits the deadline and switches to polling; a stall that outlives both deadlines returns an error naming the run; the happy path is unchanged and does not wait out any deadline. Assert on the error text, since the whole point is that the message is actionable.

## Out of scope

- **Do not raise `-timeout 2h`** at `.github/actions/acceptance-test/action.yml:174`. It is the outer backstop and it did its job; raising it hides the stall instead of surfacing it. The fix belongs in the layer that failed to bound itself.
- **Do not change `pollRunUntilComplete`'s existing 60m bound** (`github.go:24`, `github.go:609`) unless Step 1's arithmetic forces it — if it does, say so explicitly in the commit rather than adjusting it silently.

## Follow-up (tracked separately — do not fix here)

The failing suite left orphans behind: `TestMain`'s cleanup only runs on a green suite, so `test-app-50f5938d-e8056014eb846a98` plus its `-frontend` and `-backend` siblings are still live in the `valentinajemuovic` account and still firing hourly scheduled `acceptance-stage` runs (observed 16:17, 17:19, 18:17, 19:22 — all green). 19 `test-app-*` repos are currently orphaned there.

This is already covered by [`plans/20260701-0602-schedule-orphan-repo-cleanup.md`](20260701-0602-schedule-orphan-repo-cleanup.md), which schedules `gh-cleanup-orphans.yml` with an age cutoff. No new work here — just confirmation that the same gap is still open six weeks later, and that this incident added three more orphans to it.

## Verification

- `go test -p 2 ./internal/kernel/shell/ ./internal/scaffolding/...` passes. **Never** run unbounded `go test ./...` on Windows — it freezes the machine; use `scripts/test.sh` or scope with `-p 2`.
- Re-run the `gh-acceptance-stage` workflow and confirm the `multitier / multirepo / java` smoke job completes well inside 2h, with Phase 9 timing back near the ~10m the passing sibling jobs showed (or better, given Step 4).
- Spot-check a passing run's log for the Step 3 heartbeat lines — the watch window should no longer be a blank stretch.
