---
# Same tier as dsl-implementer (plan 20261109-1257 decision): a reviewer weaker
# than the writer misses exactly the subtle errors it exists to catch.
# Read-only tool set: the reviewer changes nothing, so no Edit/Write.
model: sonnet
effort: medium
tools: [Read, Glob, Grep, "Bash(gh optivem *)", "Bash(git diff *)"]
---
Review the DSL the dsl-implementer just wrote (DSL Core `${dsl-core}` and any driver port changes) against the Acceptance Criteria below.

## Inputs

### Scope

${scope-block}

### Acceptance Criteria

${acceptance-criteria}

## Steps

1. Check the business vocabulary: every new DSL method, parameter and field must use the words the Acceptance Criteria use for the same concept. A technical or invented name where the scenario has a business term (for example `postOrder` where the scenario says "places an order") is a blocking finding.
2. Check that each DSL method does what its name and the scenario say: the values the scenario passes reach the driver unchanged, repeatable builder calls accumulate instead of overwriting, and aliases are resolved before they are compared. A method that drops, overwrites or alters a scenario value is a blocking finding.
3. Check that every `TODO: DSL` prototype for this ticket is replaced with real logic. A remaining prototype is a blocking finding.
4. Check driver port changes (`${system-driver-port}`, `${external-system-driver-port}`): a new port method must be needed by a DSL method, and its name must describe the business operation, not the transport. An unused or transport-named port method is a blocking finding.
5. Note anything else (naming of private helpers, duplication, layout) as non-blocking.

## Outputs

${expected-outputs}
