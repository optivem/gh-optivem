# Split `redesign-system-structure` into system-side and external-side cycles

> ⛔ **Blocked on `plans/20260526-0832-process-diagram-cleanup.md` Items 11 + 12.**
>
> - **Item 11** splits the flat `GATE_TICKET_KIND` into hierarchical
>   `GATE_TICKET_KIND` (story/bug/task) + `GATE_TASK_SUBTYPE`
>   (cover-legacy/system-redesign/...). This plan's new branch lands
>   under `GATE_TASK_SUBTYPE`, not `GATE_TICKET_KIND` — running before
>   Item 11 means inserting under the old shape and redoing it after.
> - **Item 12** drops the `CALL_` prefix from call-activity node IDs.
>   This plan adds a new call-activity node; landing first means
>   choosing the soon-to-be-renamed `CALL_REDESIGN_EXTERNAL_SYSTEM_STRUCTURE`
>   over the post-Item-12 `REDESIGN_EXTERNAL_SYSTEM_STRUCTURE`.
>
> Wait until both Items 11 + 12 commit, then execute this plan against
> the post-commit YAML.

## Origin

Surfaced 2026-05-26 ~14:15Z during a rehearsal of issue #61 (`shop`,
TypeScript monolith, UI-only checklist). The CYCLE
`redesign-system-structure` ran `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`
unconditionally even though the ticket touched no external system — the
operator was asked to approve the external-side phase for a system-only
change, the agent loaded with nothing to do, and the cycle burned ~1
minute of churn plus two approval clicks.

Originally captured as **Item 14** in
`plans/20260526-0832-process-diagram-cleanup.md`, where it was tagged
"deferred to a separate plan" because it's the only domain-semantics
change in that plan (new ticket-kind ripples through gateways, intake
validation, Checklist-bearing enforcement, and docs). This plan replaces
that deferral. Item 14 of the diagram-cleanup plan will be updated to a
one-line pointer here.

## Problem

`redesign-system-structure` (process-flow.yaml:516-558) runs three
phases unconditionally after the checklist-progress gate:

1. `IMPLEMENT_SYSTEM_DRIVER_ADAPTERS` — system-side driver adapters.
2. `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS` — external-system
   driver adapters (test-side drivers into stubs/simulators).
3. `IMPLEMENT_AND_VERIFY_SYSTEM` — re-implement and verify the system.

There is no gateway in front of phase 2 analogous to `GATE_DSL_PORT_CHANGED`
in `refactor-port-dsl` (process-flow.yaml:959). A ticket classified as
`task/system-redesign` *always* runs the external-side phase, even when
the change is purely UI/system-side.

The cleaner shape is two sibling CYCLEs, each entered via its own
ticket-kind:

- `task/system-redesign` → `redesign-system-structure` (drop phase 2).
- `task/external-system-redesign` → `redesign-external-system-structure`
  (new sibling; runs only phase 2 + phase 3).

Both end with `IMPLEMENT_AND_VERIFY_SYSTEM` because either side of the
boundary can shift the system's port surface — changing an external-system
stub typically reflects an external-system contract change, which the
system's driven adapter has to absorb, and the acceptance tests have to
re-verify the result.

## Design

### Naming (aligned with existing conventions)

The existing taxonomy uses `<verb>-<scope>[-<aspect>]` for cycle names
and `task/<scope>-<action>` for ticket-kinds (process-flow.yaml:225-234):

| ticket-kind | CYCLE |
|---|---|
| `task/legacy-coverage` | `cover-system-behavior` |
| `task/system-redesign` | `redesign-system-structure` |
| `task/system-refactor` | `refactor-system-structure` |
| `task/test-refactor` | `refactor-test-structure` |
| `task/external-system-onboarding` | `onboard-external-system` |

New pair (pure `s/system/external-system/` substitution on both axes):

- **Cycle**: `redesign-external-system-structure`
- **Ticket-kind**: `task/external-system-redesign`

### Cycle shapes (post-split)

```
redesign-system-structure:                       redesign-external-system-structure:
  CHECK_CHECKLIST_PROGRESS                         CHECK_CHECKLIST_PROGRESS
  → GATE_CHECKLIST_PARTIALLY_DONE                  → GATE_CHECKLIST_PARTIALLY_DONE
  → STOP_CHECKLIST_PARTIALLY_DONE                  → STOP_CHECKLIST_PARTIALLY_DONE
  → IMPLEMENT_SYSTEM_DRIVER_ADAPTERS               → IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
  → IMPLEMENT_AND_VERIFY_SYSTEM                    → IMPLEMENT_AND_VERIFY_SYSTEM
  → REDESIGN_END                                   → REDESIGN_EXTERNAL_END
```

