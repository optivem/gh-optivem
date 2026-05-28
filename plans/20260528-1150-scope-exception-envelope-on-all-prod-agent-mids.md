# Scope-exception envelope must be available on every writing-agent MID

Items 1-5 landed in commit 288c543. Only the end-to-end regression
check (Item 6) remains; it requires a real ATDD rehearsal run, not a
unit/integration test, so it stays deferred until that rehearsal fires.

## Items

- [ ] Item 6: Regression check against rehearsal #71 — ⏳ Deferred:
  needs a live ATDD dispatch (`gh optivem` against an actual ticket)
  to confirm the in-the-wild fix. After the next prod-agent dispatch
  that legitimately needs the envelope, verify:

  1. The `system-implementer` (or any prod-agent MID without a flag
     `outputs:` block) successfully emits
     `scope-exception-files=…` via `gh optivem output write` —
     no `GH_OPTIVEM_OUTPUT_FILE is not set` error.
  2. The orchestrator stashes the per-dispatch JSONL path at
     `ctx.State["output-file-path"]` and
     `validate-outputs-and-scopes` reads the envelope keys back as
     `[]string` / `string` (not `[]any`).
  3. Note: the `scope_exception_requested` gate is registered
     (`internal/atdd/runtime/gates/bindings.go:128`) but **not yet
     wired into `execute-agent`** in `process-flow.yaml` — see the
     hand-off note below. Until that wiring lands, an out-of-scope
     write still routes to `FIX` even when the envelope is emitted.
     The envelope is *available* (this plan's contract); *routing
     on it* is a separate plan.

## Hand-off — separate plan needed

The `scope-exception-requested` binding has no `gateway` referencing
it in `process-flow.yaml`. Today the envelope keys land in
ctx.State correctly (with this plan), but `execute-agent` still
routes scope-diff failures into `FIX` regardless of whether the
envelope was emitted. A follow-up plan should add a gateway between
`VALIDATE_OUTPUTS_AND_SCOPES` and `GATE_OUTPUTS_AND_SCOPES_VALID`
(or between `GATE_OUTPUTS_AND_SCOPES_VALID` false-branch and
`GATE_FIX_ON_FAILURE`) that routes the envelope to a
`STOP_SCOPE_VIOLATION` end-event instead of `FIX`.

Out of scope for *this* plan: Item 6 was framed as "after this fix
lands, the cycle routes via `scope_exception_requested` instead of
FIX." That assumption doesn't hold because the gateway isn't wired;
the routing change is the follow-up.
