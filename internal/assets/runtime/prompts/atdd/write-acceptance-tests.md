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
2. If you added methods to the DSL Port (`${dsl-port}`), append a stub method body that throws a runtime exception with message `"TODO: DSL"` (using the language-appropriate exception type) to the impl class in (`${dsl-core}`) for each newly-added Port method, so compilation works. If a prior run's edits didn't compile (forgotten stub, typo, signature mismatch in `${dsl-port}` or `${dsl-core}`), fix the named issue minimally — do not change test intent. Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to "understand the structure". The asymmetric scope (dsl-core is writeable but not in `read:`) is deliberate: reading impl context would leak it into test design.

## Outputs

At the end of your final response, emit a **fenced** YAML block with the
acceptance-test methods this ticket exercises and a flag telling the
dispatcher whether the DSL Port (`${dsl-port}`) changed. The block
MUST be wrapped in triple-backtick fences exactly as shown below —
un-fenced YAML is invisible to the parser and the cycle will halt with
a missing-output failure:

````
```
outputs:
  test_names:
    - shouldRegisterCustomer
    - shouldRejectDuplicateCustomer
  dsl-port-changed: false
```
````

(The outer four-backtick fence is only there so the example renders
correctly in this prompt — your emitted block uses three backticks
opening and closing.)

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

The block may follow other prose. The parser keeps the last fenced
`outputs:` block in the response.
