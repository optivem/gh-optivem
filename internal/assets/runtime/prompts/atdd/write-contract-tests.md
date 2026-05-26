---
# Mirror of write-acceptance-tests on the contract-test side: contract-test design + DSL scaffolding fit Sonnet.
model: sonnet
effort: medium
---
Write external-system contract tests against the existing DSL surface (`${dsl-port}`) for the external system driver port(s) being changed.

## Inputs

### Scope

${scope_block}

- No per-invocation parameters; the contract-test (`${ct-test}`) target is the existing DSL surface (`${dsl-port}`) visible in scope.

## Steps

1. Write External System Contract Tests (`${ct-test}`) against the existing DSL surface (`${dsl-port}`). If new DSL methods (`${dsl-port}`) are needed, call them in the test as if they exist.
2. If you need to add methods to the DSL interface (`${dsl-port}`), then implement the DSL Core (`${dsl-core}`) by implementing method prototypes by throwing a runtime exception `"TODO: DSL"`, so that compilation works.

## Additional Notes

- If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten DSL stub, typo, signature mismatch) and fix it minimally. Do not change test intent.
