# Bound the verify→fix loop (circuit breaker)

> **Status: skeleton.** Spun off from
> [[20260530-1313-atdd-fixer-agents-audit]] (finding Q2, its
> highest-severity finding). That audit is prompt-side only; this work is
> harness-side and was deliberately kept out of it
> ([[feedback_new_plan_not_extend]]). Refine before executing.

## Problem

The `verify-tests-pass` and `verify-tests-fail` subprocesses route a
failed verification into a fixer and then loop straight back to
re-run the tests, with **no max-iteration guard**:

- `process-flow.yaml:1256` — `{from: FIX_UNEXPECTED_FAILING_TESTS, to: RUN_TESTS}`
- `process-flow.yaml:1300` — `{from: FIX_UNEXPECTED_PASSING_TESTS, to: RUN_TESTS}`

Both fixers run at **opus · high** — the most expensive tuning in the
tree. A fixer that repeatedly mis-picks which side to fix (SUT vs. test)
loops `RUN_TESTS → GATE_TESTS_OUTCOME → FIX_* → RUN_TESTS` indefinitely,
each lap burning a full opus·high pass. Per
[[feedback_statemachine_test_loop_hazard]] an unbounded fix loop is
exactly the shape that has driven statemachine tests to 20GB+ RAM
before — so this is both a runtime cost risk and a test-suite hazard.

## Goal

After N consecutive failed fix attempts on the same verification,
stop looping and escalate to a human-visible halt instead of spinning.

## Open design questions (resolve during refine)

- **Where the counter lives.** Per-subprocess iteration count in
  `ctx.State`, vs. a generic loop-guard the engine applies to any
  `FIX_* → RUN_*` back-edge. Prefer the narrowest mechanism that covers
  both loops without a new bespoke field per loop.
- **N.** Likely 2–3. A must-fail test that's still green after 2 fix
  attempts is almost certainly a genuine SUT/spec disagreement a human
  must adjudicate, not something a third opus pass will resolve.
- **Halt target.** A new `error-end-event` (e.g.
  `FIX_LOOP_EXHAUSTED`) per subprocess, vs. reuse of an existing
  escalation/halt node. Must surface enough context (which fixer, how
  many attempts, last verify tail) for a human to pick up.
- **Test-suite safety.** Whatever guard lands must be exercised by a
  statemachine fixture that *would* loop unbounded today — audit gate
  fixtures and kill on memory climb before running
  ([[feedback_statemachine_test_loop_hazard]]).

## Items

- [ ] **Add a bounded-iteration guard to both verify subprocesses.**
  Cap the `FIX_* → RUN_TESTS` back-edge at N attempts in
  `verify-tests-pass` and `verify-tests-fail`; on exhaustion route to a
  human-visible halt rather than `RUN_TESTS`. (`process-flow.yaml` +
  engine.)
- [ ] **Surface escalation context at the halt.** Ensure the halt event
  carries fixer identity, attempt count, and the last verify tail.
- [ ] **Statemachine fixture that proves the cap.** A fixture that loops
  unbounded under today's flow and terminates under the guard; audit it
  for the loop hazard before running.

## Cross-references

- Source finding: [[20260530-1313-atdd-fixer-agents-audit]] Q2 / Item 6.
- Related hazard: [[feedback_statemachine_test_loop_hazard]].
- Plan-authoring conventions: [[feedback_new_plan_not_extend]],
  [[feedback_plans_no_diagram_regen]].
