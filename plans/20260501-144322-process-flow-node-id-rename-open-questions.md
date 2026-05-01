# Process-flow node ID rename — open questions

## Motivation

Iterating on renaming node IDs in `internal/atdd/runtime/statemachine/testdata/process-flow.yaml` to a verb-prefix vocabulary (e.g. `MOVE_TICKET_IN_PROGRESS`, `CLASSIFY_TICKET`, `DISPATCH_<agent>_AGENT`, `REQUEST_HUMAN_REVIEW`). Several questions surfaced during the proposal that need to be settled before batching the rename across the YAML, `transitions_test.go`, `structural_cycle_test.go`, `driver.go`, and `atdd_commands.go`.

The rename is partially-specified — the user listed renames for the intake nodes and the structural cycle with "..." trailing each list, and the same conceptual step (`REQUEST_HUMAN_REVIEW`) appears at multiple points in the graph. The YAML schema requires unique node IDs per flow, which collides with that consistency goal.

## Open questions

### 1. Schema strategy for repeating step names *(blocks everything else)*

`REQUEST_HUMAN_REVIEW` is proposed as the rename for at least `STOP_INTAKE`, `STOP_STRUCT_REVIEW`, and `STOP_STRUCT_TEST`. The YAML has 5 human-review STOPs total: `STOP_INTAKE`, `STOP_GREEN_REVIEW`, `STOP_ONBOARD_REVIEW`, `STOP_STRUCT_REVIEW`, `STOP_STRUCT_TEST`. The latter two live in the same flow (`structural_cycle`), so a single shared id collides at YAML load time (`buildFlow` rejects duplicates).

Two paths under discussion:

- **Option 1** — keep the canonical step name in the existing `description:` field; let `id:` stay unique/positional (e.g. `REVIEW_IMPL` / `REVIEW_TESTS`). Diagrams, logs, and the spy render the description, not the id. No engine change.
- **Option 2** — add a new `step:` (or `name:`) field to the node schema, dedicated to the reusable step name; keep `id:` separate. Cleaner long-term, requires small changes to `load.go` + `types.go` + every site that reads node metadata.

### 2. `_TICKET` suffix in `DISPATCH_ATDD_TASK_TICKET_AGENT`

The proposed name reads with a redundant `_TICKET`. Confirm: intentional vocabulary, or did you mean `DISPATCH_ATDD_TASK_AGENT`?

### 3. Sibling intake agents

If `ATDD_TASK` is renamed via the `DISPATCH_<agent>_AGENT` pattern, do the siblings (`ATDD_STORY`, `ATDD_BUG`, `ATDD_CHORE`) follow the same pattern?

### 4. `STRUCT_WRITE` agent name

Proposed: `STRUCT_WRITE → DISPATCH_WRITE_DRIVERS_AGENT`. The node has `agent: ${agent}`, parameterised to `atdd-task` for SYSAPI / SYSUI / CHORE — none involve drivers (drivers are written in `AT_RED_DSL`, `AT_RED_SYSTEM_DRIVER`, `CT_RED_EXTERNAL_DRIVER`). Did you mean `DISPATCH_WRITE_AGENT`, or do you want to specialise the shared structural cycle into per-phase WRITE nodes?

### 5. `COMPILE` split — rename or process change?

`COMPILE → COMPILE_SOURCE, then COMPILE_TESTS` isn't a rename — it splits one node into two with a new edge between them, adding a step the runtime currently doesn't have. Did you mean to introduce a new step, or rename `COMPILE` to one of the two (the other being a typo)?

### 6. Scope of "..." — pattern extension

Renames so far cover the intake nodes (4) and the structural cycle (9). The "..." in both lists suggests the convention extends further; how far?

- Other service_tasks: `MOVE_TO_IN_ACCEPTANCE`, `PICK_TOP_READY`, `RUN_SMOKE`, `COMMIT_ONBOARD`.
- Every `AT_RED_*` / `AT_GREEN_*` / `CT_RED_*` user_task (AT cycle, AT-green-system, CT subprocess).
- Gateway nodes (`GATE_TICKET_TYPE`, `GATE_LEGACY`, `GATE_TYPE_CYCLE`, `GATE_DSL_AT`, `GATE_EXT_AT`, `GATE_SYS_AT`, `GATE_DSL_CT`, `GATE_EXT_CT`, `GATE_DRIVER_EXISTS`, `GATE_INSTANCE_ACCESSIBLE`, `GATE_SMOKE_PASS`, `GATE_TEST_MODE`) — verb-prefix would be e.g. `CHECK_TICKET_TYPE`, but verbs sit awkwardly on routing nodes.

### 7. `TICK_TICKET_ITEM` parallel

`TICK → TICK_TICKET_ITEM` has a parallel: `TICKET_IN_ACCEPTANCE` is also a service_task that ticks the top-level checklist (and additionally moves the issue to "In Acceptance"). Should it also be renamed/split — e.g. `TICK_TICKET_HEADER` + `MOVE_TICKET_TO_IN_ACCEPTANCE`?
