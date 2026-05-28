# Plan: Auto-default for loopable `agent: human` choosers (opt-in per node)

> 🛑 **PAUSED (2026-05-28).** Superseded in scope by
> [`20260528-1200-drop-choose-refactor-type-user-task.md`](20260528-1200-drop-choose-refactor-type-user-task.md),
> which addresses the same `CHOOSE_REFACTOR_TYPE` symptom structurally
> (deleting the user-task and starting `refactor` at
> `GATE_REFACTOR_TYPE_CHOICE`). That structural fix removes the
> redundant `Approve? [y/n]` in interactive mode AND eliminates the
> autonomous hang by letting the gateway binding's existing
> empty-input-defaults-to-`none` path handle `--auto`, so the
> per-node `auto-default:` mechanism designed below is no longer
> needed for this node.
>
> **Update (2026-05-28, post-supersede).** The Seam A code (Items 1, 2,
> 5, 7 — schema + parse-time validation + bind-time gates-registry
> cross-check + YAML annotation on `CHOOSE_REFACTOR_TYPE`) was committed
> on operator instruction in `e737f0a` despite this banner's earlier
> "stash or discard" guidance. **Whether plan 1200's Item 3 (delete the
> `auto-default:` schema) still proceeds is now an open question to be
> discussed separately** — see the matching note on plan 1200's Item 3.
> Until that's decided, the schema lives in HEAD but no dispatcher
> honours it, so net runtime behaviour is unchanged.
>
> Original supersede guidance kept below for design-trail
> traceability: the rejected "demote to gateway" objection in the
> Design section below is the alternative 1200 picks up and resolves.

## Context

Today every `user-task` with `agent: human` halts the run and demands an explicit y/n through `approval.Confirm(…, CategoryHuman, …)`. `CategoryHuman` is wired as the top tier of the approval ladder (`internal/approval/approval.go:53-60`, `:126-130`), and the dispatcher comments are loud about why:

- `internal/atdd/runtime/driver/driver.go:682-686` — "CategoryHuman is always in the resolved confirm set, so this always delegates to the interactive prompt regardless of --auto. The BPMN human-STOP author chose this STOP precisely because no machine decides it; --auto explicitly cannot opt out."
- `internal/atdd/runtime/agents/registry.go:48-53` — same invariant on the fallback dispatcher used by transitions tests.

This invariant is correct for every human-STOP we have today **except one**: `CHOOSE_REFACTOR_TYPE` inside the `refactor` TOP process (`internal/atdd/runtime/statemachine/process-flow.yaml:357-360`). That node is unusual:

- It is **loopable**: the four refactor-CYCLE branches all return to `CHOOSE_REFACTOR_TYPE`, and the `none` branch is a no-op exit to `REFACTOR_TOP_END`.
- The opportunistic-refactor call from `change-system-behavior` step 3 (`process-flow.yaml:458-472`) lands here. In autonomous mode, "no opportunistic refactor this iteration" is a perfectly legitimate outcome — and is in fact the only outcome the runtime can defensibly pick without an operator deciding which refactor to do.
- The gateway binding already handles "no answer" cleanly: `gates/bindings.go:371-391` reads `refactor-type-choice` off `ctx.State` first, falls back to a prompter, and **defaults empty input to `"none"`**. The plumbing for "auto-default to none" is already in place at the gateway; only the human-STOP in front of it stops us from using it.

Symptom that motivated this plan: in a `--auto` (autonomous) run of `change-system-behavior`, after `IMPLEMENT_AND_VERIFY_SYSTEM` finishes green, the engine prints

```
[trace] > REFACTOR_OPPORTUNISTICALLY  kind=call-activity process=refactor
[trace] > CHOOSE_REFACTOR_TYPE        kind=user-task agent=human
[CHOOSE_REFACTOR_TYPE] Choose refactor type (loopable; none = exit)
  Approve? [y/n]:
```

…and then halts forever waiting for stdin. The autonomous run is not autonomous.

