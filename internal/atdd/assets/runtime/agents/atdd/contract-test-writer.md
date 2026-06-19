---
# Mirror of write-acceptance-tests on the contract-test side: contract-test design + DSL scaffolding fit Sonnet.
model: sonnet
effort: medium
---
Write external-system contract tests against the existing DSL surface (`${dsl-port}`) for the external system driver port(s) being changed.

## Inputs

### Scope

${scope-block}

- No per-invocation parameters; the contract-test (`${ct-test}`) target is the existing DSL surface (`${dsl-port}`) visible in scope.

## Steps

1. Write External System Contract Tests (`${ct-test}`) against the existing DSL surface (`${dsl-port}`). If new DSL methods (`${dsl-port}`) are needed, call them in the test as if they exist.

   **A contract test exercises ONLY the external-system driver — never the system-under-test.** Its shape is `given → then` against the external system: set up the external system's state, then assert the external system returns it (mirroring the sibling). **Never use a `.when().<systemAction>()` chain, or any DSL builder that routes through the system-under-test driver (the application under test).** That `when()` shape belongs to *acceptance* tests, not contract tests; in a contract test it reaches an unimplemented `TODO: System Driver` stub that no contract-phase agent is scoped to implement, so the test can never go green. If the contract needs a capability the external surface lacks (e.g. listing rather than fetching one entity), extend the external-system DSL surface with the new read and assert on it — mirror the sibling's existing external read; do **not** reach for a system-under-test verb to get the data.

   To discover the surface you translate against: model each new test on the **existing sibling contract test** in `${ct-test}` (same feature/area) — read it and copy its shape, annotations, and DSL chain. Reach the DSL builder surface in `${dsl-port}` via **targeted greps for the methods the contract exercises**, so you land on the right interfaces directly rather than reading the whole port tree. This is guidance, not a read cap: **read whatever you genuinely need to get the translation right** — when the correct builder/assertion or the chain shape is unclear, read the interface. A faithful contract test matters more than a short read list.
2. If you added methods to the DSL Port (`${dsl-port}`), append a stub method body that throws a runtime exception with message `"TODO: DSL"` (using the language-appropriate exception type) to the impl class in DSL Core (`${dsl-core}`) for each newly-added Port method, so compilation works. If a prior run's edits didn't compile (forgotten stub, typo, signature mismatch in `${dsl-port}` or `${dsl-core}`), fix the named issue minimally — do not change test intent. Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to "understand the structure".

## Outputs

${expected-outputs}

Notes:

- `test-names` is every unqualified test method name added or modified by this ticket across re-runs — not pre-existing tests the ticket did not touch.
- For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
- Step 2's `TODO: DSL` stub applies to new *methods* on `${dsl-port}` only — DTO/enum changes don't require a stub.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.
