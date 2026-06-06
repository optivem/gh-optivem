# Plan: Remove the dead `task-name` params on the CT-HIGH verify calls

> **DECISION MADE (2026-06-06):** the `task-name` params on the three contract-test verify calls are
> unconsumed leftovers (shadowed before any reader sees them) and the only thing that makes these
> verify calls differ from every other caller — delete them. Settled in review discussion.
>
> Review finding §3c. Independent plan from the `process-flow.yaml` review; no dependency on the other
> review plans.

## Why

`implement-and-verify-external-system-driver-adapters-contract-tests` passes a `task-name` param on
three verify calls — `VERIFY_TESTS_PASS_CONTRACT_REAL` (`process-flow.yaml:994`),
`VERIFY_TESTS_FAIL_CONTRACT_STUB` (`:1010`), `VERIFY_TESTS_PASS_CONTRACT_STUB` (`:1036`) — while no
other caller of `verify-tests-pass` / `verify-tests-fail` does (`implement-test-layer`,
`implement-and-verify-system`, `refactor-and-verify-tests` all omit it).

It is dead. `${task-name}` is referenced only inside `execute-agent` (`:2271, :2289, :2339, :2392`)
and `fix` (`:2566`); the `verify-tests-*` and `run-tests` processes never reference it. The only
`execute-agent` reachable from the verify subtree is dispatched by the `fix-unexpected-*-tests` MID,
which **re-binds** `task-name` to its own value (`:1720`, `:1745`) before `execute-agent` runs — so
the inherited `task-name` is shadowed before any consumer (the approval prompts, `RUN_AGENT`,
`validate-outputs-and-scopes`, which reads `ctx.Params["task-name"]` at `actions/bindings.go:1049`)
reads it. `run-tests` also pins `fix-on-failure: false`, so `execute-command`'s command-failed FIX
(the only other `task-name` consumer) never fires here either.

Removing the params changes no dispatch, no scope lookup, and no approval prompt — it only makes the
trace's `in=` line consistent with the other verify callers. Strict `ExpandParams` is unaffected:
there is no `${task-name}` reference in the verify subtree to leave unresolved.

## Items

1. **`internal/atdd/runtime/statemachine/process-flow.yaml`** — delete the `task-name:` line from the
   `params:` of `VERIFY_TESTS_PASS_CONTRACT_REAL` (`:994`), `VERIFY_TESTS_FAIL_CONTRACT_STUB`
   (`:1010`), and `VERIFY_TESTS_PASS_CONTRACT_STUB` (`:1036`). Leave each call's `suite:` and
   `test-names:` params intact.
2. **Test check** — if any test asserts these three nodes carry `task-name` in their call params
   (e.g. `statemachine/transitions_test.go`, `trace` fixtures), update it. Scope `go test` per
   `[[feedback_go_test_windows.md]]`.

## Verification

- A CT-HIGH run still verifies contract-real (pass), contract-stub (fail-then-pass) exactly as before;
  the trace `in=` lines for these verify nodes no longer carry `task-name`.
