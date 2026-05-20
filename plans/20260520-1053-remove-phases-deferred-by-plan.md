# Plan: remove the `PhasesDeferredByPlan` mechanism

**Date:** 2026-05-20 10:53 UTC

## Purpose

`internal/atdd/phase_scopes.go` carries a `PhasesDeferredByPlan` map: an
allowlist of writing-agent phase ids that knowingly have **no scope
declared** in `phase-scopes.yaml`, each citing a deferred plan that's
supposed to pin the scope later. At runtime, `checkPhaseScope`
short-circuits for allowlisted phases (logs `"deferred per <plan>"` to
stderr, marks the phase scope-clean, runs nothing).

Per the user's doctrine (see memory `feedback-no-deferred-mechanism`),
this allowlist is a workaround the project does not want. Every
writing-agent phase must have its scope **explicitly pinned** in
`phase-scopes.yaml` — no "skip scope-check until a future plan resolves
it" band-aid.

The current allowlist has 3 entries:

- `SYSTEM_INTERFACE_REDESIGN_CYCLE`           → `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`
- `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`  → `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`
- `SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE`   → `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`

This plan pins scopes for those three, removes the allowlist + its
runtime short-circuit + its tests + its build-time drift guards, and
closes the deferred structure-cycle-ssot-alignment plan.

## Items to walk

Items are stubs — refine with `/refine-plan` before executing. Items 1,
2, 3 each require an explicit scope-doctrine decision from the user
(which layer partition applies to each cycle).

### Item 1 — Pin scope for `SYSTEM_INTERFACE_REDESIGN_CYCLE`

**Scope:** `[system_path, driver_adapter]`

Grounding: the BPMN node (`process-flow.yaml:1285-1292`) invokes
`structural_cycle` with agent `task-system-interface-redesign`. That
prompt edits the system surface under `system/` (→ `system_path`,
Family A) and the System Driver adapter(s) under
`${driver_adapter}/<channel>` (→ `driver_adapter`, Family B).

`driver_port` is **deliberately excluded.** The prompt's Step-4
guardrail says do NOT modify `${driver_port}/` casually — if an
interface change is unavoidable, STOP and get explicit user approval
first. Treat that as escalation, not normal scope: the cycle's
allowed-write set is `[system_path, driver_adapter]`, and a driver-port
change is a separate, user-approved scope amendment when it happens.

### Item 2 — Pin scope for `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`

**Scope:** `[external_system_driver_adapter]`

Mirror of Item 1 on the external-system side. The BPMN node
(`process-flow.yaml:1294-1301`) invokes `structural_cycle` with agent
`task-external-system-interface-redesign`. That prompt edits the
external-system driver layer under
`${external_system_driver_adapter}/<external-system>/` (Real driver,
Stub driver, Base client, Ext* DTOs) → `external_system_driver_adapter`.

`external_system_driver_port` is **deliberately excluded** by the same
reasoning as Item 1: the prompt's Step-5 guardrail makes any port
change a user-approved escalation (and explicitly warns it requires
contract-test updates), not normal scope. Unlike Item 1, `system_path`
is not in scope — this cycle does not touch the system itself, only its
external-system driver layer.

### Item 3 — Pin scope for `SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE`

**Scope:** `[system_path]`

(The plan originally called this `CHORE_CYCLE`; that was stale — the
node was renamed to `SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE` per
`plans/20260520-1145-system-implementation-refactoring-rename.md`.)

Grounding: the BPMN node (`process-flow.yaml:1318-1333`) is the only
node in `sut_cycle`, invoking `structural_cycle` with agent
`task-system-implementation-refactoring`. That prompt is explicit:
`system/` only — drivers (port + adapter), tests, DSL, and Gherkin are
untouched.

Guardrail is stronger than Items 1/2: if the work turns out to need a
driver or test change, the agent STOPs and **reclassifies the
ticket** as `system-interface-redesign` (or escalates to Story/Bug for
test work). That is a ticket-misclassification escape, not an
escalate-and-amend-scope path — so the layer list stays minimal.

### Item 4 — Add the three rows to `phase-scopes.yaml`

Append a new section under the existing AT and CT blocks:

```yaml
  # ---- Structural cycles --------------------------------------------
  SYSTEM_INTERFACE_REDESIGN_CYCLE:          [system_path, driver_adapter]
  EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE: [external_system_driver_adapter]
  SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE:  [system_path]
```

