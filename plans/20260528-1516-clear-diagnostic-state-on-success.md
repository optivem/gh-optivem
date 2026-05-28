# Plan: Clear failure-diagnostic state keys when the producing action succeeds

## Context

`ctx.State` is a flat run-scoped key-value map carried through the entire BPMN execution. This is deliberate: `ExpandParams` (see `internal/atdd/runtime/statemachine/run.go:308` comment) falls back to `ctx.State` so downstream prompts can read `${failure-kind}`, `${command-stderr-tail}`, etc. without every intermediate call-activity declaring a passthrough param. That bridge is what makes the `fix-command-failed` / `fix-scope-diff` / `fix-missing-output` recovery prompts work.

The cost: a key written by a failed action stays in `ctx.State` forever unless explicitly overwritten. Two producers stamp diagnostic payloads on failure and **never clear them on success**:

| Producer | On failure sets | On success clears |
|---|---|---|
| `runCommand` (`actions/bindings.go:745`) | `failure-kind`, `command-line`, `command-exit-code`, `command-stderr-tail`, `verify_results_text` | nothing |
| `validateOutputsAndScopes` (`actions/bindings.go:975`) | `failure-kind`, `failing-task-name`, `missing-outputs`, `scope-violating-paths` | nothing |

This causes two distinct symptoms:

**Symptom 1 — confusing trace output.** Observed in `worktrees/rehearsal-20260528-145513.log:2055`:

```
OK IMPLEMENT_TICKET -> command-exit-code=1, command-succeeded=true, ...,
                       failure-kind=command-failed, ...,
                       command-stderr-tail=…BUILD FAILED in 15s…
```

The call-activity's state-delta line (computed by `trace/trace.go:428 stateDelta`) honestly reports every key that changed during the phase. Inside that phase, `runCommand` fired twice: first as the expected-failure check (test was supposed to be red — exit 1 stamped the diagnostic keys), then as the verify-after-implementation check (test passed — only `test-outcome=pass` and `command-succeeded=true` overwritten). The four stale diagnostic keys ride the delta out to the top-level line. Operator reading the trace sees `command-succeeded=true` next to `command-exit-code=1` and reasonably concludes the build failed.

**Symptom 2 — fix-* prompts may receive stale diagnostic payload.** If a future `fix-*` dispatch fires after a producer that didn't fail, `ExpandParams`'s state-fallback path will substitute whichever values the most recent *failed* producer left behind. The recovery prompt then reasons about a failure that's already been resolved.

Goal: when the producer succeeds, it clears the diagnostic keys it owns. State semantics unchanged for the failure path — `fix-*` prompts still see fresh values written by the failure that just occurred.

## Decision: clear at the producer, not at the trace

Rejected alternatives:

- **Trace-only "render fresh keys only" fix.** Would address Symptom 1 but not Symptom 2. The state-fallback path for `ExpandParams` would still hand stale `failure-kind` to a later `fix-*` dispatch.
- **Push/pop a state-scope frame per call-activity.** Largest refactor and pushes back against the deliberate global-state design documented at `run.go:308`. The whole point of state-fallback was to avoid per-call-site discipline.
- **Declarative "owned diagnostic keys" per action.** Promising but premature — only two producers have diagnostic payloads today. Revisit if a third grows them.

The surgical clear-on-success at each producer is the smallest change that fixes both symptoms without touching the state-fallback contract.

## Items

### Item 1 — Clear `runCommand`'s diagnostic keys on success

**File:** `internal/atdd/runtime/actions/bindings.go`

In `runCommand`, after the existing `if err != nil { ... ctx.Set("command-stderr-tail", ...) ... }` block (currently `bindings.go:782-788`), add an `else` branch that zeroes the five diagnostic keys `runCommand` owns:

- `failure-kind`
- `command-line`
- `command-exit-code`
- `command-stderr-tail`
- `verify_results_text`

`verify_results_text` is owned by the `isTestRun && !succeeded` branch (`bindings.go:802`) but is in the same class — operator-facing trace residue and stale fix-prompt input. Clearing it in the same place keeps the "diagnostic keys owned by runCommand" list complete.

Use `ctx.Set(key, nil)` (or the engine's idiomatic "delete this key" call — check `statemachine.Context` for a `Delete` / `Unset` method first; if none exists, `ctx.Set(key, "")` is acceptable, since `stateDelta` already treats empty post-values as the `-key` delete marker per `trace/trace.go:458`).

Add one short comment above the else branch explaining *why* — "diagnostic keys owned by this action; clear on success so a later success doesn't carry residue from an earlier failure into the trace or into a downstream fix-* prompt via ExpandParams's state-fallback."

### Item 2 — Clear `validateOutputsAndScopes`'s diagnostic keys on success

**File:** `internal/atdd/runtime/actions/bindings.go`

`validateOutputsAndScopes` has three failure exits, each setting `failure-kind` + payload keys:

- `missing-output` branch (`bindings.go:989-997`): sets `failure-kind`, `failing-task-name`, `missing-outputs`
- `scope-diff` branch (`bindings.go:1043-1052`): sets `failure-kind`, `failing-task-name`, `scope-violating-paths`
- (The early-success exit at `bindings.go:1006` for `scope: none` MIDs already returns `outputs-and-scopes-valid=true` without stamping anything — no clear needed there.)

At the *successful* exit (after the scope check passes — i.e. the final fall-through where `outputs-and-scopes-valid` should be set to `true`), clear:

- `failure-kind`
- `failing-task-name`
- `missing-outputs`
- `scope-violating-paths`

Same clearing mechanism as Item 1. Note that `phase-changed-files` is stamped unconditionally (`bindings.go:1036`) and is NOT a diagnostic key — leave it alone.

Verify there is a clean "success" sink in this function today; if the function currently has no explicit `ctx.Set("outputs-and-scopes-valid", true)` at the end of the happy path, add it alongside the clears so the writes are colocated.

### Item 3 — Tests for both producers

**File:** `internal/atdd/runtime/actions/bindings_test.go`

For each producer, add a "success clears prior failure diagnostics" test that:

1. Seeds `ctx.State` with each diagnostic key set to a non-empty sentinel (simulating a prior failed dispatch in the same run).
2. Invokes the action under a success scenario (for `runCommand`: a shell that exits 0; for `validateOutputsAndScopes`: a passing validate against a fixture MID).
3. Asserts each diagnostic key is now empty / absent in `ctx.State`.

One test per producer is enough — the existing failure-path tests already cover the "on failure, sets keys" half of the contract.

## Verification

- Re-run the rehearsal that produced `worktrees/rehearsal-20260528-145513.log`. The top-level `OK IMPLEMENT_TICKET ->` line should no longer carry `command-exit-code=1`, `failure-kind=command-failed`, `command-stderr-tail=…`, or `verify_results_text=…` when the verify-after-implementation step succeeded.
- The fix-on-failure path should still surface a fresh `failure-kind` when a `runCommand` actually fails — covered by the existing failure tests in `bindings_test.go`.

## Out of scope

- Generalising into a declarative "owned diagnostic keys per action" registry. Two producers don't justify the mechanism; revisit when a third grows a diagnostic payload.
- Push/pop state-scope frames per call-activity. Conflicts with the documented state-fallback design in `run.go:308`.
- Trace-level rendering changes in `trace/trace.go`. The fix at the producer makes the trace correct by construction; no second-layer filtering needed.
- Whether `VERIFY_TESTS_PASS`'s inner `EXECUTE_COMMAND` should enable `fix-on-failure` (currently `false` at `process-flow.yaml:1835`). Separate concern, separate plan if warranted.