Perfectly symmetric. Only the driver-adapter step differs.

## Resolved questions

These map 1:1 onto Item 14's open questions:

- **Q14.1 (split direction)** — **Two sibling CYCLEs.** Not "keep
  unified, add a gateway": the right axis to discriminate on is the
  *ticket-kind* (operator-declared at intake), not a per-cycle predicate
  computed at runtime. Two cycles also make the opportunistic-refactor
  menu legible (operator picks "redesign system" or "redesign external
  system", not "redesign and then maybe skip half").
- **Q14.2 (verify step on the external side)** — **Yes, include
  `IMPLEMENT_AND_VERIFY_SYSTEM`.** Changing an external-system stub
  reflects an external-system contract change, which the system's
  driven adapter has to absorb; even when the system code is untouched,
  the acceptance tests must re-run against the new stub behavior.
- **Q14.3 (naming)** — **`redesign-external-system-structure` /
  `task/external-system-redesign`.** Direct mirrors of
  `redesign-system-structure` / `task/system-redesign`. The verbose
  alternative `redesign-external-system-driver-structure` breaks
  alignment (no other cycle name encodes the driver/driven split) and
  the system-side name has no `-driver-structure` suffix either even
  though it starts with a driver-adapter step.
- **Q14.4 (scope/timing)** — **Standalone plan**, this file. Cannot be
  rolled into `20260526-0832-process-diagram-cleanup.md` because that
  plan is consciously naming/layout-only; this is the only
  domain-semantics change.

## Scope

### `internal/atdd/runtime/statemachine/process-flow.yaml`

- Modify CYCLE `redesign-system-structure` (lines 516-558):
  - Drop node `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`.
  - Rewire `IMPLEMENT_SYSTEM_DRIVER_ADAPTERS → IMPLEMENT_AND_VERIFY_SYSTEM`
    directly.
  - Update the cycle header comment (lines 508-515) — the "Step 1 splits
    into two MID-direct calls" framing no longer applies; rewrite to
    describe the single system-side path.
- Add new CYCLE `redesign-external-system-structure` immediately after
  the system-side cycle. Five-node shape per the diagram above. Node
  IDs: `CHECK_CHECKLIST_PROGRESS`, `GATE_CHECKLIST_PARTIALLY_DONE`,
  `STOP_CHECKLIST_PARTIALLY_DONE`, `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`,
  `IMPLEMENT_AND_VERIFY_SYSTEM`, `REDESIGN_EXTERNAL_END`. Cycle-header
  comment mirrors the system-side comment.
- Update CYCLE `implement-ticket` (process-flow.yaml:241-310):
  - Add new node — name depends on whether diagram-cleanup Item 12 has
    landed: pre-Item-12 → `CALL_REDESIGN_EXTERNAL_SYSTEM_STRUCTURE`;
    post-Item-12 → `REDESIGN_EXTERNAL_SYSTEM_STRUCTURE`.
  - Add sequence-flow:
    `GATE_TICKET_KIND → <new node> when ticket-kind == task/external-system-redesign`.
    (Or, if diagram-cleanup Item 11 has landed:
    `GATE_TASK_SUBTYPE → <new node> when task-subtype == external-system-redesign`.)
  - Add flow `<new node> → MARK_IN_ACCEPTANCE`.
  - Update the ticket-kind lookup-table comment (lines 224-234) to add
    `task/external-system-redesign | redesign-external-system-structure`.
- **Open call**: opportunistic-refactor menus. `redesign-system-structure`
  appears as an opportunistic-refactor choice in both `refactor-top`
  (process-flow.yaml:334-348) and `change-system-behavior`
  (process-flow.yaml:464-479). **Decision needed during execution**:
  does `redesign-external-system-structure` belong in those menus too?
  Argument for: symmetric with system-side; an operator pausing after
  GREEN may want to clean up external-side structure. Argument against:
  opportunistic refactor on the external boundary is rarely the right
  reflex — external-system changes are usually ticket-driven (a partner
  changed their API), not an in-flight cleanup. **Recommendation**:
  include in both menus for symmetry; the operator picks `none` if not
  applicable, which is the existing escape hatch.

### `internal/atdd/runtime/gates/bindings.go`

- Add `"external-system-redesign"` to `ticketKindTaskSubtypes`
  (currently lines 500-506).
- Update the kind-derivation doc-comment table (lines 434-441) to add
  the new row: `task | external-system-redesign | task/external-system-redesign`.

### `internal/atdd/runtime/gates/bindings_test.go`

- Add a test case asserting that `task/external-system-redesign` is
  recognised as a valid task subtype (mirror the existing
  `task/system-redesign` case).

### `internal/atdd/runtime/clauderun/clauderun.go`

- Update the doc-comment at lines 608-615 — the Checklist-bearing
  subtype list expands from four to five (add
  `external-system-redesign`). No code change if the list is purely
  documentation; if there's a hard-coded list anywhere, add the value.

### `internal/atdd/runtime/clauderun/clauderun_test.go`

- Add a Checklist-required test for `task/external-system-redesign`
  (mirror the existing `task/system-redesign` test).

### `internal/atdd/runtime/intake/parse.go`

- Update the doc-comment at lines 77-81 mentioning
  `task/system-redesign → Checklist` — add the external-side row, or
  generalise to "the five task subtypes that consume `${checklist}`."

### `docs/process-diagram.md`

- Regenerates from the YAML via `internal/atdd/runtime/diagram/diagram.go`.
  Verify regeneration after YAML edits land.

### Tests

- Statemachine smoke tests
  (`internal/atdd/runtime/driver/embedded_smoke_test.go`,
  `internal/atdd/runtime/gates/bindings_test.go`,
  `internal/atdd/runtime/intake/parse_test.go`) — extend with the new
  ticket-kind. Per [[feedback_statemachine_test_loop_hazard]], audit
  the gate fixtures for any new loopback risk before running the full
  suite; scope to the touched packages first.

## Items

### Item 1 — YAML: split `redesign-system-structure`, add `redesign-external-system-structure`

Single commit. All YAML changes (cycle split + new sibling + lookup-table
comment + `implement-ticket` branch + opportunistic-refactor menus if
Q-menus = include).

### Item 2 — Go: extend `ticketKindTaskSubtypes` and doc comments

Single commit. `gates/bindings.go` + `gates/bindings_test.go` +
`clauderun/clauderun.go` + `clauderun/clauderun_test.go` +
`intake/parse.go`. Pure additive — no rename of existing values.

### Item 3 — Regenerate diagram and verify

Run `gh optivem architecture show` (or whatever the local diagram
regen is — verify against `internal/atdd/runtime/diagram/diagram.go`)
and confirm `docs/process-diagram.md` reflects the new cycle + the new
`implement-ticket` branch. No manual doc edits.

### Item 4 — Smoke-test against issue #61 rehearsal

Re-run `bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 --config
gh-optivem-monolith-typescript.yaml` after retagging the rehearsal
issue from `task/system-redesign` to confirm:

- A `task/system-redesign` ticket no longer triggers
  `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`.
- A `task/external-system-redesign` ticket (manually classify a test
  issue) runs the new cycle end-to-end.

## Dependencies / sequencing

See the blocker callout at the top — **Items 11 + 12 of
`plans/20260526-0832-process-diagram-cleanup.md` are hard prerequisites**.
This section captures other interactions.

- **`plans/20260526-1430-scope-validation-per-phase-baseline.md`** —
  fixes a different symptom from the *same* rehearsal (false-positive
  scope-diff after a no-op phase). Independent; can land in either
  order. After this plan, the no-op phase that triggered the
  scope-validation bug no longer fires for system-only tickets, so the
  1430 plan's value remains real (it generalises beyond this case) but
  the worst-case reproducer goes away.

## Cross-references

- Original deferral: `plans/20260526-0832-process-diagram-cleanup.md`
  Item 14 (this plan replaces the deferred brainstorm; update Item 14
  to a one-line pointer to this file).
- Historical brainstorm: `plans/archived/20260525-1057-bpmn-refactor-design.md`
  Q-new-2 (original "unified cycle" choice, now superseded).
- Related rehearsal-derived plan:
  `plans/20260526-1430-scope-validation-per-phase-baseline.md`.
