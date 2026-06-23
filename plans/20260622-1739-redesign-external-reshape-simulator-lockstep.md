# 2026-06-22 15:39:00 UTC — Reshape the external simulator in lockstep on the redesign-external-system path

## TL;DR

**Why:** Rehearsal #81 ("Reshape ERP `GetProductResponse` structure, no behavior change") went deterministically RED on `ErpRealContractTest` because `redesign-external-system-structure` reshapes only the *consumer* side (testkit `Ext*` DTOs + system surface) and never reshapes the simulated *producer* the real contract test verifies against. The simulator kept emitting top-level `price` while the reshaped DTO expected nested `pricing`, so Jackson threw `UnrecognizedPropertyException`.
**End result:** The `redesign-external-system-structure` process reuses the CT cycle's *probe-driven* contract-real reconciliation leg — after the consumer reshape it probes the real contract test and, only if RED, branches on `real-kind`: reshape the simulator if we own it, or hard-halt with an upstream-gap message if it's a vendor test-instance. A shared plumbing fix wires the `external-systems:` registry's `simulator.path`/`stub.path` into the scope mechanism (today they are config-known but not scope keys), which also repairs the same latent gap in the CT cycle's simulator leg.

## Outcomes

What we get out of this — the goals and deliverables:

- A `redesign-external-system-structure` run that reshapes an external response contract reshapes **both** the simulated producer and the consumer adapter; `ErpRealContractTest` (and any simulator-backed real-contract test) stays GREEN when the reshape is behavior-preserving.
- The redesign cycle **reuses the CT cycle's probe-driven contract-real leg** (`PROBE_CONTRACT_REAL` → `GATE_CONTRACT_REAL_OUTCOME` → `GATE_CONTRACT_REAL_RED_KIND` → `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR` / `CONTRACT_REAL_UPSTREAM_GAP_HALT`) rather than a bespoke "reshape simulator" phase — composed into `redesign-external-system-structure` (`internal/atdd/process/process-flow.yaml:573–604`) after `UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS` and before the final full-regression `IMPLEMENT_AND_VERIFY_SYSTEM`.
- **Real-kind safety baked in:** the simulator is in the coupled set *only because and when* `real-kind == simulator` (we own the mock-server). A `real-kind == test-instance` system — a real vendor sandbox we do **not** own and **cannot** reshape — never gets a doomed simulator reshape; it routes to the existing upstream-gap halt. (A "reshape the external response" ticket is impossible by construction against a test-instance: we conform to the vendor, we don't restructure them.)
- The `external-systems:` registry's `simulator.path` / `stub.path` (shop config:43–67) **wired into the scope mechanism** as path keys (two keys, mirroring the config's simulator/stub split), so an agent can be scoped to write the dockerized simulator + stub mappings — today nothing can, even via scope-exception.
- **The coupled interface is visible across the reshape phases:** driver-adapter ↔ stub ↔ simulator are one interface seen from three sides, so each reshape phase can **read** all three coupled paths while **writing** only its own side — the consumer DTO change, the simulator reshape, and the stub reshape each see the whole interface but stay responsible for one representation (preserving the probe-driven "which side is out of sync" diagnostic).
- The **latent CT-cycle gap repaired as a side effect:** `external-system-real-simulator-implementer` (and the stub implementer) re-scoped to the actual simulator/stub dir instead of `${external-system-driver-adapter}` (the testkit), so its prose ("dockerized simulator, routes, fixtures, middleware") matches its scope.
- The existing consumer-side `external-system-driver-adapter-updater` agent — its scope and prompt — left **untouched** (per the chosen scope).

## ▶ Next executable step (resume here)

**BLOCKED — design decision required before Step 4 (see Open questions Q6).** Steps 2 and 3 are DONE (engine scope plumbing + simulator/stub MID scope + prompts + render fixtures; all `internal/atdd/...` + `projectconfig` tests green, uncommitted). Step 4 (compose the CT contract-real/stub leg into `redesign-external-system-structure`) hit a grounding gap the plan under-specified: the redesign cycle is **not unrolled per external system** (`UnrollExternalSystems` rewrites only the `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` anchor in shared-contract), so inside the redesign cycle there is **no `external-system-name`, no `real-kind` in state, and no `ct-test-names`** — yet the reused CT leg's `GATE_CONTRACT_REAL_RED_KIND` binds `real-kind`, the simulator/stub dispatch needs `${external-system-name}`, and the probe needs a test selection. Resolve Q6 (which composition model), then implement Step 4 + Step 5 (docs) + Step 6 (audit). Steps 4→5→6 are sequential.

## Steps

