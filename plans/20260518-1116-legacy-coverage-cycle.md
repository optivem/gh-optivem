# Plan: legacy coverage cycle

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-18T11:25:19Z`

> ⚠️ **NOT YET REFINED** — this plan was promoted out of [Part 1's item 5 discussion](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) without per-item refinement. Run `/refine-plan` on this file before `/execute-plan`.

**Date:** 2026-05-18 (split from the AT-cycle Part 1 plan during refinement)
**Context:** Defines the **legacy coverage cycle** as a top-level sibling of the AT cycle (peer to the structural cycle; CT remains a sub-cycle of AT). Triggered by **legacy acceptance criteria** in a ticket — retroactively writes acceptance tests (and external-system contract tests) for already-existing behaviour that lacks coverage. **Inverted RED-GREEN shape:** tests should **pass on first run** (the behaviour already exists); if they don't, the test is probably wrong and needs revision. No code-writing phase.

**Sibling plans referenced:**
- [Part 1 — Cycle architecture & §Conventions](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) — defines §Conventions (disable-reason, phase-output flags, phase scope policy). This plan extends them.
- [Part 2 — `atdd-at-cycle.md` per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — independent doc-content work for the AT cycle.

## Scope

1. **Define `docs/legacy-coverage-cycle.md`** — canonical prose home for the legacy cycle process spec, parallel to `atdd-at-cycle.md` and `atdd-ct-cycle.md`:
   - Cycle shape (test → expect pass; if fails, revise test).
   - Phases (or absence of fail-first RED).
   - Behavioural expectations and escalation rules.
   - Relationship to AT and CT cycles (sibling at the top level; AT-RED-TEST encounters legacy tests but does not author them).

   **BPMN counterpart (must land in parallel with the doc).** The `LEGACY_CYCLE` sub-process already exists in `internal/atdd/runtime/statemachine/process-flow.yaml` (rendered to `docs/process-diagram.md` § *Legacy Acceptance Criteria Cycle*) but is currently a **STOP — HUMAN REVIEW** TBD stub. As part of this plan, that stub must be fleshed out in `process-flow.yaml` to match the prose spec, the same way AT and CT have parallel BPMN + prose surfaces. Tracked as a Phase 7 BPMN dependency — see item 4.

   > **Refined 2026-05-18:** Reframed from "create a doc" → "create a prose doc *parallel* to atdd-at-cycle.md/atdd-ct-cycle.md, AND flesh out the BPMN `LEGACY_CYCLE` sub-process in parallel". **Why:** the user clarified that the legacy cycle should follow the established pattern — every cycle has both a BPMN representation in `process-flow.yaml` and a prose doc; legacy already has a BPMN stub but no flesh, and was missing the prose doc entirely. The BPMN side was previously invisible in this plan (item 4 covers only the failing-legacy *detector*, not fleshing out the cycle body), so it gets surfaced here and tracked as a Phase 7 BPMN dependency.

2. **§Conventions tightening** (extends Part 1):
   - **Disable-reason convention** — explicit domain restriction: "applies only to change-driven scenarios; **never** to legacy. The re-enable filter must not match legacy markers."
   - **Legacy marker convention** (new schema) — annotation / naming / location convention so legacy intent is unambiguous and machine-checkable by the BPMN failing-legacy detector. To be designed in this plan.

3. **AT-side updates to `atdd-at-cycle.md`** (lands as part of this plan, once the legacy cycle exists and the marker convention is defined):
   - **Boundary statement** near the top of the doc: *"This is the behavioural cycle, triggered by change-driven acceptance criteria. Other top-level cycles dispatched alongside AT: structural (refactors) and legacy (retroactive coverage of legacy acceptance criteria). CT (Contract Test) is a sub-cycle of AT — see the External System Driver section below."*
   - **"Failing legacy = STOP, never @Disabled" guardrail** — because legacy tests authored by prior legacy-cycle runs will be present and passing in the test class during AT-RED-TEST. If any legacy test fails during the AT cycle, the AT-RED-* agents must escalate to user, never `@Disabled`. A failing legacy = real regression.

4. **BPMN failing-legacy detector** — cross-plan reference to [Part 1's Phase 7 BPMN orchestration bullet](20260516-1701-atdd-at-cycle-absorb-internal-assets.md). Part 1's bullet already mentions the detector; this plan supplies the marker convention it depends on.

## Out of scope

- Legacy cycle's own §Conventions schema beyond disable-reason tightening + marker (e.g. legacy cycle's own scope policy rows — handled when the cycle doc itself is fleshed out).
- Implementing the BPMN failing-legacy detector — orchestration code lives in the BPMN-orchestration plan signposted in [Part 1 Phase 7](20260516-1701-atdd-at-cycle-absorb-internal-assets.md).
- Structural cycle definition and dispatcher/router — signposted in [Part 1 Phase 7](20260516-1701-atdd-at-cycle-absorb-internal-assets.md).

## Open questions

- What's the simplest marker convention for legacy tests? Annotation (`@LegacyCoverage`), naming (`*_LegacyTest`), or directory (`tests/legacy/`)? Each has tradeoffs for the BPMN detector.
- Does the legacy cycle have its own scope-policy row (per §Conventions → Phase scope policy in Part 1)? Probably yes — same shape, different allowed-paths.
- Symmetric question for legacy *contract* tests: same cycle, just different test layer? Or its own sub-cycle within legacy?