**Goal.** Let the BPMN author opt a specific `agent: human` node into "in autonomous mode, write a fixed value to a binding and continue, instead of prompting." Apply it only to `CHOOSE_REFACTOR_TYPE` for now. Preserve the loud `CategoryHuman` invariant everywhere else — including the two existing human-STOP sites in `change-system-behavior` and friends, and the `ASK_HUMAN` node inside the `approve` primitive (which already has its own routing via `newApproveDispatcher` so this plan does not touch it).

## Design (resolved)

### One new node field: `auto-default`

Add an optional field to the BPMN node schema:

```yaml
- id: CHOOSE_REFACTOR_TYPE
  type: user-task
  agent: human
  name: "Choose refactor type (loopable; none = exit)"
  auto-default:
    binding: refactor-type-choice
    value: none
```

Semantics:

- Both sub-fields are required when `auto-default:` is present. The loader rejects half-specified `auto-default:` blocks at parse time.
- The field is only meaningful on `type: user-task` with `agent: human`. On any other node kind/agent the loader hard-errors (no silent ignore — a misplaced `auto-default:` is an authoring mistake, not a no-op).
- The `binding:` must be a string-coerced gateway binding (Outcome.Value, not Outcome.Bool). The loader cross-checks against the gates registry at load time. For `refactor-type-choice` this matches `gates/bindings.go:371-391`.
- `value:` is a literal string written to `ctx.State[binding]` before the dispatcher returns. No `${…}` placeholder expansion (kept simple — the use case is enum literals).

Schema sits next to the existing per-node fields on `statemachine.RawNode` (`internal/atdd/runtime/statemachine/load.go:28-56`):

```go
type RawAutoDefault struct {
    Binding string `yaml:"binding"`
    Value   string `yaml:"value"`
}

type RawNode struct {
    // …existing fields…
    AutoDefault *RawAutoDefault `yaml:"auto-default,omitempty"`
}
```

### Dispatcher behaviour

In `newHumanStopDispatcher` (`driver/driver.go:669-695`):

1. Expand `raw.Name` and print the banner exactly as today (so trace logs still show what was supposed to happen, in autonomous mode too).
2. If `raw.AutoDefault != nil` **and** `opts.Approval.Auto` is true **and** `opts.Approval.ConfirmFloor >= CategoryHuman` (i.e. fully autonomous with no escalation), write `ctx.Set(raw.AutoDefault.Binding, raw.AutoDefault.Value)` and return `Outcome{}` without prompting.
3. Otherwise prompt as today via `approval.Confirm(…, CategoryHuman, …)`.

Bullet 2 is deliberately narrow: an operator who runs `gh optivem implement --auto --confirm=test-commit` (i.e. autonomous below test-commit, prompt at or above) still gets prompted at the chooser. Only the "truly autonomous" path (`--auto` with the default `--confirm=human` floor, or no `--confirm`) skips the prompt. This keeps the loud "human means human" invariant the dominant case and treats `auto-default:` as a node-author-declared exemption that the operator can still override by tightening the floor.

