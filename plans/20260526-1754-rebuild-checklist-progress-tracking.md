# Rebuild checklist progress-tracking (later)

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-26T16:00:05Z`

Spinoff from `plans/20260526-1730-bpmn-process-review.md` item 8. The checklist progress-tracking machinery (pre-CYCLE resume gate + post-CYCLE auto-tick) is being removed from the BPMN because the agent works atomically — a CYCLE either completes the whole ticket or it doesn't, so tracking partial checklist progress at runtime doesn't earn its keep. This plan captures the **exact** current shape so it can be re-introduced cleanly later if the atomicity assumption changes (e.g., long-running tickets with multi-step work that does need resumption).

The **spec/input** role of the checklist stays: the agent still reads the `Checklist` section from the ticket body as the list of sub-items to do. Only the BPMN gating + post-step ticking is being cut.

**Do not pick this up until there is a concrete need.** This is a stash document, not an execution plan.

## Why removed (2026-05-26)

The pre-CYCLE resume gate (`CHECK_CHECKLIST_PROGRESS` → `GATE_CHECKLIST_PARTIALLY_DONE` → `STOP_CHECKLIST_PARTIALLY_DONE`) exists to ask the operator "this checklist is partially complete — proceed?" on cycle re-runs. But the agent today completes the ticket atomically (commits at the end of the CYCLE, not per-item), so a failed mid-run leaves no resumable partial state. The gate fires only on re-runs of already-completed tickets — operationally noise.

The post-CYCLE auto-tick (`Tracker.MarkChecklistComplete` inside `move-to-in-acceptance`) records "every checklist item is done" on the ticket body when the CYCLE finishes. It's documentation of completion, not workflow state. If we want that record, the agent can tick boxes as part of its own task; the BPMN doesn't need to do it.

If atomicity stops holding later (e.g., we introduce a CYCLE that commits per-item and supports genuine resume), this plan describes what to re-add.

## Original YAML (verbatim, for reinsertion or reference)

### Pre-CYCLE prefix block — appeared in four CYCLEs

The triad below was copy-pasted (identical) into the start of:
- `redesign-system-structure` (`process-flow.yaml:543-555`)
- `redesign-external-system-structure` (`process-flow.yaml:595-607`)
- `refactor-system-structure` (`process-flow.yaml:639-651`)
- `refactor-test-structure` (`process-flow.yaml:675-687`)

```yaml
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
```

And the matching `sequence-flows` wired the triad into each CYCLE's start:

```yaml
      - {from: CHECK_CHECKLIST_PROGRESS,      to: GATE_CHECKLIST_PARTIALLY_DONE}
      - {from: GATE_CHECKLIST_PARTIALLY_DONE, to: STOP_CHECKLIST_PARTIALLY_DONE, when: "checklist-partially-done == true"}
      - {from: GATE_CHECKLIST_PARTIALLY_DONE, to: <first-real-step>,             when: "checklist-partially-done == false"}
      - {from: STOP_CHECKLIST_PARTIALLY_DONE, to: <first-real-step>}
