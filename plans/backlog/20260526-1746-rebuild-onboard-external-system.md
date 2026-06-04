# Rebuild `onboard-external-system` (later)

> **At-a-glance (2026-06-04 review):** "Stash and revisit" doc. The `onboard-external-system` subprocess (four agent-less `human` placeholder steps) was removed from the BPMN as an unfinished brainstorm shape; this captures the exact YAML + all call-site wiring for a clean redesign later, plus five open design questions.
>
> **Verdict: KEEP.** Confirmed removed from `process-flow.yaml` (zero matches). Intentional archive for a future redesign — the file *is* the reinsertion spec. Don't delete.

Spinoff from `plans/20260526-1730-bpmn-process-review.md` item 5. The current `onboard-external-system` subprocess is being removed from the BPMN because its four steps are agent-less placeholders that haven't been thought through properly. This plan captures the **exact** current definition + all its references so it can be redesigned and re-added cleanly later.

**Do not pick this up until the rebuild design is ready.** This is a "stash and revisit" document, not an execution plan.

## Why removed (2026-05-26)

The subprocess in its current form is a tentative shape from an earlier brainstorm. Four `agent: human` user-tasks with no real workflow behind them — the operator just gets a list of things to do. The user wants to redesign it properly (likely promoting one or more steps to a dedicated onboarding agent, per the original `# Phase D may promote one or more steps` comment) rather than carry the placeholder forward.

## Original YAML (verbatim, for reinsertion or reference)

### Subprocess definition

Located at `internal/atdd/runtime/statemachine/process-flow.yaml:411-464` before removal.

```yaml
  # Per-system onboarding for an external system. The four steps are
  # currently agent-less placeholders (`agent: human`) matching the
  # brainstorm's tentative shape — Phase D may promote one or more steps
  # to a dedicated onboarding agent.
  onboard-external-system:
    start: CHECK_CHECKLIST_PROGRESS
    nodes:
      - id: CHECK_CHECKLIST_PROGRESS
        type: service-task
        action: check-checklist-progress

      - id: GATE_CHECKLIST_PARTIALLY_DONE
        type: gateway
        binding: checklist-partially-done
        documentation: "checklist-partially-done"

      - id: STOP_CHECKLIST_PARTIALLY_DONE
        type: user-task
        agent: human
        documentation: "${checklist_progress_summary} Approve re-running this cycle?"

      - id: IDENTIFY_EXTERNAL_SYSTEM
        type: user-task
        agent: human
        documentation: "Identify external system"

      - id: DOCUMENT_EXTERNAL_SYSTEM_CONTRACT
        type: user-task
        agent: human
        documentation: "Document external system contract"

      - id: SETUP_EXTERNAL_SYSTEM_ACCESS
        type: user-task
        agent: human
        documentation: "Set up external system access (credentials, endpoints, sandbox)"

      - id: VERIFY_EXTERNAL_SYSTEM_REACHABLE
        type: user-task
        agent: human
        documentation: "Verify external system reachable"

      - id: ONBOARD_END
        type: end-event
        documentation: "External System Onboarded"

    sequence-flows:
      - {from: CHECK_CHECKLIST_PROGRESS,          to: GATE_CHECKLIST_PARTIALLY_DONE}
      - {from: GATE_CHECKLIST_PARTIALLY_DONE,     to: STOP_CHECKLIST_PARTIALLY_DONE, when: "checklist-partially-done == true"}
      - {from: GATE_CHECKLIST_PARTIALLY_DONE,     to: IDENTIFY_EXTERNAL_SYSTEM,      when: "checklist-partially-done == false"}
      - {from: STOP_CHECKLIST_PARTIALLY_DONE,     to: IDENTIFY_EXTERNAL_SYSTEM}
      - {from: IDENTIFY_EXTERNAL_SYSTEM,          to: DOCUMENT_EXTERNAL_SYSTEM_CONTRACT}
      - {from: DOCUMENT_EXTERNAL_SYSTEM_CONTRACT, to: SETUP_EXTERNAL_SYSTEM_ACCESS}
      - {from: SETUP_EXTERNAL_SYSTEM_ACCESS,      to: VERIFY_EXTERNAL_SYSTEM_REACHABLE}
      - {from: VERIFY_EXTERNAL_SYSTEM_REACHABLE,  to: ONBOARD_END}
```

