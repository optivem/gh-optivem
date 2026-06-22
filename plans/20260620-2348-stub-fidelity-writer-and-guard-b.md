# 2026-06-20 21:48:35 UTC — stub-fidelity-test-writer + Guard B (deferred pieces of the ESCC plan)

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-22T07:19:30Z`

## TL;DR

**Why:** The ESCC plan (`plans/20260620-1850-external-boundary-list-story-prevention.md`) shipped Slice A + Step 2 + the `contract-test-writer` half of Step 3, which already removes the #65 halt. Two pieces were deferred because each needs a design decision before coding: the **`stub-fidelity-test-writer`** (Stub-only / exact-set + empty register) and **Guard B** (loud "contract needed but undeclared" halt). Both are blocked on plumbing/categorization choices, not on prompt authoring.
**End result:** The two deferred design questions are resolved and implemented: (1) the stub-only fidelity register gets its own test-names key + probe/verify so its `@Isolated` exact-set/empty tests run against the **stub only** (never leaking into the `PROBE_CONTRACT_REAL` run), and the new `stub-fidelity-test-writer` agent authors `<Sys>StubContractIsolatedTest`; (2) a `system-implementer` scope-exception that names external-contract/stub files on a ticket with **no ESCC** fails loud with an actionable "add a `## External System Contract Criteria` section for `<system>`" message, categorized deterministically by path family rather than the cryptic generic `STOP_SCOPE_VIOLATION`.

## Outcomes

What we get out of this — the goals and deliverables:

- **A second contract writer exists.** `stub-fidelity-test-writer` authors the Stub-only register (exact-set `Then <Sys> has exactly products …` + empty `Then <Sys> has no products`) as `<Sys>StubContractIsolatedTest` (mirroring `ClockStubContractIsolatedTest`, `@Isolated` at class level), with its own clean non-conditional invariant — assert exact-set/empty, stub only — sitting beside `contract-test-writer`'s untouched shared/containment invariant.
- **The stub-only tests never run against the real driver.** The fidelity tests land under a **distinct test-names key** (not the shared `ct-test-names`), so `PROBE_CONTRACT_REAL` / the real-simulator verify keep running only the shared/containment register, while a new stub-only probe/verify runs the isolated fidelity tests against the **stub**. Clean failure localization survives: shared red → stub/real disagree; stub-only red → stub broken.
- **The fidelity register greens inside the existing room, before system implementation.** The new node rides inside the per-system callee (`implement-and-verify-external-system-driver-adapters-contract-tests`) that `UnrollExternalSystems` already clones — no `channels.go` re-anchor, no phase reorder. Boundary proven faithful before `system-implementer` relies on it.
- **Guard B fails loud and teaches.** When a `system-implementer` scope-exception names external-contract/stub files **and** the ticket declares no ESCC, the run halts with an actionable error naming the system and telling the author to add a `## External System Contract Criteria` section — instead of the generic `STOP_SCOPE_VIOLATION`. Unrelated scope-exceptions (non-contract files, or ESCC already present) keep routing to the existing generic halt, unchanged.
- **Path categorization is deterministic.** "Which files count as external-contract/stub" is decided by matching `scope-exception-files` against the **Family B path placeholders** that define the contract/stub writers' own scope blocks — not prose pattern-guessing (`[[feedback_paths_deterministic_no_guessing]]`, `[[feedback_substitutable_paths_in_docs]]`).
- **#65 fully exercises the new path** once Step 5/6 (operator: ESCC on issue #65 + rehearsal re-run) run — the list contract test, the stub-only fidelity test, and the list stub all exist and green before the system code is written.

## ▶ Next executable step (resume here)

**Slice 1 (stub-fidelity-test-writer) is DONE and committed (2026-06-22).** Slice 2 (Guard B) remains — it is independent of Slice 1, so a fresh `/execute-plan` can pick it up directly. Steps 6–8 below are fully specified (Q3/Q4 resolved). Concretely:

