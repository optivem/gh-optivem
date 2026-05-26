# Fix MARK_* state-transition routing — replace `update-ticket` wrapper with direct service-task actions

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-26T10:06:14Z`

> ⚠️ **Blocked on `plans/20260526-0907-legacy-bindings-dead-code-audit.md`.**
> That plan is currently in flight and touches
> `internal/atdd/runtime/actions/bindings.go` — the same file this plan
> modifies. Pick this up AFTER 0907 completes (per
> `feedback_check_concurrent_agents` / `feedback_concurrent_agent_collision`).
> See **Coordination with in-flight 0907** below — 0907 may delete two
> bindings this plan revives.

## Origin / incident

Rehearsal at 2026-05-26T11:41 against issue #61 (config
`gh-optivem-monolith-typescript.yaml`) failed with:

```
FAIL RUN_AGENT -> clauderun: prompt has unfilled placeholders after substitution: ${parsed_concepts}, ${ticket_source}
  this usually means the field was not seeded into Context.State before dispatch — check seedScopeState and preResolveIssue
```

Trace fired `MARK_IN_PROGRESS` (first node of `implement-ticket`), which
dispatched the `update-ticket` subprocess, which called `execute-agent`
with hardcoded `task-name: update-ticket`, which loaded the AC-writing
Claude prompt at `internal/assets/runtime/prompts/atdd/update-ticket.md`.
That prompt's `${parsed_concepts}` and `${ticket_source}` placeholders
are never seeded on the state-transition path → dispatch error.

## Root cause

Two unrelated concerns share the name `update-ticket`:

- **`update-ticket` subprocess** (`process-flow.yaml:1390-1403`) — a
  state-transition wrapper called 4× from MARK_* nodes (lines 172, 182,
  237, 272) with a `target-state:` param.
- **`update-ticket` agent prompt**
  (`internal/assets/runtime/prompts/atdd/update-ticket.md`) — overwrites
  two AC sections from the parsed-concepts artifact. Nothing to do with
  state transitions.

The subprocess hardcodes `task-name: update-ticket` and dispatches the
AC-writing agent on every MARK_*. The agent's `${parsed_concepts}`
placeholder is only seeded by `materialize_parsed_concepts` in the
refine-acc cycle, never on the MARK_* path. The agent's
`${ticket_source}` placeholder has **no producer anywhere in the
codebase** (grep returns 0 matches in `internal/atdd/runtime/`) — the
prompt was never wireable.

Meanwhile, `bindings.go` already has the correct service-task pattern:

- `move_to_in_progress` (line 277) — calls `Tracker.SetStatus(handle, "In progress")`.
- `move_to_in_acceptance` (line 294) — ticks checklist via
  `markChecklistComplete`, then `Tracker.SetStatus(handle, "In acceptance")`.

Neither is referenced from `process-flow.yaml` — both are dead bindings
today (called out in
`plans/20260526-0907-legacy-bindings-dead-code-audit.md` as deletion
candidates).

## Direction (decided 2026-05-26)

- **D1 — Replace, don't rename.** Eliminate the `update-ticket` wrapper
  subprocess entirely. Each MARK_* call site becomes a direct
  service-task referencing one of 4 discrete action bindings (one per
  canonical state). No shared logic to wrap; renaming the wrapper to
  `mark-ticket` would be a single-node passthrough.
- **D2 — Discrete actions, stringly-typed Tracker for now.** Four
  bindings (`move-to-in-refinement`, `move-to-ready`,
  `move-to-in-progress`, `move-to-in-acceptance`). Tracker interface
  stays `SetStatus(ctx, handle, "<state-string>")` — matches existing
  code, smallest blast radius to unblock the rehearsal. Typed Tracker
  + per-tracker column mapping is real portability work and gets its
  own plan (operators whose ticketing system uses different column
  labels will hit this; see "Out of scope" below).
- **D3 — Keep checklist tick coupled in `move_to_in_acceptance`.**
  Preserve existing behaviour: 3 pure state-transition actions + the
  4th tick-then-SetStatus pair. Lowest-risk shape for an asap unblock.
- **D4 — Delete the AC-writing agent prompt** (`update-ticket.md`) as
  dead code: no producer for `${ticket_source}`, no working dispatch
  site, no caller. The AC-writeback feature gets its own plan when
  actually needed (per
  `plans/upcoming/20260525-1753-implement-pre-refine-check-and-post-refactor-offer.md`).
- **D5 — Kebab-named action registrations.** Per the kebab-everywhere
  convention (commit `a10ce21`), all 4 action bindings register under
  kebab names (`move-to-in-progress`, not `move_to_in_progress`). If
  the in-flight 0907 audit has already deleted the snake-named
  versions, we re-create them directly under their kebab names rather
  than restoring snake and then renaming.

## Supersedes / cross-references

- **Supersedes**
  `plans/20260526-0832-process-diagram-cleanup.md` **Item 9**
  (decisions Q9.1/Q9.2: rename `update-ticket` → `mark-ticket`). Mooted
  because the wrapper subprocess is eliminated entirely, not renamed.
  After this plan ships, Item 9 of the cleanup plan can be marked done
  + cross-referenced here.
- **Blocked on**
  `plans/20260526-0907-legacy-bindings-dead-code-audit.md` (in flight).
- **Related future work**:
  `plans/upcoming/20260525-1753-implement-pre-refine-check-and-post-refactor-offer.md`
  (post-refine AC writeback — would re-introduce a dispatch site for an
  AC-writing agent, but with proper wiring and a producer for
  `${ticket_source}`).

## Coordination with in-flight 0907

`plans/20260526-0907-legacy-bindings-dead-code-audit.md` lists
`move_to_in_progress` and `move_to_in_acceptance` (lines 69, 71) as
legacy bindings candidate for deletion. This plan **revives** them
under kebab names.

When picking up this plan:

1. Re-grep `internal/atdd/runtime/actions/bindings.go` for
   `moveToInProgress` and `moveToInAcceptance`.
2. **If both Go funcs are still present** (0907 left them as live, or
   still hadn't run): rename their `r.Register("move_to_..."` lines to
   kebab in Item 1. Bodies untouched.
3. **If either Go func has been deleted by 0907**: restore from git
   history. The last known-good revision is `1587951` (HEAD at the
   time of this plan) — `git show 1587951:internal/atdd/runtime/actions/bindings.go`
   at lines 277-307 contains both `moveToInProgress` and
   `moveToInAcceptance`. Restore the implementations + add their kebab
   registrations in Item 1.

Per `feedback_check_concurrent_agents`, do **not** add a pickup marker
to this plan until 0907's marker is gone from
`plans/20260526-0907-legacy-bindings-dead-code-audit.md`.

## Items

### Item 8 — Rebuild + rerun the failing rehearsal

```powershell
go build -o gh-optivem.exe .
bash ../shop/../gh-optivem/scripts/atdd-rehearsal.sh 61 --config gh-optivem-monolith-typescript.yaml
```

Expected: `MARK_IN_PROGRESS` runs as a service-task and succeeds via
`Tracker.SetStatus`; flow continues into `GATE_TICKET_KIND`.

## Verification

- `go test ./internal/atdd/runtime/... -p 2` (per
  `feedback_go_test_windows`).
- Full rehearsal of issue #61 progresses past `MARK_IN_PROGRESS`.
- `gh optivem architecture show` regenerates without referencing the
  `update-ticket` subprocess.
- `grep -r "update-ticket" internal/` returns no live references
  outside of intentional removal-context comments.

## Out of scope

- **Tracker interface typed-state rework** — the existing
  `Tracker.SetStatus(ctx, handle, "<string>")` keeps the
  user-ticketing-column-name fragility (each tracker adapter is
  hardcoded to recognize "In progress" / "In refinement" / "Ready" /
  "In acceptance"). Operators whose board columns are named
  differently will need this fixed. Separate plan when prioritized.
- **Post-refine AC writeback** — the original `update-ticket.md`
  prompt was *aspirationally* the writeback agent. Its proper home is
  a future plan tied to
  `plans/upcoming/20260525-1753-implement-pre-refine-check-and-post-refactor-offer.md`,
  which will need a producer for `${ticket_source}` (and possibly a
  redesigned agent prompt rather than recovering this one from git).
- **Other generic MID names** (`commit`, `compile`, `run-tests`,
  `approve`) — 0832 plan Q9.3 (deferred). This plan touches only the
  `update-ticket` / MARK_* shape.