```

Each CYCLE's `start:` was `CHECK_CHECKLIST_PROGRESS` (now becomes the first real step of that CYCLE).

### Post-CYCLE auto-tick (inside `move-to-in-acceptance`)

The `MARK_IN_ACCEPTANCE` node in `implement-ticket` calls action `move-to-in-acceptance`, which today does two things (see `internal/atdd/runtime/actions/bindings.go:349-351`):

```go
// moveToInAcceptance ticks every issue checklist box via
// Tracker.MarkChecklistComplete and sets the item status to "In
// acceptance" via Tracker.SetStatus.
```

After the cut, this action does only the `SetStatus` half. The `MarkChecklistComplete` call is removed.

## Code-side references touched at removal time

When re-introducing, these are the modules that were touched at removal — i.e., the same modules need touching again to re-register the resumed machinery:

### Action layer

- `internal/atdd/runtime/actions/bindings.go`:
  - Re-register the `check-checklist-progress` action (reads `Tracker.ReadSections`, parses the `Checklist` section into `(total, done)` counts, sets `ctx.State["checklist-partially-done"]` and `ctx.State["checklist_progress_summary"]`).
  - Re-add the `MarkChecklistComplete` call inside `moveToInAcceptance`.
- `internal/atdd/runtime/actions/bindings_test.go`: re-add the corresponding tests; the `fakeTracker.MarkChecklistComplete` stub already exists (panics today, would need a real impl on the fake).

### Gate layer

- `internal/atdd/runtime/gates/bindings.go`: re-register the `checklist-partially-done` gate binding (reads `ctx.State["checklist-partially-done"]` bool).
- `internal/atdd/runtime/gates/bindings_test.go`: re-add `TestChecklistPartiallyDone_TrueRoutes` and the false-routes counterpart.

### Tracker layer

- `internal/atdd/runtime/tracker/tracker.go`: re-add the `MarkChecklistComplete(ctx, issue) error` method to the `Tracker` interface.
- `internal/atdd/runtime/tracker/markdown/markdown.go`: re-implement the markdown-backed version (rewrites `- [ ]` → `- [x]` in the Checklist section of the ticket body file).
- `internal/atdd/runtime/tracker/github/github.go`: re-implement the GitHub-backed version (uses the GraphQL/REST mutation that replaces the body with all checkboxes ticked).
- Test files for both backends.

### Statemachine fixtures

- `internal/atdd/runtime/statemachine/transitions_test.go`: re-add fixtures exercising the `checklist-partially-done == true` and `== false` routes through each of the four CYCLEs (or whatever subset re-introduces the gate).

### Diagrams

- `docs/process-diagram.md` + SVGs regenerate to include the new prefix block.

(Verify the exact list at execute time by grepping `checklist|MarkChecklistComplete|check-checklist-progress|checklist-partially-done|checklist_progress_summary` after the rebuilt design is in place.)

## Open design questions for the rebuild

These are the questions worth answering **before** re-introducing checklist progress-tracking:

1. **Has atomicity actually broken?** What concrete CYCLE or workflow now commits per-item, justifying the resume gate? If we're re-adding speculatively, don't.
2. **Per-CYCLE prefix vs hoisted-to-implement-ticket.** The original removal had four copies of the same block. If re-introduced, hoist it to `implement-ticket` (between `PARSE_TICKET` and `GATE_TICKET_KIND`) so it lives in one place. See item 8 in the source plan for the hoist-vs-status-quo discussion.
3. **Operator-prompt wording.** Original: `"${checklist_progress_summary} Approve re-running this cycle?"`. If hoisted to `implement-ticket`, change `"this cycle"` → `"this ticket"` (the CYCLE isn't yet selected at the hoist point).
4. **Per-task-kind guard?** Stories/bugs are AC-driven (no checklist). On re-introduction, decide whether the gate runs for every ticket (no-op for non-task) or guarded by `ticket-kind == task`.
5. **Per-item commits vs whole-ticket commit.** Tied to Q1. If the CYCLE genuinely commits per-item, the gate needs to know which items have associated commits. The original implementation did NOT have this — it only read the body's checkbox state. If we re-introduce with per-item commits, the data model needs to grow (or commit-history scanning needs to be added).
6. **Auto-tick semantics.** The removed `MarkChecklistComplete` ticked *every* box at the end of the CYCLE — even items that hadn't been individually completed. That's symbolic "ticket done" marking, not real progress tracking. On re-introduction, decide whether to (a) keep that symbolic semantics, (b) tick only items the agent actually completed, or (c) drop the auto-tick entirely and let the agent tick boxes itself.

## Cross-references

- Triggered by: `plans/20260526-1730-bpmn-process-review.md` item 8.
- Related: `plans/20260526-1746-rebuild-onboard-external-system.md` (sibling stash plan — same cut-and-stash pattern).
