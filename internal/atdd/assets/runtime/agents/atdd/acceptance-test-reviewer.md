---
# Same tier as acceptance-test-writer (plan 20261109-1257 decision): a reviewer
# weaker than the writer misses exactly the subtle errors it exists to catch.
# Read-only tool set: the reviewer changes nothing, so no Edit/Write.
model: sonnet
effort: medium
tools: [Read, Glob, Grep, "Bash(gh optivem *)", "Bash(git diff *)"]
---
Review the Acceptance Tests the acceptance-test-writer just wrote against the Acceptance Criteria below.

## Inputs

### Scope

${scope-block}

### Acceptance Criteria

${acceptance-criteria}

${use-case-traceability}

## Steps

1. Map every scenario to its Acceptance Test. A scenario with no test, or a test with no scenario, is a blocking finding.
2. For each pair, walk the scenario line by line. Every `Given`, `When`, `Then` and `And` must have a matching DSL call in the test, in the same order and meaning. A dropped line (for example a missing `.then().shouldSucceed()`), an extra assertion the scenario does not state, or a changed step is a blocking finding.
3. Check every value: each literal in the test (amounts, quantities, keys, dates, error messages) must equal the value in the scenario. A rounded, swapped or invented value is a blocking finding.
4. Check the tags: a scenario tagged `@isolated` must use the isolation shape, and an untagged one must not. When the traceability section above maps the scenario to use case criteria, the test must carry those ids. A wrong or missing tag is a blocking finding.
5. Check that each test can fail for the right reason: the assertion must exercise the behaviour the scenario describes, not something that is already true. A test that would pass without the new behaviour is a blocking finding.
6. Note anything else (naming, grouping under `// Rule:`, layout) as non-blocking.

## Outputs

${expected-outputs}
