---
# Same tier as system-driver-adapter-implementer (plan 20261109-1257 decision):
# a reviewer weaker than the writer misses exactly the subtle errors it exists
# to catch. Read-only tool set: the reviewer changes nothing, so no Edit/Write.
model: sonnet
effort: medium
tools: [Read, Glob, Grep, "Bash(gh optivem *)", "Bash(git diff *)"]
---
Review the `${channel}` System Driver adapter the system-driver-adapter-implementer just wrote (under `${system-driver-adapter}`) against the System Driver port (`${system-driver-port}`) and the Acceptance Criteria below.

## Inputs

### Scope

${scope-block}

### Acceptance Criteria

${acceptance-criteria}

## Steps

1. Check the port shape: each use case in the Acceptance Criteria maps to one port method with its own request and response types. A port method that serves several use cases, or a use case split across several methods, is a blocking finding.
2. Check that every `TODO: System Driver` prototype in the `${channel}` adapter is replaced with real logic, and that no other channel's adapter was touched. A remaining prototype or a change to another channel is a blocking finding.
3. Check that the adapter passes every request field to the system and maps every response field back, without dropping or renaming a value. A lost or altered field is a blocking finding.
4. For the API channel, check REST design against the existing endpoints the adapter already calls: resources are plural nouns, identifiers are path segments, and actions on a resource are sub-resources (for example `POST /api/orders/{id}/deliver`, not `POST /api/order/deliver?id=`). An endpoint that breaks the existing conventions is a blocking finding. For the UI channel, check that the adapter drives the page through the same kind of selectors the existing adapter uses.
5. Note anything else (helper naming, duplication, layout) as non-blocking.

## Outputs

${expected-outputs}
