# process-flow.yaml remarks

Source: review of `docs/process-diagram.md` (rendered from `internal/atdd/runtime/statemachine/process-flow.yaml`).

## Remarks

### 1. Reorder: BACKLOG_REFINEMENT between INTAKE and RUN_LEGACY_CYCLE

**Current** (`process-flow.yaml:132–135`):

```
MOVE_TICKET_IN_PROGRESS → INTAKE
INTAKE                  → RUN_LEGACY_CYCLE
RUN_LEGACY_CYCLE        → BACKLOG_REFINEMENT
BACKLOG_REFINEMENT      → RUN_CYCLE
```

**Desired**: `BACKLOG_REFINEMENT` should sit after `INTAKE` and **before** `RUN_LEGACY_CYCLE`.

```
MOVE_TICKET_IN_PROGRESS → INTAKE
INTAKE                  → BACKLOG_REFINEMENT
BACKLOG_REFINEMENT      → RUN_LEGACY_CYCLE
RUN_LEGACY_CYCLE        → RUN_CYCLE
```

**Files touched**:
- `internal/atdd/runtime/statemachine/process-flow.yaml` — update the `edges:` in the top-level ticket-lifecycle process (lines ~132–135).
- Regenerate `docs/process-diagram.md` + `docs/images/process-diagram-2-ticket-lifecycle.svg` (and any other affected sub-diagrams).
- Re-run statemachine tests (`structural_cycle_test.go`, `transitions_test.go`, `behavioral_cycle_test.go`) — watch for loopback hazards per [[feedback_statemachine_test_loop_hazard]].

**Open questions**: none yet.

### 2. Rename `subtype` → `ticket_subtype`

**Context**: `github_intake.outputs` (`process-flow.yaml:155–159`) lists:

```yaml
outputs:
  - ticket_type
  - subtype (tasks)
  - change_type
  - parsed body sections
```

`ticket_type` and `change_type` use the `<thing>_type` shape; `subtype` is the odd one out. Rename to `ticket_subtype` for consistency.

**Files touched** (occurrences in `process-flow.yaml`):
- Output list: line 157 — `subtype (tasks)` → `ticket_subtype (tasks)`.
- Node ids / labels: `CLASSIFY_TICKET_SUBTYPE` (line 182), `GATE_SUBTYPE_OK` (line ~189 — consider `GATE_TICKET_SUBTYPE_OK`), `STOP_SUBTYPE_MISSING` (line 196 — consider `STOP_TICKET_SUBTYPE_MISSING`).
- Action: `read_subtype` (line 184) → `read_ticket_subtype`.
- Binding: `subtype_ok` (line 189) → `ticket_subtype_ok`.
- Edge guards using `subtype_ok` (lines 231–232).
- DA-cycle params using `subtype:` key (lines 1287, 1295) and the comment on line 1263.
- Prose comments referring to "subtype" (lines 6, 143, 339, 347).

**Knock-on**:
- Go runtime: any reader of the `subtype`/`subtype_ok` binding names (search `internal/atdd/runtime/` for these literals).
- Gate fixtures in statemachine tests.
- Regenerate `docs/process-diagram.md` + SVGs.

**Open questions**:
- Also rename `GATE_SUBTYPE_OK` → `GATE_TICKET_SUBTYPE_OK` and `STOP_SUBTYPE_MISSING` → `STOP_TICKET_SUBTYPE_MISSING` for full consistency, or keep node IDs shorter? (Recommend rename — matches the binding.)
- Should the DA-cycle `subtype:` param key also become `ticket_subtype:`? (Recommend yes — single vocabulary.)
