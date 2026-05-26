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
2. If you added methods to the DSL Port (`${dsl-port}`), append a stub method body that throws a runtime exception with message `"TODO: DSL"` (using the language-appropriate exception type) to the impl class in (`${dsl-core}`) for each newly-added Port method, so compilation works. If a prior run's edits didn't compile (forgotten stub, typo, signature mismatch in `${dsl-port}` or `${dsl-core}`), fix the named issue minimally — do not change test intent. Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to "understand the structure". The asymmetric scope (dsl-core is writeable but not in `read:`) is deliberate: reading impl context would leak it into test design.

## Outputs

Emit each declared output by calling `gh optivem output write KEY=VAL`
from the `Bash` tool. The dispatcher reads the resulting per-invocation
JSONL file after you exit. Call once per dispatch (multiple `KEY=VAL`
args allowed in a single call); if you need to correct a value, call
again with the new value (last-write-wins).

${expected_outputs}

Key semantics:

- `test-names` is every unqualified test method name added or modified
  by this ticket (across re-runs) — not every test in the file. If a
  re-run adds another test for the same ticket, include both; do not
  include pre-existing tests the ticket did not touch.
- `dsl-port-changed` is `true` if you added or modified any method on
  the DSL Port (`${dsl-port}`) — i.e. you also wrote a `"TODO: DSL"`
  stub in the DSL Core per Step 2 — and `false` otherwise. The
  dispatcher routes into the DSL implementation phase iff this flag is
  `true`, so an omitted or incorrect value will mis-route the cycle.
- `scope-exception-files` / `scope-exception-reason` are the
  scope-exception envelope (see `${references_root}/scope.md`). Emit
  them only when you had to read or write outside this MID's scope.

Example call:

```
gh optivem output write \
  dsl-port-changed=false \
  test-names=shouldFetchCustomerProfile,shouldRejectMalformedRequest
```
