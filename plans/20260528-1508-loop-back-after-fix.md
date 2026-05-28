# Plan: Loop back to the originating step after every fix dispatch

## Context

Today every fix dispatch in the BPMN exits forward without re-verifying the originating step, so the fix agent's effect is never confirmed by the state machine — the operator has to spot a successful fix by re-reading the trace or re-running the ticket.

Four current edges, all forward-only:

| Process | Current edge | File / line |
|---|---|---|
| `execute-command` | `{from: FIX, to: EXECUTE_COMMAND_END}` | `internal/atdd/runtime/statemachine/process-flow.yaml:2325` |
| `execute-agent` | `{from: FIX, to: APPROVE_POST}` | `internal/atdd/runtime/statemachine/process-flow.yaml:2246` |
| `verify-tests-pass` | `{from: FIX_UNEXPECTED_FAILING_TESTS, to: VERIFY_PASS_END}` | `internal/atdd/runtime/statemachine/process-flow.yaml:1285` |
| `verify-tests-fail` | `{from: FIX_UNEXPECTED_PASSING_TESTS, to: VERIFY_FAIL_END}` | `internal/atdd/runtime/statemachine/process-flow.yaml:1329` |

The 2026-05-28 rehearsal (`worktrees/rehearsal-20260528-135952.log`) caught this on the `verify-tests-pass` path: tests failed unexpectedly, `unexpected-failing-tests-fixer` was dispatched and approved, the sub-process exited via `VERIFY_PASS_END` without re-running the AT, and the operator had no signal whether the fix worked.

Goal: every fix dispatch loops back to the upstream step it remediated, so the cycle is `RUN → fail → FIX → RUN → (pass → exit | fail → FIX again | infra → halt)`.

This plan supersedes `plans/20260528-1447-verify-tests-rerun-after-fix-loop.md`, which covered only the two legacy verify-tests-* MIDs. The consolidation adds the symmetric loop-back at the two shared primitives so all four fix dispatches share the same `FIX → upstream-node` shape.

### Why four loops, not one unified mechanism

Folding the verify-tests-* fix dispatch down into `execute-command`'s generic fix path was rejected after evaluation. The layers have genuinely different semantics that can't be flattened without either layer violation or a schema field with one caller:

- `execute-command` reasons about 2-way binary success (`command-succeeded`).
- `run-tests` reasons about 3-way `test-outcome` (pass / fail / infra).
- `verify-tests-pass` / `verify-tests-fail` add an inverse-expectation frame on top.

The `fix-on-failure: "false"` setting on `run-tests` (documented inline at `process-flow.yaml:2253-2260`) exists precisely to keep `execute-command`'s 2-way fix path from pre-empting the 3-way `test-outcome` routing — that's the BPMN bug observed when an expected acceptance-test failure mis-routed to FIX instead of VERIFY_FAIL_END. Introducing a polymorphic `failure-kind-on-fail` param at the `execute-command` call-site would create a schema field exercised by exactly one caller (`run-tests`) — violates `feedback_schema_fields_earn_slot.md`.

So all four fix dispatches stay where they are; each gets its own loopback edge. Shape identical, dispatch locally meaningful.

### Human cap on every iteration

Every fix dispatch is pinned at `category: human` ("never bypassable"):

- `fix` process `APPROVE_PRE` at `process-flow.yaml:2339`
- `fix` process inner `EXECUTE_AGENT` at `process-flow.yaml:2365`
- `fix-unexpected-failing-tests` `EXECUTE_AGENT` at `process-flow.yaml:1722`
- `fix-unexpected-passing-tests` `EXECUTE_AGENT` at `process-flow.yaml:1747`

Operator approval at every iteration is the natural loop cap — no numeric retry counter, no new schema field. Operator decline at `APPROVE_PRE` terminates the loop cleanly via the `approve` sub-process's plain end-event.

## Items

### Item 1 — Loop back in `execute-command`

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Replace the unconditional exit edge with a loop-back to `RUN_COMMAND`:

```yaml
# was (line 2325):
- {from: FIX, to: EXECUTE_COMMAND_END}
# new:
- {from: FIX, to: RUN_COMMAND}
```

The cycle becomes: `RUN_COMMAND → fail → FIX → RUN_COMMAND → (succeed → end | fail → FIX again)`. `APPROVE_PRE` sits before `RUN_COMMAND` on the entry path and is not re-traversed by the loopback — the operator approved the original command once; subsequent re-runs after a fix are gated by `fix`'s own `APPROVE_PRE` instead.

### Item 2 — Loop back in `execute-agent`

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Replace the post-FIX edge with a loop-back to `RUN_AGENT`:

```yaml
# was (line 2246):
- {from: FIX, to: APPROVE_POST}
# new:
- {from: FIX, to: RUN_AGENT}
```

The cycle becomes: `RUN_AGENT → VALIDATE → invalid → FIX → RUN_AGENT → VALIDATE → (valid → APPROVE_POST | invalid → FIX again)`. `APPROVE_PRE` sits before `SNAPSHOT_WORKING_TREE` on the entry path and is not re-traversed — same reasoning as Item 1.

Note: `RUN_AGENT` is the user-task that dispatches the writing agent. Looping back to `RUN_AGENT` (not `VALIDATE_OUTPUTS_AND_SCOPES`) means the writing agent runs again after the fix, which is the intended semantic — the fix may have rewritten an input the writing agent needs to reconsider.

### Item 3 — Loop back in `verify-tests-pass`

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

