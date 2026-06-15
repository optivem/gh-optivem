# Support a ticket that changes MORE THAN ONE external system's driver port

**Date:** 2026-06-15 (local)
**Status:** Proposed — design plan, key decisions flagged for `/refine-plan`.
**Follow-up to:** `plans/20260613-1835-external-system-identity-dto-only-change.md`
(the identity fix). That plan resolves identity **solely** from the preserved
external-driver-port changed-paths and makes a **two-systems ticket a hard error**
(`>1` names → stop). This plan replaces that hard stop with actual multi-system
handling, reusing the same preserved `external-driver-port-changed-paths` set as the
per-system selector.

---

## Problem

The external-driver contract cycle
(`implement-and-verify-external-system-driver-adapters-contract-tests`,
`process-flow.yaml:1073`) is invoked **once** per ticket, from a single
call-activity (`IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`, ~784) behind a single
boolean entry gate (`GATE_EXTERNAL_DRIVER_PORTS_CHANGED`, binding
`at-external-driver-port-changed`, ~764). The whole cycle — write-contract-tests →
compile-only DSL/port → implement adapters → identify → probe-real → simulator/stub —
**targets exactly one external system** (one `real-kind`, one `PROBE_CONTRACT_REAL`).

So a ticket whose change touches **two** external systems' ports (e.g. a method on
`erp` **and** a DTO on `clock`) cannot be served:

- **Before the identity fix:** the adapter-scan resolved only the system that happened to
  get an adapter file and **silently skipped the other** (its contract was never tested).
- **After the identity fix (parent plan):** the preserved port-path set now contains both
  names, so the `>1` branch fires a **hard error** — honest, but the ticket still can't
  proceed. The author must split it into one-system-per-ticket by hand.

This plan makes a multi-external-system ticket run the contract cycle **once per touched
system**, each resolving its own `real-kind` and red→green independently.

## Background — the existing precedent is STATIC UNROLL, not a runtime loop

The statemachine already solves "run this sub-cycle once per project-declared item" for
**channels** (`internal/atdd/runtime/statemachine/channels.go`):

- `UnrollSystemChannels` / `UnrollSystemDriverAdapterChannels` take the project-declared
  channel set and, **at load time**, replace one template "anchor" call-activity node with
  **one cloned node per channel** (`unrollAnchor`). Each clone carries the template
  `params:` verbatim and overrides only the per-channel keys (`channel`, `suite`, and
  `common: "true"` on the first clone only).
- This is **static** (analyzable at load, no runtime iteration) precisely because the
  statemachine is prone to deadlock / 20GB-RAM blowups on runtime loopback edges
  (`feedback_statemachine_test_loop_hazard.md`). The unroll keeps the graph acyclic.

External systems are **also project-declared** — `cfg.ExternalSystems` (the
`external-systems:` registry in `gh-optivem.yaml`, the same one IDENTIFY validates against).
So the same static-unroll mechanism is the natural fit.

**The one structural difference from channels:** every channel is *always* exercised on
every ticket, but a ticket touches only a **subset** of external systems (the ones whose
port changed). A naive static unroll over the full registry would run the cycle for every
registered system on every ticket — wrong. So each unrolled per-system clone must be
**guarded** by a per-system "did *this* system's port change?" gate that **skips** the
clone when the system is untouched.

## Recommended approach — static unroll over the registry + per-system "changed" guard

Mirror `UnrollSystemDriverAdapterChannels`, adding the per-system skip guard:

1. **Unroll the external cycle per registered system.** A new
   `UnrollExternalSystems(systems []string)` clones the
   `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` anchor into one call-activity per entry
   in `cfg.ExternalSystems`, injecting `external-system-name: <name>` into each clone's
   `params:`. Iteration order is the registry's deterministic key order (mirrors channels).
2. **Guard each clone with a per-system changed-gate.** Each clone is preceded by a gateway
   that is `true` iff `<name>` appears in the preserved `external-driver-port-changed-paths`
   set (parent plan). Untouched systems → the clone is skipped (routed past), so a ticket
   touching only `erp` runs only the `erp` clone; a ticket touching `erp` + `clock` runs
   both, sequentially.
3. **IDENTIFY consumes the injected name instead of deriving it.** With the unroll supplying
   `external-system-name`, `identifyExternalSystem` no longer needs to derive the name at all
   for the multi-system path — it **validates** the injected name against `cfg.ExternalSystems`
   and stamps `real-kind`. The `>1` detection moves **up** to the unroll/guard layer (which
   sees the whole set); the parent plan's in-IDENTIFY `>1` hard error is thereby **retired**.
   The zero-names / unregistered-name errors remain as genuine stops.
