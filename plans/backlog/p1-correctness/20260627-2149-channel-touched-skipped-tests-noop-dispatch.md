# 2026-06-27 19:49:00 UTC — Stop the channel-touched probe counting SKIPPED tests as touched (no-op channel dispatch → scope-diff halt)

**Ticket:** #76 — Bug: Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00
**Machine:** Valentina_Desk
**Source run:** `.gh-optivem/runs/20260627-183032/` (rehearsal worktree `rehearsal-20260627-203022-76-bug-order-cancellation-blackout-on-dec`)

## TL;DR

**Why:** `GATE_CHANNEL_TOUCHED` stamped the **UI** channel as touched for a **backend-only** bug, so the orchestration dispatched a UI `system-implementer` with provably nothing to do. It flailed for 27m51s, left `jar -xf` extraction debris in the worktree (`system-test/java/com/optivem/testing/Channel.java`), and `validate-outputs-and-scopes` flagged it as `failure-kind=scope-diff` → `FIX → APPROVE_PRE (category:human) → ASK_HUMAN` → halt under no-TTY (exit 32). Root cause: the channel-touched probe reads the RED report via `NamesInReport → executedTestNames`, which counts **skipped** `<testcase>` entries as executed. The ticket's cancel tests are *skipped* on the UI channel (the `@Channel` / `alsoForFirstRow` behaviour in `optivem-testing:1.1.9`), so they never execute there — yet they made the channel read as touched.
**End result:** A channel whose tests are skipped (not executed) reads as **untouched** → routes to `CHANNEL_SKIPPED` → no implementer is dispatched. The wasted 27-min dispatch *and* the scope-diff halt are both eliminated at the source, with optional defense-in-depth so a future no-op dispatch can't pollute the tree.

## Outcomes

What we get out of this — the goals and deliverables:

- A backend-only (or any common-layer-only) fix never dispatches a per-channel `system-implementer` for a channel where the ticket's acceptance tests are skipped rather than executed.
- The channel-touched gate's report-read path (`NamesInReport`) gains the same skipped-exclusion semantics the run-side cross-channel-skip net already has (`internal/build/runner/tests.go:161-181`, the comment that literally names "the rehearsal-#76 shape").
- `executedTestNames` lives up to its name and doc ("names that **executed** at least once") across all three report formats (JUnit `<skipped/>`, TRX `outcome="NotExecuted"`, Playwright skipped specs).
- No regression for WIP-gated tests: during the RED acceptance verify `GH_OPTIVEM_RUN_WIP_TESTS=1` is set, so WIP tests appear as **executed (failing)**, not skipped — they stay counted as touched.

## ▶ Next executable step (resume here)

Option A shipped (all steps removed). Options B and C were not selected and are dropped. Only the operator verification below remains.

## Verification (operator)

- [ ] Re-run the #76 rehearsal loop with Option A applied and confirm the UI channel routes to `CHANNEL_SKIPPED` (no UI `system-implementer` dispatched), the run reaches GREEN/commit without a scope-diff, and the worktree is left clean.
  - Skipped by the unattended executor: a full shop rehearsal is not feasible unattended. Unit coverage (`TestExecutedTestNames_ExcludesSkipped`, `TestNamesInReport_SkippedTestsAreNotPresent`, `TestResolveChannel_SkippedOnChannel_Untouched`) passes.

## Notes

- Halt path: `flow.txt:1529` (`failure-kind=scope-diff`, `scope-violating-paths=system-test/java/com/`) → `flow.txt:1539-1545` (`FIX → APPROVE_PRE category=human → ASK_HUMAN`, no TTY).
- The UI implementer's own self-report (003 event log) concluded *"the acceptance-ui slice is fully green … No changes to frontend-react are required"* — i.e. it correctly found nothing to do; the only working-tree delta was the `jar -xf` debris from inspecting why the tests skip.
- Channel-touched membership shipped 2026-06-19 (plan 20260619-1139, commit 78a283b); this plan closes the skipped-test gap left in that probe. The run-side twin already handles it (`tests.go:161-181`); Option A brings the report-read side to parity.
- Step 5 finding: `executedTestNames` is ALSO called by `runOneSuite` (named-run presence check), not only `NamesInReport`. Skipped names are now excluded there too: a requested test skipped in every selected suite now fails loud as "never executed" (previously it silently passed); a test skipped on one channel but executed on another is unaffected. Count semantics (`countExecutedTests`) were already skipped-exclusive, so the two are now consistent.
