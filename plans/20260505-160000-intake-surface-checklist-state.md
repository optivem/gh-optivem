# Surface checklist state at intake

## Motivation

A rehearsal of `atdd implement-ticket --issue 61` (system-interface-redesign,
TS monolith scope) parsed a ticket whose Checklist section already had every
item marked `[x]`:

```
ticket_checklist=- [x] Rename "New Order" card on the home page to "Place Order"
- [x] Rename the SKU input aria-label from "SKU" to "Product SKU"
```

The pre-checked state is ambiguous on its face:

- **(a) Work already done.** Someone ticked the items by hand — the cycle should
  exit cleanly without modifying anything.
- **(b) Stale checklist.** The boxes were checked in a prior ticket revision or
  by mistake — the underlying code change still needs to be made.

The current intake (`actions/bindings.parseTicketBody`,
`internal/atdd/runtime/actions/bindings.go:378-407`) sets `ticket_checklist`
into Context state and prints a single line to stdout:

```
Parsed #61 (task): all required sections present.
```

There is no operator-visible signal that the parsed checklist arrives
pre-checked. The full text appears later in trace output (`PARSE_BODY -> …
state: ticket_checklist=…`), but trace is verbose, the checklist is one of
many keys on a single long `state:` line, and operators don't read trace
line by line.

**Consequence.** On the rehearsal run, the operator had no chance to choose
between (a) and (b) at intake. The downstream Task Agent grepped the code,
found the strings still present, and proceeded as if (b) — which conflated
with the unrelated substitution bug that was already in flight (see
[`20260505-150000-fix-prompt-substitution-and-add-prompt-log.md`](20260505-150000-fix-prompt-substitution-and-add-prompt-log.md))
to produce a multi-tree edit that wasn't what anyone wanted.

The fix proposed here is small and intake-local: at parse time, count
`[x]` and `[ ]` items, print a structured summary the operator can act on,
and (in interactive mode) prompt for confirmation when every item is
already checked. The agent layer is left alone — this plan does not commit
to a "short-circuit when all `[x]`" behaviour because we don't yet know
whether (a) or (b) is the more common case post-fix.

## Items

Ordered by dependency. Item 1 is the parser change; items 2-3 are the
operator-facing surface.

### 1. Extend `intake.Parse` to surface checklist counts

**Files:**
- `internal/atdd/runtime/intake/parse.go` (`Result.Checklist`,
  `ExtractSection` or a sibling)
- `internal/atdd/runtime/intake/parse_test.go` (count assertions)

**Change:** add count fields to the parser output so callers don't each
re-parse the body. Two shape options:

```go
// Option A: extra counts on Result.
type Result struct {
    ...
    Checklist                Section
    ChecklistTotal, ChecklistChecked int
}

// Option B: structured Checklist type.
type ChecklistResult struct {
    Section                          // embed: Heading / Body / Found
    Items []ChecklistItem            // one per `- [ ]` / `- [x]` line
}
type ChecklistItem struct {
    Text    string
    Checked bool
}
```

Option B is the better long-term shape — it's the structure other readers
(`tickRemoteChecklist` in particular, `bindings.go:806`) already
re-implement by re-scanning the body. Folding the structure into the parser
deletes that duplication and gives any future consumer a typed item list
to walk. Option A is cheaper but kicks the can.

**Recommend Option B.** Effort difference is ~30 minutes; the duplication
deletion downstream pays it back.

**Risk:** changing `Result.Checklist` shape touches every existing call site
that reads it. Today there's exactly one (`actions/bindings.go:404`) plus
the parser tests, so the surface is small. Keep `Body` on the embedded
`Section` so the existing `ticket_checklist` Context value (the raw
markdown) still flows through unchanged for the agent prompt's
`${checklist}` substitution.

**Effort:** ~1 hour including tests.

### 2. Print a checklist summary after `parseTicketBody`

**Files:**
- `internal/atdd/runtime/actions/bindings.go` (`parseTicketBody`,
  ~10 lines after the existing "all required sections present" line)
- `internal/atdd/runtime/actions/bindings_test.go` (capture stdout, assert
  on the new lines)

**Change:** when the parsed ticket has a Checklist, append a structured
summary block to `a.deps.Stdout` after the existing print. Skip when the
ticket has no Checklist (e.g. story, bug — neither requires the section).

Format:

```
Parsed #61 (task): all required sections present.
Checklist (2 items, 2 already [x]):
  [x] Rename "New Order" card on the home page to "Place Order"
  [x] Rename the SKU input aria-label from "SKU" to "Product SKU"
```

