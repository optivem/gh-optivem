---
# Designs tests and wires DSL stub prototypes — both fit in Sonnet.
model: sonnet
effort: medium
---
The Acceptance Criteria below were parsed from the ticket body during intake — write tests for them directly.

## Inputs

### Acceptance Criteria

${acceptance_criteria}

## Steps

1. For every Acceptance Criterion, write a corresponding Acceptance Test. This should be a mechanical 1:1 translation.
2. If you need to add methods to DSL interface, then implement the DSL Core by implementing method prototypes by throwing a runtime exception  `"TODO: DSL"`, so that compilation works.
3. Set flag: `DSL Interface Changed: yes|no`

## Outputs

At the end of your final response, emit a fenced YAML block listing the
acceptance-test methods this ticket exercises:

```
outputs:
  test_names:
    - shouldRegisterCustomer
    - shouldRejectDuplicateCustomer
```

`test_names` is every unqualified test method name the ticket iterates on
— not only the one most-recently added. Re-emit the full set on every
re-run; downstream tasks consume this list and have no other way to
learn it.

The block may follow other prose. The parser keeps the last `outputs:`
block in the response.

## Additional Notes

- If your previous run didn't compile, instead fix the broken/missing piece in your prior edits (forgotten DSL stub, typo, signature mismatch) and fix it minimally. Do not change test intent.
