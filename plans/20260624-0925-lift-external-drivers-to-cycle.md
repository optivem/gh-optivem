# 2026-06-24 09:25:00 UTC — Lift External Driver Setup to Cycle Level

## TL;DR

**Why:** The ordering invariant "external system drivers and contract tests run before acceptance test writing" is invisible at the cycle level — buried 4 call-activity levels deep. Worse, the fallback gate `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` in `shared-contract` reads `at-external-driver-port-changed`, which is only stamped by the DSL writing agent (after AT writing) — so on a fresh first run with no ESCC declared, the gate always evaluates false and external drivers are silently skipped. The flag is only non-false on re-runs where State persists from a prior DSL phase.

**End result:** ESCC (External System Contract Criteria) is the sole and authoritative signal for whether a ticket needs external driver work. The broken port-changed fallback gate is removed. Both `change-system-behavior` and `cover-system-behavior` explicitly show `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` (guarded only by `GATE_TICKET_HAS_ESCC`) before `WRITE_AND_VERIFY_ACCEPTANCE_TESTS`. The two thin wrapper processes are deleted. `shared-contract` starts at AT writing with no external-driver entry logic.

## Outcomes

- `change-system-behavior` and `cover-system-behavior` are self-documenting: external drivers visibly precede acceptance test writing at the top level of each cycle.
- Two unnecessary thin wrapper processes (`write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`) are deleted — one less indirection level.
- A new named sub-process `implement-external-drivers-if-needed` has a single entry gate (`GATE_TICKET_HAS_ESCC`). No port-changed fallback. Shared by both cycles with no duplication.
- The broken `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` fallback is removed everywhere. ESCC is the sole entry signal for external driver work.
- `shared-contract` is simplified: loses both external-driver entry gates (`GATE_TICKET_HAS_ESCC`, `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`), `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`, and `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`. Starts directly at `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`.
- A new post-DSL integrity gate in `shared-contract` detects the case where the DSL writing agent touched external driver port files but no ESCC was declared — and halts with a hard error ("add ESCC and re-run"). This enforces: no ESCC → DSL must not touch external driver ports.
- Go code referencing deleted/moved process names and the now-repurposed `at-external-driver-port-changed` flag is cleaned up.

## ▶ Next executable step (resume here)

Step 1: grep Go source for all string references to `write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`, and find `UnrollExternalSystems` in `channels.go` to establish the full impact surface before touching the YAML.

## Steps

- [ ] Step 1: Audit Go code — grep `internal/` for string references to `write-and-verify-acceptance-tests-fail`, `write-and-verify-acceptance-tests-pass`. Find `UnrollExternalSystems` in `channels.go` to see if it hard-codes a process name when searching for the unroll anchor. Record what needs updating.
- [ ] Step 2: Add new sub-process `implement-external-drivers-if-needed` to `process-flow.yaml`. Single gate: `GATE_TICKET_HAS_ESCC` → true: `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED` → `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` (unroll anchor) → end; false: end (skip). No `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`.
- [ ] Step 3: Revise `change-system-behavior` — replace `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` with two nodes: `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` (call-activity → `implement-external-drivers-if-needed`) then `WRITE_AND_VERIFY_ACCEPTANCE_TESTS` (call-activity → `write-and-verify-acceptance-tests`, params: `expected-test-result: failure`). Update sequence-flows.
- [ ] Step 4: Revise `cover-system-behavior` symmetrically — replace `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS` with `IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED` then `WRITE_AND_VERIFY_ACCEPTANCE_TESTS` (params: `expected-test-result: success`).
- [ ] Step 5: Simplify `shared-contract` — remove `GATE_TICKET_HAS_ESCC`, `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`, `VALIDATE_EXTERNAL_SYSTEMS_REGISTERED`, `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`, and their sequence-flows. Change `start:` to `WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`.
- [ ] Step 6: Add post-DSL integrity gate to `shared-contract` — after `IMPLEMENT_AND_VERIFY_DSL`, add `GATE_EXTERNAL_PORT_CHANGED_WITHOUT_ESCC` (gateway binding: `at-external-driver-port-changed-without-escc`, which is true when `at-external-driver-port-changed == true && ticket-has-escc == false`). True branch routes to a new `EXTERNAL_PORT_CHANGED_WITHOUT_ESCC_HALT` error-end-event ("DSL touched external driver port files but ticket declares no ESCC — add an ## External System Contract Criteria section to the ticket and re-run from the start"). False branch routes to `SHARED_CONTRACT_END`. Requires a new gate binding in `bindings.go`.
- [ ] Step 7: Delete `write-and-verify-acceptance-tests-fail` and `write-and-verify-acceptance-tests-pass` from `process-flow.yaml`.
- [ ] Step 8: Update Go code — apply changes found in Step 1 (process name strings, `UnrollExternalSystems` anchor lookup). Add the new action binding for Step 6. Remove or repurpose the `at-external-driver-port-changed` gate binding in `bindings.go` if it is no longer used as a routing gate.
- [ ] Step 9: Run `go build ./...` and the process-flow tests to confirm nothing is broken.

## Open questions

- Does `UnrollExternalSystems` in `channels.go` hard-code `shared-contract` or `write-and-verify-acceptance-tests-fail` as the process name when searching for the unroll anchor, or does it search by node ID across all processes? (Step 1 will answer this.)
- Are there any ATDD runtime prompt files or doc references that name `write-and-verify-acceptance-tests-fail` or `write-and-verify-acceptance-tests-pass` that also need updating?
- Are there any ATDD runtime prompt files or doc references that name `write-and-verify-acceptance-tests-fail` or `write-and-verify-acceptance-tests-pass` that also need updating? (Step 1 will catch Go, but prompts/docs need a separate grep.)
