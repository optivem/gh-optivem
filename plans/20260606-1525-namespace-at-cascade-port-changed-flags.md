# Plan: Namespace the AT-cascade port-changed flags so the CT excursion can't clobber them

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-06T13:48:58Z`

## TL;DR

**Why:** The three `*-port-changed` verdicts are flat global `ctx.State` keys, so the nested external CT excursion overwrites the acceptance cascade's values (and `bindings.go:1066` forces `system-driver-port-changed=false`); the parent re-gate then skips the required system-driver-adapter step, leaving the AT red and mis-routing it to the fixer.
**End result:** Each verdict is stored under a cascade-namespaced key (`at-*` for the acceptance cascade, `ct-*` for the contract excursion); the CT excursion writes only `ct-*`, the parent re-gates read the surviving `at-*` values, the required adapter step runs, and a human trace shows both verdicts distinctly attributed.

> **DECISION MADE (2026-06-06):** the three `*-port-changed` verdicts are stored under
> **cascade-namespaced** State keys (`at-…` for the acceptance cascade, `ct-…` for the nested
> contract excursion) so the parent re-gates read the AT-side verdict the CT excursion can never
> overwrite — and so a human reading the trace sees both verdicts distinctly attributed. Chosen over
> snapshot/restore (B) because B leaves a "magic" restore mutation in the trace with no writer
> attribution. Settled in review discussion.
>
> Review finding §1(b). Independent plan from the `process-flow.yaml` review;
> `[[20260606-1518-cover-path-green-when-plumbing-complete]]` (§1a) has a **hard dependency** on this
> one — see Dependents.

## Why

`dsl-port-changed`, `system-driver-port-changed`, and `external-driver-port-changed` are flat global
`ctx.State` keys. `wrapCallActivity` shares `State` across the whole call tree (only Params are
push/popped — `run.go`), and every writer/implementer that emits one **overwrites** the same key.

Inside `write-and-verify-acceptance-tests` → `shared-contract`, the nested external CT-HIGH
(`implement-and-verify-external-system-driver-adapters-contract-tests`) re-runs writers that emit the
same keys:

```
WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE   → emits dsl-port-changed        (AT writer)
IMPLEMENT_AND_VERIFY_DSL                → emits system/external-port-changed (AT-side DSL impl)
  CT-HIGH:
    WRITE_CONTRACT_TESTS                → emits dsl-port-changed          ← CLOBBERS AT value
    IMPLEMENT_AND_VERIFY_DSL            → emits system/external flags      ← CLOBBERS again
...parent re-gates (process-flow.yaml:782, :787) now read the CT writer's values, not the AT's.
```

`actions/bindings.go:1066` *forces* `system-driver-port-changed=false` on the contract path (it halts
otherwise). So whenever the external CT path runs, `system-driver-port-changed` is deliberately driven
to **false** in shared state. If the AT-side DSL impl had set it **true** (system-driver adapters
required) and `external-driver-port-changed` was also true (so the CT-HIGH runs), the parent re-gate
`GATE_SYSTEM_DRIVER_PORTS_CHANGED` (:787) then reads **false** and **skips the required
`IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS` step** — the AT stays red, mis-routing to the fixer.
The `dsl-port-changed` re-read (:782) has the same hazard.

Params can't fix this: the buggy re-reads live in the **parent** (`write-and-verify-acceptance-tests`)
reading flags produced inside the **child** (`shared-contract`); Params are popped on return, so only
shared `State` flows upward. The value must stay in `State` — the fix is to stop the CT excursion
from writing the AT cascade's keys.

## Design (chosen: A — cascade namespacing)

Store each verdict under a key namespaced by the cascade that produced it, discriminated by the
already-threaded `tests` context (`acceptance` vs `contract`):

- acceptance cascade → `at-dsl-port-changed`, `at-system-driver-port-changed`,
  `at-external-driver-port-changed`
- contract excursion → `ct-dsl-port-changed`, `ct-system-driver-port-changed`,
  `ct-external-driver-port-changed`

The CT excursion writes only `ct-*`, so the AT cascade's `at-*` keys survive it untouched. Gate
bindings are repointed to the namespaced key for their cascade. No agent prompt changes — the agents
still emit the bare `*-port-changed` output; the **landing layer** (the dispatcher/validator that
flattens `gh optivem output write` into `ctx.State`) writes the namespaced key based on the active
`tests` context. Single backend, no dual-path generic+namespaced coexistence per
`[[feedback_testselect_parsing_escalation]]`.

Names describe scope (which cascade owns the verdict), not layer/grammar position, per
`[[feedback_no_layer_coding_in_names]]`.

## Items

1. **Landing layer namespacing** (`actions/bindings.go`, `validate-outputs-and-scopes` /
   output-flatten path). When a writer emits a `*-port-changed` output, write it to the
   `at-`/`ct-`-prefixed key selected by the active `tests` context rather than the bare key. Remove
   the bare-key write (single backend). Update the CT-path guard at `bindings.go:1066` to assert on
   `ct-system-driver-port-changed`.
2. **Gate bindings** (`gates/bindings.go`). Repoint `dslPortChanged`,
   `systemDriverPortChanged`, `externalDriverPortChanged` to the namespaced keys. If a single gate
   node must read whichever cascade is active, derive the key from the `tests` context inside
   `boolStateGate`; if specific nodes are cascade-fixed, register cascade-specific bindings.
3. **`process-flow.yaml` gateway bindings.** Repoint the 5 gateway `binding:` sites to the
   namespaced verdicts: AT-side `GATE_DSL_PORT_CHANGED` (:701), `GATE_EXTERNAL_DRIVER_PORTS_CHANGED`
   (:716), and the parent re-gates (:782 `dsl-port-changed`, :787 `system-driver-port-changed`) → the
   `at-*` keys; the CT-HIGH's inner `GATE_DSL_PORT_CHANGED` (:959) → `ct-dsl-port-changed`.
4. **Audit other readers.** Check `preflight/preflight.go` and any non-test reader surfaced by
   grepping the three flag names; repoint or confirm out of scope. No reader should still read a bare
   `*-port-changed` key after this lands.
5. **Tests.** Add a regression for the clobber scenario: AT-side DSL impl sets
   `system-driver-port-changed=true` **and** `external-driver-port-changed=true`, the CT-HIGH runs and
   forces its `ct-` flag false, and the parent re-gate still routes into
   `IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS`. Update existing flag-name assertions in
   `actions/bindings_test.go`, `gates/bindings_test.go`, `statemachine/*_test.go`, `trace_test.go`,
   `clauderun_test.go`. Scope `go test` per `[[feedback_go_test_windows.md]]`.

## Verification

- Trace a run where the AT side needs system-driver adapters *and* an external-driver contract change:
  confirm the trace shows `at-system-driver-port-changed=true` and `ct-system-driver-port-changed=false`
  as distinct, attributed entries, and that the adapter step runs.
- Confirm a run with no external-driver change behaves exactly as before (no `ct-*` keys written).

## Dependents

- **§1(a) `[[20260606-1518-cover-path-green-when-plumbing-complete]]` (hard).** Its `plumbing-pending`
  derivation reads these same three flags; it is only correct once they survive the CT excursion under
  the `at-*` keys. Land this plan first, or land both together.