The fallback `humanStop` in `agents/registry.go:61-66` gets the same logic so transitions tests and any non-driver code paths agree. (The registry version doesn't currently see `opts.Approval`; pass a `Resolved` through the registry constructor — see Item 4.)

### YAML annotation on `CHOOSE_REFACTOR_TYPE`

Add the block to the one node that opts in, with a doc comment above it explaining why this specific node is the exemption (loopable, no-op exit branch, gateway binding has a sensible default already). No other YAML changes.

### Validation

Three new validators at load time:

- `auto-default:` requires `type: user-task` AND `agent: human`. Anything else is a parse-time error with a fix-it message ("auto-default is only valid on `agent: human` user-tasks").
- `auto-default.binding:` must be a registered gateway binding. Resolution happens in the same pass that resolves gateway `binding:` fields. Unknown bindings are a parse-time error.
- `auto-default.value:` must non-empty. Empty value is a parse-time error (catches `value: ""` and `value:` typos).

These mirror the existing parse-time validation style for `category:` and `scope:` — fail loud at load, not at dispatch.

### What this does NOT do

- Does not change `CategoryHuman`'s semantics in `approval.Confirm`. Other human-STOPs still hard-prompt under `--auto`.
- Does not introduce a `--skip-human` or `--accept-defaults` flag. The exemption is per-node, declared by the BPMN author, not a global operator escape hatch.
- Does not change the `approve` primitive's `ASK_HUMAN` node. That has its own dispatcher and its own rejection-is-routable semantics; auto-defaulting an approval would be a policy decision, not a UX one, and is out of scope here.
- Does not auto-default any other refactor-related node. The four refactor-CYCLE branches inside the `refactor` TOP are call-activities, not user-tasks; they aren't blocked on stdin.

### Why not "demote CHOOSE_REFACTOR_TYPE to a gateway"

Considered and rejected. Removing the user-task entirely would mean the gateway binding's prompter (`gates/bindings.go:375`) becomes the operator-facing chooser. That works in autonomous mode (prompter returns empty → defaults to "none") but in interactive mode the operator loses the labeled chooser node in the diagram and trace — the gateway prompt is less discoverable and shows up as a one-shot prompt instead of a loopable menu. Keeping the user-task and adding a narrow `auto-default:` opt-in preserves the diagram, the trace, and the interactive UX, and only changes what autonomous mode does.

### Why not "treat `--auto` as bypassing all human-STOPs"

Considered and rejected. The other human-STOPs (operator authorize, dispatch confirmations, fix-* approval, release approval) exist specifically because the BPMN author decided no machine should decide. Demoting them universally would silently auto-approve real human gates and break the load-bearing invariant in `approval.go:126-130`. The narrow `auto-default:` opt-in keeps the invariant and only opens an exit for nodes whose author explicitly declares the exit value.

## Items

> **Seam-A status (2026-05-28).** Items 1, 2, 5, 7 landed as one commit on
> the `statemachine` package + the shipped YAML. The schema, parse-time
> validators, Bind-time gates-registry lookup, the `CHOOSE_REFACTOR_TYPE`
> annotation, and the YAML schema comment-block reference are all in
> place. Tests cover the five parse-time placement/shape cases, the two
> Bind-time binding-lookup cases, and a shipped-YAML guard that pins
> `CHOOSE_REFACTOR_TYPE` as the sole auto-defaulting node.
>
> The remaining items (3, 4, 6, 8) are the dispatcher wiring + tests +
> manual smoke. They are deferred because plan
> `20260528-1145-output-levels-phase-detail.md` is in-flight against
> `internal/atdd/runtime/driver/driver.go` lines 676-680 — the exact
> `newHumanStopDispatcher` body Item 3 needs to extend. Re-pick this plan
> once plan 1145 lands.

3. **Teach `newHumanStopDispatcher` to honour `auto-default:` under fully-autonomous mode.** — ⏳ Deferred: blocks on plan `20260528-1145-output-levels-phase-detail.md` landing (same function body).
   - File: `internal/atdd/runtime/driver/driver.go` — modify `newHumanStopDispatcher` (~line 669-695).
   - New branch *before* `approval.Confirm`:
     ```go
     if raw.AutoDefault != nil &&
        opts.Approval.Auto &&
        opts.Approval.ConfirmFloor >= approval.CategoryHuman {
         ctx.Set(raw.AutoDefault.Binding, raw.AutoDefault.Value)
         fmt.Fprintf(opts.Stdout, "  [auto-default] %s = %s\n",
             raw.AutoDefault.Binding, raw.AutoDefault.Value)
         return statemachine.Outcome{}
     }
     ```
   - The `[auto-default]` line is intentional: in autonomous mode the operator may be reading the log afterwards, and seeing "we wrote `refactor-type-choice = none` and moved on" is materially more informative than seeing the trace silently skip the prompt.
   - **Tests** (`driver_test.go`): three table cases — (a) `--auto` default floor + `auto-default:` ⇒ no prompt, state set, Outcome{} returned, (b) `--auto --confirm=test-commit` + `auto-default:` ⇒ prompt (floor too tight to opt out), (c) no `--auto` ⇒ prompt.

4. **Same logic in the registry fallback `humanStop`.** — ⏳ Deferred: paired with Item 3 (only useful once the driver-side branch is live).
   - File: `internal/atdd/runtime/agents/registry.go:61-66`.
   - Currently constructs an empty `approval.Resolved{}` and calls `approval.Confirm`. To honour `auto-default:` here it needs to see (a) the live `Resolved` policy and (b) the node's `RawNode`. The cleanest threading:
     - Change `agents.New()` to `agents.New(resolved approval.Resolved)` and stash it on `Registry`.
     - Change `Registry.Lookup(name)` to return a closure that has access to both `resolved` and (via the call site in `statemachine.run.go`) the current `RawNode`. The existing `AgentFn func(name string) NodeFn` signature already gives us the node at dispatch time via the `Context`, BUT `Context` does not currently carry `RawNode`. Two options:
       - **4a (preferred):** Add `ctx.Node *RawNode` (set by `Engine.Run` at each step). Existing tests don't read `Node` so the field is purely additive. This also unblocks future per-node behaviour without further plumbing.
       - **4b:** Plumb `RawNode` through a closure capture at registration time. Cheaper than 4a but couples `agents.Registry` to the statemachine in a new way.
   - Decision: **4a**. The runtime already exposes `Node` shape on diagnostics paths (trace decorator) — making it available on `Context` is a small generalisation that earns its keep beyond this plan.
   - **Tests** (`registry_test.go` or sibling): same three table cases as Item 3, against `humanStop`.

6. **Transition test: opportunistic-refactor exit under `--auto`.** — ⏳ Deferred: depends on Item 3 dispatcher branch being live.
   - File: `internal/atdd/runtime/statemachine/transitions_test.go` (or sibling).
   - Walk `change-system-behavior` end to end with a stubbed `--auto` Resolved and the real `refactor` sub-process. Assert no human-STOP prompt is hit and the run terminates at `CHANGE_SYSTEM_BEHAVIOR_END`.
   - Walk the same path with `--auto --confirm=test-commit`. Assert the chooser DOES prompt (and the test feeds "none" via stdin to terminate cleanly).

8. **Smoke: run `gh optivem implement --auto` end-to-end on a fixture ticket** that goes through `change-system-behavior`. — ⏳ Deferred: depends on Item 3.
   - Confirm the opportunistic-refactor step prints the `[auto-default]` line and the run completes without prompting. This is the goalpost — Items 1-6 are the wiring; this is the proof.

## Verification

- `go test ./internal/atdd/runtime/...` passes with the new tests in Items 1-6.
- Manual smoke (Item 8) shows a clean autonomous run through `change-system-behavior` with the chooser auto-defaulting.
- Interactive mode (no `--auto`) is unchanged — `gh optivem implement` without `--auto` still hits the chooser prompt.
- Tightened-floor autonomous (`--auto --confirm=test-commit`) still hits the chooser prompt — verify by running the same fixture ticket with the tighter floor and observing the prompt.
- No other `agent: human` node carries `auto-default:` after this plan — `grep -n "auto-default:" internal/atdd/runtime/statemachine/process-flow.yaml` returns exactly one hit (`CHOOSE_REFACTOR_TYPE`).

## Out of scope (deliberately)

- Auto-defaulting `ASK_HUMAN` in the `approve` primitive.
- A `--accept-defaults` global flag or any other operator-side escape hatch.
- Changing what `CategoryHuman` means in `approval.Confirm`.
- Auto-defaulting any node that's not a `user-task` with `agent: human`.
- Generalising `auto-default:` to `${…}` placeholder expansion. Add when a second use case demands it; not before.
