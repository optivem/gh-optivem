# 2026-06-24 09:25:00 UTC — Lift External Driver Setup to Cycle Level

## TL;DR

**Why:** The ordering invariant "external system drivers and contract tests run before acceptance test writing" is invisible at the cycle level — it is buried 4 call-activity levels deep inside `shared-contract`, which is itself nested inside two thin wrapper processes. Developers reading `change-system-behavior` or `cover-system-behavior` cannot see that external drivers happen first without tracing deep into sub-processes.

**End result:** The ordering invariant is visible at the cycle level. Both `change-system-behavior` and `cover-system-behavior` have an explicit `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` node followed by `WRITE_AND_VERIFY_ACCEPTANCE_TESTS`. The two thin wrapper processes are deleted. `shared-contract` starts at AT writing with no external-driver entry logic.

## Outcomes

- `change-system-behavior` and `cover-system-behavior` are self-documenting: external drivers visibly precede acceptance test writing at the top level of each cycle.
- Two unnecessary thin wrapper processes (`write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`) are deleted — one less indirection level.
- A new named sub-process `implement-external-drivers-if-needed` encapsulates the ESCC gate pattern, shared by both cycles with no duplication.
- `shared-contract` is simplified: it loses the external-driver entry gates and starts directly at `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`.
- Go code referencing deleted/moved process names is updated so no string references rot.

## ▶ Next executable step (resume here)

Step 1: grep Go source for all string references to `write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`, and the `UnrollExternalSystems` anchor in `channels.go` — establish the full impact surface before touching the YAML.

## Steps

- [ ] Step 1: Audit Go code — grep `internal/` for string references to `write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`, and find `UnrollExternalSystems` in `channels.go` to see if it hard-codes a process name for the unroll anchor. Record what needs updating.
- [ ] Step 2: Add new sub-process `implement-external-drivers-if-needed` to `process-flow.yaml`. Nodes: `GATE_TICKET_HAS_ESCC`, `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`, `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`, `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` (unroll anchor). Sequence-flows: same gate pattern currently at the top of `shared-contract`.
- [ ] Step 3: Revise `change-system-behavior` — replace `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` with two nodes: `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` (call-activity → `implement-external-drivers-if-needed`) then `WRITE_AND_VERIFY_ACCEPTANCE_TESTS` (call-activity → `write-and-verify-acceptance-tests`, params: `expected-test-result: failure`). Update sequence-flows accordingly.
- [ ] Step 4: Revise `cover-system-behavior` symmetrically — replace `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS` with `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` then `WRITE_AND_VERIFY_ACCEPTANCE_TESTS` (params: `expected-test-result: success`).
- [ ] Step 5: Simplify `shared-contract` — remove `GATE_TICKET_HAS_ESCC`, `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`, `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`, `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`, and their sequence-flows. Change `start:` to `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`.
- [ ] Step 6: Delete `write-and-verify-acceptance-tests-fail` and `write-and-verify-acceptance-tests-pass` from `process-flow.yaml`.
- [ ] Step 7: Update Go code — apply any changes found in Step 1 (process name strings, `UnrollExternalSystems` anchor lookup).
- [ ] Step 8: Run `go build ./...` and the process-flow tests to confirm nothing is broken.

## Open questions

- Does `UnrollExternalSystems` in `channels.go` hard-code `shared-contract` or `write-and-verify-acceptance-tests-fail` as the process name when searching for the unroll anchor, or does it search by node ID across all processes? (Step 1 will answer this.)
- Are there any ATDD runtime prompt files or doc references that name `write-and-verify-acceptance-tests-fail` or `write-and-verify-acceptance-tests-pass` that also need updating?
