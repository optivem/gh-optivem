# Process-flow node ID rename — decisions

## Motivation

Iterating on renaming node IDs in `internal/atdd/runtime/statemachine/process-flow.yaml` to a verb-prefix vocabulary (e.g. `MOVE_TICKET_IN_PROGRESS`, `CLASSIFY_TICKET`, `DISPATCH_<agent>_AGENT`, `REQUEST_HUMAN_REVIEW`). Several questions surfaced during the proposal that needed settling before batching the rename across the YAML, `transitions_test.go`, `structural_cycle_test.go`, `driver.go`, and `atdd_commands.go`.

The rename is partially-specified — the user listed renames for the intake nodes and the structural cycle with "..." trailing each list, and the same conceptual step (`REQUEST_HUMAN_REVIEW`) appears at multiple points in the graph. The YAML schema requires unique node IDs per flow, which collides with that consistency goal.

> **Note on YAML drift.** The questions below reference node IDs from an earlier YAML revision (e.g. `STOP_INTAKE`, `MOVE_TO_IN_PROGRESS`, sibling `ATDD_STORY/BUG/CHORE` user_tasks). The YAML has since been restructured: intake STOPs are now `STOP_CLASSIFY_CONFLICT` / `STOP_SUBTYPE_MISSING` / `STOP_PARSE_ERROR`, and ticket-type fan-out happens via `change_type` call_activity dispatch in `run_cycle`, not via per-type `ATDD_*` user_tasks. Resolutions below are recorded against the current YAML state, and the per-node rename mappings still need to be re-derived against the current YAML before the rename pass batch lands.

## Resolutions

### 1. Schema strategy for repeating step names

**Resolution:** Option 2 — add a new `name:` field to the node schema. `id:` stays per-process unique (used by `sequence_flows.from/to` and node lookups); `name:` carries the reusable canonical step vocabulary and may repeat freely. Implementation lands in `plans/20260501-155353-consolidate-process-flow-with-bpmn.md` Item 2.

### 2. `_TICKET` suffix in `DISPATCH_ATDD_TASK_TICKET_AGENT`

**Resolution:** Drop the `_TICKET` suffix. The canonical name is `DISPATCH_ATDD_TASK_AGENT`.

### 3. Sibling agents

**Resolution:** Yes — sibling agents follow the `DISPATCH_<agent>_AGENT` pattern. Against the current YAML (the `ATDD_STORY/BUG/CHORE` siblings no longer exist as user_tasks), this applies to the `at_green_system` siblings:

- `ATDD_BACKEND` → `name: DISPATCH_ATDD_BACKEND_AGENT`
- `ATDD_FRONTEND` → `name: DISPATCH_ATDD_FRONTEND_AGENT`
- `ATDD_RELEASE` → `name: DISPATCH_ATDD_RELEASE_AGENT`

(`id:` may stay positional.)

### 4. `STRUCT_WRITE` agent name

**Resolution:** `name: DISPATCH_AGENT`. The node's agent is parameterised (`atdd-task` for system-interface-redesign, `atdd-chore` for chore) — naming it after a specific agent would be wrong, and "DRIVERS" is wrong for the chore call site (chore has no drivers). The generic `DISPATCH_AGENT` matches the parameterised reality without extra schema work.

### 5. `COMPILE` split — rename or process change?

**Resolution:** Dropped from the rename pass. Splitting `COMPILE → COMPILE_SOURCE / COMPILE_TESTS` is a process change (new node, new edge, new action), not a rename. Revisit only when someone proposes the split with a real motivation. For the rename pass, leave `COMPILE` ids as-is.

### 6. Scope of "..." — pattern extension

**Resolution:** Minimal rename scope. Apply the verb-prefix convention only where it adds clarity — not to gateways (which have their own consistent `GATE_` convention). Specifically:

- Schema-driven: human-review STOPs get `name: REQUEST_HUMAN_REVIEW` (where the intent is "approve / review"). Per-node mapping for fix-and-resume STOPs (`STOP_CLASSIFY_CONFLICT`, `STOP_SUBTYPE_MISSING`, `STOP_PARSE_ERROR`) still TBD — see remaining work below.
- `DISPATCH_*_AGENT` for user_task dispatch nodes.
- Verb prefix for the few service_tasks that lack one: `TICK`, `SAMPLE`, `TICKET_IN_ACCEPTANCE`.
- Gateways (`GATE_*`) untouched.

### 7. `TICK_TICKET_ITEM` parallel — `TICKET_IN_ACCEPTANCE`

**Resolution:** Pure rename, no split. Under the new schema:

- `TICK` (`structural_cycle`) → `name: TICK_CHECKLIST_ITEM` (or similar — TBD when mapping #3 lands).
- `TICKET_IN_ACCEPTANCE` (`main`) → `name: MOVE_TICKET_TO_IN_ACCEPTANCE`.

The fact that `TICKET_IN_ACCEPTANCE` also ticks the ticket header stays in `description:` — the action's primary intent is the move.

## Remaining work — per-node rename mappings

The schema decisions above are settled. The actual per-node `id` / `name` mappings against the current YAML still need to be enumerated before the rename batch (Item 3 of the consolidate-bpmn plan) can run. Specifically:

1. **Which human-review STOPs share `name: REQUEST_HUMAN_REVIEW`?** The current YAML has ~12 user_tasks with `agent: human, role: review`. The "approve" STOPs (e.g. `STOP_GREEN_REVIEW`, `STOP_STRUCT_REVIEW`, `STOP_ONBOARD_REVIEW`) clearly share that name; the "fix and re-run" STOPs (`STOP_CLASSIFY_CONFLICT`, `STOP_SUBTYPE_MISSING`, `STOP_PARSE_ERROR`) and the "review test results" STOPs (`STOP_RED_REVIEW`, `STOP_PROTOTYPE_REVIEW`, `STOP_RED_NOT_RUNTIME_FAIL`, `STOP_STRUCT_TEST`) are open. `ASK_SUPPORT` (asks for help, not review) and `LEGACY_TBD` (placeholder) are likely separate.
2. **Full enumeration of `name:` values for every renamed node** — each node touched under Q6's minimal scope needs its `name:` chosen.
3. **`id:` adjustments** — only where the new schema requires it (e.g. positional ids when multiple nodes share a `name:`). Most existing ids can stay.
