# Plan: remove the `PhasesDeferredByPlan` mechanism

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-20T11:00:02Z`

**Date:** 2026-05-20 10:53 UTC
**Cross-references:**
- `plans/20260520-1213-collapse-at-green-backend-frontend.md` — upstream plan
  that removes the `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` entries (Item 4
  in that plan) and surfaced the doctrinal preference that drives this one.

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

After the AT_GREEN collapse plan lands, 3 entries remain:

- `SYSTEM_INTERFACE_REDESIGN_CYCLE`         → `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`
- `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE` → `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`
- `CHORE_CYCLE`                              → `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`

This plan pins scopes for those three, removes the allowlist + its
runtime short-circuit + its tests + its build-time drift guards, and
closes the deferred structure-cycle-ssot-alignment plan.

## Out of scope (deliberately)

- **AT_GREEN_BACKEND / AT_GREEN_FRONTEND removal** — already handled by
  `plans/20260520-1213-collapse-at-green-backend-frontend.md` Item 4.
  This plan starts from the post-collapse state where only the three
  structure-cycle entries remain.

## Items to walk

Items are stubs — refine with `/refine-plan` before executing. Items 1,
2, 3 each require an explicit scope-doctrine decision from the user
(which layer partition applies to each cycle).

### Item 1 — Pin scope for `SYSTEM_INTERFACE_REDESIGN_CYCLE`

What writing-agent paths does this cycle's agent operate on? The cycle's
name suggests system-internal interface refactoring; the canonical layer
keys to choose from live in `internal/projectconfig/paths_defaults.go`
`CanonicalPathKeys()` (Family B) plus `system_path` (Family A).

**Decision needed from user.** Read the cycle's BPMN definition in
`internal/atdd/runtime/statemachine/process-flow.yaml` first to ground
the question, then ASK — do not infer.

### Item 2 — Pin scope for `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`

Same shape as Item 1, for the external-system variant. **Decision
needed from user.**

### Item 3 — Pin scope for `CHORE_CYCLE`

Same shape. Note: chore cycles may be intentionally scope-broad
(anywhere in the repo), in which case the canonical-layer set may not
fit and a new doctrinal layer may be needed. **Decision needed from
user.**

### Item 4 — Add the three rows to `phase-scopes.yaml`

Once Items 1–3 settle, add the three rows. Verify against
`TestPhaseScopes_LayersAreCanonical` and `TestPhaseScopes_NonEmptyLayerLists`.

### Item 5 — Remove `PhasesDeferredByPlan` from `phase_scopes.go`

Delete the map (lines 37-43 today). Search for callers of `PhasesDeferredByPlan`
and remove them — at minimum:

- `internal/atdd/runtime/actions/bindings.go` — `checkPhaseScope`
  allowlist short-circuit (the path that emits `"deferred per <plan>"`
  to stderr).
- `internal/atdd/phase_scopes_test.go` —
  `TestPhaseScopes_ReverseFK_WritingAgentsScopedOrAllowlisted` (line 100)
  must drop the `inAllowlist` branch; phases must be in `phase-scopes.yaml`
  full stop.
- `internal/atdd/phase_scopes_test.go` —
  `TestPhaseScopes_AllowlistEntriesStillExistInBPMN` (line 118) is
  deleted entirely.

### Item 6 — Remove the runtime test of the allowlist short-circuit

`internal/atdd/runtime/actions/bindings_test.go` —
`TestCheckPhaseScope_AllowlistedPhaseIsNoop` (around line 1760-1775)
tests a mechanism this plan removes. Delete the test.

This is the test that
`plans/20260520-1213-collapse-at-green-backend-frontend.md` Item 7
cross-references — that plan's Item 7 becomes a no-op once this plan
lands first.

### Item 7 — Close the deferred plan

`plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md` —
add a `RESOLVED-BY` header citing this plan. Leave the body intact.

## Sequencing (within this plan)

Items 1–3 (scope decisions) gate Item 4 (apply rows). Items 5, 6, 7
follow once 4 lands. Items 5 and 6 must land together with 4 (the
runtime can't lose the allowlist branch before the surviving rows have
their scopes pinned, or `checkPhaseScope` will hard-error on the three
formerly-allowlisted phases).

## Sequencing (vs other in-flight plans)

- **Downstream of:** `plans/20260520-1213-collapse-at-green-backend-frontend.md`.
  That plan must land first so this plan starts from a 3-entry allowlist,
  not a 5-entry one.
- **Possibly upstream of:** any future plan that touches the
  structure-cycle SSoT alignment (this plan closes the deferred plan
  those would otherwise inherit).
