# 2026-06-20 16:24:07 UTC — Pend-for-human on non-TTY approval gates (fix silent auto-reject of human-tier dispatches)

## TL;DR

**Why:** In an unattended rehearsal/CI/cron run, a human-tier approval gate (e.g. the `unexpected-failing-tests-fixer` dispatch) was supposed to *pend for a human and stay resumable* — but `newApproveDispatcher` lacks the non-TTY guard that `newClaudeRunDispatcher` already has, so the y/n read hit `io.EOF` and was silently treated as a rejection. The BPMN then counted that never-run dispatch as a fix-loop visit and fired a misleading `FIX_LOOP_EXHAUSTED` ("2 fix attempts" that never happened).
**End result:** A **single shared guard** — referenced by every human-gate dispatcher, never copy-pasted — yields `ErrPendingHuman` whenever stdin is not a TTY and the resolved category is `human`, so the run exits `ExitCodePendingHuman` and is resumable from the last committed phase. No human-tier gate (approval, fixer, or `agent: human` STOP) silently auto-rejects unattended again. Belt-and-suspenders: the BPMN fix loop no longer counts a rejected (never-run) dispatch as a fix attempt.

## Outcomes

What we get out of this — the goals and deliverables:

- An unattended run (`--auto --headless`, non-TTY stdin) that reaches a **human-tier approval gate** pends-for-human and is **resumable**, instead of silently auto-rejecting.
- A never-run (rejected-because-unattended) fixer dispatch is **never counted as a fix attempt** — so no spurious `FIX_LOOP_EXHAUSTED` claiming attempts that didn't happen, and no wasted no-op test re-runs.
- **All** human-gate dispatchers — `newApproveDispatcher`, `newHumanStopDispatcher`, and `newClaudeRunDispatcher` — share **one** guard (a single helper, or a single chokepoint at the wrap site), so guard drift between paths (the exact root cause here) is structurally impossible going forward.
- The BPMN fix loop treats a fix dispatch that ends via `EXECUTE_AGENT_REJECTED_END` as **not an attempt** (Option 2): it does not count toward `max-visits` and does not masquerade as `FIX_LOOP_EXHAUSTED` — covering a deliberate *interactive* human reject as well as any future reject-without-run path.
- Regression coverage: a test proving a non-TTY human-tier gate yields `ErrPendingHuman` rather than `approval-outcome=rejected`, and a test proving a rejected fix dispatch is not counted as a fix attempt.

## ▶ Next executable step (resume here)

Read-only verification in `internal/atdd/runtime/driver/driver.go` to settle *where the single guard lives* and confirm it can be applied uniformly:
1. Category resolution per dispatcher: `newApproveDispatcher` (≈1092-1128) resolves via `classifyApproveCategory(raw, ctx)` at **runtime** (~1114, call-site override from `ctx.Params`); `newHumanStopDispatcher` (~1028) and `newClaudeRunDispatcher` (~1160) determine human-ness differently. Establish a single "is this a human-tier gate?" predicate usable by all three.
2. Whether category is resolvable at the **wrap site** (`wrapAgentDispatchers`, ~1007-1036) so the guard can be applied once there (preferred — auto-covers future dispatchers); if not (because approve's category is runtime/ctx-dependent), fall back to one **shared helper** each dispatcher calls at the top of its `NodeFn`.
3. `stdinIsTTYFn` and the `ErrPendingHuman` sentinel are in scope for all three dispatchers (already used by `newClaudeRunDispatcher` at ~1211-1214).
4. `ErrPendingHuman` returned from an `approve` user-task propagates to `Run` → `ExitCodePendingHuman` identically to the agent-dispatch path (uncommitted edits discarded; resume re-enters from the clean committed tree) — nothing special-cases the `approve` process to swallow/mistranslate the sentinel.

Then implement the **single** guard (helper or wrap-chokepoint per finding #2) and route all three human-gate dispatchers through it. This is the core fix and unblocks everything else.

## Steps

- [ ] **Step 1 — Verify the seams (read-only).** Settle the four points in the resume block: a single human-tier predicate across the three dispatchers, whether the guard can live at the wrap site vs. a shared helper, `stdinIsTTYFn`/`ErrPendingHuman` scope, and that an `approve`-gate `ErrPendingHuman` reaches `Run` → `ExitCodePendingHuman` the same way the dispatcher's does.
- [ ] **Step 2 — Implement the single shared guard and route all human-gate dispatchers through it** (`internal/atdd/runtime/driver/driver.go`). Extract the non-TTY → `ErrPendingHuman` logic into one helper (or a wrap-site chokepoint, per Step 1's finding). Apply it to `newApproveDispatcher`, `newHumanStopDispatcher`, **and** refactor `newClaudeRunDispatcher`'s existing inline guard (~1211-1214) to call the same helper — so there is exactly one copy. Mirror the existing "human gate reached, no operator TTY — yielding to pending-human; resumable" notice to `opts.Stderr`.
- [ ] **Step 3 — BPMN: a rejected fix dispatch is not a fix attempt (Option 2)** (`internal/atdd/process/process-flow.yaml`). In the fix loop, route a dispatch that ends via `EXECUTE_AGENT_REJECTED_END` so it does **not** flow back to `RUN_TESTS` as a counted `max-visits` visit and does **not** surface as `FIX_LOOP_EXHAUSTED`. Prefer a distinct, honest terminal (e.g. `FIX_FLOW_NOT_APPROVED`) for a deliberate human reject. Use the `bpmn-logic-audit` agent first to confirm the gateway/back-edge change is sound and doesn't break the red-green model or `max-visits` accounting.
- [ ] **Step 4 — Regression tests.** (a) A driver test driving a `category: human` gate with a non-TTY stdin asserts `ErrPendingHuman` (not `approval-outcome=rejected`), covering the approval gate AND the human-STOP path. (b) A process/transitions test asserting a rejected fix dispatch is not counted toward `max-visits` / does not reach `FIX_LOOP_EXHAUSTED`.
- [ ] **Step 5 — Build + targeted test run.** `go build ./...`, then run the driver, approval, and process-package tests. Confirm the interactive-TTY path is unchanged (a real operator can still answer y/n, including a deliberate reject; only the non-TTY case yields pending-human).
- [ ] **Step 6 — Delete this plan after execution** (per workspace convention: completed plans aren't kept as reference — git history is the record).

## Open questions

- **Guard placement mechanism — wrap-site chokepoint vs. shared helper.** Both give a single source of truth; the choice hinges on whether the human-tier category is resolvable at the wrap site (`wrapAgentDispatchers`) given the approve primitive resolves category at runtime from `ctx.Params`. Step 1 settles this; the wrap-site chokepoint is preferred if feasible (auto-covers future dispatchers).
