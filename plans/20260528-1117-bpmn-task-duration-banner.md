# Plan: BPMN-level duration banner for agent and command tasks

## Context

Today the BPMN orchestrator (the driver + the `run-command` action) does not report how long each BPMN task spent in its underlying subprocess. The agent's own exit banner (`clauderun.writeExitBanner`) prints elapsed time inside the `clauderun` layer — but that is the runner's banner, not the BPMN's. The `run-command` action prints no duration at all.

Goal: have the BPMN process layer measure and print wall-clock duration for every BPMN-level task whose body is a subprocess — agent dispatch and shell command execution.

## Status as of 2026-05-28

**Command-side (Items 2, 3, 4, 6, 7) landed** in commit on `actions/` package — `WriteBpmnTaskTiming` + `TruncateForBanner` helpers in new file `actions/banner.go`, `runCommand` wraps `a.runShell` with timing, tests for happy / failure / `originating-task-name` fallback paths.

**Agent-side (Items 1, 5) code is written in the working tree but not yet committed** — `driver.go` and `driver_test.go` are also being modified by parallel plan `20260528-1145-output-levels-phase-detail.md` (introducing `outlog` package, refactoring `installLogFileMirror`, ~244 shared diff lines). Committing the driver-side changes via the whole-file rule would absorb plan 1145's WIP under this commit's message. Waiting for plan 1145 to land or roll back first.

## Items

### Item 1 — Wrap the BPMN agent dispatch with timing  — ⏳ Deferred: code-written in working tree, commit blocked on plan 1145 (`outlog` levels) sharing driver.go

**File:** `internal/atdd/runtime/driver/driver.go`

The wrap is already applied in the working tree at the `newClaudeRunDispatcher` call site around `clauderun.Dispatch(...)`:

```go
t0 := nowFn()
_, runErr := clauderun.Dispatch(context.Background(), opts.ClaudeRunDeps, cOpts)
actions.WriteBpmnTaskTiming(opts.Stdout, raw.ID, "agent "+agentName, nowFn().Sub(t0))
if runErr != nil {
    return statemachine.Outcome{Err: runErr}
}
```

`actions` is already imported by `driver.go`. Resume: once plan 1145 lands or stashes, `git add internal/atdd/runtime/driver/driver.go` + commit.

### Item 5 — Tests for the agent wrap point  — ⏳ Deferred: code-written in working tree, commit blocked on plan 1145 sharing driver_test.go

**File:** `internal/atdd/runtime/driver/driver_test.go`

`TestClaudeRunDispatch_EmitsBpmnTaskTiming` and `TestClaudeRunDispatch_EmitsBpmnTaskTimingOnFailure` are already added in the working tree — pin `nowFn` to a deterministic clock, capture `opts.Stdout` via a `bytes.Buffer`, assert the banner substrings ("BPMN TASK AT_RED_TEST", "agent acceptance-test-writer", "45s" / "7s"). Failure path also exercises the non-zero clauderun exit to confirm the banner still prints.

Resume: same trigger as Item 1.

## Out of scope

- **Touching `clauderun.writeExitBanner`.** The existing AGENT EXIT banner (and its `(1m 23s, 12.4k in / 1.8k out, $0.18)` suffix) stays. BPMN timing is additive — printed after the AGENT EXIT banner, not in place of it.
- **JSONL audit-log persistence.** The new line goes to Stdout only. Persisting per-task duration into the existing per-event JSONL audit log (see commit `82e0303 atdd: persist headless dispatch as per-event JSONL audit log`) is a separate concern and would need its own plan.
- **Other BPMN task kinds.** Service-tasks that are pure in-process Go (e.g. `parse-ticket`, `validate-outputs-and-scopes`) are not subprocess-bound; their wall-clock is negligible and the banner would be noise. Only the two subprocess-backed task kinds (agent dispatch + shell command) are wrapped.
- **Per-step start banner for commands.** User chose completion-only for commands. If a future plan wants symmetry with the agent's ENTERING/EXITED pair, add the start line then.

## Verification

- `go test ./internal/atdd/... -p 2` passes (per Windows test memory). **Currently blocked** — `statemachine/load.go` references undefined `validateAutoDefault` (parallel plan `20260528-1150-auto-default-on-loopable-choosers.md` Items 3/4/6/8 deferred). Once that compiles, tests can run.
- `bash scripts/atdd-rehearsal.sh <issue> --config gh-optivem-monolith-typescript.yaml` produces, after each agent dispatch and after each `run-command` invocation, a single new `⏱  BPMN TASK …` line on stdout. Visually inspect that it sits below the existing AGENT EXIT banner (for agents) and below any `run-command: <err>` line (for commands).
