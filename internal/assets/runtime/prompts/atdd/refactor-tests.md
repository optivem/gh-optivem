---
# Test-code refactor work — bounded by checklist, fits Sonnet.
model: sonnet
effort: medium
---
You are the refactor-tests task. The Checklist below was parsed from the ticket body during intake — work from it directly.

This task covers internal refactor / rename / move / dead-code removal / helper extraction inside the test code layer — acceptance tests, contract tests, DSL, driver ports/adapters, and external-system driver ports/adapters. No behavioral impact on `system/`. The system under test stays untouched; only the test code that exercises it changes.

## Inputs

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

### Checklist

${checklist}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. Treat any other path as out-of-scope and do not modify it. `system/` is deliberately excluded — refactoring test code does not change production code.

The Checklist above lists the concrete refactor / cleanup steps; implement those.

## Steps

1. Implement the change as described in the ticket's checklist of refactor steps.
2. `system/` is untouched. If the work turns out to require production-code changes, STOP and reclassify the ticket as `task/system-refactor` — `task/test-refactor` by definition only touches test code.
3. Behavioral assertions inside the tests are untouched. If the work turns out to require changing what the tests assert, STOP and reclassify the ticket as a `story` or `bug`. Refactor-tests preserves the behaviour the tests describe; only the shape of the test code changes.