1. **Step 6** — add a path categorizer + new `scope-exception-needs-escc` boolStateGate binding in `internal/atdd/process/gates/bindings.go` (+ `TestRegisterAll_AllBindingsRegistered` want-list). It returns `true` iff (≥1 `scope-exception-files` entry sits under a contract/stub Family-B path family) AND (`ticket-has-escc == false`). The contract/stub families are the union of the *write* paths of `contract-test-writer` (`ct-test`, `dsl-port`, `dsl-core`), the now-shipped `stub-fidelity-test-writer` (`ct-test`), and `external-system-stub-implementer`. Resolve via `ResolveLayerPaths` + directory-prefix match (`[[feedback_port_changed_flags_directory_keyed]]`).
2. **Step 7** — insert `GATE_SCOPE_EXCEPTION_NEEDS_ESCC` on the `scope-exception-requested == true` edge (search `STOP_SCOPE_VIOLATION` in `process-flow.yaml`, was ~line 2847 before Slice 1's edits shifted line numbers): `true` → new `ESCC_UNDECLARED_HALT` error-end with the actionable message; `false` → existing `STOP_SCOPE_VIOLATION` unchanged.
3. **Step 8** — statemachine coverage for the three reroute cases.

Reference precedents from Slice 1: presence-style binding shape (`stubFidelityTestsPresent`, `gates/bindings.go`), strict `boolStateGate`, the `ResolveLayerPaths`-driven scope categorization in `internal/atdd/process/actions/outputs.go`.

## Background — where the deferred pieces sit

From the parent plan's deferred Steps 3-remainder and 4 (`plans/20260620-1850-…:159-160`). Confirmed against the code this session:

- **Test-names collision (the Slice-1 blocker).** Every contract writer emits `test-names`; `validateOutputsAndScopes` lands it under the cascade-namespaced key `ct-test-names` (`outputs.go:140`, via `landingStateKey(k, "contract")`). That single key is consumed by `PROBE_CONTRACT_REAL` (`process-flow.yaml:1322`), `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR` (`:1356`), `PROBE_CONTRACT_STUB` (`:1378`), and `VERIFY_TESTS_PASS_CONTRACT_STUB` (`:1408`). If the new writer also emits bare `test-names`, last-write-wins clobbers the shared list **and** the stub-only `@Isolated` tests would be handed to the contract-**real** probe — which must never happen (the real ERP can't be made empty / exact-set).
- **Guard B detection point.** The scope-exception is surfaced at `GATE_SCOPE_EXCEPTION_REQUESTED` (`process-flow.yaml:2751`); on `true` it routes straight to `STOP_SCOPE_VIOLATION` (`:2847`). `scope-exception-files` / `scope-exception-reason` are already optional outputs on the writing-agent MIDs (`:1890`, `:1929`, `:1973`). `ticket-has-escc` already lands in State (Slice A). So Guard B is a categorization + reroute, no new detection.

## Open questions

> **RESOLVED 2026-06-22 — all four pre-filled recommendations accepted by operator before execution.** Q1: separate `ct-isolated-test-names` key (add `isolated-test-names` to `namespacedLandingKeys`), reuse `contract-stub` suite, no new suite group. Q2: probe/verify after `VERIFY_TESTS_PASS_CONTRACT_STUB`, reuse `implement-external-system-stubs` for red→green. Q3: union of write paths of `contract-test-writer` + `stub-fidelity-test-writer` + `external-system-stub-implementer`. Q4: new `scope-exception-needs-escc` boolStateGate binding. Detail retained below for reference.

1. **Stub-only test-names: separate key + reuse `contract-stub` suite, or a whole new `contract-stub-isolated` suite group?**
   **Recommendation: separate test-names key, reuse the existing `contract-stub` suite.** The new writer lands its names under a distinct key (e.g. `ct-isolated-test-names`) — achieved by either emitting a differently-named output (`isolated-test-names`, left at identity by `landingStateKey`) or by adding it to `namespacedLandingKeys`. A new stub-only probe/verify pair runs `suite: contract-stub, test-names: ${ct-isolated-test-names}` against the already-running stub; the `@Isolated` class marker provides serialization, so **no new runner suite is required**. A `contract-stub-isolated` *suite group* (runner `SuiteGroups` alias) is a config nicety — add it only if a human will run the isolated set standalone; default is to skip it. *(This is the parent plan's option 2, minus the unneeded suite.)*
2. **Where does the new probe/verify pair sit in the callee?**
   **Recommendation: immediately after `VERIFY_TESTS_PASS_CONTRACT_STUB` (`process-flow.yaml:1402`), before `IMPL_EXT_DRIVER_CT_END`.** The stub is already built + greened on the shared register at that point, so the fidelity probe runs against a real stub. Outcome-driven like the others (probe → gate → implement-stubs-on-red → rebuild/restart → verify). Reuse `implement-external-system-stubs` for the red→green fix (the stub fidelity gap is still a stub-mapping gap).
3. **Guard B: exactly which Family B path placeholders count as "external contract/stub"?**
   **Recommendation: the union of the *write* paths in the scope blocks of `contract-test-writer`, the new `stub-fidelity-test-writer`, and `external-system-stub-implementer`** (contract-test path, external-stub-adapter path, and — if distinct — external-driver-adapter path). Pin the exact placeholder list against those scope blocks during execution rather than enumerating prose patterns. The categorizer matches `scope-exception-files` entries against those placeholder-resolved directories (prefix match under `${…}/**`, per `[[feedback_port_changed_flags_directory_keyed]]`).
4. **Guard B binding shape: new Go action, or expression over existing state?**
   **Recommendation: a new `boolStateGate`-style binding `scope-exception-needs-escc`** computed by a small action that reads `scope-exception-files` + `ticket-has-escc` + the resolved contract/stub path families, returning `true` iff (≥1 file under a contract/stub family) AND (`ticket-has-escc == false`). Register it in `gates/bindings.go` (+ `TestRegisterAll_AllBindingsRegistered` want-list) and land it with its YAML gateway so the registered-vs-referenced startup cross-check stays green.

## Steps

> **Slice 1 (`stub-fidelity-test-writer`, Steps 1–5) — DONE, committed 2026-06-22.** Shipped: `isolated-test-names` added to `namespacedLandingKeys` (lands `ct-isolated-test-names`); new `stub-fidelity-test-writer.md` prompt (Stub-only register only, `<Sys>StubContractIsolatedTest`, `@Isolated`); `write-stub-fidelity-tests` process + `WRITE_STUB_FIDELITY_TESTS` node + the stub-only probe/verify leg gated by a new `stub-fidelity-tests-present` presence binding (handles the *optional* `Stub only:` register without crashing strict ExpandParams — a refinement beyond the plan's literal Q2). Render-matrix needed no new seeding (new prompt reuses already-seeded placeholders). All scoped tests green; no statemachine loop hazard (forward-only leg).

### Slice 2 — Guard B (resolves Q3/Q4, then implements)

- [ ] Step 6: **Add the path categorizer + `scope-exception-needs-escc` binding.** Per Q3/Q4: a small Go action resolving the contract/stub Family B path families and matching `scope-exception-files` by directory-prefix; register `scope-exception-needs-escc` in `internal/atdd/process/actions/gates/bindings.go` (+ `TestRegisterAll_AllBindingsRegistered` want). Land it with its YAML gateway so the startup cross-check passes.
- [ ] Step 7: **Reroute the scope-exception branch.** Insert `GATE_SCOPE_EXCEPTION_NEEDS_ESCC` on the `scope-exception-requested == true` edge (`process-flow.yaml:2847`): `true` → new `ESCC_UNDECLARED_HALT` error-end with actionable message (`"this story needs a contract for <system> but declares no External System Contract Criteria; add a ## External System Contract Criteria section and re-run"`); `false` → existing `STOP_SCOPE_VIOLATION`, unchanged. Surface the system name from `scope-exception-files`/`escc-systems`.
- [ ] Step 8: **Test the reroute.** Statemachine coverage: contract-file scope-exception + no ESCC → `ESCC_UNDECLARED_HALT`; same files + ESCC present → `STOP_SCOPE_VIOLATION`; non-contract scope-exception → `STOP_SCOPE_VIOLATION`.

### Operator hand-off (parent plan Steps 5–6 — not agent work)

- [ ] Add `## External System Contract Criteria` to GitHub issue #65 in the `shop` repo. *(Operator — `[[feedback_plans_contain_agent_work_only]]`.)*
- [ ] Re-run the #65 rehearsal (multitier-java) and confirm: walks past `IMPLEMENT_AND_VERIFY_SYSTEM_API` without `STOP_SCOPE_VIOLATION`; list contract test, stub-only fidelity test, and list stub all exist and green; per-SKU stories still pass. *(Operator.)*

## Verification

- Slice 1: scoped `go test` on `internal/atdd/process/...` green (render-matrix, outputs, bindings); no statemachine deadlock/RAM climb on the new probe back-edges.
- Slice 2: scoped statemachine tests green for the three reroute cases above.
- End-to-end: operator #65 rehearsal re-run (Step in hand-off) — the only full proof the deferred path works.
