# Consolidate process-flow with standard BPMN — terminology and substance

## Motivation

The header of `internal/atdd/runtime/statemachine/testdata/process-flow.yaml` claims "BPMN-shaped vocabulary". The shape is largely faithful (node types, sequence flows, callable sub-processes), but three places diverge from BPMN in ways that have started to bite:

- **`flows:` at the top level is what BPMN calls `processes:`.** "Flow" in BPMN means *Sequence Flow* — an edge between elements within a process — which the YAML correctly uses for `sequence_flows:`. The dual meaning is confusing for anyone arriving with a BPMN background, and the divergence is mirrored across every Go consumer (`Flow`, `RunFlow`, `FlowName`, …).
- **`id` carries the canonical step name.** Today's ids (`STOP_INTAKE`, `MOVE_TO_IN_PROGRESS`, `STRUCT_WRITE`) conflate BPMN's `id` role (machine ref, must be unique) with the `name` role (human label, can repeat). This collides as soon as the same conceptual step (e.g. `REQUEST_HUMAN_REVIEW`) needs to appear in two places — exactly the rename effort currently in flight.
- **`call_activity.flow:`** references a sub-process by name; BPMN spells the field `process:`.

Closing these gaps makes the "BPMN-shaped" docstring honest, makes the YAML readable cold by anyone with a BPMN background, and removes the technical reason the rename effort keeps tripping over name collisions. It also lays the groundwork for switching to a real BPMN renderer / linter in future without a second rename pass.

## Items

Sequence: terminology first (shallow rename, no schema change), then the substance change (new `name:` field), then the per-node renames against the new schema. One PR per item keeps each diff focused.

### 1. Terminology rename — `flows` → `processes`

**Files:**
- `internal/atdd/runtime/statemachine/testdata/process-flow.yaml`
- `internal/atdd/runtime/statemachine/{types.go,load.go,run.go}`
- `internal/atdd/runtime/statemachine/{transitions_test.go,structural_cycle_test.go}`
- `internal/atdd/runtime/driver/{driver.go,driver_test.go}`
- `atdd_commands.go`

YAML schema:

| Before | After |
|---|---|
| `flows:` (top-level container) | `processes:` |
| `call_activity.flow: <name>` | `call_activity.process: <name>` |

Go API (mechanical rename):

| Before | After |
|---|---|
| `Flow` (struct type) | `Process` |
| `Engine.Flows` map | `Engine.Processes` |
| `Engine.RunFlow(name, ctx)` | `Engine.RunProcess(name, ctx)` |
| `Engine.NextEdge(flowName, …)` | `Engine.NextEdge(processName, …)` |
| `Options.FlowName` (driver) | `Options.ProcessName` |
| `DefaultFlowName` const | `DefaultProcessName` |
| `flow.Name / Start / Nodes / Edges / OutgoingByNode` | `process.*` |
| `rawFlow`, `buildFlow` (loader internals) | `rawProcess`, `buildProcess` |
| `RawNode.Flow` (call_activity sub-process ref) | `RawNode.Process` |
| YAML tags `yaml:"flow"` / `yaml:"flows"` | `yaml:"process"` / `yaml:"processes"` |

Sweep comments, docstrings, banner text, and error messages that say "flow" in the *process* sense → "process". Keep `sequence_flows:` and any "Sequence Flow" prose intact — that **is** BPMN-correct.

### 2. Substance change — separate `id` from `name`

**Files:** same as #1.

Add a new `name:` field to the node schema. `id:` stays as the unique-per-process machine reference (used by `sequence_flows.from/to` and `Process.Nodes` lookups); `name:` carries the canonical step vocabulary (`REQUEST_HUMAN_REVIEW`, `DISPATCH_AGENT`, `MOVE_TICKET`, …). `name:` has no uniqueness constraint and may repeat freely.

Schema:

