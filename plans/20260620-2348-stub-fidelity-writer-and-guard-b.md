# 2026-06-20 21:48:35 UTC — stub-fidelity-test-writer + Guard B (deferred pieces of the ESCC plan)

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

**Resolve the two design questions first (they're in `## Open questions`), then implement in two independent slices.** The recommended resolutions are pre-filled below — confirm or adjust them, then a fresh `/execute-plan` can act on the Steps without re-deriving.

Concretely, **Slice 1 (stub-fidelity-test-writer)** is the larger, higher-value piece and is independent of Slice 2; start there:
1. Add the stub-only test-names landing key (so it doesn't collide with `ct-test-names` at `internal/atdd/process/actions/outputs.go:140`).
2. Add the `stub-fidelity-test-writer` agent prompt + its `write-stub-fidelity-tests` process + the in-callee node (beside `WRITE_CONTRACT_TESTS`, `process-flow.yaml:1232`) + a stub-only probe/verify pair selecting the new key against `suite: contract-stub`.
3. Wire `${external-system-contract-criteria}` + `escc-format.md` into the new prompt; seed the render-matrix tests.

**Slice 2 (Guard B)** is independent: a new Go binding categorizing `scope-exception-files` by path family + a gateway/error-end off the `scope-exception-requested == true` branch (`process-flow.yaml:2847`).

## Background — where the deferred pieces sit

From the parent plan's deferred Steps 3-remainder and 4 (`plans/20260620-1850-…:159-160`). Confirmed against the code this session:

- **Test-names collision (the Slice-1 blocker).** Every contract writer emits `test-names`; `validateOutputsAndScopes` lands it under the cascade-namespaced key `ct-test-names` (`outputs.go:140`, via `landingStateKey(k, "contract")`). That single key is consumed by `PROBE_CONTRACT_REAL` (`process-flow.yaml:1322`), `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR` (`:1356`), `PROBE_CONTRACT_STUB` (`:1378`), and `VERIFY_TESTS_PASS_CONTRACT_STUB` (`:1408`). If the new writer also emits bare `test-names`, last-write-wins clobbers the shared list **and** the stub-only `@Isolated` tests would be handed to the contract-**real** probe — which must never happen (the real ERP can't be made empty / exact-set).
- **Guard B detection point.** The scope-exception is surfaced at `GATE_SCOPE_EXCEPTION_REQUESTED` (`process-flow.yaml:2751`); on `true` it routes straight to `STOP_SCOPE_VIOLATION` (`:2847`). `scope-exception-files` / `scope-exception-reason` are already optional outputs on the writing-agent MIDs (`:1890`, `:1929`, `:1973`). `ticket-has-escc` already lands in State (Slice A). So Guard B is a categorization + reroute, no new detection.

## Open questions

> Recommendations pre-filled per `[[feedback_resolve_questions_upfront]]` / `[[feedback_autonomous_best_long_term]]`. Confirm or override.

1. **Stub-only test-names: separate key + reuse `contract-stub` suite, or a whole new `contract-stub-isolated` suite group?**
   **Recommendation: separate test-names key, reuse the existing `contract-stub` suite.** The new writer lands its names under a distinct key (e.g. `ct-isolated-test-names`) — achieved by either emitting a differently-named output (`isolated-test-names`, left at identity by `landingStateKey`) or by adding it to `namespacedLandingKeys`. A new stub-only probe/verify pair runs `suite: contract-stub, test-names: ${ct-isolated-test-names}` against the already-running stub; the `@Isolated` class marker provides serialization, so **no new runner suite is required**. A `contract-stub-isolated` *suite group* (runner `SuiteGroups` alias) is a config nicety — add it only if a human will run the isolated set standalone; default is to skip it. *(This is the parent plan's option 2, minus the unneeded suite.)*
2. **Where does the new probe/verify pair sit in the callee?**
   **Recommendation: immediately after `VERIFY_TESTS_PASS_CONTRACT_STUB` (`process-flow.yaml:1402`), before `IMPL_EXT_DRIVER_CT_END`.** The stub is already built + greened on the shared register at that point, so the fidelity probe runs against a real stub. Outcome-driven like the others (probe → gate → implement-stubs-on-red → rebuild/restart → verify). Reuse `implement-external-system-stubs` for the red→green fix (the stub fidelity gap is still a stub-mapping gap).
3. **Guard B: exactly which Family B path placeholders count as "external contract/stub"?**
   **Recommendation: the union of the *write* paths in the scope blocks of `contract-test-writer`, the new `stub-fidelity-test-writer`, and `external-system-stub-implementer`** (contract-test path, external-stub-adapter path, and — if distinct — external-driver-adapter path). Pin the exact placeholder list against those scope blocks during execution rather than enumerating prose patterns. The categorizer matches `scope-exception-files` entries against those placeholder-resolved directories (prefix match under `${…}/**`, per `[[feedback_port_changed_flags_directory_keyed]]`).
4. **Guard B binding shape: new Go action, or expression over existing state?**
   **Recommendation: a new `boolStateGate`-style binding `scope-exception-needs-escc`** computed by a small action that reads `scope-exception-files` + `ticket-has-escc` + the resolved contract/stub path families, returning `true` iff (≥1 file under a contract/stub family) AND (`ticket-has-escc == false`). Register it in `gates/bindings.go` (+ `TestRegisterAll_AllBindingsRegistered` want-list) and land it with its YAML gateway so the registered-vs-referenced startup cross-check stays green.

## Steps

### Slice 1 — `stub-fidelity-test-writer` (resolves Q1/Q2, then implements)

- [ ] Step 1: **Separate the stub-only test-names key.** Per Q1, give the new writer's `test-names` a landing key distinct from `ct-test-names` (either a distinct output name left at identity, or a new entry in `namespacedLandingKeys`/`landingStateKey` in `internal/atdd/process/actions/outputs.go`). Add/adjust the unit coverage in `outputs_test.go` so the shared and stub-only lists provably don't clobber each other.
- [ ] Step 2: **Add the `stub-fidelity-test-writer` agent prompt.** New body under `internal/atdd/assets/runtime/agents/atdd/`, parallel to `contract-test-writer.md`: consumes the verbatim `${external-system-contract-criteria}` body + `escc-format.md`; authors **only** the `Stub only:` register as `<Sys>StubContractIsolatedTest` with `@Isolated` at class level (pull `isolated-marker-{java,csharp}.md`); clean non-conditional invariant — assert exact-set (`has exactly products`) + empty (`has no products`), stub only. No layer-coding in the name (`[[feedback_no_layer_coding_in_names]]`).
- [ ] Step 3: **Add the `write-stub-fidelity-tests` process + in-callee node + scope block + `${expected-outputs}`.** New `EXECUTE_AGENT`-backed node inside `implement-and-verify-external-system-driver-adapters-contract-tests`, beside `WRITE_CONTRACT_TESTS` (`process-flow.yaml:1232`); the existing `UnrollExternalSystems` clone picks it up per system for free (`channels.go` untouched — `[[reference_system_path_monolith_only_resolver]]` pattern, no re-anchor). Its scope block lists only the stub-only test path.
- [ ] Step 4: **Add the stub-only probe/verify pair.** Per Q2: after `VERIFY_TESTS_PASS_CONTRACT_STUB` (`process-flow.yaml:1402`), a `PROBE_CONTRACT_STUB_ISOLATED` → `GATE_…_OUTCOME` → (red) `implement-external-system-stubs` → rebuild/restart → `VERIFY_…_ISOLATED`, all `suite: contract-stub, test-names: ${ct-isolated-test-names}`. Wire `${external-system-contract-criteria}` into the new write node's params (mirror the `contract-test-writer` wiring done in the parent plan's Step 3).
- [ ] Step 5: **Seed the render-matrix + binding tests.** Extend `renderMatrixOpts` (TestRenderMatrix_NoUnfilledPlaceholders) for the new prompt's placeholders (`[[project_test_fixtures_monolith_only_gap]]`); add any new gate binding to `gates/bindings.go` want-list. Run scoped tests only (`-p 2` / `scripts/test.sh`, never unbounded `go test ./...` — `[[feedback_go_test_windows]]`), and watch for statemachine loop hazards on the new probe back-edges (`[[feedback_statemachine_test_loop_hazard]]`).

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
