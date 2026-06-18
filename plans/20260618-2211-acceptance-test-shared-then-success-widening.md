# Plan: stop `acceptance-test-writer` from widening the shared `ThenSuccess` port and breaking core↔port assignability behind #68 `compile-tests` failure ({20260618-2211})

⏳ **PENDING DISCUSSION — not agreed, not picked up.** This file records the
2026-06-18 #68 (`apply-automatic-quantity-discount-on-order`) rehearsal
investigation and a proposed orchestrator-side fix. Do **not** start `## Items`
until the operator confirms scope and picks an option (see "Decision needed").
All fixes here are in **`gh-optivem`** runtime agent prompts, not in `shop`.

Sibling: [[plans/20260618-1937-contract-real-adapter-simulator-shape-gap.md]]
recorded the *same evening's* #65 contract-real wall (a `FIX_LOOP_EXHAUSTED`
runtime exhaustion). This is a different failure mode: a **compile-time
TypeScript error** in the acceptance cycle that halts before any test runs, and
the auto-fix was rejected. Unrelated root cause; same "fix the authoring agent's
prompt, not `shop`" shape.

---

## The error, and how we found it

**Symptom (from the rehearsal-loop run, worktree
`rehearsal-20260618-193812-68-apply-automatic-quantity-discount-on`, run
`20260618-173824`):** `gh optivem test compile` (`tsc`) exits 1 in
`WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE → COMPILE_TESTS`. The fix loop asked for
approval, was rejected (non-interactive `n` → `FIX_REJECTED_END`), and the run
halted. This is a **compile failure, not a test failure** — the acceptance-test-
writer produced code that does not type-check.

The compiler emits a cascade of `core/.../given-stage.ts` errors, all reducing to
one line:

```
core/scenario/given/given-stage.ts: Property 'when' (and 'order','and','country','clock',…)
  in type 'GivenStage' is not assignable to the same property in base type 'GivenStage' (port).
  …'when().cancelOrder().then().shouldSucceed()' return types incompatible:
    Property 'line' is missing in type 'ThenCancelOrderSuccess' but required in type 'ThenSuccess'.
```

**How we traced it (the read trail), in the worktree's `system-test/typescript`:**

1. **What the agent changed** (`git diff` / untracked):
   - `src/testkit/dsl/port/then/steps/then-success.ts` — added `line(sku: string): ThenOrderLine;`
     to the **shared** `ThenSuccess` port interface.
   - `src/testkit/dsl/core/scenario/then/then-place-order.ts` — added a matching
     `line()` + a new `ThenOrderLine` class (bodies are `throw new Error('TODO: DSL')` stubs).
   - `src/testkit/dsl/port/when/steps/when-place-order.ts` + core — added `withLine(sku, qty)`.
   - new untracked: `then-order-line.ts`, the two `*-quantity-discount-*-test.spec.ts`.
2. **What `ThenSuccess` actually is** (`port/then/then-result-stage.ts`):
   `ThenResultStage.shouldSucceed(): ThenSuccess` — the **generic success type
   returned by every use-case's `then().shouldSucceed()`** (place-order, view-order,
   browse-coupons, publish-coupon all route through the generic `ThenResultStage`).
3. **Why `ThenCancelOrderSuccess` collides** (`core/scenario/then/then-cancel-order.ts:159`):
   `cancelOrder` already has its **own** use-case-specific success class
   `ThenCancelOrderSuccess`, while the **port** still models cancel-order's
   `then()` as the generic `ThenResultStage` → `ThenSuccess`
   (`port/when/steps/when-cancel-order.ts:5`). That override was valid only while
   `ThenSuccess` stayed minimal (`and()` + `PromiseLike`), which
   `ThenCancelOrderSuccess` satisfied. The moment `line()` became **required** on
   `ThenSuccess`, `ThenCancelOrderSuccess` (which has no `line()`, and correctly
   should not) stopped satisfying it → the core `GivenStage`→port `GivenStage`
   structural assignability collapses, echoed across every fluent entry point.

**The single defect:** a **place-order-specific** assertion
(`.line(sku).hasDiscountRate(…).hasSubtotal(…)`, as the new positive spec uses it)
was bolted onto a **type shared with cancel-order and the other use-cases**. The
`TODO: DSL` stubs are expected for the red phase and are *not* the problem — the
**type-level widening** is what fails compilation.

---

## Root cause (orchestrator-side)

| # | Where | Gap |
|---|---|---|
| R1 | `acceptance-test-writer.md:30-31` | Step 1 tells the agent to grep the port surface and "add methods to the DSL Port", and step 2 says to stub each new port method in core — but **nothing tells it *where* on the port a new assertion belongs.** There is no rule that a **use-case-specific** assertion must attach to a **use-case-specific** Then type, never to the **shared generic** `ThenSuccess` that sibling success types (`ThenCancelOrderSuccess`, etc.) must stay structurally assignable to. So the agent took the shortest path — widen the shared interface — and broke core↔port assignability. |
| R2 | `acceptance-test-writer.md:31` | The deliberate read-minimisation ("do not read existing method bodies or browse other dsl-core files to understand the structure") is what *hides* the collision: the agent never sees that `cancelOrder` has a bespoke `ThenCancelOrderSuccess` that the shared interface change will break. The compile step is the only backstop, and the fix loop is gated behind human approval. |
| R3 (observation) | `dsl-implementer.md` | The acceptance-test-writer only writes throwing stubs; the real DSL bodies are the `dsl-implementer`'s job. If the structural-placement rule lands only in `acceptance-test-writer`, confirm `dsl-implementer` won't re-introduce the same shared-interface widening when it fleshes the stubs. Likely a one-line cross-reference, not a separate fix. |

