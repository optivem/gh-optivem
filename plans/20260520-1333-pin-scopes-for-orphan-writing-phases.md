# Plan: pin scopes for 4 orphan writing-agent phases

**Date:** 2026-05-20 13:33 UTC

## Why

`TestPhaseScopes_ReverseFK_WritingAgentsScoped` (renamed by
[plan 20260520-1053](20260520-1053-remove-phases-deferred-by-plan.md))
flags four writing-agent phase ids that have no entry in
`internal/atdd/phase-scopes.yaml`:

- `BACKLOG_REFINEMENT`  — agent `refine-acc`     — added by commit `600cd1b`
- `UPDATE_TICKET`       — agent `update-ticket`  — added by commit `600cd1b`
- `ENABLE_TESTS`        — agent `enable-tests`   — added by commit `b3b5952`
- `DISABLE`             — agent `disable-tests`  — added by commit `b3b5952`

These predate plan 20260520-1053 — they were added in recent commits that
should have pinned scopes in the same change but did not. The reverse-FK
test was passing at the time because it allowed `PhasesDeferredByPlan`
allowlist entries as an out (per the now-removed mechanism); but these 4
phases weren't on the allowlist either, so the test must have been
silently broken since `600cd1b`/`b3b5952` landed.

Per `feedback_no_deferred_mechanism`: every writing-agent phase needs its
scope explicitly pinned — no allowlist, no deferral. This plan fills the
gap.

## Items to walk

Items are stubs — refine with `/refine-plan` before executing. Each item
needs an explicit user scope-doctrine decision (which layer partition
applies). The four phases are split into two natural pairs.

### Item 1 — Pin scope for `BACKLOG_REFINEMENT`

**Scope:** TBD — needs user decision.

Grounding: the BPMN node lives in the `backlog_refinement` sub-process
(`process-flow.yaml:282-314`), invoked at top-level from the orchestrator
(`process-flow.yaml:111-135`). Its agent (`refine-acc`) reshapes the
ticket's acceptance criteria during intake. Find the prompt under
`internal/assets/runtime/prompts/atdd/refine-acc.md` (or similar — sweep
to confirm), enumerate which paths it may modify, then propose a scope.

Likely candidates (not pre-decided): the ticket / requirements artifact
itself — which may not yet have a Family B path key. If not, this item
may require pinning a new path key in `gh-optivem.yaml` first.

### Item 2 — Pin scope for `UPDATE_TICKET`

**Scope:** TBD — needs user decision.

Grounding: BPMN node `process-flow.yaml:301-308` (same sub-process as
Item 1). Agent `update-ticket` runs when refinement changed the ticket
contents. Likely scope mirrors Item 1.

### Item 3 — Pin scope for `ENABLE_TESTS`

**Scope:** TBD — needs user decision.

Grounding: BPMN node `process-flow.yaml:483-485` in the `at_green_system`
sub-process. Agent `enable-tests` re-enables tests that were disabled
mid-cycle. Sweep the prompt under
`internal/assets/runtime/prompts/atdd/enable-tests.md` to see which
files it actually touches — likely test files (`at_test`?), but
guardrails (e.g. "only re-enable, never edit assertions") may narrow.

### Item 4 — Pin scope for `DISABLE`

**Scope:** TBD — needs user decision.

Grounding: BPMN node `process-flow.yaml:994-996` in the AT-red cycle.
Agent `disable-tests` disables tests that are red-but-not-runtime-failing
to keep the red bar at a manageable size during authoring. Sweep the
prompt under `internal/assets/runtime/prompts/atdd/disable-tests.md`;
likely mirrors Item 3's scope.

## Sequencing (vs other in-flight plans)

- **Downstream of:** [plan 20260520-1053](20260520-1053-remove-phases-deferred-by-plan.md) —
  this plan's items are only visible as test failures because that plan
  renamed the reverse-FK test from "ScopedOrAllowlisted" to "Scoped".
- **Possibly overlapping with:** [plan 20260518-1116](20260518-1116-legacy-coverage-cycle.md)
  (legacy coverage cycle) — that plan adds new BPMN phases and pins their
  scopes; check whether its work coincidentally pins any of these 4
  before starting this plan.
