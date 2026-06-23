---
# Designs tests and wires DSL stub prototypes — both fit in Sonnet. Medium effort: the keystone artifact, plus grep-discovery of the DSL surface and directory-keyed *-port-changed flags need more than low.
model: sonnet
effort: medium
---
The Acceptance Criteria below were parsed from the ticket body during intake — write tests for them directly.

## Inputs

### Scope

${scope-block}

### Acceptance Criteria

${acceptance-criteria}

## Steps

1. For every Acceptance Criterion, write a corresponding Acceptance Test (`${at-test}`). This should be a mechanical 1:1 translation — translate every AC item, and do not classify items as acceptance-vs-contract or reason about what other cycles (contract tests, adapters) do; that is out of your lane. (One carve-out to "do not touch what the ticket didn't author": when the ticket *changes a rule that existing acceptance tests already encode*, reconciling those pre-existing tests **is** in your lane — see **Behavior-change tickets** below. The 1:1 rule still governs how you author the new test; it does not license leaving the suite self-contradictory.) Add the permanent WIP gate to every Acceptance Test method you add, *alongside* its existing channel-parameterization annotations/wrapper — the gate is additive, not a replacement for the test's declaration. Follow the language-specific shape exactly:

   ${gate-marker-example}

   The gate stays in the committed code for the test's whole lifetime — it is never removed. It keys on the `GH_OPTIVEM_RUN_WIP_TESTS` environment variable, which the ATDD orchestrator sets to `1` only when it runs verify steps, lifting the gate for that invocation. Feature-branch CI, a local test run, and IDE runs leave it unset, so the test is silently skipped there.

   Separately, mirror the scenario's isolation tag: when the source Acceptance Criterion's scenario carries the bare Gherkin `@isolated` tag, emit its Acceptance Test with the language isolation shape below; an untagged scenario stays plain. This is mechanical mirroring of the tag the refiner/author already set — *not* a judgement about whether the test should be isolated. The Gherkin tag is bare (`@isolated`, no free text); the isolation **reason** lives separately as an adjacent comment / scenario-description line (e.g. a `# isolated: …` line above `Scenario:`). When such a reason line is present, lift it **verbatim** into the code annotation's reason argument; when absent, emit the bare-reason form. **Never invent a reason** — if no reason line accompanies the tag, do not fabricate one. Follow the language-specific shape exactly:

   ${isolated-marker-example}

   To discover the surface you translate against: model each new test on the **existing sibling test** in `${at-test}` (same feature/area) — read it and copy its shape, annotations, and DSL chain. Reach the DSL builder surface in `${dsl-port}` via **targeted greps for the methods the Acceptance Criteria name** (the product/order/assertion builders the scenario implies), so you land on the right interfaces directly rather than reading the whole port tree. This is guidance, not a read cap: **read whatever you genuinely need to get the translation right** — when the correct builder/assertion or the chain shape is unclear, read the interface. A faithful 1:1 test matters more than a short read list.

   **Keep GIVEN preconditions self-consistent.** When a GIVEN customizes an attribute that cross-references another seeded entity — for example a line item referencing a product by its key, an order naming a customer, a reservation naming a room, a discount naming a coupon code, or any field naming a country/currency — you must seed that referenced entity with the **same** value in the same GIVEN. Auto-seeded defaults use the default key, so once you override a cross-referencing attribute the default no longer matches and the precondition fails *before* the behavior under test runs (a false RED that no production code can turn green). Mirror the seed order shown in the sibling/reference test, and read its GIVEN setup to see which entities it seeds together. (Products/SKUs are only one illustration — the rule holds for any entity-to-entity reference.)

   **Behavior-change tickets — reconcile the pre-existing tests of the rule you are changing.** When the ticket *changes an existing business rule* (a bug fix or behavior change that moves or redefines a boundary, threshold, window, or rule already covered by acceptance tests), adding a new test for the new rule is **not enough**: any pre-existing test that still encodes the **old** rule will contradict your new one, and because the system-implementers can only edit production code (never tests), no single code value can satisfy both — the run stalls, trapped between an old test and a new test. You must update those obsolete tests too, so the whole suite has **one** consistent definition of the rule.
   - **Discover** them by **grepping the rule's distinguishing literals across `${at-test}`'s tree (`tests/**/acceptance` and equivalents)** — the boundary values/times the rule turns on *and* the error-message string it uses. This finds the contradicting rows wherever they live, including `@isolated` parameterized/boundary tests in the same feature/area, which the new-test sibling lookup alone will miss.
   - **Trigger heuristic:** if that grep finds a pre-existing acceptance test already asserting a value for this rule, treat the ticket as a behavior change and reconcile it — no separate ticket-type signal is required.
   - **Reconcile** by moving each affected data row / assertion to the new rule so old and new agree. *Worked example (#76 — blackout window end moved 22:30 → 23:00):* the positive boundary case at `22:30:01` flips from "outside / cancellation allowed" to "inside / rejected"; add in-window rows like `22:45` and `22:59:59` to the negative (rejected) set; the first time that is fully **outside** the window becomes `≥23:00:01`. Update test names that bake in the old boundary (e.g. `…Between2200And2230…`) to match the new one.
   - These reconciled pre-existing tests are now **touched by this ticket** — report their method names in `test-names` (see Outputs Notes).
2. If you added methods to the DSL Port (`${dsl-port}`), append a stub method body that throws a runtime exception with message `"TODO: DSL"` (using the language-appropriate exception type) to the impl class in DSL Core (`${dsl-core}`) for each newly-added Port method, so compilation works. **Mirror the port method's exact signature on the stub** — copy the same parameter list *and* the same declared return type from the corresponding `${dsl-port}` method; only the body throws. The declared return type must be the real port return type, **never** a placeholder, because the test's fluent chain (e.g. `.viewProductList().then()…`) is type-checked against it: a placeholder return type compiles as a method body but breaks every chained caller. Concretely, do **not** declare the stub as returning `never` (TS) or any other placeholder type — a `never` return triggers `TS2339: Property 'X' does not exist on type 'never'` at the first chained call and fails `compile`. The same rule holds for Java and .NET: the throwing stub keeps the port method's real declared return type so callers still compile. Worked example (TypeScript):

   ```ts
   // port (${dsl-port}) declares the real return type:
   viewProductList(): WhenViewProductList;

   // CORRECT stub — keeps the port return type; only the body throws, so .then() still type-checks:
   viewProductList(): WhenViewProductList { throw new Error('TODO: DSL'); }

   // WRONG stub — `never` has no members, so `.viewProductList().then()` fails TS2339 and the suite won't compile:
   viewProductList(): never { throw new Error('TODO: DSL'); }
   ```

   If a prior run's edits didn't compile (forgotten stub, typo, signature mismatch in `${dsl-port}` or `${dsl-core}`), fix the named issue minimally — do not change test intent. Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to "understand the structure".

## Outputs

${expected-outputs}

Notes:

- `test-names` is every unqualified test method name added or modified by this ticket across re-runs — not pre-existing tests the ticket did not touch. A pre-existing test you reconcile under the **Behavior-change tickets** rule *is* touched by this ticket, so include its (possibly renamed) method name here.
- For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
- Step 2's `TODO: DSL` stub applies to new *methods* on `${dsl-port}` only — DTO/enum changes don't require a stub.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.
