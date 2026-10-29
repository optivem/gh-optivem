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

Decide which layer(s) to implement (see **Options** — Option A is recommended and self-sufficient), then start at Option A · Step 1. Nothing has been implemented yet; this plan is diagnosis-only.

## Options (pick layer(s) — A recommended)

The defect is preventable at three layers (defense in depth). **Option A alone fully prevents recurrence**; B and C are optional hardening.

### Option A — BPMN/interpreter: exclude skipped tests from channel-touched  ⭐ RECOMMENDED

Catches the whole class and removes both the wasted dispatch and the scope-diff. One file, well-scoped (the only caller of `executedTestNames` is `NamesInReport`, which is channel-touched-only).

- [ ] **Step 1** — In `internal/build/runner/testnames.go`, make the JUnit parser exclude testcases that carry a `<skipped/>` child. Add a `Skipped *struct{}` (xml `"skipped"`) field to `junitCase` and skip cases where it is non-nil in `namesJUnitBytes` (both the `<testsuites>` and bare `<testsuite>` branches, lines ~99-112).
- [ ] **Step 2** — Apply the equivalent exclusion to the other two formats so the semantics match: TRX (`namesTRX`, ~158-172) drop `UnitTestResult` with `outcome="NotExecuted"`; Playwright JSON (`collectPwSpecs`, ~190-197) drop specs whose result status is `skipped`. (Add the needed attrs/fields to the structs.)
- [ ] **Step 3** — Update the doc comments on `executedTestNames` (`testnames.go:13-30`) and `NamesInReport` (`tests.go:240-259`) to state explicitly that skipped testcases are excluded, and cross-reference the run-side net (`tests.go:161-181`).
- [ ] **Step 4** — Tests: extend `internal/build/runner/tests_test.go` (`TestNamesInReport_*`) and/or `testnames` coverage with a JUnit report containing a `<testcase><skipped/></testcase>` and assert the skipped name is **absent** from the union. Add the TRX `NotExecuted` and Playwright `skipped` analogues. Add a channel-level assertion in `internal/atdd/process/actions/channel_test.go` (a skipped-on-UI test → `channel-touched=false`).
- [ ] **Step 5** — Confirm `executedTestNames` has no other callers (grep), so the change stays channel-touched-scoped and does not perturb `gh optivem system-test run` counting.

### Option B — agent: `system-implementer` must not pollute the working tree

Defense-in-depth: stops the scope-diff even if a no-op channel is still dispatched for some other reason. Does **not** remove the wasted dispatch.

- [ ] **Step 1** — In `internal/atdd/assets/runtime/agents/atdd/system-implementer.md`, add explicit prohibitions: never extract dependency jars / write scratch or temp files **inside the repo** (use an OS temp dir outside the worktree, or don't); never touch `system-test/**` (test code is out of a system-implementer's scope); and on a no-op conclusion, leave the working tree **pristine** (no untracked debris).
- [ ] **Step 2** — Verify this doesn't duplicate an existing shared-chunk rule (`internal/atdd/assets/runtime/shared/scope*.md`); if a scope chunk already owns "stay within declared paths," extend it there instead of restating in the agent body.

### Option C — command/validator: scope-diff debris handling  (weakest — not recommended alone)

- [ ] **Step 1** — Evaluate (do **not** auto-implement) whether `validate-outputs-and-scopes` should distinguish untracked build/extraction *debris* from a deliberate out-of-scope *edit*. Risk: this softens the fail-loud scope guard and could mask genuine violations.
- [ ] **Note** — The human-approval gate on the `fix` process (`category:human`, `flow.txt:1541-1544`) is **intentional** — a scope violation is a serious event that warrants human eyes, and the unattended no-TTY halt is the expected rehearsal behaviour. **Do not** propose removing that gate.

## Verification (operator)

- [ ] Re-run the #76 rehearsal loop with Option A applied and confirm the UI channel routes to `CHANNEL_SKIPPED` (no UI `system-implementer` dispatched), the run reaches GREEN/commit without a scope-diff, and the worktree is left clean.

## Notes

- Halt path: `flow.txt:1529` (`failure-kind=scope-diff`, `scope-violating-paths=system-test/java/com/`) → `flow.txt:1539-1545` (`FIX → APPROVE_PRE category=human → ASK_HUMAN`, no TTY).
- The UI implementer's own self-report (003 event log) concluded *"the acceptance-ui slice is fully green … No changes to frontend-react are required"* — i.e. it correctly found nothing to do; the only working-tree delta was the `jar -xf` debris from inspecting why the tests skip.
- Channel-touched membership shipped 2026-06-19 (plan 20260619-1139, commit 78a283b); this plan closes the skipped-test gap left in that probe. The run-side twin already handles it (`tests.go:161-181`); Option A brings the report-read side to parity.
- `executedTestNames` callers: only `NamesInReport` (`tests.go:271`) and `namesJUnitDir` (same package) — confirm in Step 5 before landing.
