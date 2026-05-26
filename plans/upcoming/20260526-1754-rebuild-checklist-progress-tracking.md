# Reintroduce checklist progress-tracking in BPMN

Spinoff from `plans/20260526-1730-bpmn-process-review.md` item 8. The checklist progress-tracking machinery (pre-CYCLE resume gate + post-CYCLE auto-tick) was **removed** on 2026-05-26 because the agent works atomically — a CYCLE either completes the whole ticket or it doesn't, so tracking partial checklist progress at runtime didn't earn its keep.

The **spec/input** role of the checklist still works: the agent still reads the `Checklist` section from the ticket body as the list of sub-items to do. Only the BPMN gating + post-step ticking was cut.

This file is the **reintroduction spec** for the day atomicity stops holding. It captures:

- **Verbatim capture** of the machinery as it stood pre-removal (so it can be reinserted line-by-line if needed).
- **Reintroduction questions** to answer before any rebuild — when, how, with what semantics.

The proper reintroduction design is deliberately deferred. Don't rebuild until atomicity actually breaks (e.g., a CYCLE that genuinely commits per-item and supports resume).

## Why removed (2026-05-26)

The pre-CYCLE resume gate (`CHECK_CHECKLIST_PROGRESS` → `GATE_CHECKLIST_PARTIALLY_DONE` → `STOP_CHECKLIST_PARTIALLY_DONE`) exists to ask the operator "this checklist is partially complete — proceed?" on cycle re-runs. But the agent today completes the ticket atomically (commits at the end of the CYCLE, not per-item), so a failed mid-run leaves no resumable partial state. The gate fires only on re-runs of already-completed tickets — operationally noise.

The post-CYCLE auto-tick (`Tracker.MarkChecklistComplete` inside `move-to-in-acceptance`) records "every checklist item is done" on the ticket body when the CYCLE finishes. It's documentation of completion, not workflow state. If we want that record, the agent can tick boxes as part of its own task; the BPMN doesn't need to do it.

If atomicity stops holding later (e.g., we introduce a CYCLE that commits per-item and supports genuine resume), this plan's verbatim capture describes what to re-add.

## Verbatim capture from `process-flow.yaml` (for later reinsertion)

### Pre-CYCLE prefix triad (identical block, copy-pasted into the start of 5 CYCLEs)

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

The matching `sequence-flows` for each CYCLE (X = the CYCLE's first real step — the one that was the CYCLE's `start:` before the triad was prepended):

```yaml
      - {from: CHECK_CHECKLIST_PROGRESS,      to: GATE_CHECKLIST_PARTIALLY_DONE}
      - {from: GATE_CHECKLIST_PARTIALLY_DONE, to: STOP_CHECKLIST_PARTIALLY_DONE, when: "checklist-partially-done == true"}
      - {from: GATE_CHECKLIST_PARTIALLY_DONE, to: <X>,                           when: "checklist-partially-done == false"}
      - {from: STOP_CHECKLIST_PARTIALLY_DONE, to: <X>}
```

Each affected CYCLE's `start:` is currently `CHECK_CHECKLIST_PROGRESS` (post-removal, it becomes `<X>`).

### Per-CYCLE occurrences (where the triad is wired today)

| CYCLE | Node range (process-flow.yaml) | `<X>` (first real step) | Removed by |
| --- | --- | --- | --- |
| `onboard-external-system` | 416–460 | `IDENTIFY_EXTERNAL_SYSTEM` | sibling plan `20260526-1746-rebuild-onboard-external-system.md` (whole CYCLE removed) |
| `redesign-system-structure` | 541–577 | `UPDATE_SYSTEM_DRIVER_ADAPTERS` | **this plan** |
| `redesign-external-system-structure` | 593–629 | `UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS` | **this plan** |
| `refactor-system-structure` | 637–669 | `IMPLEMENT_AND_VERIFY_SYSTEM` | **this plan** |
| `refactor-test-structure` | 673–702 | `REFACTOR_AND_VERIFY_TESTS` | **this plan** |

