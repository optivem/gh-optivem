# AT/CT cycle — split creative work from mechanical work

Source: `plans/feedback/2026-05-05-process-feedback.md` AT Cycle 2.

This plan refactored the AT/CT pipeline so creative WRITE work goes to LLM-dispatched `user_task`s and mechanical compile / run / disable / commit work goes to `service_task`s. The bulk of the migration has shipped (see git history for the seven RED phases, `red_phase_cycle`, the `at_green_system` decomposition, and `green_phase_cycle`). Only the items below remain.

## Remaining work

1. - [ ] **Reassess `CT_GREEN_STUBS`** — currently has a TBD agent (`atdd-stubs`) in `ct_subprocess`. ⏳ Deferred: revisit when a stubs phase doc is being written; no owner picked yet. Decide whether to keep `atdd-stubs` as a real agent, fold it into another agent, or apply the RED-style creative/mechanical split (likely via reuse of `green_phase_cycle` with stubs-specific params). The `TestGapDecision_StubsOwnershipPlaceholder` test in `transitions_test.go` will fail when the placeholder is replaced — update or delete it as part of resolving the gap.

## Verification (still open)

- End-to-end manual run against a story ticket, a bug ticket, and at least one of each task subtype — confirm the orchestrator drives compile/run/disable/commit while the agent only writes.
- Regenerate `docs/process-diagram.md` and the per-flow SVGs on a CGo-enabled machine (`gh optivem atdd show diagram > docs/process-diagram.md`); they haven't been refreshed since the `green_phase_cycle` work landed.
- Token-cost sample on one ticket end-to-end pre/post refactor; record in this plan as an addendum.

## Out of scope for this plan

- Changes to `structural_cycle` (already follows the convention).
- Diagram-renderer styling changes.
- Deeper changes to per-language test invocation (e.g., supporting new test runners). Adopt `language-equivalents.md` as-is for now.
