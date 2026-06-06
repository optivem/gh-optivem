# Plan: Cover path — expect AT green only when plumbing is complete

> **DECISION MADE (2026-06-06):** on the legacy-coverage path the acceptance test is expected to go
> **green exactly when the last piece of test plumbing it needs is wired**, not at a fixed layer and
> not uniformly. Settled in review discussion. This plan describes the resulting design.
>
> Review finding §1(a). One of several independent plans spun from the `process-flow.yaml` review;
> siblings cover the flag-clobber (§1b), dead `compile`/`compile-system` processes (§2),
> scope-exception coverage (§3), and doc drift (§4). **Hard dependency on the §1(b) plan — see
> Dependencies.**

## Why

`cover-system-behavior` (the `task/legacy-coverage` cycle) pins `expected-test-result: success`
uniformly via `write-and-verify-acceptance-tests-pass`, and forwards that one constant to **every**
layer of the shared `write-and-verify-acceptance-tests` / `shared-contract` cascade
(`process-flow.yaml`: the wrapper pins `success`; `write-and-verify-acceptance-test-code`,
`implement-and-verify-dsl`, and `implement-and-verify-system-driver-adapters` each verify against it).

For legacy coverage the **system already has the behavior**. The only thing that can keep the test
red is missing *test* plumbing (DSL core + driver adapters), and the same `acceptance-test-writer`
appends throwing `TODO: DSL` stubs. So the test goes green the moment the last needed plumbing layer
is wired — and **which layer that is depends on what the writer flagged**:

| What the writer flagged | Last plumbing layer that runs | Green lands at |
|---|---|---|
| nothing (`dsl-port-changed == false`) | none | the **code** layer |
| DSL changed, no driver-port change | `implement-and-verify-dsl` | the **DSL** layer |
| DSL + system-driver-port changed | `implement-and-verify-system-driver-adapters` | the **adapters** |
| DSL + external-driver-port changed only | the external CT-HIGH | **nowhere today** ⚠ |

Two defects from the uniform `success`:

1. **Premature green expectation.** The code and DSL layers are told to expect PASS even when a
   pending DSL / driver-port change means their plumbing isn't wired yet. A genuinely-red test then
   mis-routes into `fix-unexpected-failing-tests`.
2. **No terminal green assertion on the external-only branch.** When only
   `external-driver-port-changed` is true, the external CT-HIGH verifies the *contract* tests
   (real/stub), not the AT; `write-and-verify-acceptance-tests` then re-gates on
   `system-driver-port-changed` (false) and exits to `WAV_AT_END`. The cover run *ends* having
   last-verified the AT as **failing** at the DSL layer — it never confirms the AT greened.

## Design (chosen)

**Rule:** on the cover path, each AT verify expects **PASS iff no further plumbing change is pending**
(all downstream `*-port-changed` flags are false); otherwise **FAIL**. The terminal layer always
expects PASS.

This is the exact mirror of `change-system-behavior`: change is *uniformly red* through the cascade
(behavior not yet in the system) and greened by the trailing `implement-and-verify-system` step;
cover is *red until plumbing is complete*, then green, because the behavior is already present. No
new conceptual signal is invented — the rule reuses the `dsl-port-changed`,
`system-driver-port-changed`, and `external-driver-port-changed` gates that already exist.

**Mechanism:**

- A derived state bool **`plumbing-pending`**, recomputed at each cascade step from the flags the
  step's writer/implementer just emitted:
  - after the test-code writer → `plumbing-pending = dsl-port-changed`
  - after the DSL implementer → `plumbing-pending = system-driver-port-changed || external-driver-port-changed`
  - after the system-driver adapters / external CT branch → `plumbing-pending = false`
- A **mode signal** threaded from the two cover/change wrappers down through the shared cascade so the
  shared layers behave differently without forking the processes. Carried as a `verify-mode` param
  with values `red` (change / `write-and-verify-acceptance-tests-fail`) and `green-when-complete`
  (cover / `write-and-verify-acceptance-tests-pass`).
- The per-layer verify gate (today `GATE_EXPECTED_TEST_RESULT`, binding `expected-test-result`)
  becomes **mode-aware**: in `red` mode it routes on the forwarded `expected-test-result` constant
  exactly as today (change path untouched); in `green-when-complete` mode it routes on
  `plumbing-pending` (pending → verify-fail, else verify-pass).
- **Case D fix:** add a terminal AT-green verify on the external-driver-only branch so cover always
  ends on a PASS assertion regardless of which layer was last.

Naming avoids layer/grammar coding per `[[feedback_no_layer_coding_in_names]]`; `plumbing-pending`
and `verify-mode` describe scope, not position.

## Items

1. **Binding: `plumbing-pending` derive.** Add the recompute in the relevant gate/validate bindings
   (`internal/atdd/runtime/actions/bindings.go`) so `plumbing-pending` is written into `ctx.State`
   at the three recompute points above, from the same flags the existing port-changed gates read.
   This is Phase-D-style binding wiring.
2. **Binding/gateway: mode-aware verify gate.** Make the verify gate route on `verify-mode`: `red` →
   `expected-test-result` (unchanged); `green-when-complete` → `plumbing-pending`. Either a new
   `at-expectation` binding consumed by the existing gate node, or split the gate — encoder's
   discretion, single backend.
3. **`process-flow.yaml`: thread `verify-mode`.** Pin `verify-mode: green-when-complete` on
   `write-and-verify-acceptance-tests-pass` and `verify-mode: red` on
   `write-and-verify-acceptance-tests-fail`; forward it through `shared-contract`,
   `write-and-verify-acceptance-tests`, `write-and-verify-acceptance-test-code`,
   `implement-and-verify-dsl`, `implement-test-layer`, and
   `implement-and-verify-system-driver-adapters` to each verify gate. Strict `ExpandParams` means
   every layer on both paths must bind it.
4. **`process-flow.yaml`: terminal green verify (case D).** Add an AT-green verify reached after the
   external-driver-only branch completes in `green-when-complete` mode, so cover ends on a PASS
   assertion when the external CT-HIGH was the last plumbing built. Keep the change-path topology
   (and the `UnrollSystemDriverAdapterChannels` anchor/gate the renderer relies on) intact.
5. **Tests.** Cover the four rows of the table above on the cover path (assert the verify *polarity*
   chosen at each layer), plus a regression asserting `change-system-behavior` still verifies
   uniformly fail then greens only at `implement-and-verify-system`. Scope `go test` per
   `[[feedback_go_test_windows.md]]`.

## Verification

- Walk a legacy-coverage ticket whose AT needs (a) no DSL change, (b) a DSL-only change, (c) a
  system-driver-port change, (d) an external-driver-port change — confirm each ends on a single green
  AT assertion and none mis-routes to `fix-unexpected-failing-tests`.
- Confirm a `change-system-behavior` story still behaves exactly as before.

## Dependencies

- **§1(b) flag-clobber (hard).** `plumbing-pending` is only correct if `dsl-port-changed` /
  `system-driver-port-changed` / `external-driver-port-changed` reflect the **AT-side** writers at
  the point each layer's gate reads them. Today the nested external CT-HIGH overwrites all three
  (`write-contract-tests` re-emits `dsl-port-changed`; its inner DSL impl re-emits the driver flags,
  and `bindings.go:1066` *forces* `system-driver-port-changed=false` on the contract path). Until the
  §1(b) plan scopes/snapshots those flags so the AT-cascade re-reads survive the CT excursion, the
  case-D terminal verify and the post-CT recompute will read clobbered values. Sequence §1(b) first,
  or land both together.