4. **Per-system key isolation.** Each clone's `ct-*` namespaced verdicts and test-name lists
   (`ct-test-names`, `ct-dsl-port-changed`, etc.) must not collide across systems. Channels
   already solve the analogous per-channel key isolation; extend the same per-clone key
   suffixing (or scope) to the per-system clones.

### Why not the alternatives (record, don't reopen)

- *Runtime loop over the identified system set.* Rejected — the statemachine has no generic
  runtime loop and deadlocks / blows up RAM on loopback edges
  (`feedback_statemachine_test_loop_hazard.md`). Static unroll is the established idiom.
- *Keep the `>1` hard error and force authors to split tickets.* This is the parent plan's
  status quo. Acceptable as a stopgap, but it pushes a structural limitation onto every
  multi-external-system story; the unroll removes it for the same project-declared-list cost
  channels already pay.
- *Static unroll over the full registry with NO guard.* Rejected — would run the full cycle
  (write-contract-tests, adapter impl, probe) for every registered external system on every
  ticket, most hitting a no-op / zero-change path. The per-system changed-guard is what keeps
  the cost proportional to what the ticket actually touched.

---

## Open decisions to resolve in `/refine-plan`

1. **Per-system changed-gate binding.** New gate `binding:` that reads "is `<name>` in
   `external-driver-port-changed-paths`?" — confirm where it lives (gates `bindings.go`) and
   how the clone's `<name>` reaches the binding (param vs unroll-baked literal).
2. **Key-namespacing scheme for per-system clones.** Channel clones suffix per-channel keys;
   decide the exact per-system suffix/scope for `ct-test-names`, `ct-dsl-port-changed`, and
   the contract-real probe outcome so two systems can't clobber each other. Pin against the
   channels precedent.
3. **Interaction with channel unroll.** The external cycle sits inside the AT cascade, which
   is itself channel-unrolled in places. Confirm the external-system unroll and channel
   unroll compose (order of unroll passes in the load pipeline) without producing an N×M node
   explosion where it isn't wanted.
4. **`shared` external adapter code.** Decide whether the per-system clones need any
   shared/common external layer handling (analogous to channels' `common: "true"` on the
   first clone), or whether each external system is fully independent (no common layer).

## Items (agent work — to be finalized after the decisions above)

- [ ] **1. Add `UnrollExternalSystems` in `channels.go`** (or a sibling file), cloning the
  `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` anchor once per `cfg.ExternalSystems` entry,
  injecting `external-system-name`, following the `unrollAnchor` pattern. Wire it into the
  load-time unroll pipeline alongside the channel unrolls.
- [ ] **2. Add the per-system changed-guard gate** (binding in gates `bindings.go`) that is
  `true` iff the clone's `<name>` is in the preserved `external-driver-port-changed-paths`
  set; route a false verdict past the clone.
- [ ] **3. Retire the in-IDENTIFY `>1` hard error; consume the injected name.** Update
  `identifyExternalSystem` to validate+resolve `real-kind` for the injected
  `external-system-name`; remove the `>1` branch (now handled by unroll+guard). Keep the
  zero-names and unregistered-name errors.
- [ ] **4. Per-system key namespacing** for the cloned cycle's `ct-*` verdicts and test-name
  lists, per decision (2) above.
- [ ] **5. Unit tests** (`statemachine` + `actions`): single-system ticket runs exactly one
  clone; two-system ticket runs both clones with independent `real-kind`; an untouched
  registered system's clone is skipped; key isolation holds across two systems.
- [ ] **6. BPMN doc-block / node-comment sync** for the unrolled anchor and the new guard.
  Content-only; **no diagram-regeneration step** (the regenerate-diagram workflow rebuilds
  `docs/process-diagram.md` on push, `feedback_plans_no_diagram_regen.md`).

## Verification (user-driven — not agent Items)

- [ ] `scripts/test.sh` (or scoped `go test -p 2 ./internal/atdd/runtime/...`) green. **Do
  not** run unbounded `go test ./...` on Windows (`feedback_go_test_windows.md`). Watch RAM —
  unroll/gate fixture changes can trip the statemachine loop hazard
  (`feedback_statemachine_test_loop_hazard.md`); kill on memory climb.
- [ ] Rehearsal: a story that changes two external systems' ports runs the contract cycle for
  **both** (each resolving its own `real-kind`), instead of erroring at `IDENTIFY`.
- [ ] A single-system story (e.g. `#65`-class) still runs exactly one cycle, unchanged.

## Risks / notes

- **Depends on the parent plan landing first.** This plan reuses
  `external-driver-port-changed-paths`; the identity fix
  (`20260613-1835-...`) must be implemented before this one.
- **Node-count growth.** One clone per registered external system (guarded). Keep the
  registry small; the guard keeps runtime cost proportional to systems actually touched.
- **Scope discipline.** This is a multiplicity change to the external cycle only; do not fold
  in real-kind reshaping or unrelated routing changes.
