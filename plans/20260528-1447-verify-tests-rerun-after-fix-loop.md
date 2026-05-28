# Plan: Re-run tests after the fix agent (verify-tests-pass / verify-tests-fail loopback)

## Context

Today the `verify-tests-pass` and `verify-tests-fail` sub-processes dispatch their specialised fix agents and then exit without re-running the tests, so the fix agent's effect on the test outcome is never verified by the BPMN.

Current paths (`internal/atdd/runtime/statemachine/process-flow.yaml`):

```yaml
# verify-tests-pass  (lines 1280-1285)
- {from: RUN_TESTS,                     to: GATE_TESTS_OUTCOME}
- {from: GATE_TESTS_OUTCOME,            to: VERIFY_PASS_END,              when: "test-outcome == pass"}
- {from: GATE_TESTS_OUTCOME,            to: FIX_UNEXPECTED_FAILING_TESTS, when: "test-outcome == fail"}
- {from: GATE_TESTS_OUTCOME,            to: TESTS_INFRA_HALT,             when: "test-outcome == infra"}
- {from: GATE_TESTS_OUTCOME,            to: UNKNOWN_TESTS_OUTCOME}
- {from: FIX_UNEXPECTED_FAILING_TESTS,  to: VERIFY_PASS_END}              # ← exits unconditionally

# verify-tests-fail  (lines 1323-1329)
- {from: RUN_TESTS,                     to: GATE_TESTS_OUTCOME}
- {from: GATE_TESTS_OUTCOME,            to: FIX_UNEXPECTED_PASSING_TESTS, when: "test-outcome == pass"}
- {from: GATE_TESTS_OUTCOME,            to: VERIFY_FAIL_END,              when: "test-outcome == fail"}
- {from: GATE_TESTS_OUTCOME,            to: TESTS_INFRA_HALT,             when: "test-outcome == infra"}
- {from: GATE_TESTS_OUTCOME,            to: UNKNOWN_TESTS_OUTCOME}
- {from: FIX_UNEXPECTED_PASSING_TESTS,  to: VERIFY_FAIL_END}              # ← exits unconditionally
```

Observed in the 2026-05-28 rehearsal (`worktrees/rehearsal-20260528-135952.log`):

- `verify-tests-pass` was reached on the green path with `test-outcome=fail` (Jackson `BeanDeserializer` crash on `BrowseOrdersResponse` masked the true SUT state).
- `FIX_UNEXPECTED_FAILING_TESTS` dispatched `unexpected-failing-tests-fixer` (operator approved at `APPROVE_PRE`, approved its output at `APPROVE_POST`).
- The sub-process then exited via `VERIFY_PASS_END` **without re-running the AT**, leaving the operator unable to tell whether the fix worked.

Goal: loop back to `RUN_TESTS` after each fix dispatch so the BPMN verifies the fix's effect. The two human approval gates inside `execute-agent` (`APPROVE_PRE`, `APPROVE_POST`) already rate-limit each iteration, so the human is the natural loop cap — no numeric counter, no new schema field.

## Items

### Item 1 — Loop back in `verify-tests-pass`

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Replace the unconditional exit edge from `FIX_UNEXPECTED_FAILING_TESTS` with a loopback to `RUN_TESTS`:

```yaml
- {from: FIX_UNEXPECTED_FAILING_TESTS, to: RUN_TESTS}   # was: to: VERIFY_PASS_END
```

After the change, the cycle is: `RUN_TESTS → fail → FIX → RUN_TESTS → (pass → end | fail → FIX again | infra → halt)`. The operator's `APPROVE_PRE` "no" inside `execute-agent` terminates cleanly (the approve sub-process uses plain `end-event`, see comment at `process-flow.yaml:2068-2072`).

### Item 2 — Loop back in `verify-tests-fail` (symmetric)

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

```yaml
- {from: FIX_UNEXPECTED_PASSING_TESTS, to: RUN_TESTS}   # was: to: VERIFY_FAIL_END
```

Same shape, opposite expectation: red-phase test was supposed to fail but passed; the fix agent adjusts; we re-run to confirm the test now fails as designed.

### Item 3 — Pre-flight the statemachine test fixtures for the loopback hazard

Loopback edges in `process-flow.yaml` have previously deadlocked the statemachine tests and consumed 20GB+ RAM (see CLAUDE.md feedback memory `feedback_statemachine_test_loop_hazard.md`). Before running the full test suite:

- Search `internal/atdd/runtime/statemachine/*_test.go` for fixtures that stub `test-outcome` or `command-succeeded` and trace through `verify-tests-pass` / `verify-tests-fail`. Any fixture that returns the same `test-outcome` value on every call will infinite-loop after the loopback lands.
- Update those fixtures to return `pass` (or `fail`) after the first FIX dispatch — e.g. a sequence-aware stub that flips outcome on the second visit, mirroring how `fix-on-failure-enabled` test stubs return `false` on the inner call (`internal/atdd/runtime/statemachine/run_test.go:620-627`).
- Run `go test ./internal/atdd/runtime/statemachine/ -p 2 -timeout 60s` first; abort and audit if the timeout fires (Windows test-memory guidance from CLAUDE.md `feedback_go_test_windows.md`).

### Item 4 — Update transitions tests

**File:** `internal/atdd/runtime/statemachine/transitions_test.go`

Existing tests assert the exact edge set of `verify-tests-pass` and `verify-tests-fail`. Update the expected target of the post-FIX edge from `VERIFY_PASS_END` / `VERIFY_FAIL_END` to `RUN_TESTS` in both processes.

### Item 5 — Update end-to-end fixtures

**File:** `internal/atdd/runtime/statemachine/run_test.go`

Any walk-through test that exercises the FIX path needs a sequence-aware `test-outcome` stub (see Item 3) so the loop terminates. The two patterns:

- `verify-tests-pass` walk: stub returns `fail` on the first call (triggers FIX), then `pass` on the second (loops back, exits via `VERIFY_PASS_END`).
- `verify-tests-fail` walk: stub returns `pass` on the first call (triggers FIX), then `fail` on the second (loops back, exits via `VERIFY_FAIL_END`).

## Out of scope

- **Numeric retry cap.** Operator approval per iteration is the cap. Adding a `fix-attempt-count` binding + max-attempts gateway would be a schema field that no production code branches on (anti-pattern per CLAUDE.md `feedback_schema_fields_earn_slot.md`).
- **`fix-on-failure-enabled` semantics.** The existing flag governs auto-fix at the `execute-agent` / `execute-command` layer; this plan only changes what happens *after* the specialised verify-tests fix process returns. The two mechanisms remain independent.
- **Trace banner honesty.** Separate concern — covered by the live discussion around `[atdd-rehearsal] implement succeeded`. Not folded in here to keep the BPMN edit surgical.
- **Diagram regeneration.** The `regenerate-diagram` GH Actions workflow auto-regenerates `docs/process-diagram.md` + `docs/images/*.svg` on push to main (per CLAUDE.md `feedback_plans_no_diagram_regen.md`).

## Verification

- `go test ./internal/atdd/runtime/statemachine/... -p 2` passes.
- `go test ./internal/atdd/... -p 2` passes.
- Rehearsal: `bash scripts/atdd-rehearsal.sh <issue> --config gh-optivem-monolith-java.yaml` on a ticket designed to trigger `fix-unexpected-failing-tests`. Expect to see the AT re-run after fix approval, and the cycle terminate only when `test-outcome=pass` (or operator declines a later fix dispatch).
