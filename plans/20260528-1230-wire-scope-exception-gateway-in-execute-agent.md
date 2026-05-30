# Wire `scope_exception_requested` gateway into `execute-agent`

## Decision (resolved 2026-05-30): wire the gateway — do not fall through to FIX

The 2026-05-28 open question was whether a scope exception should route to
a new `STOP_SCOPE_VIOLATION` halt, or just fall through to `FIX`.

**Resolved: wire the halt (this plan).** The "fall through to FIX" framing
rested on the premise that *FIX is where the BPMN gets corrected* — but that
is false against the code. `FIX` is `{from: FIX, to: RUN_AGENT}`
(`process-flow.yaml:2166`): it dispatches a fixer agent and loops back to
**re-run the same agent against the same `scope:`**. A fixer edits code/test
artifacts, not the `read:`/`write:` scope declarations in `process-flow.yaml`.
So falling through to FIX does not correct a too-narrow scope — it
re-dispatches into the identical scope and loops. A scope exception being
"abnormal — the BPMN was authored wrong, a human must widen the scope" is an
argument *for* a loud halt, not against it.

Why a dedicated halt over the alternatives:

- **vs. fall-through-to-FIX:** the only loop-free, semantically honest
  option; FIX cannot fix a mis-authored scope.
- **vs. reusing `EXECUTE_AGENT_OUTPUT_REJECTED_END`:** that terminal's
  doc-block (`process-flow.yaml:2141`) pins it to "operator rejected broken
  output." "Agent refused on scope" is a distinct verdict; merging them
  discards a distinction the codebase deliberately maintains.
- **Dead-code closure:** the `scope-exception-requested` binding
  (`internal/atdd/runtime/gates/bindings.go:209`) and the live envelope
  emission across the writing-agent prompts (plan `20260528-1150`, committed)
  are collected-and-ignored today. This plan is the only resolution that
  gives them a consumer rather than leaving dead code.

## Relationship to plan `20260530-1339-bound-verify-fix-loop` (landed 2026-05-30, `56fe4b9`)

Plan 1339 has already been executed and committed. Confirmed against the
merged tree:

- It added a general `max-visits` / `on-max-visits` node mechanism and a
  `FIX_LOOP_EXHAUSTED` error-end-event, but applied the cap **only** to the
  verify-subprocess FIX nodes (`FIX_UNEXPECTED_FAILING_TESTS`,
  `FIX_UNEXPECTED_PASSING_TESTS`).
- It did **not** create `STOP_SCOPE_VIOLATION` and did **not** cap
  `execute-agent`'s `FIX`. So this plan is still the sole producer of
  `STOP_SCOPE_VIOLATION` (no node-creation conflict), and `execute-agent`'s
  `FIX → RUN_AGENT` loop is still uncapped — the decision rationale above
  holds unchanged.

1339's `max-visits` / `on-max-visits` is now available as a complementary
backstop, but it is orthogonal to this plan: the gateway fires up-front on
the agent's explicit envelope, before any FIX attempt; the cap catches
runaway loops after the fact. Applying a cap to `execute-agent`'s FIX is out
of scope here.

## Background

Plan `20260528-1150-scope-exception-envelope-on-all-prod-agent-mids.md`
made the scope-exception envelope (`scope-exception-files=…`,
`scope-exception-reason=…`) emittable by **every** writing-agent MID,
not just those with a flag-level `outputs:` block. After that fix the
keys correctly land in `ctx.State` via `validate-outputs-and-scopes`.

But the `scope-exception-requested` gate binding
(`internal/atdd/runtime/gates/bindings.go:202`) is registered with no
`gateway` node consuming it in
`internal/atdd/runtime/statemachine/process-flow.yaml`. So today: the
agent's "I went out of scope on purpose" signal is *available* in
ctx state, and *nothing routes on it*. An out-of-scope write still
falls through `outputs-and-scopes-valid == false` → `GATE_FIX_ON_FAILURE`
→ `FIX`, exactly the FIX-loop the envelope was supposed to bypass.

This plan wires the gateway and the new `STOP_SCOPE_VIOLATION`
end-event into `execute-agent` so the envelope actually short-circuits
the FIX path.

## Target state

In the `execute-agent` process:

```
RUN_AGENT
  → VALIDATE_OUTPUTS_AND_SCOPES
    → GATE_SCOPE_EXCEPTION_REQUESTED   ← new gateway
        scope-exception-requested == true  → STOP_SCOPE_VIOLATION (new error-end-event)
        scope-exception-requested == false → GATE_OUTPUTS_AND_SCOPES_VALID (existing)
          ↓ true                              → APPROVE_POST
          ↓ false                             → GATE_FIX_ON_FAILURE → FIX | APPROVE_POST
```

**Placement choice.** The new gateway sits between
`VALIDATE_OUTPUTS_AND_SCOPES` and `GATE_OUTPUTS_AND_SCOPES_VALID`, not
on the `false`-branch of the latter. Reason: the envelope is the
agent's *explicit* signal that the cycle should halt with a scope
verdict, independent of whether `outputs-and-scopes-valid` happens to
be false for some unrelated reason. Putting the exception check first
makes the precedence obvious in the YAML and matches the binding
docstring's intent ("skipping DISABLE / Layer 2 / COMMIT").

`STOP_SCOPE_VIOLATION` is modelled as an `error-end-event` (same
shape as `EXECUTE_AGENT_OUTPUT_REJECTED_END`) — a deliberate
workflow halt, not a soft skip. The caller phase decides what to do
with the error verdict; this MID's contract is "I refused to write
outside scope, here's the envelope."

## Items

- [ ] **Item 6: Live rehearsal check (deferred, no code).**
  ⏳ Deferred until the next prod-agent ATDD dispatch that legitimately
  needs the envelope. Verify in the wild:
  1. A `system-implementer` (or similar) MID emits
     `scope-exception-files=…` via `gh optivem output write`.
  2. The cycle terminates at `STOP_SCOPE_VIOLATION`, **not** `FIX`.
  3. The orchestrator surfaces the envelope payload
     (`scope-exception-files`, `scope-exception-reason`) in the halt
     summary so the operator sees *why* the agent refused.

  Same caveat as the deferred item in
  `20260528-1150-scope-exception-envelope-on-all-prod-agent-mids.md`:
  this can only be confirmed by a real dispatch, not by unit tests.

## Out of scope

- Adding `scope-exception-files` / `scope-exception-reason` to the
  MID-level `outputs:` declarations of writing-agent MIDs that don't
  already have them. The previous plan made these emittable
  *regardless* of MID-level declaration; they ride the envelope
  channel and the gate reads them from ctx. Tightening the MID
  contract to also list them is a separate cleanup.
- Changing how `STOP_SCOPE_VIOLATION` is surfaced to the operator
  (UI, summary text, halt-reason formatting). The error-end-event
  itself is enough to halt the workflow; presentation polish is a
  follow-up if/when the rehearsal shows it's needed.
- Phase-scope **Layer 2** (`phase-scope-clean` binding). That gate
  already runs in a different process (the per-phase scripted
  post-check, not `execute-agent`); this plan only touches Layer 1
  (the agent-triggered escape hatch).
