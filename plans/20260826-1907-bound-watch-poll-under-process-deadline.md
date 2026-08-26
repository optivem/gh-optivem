# 2026-08-26 19:07:00 CEST — Bound watch + poll under a process-wide deadline, and stop misreporting GitHub `startup_failure`

## TL;DR

**Why:** `gh-acceptance-stage` run [32976603425](https://github.com/optivem/gh-optivem/actions/runs/32976603425) lost all four `dotnet, java` matrix legs to a GitHub-side Actions stall (~15:00–15:07 UTC on 2026-08-26). The stall was infra, but gh-optivem handled it badly three ways: the watch+poll budget overran the outer `go test -timeout 2h` so the loud, actionable error never fired; the polling fallback went 47m47s dark in the CI log; and a `startup_failure` on the `workflow_dispatch` path was reported as a scaffolded-app workflow failure.

**End result:** gh-optivem always emits its own loud error — naming the run URL and the elapsed time — *before* the outer harness kills it, the polling phase is as visible as the watch phase, and an infra `startup_failure` is recovered by bounded re-dispatch on both watch entrypoints instead of being blamed on the scaffolded app.

## Outcomes

What we get out of this — the goals and deliverables:

- A stalled GitHub run can no longer produce `panic: test timed out after 2h0m0s`. Whatever point in the scaffold verification the stall begins, gh-optivem's own error (`workflow run <URL> never reported a terminal state … elapsed across watch + polling`) fires first.
- The polling fallback logs a heartbeat on the same 5m cadence as the watch phase, carrying the run URL — no more multi-hour silent stretches in non-TTY Actions logs.
- A GitHub `startup_failure` on a `workflow_dispatch`-triggered run (prod-stage, cleanup, acceptance-stage) is recovered by bounded re-dispatch, exactly as it already is for push-triggered runs — so operators stop being pointed at nonexistent bugs in the scaffolded app.
- The doc-comment arithmetic in `internal/kernel/shell/github.go:29-42` states an invariant that is actually true, instead of the current false "leaving 30m of headroom" claim.
- Regression coverage in `internal/kernel/shell/github_test.go` that would have caught all three defects, driven through the existing `nowFn` / `sleepFn` / `watchRunFn` seams (no real subprocess, no real clock).

## Evidence (the run that exposed this)

Job `98212141832` (`monolith, monorepo, dotnet, java`), exact timeline:

| Time (UTC) | Event |
|---|---|
| 14:20:35 | `go test -tags=system … -timeout 2h` starts → hard kill at **16:20:35** |
| 15:03:00 | cleanup watch starts — **42m25s into the test** |
| 15:33:00 | `watchMaxDuration` (30m) expires; falls back to polling ✅ |
| 15:33:00 → 16:20:48 | **zero log lines — 47m47s dark** |
| 16:20:48 | `panic: test timed out after 2h0m0s` |
| *16:33:00* | *when `pollMaxDuration` (60m) would have emitted the loud error — 12m too late* |

Supporting facts, for anyone re-reading this later:

- Cleanup run `32983911817` was still `queued` two hours after creation; it never started.
- Prod-stage run `32983779814` and cleanup run `32983833982` were both stamped `startup_failure`.
- `32983833982` was stamped `startup_failure` at 15:04:47, yet its `cleanup` job started at 15:07:20 and **succeeded** — proof that the run-level conclusion is not a verdict on the workflow.
- The parent `optivem/gh-optivem` run itself remained `queued` on `Create Prerelease`. Same window, both the org repo and the personal scaffolded repos — infra, not us.

## ▶ Next executable step (resume here)

Implement **Step 1** — the process-wide hard deadline — in `internal/kernel/shell/github.go`:

- Add a `processDeadline` seam alongside the existing `nowFn` (near `github.go:25-47`): resolved once, lazily, from `GH_OPTIVEM_HARD_DEADLINE_MINUTES`; when the var is absent or unparseable, it resolves to "no process deadline" and today's behaviour is unchanged.
- Clamp `runBoundedWatch`'s `deadline` (`github.go:708`) and `pollRunUntilComplete`'s `deadline` (`github.go:857`) to `min(own budget, processDeadline)`.
- Rewrite the false arithmetic in the `watchMaxDuration` doc-comment (`github.go:29-42`).

Stop at a compiling package with `go test -p 2 ./internal/kernel/shell/` green (do **not** run unbounded `go test ./...` on Windows). This unblocks Step 4's clamp test and is independent of Steps 2 and 3.

## Steps

- [ ] **Step 1 — Process-wide hard deadline (Defect 1, primary).** In `internal/kernel/shell/github.go`: introduce a lazily-resolved process deadline sourced from `GH_OPTIVEM_HARD_DEADLINE_MINUTES` (`GH_OPTIVEM_` prefix, no internal jargon in the name). Clamp both `runBoundedWatch` (`github.go:708`) and `pollRunUntilComplete` (`github.go:857`) to `min(own budget, processDeadline)`. Unset → current behaviour, so local dev and `gh optivem` end-user runs are unaffected. Correct the doc-comment at `github.go:29-42` to state the real invariant: *gh-optivem emits its own loud error before the outer harness kills it, from any start offset* — replacing the false `30m + 60m = 90m, leaving 30m of headroom` arithmetic.

- [ ] **Step 2 — Export the deadline from CI.** In `.github/actions/acceptance-test/action.yml` (the `go test … -timeout 2h` invocation is at line 174): export `GH_OPTIVEM_HARD_DEADLINE_MINUTES` derived from the same `2h` value minus a safety margin, so the timeout and the deadline cannot drift apart. Prefer deriving both from one place in the action rather than writing `2h` and `105` as two independent literals.

- [ ] **Step 3 — Heartbeat the polling phase (Defect 2).** Reuse `startWatchHeartbeat` (`github.go:752`) — or extract the shared shape — inside `pollRunUntilComplete` (`github.go:856-902`) so the poll phase logs `Still polling <runURL> — <elapsed> of a <budget> poll deadline` on the `watchHeartbeatInterval` (5m) cadence. The existing doc-comment at `github.go:744-751` already explains *why* this is mandatory in non-TTY logs; the poll path simply never got it.

- [ ] **Step 4 — `startup_failure` recovery on the dispatch path (Defect 3).** In `RunWatchWorkflow` (`github.go:805`), reuse `hasRecentStartupFailure` (`github.go:631`) plus bounded re-dispatch (`maxReDispatches`, `github.go:591`) the way `RunWatchPushWorkflow` (`github.go:828`) already does. Keep fail-loud semantics per the repo's `check-*` convention: once the re-dispatch budget is spent, still return an error — never coerce an indeterminate result into success. Check whether the re-dispatch logic is worth extracting so the two entrypoints share one implementation rather than diverging again.

- [ ] **Step 5 — Regression tests.** Extend `internal/kernel/shell/github_test.go` (seams: `nowFn`, `sleepFn`, `watchRunFn` / `SetWatchRunFnForTest`; existing coverage at `github_test.go:127` and `github_test.go:374` already drives watch-deadline → poll-deadline → loud failure):
  - (a) with a process deadline set and a watch started *late* (simulating the 42m offset), assert the loud error fires strictly before the process deadline;
  - (b) assert the poll phase emits heartbeat lines at the expected cadence;
  - (c) assert `RunWatchWorkflow` re-dispatches on `startup_failure` and still fails loud once `maxReDispatches` is spent.

- [ ] **Step 6 — Verify.** Run `scripts/test.sh` (or package-scoped `go test -p 2 ./internal/kernel/shell/ ./internal/scaffolding/steps/`). Never `go test ./...` unbounded on Windows — it freezes the machine.

## Verification

- One re-run of `gh-acceptance-stage`, confirming the `dotnet, java` legs pass.
- Confirm from a future stalled run (or by inspecting the new tests' output) that a GitHub-side stall surfaces as gh-optivem's own loud error with the run URL, not a `panic: test timed out` goroutine dump.

## Open questions

- **Safety margin size for `GH_OPTIVEM_HARD_DEADLINE_MINUTES`.** Inferred, not user-stated. Suggest 105m against the 2h timeout (15m margin) — enough for the error to print, the bug-report prompt to render, and the `Keeping local dir` cleanup lines to flush. Confirm or adjust.
- **Should the process deadline also bound `waitForRunToAppear` (`github.go:805` → appear-poll loop, `runAppearAttempts = 6` at `github.go:585`)?** It is separately bounded today and did not contribute to this failure, but it is the third place a GitHub stall can burn wall-clock. Inferred as out of scope for now.
- **Does the `acceptance-stage.yml` / `acceptance-stage-legacy.yml` watch path (`internal/scaffolding/steps/verify.go:341,349`) want the same `startup_failure` recovery?** It goes through `RunWatchWorkflow`, so Step 4 gives it that recovery automatically — flagging it so the behaviour change is a deliberate choice rather than a side effect.
