# Add a canonical "no order is persisted" absence-assertion example to shop

## TL;DR

**Why:** Any acceptance criterion of the shape *"And no order is persisted"* deterministically crashes the ATDD run at `FIX_LOOP_EXHAUSTED`. shop ships **zero** worked examples of an absence / non-persistence assertion, so the `dsl-implementer` invents the primitive from scratch every time — and gets it wrong (only in Java, wired onto an eager-fetch that explodes on a failed-operation alias). The downstream `verify-tests-pass` fix loop can never recover it, because by then the only running agent (`system-implementer`) is scoped to **system code**, not the testkit DSL.
**End result:** shop ships a committed, always-on acceptance test demonstrating *"place-order is rejected **and** no order is persisted"*, backed by a correct absence-assertion DSL primitive mirrored across Java / .NET / TypeScript and passing on both monolith and multitier. The agent now **copies** a working reference instead of reinventing a broken one, and absence-shaped tickets go green.

## ▶ Next executable step (resume here)

Start at **Item 1 (Java)** — redesign the absence-assertion primitive and prove it with a first-class shop test, then mirror to .NET and TS (Items 2–3). No pickup marker yet; `git status` clean on `main` at authoring time.

## Motivation

Rehearsal #69 ("Reject order with line quantity of 100", acceptance line *"And no order is persisted"*) failed at `FIX_LOOP_EXHAUSTED` after the system code was **already correct**. The captured system response proves the rejection worked exactly as specified:

```
SystemError{message='The request contains one or more validation errors',
  fieldErrors=[FieldError{field='quantity', message='Quantity must be less than 100', code='Max'}]}
```

The red was raised entirely in the **testkit DSL**, by the final assertion the `acceptance-test-writer` used to encode "no order persisted":

```java
.then().shouldFail()
    .fieldErrorMessage("quantity", "Quantity must be less than 100")
.and().order().doesNotExist();   // PlaceOrderNegativeTest.java:174
```

Two stacked faults make this a **deterministic** blocker, not a flaky one:

### Fault 1 — no reference example, so the primitive is invented (badly) every run

`doesNotExist()` is a brand-new method the sonnet `dsl-implementer` invented for #69, **only in the Java testkit**. It exists in neither the .NET nor the TypeScript testkits, and no committed test in shop uses any absence pattern. With nothing to copy, the agent wired it onto the existing eager-fetch `ThenOrderImpl`:

- `.and().order()` → `BaseThenStep.order(executionResult.getOrderNumber())` resolves the `DEFAULT-ORDER` alias (non-null).
- The `ThenOrderImpl` constructor **eagerly fetches** the order: `app.myShop().viewOrder().orderNumber("DEFAULT-ORDER").execute().shouldSucceed()`.
- `ViewOrder.execute()` calls `UseCaseContext.getResultValue("DEFAULT-ORDER")`, but the failed place-order set that alias to `FAILED: …`, so it throws `IllegalStateException` **before `doesNotExist()` ever runs**.
- Even if it didn't throw, `doesNotExist()` asserts the *alias string* is null while `getOrderNumber()` returns the non-null alias — so the semantics are wrong too.

### Fault 2 — the fix loop structurally cannot repair a DSL defect

The crash surfaces in `verify-tests-pass` inside `implement-and-verify-system`, where the `system-implementer` (api channel) loops. Its scope is **system code only**; the `implement-dsl` phase has already committed by then. So a DSL defect introduced upstream is unreachable by the only agent still running — it retries system fixes that were never the problem, stays red, and exhausts after 2 attempts. *Every time.*

This plan fixes **Fault 1 at the root** (give shop a correct, mirrored, committed reference the agent copies). Fault 2 is a real but separate BPMN-scoping concern — see *Out of scope*.

## Current behaviour (verified against shop `main` + the #69 worktree)

- `system-test/{java,dotnet,typescript}` are three mirrored testkits; the DSL lives at `…/testkit/dsl/` (Java: `…/dsl/core/scenario/then/steps/` + `…/dsl/port/then/steps/`).
- The Java DSL exposes `ViewOrder` (lookup **by order number**) and `BrowseCoupons` — **but no browse/list-orders use case.** A validation-rejected order is never assigned a number, so there is no key to look it up by.
- `ThenOrderImpl` resolves its order **eagerly in the constructor** via a `viewOrder(...).shouldSucceed()` call — incompatible with any after-a-failure assertion.
- `UseCaseContext.getResultValue(alias)` throws `IllegalStateException` when the alias is in `FAILED:` state (`UseCaseContext.java:76`).
- TypeScript currently has only `…/dsl/port/then/steps` (the interface); the `core` impl + a `ThenOrder` step may need creating. .NET's `ThenOrder` equivalent was not found by name — the executor must locate or create it.

## Design decision — response-level absence assertion (resolved; recommendation reversed during authoring)

**Chosen: a response-level primitive.** `doesNotExist()` (or a clearer name — see below) asserts that the **failed operation produced no persisted order**, read directly from the failure outcome — *without* routing through the eager-fetch `.order()` path and *without* touching the `FAILED` alias.

This reverses the persistence-level recommendation floated in discussion. The reversal is deliberate: authoring discovery showed shop has **no list/browse-orders use case**, so a true persistence query would require a new use case + driver adapter mirrored across all three languages (and possibly a new system query endpoint) — disproportionate to what the rejection already guarantees.

Deciding reasons:

