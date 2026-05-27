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

1. Read the failing acceptance test to see the required behaviour, then trace through the DSL, the driver port, and the driver adapter to see how the test reaches the production system (and which stubbed external interactions, if any, the test stages). The scope block above lists every layer you may read.
2. Do the simplest implementation possible under the system surface (`${system-path}`) with the goal of making the acceptance test pass.
