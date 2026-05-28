---
# Designs tests and wires DSL stub prototypes — both fit in Sonnet.
model: sonnet
effort: low
---
The Acceptance Criteria below were parsed from the ticket body during intake — write tests for them directly.

## Inputs

### Scope

${scope-block}

### Acceptance Criteria

${acceptance-criteria}

## Steps

1. For every Acceptance Criterion, write a corresponding Acceptance Test (`${at-test}`). This should be a mechanical 1:1 translation.
2. If you added methods to the DSL Port (`${dsl-port}`), append a stub method body that throws a runtime exception with message `"TODO: DSL"` (using the language-appropriate exception type) to the impl class in (`${dsl-core}`) for each newly-added Port method, so compilation works. If a prior run's edits didn't compile (forgotten stub, typo, signature mismatch in `${dsl-port}` or `${dsl-core}`), fix the named issue minimally — do not change test intent. Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to "understand the structure".

## Outputs

${expected-outputs}

Notes:

- `test-names` is every unqualified test method name added or modified by this ticket across re-runs — not pre-existing tests the ticket did not touch.
- For `*-port-changed` flags, list every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle. For new methods you add to `${dsl-port}` you must also write a `"TODO: DSL"` stub in the DSL Core per Step 2; DTO/enum changes don't require a stub.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.
