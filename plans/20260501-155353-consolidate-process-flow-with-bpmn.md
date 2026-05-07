# Consolidate process-flow with standard BPMN — terminology and substance

## Motivation

Items 1 (terminology rename `flows`→`processes`, Go API rename) and 2 (new `name:` field, `description:`→`documentation:`) have landed. What remains is the per-node rename pass, which still needs the per-node mapping enumeration in `plans/20260501-144322-process-flow-node-id-rename-open-questions.md` to be resolved against the current YAML before it can run.

## Items

### 3. Per-node rename pass against the new schema

Batch the renames sketched in `plans/20260501-144322-process-flow-node-id-rename-open-questions.md` against the new two-field schema:

- `id`s stay positional / contextual (most existing ids are fine).
- `name`s carry canonical vocabulary (`REQUEST_HUMAN_REVIEW`, `DISPATCH_AGENT`, `CLASSIFY_TICKET`, `MOVE_TICKET`, `COMMIT`, …).

Schema-level questions in the older plan are resolved (separate `id`/`name`, `_TICKET` suffix dropped, sibling `DISPATCH_*_AGENT` pattern, `STRUCT_WRITE`→`DISPATCH_AGENT`, COMPILE split dropped from rename pass, minimal scope, `TICKET_IN_ACCEPTANCE`→`MOVE_TICKET_TO_IN_ACCEPTANCE`). Remaining work is the per-node mapping enumeration against the current YAML — specifically which human-review STOPs share `name: REQUEST_HUMAN_REVIEW` and the full per-node `name:` value list.

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
