---
# Designs tests and wires DSL stub prototypes — both fit in Sonnet.
model: sonnet
effort: medium
---
The Acceptance Criteria below were parsed from the ticket body during intake — write tests for them directly.

## Inputs

### Scope

${scope_block}

### Acceptance Criteria

${acceptance_criteria}

## Steps

1. For every Acceptance Criterion, write a corresponding Acceptance Test (`${at-test}`). This should be a mechanical 1:1 translation.
2. If you need to add methods to the DSL Port (`${dsl-port}`), then implement the DSL Core (`${dsl-core}`) by implementing method prototypes by throwing a runtime exception  `"TODO: DSL"`, so that compilation works.

## Outputs

At the end of your final response, emit a fenced YAML block with the
acceptance-test methods this ticket exercises and a flag telling the
dispatcher whether the DSL Port (`${dsl-port}`) changed:

```
outputs:
  test_names:
    - shouldRegisterCustomer
    - shouldRejectDuplicateCustomer
  dsl-port-changed: false
```

`test_names` is every unqualified test method name added or modified by
this ticket (across re-runs) — not every test in the file. If a re-run
adds another test for the same ticket, include both; do not include
pre-existing tests the ticket did not touch.

`dsl-port-changed` is `true` if you added or modified any method on the
DSL Port (`${dsl-port}`) — i.e. you also wrote a `"TODO: DSL"` stub in
the DSL Core per Step 2 — and `false` otherwise. The dispatcher routes
into the DSL implementation phase iff this flag is `true`, so an
omitted or incorrect value will mis-route the cycle. Both downstream
tasks consume these values and have no other way to learn them.

The block may follow other prose. The parser keeps the last `outputs:`
block in the response.

## Additional Notes

- If your previous run didn't compile, instead fix the broken/missing piece in your prior edits (forgotten DSL stub, typo, signature mismatch) and fix it minimally. Do not change test intent.