## Verbatim capture from `internal/atdd/runtime/actions/bindings.go` (post-CYCLE auto-tick)

The `MARK_IN_ACCEPTANCE` node in `implement-ticket` calls action `move-to-in-acceptance`. Today that action does two things — ticks every checklist box, then sets the item status to "In acceptance":

```go
// moveToInAcceptance ticks every issue checklist box via
// Tracker.MarkChecklistComplete and sets the item status to "In
// acceptance" via Tracker.SetStatus. Both halves error out hard on
// failure — a missing Status option or a permission failure on edit is
// a misconfiguration the operator must fix before re-running.
func (a actions) moveToInAcceptance(ctx *statemachine.Context) statemachine.Outcome {
    if err := a.markChecklistComplete(ctx); err != nil {
        return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: tick checklist: %w", err)}
    }
    handle := ctx.GetString("issue_handle")
    if handle == "" {
        return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: issue_handle not in Context")}
    }
    if err := a.deps.Tracker.SetStatus(context.Background(), handle, "In acceptance"); err != nil {
        return statemachine.Outcome{Err: fmt.Errorf("move-to-in-acceptance: %w", err)}
    }
    fmt.Fprintln(a.deps.Stdout, "Moved card to In acceptance.")
    return statemachine.Outcome{}
}

// markChecklistComplete is the shared helper used by move-to-in-acceptance
// to tick every `- [ ]` checkbox in the issue body via
// Tracker.MarkChecklistComplete. A missing or non-positive issue_num is
// silently skipped (transitions tests and dry-runs that don't seed a real
// issue still exercise the SetStatus half).
func (a actions) markChecklistComplete(ctx *statemachine.Context) error {
    if issueNum, err := strconv.Atoi(ctx.GetString("issue_num")); err != nil || issueNum <= 0 {
        return nil
    }
    issue, err := issueFromContext(ctx)
    if err != nil {
        return err
    }
    return a.deps.Tracker.MarkChecklistComplete(context.Background(), issue)
}
```

Action registry line at `bindings.go:229`:

```go
r.Register("check-checklist-progress", a.checkChecklistProgress)
```

The `checkChecklistProgress` action itself lives at `bindings.go:431` (reads the parsed `Checklist` body and sets `ctx.State["checklist-partially-done"]` + `ctx.State["checklist_progress_summary"]`).

## Reintroduction — revisit later

If atomicity stops holding (e.g., a CYCLE genuinely commits per-item and supports resume), the rebuild needs to answer at least these questions and decide on the touch points before re-adding any machinery:

- Has atomicity actually broken? Which CYCLE commits per-item, justifying the resume gate?
- Per-CYCLE prefix vs. hoisted to `implement-ticket` (single block between `PARSE_TICKET` and `GATE_TICKET_KIND`)?
- Operator-prompt wording — `"this cycle"` vs `"this ticket"` depending on hoist point.
- Per-task-kind guard (stories/bugs have no checklist) — gate-for-all vs `ticket-kind == task`?
- Data model — body-only checkbox scan (today) vs commit-history scan (per-item commits)?
- Auto-tick semantics — symbolic "ticket done" mass-tick (today), only-items-completed, or drop the auto-tick entirely?
- Touch points to re-register: `actions/bindings.go` (`check-checklist-progress` + `markChecklistComplete` half of `moveToInAcceptance`), `gates/bindings.go` (`checklist-partially-done`), `tracker.go` interface + both backends, statemachine fixtures, diagrams.

These are deliberately left unresolved — they're decisions for the day this is actually picked up, not now.

## Cross-references

- Triggered by: `plans/20260526-1730-bpmn-process-review.md` item 8.
- Related: `plans/20260526-1746-rebuild-onboard-external-system.md` (sibling stash plan — removes the whole `onboard-external-system` CYCLE, including its copy of the checklist triad).