- [ ] Step 4: **Compose the contract-real AND contract-stub reconciliation legs into `redesign-external-system-structure`** (process-flow.yaml, the cycle starting at `redesign-external-system-structure:`): after `UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`, add build/start → `PROBE_CONTRACT_REAL` → `GATE_CONTRACT_REAL_OUTCOME` (green → continue) → `GATE_CONTRACT_REAL_RED_KIND` (simulator → `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR` → build/restart → verify; test-instance → `CONTRACT_REAL_UPSTREAM_GAP_HALT`; infra → `TESTS_INFRA_HALT`) → then the stub leg (`PROBE_CONTRACT_STUB` → reshape stub on RED) → then the existing `IMPLEMENT_AND_VERIFY_SYSTEM` full regression. **Both legs are included unconditionally**: each probes first and only reshapes on RED, so an already-consistent stub (as in #81, where the in-process stub driver self-reconciled) passes the probe at zero cost, and a stale stub gets caught. **BLOCKED on Q6** — the reused leg needs per-system identity (`external-system-name`) + `real-kind` + a contract-test selection, none of which exist in the (non-unrolled) redesign cycle today.
- [ ] Step 5: **Update process docs + render fixtures.** Sync `docs/atdd/process/*.md` for the redesign-external cycle, and extend the clauderun render matrix / test fixtures so the simulator/stub scope keys render for every agent × architecture × channel combination (per the monolith-only render-fixture gap convention).
- [ ] Step 6: **Verify via bpmn-logic-audit + targeted re-run.** Run the `bpmn-logic-audit` agent over the changed process to confirm call-graph reachability, gateway branch completeness, and YAML↔Go binding consistency for the composed leg. (Operator: re-run the #81 rehearsal to confirm the real contract test goes GREEN — captured under Verification, not an agent step.)

## Verification

- Operator re-runs the #81 `reshape-erp-getproductresponse` rehearsal end-to-end; `ErpRealContractTest.shouldBeAbleToGetProduct()` passes and the run no longer halts at `FIX_UNEXPECTED_FAILING_TESTS`.
- `go test` (scoped to `internal/atdd/process/...` with `-p 2`, per the Windows test-safety rule) passes for the binding/render tests.

## Open questions

Resolved by studying the CT cycle (folded into the steps above):
- ~~One scope key or two?~~ **Two** — the config already splits `simulator.path` vs `stub.path`; the scope keys mirror that split (Step 2).
- ~~New MID vs. repurpose?~~ **Reuse**, don't author a new MID — the redesign cycle composes the CT cycle's existing `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR` leg (Step 4). Resolves the earlier verb-split concern: no new prompt, so nothing to verb-name.
- ~~Phase ordering?~~ **Consumer-first, then probe-and-fix producer** — mirrors the CT cycle (adapters → probe contract-real → reshape simulator if RED). Not "producer-first" (Step 4).
- ~~Generality across external systems?~~ **Class-wide** — clock/erp/tax all declare `real-kind: simulator` and the same simulator/stub dirs; `test-instance` systems route to the upstream-gap halt (Step 2/4).

- ~~Stub leg needed, or just real?~~ **Both, unconditionally** — a contract reshape must keep the stub and real representations faithful. The probe-driven gate makes this free: an already-consistent stub (as in #81, where the in-process stub driver self-reconciled) passes the probe; a stale one gets reshaped (Step 4).

### Q6 (NEW — surfaced during Step 4 execution, 2026-06-22): how does the redesign cycle acquire per-system identity + real-kind?

The CT leg the plan asked to "reuse verbatim" depends on machinery the redesign cycle does not have. `UnrollExternalSystems` (channels.go) rewrites exactly ONE anchor — `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` in shared-contract — into per-system clones that bake `external-system-name` + `real-kind` into params and stamp `real-kind` into state (via `resolve-external-system`). `redesign-external-system-structure` is reached from the implement-ticket `external-system-redesign` subtype gate and the refactor menu — **neither unrolls it**. So the composed leg would reference an unbound `${external-system-name}` (simulator/stub dispatch), read an absent `real-kind` (the RED-kind gate), and have no contract-test list to probe.

Options (pick one, then Step 4 proceeds):

- **(A) Unroll the redesign cycle per external system** — add `redesign-external-system-structure` as a second `UnrollExternalSystems` anchor (clone its start node per registered system, baking `external-system-name` + `real-kind`, with a `resolve-external-system` + `GATE_EXTERNAL_SYSTEM_TOUCHED` self-guard like the CT cycle). Most faithful "reuse," gives `real-kind` for the gate. **Open sub-problem:** the per-system touched-guard reads `external-driver-port-changed-paths`, which the redesign path never populates (no AT cascade) — so the guard has no selection source; clones would run for every registered system, or need a new selection signal (parse the redesign ticket / ESCC for the target system).
- **(B) Real-kind-free, single-system composition** — assume a redesign-external ticket targets a simulator-backed system we own (the plan's own framing: a reshape is "impossible by construction against a test-instance"). Drop `GATE_CONTRACT_REAL_RED_KIND` + the upstream-gap halt; after the adapter reshape, probe contract-real and on RED go straight to the simulator implementer. **Still needs** `external-system-name` resolution + a contract-test selection, so it does not fully escape the gap — and it sheds the real-kind safety branch the plan explicitly wanted.
- **(C) Resolve identity without unrolling** — add a redesign-specific `resolve-*` service-task at the cycle start that derives the target external system(s) + real-kind from the redesign ticket (ESCC or a new ticket field) and stamps them into state, then run the leg whole-suite (`suite: contract-real, test-names: ""`). Keeps the real-kind gate; needs a defined selection source on the redesign ticket.

Recommendation: **(C)** — it preserves the real-kind safety branch the plan wanted and the probe-driven design, without forcing the full per-system unroll machinery (and its missing touched-signal sub-problem) onto a path that has no AT cascade. But it requires deciding the redesign ticket's external-system selection source. **This is the author's call — the choice changes the process shape materially.**

### Resolved earlier (CT-cycle study)
- ~~One scope key or two?~~ **Two** — config splits `simulator.path` vs `stub.path`; scope keys mirror that (Step 2, DONE).
- ~~New MID vs. repurpose?~~ **Reuse** the CT cycle's existing `IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR` leg (Step 4).
- ~~Phase ordering?~~ **Consumer-first, then probe-and-fix producer** (Step 4).
- ~~Generality across external systems?~~ **Class-wide** — clock/erp/tax all `real-kind: simulator` + same dirs (Step 2 DONE, Step 4).
- ~~Stub leg needed, or just real?~~ **Both, unconditionally** — probe-driven so an already-consistent stub passes free (Step 4).
