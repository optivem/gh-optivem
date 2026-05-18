# Plan: bring `docs/atdd/at-cycle.md` to parity with internal/assets — Part 1: Cycle architecture & §Conventions

**Date:** 2026-05-16 (split into Part 1 / Part 2 / Legacy on 2026-05-18 during refinement)
**Context:** The goal is to eventually delete `internal/assets/`. `docs/atdd/at-cycle.md` is intended to become the canonical home for the AT cycle process spec, replacing the four global process pages under `internal/assets/global/docs/atdd/process/at-{red,green}-*.md`.

This is **Part 1** of three sibling plans created during refinement on 2026-05-18:

- **Part 1 (this file):** Cycle architecture and §Conventions. Establishes the normative schemas (disable-reason, phase-output flags, phase scope policy) and the doc-side items (1–4b) that wire them into `atdd-at-cycle.md`. Self-contained; can execute first.
- **[Part 2 — per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md):** Phases 2–6 of the original plan (per-phase rules, framing, examples, cross-refs, mechanical fixes). Independent of Part 1; can run in parallel or after. **Not yet refined.**
- **[Legacy coverage cycle plan](20260518-1116-legacy-coverage-cycle.md):** Defines `docs/legacy-coverage-cycle.md` (sibling top-level cycle to AT) and its AT-side updates (boundary statement + "failing legacy = STOP" guardrail). Defines the legacy marker convention that extends §Conventions. **Not yet refined.**

**Source:** Gap analysis in [reports/atdd-at-cycle-gap-analysis.md](../reports/atdd-at-cycle-gap-analysis.md).

## Phase 7 — NOT in this file (flagged as related work)

- **BPMN orchestration work** (dependency for items 1, 2, 4b — and items in [Legacy plan](20260518-1116-legacy-coverage-cycle.md)): the BPMN ATDD process needs new pieces that the doc reframes assume exist:
  - **Disable/enable steps around the commit** (item 1) — mark change-driven tests `@Disabled` after each RED sub-phase per §Conventions → *Disable-reason convention*, and re-enable them by ticket-prefix at the start of the next phase. Cheap implementation (scripted, or Haiku at most).
  - **Post-RED-DSL gateway** (item 2) — validate both flags from §Conventions → *Phase-output flags* are set (error if unset), then branch onto RED-SYSTEM-DRIVER and/or the CT cycle based on their values.
  - **Post-phase scope check** (item 4b — and any phase with a scope rule) — after each phase agent finishes, diff modified files against §Conventions → *Phase scope policy*; on violation, halt and prompt the user with the four options (Accept / Rewind to upstream phase / Revert + rerun / Abort). Pure scripted check (no LLM); user prompt is BPMN's standard human-task pattern.
  - **Failing-legacy detector** (defined in [Legacy plan](20260518-1116-legacy-coverage-cycle.md)) — same shared sub-process pattern as the scope check.
  - **Shared "Run Phase Agent" call activity** wrapping all of the above so every phase reuses the same envelope (load scope → inject into prompt → run agent → handle scope-exception signal → post-phase scope check → post-phase legacy check → escalation).

  Per the standing "new plan, never extend an existing one" rule, this is tracked in a separate plan file: `plans/<YYYYMMDD>-<HHMM>-atdd-bpmn-orchestration.md` (to be drafted), cross-referenced from here.

  > **Refined 2026-05-18:** Bullet added (item 1), then extended for the post-RED-DSL gateway (item 2), then again for the post-phase scope check, then again for the shared call-activity envelope. **Why:** each doc reframe introduced a real BPMN dependency this plan would otherwise leave invisible; grouping all the orchestration work under one BPMN plan keeps it coherent. The scope check is the enforcement layer for §Conventions → *Phase scope policy*; the Rewind-to-upstream-phase escalation preserves the per-phase RED guarantee when a downstream phase reveals an upstream bug; the shared call activity means adding a new phase = adding a §Conventions row + one BPMN call, with consistency by construction.

- **Sibling top-level cycles** (router-dispatched alongside AT; each gets its own plan file):
  - **[Legacy Coverage Cycle](20260518-1116-legacy-coverage-cycle.md)** — backfills retroactive acceptance tests (and external-system contract tests) driven by **legacy acceptance criteria** in the ticket. **Inverted RED-GREEN shape:** tests should pass on first run (the behaviour already exists); if they don't, the test is probably wrong → revise. Plan file created; not yet refined.
  - **Structural Cycle** — refactor / restructure work with no behavioural change. "Behaviour preserved" is the gate; no fail-first RED. Plan file: `plans/<YYYYMMDD>-<HHMM>-structural-cycle.md` (to be drafted).
  - **Cycle Router / Dispatcher** — upstream BPMN step that reads the ticket's acceptance criteria and dispatches to the appropriate top-level cycle(s). A single ticket may route to multiple cycles concurrently or sequentially (e.g. legacy AC + change-driven AC). Plan file: `plans/<YYYYMMDD>-<HHMM>-cycle-router.md` (to be drafted).

  > **Refined 2026-05-18:** New bundle. **Why:** the AT cycle is one of N top-level cycles, not the only one. The router dispatches by acceptance-criteria type (change-driven → AT, legacy → Legacy, refactor → Structural; CT remains a sub-cycle of AT, invoked from AT-RED-EXTERNAL-SYSTEM-DRIVER). Each sibling cycle deserves its own plan; signposting them here prevents their existence from being forgotten as we focus on AT.

- **CT-cycle parity work** (sub-cycle of AT, invoked from AT-RED-EXTERNAL-SYSTEM-DRIVER): `docs/atdd/ct-cycle.md` likely has the same gaps vs its four internal CT pages (ct-red-test, ct-red-dsl, ct-red-external-driver, ct-green-stubs). Worth a parallel gap analysis.

- **Runtime prompt content** (compile-fix retry policy, batch-edits hint, "no approval inside agent", model/effort): these are agent-operational, not process-spec. They belong in the prompt files. The prompt files themselves are a separate migration concern — if `internal/assets/runtime/` is going away, those need a new generation mechanism, not relocation into `docs/`.

- **Supporting docs migration** (architecture/, language-equivalents/, glossary.md, testkit-*, placeholders.md, cycles.md, task-and-chore-cycles.md, system-interface-redesign.md, diagram-phase-details.md): 22 files in `internal/assets/global/docs/atdd/` that aren't process pages. Each needs a `docs/` home decided before internal/assets can be deleted.