```go
// types.go / load.go
type RawNode struct {
    ID   string `yaml:"id"`
    Name string `yaml:"name,omitempty"`   // NEW — canonical step vocabulary, may repeat
    Type string `yaml:"type"`
    // ...existing fields unchanged...
}
```

YAML use:

```yaml
- id: STRUCT_REVIEW_IMPL
  name: REQUEST_HUMAN_REVIEW
  type: user_task
  agent: human
  role: review

- id: STRUCT_REVIEW_TESTS
  name: REQUEST_HUMAN_REVIEW          # repeat — different id, same name
  type: user_task
  agent: human
  role: review
```

Render `name` (with `id` fallback when absent) in:
- the spy/history in `structural_cycle_test.go`
- the driver's `promptForAgent` banner
- Mermaid / diagram generators
- log lines that surface "step ran"

Per-process id-uniqueness is already enforced by the loader (`buildFlow` → `buildProcess`); that stays. Per-name uniqueness is intentionally not enforced.

### 3. Per-node rename pass against the new schema

Once #2 lands, batch the renames sketched in `plans/20260501-144322-process-flow-node-id-rename-open-questions.md` against the new two-field schema:

- `id`s become positional / contextual (e.g. `STRUCT_REVIEW_IMPL`, `STRUCT_REVIEW_TESTS`, `INTAKE_REVIEW`, `ONBOARD_REVIEW`).
- `name`s become canonical vocabulary (`REQUEST_HUMAN_REVIEW`, `DISPATCH_AGENT`, `CLASSIFY_TICKET`, `MOVE_TICKET`, `COMMIT`, …).

The per-rename open questions in that older plan (which mappings, scope of "...", `_TICKET_AGENT` clarification, `COMPILE` split-or-rename, etc.) still need to be resolved before this batch lands. That plan stays the source of truth for the specific mappings; this plan is the wider BPMN-alignment roadmap.

### 4. Optional — `description:` → `documentation:`

BPMN distinguishes `name` (label) from `documentation` (free-text detail). After #2 lands, the existing `description:` field overlaps semantically with `name:`. Recommended: rename `description:` → `documentation:` for BPMN faithfulness, keeping the existing semantics (free-text detail rendered on diagrams). Skip if reviewer churn outweighs the win.

## Out of scope

The following BPMN features exist but the current modeling does not need them. Add only if a use case appears.

- **Event subtypes** (Message / Timer / Error / Signal / Compensation events) — current `start_event` / `end_event` are plain markers.
- **Gateway subtypes** (Exclusive / Inclusive / Parallel / Event-Based) — single `gateway` is effectively XOR; no parallel paths or "wait for one of N events" semantics exist.
- **Data objects + data input/output associations** — implicit `Context.State` is sufficient.
- **Boundary events / interrupt handlers** — engine-level error halting is the equivalent.
- **Swim lanes / pools** — `agent: human` / `agent: atdd-task` is the functional equivalent.

The following app-level extensions are deliberate engine choices, **not** divergences worth fixing:

- `role:` (diagram styling — `review` vs `implement`)
- `phase_doc:` (link to per-phase markdown)
- `params:` on `call_activity` (simpler than `<dataInputAssociation>`)
- `binding:` on gateway (gateway computes a value via `GateFn(binding)`; edge predicates read it back through `Context.State` — useful for hand-edited YAML, lets downstream gates reuse upstream decisions)
- `when:` predicates as plain strings (custom mini-language instead of XPath/FEEL)

## Open questions

1. **Sequencing.** Confirm: terminology rename (#1) → name field (#2) → per-node renames (#3). One PR per item.
2. **#1 single PR vs split.** The YAML key swap and the Go struct/method rename must land together (the loader needs both). Confirm one PR.
3. **`description:` → `documentation:` (#4).** Land alongside #2, or skip. Recommendation: do it; cheap relative to #2's other touches.
4. **Per-node id/name mappings** are tracked in `plans/20260501-144322-process-flow-node-id-rename-open-questions.md`. All open questions there still gate #3.
