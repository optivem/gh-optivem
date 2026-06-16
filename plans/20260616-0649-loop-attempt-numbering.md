# 2026-06-16 04:51:38 UTC — Generic loop-attempt numbering (Attempt #N) for fixers and any looped node

## TL;DR

**Why:** The engine already knows which attempt a looped node is on — `runProcess`'s per-node `visits` counter (`internal/engine/statemachine/run.go:232,251`) drives the `max-visits`/`on-max-visits` fixer-loop cap — but that count is **local to `runProcess` and never escapes**. So no fixer prompt knows it's on its 2nd (last) pass, and the end-of-run summary shows two consecutive `command-failed-fixer` rows as if they were unrelated agents rather than Attempt 1 / Attempt 2 of one loop.
**End result:** A single generic wiring exposes the current node's visit count downstream so that (a) any looped agent's prompt can render **"Attempt #N of M"**, and (b) the agent-summary table labels repeated dispatches of the same looped node as **`(attempt N/M)`**. Loop-agnostic: it covers the four fixers and any future `max-visits` loop for free.

## Outcomes

- The statemachine engine exposes the **current node's visit count and its max-visits cap** to the run `Context` (generic "visit count" vocabulary at the engine layer — no ATDD "attempt" terms leak into `internal/engine`).
- The ATDD driver maps that visit count to an **attempt number** and threads it into `clauderun.Options`, so a `${attempt-number}` / `${attempt-max}` substitution is available to prompts of looped nodes.
- The **shared fixer preamble** surfaces the loop-level attempt context ("the loop has dispatched you N of M times; after M it halts") — kept distinct from the existing per-agent **"one attempt only — do not retry"** rule (that rule governs the agent's *own* single pass; the new line is loop-level caller context).
- The **agent summary** (live `renderAgentSummary` + the replayable summary sidecar) labels repeated dispatches of a looped node `agent (attempt N/M)`, so multi-pass fix loops read as one loop, not N strangers.
- **Non-looped nodes are unchanged** — attempt labelling engages only where `max-visits > 0`. A single-pass dispatch renders no attempt line and no summary suffix.

## ▶ Next executable step (resume here)

Start at **Step 1 (engine)** — it is the single source of truth everything else reads, and it is mechanical. Confirm the `Context` reserved-key naming with the user only if Step 1's grep shows an existing collision; otherwise proceed. Decisions are already settled (see Resolved decisions): per-node scope that resets each loop, and surfacing in **both** prompt and summary.

## Steps

- [ ] **Step 1 — Engine exposes the current node's visit count (generic).** In `internal/engine/statemachine/run.go`, right after `visits[cur]++` (line 251) and before `node.Fn(ctx)` (line 255), write the current node's visit count and its cap onto `ctx` via `Context.Set` (`internal/engine/statemachine/context.go:34`). Use generic engine-layer key names (e.g. `visit-count` / `visit-max`) — **not** ATDD "attempt" vocabulary (keeps `internal/engine` free of process-specific terms). `visits[cur]` is already 1-based at this point (incremented before the body runs), and `node.Raw.MaxVisits == 0` means "uncapped / not a loop". No behaviour change to the existing `max-visits`/`on-max-visits` routing (lines 247-249).
- [ ] **Step 2 — Driver threads attempt into `clauderun.Options`.** In `internal/atdd/runtime/driver/driver.go` where `cOpts := clauderun.Options{…}` is built (around line 1201), read the engine's `visit-count` / `visit-max` from `ctx` and pass them as new `Options` fields (e.g. `AttemptNumber`, `AttemptMax`), mapping the engine's generic "visit" to the ATDD "attempt" here at the boundary. Only meaningful when `AttemptMax > 0`.
- [ ] **Step 3 — clauderun renders the attempt placeholders.** In `internal/atdd/process/clauderun/clauderun.go`, add `AttemptNumber`/`AttemptMax` to the `Options` struct (near line 276) and fill the `params` map (near line 770, alongside `scope-block`) with an **`${attempt-block}`** pre-rendered string — non-empty (e.g. "Attempt #2 of 2 — the loop halts after this pass.") when `AttemptMax > 0`, otherwise leave it unfilled. **Constraint:** `findUnfilledPlaceholders` (`internal/atdd/process/clauderun/clauderun.go:580,690`) hard-fails any dispatch whose prompt references an unfilled `${…}`. Prefer one pre-rendered `${attempt-block}` placed only in the fixer preamble (always a loop node, so always fillable) over raw `${attempt-number}`/`${attempt-max}` scattered in prompts — that keeps non-loop prompts that never mention it safe. If raw number/max placeholders are also wanted, gate them behind the same loop-only inclusion.
- [ ] **Step 4 — Fixer preamble surfaces the attempt block.** In `internal/atdd/assets/runtime/shared/fixer-preamble.md`, add a line rendering `${attempt-block}`, worded as **loop-level caller context** ("the orchestrator has dispatched you N of M times; on the Mth the fix loop is exhausted and a human is asked to widen scope"). Explicitly do **not** weaken or contradict the existing line 3 "One attempt only — do not retry … the caller re-validates after you exit" — the new line tells the agent *where in the loop it is*, not that it may loop itself.
- [ ] **Step 5 — Summary labels repeated looped dispatches.** Add an `attempt` field to `dispatchRecord` (`internal/atdd/runtime/driver/driver.go:1590`), populated where the record is built (line 1259) from the same `visit-count` used in Step 2. In `renderAgentSummary` (lines 1798-1825) suffix the agent column `name` with ` (attempt N/M)` when the record carries `AttemptMax > 0`; leave single-pass rows untouched. The leading `#` column stays a dispatch-sequence counter (`i+1`) — attempt is orthogonal and shown in the agent name.
- [ ] **Step 6 — Persist attempt in the replayable summary sidecar.** Add the attempt to the JSON record written/read by the summary sidecar (`internal/atdd/runtime/driver/summary_sidecar.go`) so `gh optivem run summary` replays show the same `(attempt N/M)` labels as a live run.
- [ ] **Step 7 — Tests.** Engine test: a node with `max-visits: 2` sets `visit-count` 1 then 2 on `ctx` across two visits (and `0`/unset for an uncapped node). clauderun test: `AttemptMax > 0` renders the attempt block; `== 0` leaves `${attempt-block}` unfilled and does not trip `findUnfilledPlaceholders` for prompts that omit it. Summary test: two same-agent records with attempts render `(attempt 1/2)` / `(attempt 2/2)`; a single-pass record renders no suffix. Scope `go test` per-package (no unbounded `go test ./...` on Windows — use `-p 2` or `scripts/test.sh`).

## Verification

- Run a story that deliberately fails a fixer-governed gate (e.g. a failing `RUN_COMMAND`) so the fix loop dispatches twice, and confirm by eye: the 2nd fixer prompt shows "Attempt #2 of 2", and the end-of-run agent summary shows `command-failed-fixer (attempt 1/2)` then `(attempt 2/2)`.
- Replay that run via `gh optivem run summary` and confirm the sidecar shows the same attempt labels.

## Resolved decisions

1. **Scope = per-node, resets each loop.** Attempt number is `visits[cur]` for the specific looped node, matching the engine's existing semantics; a second independent loop later in the run starts back at #1. (Chosen over a single global retried-dispatch counter, which would conflate unrelated loops.)
2. **Surface in both prompt and summary** — full generic wiring (not summary-only or prompt-only).
3. **Engine stays generic.** The "attempt" vocabulary lives only in the ATDD driver/prompts; `internal/engine/statemachine` speaks in "visit count" so the mechanism isn't ATDD-coupled.

## Open questions

1. **Engine `Context` key names.** Confirm `visit-count` / `visit-max` don't collide with existing reserved keys (grep at Step 1). If a typed accessor on `Context` is preferred over string state keys, decide that before Step 1.
2. **`${attempt-block}` wording.** Final phrasing of the rendered block (and whether to also expose raw `${attempt-number}`/`${attempt-max}` for prompt authors) — settle during Step 3/4 encoding.

## Cross-references

- Loop mechanics live in `internal/atdd/process/process-flow.yaml` (`max-visits: 2` / `on-max-visits: FIX_LOOP_EXHAUSTED` on `RUN_TESTS`, `RUN_COMMAND`, and `FIX` nodes) and `internal/engine/statemachine/run.go`. This plan does **not** change the YAML caps — only what the engine exposes about them.
