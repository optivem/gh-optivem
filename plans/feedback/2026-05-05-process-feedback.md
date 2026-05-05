# Feedback from Valentina

For each of the topics below, please provide feedback to me... then when we reach mutual agreement you can mark it as ticked....

We can discuss one by one.

Based on this feedback you can make a correspidnign plan file. For each item, write if action for it was created in plan file.

Mark each item one by one as soon as we discuss it.

## Intake Feedback

1. [x] Intake - cna we rename Auto-classify to just be Read ticket
   - Agreed: `Auto-classify ticket` → `Read ticket type`, `Auto-classify subtype` → `Read ticket subtype`. Rename internal action IDs too (`classify_ticket_type` → `read_ticket_type`, `classify_subtype` → `read_subtype`).
   - Plan: pending (collected at end into a single plan file).

2. [x] Intake - after Classification confident, instea of the vbinary Task ticket, can it instead be the 3 types: Story | Bug | Task
   - Agreed: replace intake's 2-way `Task ticket?` gate with 3-way `Ticket type?` (story / bug / task). Story and bug both → PARSE_BODY; task → CLASSIFY_SUBTYPE.
   - **Run_cycle mirroring (originally agreed) is dropped — superseded by item #7**, which collapses run_cycle's gates into a single `change_type` gate. The 3-way gate now applies inside intake only.
   - Plan: `plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`.

3. [x] Intake - what is Classification confident?
   - Answer: it's a misnomer — no LLM, no confidence score. It returns false only when the ticket's GitHub-native issue type is missing or not one of story/bug/task.
   - Agreed rename: gate label `Classification confident?` → `Ticket type recognized?`; binding `classify_confident` → `ticket_type_recognized`. STOP node label unchanged.
   - Plan: `plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`.

4. [x] Intake - should it maybe print at the end what it found in the ticket, or mauybe log it?
   - Agreed: add a service_task at end of intake that emits ticket #, ticket_type, subtype (if any), parsed body section names. Stdout only (no log file).
   - Naming: BPMN-aligned `REPORT_INTAKE_SUMMARY` / `"Report intake summary"` / action `report_intake_summary`. Also rename existing `DRIFT` description from `"Print drift warning if applicable"` → `"Report drift warning if applicable"` and action `print_drift_warning` → `report_drift_warning` for convention consistency.
   - Plan: `plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`.

5. [x] Intake - shall we illustrate the data outputs at end of intake... I rememebr we discused it before, not sure if we had a plan for it, or what happened there
   - History: added 2026-05-04 in commit 78e16b9 as a manual edit to `process-diagram.md` (BPMN data-object node `INTAKE_OUTPUTS` with dashed `produces` edge). Removed same day by commit 0e3e5dd (auto-regenerator) because it lived in the diagram MD only, not the YAML, so the regenerator overwrote it. No plan file existed.
   - Agreed (Option C, most BPMN-aligned): encode in YAML as a process-level `outputs:` field on the flow itself (matches BPMN `Process.dataOutputs`). Update diagram generator to render synthesized data-object node + dashed `produces` edge from terminal nodes, with `outputNode` styling. Scope: just `intake` for now.
   - YAML shape: `outputs:` list on the `intake:` flow with values `ticket_type`, `subtype (tasks)`, `parsed body sections`.
   - Plan: `plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`.

6. [x] Intake - could we have a subprocess called Intake, and it has branching Source, currently the only valdi value is github but in the future it could be others... so then Intake --> GitHub Intake
   - Agreed Option B (rename only, defer wrapper): rename current `intake` flow → `github_intake`. Update `main` flow's `call_activity` to reference `github_intake`. No source-branching gate yet — added when a second source actually exists.
   - Plan: `plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`.

7. [x] Intake - does it classify ticket as behavioral of structural change (change type)... and also the structural change type (system-interface-redesign, external-system-interface-redesign, system-implementatio-hcnage? (the reason I'm thinkign about this is so that in flows below, that we don't have to think about ticket tyope anymmore but just about change type
   - History: a maximal 4-axis version (`change_type`+`change_subtype`+`change_scope`+`change_channel`) shipped in 5c5085b on 2026-05-04 morning and was walked back ~3h later in 25cee6b for over-abstraction. The single-axis version we're agreeing to here is the minimal subset of that work.
   - Agreed: intake derives a single `change_type` enum (4 values: behavioral / system-interface-redesign / external-system-interface-redesign / system-implementation-change) from (ticket_type, subtype). Add `change_type` to intake's `outputs:` list (item #5).
   - run_cycle: collapse `GATE_TICKET_TYPE` + `GATE_SUBTYPE` into a single `GATE_CHANGE_TYPE` with 4 outgoing edges (behavioral → at_cycle, two interface-redesign → da_cycle, system-implementation-change → sut_cycle). **Supersedes item Run Cycle #2 below.**
   - da_cycle: internal gate continues to split SYSTEM_INTERFACE_REDESIGN_CYCLE vs EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE but binds on `change_type` instead of `subtype`.
   - Naming: reuse `change_type` (consistent with structural_cycle's template parameter and with prior 5c5085b convention).
   - Plan: `plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`.

## Run Cycle Feedback

1. [x] Run Cycle - the diamond is behavioral or structural, so then the paths should be behavioral (story / bug) and structural (task)
   - Superseded by item #7. The proposed direction is delivered at higher resolution: run_cycle's gate becomes the 4-way `change_type` gate, which inherently distinguishes behavioral from structural without a separate intermediate gate.
   - Plan: covered under item #7's plan (`plans/20260505-230000-process-flow-feedback-intake-and-run-cycle.md`, section E).

## AT Cycle

2. [x] AT Cycle - is AT - RED - TEST a step or a subprocess?
   - Direct answer: today it's modeled as a single `user_task`, deliberately (the YAML comment block draws an orchestrator/agent boundary). In proper BPMN it should be a Sub-Process, because internally it contains: multiple distinct activity types, internal gateways (compile passed?), and human-gated STOPs (REVIEW, DSL approval) that are process-level — not performer-internal procedure.
   - Deeper finding (key insight): the current modeling also conflates *creative* work (writing tests/prototypes — needs LLM) with *mechanical* work (compile, run tests, mark @Disabled, commit — no LLM needed). `structural_cycle` already follows the principle of splitting these (only `STRUCT_WRITE` is agent-dispatched; everything else is `service_task` or `user_task agent:human`). AT/CT RED phases violate this convention.
   - Agreed in principle: refactor AT/CT RED/WRITE phases (7+ nodes) so the agent only writes (creative); CLI handles compile, run, disable, commit (service_tasks); human-gated STOPs become first-class. Per-agent prompts shrink correspondingly.
   - Plan: `plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md` (separate dedicated plan — redraws the agent/CLI contract; depends on the feedback plan landing first).