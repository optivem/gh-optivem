# Process flow feedback — intake + run_cycle changes

Source: `plans/feedback/2026-05-05-process-feedback.md` items 1-7 (Intake) + Run Cycle 1.

Run Cycle 1 is delivered by item 7 (no separate work). AT Cycle 2 lives in a **separate** plan because it changes the agent/CLI contract.

## Goals

1. Make intake's vocabulary honest (read, not classify; recognize, not confidence).
2. Make run_cycle's dispatch single-axis on a derived `change_type`.
3. Make intake's outputs first-class in the YAML schema (BPMN process outputs).
4. Add a runtime summary at end of intake.
5. Honest naming: `intake` → `github_intake` (acknowledge GitHub coupling); defer source wrapper.
6. Convention consistency: align all "emit to user" actions on `report_*` verb.

## Out of scope

- 4-axis classification (`change_subtype`/`change_scope`/`change_channel`). Already rejected in 25cee6b.
- Source wrapper / multi-source intake. Defer until a second source exists.
- Per-step or per-flow data outputs beyond `intake`. Add `outputs:` only where the contract is non-obvious.
- Diagram-renderer styling beyond what `outputNode` already supports.
- AT/CT RED phase decomposition. Separate plan.

## Changes

### A. Intake renames (items 1, 3, 4, 6)

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

- Flow rename: top-level key `intake:` → `github_intake:`. Update YAML comment block accordingly.
- `main` flow's `INTAKE` `call_activity` → `flow: github_intake`.
- Node `CLASSIFY`: description `"Auto-classify ticket"` → `"Read ticket type"`; `action: classify_ticket_type` → `action: read_ticket_type`.
- Node `CLASSIFY_SUBTYPE`: description `"Auto-classify subtype"` → `"Read ticket subtype"`; `action: classify_subtype` → `action: read_subtype`.
- Node `GATE_CLASSIFY_CONFIDENT`: description `"Classification confident?"` → `"Ticket type recognized?"`; `binding: classify_confident` → `binding: ticket_type_recognized`. Update outgoing `when:` predicates to match.
- Node `DRIFT` (in `structural_cycle`): description `"Print drift warning if applicable"` → `"Report drift warning if applicable"`; `action: print_drift_warning` → `action: report_drift_warning`.

**File:** `internal/atdd/runtime/actions/bindings.go`
- Rename action registrations: `classify_ticket_type` → `read_ticket_type`, `classify_subtype` → `read_subtype`, `print_drift_warning` → `report_drift_warning`. Function names follow.

**File:** `internal/atdd/runtime/gates/bindings.go`
- Rename gate binding `classify_confident` → `ticket_type_recognized`.

**Tests:** update `bindings_test.go`, `transitions_test.go`, `structural_cycle_test.go` to use new names. No behavioral change.

### B. Intake gate restructure (item 2)

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Replace the binary `GATE_NEEDS_SUBTYPE` ("Task ticket?") with a 3-way `GATE_TICKET_TYPE_INTAKE` ("Ticket type?") gate.

- Outgoing edges:
  - `when: "ticket_type == story"` → `PARSE_BODY`
  - `when: "ticket_type == bug"` → `PARSE_BODY`
  - `when: "ticket_type == task"` → `CLASSIFY_SUBTYPE` (now `READ_SUBTYPE`)
- Binding stays `ticket_type` (already 3-valued).
- `STOP_CLASSIFY_CONFLICT` continues to loop back to `CLASSIFY` (now `READ_TICKET_TYPE`).

The visual diagram now shows three explicit branches; mechanically story and bug both still hit `PARSE_BODY`.

### C. End-of-intake summary node (item 4)

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Add a new `service_task` between `PARSE_BODY` (on `parse_ok == true`) and `INTAKE_END`:

```yaml
- id: REPORT_INTAKE_SUMMARY
  type: service_task
  action: report_intake_summary
  description: "Report intake summary"
```

Rewire: `GATE_PARSE_OK -- "parse_ok == true" --> REPORT_INTAKE_SUMMARY --> INTAKE_END`. The failure path stays as-is.

**File:** `internal/atdd/runtime/actions/bindings.go`
- New `report_intake_summary` action: prints to stdout — ticket #, ticket_type, subtype (if any), `change_type`, parsed body section names. List section names only (not contents).

**Tests:** add a binding test asserting the action runs without side effects on a known intake state and produces a non-empty summary.

### D. `outputs:` on flow + diagram support (item 5)

**Schema extension** — `internal/atdd/runtime/statemachine/process-flow.yaml`:

```yaml
github_intake:
  start: READ_TICKET_TYPE
  outputs:
    - ticket_type
    - subtype (tasks)
    - change_type
    - parsed body sections
  nodes: [...]
  sequence_flows: [...]
```

The `outputs:` field is optional; absent on every other flow.

**File:** `internal/atdd/runtime/statemachine/loader.go` (or equivalent — wherever the YAML is parsed)
- Add `Outputs []string` to the flow struct. Validate it's only present where defined.

**File:** `internal/atdd/runtime/diagram/diagram.go`
- When emitting a flow's Mermaid section, if `outputs:` is non-empty:
  - Synthesize a node `<FLOW_ID_UPPER>_OUTPUTS` styled `[/"<comma-joined outputs>"/]`.
  - For every `end_event` reachable in the flow, emit `<end_id> -. produces .-> <FLOW_ID_UPPER>_OUTPUTS`.
  - Apply the existing `outputNode` classDef (dashed blue stroke).