Verify against `TestPhaseScopes_LayersAreCanonical` and
`TestPhaseScopes_NonEmptyLayerLists` (every layer must be in
`CanonicalPathKeys()` or `FamilyAPathKeysInScope` — `system_path`,
`driver_adapter`, and `external_system_driver_adapter` all qualify).

### Item 5 — Remove `PhasesDeferredByPlan` from `phase_scopes.go`

Delete the map (`internal/atdd/phase_scopes.go:30-41`). Then remove
every caller. The full list (`grep -rn PhasesDeferredByPlan`):

- `internal/atdd/runtime/actions/bindings.go` — `checkPhaseScope`
  allowlist short-circuit (the path that emits `"deferred per <plan>"`
  to stderr). Delete the `if plan, deferred := atdd.PhasesDeferredByPlan[phaseID]; deferred { … }`
  block at `bindings.go:886-905` and fix the error message at line 914
  (drop the `"and not in PhasesDeferredByPlan"` clause).
- `internal/atdd/phase_scopes_test.go` —
  `TestPhaseScopes_ReverseFK_WritingAgentsScopedOrAllowlisted` (line 100):
  drop the `inAllowlist` branch and **rename to**
  `TestPhaseScopes_ReverseFK_WritingAgentsScoped`. Phases must be in
  `phase-scopes.yaml`, full stop.
- `internal/atdd/phase_scopes_test.go` —
  `TestPhaseScopes_AllowlistEntriesStillExistInBPMN` (line 118):
  delete entirely.
- `internal/atdd/runtime/process_commands.go` — three call sites:
  - `printAllPhases` (line 159): delete the deferred-section emission
    (lines 173-185, including the "Deferred — scope not yet declared"
    header); function now only prints phase-scopes entries. Update the
    function doc comment at line 159-160 to match.
  - `printOnePhase` (line 198-203): delete the `PhasesDeferredByPlan`
    fallback block.
  - `printOnePhase` final error (line 204): drop the
    `"or PhasesDeferredByPlan"` clause from the hint.
- `internal/atdd/runtime/process_commands_test.go` —
  `TestProcessScope_AllPhases_NoProject` (around line 39): delete the
  `for phaseID := range atdd.PhasesDeferredByPlan` loop and the
  `"Deferred — scope not yet declared"` header assertion (lines 39-46).

### Item 6 — Remove the runtime test of the allowlist short-circuit

`internal/atdd/runtime/actions/bindings_test.go` —
`TestCheckPhaseScope_AllowlistedPhaseIsNoop` (lines 1776-1791) tests
a mechanism this plan removes. Delete the test.

### Item 7 — Close the deferred plan

`plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md` —
add a `RESOLVED-BY` header citing this plan. Leave the body intact.

Also sweep the one residual `${sut_namespace}` reference in
`internal/assets/runtime/prompts/atdd/task-system-interface-redesign.md:23`
(the deferred plan's Step 4 leftover). The current line reads:

> Examples: `${sut_namespace}/api`, `${sut_namespace}/ui`,
> `${sut_namespace}/mobile`, `${sut_namespace}/cli`,
> `${sut_namespace}/admin`.

Rewrite by dropping the `${sut_namespace}/` prefix; keep the channel
categories as discovery hints:

> Examples: `api`, `ui`, `mobile`, `cli`, `admin`.

This is a doctrinal cleanup, not a runtime fix —
`cfg.PlaceholderMap()` (see `driver.go:789-798`) still includes
`sut_namespace`, so the line currently expands correctly at runtime.
The rewrite removes a reference to a placeholder the SSoT plan's
decision δ retired.

Other parallel docs were inlined into prompts in commit `4b44722`; a
fresh grep confirms `internal/assets/runtime/prompts/` has no other
`${sut_namespace}` references, so no further sweep is needed. The
remaining matches across the codebase (`config.go`, `driver_test.go`,
`clauderun_test.go`, `reports/atdd-at-cycle-gap-analysis.md`) are
internal doc comments / analysis notes describing the substitution
mechanism itself, not user-facing prompts.

## Sequencing (within this plan)

Items 1–3 (scope decisions) gate Item 4 (apply rows). Items 5, 6, 7
follow once 4 lands. Items 5 and 6 must land together with 4 (the
runtime can't lose the allowlist branch before the surviving rows have
their scopes pinned, or `checkPhaseScope` will hard-error on the three
formerly-allowlisted phases).

## Sequencing (vs other in-flight plans)

- **Possibly upstream of:** any future plan that touches the
  structure-cycle SSoT alignment (this plan closes the deferred plan
  those would otherwise inherit).