This is a reusable invariant about the DSL's core↔port split, worth a memory:
**use-case-specific Then assertions attach to use-case-specific Then types; the
shared generic `ThenSuccess`/`ThenFailure` port interfaces stay minimal**, because
every sibling success type must remain assignable to them.

---

## Decision needed (pick during discussion, before execution)

**Option A — fix R1 (+ R3 cross-ref) in the agent prompts (recommended).** Add a
short placement rule to `acceptance-test-writer` step 1: when a new assertion is
specific to one use-case (it reads/asserts that use-case's result shape), put it
on that use-case's Then type (mirror the existing `cancelOrder` →
`ThenCancelOrderResultStage`/`ThenCancelOrderSuccess` pattern), and model
place-order's `then()` as a place-order-specific result stage — **never widen the
shared `ThenSuccess`/`ThenFailure` port interface.** Cross-reference the same rule
from `dsl-implementer`. Orchestrator-only, language-agnostic, survives worktree
regeneration.

**Option B — A, plus a targeted read carve-out (relax R2).** On top of A, let the
acceptance-test-writer read the sibling use-case-specific Then types before
adding an assertion, so it *sees* the bespoke success classes rather than relying
on the rule alone. Costs a few reads against the standing read-minimisation
guidance — weigh against [[feedback_acceptance_tests_accuracy_over_speed]]
(accuracy outranks read count) vs the explicit "don't browse dsl-core" line.

**Option C — A, plus a structural guard.** Add a lightweight check (lint rule or
a note in the compile-fix loop) that flags new members on the shared
`ThenSuccess`/`ThenFailure` interfaces. Heaviest; only if discussion concludes
prompt guidance is insufficient.

Recommendation: **A** (with the R3 cross-reference). B's carve-out is the
fallback if A alone proves too easy to ignore; C deferred unless discussion asks.

---

## Items (Option A) — proposed, pending confirmation

> All edits are in **`gh-optivem`** runtime agent prompts. Do not start until the
> operator confirms the option above.

### 1. [agent · acceptance-test] Add the use-case-specific-assertion placement rule

**Where:** `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md`
(step 1, ~line 30; and the step-2 stub note, ~line 31).

**Change (resolves R1):** add guidance that when a newly-named assertion is
specific to one use-case's result (asserting that use-case's own response shape —
e.g. per-order-line discount on place-order), it attaches to that use-case's Then
type, mirroring the existing use-case-specific pattern (`cancelOrder` →
`ThenCancelOrderResultStage` → `ThenCancelOrderSuccess`). Call out explicitly:
**do not add members to the shared generic `ThenSuccess`/`ThenFailure` port
interfaces** — sibling use-case success/failure types must stay assignable to
them, and widening the shared interface breaks core↔port compilation. Keep the
language phrased as scope/shape, not layer-coding
(cf. [[feedback_no_layer_coding_in_names]]).

### 2. [agent · dsl-implementer] Cross-reference the same invariant

**Where:** `internal/atdd/assets/runtime/agents/atdd/dsl-implementer.md`.

**Change (resolves R3):** add a one-line guard so that when the implementer
fleshes the `TODO: DSL` stubs it does not migrate or re-home a use-case-specific
assertion onto the shared `ThenSuccess`/`ThenFailure` interface. Confirm during
discussion whether this is needed or already implied by item 1.

---

## Verification

(Operator-driven — not agent `## Items` work.)

- Re-run `bash scripts/atdd-rehearsal.sh 68 --config gh-optivem-monolith-typescript.yaml`
  (drop `--headless` to inspect); `gh optivem test compile` passes at
  `COMPILE_TESTS`, and the two `place-order-quantity-discount-*` specs reach the
  expected red (missing behaviour / `TODO: DSL`), not a compile error.
- The unchanged `cancelOrder` / `viewOrder` / coupon stories still compile.
- Re-run `bash scripts/atdd-rehearsal-loop.sh --config gh-optivem-monolith-typescript.yaml`
  so #68 no longer stops the corpus.

---

## Notes / open questions (resolve during discussion)

- **Confirm B vs A.** Decide whether the placement rule alone is enough or the
  acceptance-test-writer also needs a read carve-out to *see* the bespoke sibling
  Then types before adding an assertion.
- **Other languages/architectures.** This run was the TypeScript monolith; the
  core↔port DSL split exists in dotnet/java and multitier too, so the prompt fix
  is language-agnostic — but re-verify per config, since `tsc`'s structural
  typing surfaces this more eagerly than nominal-typed languages may.
- **Sibling-assertion sweep.** Run `runtime-prompts-audit` to confirm no other
  acceptance/contract assertion guidance steers agents toward the shared
  interfaces; cross-ref [[project_bpmn_full_coverage_story_and_realkind_gap]].
- **Worktree disposition.** The failing worktree
  `rehearsal-20260618-193812-68-apply-automatic-quantity-discount-on` was kept
  (branch `rehearsal/...`); discard after the prompt fix lands and #68 re-runs green-to-red.
