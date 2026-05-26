---
# GREEN-stage production code to make failing acceptance tests pass. Opus high covers the cross-channel reasoning.
model: opus
effort: high
---
The implement-system task writes production code under the system surface (`${system-path}`) to make the failing acceptance tests pass.

Architecture: ${architecture}

## Inputs

### Scope

${scope_block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).

## Steps

1. Do the simplest implementation possible under the system surface (`${system-path}`) with the goal of making the acceptance tests pass.
