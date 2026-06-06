# CT-HIGH DSL layer: compile-only, defer contract-suite outcome to the PROBE

## TL;DR

**Why:** In the contract sub-flow, the DSL/port layer (`IMPLEMENT_AND_VERIFY_DSL`) asserts `contract-real` should be **green** right after the port impl — but the External-System Driver adapters that actually green it run in the *next* node. So the DSL-layer verify fails, dispatches the generic `fix-unexpected-failing-tests`, and that fixer front-runs the dedicated `external-system-driver-adapter-implementer`, plumbing the adapter + `Ext*` client DTOs ad hoc (exactly what the #72 rehearsal hit — the fixer hand-plumbed five driver/client files).
**End result:** The CT-HIGH DSL/port layer **compiles + commits only**; it no longer runs or asserts the contract suite. `PROBE_CONTRACT_REAL` (outcome-driven, post-adapters, from `20260606-1943`) becomes the **single** authority for contract-real red/green. The dedicated adapter-implementer owns the adapter/client plumbing; the generic fixer never fires on a pre-adapter contract red.

**Created:** 2026-06-06 23:30 (CEST)
**Builds on:** [[plans/20260606-1943-contract-real-outcome-driven-verify.md]] — that plan made the *external-boundary* check (`PROBE_CONTRACT_REAL/STUB`) outcome-driven. This plan removes the *remaining* polarity assertion one layer upstream (the DSL step) so the PROBE is the sole outcome judge. It does **not** edit the 1943 plan.
**Related:** [[project_bpmn_full_coverage_story_and_realkind_gap]] (#72 is the full-BPMN rehearsal story), [[feedback_port_changed_flags_directory_keyed]].

## Context — why

In `implement-and-verify-external-system-driver-adapters-contract-tests`
(`internal/atdd/runtime/statemachine/process-flow.yaml`, ~1039–1263) the node order is:

```
WRITE_CONTRACT_TESTS
  → [GATE_DSL_PORT_CHANGED true] → IMPLEMENT_AND_VERIFY_DSL   (~1055; expected-test-result: success, suite: contract-real)
  → IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS                 (~1065; the dedicated adapter-implementer)
  → IDENTIFY → BUILD → START → PROBE_CONTRACT_REAL           (outcome-driven; the 1943 authority)
```

`IMPLEMENT_AND_VERIFY_DSL` → `implement-and-verify-dsl` → `implement-test-layer`
(~1390) → `GATE_EXPECTED_TEST_RESULT` (binding `at-verify-expectation`). The CT-HIGH DSL
step passes **no `verify-mode`** (the `green-when-complete`/`verify-pending-on` cover-gate
machinery lives only on the AT side, ~663–781), so `atVerifyExpectation`
(`internal/atdd/runtime/gates/bindings.go:414`) takes the **change path** and returns the
pinned `expected-test-result` verbatim = `success` → `VERIFY_TESTS_PASS_FILTERED` on
`suite: contract-real`.

But after only the DSL/port impl the External-System Driver adapters are still
throwing-`TODO` stubs (they are implemented at the *next* node,
`IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`, which both branches of `GATE_DSL_PORT_CHANGED`
always flow through). So `contract-real` is **red**, `verify-tests-pass` dispatches
`fix-unexpected-failing-tests`, and the generic fixer does the adapter/client plumbing that
`external-system-driver-adapter-implementer` was about to do one node later. The five files
the #72 fixer touched — `ExtProductDetailsResponse`, `ExtCreateProductRequest`,
`BaseErpDriver`, `ErpRealDriver`, `ErpStubDriver` — all live under
`${external-system-driver-adapter}/**` (the `client/` sublayer is a subdirectory of the
adapter dir), i.e. one clean boundary owned by the adapter-implementer, not the dsl-implementer.

### Why compile-only, not "flip to `expected-test-result: failure`"

Pinning `failure` instead of `success` would stop the fixer, but it is **still polarity
prediction** — the very thing `20260606-1943` is retiring for CT. Worse, `verify-tests-fail`
(expect RED) is the exact construct 1943 *deleted* from the external boundary (the echo
simulator returns GREEN for additive fields), and a CT-HIGH change that happens to leave
`contract-real` green at the DSL step would wrongly halt it. The outcome-aligned move is to
**not assert the contract suite's polarity at the DSL layer at all** — the suite's red/green
is `PROBE_CONTRACT_REAL`'s job, post-adapters. The DSL layer's real work is "change the port,
ensure it compiles, commit"; `COMPILE_TESTS` already does the meaningful check.

Making the DSL step *itself* probe-and-branch is rejected: it just duplicates
`PROBE_CONTRACT_REAL` one node later (extra cost, no benefit).

### Decision

Add a third `verify-mode` to the shared `implement-test-layer` — **`none`** — that routes
`GATE_EXPECTED_TEST_RESULT` straight to `COMMIT_LAYER`, skipping both `VERIFY_TESTS_*`
nodes. Wire the CT-HIGH DSL step to use it. `START_SYSTEM` stays (idempotent; it warms the
system for the downstream adapter-impl + PROBE). The new mode is opt-in, so every existing
caller (AT DSL layers, system-driver-adapter layer, acceptance-test-code writer) is unchanged.

**Non-goal:** the AT-side DSL/driver layers stay on the change-cascade red-intermediate model
(`bindings.go:407`); this plan touches CT-HIGH only.

## Items

### Item 1 — `bindings.go`: `verify-mode: none` → `at-verify-expectation == "none"`
- `internal/atdd/runtime/gates/bindings.go`, `atVerifyExpectation` (~414): add, ahead of the
  `green-when-complete` branch, `if verify-mode == "none" { return Outcome{Value: "none"} }`.
- Extend the doc-comment block (~397–413) to document the third mode: *none → caller skips the
  suite-polarity assertion entirely (compile-only layer); the gate routes straight to commit.*
- `internal/atdd/runtime/gates/bindings_test.go`: add a case asserting `verify-mode: none`
  yields `"none"` (and that it does not consult `plumbingPending` / `expected-test-result`).

### Item 2 — `process-flow.yaml` `implement-test-layer`: add the skip edge
- Add sequence-flow `{from: GATE_EXPECTED_TEST_RESULT, to: COMMIT_LAYER, when: "at-verify-expectation == none"}`,
  placed before the catch-all `→ UNKNOWN_EXPECTED_TEST_RESULT` (~1460).
- Update the `GATE_EXPECTED_TEST_RESULT` doc-comment (~1411–1419) to describe the
  compile-only `none` route; note `START_SYSTEM` intentionally still runs (idempotent, warms
  the system for downstream nodes).

### Item 3 — `process-flow.yaml` CT-HIGH `IMPLEMENT_AND_VERIFY_DSL`: compile-only
- Node `IMPLEMENT_AND_VERIFY_DSL` (~1055–1063) in
  `implement-and-verify-external-system-driver-adapters-contract-tests`: add `verify-mode: none`.
- `expected-test-result`: now inert (no agent prompt consumes it — confirmed by grep; only the
  gate read it, and the gate now skips). **Keep it provided** (value irrelevant) so the inner
  `${expected-test-result}` reference in `implement-and-verify-dsl`/`implement-test-layer`
  still resolves under strict `ExpandParams`; add a one-line comment that it is inert on the
  `none` path. `suite: contract-real` / `test-names` likewise inert-but-retained.
- Add a comment: the DSL/port layer compiles + commits only; `contract-real` red/green is
  owned solely by `PROBE_CONTRACT_REAL` (outcome-driven, post-adapters); this stops
  `fix-unexpected-failing-tests` from front-running `external-system-driver-adapter-implementer`.

### Item 4 — `transitions_test.go`: lock the topology
- Assert the new `implement-test-layer` edge `at-verify-expectation == none → COMMIT_LAYER`.
- Assert the CT-HIGH `IMPLEMENT_AND_VERIFY_DSL` call pins `verify-mode: none` (so no
  contract-suite verify sits between the DSL impl and `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`).
- Guard: no path from the CT-HIGH DSL step reaches `VERIFY_TESTS_PASS_FILTERED` /
  `fix-unexpected-failing-tests` on `contract-real`.
- These cases must **fail on current `HEAD`** (proof the change bites) and pass after Items 1–3.

### Item 5 — comment / prose drift
- Sweep the CT-HIGH doc-blocks for any text implying the DSL layer green-checks `contract-real`
  and correct it. **No diagram regeneration step** — `docs/process-diagram.md` + `docs/images/*.svg`
  are auto-regenerated by the GH Actions workflow on push to `main` (see
  [[feedback_plans_no_diagram_regen]]); a local regen races it.

## Verification (after agent work)

- Re-run the #72 rehearsal (`rehearsal-…-72-charge-shipping-based-on-product-weight`): the
  CT-HIGH DSL step compiles + commits with **no** `fix-unexpected-failing-tests`;
  `external-system-driver-adapter-implementer` does the `Ext*`/driver plumbing; then
  `PROBE_CONTRACT_REAL` judges green and proceeds to the stub leg.
- Confirm a contract change that genuinely needs new adapter work greens via the
  adapter-implementer (not the fixer).
- Confirm a **new-endpoint** contract change still goes real-RED at `PROBE_CONTRACT_REAL` →
  simulator branch (the path `20260606-1943` protects must not regress).
- Run `go test ./internal/atdd/...` (Windows: `-p 2` or `scripts/test.sh`, never unbounded —
  [[feedback_go_test_windows]]).
