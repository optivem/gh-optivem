# Support a ticket that changes MORE THAN ONE external system's driver port

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-16T05:37:27Z`

## TL;DR

**Why:** A ticket whose change touches two external systems' driver ports (e.g. a method on `erp` and a DTO on `clock`) cannot be served today — the parent identity fix turns `>1` external systems into a hard error, so the author must split the ticket by hand.
**End result:** The external-driver contract cycle runs **once per touched external system** — statically unrolled over the `external-systems:` registry, each clone guarded by its own "did *my* port change?" check and carrying its own baked `real-kind` routing. `identifyExternalSystem` is retired; its port→names-set logic survives as a shared helper feeding the per-clone guard and an upfront "all touched systems registered" validation.

**Date:** 2026-06-15 (local)
**Status:** Refined 2026-06-16 — all open questions (A–E) resolved, design fully pinned; ready for `/execute-plan`.
**Follow-up to:** `plans/20260613-1835-external-system-identity-dto-only-change.md`
(the identity fix). That plan resolves identity **solely** from the preserved
external-driver-port changed-paths and makes a **two-external-systems ticket a hard error**
(`>1` names → stop). This plan replaces that hard stop with actual multi-external-system
handling, reusing the same preserved `external-driver-port-changed-paths` set as the
per-external-system selector.

The refined design (below) goes further than the parent's slim-IDENTIFY assumption: the
only **consumed** output of identification is `real-kind` (the contract-real probe-routing
polarity, read by `GATE_CONTRACT_REAL_RED_KIND`) — `external-system-name` is stamped today
but **never read** downstream. So identifying an external system exists purely to resolve its
`real-kind` (and, by the same name key, its config-declared stub/simulator paths). The
design therefore **retires `identifyExternalSystem` entirely**, baking each external system's name
and `real-kind` into its unrolled clone at load time.

**Terminology:** throughout this plan, "system" means an **external system** — a
`cfg.ExternalSystems` entry (e.g. `erp`, `clock`). Our own system-under-development is
singular, is never unrolled, and is out of scope; the multiplicity this plan adds lives
entirely on the external side. Bare "system" / "per-external-system" / "registered external system" below all
mean the external one.

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

- **Before the identity fix:** the adapter-scan resolved only the external system that happened to
  get an adapter file and **silently skipped the other** (its contract was never tested).
- **After the identity fix (parent plan):** the preserved port-path set now contains both
  names, so the `>1` branch fires a **hard error** — honest, but the ticket still can't
  proceed. The author must split it into one-external-system-per-ticket by hand.

This plan makes a multi-external-system ticket run the contract cycle **once per touched external
system**, each resolving its own `real-kind` and red→green independently.

## Background — the existing precedent is STATIC UNROLL, not a runtime loop

The statemachine already solves "run this sub-cycle once per project-declared item" for
**channels** (`internal/engine/statemachine/channels.go`):

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
registered external system on every ticket — wrong. So each unrolled per-external-system clone must be
**guarded** by a per-external-system "did *this* external system's port change?" gate that **skips** the
clone when the external system is untouched.

## Recommended approach — static unroll over the registry, per-external-system guard, identity+real-kind baked at load

The architecture is essentially **forced**: unroll runs at **load time** over config-known
lists, but the touched-external-system subset is **runtime data** (computed by the DSL phase). You
cannot unroll over the touched external-system set — you don't know it yet. So you unroll over the
**registry** (load-time-known, like channels) and **guard** each clone at runtime. The plan's
original instinct was right; the refinements below come from *what identification is actually
for*.

What we established from the code:

- The cycle's only **consumed** identity output is `real-kind` — read by
  `GATE_CONTRACT_REAL_RED_KIND` (`internal/atdd/process/gates/bindings.go`), which routes the
  contract-real probe (`simulator` → red→implement→green; `test-instance` → upstream-gap-halt).
- `external-system-name` is stamped by `identifyExternalSystem`
  (`internal/atdd/process/actions/bindings.go`) but **never read** by any non-test code.
- Each `cfg.ExternalSystems[<name>]` entry is **fully self-describing**: `real-kind`, plus the
  always-present `stub.{path,repo}` and (iff simulator) `simulator.{path,repo}`. Those paths
  are already consumed by `preflight.go` (whole-registry existence checks) and the
  `driver.go` config banner — no new config is needed for multi-external-system.

So "run the cycle per external system" exists to give each external system **its own `real-kind` routing**, and
the baked `<name>` is the registry lookup key for everything else (real-kind, stub/simulator
paths). Mirror `UnrollSystemDriverAdapterChannels` (`internal/engine/statemachine/channels.go`),
with these five points:

1. **Unroll per *registered* external system, baking the per-external-system config attributes.** A new
   `UnrollExternalSystems` clones the `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` anchor
   into one call-activity per `cfg.ExternalSystems` entry (deterministic key order, like
   channels), baking `external-system-name: <name>` AND `real-kind: <cfg value>` into each
   clone's `params:` at load time. (If the cycle's stub-writing steps consume the
   stub/simulator path at runtime, bake those too — same registry entry, same mechanism;
   see verify E.) Looking up `real-kind` at unroll makes it a static, analyzable value and
   turns the enum check (`test-instance | simulator`) into a **load-time** validation.
2. **Per-external-system entry guard replaces the boolean gate.** Today
   `GATE_EXTERNAL_DRIVER_PORTS_CHANGED` ("did *any* port change?") gates the single cycle. Per
   clone it becomes "is *my* baked `<name>` in the names-set derived from
   `external-driver-port-changed-paths`?" Untouched external system → the clone no-ops (routed past).
   The membership check reuses the path→names-set logic factored out of the retired IDENTIFY
   (point 3). A ticket touching only `erp` runs only the `erp` clone; `erp` + `clock` runs
   both, sequentially (the unroll wires clones in a linear chain).
3. **Retire `identifyExternalSystem` entirely** — not just its `>1` branch. Its only consumed
   output (`real-kind`) is baked at unroll, so the whole action goes. Its port-path→names-set
   derivation (the parent plan's #65 fix — identity from the port change, never adapter files)
   is **preserved** by factoring it into a shared helper that the guard (2) and the
   registration check (4) both call. The parent's in-IDENTIFY `>1` hard error retires: `>1`
   now simply means `>1` guard fires, which is the whole point. The dead `external-system-name`
   stamping is removed.
4. **One upfront "all touched external systems registered" validation**, before the unrolled clones.
   Every name in the changed-set must correspond to a registered external system (i.e. have a clone);
   an unmatched name is a **hard error**. This is where the **no-silent-skip guarantee** (the
   original #65-class bug the parent plan closed with its unregistered-name error) now lives —
   it sees the whole changed-set against the whole registry. The zero-names case stays covered
   by the existing entry gate (a port change is guaranteed whenever the cascade reaches here).
5. **`GATE_CONTRACT_REAL_RED_KIND` reads the baked `real-kind`** from clone params (seeded to
   `ctx.State` at sub-process entry, per the channel precedent) instead of from
   IDENTIFY-stamped state. Pure state-reader, otherwise unchanged.

### Why not the alternatives (record, don't reopen)

- *Runtime loop over the identified external-system set.* Rejected — the statemachine has no generic
  runtime loop and deadlocks / blows up RAM on loopback edges
  (`feedback_statemachine_test_loop_hazard.md`). Static unroll is the established idiom.
- *Unroll over only the touched external systems.* Impossible — the touched external-system subset is runtime data,
  but unroll runs at load time over config-known lists. Hence unroll-over-registry + runtime
  guard.
- *Keep the `>1` hard error and force authors to split tickets.* The parent plan's status quo.
  Pushes a structural limitation onto every multi-external-system story; the unroll removes it
  for the same project-declared-list cost channels already pay.
- *Slim IDENTIFY to validate+stamp the injected name (this plan's original draft).* Superseded
  — once `real-kind` is baked at unroll, IDENTIFY has no surviving consumer, so deleting it is
  cleaner than keeping a validate-only action. Identity/real-kind resolution moves to load
  time, matching the statemachine's static-analysis philosophy.
- *Static unroll over the full registry with NO guard.* Rejected — would run the full cycle
  (write-contract-tests, adapter impl, probe) for every registered external system on every
  ticket, most hitting a no-op / zero-change path. The per-external-system changed-guard keeps the
  cost proportional to what the ticket actually touched.

---

## Resolved decisions (refined 2026-06-15)

1. **Per-external-system changed-gate binding → RESOLVED.** The unroll bakes `external-system-name`
   into each clone's `params:`; the guard reads that baked name and checks membership in the
   names-set derived from `external-driver-port-changed-paths`. The derivation is factored out
   of the (retired) `identifyExternalSystem` into a shared helper. The gate binding lives in
   `internal/atdd/process/gates/bindings.go` (where the `real-kind` gate already lives); the
   changed-paths set is stashed by the actions binding in
   `internal/atdd/process/actions/bindings.go`.
2. **`real-kind` baked at load → RESOLVED.** Looked up from `cfg.ExternalSystems[<name>].RealKind`
   at unroll time and baked into each clone; `GATE_CONTRACT_REAL_RED_KIND` becomes a reader of
   the baked param. `identifyExternalSystem` is retired entirely (its only consumed output was
   `real-kind`).
3. **Stub/simulator paths → already in config, no new mechanism.** Each
   `cfg.ExternalSystems[<name>]` declares `stub.{path,repo}` and (iff simulator)
   `simulator.{path,repo}`, already validated whole-registry by `preflight.go`. The baked
   `<name>` is the lookup key. Paths are **never baked** into clones (verify E resolved): the
   writing steps resolve their target via layer scope keys plus the threaded
   `external-system-name`; `stub.path`/`simulator.path` stay preflight/banner-only.

## Answers to A–E — ✅ RESOLVED (VJ confirmed 2026-06-16)

> Grounded recommendations written 2026-06-15 after reading the code; A re-verified against the
> runtime and B–E confirmed by VJ on 2026-06-16. All five are now Resolved and the dependent
> Item edits below have been applied.

A. **Params → `ctx.State` seeding → bake `real-kind` as a clone param + a ~3-line shim.
   RESOLVED (code-verified 2026-06-16).** Traced the runtime: baked call-activity `params:`
   land in `ctx.Params` (`run.go:180`, merged/pushed at call-activity entry), but
   `GATE_CONTRACT_REAL_RED_KIND` reads **only** `ctx.State["real-kind"]`
   (`gates/bindings.go:513`, strict presence check) — the two are separate spaces with no
   automatic bridge (`ExpandParams` reads both but never writes State). So the
   "push-seeds-gate-readable-state, IDENTIFY fully deleted, gate unchanged" branch does **not**
   hold; the **shim branch is confirmed.** Bake `real-kind` as a clone param, then copy it into
   gate-readable state once before the gate (e.g. `ctx.Set("real-kind", ctx.Params["real-kind"])`,
   today's stamp at `actions/bindings.go:595`). This is still a near-total deletion of IDENTIFY
   (no derivation, no `>1`, no name-resolution) — only a minimal param→State copy survives, and
   the gate binding itself is **unchanged**.
B. **Per-external-system key isolation → no suffixing. RESOLVED (confirmed 2026-06-16).** The
   cycle's only namespaced state is `${ct-test-names}` / `ct-dsl-port-changed`, produced by
   `write-contract-tests` at the *start* of each clone and consumed within the same clone
   (`process-flow.yaml:1077,1178`). Sequential clones regenerate before consuming — no
   cross-clone read. **Item 5 dropped.**
C. **Channel-unroll composition → no N×M risk. RESOLVED (confirmed 2026-06-16).** The channel
   unrolls target `change-system-behavior` and `write-and-verify-acceptance-tests`
   (`channels.go:52,102`); the external anchor `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS`
   lives in **`shared-contract`** (`process-flow.yaml:705,784`) — a different process they never
   touch. `shared-contract` is a shared sub-process; unrolling external systems rewrites its
   single definition into N nodes regardless of channel call-frequency (additive, not
   multiplicative). **Resolved:** add `UnrollExternalSystems(cfg.ExternalSystemNames())` to the
   pre-`Bind()` block (`driver.go:318-331`), guarded by `len(cfg.ExternalSystems) > 0`;
   order-independent of the channel unrolls.
D. **`shared` external layer → fully independent clones, no `common:"true"`. RESOLVED (confirmed
   2026-06-16).** A shared layer exists (`system-driver-adapter-shared`, read+write-scoped by all
   three writing steps — `process-flow.yaml:1937,1991,2023`), but unlike the channel **common**
   layer (a from-scratch build) it is a pre-existing transport foundation each step "may extend"
   incrementally. **Resolved:** no first-clone ownership flag; each clone independent.
E. **Stub/sim path flow → do NOT bake paths; bake `external-system-name` and thread it into the
   writing agents' prompts. RESOLVED (confirmed 2026-06-16).** The writing steps resolve their
   target via **layer scope keys** (`external-system-driver-adapter`, `system-driver-adapter-shared`
   — `process-flow.yaml:1937,1991,2023`), not `stub.path`/`simulator.path` (preflight + banner
   only) and not `external-system-name` (currently unconsumed). Direct channel analog: the
   channel agent "reads `${channel}` and writes only that channel's adapter" (`channels.go:89-92`).
   **Resolved:** thread the baked `external-system-name` into the three writing agents' prompts
   (`${external-system-name}`, like `${channel}`); keep layer-level scope; **do not** bake
   config paths (kills the "bake the path" branch of resolved-decision 3).

### Item changes — ✅ APPLIED (2026-06-16)

- **Dropped Item 5** (per B); remaining items renumbered.
- **Item 4** gained a sub-task: thread `external-system-name` into the prompts of
  `implement-external-system-driver-adapters`, `implement-external-system-stubs`, and
  `implement-external-system-real-simulator` (per E).
- **Resolved-decision 3** lost its "bake the path into the clone" branch (per E): paths are
  preflight/banner-only and never baked.
- **Item 1** pass-order caveat (verify C) resolved to "order-independent; guard on non-empty
  registry; wire into the pre-`Bind()` block at `driver.go:318-331`."
- **Item 4 / verify A** resolved to the shim branch: gate unchanged, minimal param→State copy.

## Items (agent work)

- [ ] **1. Add `UnrollExternalSystems`** in `internal/engine/statemachine/channels.go` (or a
  sibling file), cloning the `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` anchor once per
  `cfg.ExternalSystems` entry via the `unrollChannelAnchor` pattern, baking
  `external-system-name: <name>` and `real-kind: <cfg value>` into each clone's `params:`.
  Wire it into the pre-`Bind()` block (`driver.go:318-331`) alongside the channel unrolls,
  guarded by `len(cfg.ExternalSystems) > 0`; order-independent of the channel unrolls (verify C
  resolved).
- [ ] **2. Per-external-system entry guard.** Replace/augment the cycle's boolean entry gate so each
  clone runs iff its baked `<name>` is in the names-set from `external-driver-port-changed-paths`;
  route a false verdict past the clone. Factor the path→names-set derivation out of
  `identifyExternalSystem` into a shared helper.
- [ ] **3. Upfront "all touched external systems registered" validation.** Before the unrolled clones,
  hard-error if any name in the changed-set is not a registered external system (preserves the
  no-silent-skip guarantee). Uses the shared names-set helper from Item 2 against
  `cfg.ExternalSystems`.
- [ ] **4. Retire `identifyExternalSystem`.** Delete the action and its registration from
  `internal/atdd/process/actions/bindings.go`. Replace its `real-kind` stamp with a minimal
  param→State shim that copies the baked clone param into gate-readable state before the gate
  (`ctx.Set("real-kind", ctx.Params["real-kind"])`); `GATE_CONTRACT_REAL_RED_KIND` itself is
  **unchanged** (still reads `ctx.State["real-kind"]`) — verify A confirmed baked params land in
  `ctx.Params`, not `ctx.State`. Remove the now-dead `external-system-name` stamping. The
  `>1` / unregistered-name / zero-name error cases are absorbed by Items 2–3 and the existing
  entry gate. **Sub-task (per E):** thread the baked `external-system-name` into the prompts of
  the three writing agents — `implement-external-system-driver-adapters`,
  `implement-external-system-stubs`, and `implement-external-system-real-simulator`
  (`${external-system-name}`, like `${channel}`); keep layer-level scope, do not bake config paths.
- [ ] **5. Unit tests** (`statemachine` + `actions`/`gates`): single-external-system ticket runs
  exactly one clone; two-external-system ticket runs both clones, each with its own baked `real-kind`;
  an untouched registered external system's clone is skipped; an unregistered touched external system
  hard-errors upfront.
- [ ] **6. BPMN doc-block / node-comment sync** for the unrolled anchor, the per-external-system guard,
  the upfront registration check, and the retired IDENTIFY. Content-only; **no
  diagram-regeneration step** (the regenerate-diagram workflow rebuilds `docs/process-diagram.md`
  on push, `feedback_plans_no_diagram_regen.md`).

## Verification (user-driven — not agent Items)

- [ ] `scripts/test.sh` (or scoped `go test -p 2 ./internal/atdd/runtime/...`) green. **Do
  not** run unbounded `go test ./...` on Windows (`feedback_go_test_windows.md`). Watch RAM —
  unroll/gate fixture changes can trip the statemachine loop hazard
  (`feedback_statemachine_test_loop_hazard.md`); kill on memory climb.
- [ ] Rehearsal: a story that changes two external systems' ports runs the contract cycle for
  **both** (each resolving its own `real-kind`), instead of erroring at `IDENTIFY`.
- [ ] Rehearsal with two external systems of **different** `real-kind` (one `simulator`, one
  `test-instance`) — confirms each clone routes its own `GATE_CONTRACT_REAL_RED_KIND`
  independently (the case the per-external-system split exists for; note the `test-instance` branch has
  no shop coverage today per `project_bpmn_full_coverage_story_and_realkind_gap.md`).
- [ ] A single-external-system story (e.g. `#65`-class) still runs exactly one cycle, unchanged.

## Risks / notes

- **Parent plan has landed.** This plan reuses `external-driver-port-changed-paths`; the
  identity fix (`20260613-1835-...`) is confirmed implemented (the set is stashed in
  `internal/atdd/process/actions/bindings.go`).
- **Retiring IDENTIFY is a delete right after the parent added it.** The parent plan
  (`20260613-1835`) created `identifyExternalSystem`; this plan deletes it. Not churn — its
  port-path→names-set logic (the #65 fix) is preserved as the shared helper; only the runtime
  action wrapper, the `>1` collapse, and the dead `external-system-name` stamping go.
- **Node-count growth.** One clone per registered external system (guarded). Keep the
  registry small; the guard keeps runtime cost proportional to external systems actually touched.
- **Scope discipline.** This is a multiplicity change to the external cycle only; do not fold
  in real-kind reshaping or unrelated routing changes. Baking `real-kind` at load is a
  *relocation* of the existing lookup, not a reshape.