- Tests: extend `diagram` package tests with a fixture flow that has `outputs:`.

### E. `change_type` derivation + run_cycle collapse + da_cycle binding (item 7, supersedes Run Cycle 1)

**Concept.** Intake derives `change_type ∈ {behavioral, system-interface-redesign, external-system-interface-redesign, system-implementation-change}` from `(ticket_type, subtype)` deterministically:

| `ticket_type` | `subtype` | `change_type` |
|---|---|---|
| story | — | behavioral |
| bug | — | behavioral |
| task | system-interface-redesign | system-interface-redesign |
| task | external-system-interface-redesign | external-system-interface-redesign |
| task | system-implementation-change | system-implementation-change |

**Where derivation lives.** Add to the runtime context populated by intake — likely in `internal/atdd/runtime/statemachine/context.go` or wherever the intake context is built (alongside `ticket_type`, `subtype`). No new YAML node; derivation is implicit during intake completion. Surfaced via the `outputs:` list (item 5) and the `report_intake_summary` action (item 4).

**Run_cycle collapse** — `internal/atdd/runtime/statemachine/process-flow.yaml`:

Replace the current `run_cycle` flow's two-gate structure:

```yaml
run_cycle:
  start: GATE_CHANGE_TYPE
  nodes:
    - id: GATE_CHANGE_TYPE
      type: gateway
      binding: change_type
      description: "Change type?"
    - id: AT_CYCLE
      type: call_activity
      flow: at_cycle
    - id: DA_CYCLE
      type: call_activity
      flow: da_cycle
    - id: SUT_CYCLE
      type: call_activity
      flow: sut_cycle
    - id: CYCLE_END
      type: end_event
  sequence_flows:
    - {from: GATE_CHANGE_TYPE, to: AT_CYCLE,  when: "change_type == behavioral"}
    - {from: GATE_CHANGE_TYPE, to: DA_CYCLE,  when: "change_type == system-interface-redesign"}
    - {from: GATE_CHANGE_TYPE, to: DA_CYCLE,  when: "change_type == external-system-interface-redesign"}
    - {from: GATE_CHANGE_TYPE, to: SUT_CYCLE, when: "change_type == system-implementation-change"}
    - {from: AT_CYCLE,  to: CYCLE_END}
    - {from: DA_CYCLE,  to: CYCLE_END}
    - {from: SUT_CYCLE, to: CYCLE_END}
```

Removes the prior `GATE_TICKET_TYPE` + `GATE_SUBTYPE` chain.

**da_cycle binding swap** — same file:

In `da_cycle.GATE_SUBTYPE`, change:
- `binding: subtype` → `binding: change_type`
- Sequence flow predicates from `subtype == system-interface-redesign` / `subtype == external-system-interface-redesign` to the matching `change_type == ...` form.
- Optional: rename node `GATE_SUBTYPE` inside `da_cycle` → `GATE_CHANGE_TYPE_DA` for clarity. Internal-only rename.

**File:** `internal/atdd/runtime/gates/bindings.go`
- Add `change_type` gate binding that reads the derived value from context.

**Tests:** update `transitions_test.go` to drive the new single-gate path. Existing test cases mapping (ticket_type, subtype) → cycle should pass with the new `change_type` plumbing.

## Sequencing

The changes are interdependent at the YAML level but can land in this order to keep each commit small and reviewable:

1. **Schema + diagram support for `outputs:`** (item 5 plumbing only — generator + loader + tests). No YAML output additions yet. Lands first because it's pure infrastructure.
2. **Renames batch** (items 1, 3, 4 description + DRIFT rename). Mechanical YAML/Go renames + tests.
3. **`github_intake` flow rename** (item 6). Single rename + main reference update.
4. **3-way intake gate** (item 2). YAML edit + transitions test.
5. **End-of-intake summary node** (item 4 service_task addition). Action + YAML node + test.
6. **`change_type` derivation + run_cycle collapse + da_cycle rebind** (item 7). Largest functional change; lands when items 1-5 are stable.
7. **Add `outputs:` to `github_intake`** (item 5 actual data). Lands last so the diagram render is the final visible artifact.
8. **Regenerate diagrams** (`gh optivem atdd show diagram > docs/process-diagram.md` + SVG regen via existing workflow).

Each step has its own commit. Step 8 is a single auto-generated commit at the end (matches the existing pattern from 0e3e5dd, but this time the YAML is the source of truth for everything that appears in the diagram).

## Verification

- `go test ./internal/atdd/runtime/...` clean.
- Generated `docs/process-diagram.md` shows:
  - Intake flow with the renamed labels (Read ticket type, Read ticket subtype, Ticket type recognized?, Ticket type? gate with three labeled branches, Report intake summary).
  - A dashed `produces` edge from `INTAKE_END` to a synthesized data-object node listing the outputs.
  - Run Cycle with a single 4-way `Change type?` gate.
  - DRIFT node says "Report drift warning if applicable".
- Manual smoke: run `gh optivem atdd implement-ticket --issue <N>` against a story/bug ticket and a task ticket of each subtype; confirm the printed intake summary lines up with the parsed values, and that the cycle dispatched matches the table in section E.

## Notes on what NOT to change

- Do **not** add a `Source?` gate in `github_intake` (item 6 deferred wrapper).
- Do **not** introduce `change_subtype`, `change_scope`, or `change_channel`. Item 7 is single-axis only.
- Do **not** add `outputs:` to flows other than `github_intake` in this plan.
- Do **not** touch AT/CT RED/WRITE phase decomposition (separate plan).
