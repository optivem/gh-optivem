---
# Mirror of write-acceptance-tests on the contract-test side: contract-test design + DSL scaffolding fit Sonnet.
model: sonnet
effort: medium
---
This is the `write-contract-tests` task. It is called from the `implement-and-verify-external-system-driver-adapters-contract-tests` HIGH orchestration when a `change-system-behavior` CYCLE detects that external system driver ports changed.

## Steps

1. Write External System Contract Tests against the existing DSL surface. If new DSL methods are needed, call them in the test as if they exist.
2. If you need to add methods to the DSL interface, then implement the DSL Core by implementing method prototypes by throwing a runtime exception `"TODO: DSL"`, so that compilation works.

If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten DSL stub, typo, signature mismatch) and fix it minimally. Do not change test intent.