### Call-site wiring in `implement-ticket`

The subprocess was reached via the `task/external-system-onboarding` subtype route. Reinsertion requires re-adding:

1. **Call-activity node** under `implement-ticket.nodes`:
   ```yaml
         - id: ONBOARD_EXTERNAL_SYSTEM
           type: call-activity
           process: onboard-external-system
           documentation: "Onboard External System"
   ```

2. **Gateway edge** under `implement-ticket.sequence-flows` (added between the other subtype edges, before the catch-all to `UNKNOWN_TASK_SUBTYPE`):
   ```yaml
         - {from: GATE_TASK_SUBTYPE, to: ONBOARD_EXTERNAL_SYSTEM,            when: "task-subtype == external-system-onboarding"}
   ```

3. **Convergence edge** (so the subprocess re-joins `MARK_IN_ACCEPTANCE`):
   ```yaml
         - {from: ONBOARD_EXTERNAL_SYSTEM,            to: MARK_IN_ACCEPTANCE}
   ```

4. **Gateway lookup table comment** (in the `implement-ticket` block-comment around line 213): add back the `external-system-onboarding → onboard-external-system` row.

### Code-side references touched at removal time

When the subprocess is rebuilt, these are the modules that needed updates at removal — i.e., the same modules will need touching again to re-register the rebuilt subprocess:

- `internal/atdd/runtime/actions/bindings.go` — any `onboard-external-system` action registration (audit).
- `internal/atdd/runtime/gates/bindings.go` — gateway binding for `external-system-onboarding` subtype.
- `internal/atdd/runtime/gates/bindings_test.go` — corresponding test cases.
- `internal/atdd/runtime/clauderun/clauderun.go` — dispatch routing if `onboard-external-system` is referenced there.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — test cases.
- `internal/atdd/runtime/statemachine/transitions_test.go` — fixture / transition assertions.
- `internal/atdd/runtime/diagram/diagram.go` — only if there's a specific reference (likely just process-loading paths).
- `docs/process-diagram.md` — regenerated section.
- `docs/images/process-diagram-4-implement-ticket.svg` — regenerated.
- `docs/images/process-diagram-7-onboard-external-system.svg` — deleted at removal; regenerated when rebuilt.

(Verify the exact list at execute time by grepping `onboard-external-system|ONBOARD_EXTERNAL_SYSTEM|external-system-onboarding` after the rebuilt design is in place.)

## Open design questions for the rebuild

These are the questions the original placeholder shape didn't answer — resolve them **before** picking this back up:

1. **Agent vs human for each step.** The original `# Phase D may promote one or more steps to a dedicated onboarding agent` suggests some steps should be automated. Which?
   - `IDENTIFY_EXTERNAL_SYSTEM` — agent or human?
   - `DOCUMENT_EXTERNAL_SYSTEM_CONTRACT` — agent (parses an existing spec / OpenAPI)? Or human (writes from scratch)?
   - `SETUP_EXTERNAL_SYSTEM_ACCESS` — operator/human (credentials are sensitive).
   - `VERIFY_EXTERNAL_SYSTEM_REACHABLE` — agent could run a smoke command; human could click through; what's the contract?

2. **Outputs and scopes.** What artifacts does each step produce (and where do they land in the repo)? Today the steps don't declare any outputs — phase-scopes integration is missing.

3. **Idempotence / re-runs.** The CHECKLIST_PROGRESS prefix is copy-pasted from refactor/redesign cycles. Does the onboarding flow really fit that shape, or is it a one-shot per external-system?

4. **Naming / vocabulary.** "Onboard External System" implies a one-shot ceremony. Is it really one-shot per external system per project (lifecycle event), or could the same external system get re-onboarded? Affects whether the gating belongs at ticket level or at config-init time.

5. **Relationship to `redesign-external-system-structure`.** That CYCLE assumes the external system is already known and contract-documented. Is onboarding strictly a prerequisite, or do they share steps?

## Cross-references

- Triggered by: `plans/20260526-1730-bpmn-process-review.md` item 5.
- Related: `docs/bpmn-process-design.md` (distilled design rationale that shaped the placeholder; was `plans/archived/20260525-1057-bpmn-refactor-design.md`).
- Related: `plans/backlog/20260516-1754-per-node-implementer-human-or-agent.md` (the broader question of which steps are human vs agent — overlaps with Q1 above).