(Inherited from `plans/20260528-1447-verify-tests-rerun-after-fix-loop.md` Item 1.)

```yaml
# was (line 1285):
- {from: FIX_UNEXPECTED_FAILING_TESTS, to: VERIFY_PASS_END}
# new:
- {from: FIX_UNEXPECTED_FAILING_TESTS, to: RUN_TESTS}
```

Cycle: `RUN_TESTS → fail → FIX → RUN_TESTS → (pass → end | fail → FIX again | infra → halt)`.

### Item 4 — Loop back in `verify-tests-fail`

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

(Inherited from `plans/20260528-1447-verify-tests-rerun-after-fix-loop.md` Item 2.)

```yaml
# was (line 1329):
- {from: FIX_UNEXPECTED_PASSING_TESTS, to: VERIFY_FAIL_END}
# new:
- {from: FIX_UNEXPECTED_PASSING_TESTS, to: RUN_TESTS}
```

Cycle: `RUN_TESTS → pass → FIX → RUN_TESTS → (fail → end | pass → FIX again | infra → halt)`.

### Item 5 — Pre-flight the statemachine test fixtures for the loopback hazard

(Inherited from `plans/20260528-1447-verify-tests-rerun-after-fix-loop.md` Item 3, expanded for the two new loops.)

Loopback edges in `process-flow.yaml` have previously deadlocked the statemachine tests and consumed 20GB+ RAM (CLAUDE.md `feedback_statemachine_test_loop_hazard.md`). Before running the full test suite:

- Search `internal/atdd/runtime/statemachine/*_test.go` for fixtures that stub `test-outcome`, `command-succeeded`, or `outputs-and-scopes-valid` and trace through any of the four affected processes. Any fixture that returns the same value on every call will infinite-loop after the corresponding loopback lands.
- Update those fixtures to flip the outcome on the second visit, mirroring how `fix-on-failure-enabled` test stubs return `false` on the inner call (`internal/atdd/runtime/statemachine/run_test.go:620-627` — verify line is still accurate before mirroring).
- Run `go test ./internal/atdd/runtime/statemachine/ -p 2 -timeout 60s` first; abort and audit if the timeout fires (Windows test-memory guidance from CLAUDE.md `feedback_go_test_windows.md`).

### Item 6 — Update transitions tests

**File:** `internal/atdd/runtime/statemachine/transitions_test.go`

Existing tests assert the exact edge set of the four affected processes. Update the expected target of each post-FIX edge to the upstream loopback target:

- `execute-command`: post-FIX target `EXECUTE_COMMAND_END` → `RUN_COMMAND`
- `execute-agent`: post-FIX target `APPROVE_POST` → `RUN_AGENT`
- `verify-tests-pass`: post-FIX target `VERIFY_PASS_END` → `RUN_TESTS`
- `verify-tests-fail`: post-FIX target `VERIFY_FAIL_END` → `RUN_TESTS`

### Item 7 — Update end-to-end fixtures

**File:** `internal/atdd/runtime/statemachine/run_test.go`

Any walk-through test that exercises a FIX path needs a sequence-aware stub (see Item 5) so the loop terminates. The four patterns:

- `verify-tests-pass`: stub returns `fail` first, `pass` second → loops back, exits via `VERIFY_PASS_END`.
- `verify-tests-fail`: stub returns `pass` first, `fail` second → loops back, exits via `VERIFY_FAIL_END`.
- `execute-command`: stub returns `command-succeeded=false` first, `true` second → loops back, exits via `EXECUTE_COMMAND_END`.
- `execute-agent`: stub returns `outputs-and-scopes-valid=false` first, `true` second → loops back, exits via `APPROVE_POST` → `EXECUTE_AGENT_END`.

### Item 8 — Delete the superseded plan

```bash
git rm plans/20260528-1447-verify-tests-rerun-after-fix-loop.md
```

Per `feedback_drop_dont_relocate.md`: this plan fully covers the original — Items 3 + 4 + 5 + 6 + 7 (the verify-tests parts) inherit the original's content. Keeping both would create drift.

## Out of scope

- **Numeric retry cap.** Human approval per iteration is the cap (`category: human` on every fix dispatch — see Context). A `fix-attempt-count` binding + max-attempts gateway would be a schema field that no production code branches on (`feedback_schema_fields_earn_slot.md`).
- **`fix-on-failure-enabled` semantics.** The flag still governs whether `execute-command` / `execute-agent` enter the fix path on the *first* failure; this plan only changes what happens after the fix returns. The two mechanisms remain independent.
- **Folding verify-tests fix dispatch into `execute-command`.** Rejected — see Context for the layer-violation / schema-field-with-one-caller reasoning.
- **Trace banner honesty.** Separate concern (`[atdd-rehearsal] implement succeeded` is being addressed in a different plan); not folded in here to keep the BPMN edit surgical.
- **Diagram regeneration.** The `regenerate-diagram` GH Actions workflow auto-regenerates `docs/process-diagram.md` + `docs/images/*.svg` on push to main (`feedback_plans_no_diagram_regen.md`).

## Verification

- `go test ./internal/atdd/runtime/statemachine/... -p 2` passes.
- `go test ./internal/atdd/... -p 2` passes.
- Rehearsal: `bash scripts/atdd-rehearsal.sh <issue> --config gh-optivem-monolith-java.yaml` on a ticket designed to trigger each of the four fix paths in turn. Expect to see the originating step re-run after every fix approval, and each cycle terminate only when the outcome flips (or operator declines a later fix dispatch via `fix`'s `APPROVE_PRE`).
