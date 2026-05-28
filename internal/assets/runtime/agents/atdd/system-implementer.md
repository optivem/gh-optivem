---
# GREEN-stage production code to make failing acceptance tests pass. Opus high covers the cross-channel reasoning.
model: opus
effort: high
---
The implement-system task writes production code under the system surface (`${system-path}`) to make the failing acceptance tests pass.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).

## Steps

1. Read the failing Acceptance Test (`${at-test}`) to see the required behaviour, then trace through the DSL Port (`${dsl-port}`) and DSL Core (`${dsl-core}`) to the System Driver port/adapter pair (`${driver-port}`, `${driver-adapter}`) to see how the test reaches the production system. If the test stages stub external interactions, also read the External System Driver port/adapter pair (`${external-system-driver-port}`, `${external-system-driver-adapter}`) and the Contract Tests (`${ct-test}`) to see the stub contract the implementation must satisfy.
2. Do the simplest implementation possible under the system surface (`${system-path}`) with the goal of making the acceptance test pass.
