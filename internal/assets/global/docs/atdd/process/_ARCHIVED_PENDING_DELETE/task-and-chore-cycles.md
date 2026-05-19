# Structural Cycle Mechanics

## Purpose

This doc defines the **WRITE** mechanics for the structural cycles triggered by the three `subtype:*` labels on Task tickets: `subtype:system-interface-redesign`, `subtype:external-system-interface-redesign`, and `subtype:system-implementation-change`. Cycle-level placement (which cycle dispatches when, and what comes after) is owned by the Go runtime in `gh-optivem` (canonical YAML embedded in the binary; see the rendered [process-flow diagram](https://github.com/optivem/gh-optivem/blob/main/docs/process-flow-diagram.md)); see also [cycles.md](cycles.md). This file is the substance of what happens *inside* each phase.

It mirrors the role of the AT per-phase docs (`at-red-test.md`, `at-red-dsl.md`, `at-red-system-driver.md`, `at-green-system.md`) and the CT per-phase docs (`ct-red-test.md`, `ct-red-dsl.md`, `ct-red-external-driver.md`, `ct-green-stubs.md`) for behavioral cycles.

---

## SYSTEM INTERFACE REDESIGN

WRITE / REVIEW mechanics live in [system-interface-redesign.md](system-interface-redesign.md). The cycle's commit-message phase suffix is `SYSTEM INTERFACE REDESIGN`.

---

## EXTERNAL SYSTEM INTERFACE REDESIGN

### Purpose

The `subtype:external-system-interface-redesign` path has **NO standalone WRITE** — it routes entirely through the **Contract Test Sub-Process**. See [ct-red-test.md](ct-red-test.md), [ct-red-dsl.md](ct-red-dsl.md), [ct-red-external-driver.md](ct-red-external-driver.md), and [ct-green-stubs.md](ct-green-stubs.md) for the per-phase mechanics.

### Anti-patterns

- Bypassing the CT sub-process by touching the External Driver directly without going through `CT - RED - EXTERNAL DRIVER`.

---

## CHORE

This is the WRITE phase of `subtype:system-implementation-change` — the cycle still exists, just under the runtime name `sut_cycle` rather than a separate ticket-type cycle. The phase suffix on the commit message is still `CHORE`.

### Purpose

Refactor / rename / move / dependency upgrade / build tweak / dead-code removal / internal abstraction change inside `system/`, with no boundary or behavioral impact. By definition, a `subtype:system-implementation-change` ticket does not change drivers, tests, DSL, or Gherkin; if it does, it has been misclassified.

### What it produces

After WRITE: only `system/` edits; drivers, tests, DSL, Gherkin untouched.

### CHORE - WRITE

**Goal:** the structural change is implemented inside `system/`; drivers and tests are untouched; existing acceptance and contract tests still compile.

1. Implement the change as described in the ticket's checklist of refactor / upgrade steps.
2. Drivers — interfaces (`driver-port/`) and implementations (`driver-adapter/`) — are untouched. If the work turns out to require driver changes, STOP and reclassify the ticket — `subtype:system-implementation-change` by definition does not change boundaries; relabel as `subtype:system-interface-redesign` (or `subtype:external-system-interface-redesign` for an external-system change).
3. Tests, DSL, and Gherkin are untouched. If the work turns out to require behavioral test changes, STOP and reclassify the ticket as a Story or Bug.

### Anti-patterns

- Driver / test / DSL changes (`subtype:system-implementation-change` by definition does not change boundaries — STOP and relabel as `subtype:system-interface-redesign`).
- Behavioral test changes (likewise — STOP and reclassify as a Story or Bug).
- Bundling unrelated refactors into one commit; keep the diff scoped to the ticket's checklist.
