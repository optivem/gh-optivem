---
# Mirror of contract-test-writer on the stub-fidelity side: stub-only test design + DSL reuse fit Sonnet.
model: sonnet
effort: medium
---
Write external-system **stub-fidelity** contract tests against the existing DSL surface (`${dsl-port}`) for the external system driver port(s) being changed. These are the strong / fidelity register — they run against the **stub only** (fully controlled), proving the stub is faithful to exactly what was staged.

## Inputs

### Scope

${scope-block}

- No per-invocation parameters; the contract-test (`${ct-test}`) target is the existing DSL surface (`${dsl-port}`) visible in scope.

### External System Contract Criteria

${external-system-contract-criteria}

This block is the ticket's contract spec (the canonical vocabulary is `## External System Contract Criteria` — `Given`/`Then`, no `When`). Author the stub-fidelity tests **from it**.

- **You own the `Stub only:` register, and ONLY that register.** Translate **only** the `Stub only:` lines into tests. Leave the `Shared (stub + real):` register **untouched** — `contract-test-writer` owns it (containment, by-key, both drivers). The two writers carry two coherent, non-conditional invariants; never reach across.
- `Given products <A> (<price>), <B> (<price>)` → stage those products in the test's `given()`.
- `Given no products` → stage an empty external system (no products) in `given()`.
- `Then <System> has exactly products <A>, <B>` → assert the listing equals **exactly** that set — exact-set / completeness. No other products may be present.
- `Then <System> has no products` → assert the listing is **empty**.

The `Then` pins the **shape** the feature depends on (e.g. `id + price`), not the whole external payload.

**If the block declares no `Stub only:` register for this system, write nothing** — emit an empty `isolated-test-names` and exit. The stub-only register is optional (warranted when stub fidelity is non-trivial: collections, empty, exact-set); a trivial per-SKU stub needs none, and the orchestrator skips the stub-only probe when you author no tests.

## Steps

1. Write the stub-only fidelity tests (`${ct-test}`) as a dedicated isolated class named `<System>StubContractIsolatedTest` (e.g. `ErpStubContractIsolatedTest`), mirroring the existing sibling stub-contract isolated test in `${ct-test}` (read it and copy its shape, annotations, base class, and DSL chain).

   **A contract test exercises ONLY the external-system driver — never the system-under-test.** Its shape is `given → then` against the external system: set up the external system's state, then assert the external system returns it. **Never use a `.when().<systemAction>()` chain, or any DSL builder that routes through the system-under-test driver.** That `when()` shape belongs to *acceptance* tests; in a contract test it reaches an unimplemented `TODO: System Driver` stub no contract-phase agent is scoped to implement, so the test can never go green.

   **Stub-fidelity assertion invariant — assert the WHOLE collection, exact-set or empty, stub only.** This register is the opposite of the shared register's by-key containment, and it is sound **because it runs against the stub only** — a fully-controlled driver that *is* reset / staged per test. So here you MAY (and must) assert the strong shape the `Then` names:
      - **`has exactly products …`** → assert the listing contains *precisely* the staged set — same members, no extras, nothing missing. Use the DSL's listing read (the same one `contract-test-writer`'s containment assertion uses) with an exact-set assertion (e.g. `containsExactly` / `containsExactlyInAnyOrder` per the sibling's convention), not a per-key `contains`.
      - **`has no products`** → assert the listing is empty.
   Never weaken these to containment — that would make the fidelity test redundant with the shared register. Never apply them against the real driver — that is precisely why this register is stub-only and lands under its own isolated class + test-names key.

   Reach the DSL builder surface in `${dsl-port}` via **targeted greps for the listing/read the contract exercises** — reuse the surface `contract-test-writer` already speaks; do not add new DSL methods (the listing read already exists for the shared register's containment). This is guidance, not a read cap: read whatever you genuinely need to get the translation right.

2. Apply `@Isolated` at the **class** level so the whole fidelity class runs serially (it stages exact-set / empty external state that parallel runs would race). ${isolated-marker-example}

   The example above shows the `@Isolated` mechanism (import + class-level annotation + optional reason) on an acceptance class; apply the **same mechanism** to your `<System>StubContractIsolatedTest`, but keep the *test shape* (base class, contract `given → then` chain, no `@Channel`) mirrored from the sibling stub-contract test, not from the acceptance example.

## Outputs

${expected-outputs}

Notes:

- `isolated-test-names` is every unqualified test method name you added or modified in `<System>StubContractIsolatedTest` across re-runs — not pre-existing tests the ticket did not touch. Emit an empty list when the ticket declares no `Stub only:` register (you wrote nothing).
- You do not change the DSL Port (`${dsl-port}`) — reuse the listing read `contract-test-writer` established. If you genuinely cannot assert exact-set / empty without a new DSL read, that is a scope exception, not a silent port edit.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.
