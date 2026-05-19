# Plan: author `change/structure/external-system-interface-redesign.md` (ESIR WRITE phase doc)

**Date:** 2026-05-19

## Context

The new hierarchical `internal/assets/global/docs/atdd/process/` tree has `change/structure/system-interface-redesign.md` and `change/structure/system-implementation-change.md`, but no ESIR sibling. The BPMN (`internal/atdd/runtime/statemachine/process-flow.yaml:1102-1118`) treats ESIR as its own `structural_cycle` call_activity with its own `phase_doc:`, runs WRITE → REVIEW → TEST → COMMIT → DA_END, and does **not** route through CT automatically. The runtime prompt `internal/assets/runtime/prompts/atdd/task-external-system-interface-redesign.md` is the agent for that WRITE phase.

At present that agent prompt reads SIR's doc as a stopgap — the ESIR phase doc was never authored separately from SIR. This plan authors it.

**Sibling / coordinated plans:**

- [Rewire BPMN runtime + agent prompts to the new `process/` hierarchy (20260519-0922)](20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md) — Q1 resolution spun out this plan. That plan's Item 2 row at `process-flow.yaml:1108` and Item 3 row at `task-external-system-interface-redesign.md:21` forward-reference `change/structure/external-system-interface-redesign.md`. **This plan must land at or before that plan's execute** so those references resolve to an existing file. (If 0922 lands first, ESIR materializes to a missing file until 0911 lands — acceptable transitional state but ideally avoided.)
- [BPMN external-system naming consistency (20260519-0704)](20260519-0704-bpmn-external-system-naming-consistency.md) — touches the ESIR agent name; coordinate ordering if landing simultaneously.

## Doctrinal note

The archived `cycles.md` (in `_ARCHIVED_PENDING_DELETE/`) carries an error claiming ESIR "routes entirely through the Contract Test Sub-Process" with "no standalone WRITE" — see `cycles.md:50, 204-208, 258` and `task-and-chore-cycles.md:17-21`. The BPMN is the live source of truth; the archived claim is stale. This plan authors the doc per the BPMN's treatment (standalone WRITE phase). The archive is scheduled for deletion in plan 20260519-0922 Item 6, so the stale claim self-resolves.

## Items

- [ ] **Item 2: Verify the path placeholders are wired** — ⏳ Deferred: requires a scaffolded test project with external driver paths configured to run `gh optivem sync` against. Placeholder vocabulary (`${external_driver_adapter}`, `${external_driver_port}`) is confirmed present in `internal/atdd/phase-scopes.yaml` (lines 34–36), but end-to-end substitution into materialized output is out of scope for this session. Pick up once a scaffolded ESIR-bearing fixture project is available.

- [ ] **Item 4: Verify the BPMN + prompt reference resolves** — ⏳ Deferred: explicitly conditional on plan [20260519-0922](20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md) landing. 0922 is still in `plans/` (not executed); until its Items 2/3 land, `process-flow.yaml:1108` and `task-external-system-interface-redesign.md:21` still point at the old flat path. Pick up immediately after 0922's execute completes — the file authored in Item 1 above is now in place, so the references will resolve cleanly the moment 0922 rewrites them.

## Hand-off dependencies

- **Land at or before plan 20260519-0922's execute.** If 0911 lands first: stable, references resolve cleanly. If 0922 lands first: the new file is missing until 0911 lands; runtime breaks for ESIR-cycle dispatches in that window. Ideally co-land.
- **No dependency on plan 20260519-0704** — that plan touches naming of the ESIR agent, not the phase doc. Independent.

## What this plan does NOT do

- Does NOT correct the archived `cycles.md` / `task-and-chore-cycles.md` ESIR error inline — archive is scheduled for deletion in plan 20260519-0922 Item 6.
- Does NOT touch BPMN structure — that's set by the runtime; this plan just authors a doc that matches what the runtime already does.
- Does NOT modify any test fixture or runtime code — pure new-doc authoring.
