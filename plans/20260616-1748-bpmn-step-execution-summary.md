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

All code is implemented and committed (see git log: `step_summary.go` + driver/sidecar/run-command wiring + tests, all passing under `go test -p 2 ./internal/atdd/runtime/driver/`). Only **operator verification** remains (see `## Verification`) — run a real slice and eyeball the new `=== Step summary ===` table + `summary.md` "Steps executed" section. No further mechanical edits are queued; nothing for `/execute-plan` to do.

## Verification (operator-run)

- [ ] Run a real slice (e.g. shop #72, the full-coverage rehearsal story) and confirm: command steps now appear in the `=== Step summary ===` table with timing, the wall-clock total lines up with the existing `[phase]` boundaries, and `gh optivem run summary` + `--markdown` replay the step table from `steps.jsonl` / `summary.md`.

## Resolved decisions

- **Granularity:** MID-level atomic steps only in the table; composites stay in `[phase]` banners + `flow.txt`.
- **Output destination:** all three for parity with the agent summary — terminal + `summary.md` digest section + `steps.jsonl` sidecar.
- **Columns:** name, kind, elapsed; one row per execution (loops/retries each get a row).
- **Total:** run wall-clock as the headline, sum of step elapsed times shown alongside.
