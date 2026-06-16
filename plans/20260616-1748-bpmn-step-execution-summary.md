# 2026-06-16 17:48:33 UTC — End-of-run BPMN step-execution summary (agents + commands, with timing)

## TL;DR

**Why:** When `gh optivem implement ticket` finishes, the only end-of-run summary covers **agent dispatches**. Command steps (and the overall shape of which BPMN steps ran) are invisible, and there's no per-step timing or grand total that lets you see where a run spent its wall-clock.
**End result:** Alongside the existing agent-summary table, the run prints an additional **step-execution summary**: every executed BPMN step in order, its kind (agent / command / human), its elapsed time, and a total-execution-time row at the bottom. It's also persisted to the run's sidecar so `gh optivem run summary` can replay it.

## Outcomes

What we get out of this — the goals and deliverables:

- A new end-of-run table listing **every atomic (MID-level) BPMN step that executed**, in execution order — not just agent dispatches. Composite HIGH/CYCLE levels stay out of the table; the existing `[phase]` banners + `flow.txt` carry the hierarchy.
- Each row shows: step name, **kind** (agent / command / human-approval), and **elapsed time**. When a step runs more than once (fix loops / retries), each execution is its **own row** in order.
- A bottom **total** = the run **wall-clock** as the headline figure, with the **sum of step elapsed times** shown alongside it (the two differ due to gaps/overhead).
- Command steps (`execute-command`) are timed and recorded, not just agents.
- The step summary is written to the run sidecar (`.gh-optivem/runs/<run-ts>/`) so it survives the terminal and is replayable via `gh optivem run summary`.
- The existing agent-summary table and run digest (`summary.md`) are preserved — this is additive, not a replacement.

## ▶ Next executable step (resume here)

Design is settled (see Outcomes). The first mechanical unit is **Step 1**: introduce a `stepRecord` type + a step-timer wrapper that wraps *all* MID-level NodeFns (mirroring how `wrapPhaseBoundaries` already wraps TOP-process call-activities and how the agent dispatcher already times itself), so both agent and command steps accumulate `(name, kind, elapsed, err)` into `runState` in execution order.

## Steps

- [ ] Step 1: Add a `stepRecord` type (name, kind: agent|command|human, elapsed, err) and a per-step timer wrapper that wraps every MID-level NodeFn — reuse the wrapping pattern in `wrapPhaseBoundaries()` (driver.go ~1594) and the `nowFn()` timing pattern already used in `newClaudeRunDispatcher()` (driver.go ~1346). Accumulate records into `runState` in execution order.
- [ ] Step 2: Classify each step's kind. Agent steps are identifiable today (`node.Kind == UserTask && raw.Agent != "" && != "human"`); command steps run via the `runCommand` action (`internal/atdd/process/actions`); human-approval steps are `approve`. Decide the kind at wrap time, not at render time.
- [ ] Step 3: Render the new step-execution table at run end. Add a `renderStepSummary()` alongside `renderAgentSummary()` (driver.go ~1875) and print it from a deferred tail in `Run()` (alongside `printAgentSummary`). Bottom row = total execution time (the run wall-clock already captured as `flowStart`/`WallClock`, driver.go ~410).
- [ ] Step 4: Persist the step records to the run sidecar — a `steps.jsonl` (mirror `appendSummaryLine()` in `summary_sidecar.go`) and/or a new section in the Markdown digest (`renderRunDigest()` ~351). Wire `gh optivem run summary` replay (`PrintSummaryFile()` ~274) to render it.
- [ ] Step 5: Tests — unit-test the renderer (ordering, kind labels, total row) with a fixed `nowFn`; verify command steps now appear. Scope to the driver package; never run unbounded `go test ./...` on Windows (use `-p 2` / `scripts/test.sh`).
- [ ] Step 6: Verify on a real slice (e.g. shop #72, the full-coverage rehearsal story) that command steps are timed and the totals line up with the existing phase boundaries / wall-clock.

## Resolved decisions

- **Granularity:** MID-level atomic steps only in the table; composites stay in `[phase]` banners + `flow.txt`.
- **Output destination:** all three for parity with the agent summary — terminal + `summary.md` digest section + `steps.jsonl` sidecar.
- **Columns:** name, kind, elapsed; one row per execution (loops/retries each get a row).
- **Total:** run wall-clock as the headline, sum of step elapsed times shown alongside.
