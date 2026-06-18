# Plan: close the adapter↔simulator wire-shape gap behind #65 contract-real `FIX_LOOP_EXHAUSTED` ({20260618-1937})

⏳ **PENDING DISCUSSION — not agreed, not picked up.** This file records the
2026-06-18 #65 contract-real investigation and a proposed orchestrator-side fix.
Do **not** start `## Items` until the operator confirms scope and picks an option
(see "Decision needed"). The fixes here are all in **`gh-optivem`** (agent prompts
/ process), not in `shop`.

Sibling: [[plans/deferred/20260613-1950-view-product-list-erp-list-gap.md]] closed
the *earlier* #65 wall (Java monolith `system-implementer` `STOP_SCOPE_VIOLATION`
— no enumerable ERP list path). That fix held: this 2026-06-18 TypeScript-monolith
rehearsal walked **past** it and hit a new, deeper wall in the **contract-real**
cycle. This plan addresses the new wall only.

---

## The error, and how we found it

**Symptom (from the rehearsal-loop run, worktree
`rehearsal-20260618-162156-65-view-product-list`, run `20260618-142209`):**

```
process "main" → IMPLEMENT_TICKET → CHANGE_SYSTEM_BEHAVIOR
  → WRITE_AND_VERIFY_ACCEPTANCE_TESTS → SHARED_CONTRACT
  → IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS_ERP
  → VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR
  → verify-tests-pass reached FIX_LOOP_EXHAUSTED

✗ shouldBeAbleToGetProductList (contract-real, erp)
  expect(received).toHaveLength(expected)
  Expected length: 1
  Received length: 0
  Received array:  []
```

The fix loop ran the test 3× (initial + 2 fix passes, summary steps 29–31) and
halted. It is a runtime fix-loop exhaustion, **not** a crash.

**How we traced it (the read trail):**

1. **The failing test** — `system-test/typescript/tests/latest/contract/erp/BaseErpContractTest.ts:16-28`
   seeds one product (`SKU-001` / `Product Alpha` / 10) then asserts
   `.productList().hasCount(1).and().includesProduct('Product Alpha', 10)`.
2. **The DSL arrange/act** — `src/testkit/dsl/core/scenario/then/then-contract.ts:75-82`
   (`ThenContractStage._arrange`) POSTs the seeded product via
   `erpDriver.returnsProductList(...)`; `:110-117` reads it back via
   `erpDriver.getProductList()`. The empty result lands here.
3. **The read client** — `src/testkit/driver/adapter/external/erp/client/BaseErpClient.ts:25-36`
   parses the list as a **wrapped object**:
   ```ts
   const data = (await response.json()) as { products?: ExtProductDetailsResponse[] };
   const products = (data.products ?? []).map(...)   // ← undefined ?? [] → []
   ```
4. **The real simulator** — `external-systems/simulators/mock-server.js:42-93`
   serves ERP via **json-server**, whose `GET /erp/api/products` returns a
   **bare array** `[ {id,title,price,…}, … ]` — never `{ products: [...] }`.
   So `data.products` is `undefined` → `[]`. **That is the `Received array: []`.**

**Two distinct defects confirmed:**

- **D1 — wire-shape mismatch (the visible red).** The adapter client
  (`external-system-driver-adapter-implementer`) parses `{ products: [...] }`;
  the simulator (`external-system-real-simulator-implementer`) emits a bare
  array. Neither side was pinned to the other's shape. Single-product works
  (`shouldBeAbleToGetProduct` hits `/api/products/{id}` → single object), so only
  the **list** endpoint diverges.
- **D2 — exact-count assertion vs pre-seeded simulator (latent second red).**
  `hasCount(1)` is unsatisfiable: the simulator ships 3 fixture products
  (`mock-server.js:42-69`) and the DSL POSTs 1 → a correctly-parsed list returns
  **4**, not 1. Even after D1 is fixed, `hasCount(1)` stays red.

**Why the fix loop could never repair it.** `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR`
(`internal/atdd/process/process-flow.yaml:1279`) dispatches `verify-tests-pass`,
whose fixer is `unexpected-failing-tests-fixer`. That fixer is explicitly told
(`internal/atdd/assets/runtime/agents/atdd/unexpected-failing-tests-fixer.md:32,38`)
that a never-green **new** behaviour is "case 3 → make no edits, bail." A
client/simulator shape mismatch is neither a SUT regression nor a reshaped
surface, so the fixer correctly declines, the loop hits its visit cap, and
`FIX_LOOP_EXHAUSTED` is the *designed* outcome — a tuning change would not help.
The real fix is upstream, at the agents that author the two sides.

---

## Root cause (orchestrator-side)

