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

1. For every Acceptance Criterion, write a corresponding Acceptance Test (`${at-test}`). This should be a mechanical 1:1 translation — translate every AC item, and do not classify items as acceptance-vs-contract or reason about what other cycles (contract tests, adapters) do; that is out of your lane. Add the permanent WIP gate to every Acceptance Test method you add, *alongside* its existing channel-parameterization annotations/wrapper — the gate is additive, not a replacement for the test's declaration. Follow the language-specific shape exactly:

   ${gate-marker-example}

   The gate stays in the committed code for the test's whole lifetime — it is never removed. It keys on the `GH_OPTIVEM_RUN_WIP_TESTS` environment variable, which the ATDD orchestrator sets to `1` only when it runs verify steps, lifting the gate for that invocation. Feature-branch CI, local `mvn test` / `dotnet test` / `npx playwright test`, and IDE runs leave it unset, so the test is silently skipped there.

   Separately, mirror the scenario's isolation tag: when the source Acceptance Criterion's scenario carries the Gherkin `@isolated` tag, emit its Acceptance Test with the language isolation shape below; an untagged scenario stays plain. This is mechanical mirroring of the tag the refiner/author already set — *not* a judgement about whether the test should be isolated. Follow the language-specific shape exactly:

   ${isolated-marker-example}

   To discover the surface you translate against: model each new test on the **existing sibling test** in `${at-test}` (same feature/area) — read it and copy its shape, annotations, and DSL chain. Reach the DSL builder surface in `${dsl-port}` via **targeted greps for the methods the Acceptance Criteria name** (the product/order/assertion builders the scenario implies), so you land on the right interfaces directly rather than reading the whole port tree. This is guidance, not a read cap: **read whatever you genuinely need to get the translation right** — when the correct builder/assertion or the chain shape is unclear, read the interface. A faithful 1:1 test matters more than a short read list.
2. If you added methods to the DSL Port (`${dsl-port}`), append a stub method body that throws a runtime exception with message `"TODO: DSL"` (using the language-appropriate exception type) to the impl class in DSL Core (`${dsl-core}`) for each newly-added Port method, so compilation works. If a prior run's edits didn't compile (forgotten stub, typo, signature mismatch in `${dsl-port}` or `${dsl-core}`), fix the named issue minimally — do not change test intent. Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to "understand the structure".

## Outputs

${expected-outputs}

Notes:

- `test-names` is every unqualified test method name added or modified by this ticket across re-runs — not pre-existing tests the ticket did not touch.
- For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
- Step 2's `TODO: DSL` stub applies to new *methods* on `${dsl-port}` only — DTO/enum changes don't require a stub.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.