When 0 of N are checked:

```
Checklist (3 items, 0 already [x]):
  [ ] Rename …
  [ ] Move …
  [ ] Delete …
```

When mixed:

```
Checklist (3 items, 1 already [x]):
  [x] Rename … (already done)
  [ ] Move …
  [ ] Delete …
```

The "already done" suffix on individual `[x]` lines in the mixed case is
the cheapest way to make the partial-completion state visually scannable
without a second summary block. It does not appear in the all-done or
none-done cases because the count line already conveys the state.

**Why print, not just trace.** Trace output is for debugging the state
machine, not for operator decision support. The intake summary belongs
on the operator's primary surface (stdout), alongside the existing
"Resolved issue …" and "Classified … as task." banners.

**Effort:** ~45 minutes including stdout-capture tests.

### 3. Interactive-mode confirmation when all items are already `[x]`

**Files:**
- `internal/atdd/runtime/actions/bindings.go` (`parseTicketBody`)
- `internal/atdd/runtime/actions/bindings_test.go` (Yes / No / abort
  paths)

**Change:** in interactive mode (i.e. when `opts.Stdin` is a TTY and
`opts.Autonomous` is false) and when `ChecklistTotal > 0 &&
ChecklistChecked == ChecklistTotal`, prompt for confirmation after the
summary block:

```
All 2 checklist items are already marked [x].
This usually means either:
  (a) the work was already done — run `gh issue close #61` and skip this cycle.
  (b) the checklist is stale — proceed and the agent will inspect the code.

Proceed with the cycle? [y/N]
```

Default to `N` (don't proceed) so a stray Enter doesn't fire a
multi-tree edit on a ticket that was meant to be skipped. Operator types
`y` to proceed; anything else exits cleanly with rc=0 and a "skipped per
operator request" line.

**Skip the prompt in autonomous mode** (`gh optivem atdd implement-ticket
--autonomous` or any code path where stdin is not a TTY). Print a warning
instead:

```
warning: all 2 checklist items are already marked [x] — proceeding anyway in autonomous mode
```

The warning is the autonomous-mode echo of the interactive prompt: same
information, no blocking. An operator running unattended may still see it
in logs and decide to investigate.

**Why default-N rather than default-Y.** The destructive case (proceed
when the work is already done) is materially worse than the conservative
case (skip when proceeding would have been fine). Skipped tickets can be
re-run with one command; over-eager edits across four parallel
implementations cost a worktree-discard and a re-rehearsal — exactly the
class of damage we're trying to make rarer.

**Effort:** ~1-1.5 hours including the TTY-detection seam, the prompt
helper, and tests for the three branches (yes / no / autonomous-mode warn).

## Out of scope

- **Agent-layer handling of pre-checked items.** The Task Agent's
  decision logic (proceed / stop / verify-and-decide) is left untouched
  in this plan. Re-rehearse after items 1-3 land plus the substitution
  fix from the sister plan; if the post-fix agent behaviour is still
  wrong, write a follow-up with that data in hand.
- **Auto-modifying the ticket body to uncheck items.** Speculative and
  destructive — the operator may have intentionally pre-checked items
  for a reason (audit trail, partial-completion tracking).
- **Surfacing pre-checked AC scenarios for stories / bugs.** Stories and
  bugs use Acceptance Criteria, not Checklist; if there's a similar
  pre-checked-state ambiguity there, it's a separate intake concern.
  Defer until someone reports it.
- **Persisting the intake summary to the prompt log.** The summary is an
  operator-facing artifact; the agent prompt already carries the full
  checklist text. Keeping these two surfaces independent means the
  summary format can evolve without changing what the agent receives.

## Total effort

~3 hours including tests. Single PR. Touches `intake/`, `actions/`, and
their tests — no driver-level or clauderun-level changes.

## Coordination with the substitution-fix plan

This plan and
[`20260505-150000-fix-prompt-substitution-and-add-prompt-log.md`](20260505-150000-fix-prompt-substitution-and-add-prompt-log.md)
both surfaced from the same #61 rehearsal. They are functionally
independent — either can land without the other — but landing the
substitution fix first is preferable because:

1. The substitution fix is the actual bug that produced the wrong
   output. The intake summary is a UX improvement that helps catch
   *future* bugs of a different shape.
2. Re-running the rehearsal post-substitution-fix gives us cleaner
   data on what `atdd-task` does with a pre-checked checklist when
   the rest of its prompt is correct — the data needed to decide
   the deferred "agent-layer handling" question above.