| # | Where | Gap |
|---|---|---|
| R1 | `external-system-driver-adapter-implementer.md:25` + `external-system-real-simulator-implementer.md:20` | No shared SSoT for the endpoint **wire shape**. Both prompts cite a "published contract" that exists nowhere concrete. The adapter prompt even **forbids** reading external-system source ("Do NOT read external-system source code"), so the adapter cannot learn the simulator's real shape; the simulator prompt's scope is the simulator dir and nothing points it at the adapter's parse. The two agents independently guess envelopes → D1. |
| R2 | `contract-test-writer.md:20` | No rule against **state-dependent** assertions (exact counts/totals) when `real-kind: simulator`. "Copy the sibling shape" produced `hasCount(1)` against a pre-seeded simulator → D2. |
| R3 | `external-system-real-simulator-implementer.md` (whole body) | Prompt is a flagged **TBD placeholder** (frontmatter: "TBD placeholder — Sonnet until this task is fleshed out"). One terse step, no shape-discovery guidance, no warning that json-server returns bare arrays for collections — and it is exactly the agent that had to get the list envelope right. |
| R4 (observation, likely no code change) | `process-flow.yaml` contract subprocess (`:1132-1331`) | No gate reconciles adapter-expected vs simulator-emitted shape before the build→verify→fix cycle. The routing itself is *correct* (it rightly refuses to let the SUT-fixer hack harness code). Conclusion: fix at R1–R3, not by adding a BPMN node — but flag for confirmation during discussion. |

---

## Decision needed (pick during discussion, before execution)

**Option A — fix R1 + R2 + R3 in the agent prompts (recommended).** Make the
simulator agent the shape-conformance owner (verify each route returns exactly
what the adapter parses, or vice-versa); add the `contract-test-writer`
exact-count guard; flesh out the TBD simulator prompt. Orchestrator-only, no
`shop` edits, survives worktree regeneration. Items below assume A.

**Option B — A, minus R2.** Treat the `hasCount` problem as a `shop` test-design
issue and fix only the wire-shape coordination. Cheaper, but leaves the latent
D2 red for the next list-shaped story.

**Option C — add a BPMN reconciliation gate (R4) on top of A.** Heaviest;
only if discussion concludes prompt guidance is insufficient and a structural
shape-check is warranted.

Recommendation: **A**. R4 deferred unless discussion says otherwise.

---

## Items (Option A) — proposed, pending confirmation

> All edits are in **`gh-optivem`** runtime agent prompts. Do not start until the
> operator confirms the option above.

### 1. [agent · simulator] Make `external-system-real-simulator-implementer` own shape-conformance

**Where:** `internal/atdd/assets/runtime/agents/atdd/external-system-real-simulator-implementer.md`.

**Change:** flesh out the TBD body (resolves R3) with a concrete contract:
- Discover the exact request/response shape each route must emit by reading the
  **driver adapter client** that parses it (the contract-real consumer), not by
  guessing — name the read target.
- Add an explicit self-verify step: every collection/list route must return the
  exact envelope the adapter parses (call out the json-server bare-array default
  as a known footgun).
- Keep the "implement only `${external-system-name}`" scope guard.

### 2. [agent · adapter] Let `external-system-driver-adapter-implementer` reconcile against the real shape

**Where:** `internal/atdd/assets/runtime/agents/atdd/external-system-driver-adapter-implementer.md:25`.

**Change:** relax the blanket "Do NOT read external-system source code" so the
adapter **may read the simulator's emitted response shape** for the endpoint it
translates (resolves R1's forbidden-coupling half), while keeping the ban on
inferring *behaviour* from system source. Pin which side is authoritative on
envelope shape so the two agents converge instead of guessing.

### 3. [agent · contract-test] Guard against exact-state assertions on `real-kind: simulator`

**Where:** `internal/atdd/assets/runtime/agents/atdd/contract-test-writer.md`.

**Change:** add guidance (resolves R2): when the external system is
`real-kind: simulator` (pre-seeded fixtures), prefer **presence** assertions
(`includesProduct`/seeded-entity-present) over exact totals (`hasCount(N)`);
exact counts are only valid against a controllable empty starting state.

---

## Verification

(Operator-driven — not agent `## Items` work.)

- Re-run `bash scripts/atdd-rehearsal.sh 65 --config gh-optivem-monolith-typescript.yaml`
  (drop `--headless` to inspect); `shouldBeAbleToGetProductList` goes red→green at
  `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR` instead of `FIX_LOOP_EXHAUSTED`.
- `shouldBeAbleToGetProduct` and the per-SKU stories still pass.
- Re-run `bash scripts/atdd-rehearsal-loop.sh --config gh-optivem-monolith-typescript.yaml`
  so #65 no longer stops the corpus.

---

## Notes / open questions (resolve during discussion)

- **Confirm scope before R4.** Decide whether prompt guidance (R1–R3) is enough
  or a structural reconciliation gate is warranted.
- **Sibling-endpoint sweep.** Before finalizing, run `runtime-prompts-audit` +
  `bpmn-logic-audit` to confirm tax/clock list-style endpoints don't carry the
  same latent D1 mismatch. Cross-ref [[project_bpmn_full_coverage_story_and_realkind_gap]].
- **Other languages/architectures.** This run was the TypeScript monolith; the
  same agent-prompt gaps apply to dotnet/java and multitier — the prompt fixes
  are language-agnostic, so one fix covers all, but re-verify per config.
- **D1 vs D2 ordering.** D1 produces the visible `[]`; D2 is masked behind it.
  After fixing D1, expect the count assertion to surface — item 3 must land in
  the same change or the next rehearsal stops on `hasCount`.
```
