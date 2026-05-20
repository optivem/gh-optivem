# Plan (deferred): Align structure-cycle docs with SSoT phase-scope architecture

**RESOLVED-BY:** [plans/20260520-1053-remove-phases-deferred-by-plan.md](../20260520-1053-remove-phases-deferred-by-plan.md) — pinned scopes for the three formerly-deferred structural phases (`SYSTEM_INTERFACE_REDESIGN_CYCLE`, `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`, `SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE`) directly in `internal/atdd/phase-scopes.yaml` and removed the `PhasesDeferredByPlan` allowlist. The structure-cycle docs question (rewrite §Scope sections to reference bare layer names) is now moot at the `phase-scopes.yaml` layer — any remaining §Scope doc rewrites live with the structure-cycle doc authors, not behind a deferral allowlist.

**Date filed:** 2026-05-18

**Filed from:** [SSoT phase-scope plan (20260518-1530)](../20260518-1530-atdd-phase-scope-ssot.md), item 7's sweep-scope refinement.

## Why deferred

The SSoT phase-scope plan rewrites §Scope sections in behavior-cycle phase docs (AT-cycle, CT-cycle) to reference layer names by bare name and point at `docs/atdd/process/shared/scope.md`. It deliberately excludes the structure-cycle docs (`docs/atdd/process/change/structure/1-sir-write.md`, `2-chore-write.md`) for two reasons:

1. **Structure-cycle phases aren't user_tasks in BPMN.** In `internal/atdd/runtime/statemachine/process-flow.yaml` (lines 1100–1141), SIR and CHORE are `call_activity` invocations of a generic `structural_cycle` process parameterized by `change_type`, `agent`, and `phase_doc`. They have no per-phase `user_task` node id, so they cannot be entries in `phase-scopes.yaml`, and there's no projected runtime-prompt `scope:` frontmatter to point at.

2. **The docs are uncommitted (mid-authoring).** Both files are in `git status` (unstaged-new), being authored by the user. Editing them mid-authoring risks merge collisions.

## Soft inconsistency this leaves behind

The SSoT plan's locked decision δ retires the `${sut_namespace}` Family A placeholder. The structure docs currently use `${driver_adapter}/${sut_namespace}/<channel>` and similar substitution-style references in their §Scope and Steps sections. Once the SSoT plan lands, those `${...}` markers in `docs/atdd/process/change/structure/*.md` will reference a retired placeholder mechanism. Until this deferred plan executes, that's a soft documentation inconsistency — the docs read as if substitution still works.

The risk is contained: structure-cycle prompts don't go through the substitution machinery the SSoT plan retires (substitution was scaffold-time-only in the AT/CT flow), and `check_phase_scope` doesn't run on structure-cycle phases today. So the inconsistency is doc-only, not runtime.

## Work this plan would do

Pre-requisites for the work:
- Structure-cycle BPMN shape settled: decide whether SIR/CHORE/etc. become `user_task` phases with their own `phase-scopes.yaml` entries, or stay `call_activity` with scope expressed differently (e.g. via the `phase_doc:` param's contents).
- User's structure-doc authoring committed and stable (no more `git status` churn on `docs/atdd/process/change/structure/`).

Work itself:

1. **`docs/atdd/process/change/structure/1-sir-write.md`** — rewrite §Scope to reference bare layer names (per the chosen mechanism from the pre-req). Today: `system/`; `${driver_adapter}/${sut_namespace}/<channel>` (exceptionally `${driver_port}/${sut_namespace}/<channel>` with approval). Probable rewrite: `system_path`, `driver_adapter` (exceptionally `driver_port` with approval) — the `<channel>` qualifier handled in prose, not in the path value.

2. **`docs/atdd/process/change/structure/2-chore-write.md`** — §Scope today: `system/` only; drivers, tests, DSL, Gherkin untouched. Probable rewrite: `system_path` only.

3. **Other §Scope-bearing structure-cycle docs** as the structure cycle's design matures (new files may exist by pickup time).

4. **Sweep `${name}/${sut_namespace}/` markers in §Steps** of these docs and rewrite to bare layer names. Same shape as SSoT plan's item 7 sweep.

## Open design questions to resolve at pickup

1. **Phase identification for structure cycles.** Today the BPMN's `params: { change_type: SIR, agent: task-system-interface-redesign, phase_doc: ... }` carries all the per-cycle info. Should `phase-scopes.yaml` get entries keyed by `change_type` instead of `phase_id`? Or should structure cycles get their own `user_task` nodes per phase (effectively un-genericising `structural_cycle`)? Architecture-level decision; doesn't belong here.

2. **`<channel>` qualifier.** SIR-WRITE's `${driver_adapter}/${sut_namespace}/<channel>` includes a per-execution `<channel>` segment (api/ui/cli/...). It's not a layer name; it's a per-ticket choice. How does scope encode "the channel sub-dir matters"? Probably prose, not path.

3. **Coupling with task-and-chore-cycles.md.** That doc references structure cycles' scope from a different angle. Walk both at pickup.

## Pre-requisites

- SSoT phase-scope plan ([20260518-1530](../20260518-1530-atdd-phase-scope-ssot.md)) must have landed (defines the scope.md rule + `phase-scopes.yaml` + the doc-rewrite convention this plan extends).
- Structure-cycle authoring committed by user (`docs/atdd/process/change/structure/*.md` no longer in `git status` as unstaged-new).
- Architecture decision on structure-cycle BPMN shape (open question 1 above).

## Out of scope

- Inventing new BPMN structure for structure cycles. This plan applies SSoT to whatever BPMN shape exists at pickup.
- Asset-template parallel docs under `internal/assets/global/docs/atdd/process/` — per the AT-cycle predecessor's out-of-scope ruling, asset-template consolidation is a separate plan.

## Hand-off

Pick up when: (a) SSoT phase-scope plan has landed, AND (b) structure-cycle docs are committed/stable, AND (c) the BPMN-shape decision (open question 1) has been made.