1. **Faithful to how the system actually rejects.** Validation short-circuits **before** any persistence (the #69 `@Max` rejection never reaches the service/DB). "No order persisted" is guaranteed by the rejection itself; a response-level check states exactly that invariant.
2. **Dodges the structural alias bug without new plumbing.** Reading the failure outcome directly never calls `getResultValue` on the `FAILED` alias, so the `IllegalStateException` simply cannot occur — the same safety the persistence path was wanted for, at a fraction of the cost.
3. **Uses data already on hand.** No new `BrowseOrders` use case, no new driver method, no system-side query — so it mirrors cleanly across Java/.NET/TS.
4. **Right altitude for a teaching reference.** The canonical example demonstrates the *pattern* ("a rejected command persists nothing") with the minimum machinery, which is what the agent should copy.

The stronger **persistence-level** assertion (query the order list, assert absent) is recorded under *Alternatives* as a future upgrade under the rule of three — pursue it only if a class of tickets needs to verify side-effect absence that validation does **not** structurally guarantee.

## Items

> Each item is agent-executable. Reuse an **existing** passing negative scenario for the example (e.g. `cannotPlaceOrderWithNonExistentCoupon`) so the example exercises *only* the new DSL primitive and needs **no new system behaviour**. Do not resurrect #69's quantity<100 constraint here.

1. **Java — redesign the absence-assertion primitive + add the canonical example.**
   - Make the after-failure absence assertion reachable without the eager `viewOrder(...).shouldSucceed()` fetch. Options for the executor: a dedicated terminal step off `ThenFailureImpl` (e.g. `.and().order().doesNotExist()` where `doesNotExist()` reads the failure result), or short-circuiting `ThenOrderImpl` when the prior operation failed. Whichever is chosen, `getResultValue` must **not** be called on a `FAILED` alias.
   - `doesNotExist()` must assert the **real** invariant — the failed operation yielded no persisted order — not that an alias string is null. Pick a name that reads correctly on the failure path; rename if `doesNotExist` is misleading.
   - Add a **first-class, always-on** acceptance test (not `@EnabledIfEnvironmentVariable`/WIP) extending an existing negative scenario with the absence tail. This committed test is the reference pattern.
   - Confirm green on monolith **and** multitier (`-p 2` or `scripts/test.sh`; never unbounded `go test`/Gradle-all per the Windows hazard).

2. **.NET — mirror the primitive + example.** Locate or create the `ThenOrder`/failure-path equivalent; implement the same response-level absence assertion and the same first-class example. Green on monolith + multitier.

3. **TypeScript — mirror the primitive + example.** TS currently ships only the `port/then/steps` interface; create the `core` impl + `ThenOrder` step as needed. Same response-level assertion + first-class example. Green on monolith + multitier.

## Verification

(User-driven — not agent steps.)

- Re-run the rehearsal corpus on an absence-shaped ticket (#69 "Reject order with line quantity of 100" is the canonical one) and confirm the run reaches green instead of `FIX_LOOP_EXHAUSTED` — i.e. the `dsl-implementer` now copies the shop reference rather than reinventing it.
- Spot-check that the new shop example is part of the default suite (not WIP-gated) and passes for all three languages on both architectures.
- Confirm no regression in the existing negative scenarios that share the extended test.

## Alternatives considered

### Persistence-level absence assertion (deferred — stronger but disproportionate)

`doesNotExist()` issues a fresh query (browse/list orders) and asserts the order is genuinely absent. Strongest fidelity — it would catch a hypothetical "persisted-but-returned-error" bug that a response-level check misses. Deferred because: (a) shop has no list/browse-orders use case, so it needs a new use case + driver adapter mirrored ×3 languages, plus possibly a system query endpoint; and (b) for validation-rejection criteria the order is never created by construction, so the extra fidelity buys little here. Revisit under the rule of three if a non-validation side-effect-absence class of tickets appears.

### Java-only fix now, mirror later (rejected)

Repair only Java to unblock #69 and file a follow-up for .NET/TS. Rejected: shop is the reference the agents pattern-match against, so a Java-only example leaves .NET/TS agents reinventing the broken primitive — the bug stays live for two-thirds of the matrix. The mirror invariant is the whole point of an example.

### Drop the `.and().order().doesNotExist()` tail from acceptance tests (rejected)

Let absence criteria be implied by the rejection and stop encoding them. Rejected: it silently drops a real acceptance criterion from the test, lowering fidelity — the opposite of the standing "accuracy over speed" gate. The criterion should be expressible and verified.

## Out of scope

- **Fault 2 — the fix loop's inability to repair DSL defects.** That the `verify-tests-pass` loop can only adjudicate/fix system code (never the testkit DSL committed in the upstream `implement-dsl` phase) is a genuine BPMN-scoping gap, but it is a separate concern with its own blast radius. File a dedicated plan if it recurs beyond absence assertions.
- A new browse/list-orders use case (only needed for the deferred persistence-level alternative).
- Any change to gh-optivem agent prompts — the fix is purely "give shop a correct reference"; the agents already pattern-match shop and need no prompt edit.
- Diagram/flow regeneration — none; this touches neither `process-flow.yaml` topology nor any rendered node.

## Estimated effort

~half a day. The Java redesign is small (one terminal step + a corrected assertion + one test). The bulk is faithful mirroring across .NET and TypeScript — including potentially standing up the TS `core` then-step — and confirming green on both architectures per language.
