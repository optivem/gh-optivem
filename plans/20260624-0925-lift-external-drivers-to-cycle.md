# 2026-06-24 09:25:00 UTC — Lift External Driver Setup to Cycle Level

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-06-24T10:09:39Z`

## TL;DR

**Why:** The ordering invariant "external system drivers and contract tests run before acceptance test writing" is invisible at the cycle level — buried 4 call-activity levels deep. Worse, the fallback gate `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` in `shared-contract` reads `at-external-driver-port-changed`, which is only stamped by the DSL writing agent (after AT writing) — so on a fresh first run with no ESCC declared, the gate always evaluates false and external drivers are silently skipped. The flag is only non-false on re-runs where State persists from a prior DSL phase.

**End result:** ESCC (External System Contract Criteria) is the sole and authoritative signal for whether a ticket needs external driver work. The broken port-changed fallback gate is removed. Both `change-system-behavior` and `cover-system-behavior` explicitly show `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` (guarded only by `GATE_TICKET_HAS_ESCC`) before `WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS`. The two thin wrapper processes are deleted. Processes are renamed to communicate the channel boundary: `write-acceptance-tests-and-dsl` (runs once, shared) and `write-acceptance-tests-and-system-adapters` (adds per-channel system driver adapter work).

## Outcomes

- `change-system-behavior` and `cover-system-behavior` are self-documenting: external drivers visibly precede acceptance test writing at the top level of each cycle.
- Two unnecessary thin wrapper processes (`write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`) are deleted — one less indirection level.
- A new named sub-process `implement-external-drivers-if-needed` has a single entry gate (`GATE_TICKET_HAS_ESCC`). No port-changed fallback. Shared by both cycles with no duplication.
- The broken `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` fallback is removed everywhere. ESCC is the sole entry signal for external driver work.
- `shared-contract` is renamed `write-acceptance-tests-and-dsl` — a name that describes what it does rather than implying a contract-test concern that has moved elsewhere.
- `write-acceptance-tests-and-dsl` is simplified: loses both external-driver entry gates, `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`, and `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`. Starts directly at `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`.
- A new post-DSL integrity gate in `write-acceptance-tests-and-dsl` halts with an error if the DSL agent touched external driver port files — always wrong at this stage regardless of ESCC. Forces the developer to declare ESCC and re-run from the start.
- `write-and-verify-acceptance-tests` is renamed `write-acceptance-tests-and-system-adapters` — communicates that system adapters (naturally per-channel) are what this layer adds on top of the shared DSL work.
- Go code referencing deleted/moved/renamed process names is cleaned up; `at-external-driver-port-changed` gate binding is reused as the post-DSL integrity gate.

## ▶ Next executable step (resume here)

Step 1: grep Go source for all string references to `write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`, and find `UnrollExternalSystems` in `channels.go` to establish the full impact surface before touching the YAML.

## Steps

- [ ] Step 1: Audit Go code — grep `internal/` for string references to `write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`. Find `UnrollExternalSystems` in `channels.go` to see if it hard-codes a process name when searching for the unroll anchor. Record what needs updating.
- [ ] Step 2: Add new sub-process `implement-external-drivers-if-needed` to `process-flow.yaml`. Single gate: `GATE_TICKET_HAS_ESCC` → true: `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED` → `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` (unroll anchor) → end; false: end (skip). No `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`.
- [ ] Step 3: Rename `write-and-verify-acceptance-tests` to `write-acceptance-tests-and-system-adapters` throughout `process-flow.yaml` and any Go code that references it by string. Also initialize `at-system-driver-port-changed` to `false` at process start so the outer `GATE_DSL_PORT_CHANGED` guard (currently needed because the key is unset when DSL doesn't run) can be removed — leaving just `GATE_SYSTEM_DRIVER_PORTS_CHANGED` after the inner call.
- [ ] Step 4: Revise `change-system-behavior` — replace `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` with two nodes: `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` (call-activity → `implement-external-drivers-if-needed`) then `WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS` (call-activity → `write-acceptance-tests-and-system-adapters`, params: `expected-test-result: failure`). Update sequence-flows.
- [ ] Step 5: Revise `cover-system-behavior` symmetrically — replace `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS` with `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` then `WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS` (params: `expected-test-result: success`).
- [ ] Step 6: Rename `shared-contract` to `write-acceptance-tests-and-dsl` throughout `process-flow.yaml` and any Go code that references it by string.
- [ ] Step 7: Simplify `write-acceptance-tests-and-dsl` — remove `GATE_TICKET_HAS_ESCC`, `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`, `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`, `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`, and their sequence-flows. Change `start:` to `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`.
- [ ] Step 8: Add post-DSL integrity gate to `write-acceptance-tests-and-dsl` — after `IMPLEMENT_AND_VERIFY_DSL`, add `GATE_EXTERNAL_DRIVER_PORT_CHANGED` (gateway binding: `at-external-driver-port-changed`). True branch routes to a new `EXTERNAL_DRIVER_PORT_CHANGED_HALT` error-end-event ("DSL touched external driver port files — this ticket needs an ## External System Contract Criteria section; add it and re-run from the start"). False branch routes to `WRITE_ACCEPTANCE_TESTS_AND_DSL_END`. No `ticket-has-escc` check — any external driver port change at this stage is always an error. Requires a new gate binding in `bindings.go`.
- [ ] Step 9: Delete `write-and-verify-acceptance-tests-fail` and `write-and-verify-acceptance-tests-pass` from `process-flow.yaml`.
- [ ] Step 10: Update Go code — apply changes found in Step 1 (process name strings, `UnrollExternalSystems` anchor lookup). Add the new gate binding for Step 8. The `at-external-driver-port-changed` binding in `bindings.go` is reused as-is (same key, now used as the post-DSL integrity gate rather than the entry gate).
- [ ] Step 11: Run `go build ./...` and the process-flow tests to confirm nothing is broken.

## Open questions

- Does `UnrollExternalSystems` in `channels.go` hard-code `shared-contract` or `write-and-verify-acceptance-tests-fail` as the process name when searching for the unroll anchor, or does it search by node ID across all processes? (Step 1 will answer this.)
- Are there any ATDD runtime prompt files or doc references that name `write-and-verify-acceptance-tests-fail` or `write-and-verify-acceptance-tests-pass` that also need updating?
- Are there any ATDD runtime prompt files or doc references that name `write-and-verify-acceptance-tests-fail` or `write-and-verify-acceptance-tests-pass` that also need updating? (Step 1 will catch Go, but prompts/docs need a separate grep.)
