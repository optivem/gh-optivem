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

### 1. Draft `internal/assets/global/docs/atdd/process/change/structure/external-system-interface-redesign.md`

Mirror the structure of the live `change/structure/system-interface-redesign.md` but retarget at the external boundary.

**Header & scope:**

```
# EXTERNAL SYSTEM INTERFACE REDESIGN - WRITE

Reshape the external-system driver layer to match the new external API.
The Real driver + Stub driver(s) + Ext* DTOs absorb the change so DSL,
Gherkin, and tests stay untouched.

## Scope

`${external_driver_adapter}/<external-system>/...` (exceptionally
`${external_driver_port}/<external-system>/...` with approval).
```

**Steps (mirror SIR's 5-step structure):**

1. Identify the external system from the ticket Checklist; locate its driver components: `XyzRealDriver`, `XyzStubDriver` (one per stub variant), `BaseXyzClient`, `Ext*` DTOs.
2. Update `Ext*` DTOs to match the new external surface.
3. Update the Real driver impl (`${external_driver_adapter}/.../XyzRealDriver`) to consume the new surface. Apply across **all parallel implementations** (Java/.NET/TS × monolith/multitier — see [architecture/external-driver-adapter.md](../../../architecture/external-driver-adapter.md)).
4. Update the Stub driver impl(s) to mirror the new surface so stubs stay consistent with reality.
5. **External driver port guardrail.** Do NOT modify `${external_driver_port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the Real/Stub adapters alone cannot absorb the change, the proposed new signature(s), and the explicit warning that this WILL require contract-test updates (CT sub-process gets invoked for affected scenarios). Wait for explicit user approval before editing any `${external_driver_port}/` file.
6. Do not modify acceptance tests, DSL, Gherkin, or any code outside the external-system layer + its driver. `${system_test_path}/.../Legacy/` is read-only.

**Verify against:**

- Live `change/structure/system-interface-redesign.md` — quality bar + step shape.
- Live `task-external-system-interface-redesign.md` prompt — terminology (XyzRealDriver, XyzStubDriver, BaseXyzClient, Ext* DTOs).
- `internal/assets/global/docs/atdd/architecture/external-driver-adapter.md` — boundary terminology (confirm path exists before linking).

### 2. Verify the path placeholders are wired

`${external_driver_adapter}` and `${external_driver_port}` are two of the seven Family B keys (per `internal/atdd/phase-scopes.yaml` + `CanonicalPathKeys`). Confirm both substitute correctly in materialized output by:

- Running `gh optivem sync` against a scaffolded test project that has external driver paths configured.
- Inspecting `./.gh-optivem/docs/atdd/process/change/structure/external-system-interface-redesign.md` for substituted values.

### 3. (Optional) Cross-link from SIR doc

If the SIR doc's step 4 parenthetical (*"...this is `${sut_namespace}/`, not `external/`..."*) would benefit from a `→ see external-system-interface-redesign.md` link, add it. Skip if the parenthetical reads fine standalone.

### 4. Verify the BPMN + prompt reference resolves

After the file exists and after plan 20260519-0922 lands:

- `process-flow.yaml:1108` should read `docs/atdd/process/change/structure/external-system-interface-redesign.md`.
- `task-external-system-interface-redesign.md:21` should read `${docs_root}/atdd/process/change/structure/external-system-interface-redesign.md`.
- Run a phase-render smoke (one of the test fixtures in `internal/atdd/runtime/clauderun/clauderun_test.go`) targeted at the ESIR phase and confirm the materialized prompt references an existing file.

### 5. Run build + tests

- `go build ./...`
- `go test ./internal/atdd/... ./internal/assets/... -p 2` (per `[[feedback_go_test_windows]]`)

## Hand-off dependencies

- **Land at or before plan 20260519-0922's execute.** If 0911 lands first: stable, references resolve cleanly. If 0922 lands first: the new file is missing until 0911 lands; runtime breaks for ESIR-cycle dispatches in that window. Ideally co-land.
- **No dependency on plan 20260519-0704** — that plan touches naming of the ESIR agent, not the phase doc. Independent.

## What this plan does NOT do

- Does NOT correct the archived `cycles.md` / `task-and-chore-cycles.md` ESIR error inline — archive is scheduled for deletion in plan 20260519-0922 Item 6.
- Does NOT touch BPMN structure — that's set by the runtime; this plan just authors a doc that matches what the runtime already does.
- Does NOT modify any test fixture or runtime code — pure new-doc authoring.
