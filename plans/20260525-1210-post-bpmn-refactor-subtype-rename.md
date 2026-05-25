# Post-BPMN-refactor: `subtype` → `ticket_subtype` rename

**Date:** 2026-05-25
**Trigger:** runs *after* [`20260525-1057-bpmn-refactor-design.md`](20260525-1057-bpmn-refactor-design.md) Item 9 (Phase C.3 — Migrate rest of YAML) has landed. Do not execute earlier — the BPMN rewrite (Q17=A: full replacement) may rename, restructure, or eliminate the `subtype` concept entirely, in which case some or all of this plan is a no-op.

## Origin

This plan is the carry-forward of remark 2 from the now-deleted `plans/20260520-1825-process-flow-remarks.md`. That plan flagged a naming inconsistency in the old `process-flow.yaml`:

- `github_intake.outputs` listed `ticket_type`, `subtype (tasks)`, `change_type` — `subtype` is the odd one out (the other two use the `<thing>_type` shape).
- The same `subtype` token also appears in node IDs (`CLASSIFY_TICKET_SUBTYPE`, `GATE_SUBTYPE_OK`, `STOP_SUBTYPE_MISSING`), action names (`read_subtype`), bindings (`subtype_ok`), edge guards, DA-cycle params, and prose comments.

Remark 1 of the 1825 plan (BACKLOG_REFINEMENT reorder) is fully superseded by the BPMN refactor's Q9=A (new CYCLE `refine-backlog` as a sibling cycle) and does not carry forward.

## Scope

**Audit, then rename if still present.** The BPMN refactor's Phase C.3 rewrites the YAML from scratch. Three possible outcomes:

1. The new YAML preserves a `subtype` concept under the same name → rename to `ticket_subtype`.
2. The new YAML preserves the concept under a different name (e.g., absorbed into the TOP `implement-ticket` ticket-classification gateway with a different output key) → confirm the new name is consistent with `ticket_type` / `change_type` siblings; rename if not.
3. The new YAML eliminates the concept (e.g., the rewrite collapses ticket classification into a different shape) → this plan is a no-op; mark all items done with a one-line "concept no longer present in new YAML" note and delete the plan.

## Items

1. - [ ] **Audit the new YAML.** Grep `internal/atdd/runtime/statemachine/process-flow.yaml` for `subtype` (case-insensitive). Grep `internal/atdd/runtime/` Go source for the literal strings `"subtype"` and `"subtype_ok"`. Record findings here:
    - Token occurrences found: _(fill in during execution)_
    - New concept name (if absorbed under a different name): _(fill in)_
    - **Decision:** rename (proceed to Item 2) / no-op (skip to Item 4).

2. - [ ] **Rename in `process-flow.yaml`.** Replace `subtype` with `ticket_subtype` in all of:
    - Output lists (e.g., the `outputs:` block of whatever node owns ticket classification in the new YAML — likely a TOP-level task per Q26=A).
    - Node IDs: `CLASSIFY_TICKET_SUBTYPE` (keep), `GATE_SUBTYPE_OK` → `GATE_TICKET_SUBTYPE_OK`, `STOP_SUBTYPE_MISSING` → `STOP_TICKET_SUBTYPE_MISSING`.
    - Action names: `read_subtype` → `read_ticket_subtype`.
    - Bindings: `subtype_ok` → `ticket_subtype_ok`.
    - Edge guards using the renamed binding.
    - Any cycle params using `subtype:` as a key → `ticket_subtype:`.
    - Prose comments referring to "subtype".
    Commit.

3. - [ ] **Rename in Go runtime.** Grep `internal/atdd/runtime/` for any reader of the renamed binding / state key literals. Rename to match. Re-run statemachine tests (`structural_cycle_test.go`, `transitions_test.go`, `behavioral_cycle_test.go`) — watch for loopback hazards per [[feedback_statemachine_test_loop_hazard]]. Commit.

4. - [ ] **Regenerate diagrams.** `gh optivem process show > docs/process-diagram.md`; verify the SVGs in `docs/images/` reflect the rename. If Item 1 resolved to no-op, this item only verifies no spurious doc changes. Commit.

## Open questions

- **Q1 — Node ID suffix length.** Renaming `GATE_SUBTYPE_OK` → `GATE_TICKET_SUBTYPE_OK` is more consistent but longer. The 1825 plan recommended renaming for full consistency. Confirm during Item 2.
- **Q2 — Cycle param key.** The 1825 plan flagged DA-cycle `subtype:` params (lines 1287, 1295 in the old YAML). If the new YAML's equivalent cycle (e.g., `redesign-system-structure` per the Q26 reframe) still passes a subtype param, rename the key. Confirm during Item 2.

## Cross-references

- Supersedes (carry-forward of remark 2): `plans/20260520-1825-process-flow-remarks.md` (deleted 2026-05-25).
- Depends on: [`20260525-1057-bpmn-refactor-design.md`](20260525-1057-bpmn-refactor-design.md) Item 9 (Phase C.3) being complete.
- Naming doctrine reference: `ticket_type` / `change_type` use the `<thing>_type` shape; this plan brings `subtype` in line with that shape.
