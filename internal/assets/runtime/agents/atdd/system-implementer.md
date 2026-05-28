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
3. When the AT asserts persisted state (a column read/written, an audit-log entry, a soft-delete tombstone, etc.), also add a schema migration under the shared migration set (`${system-db-migration-path}`) — a single timestamped SQL file in the Flyway naming convention (`V{YYYYMMDDHHMMSS}__{description}.sql`, forward-only, no undo). Read the existing migrations first to see the current schema; do not redeclare columns that already exist. The migration set is shared across every SUT (3 languages × 2 architectures); your one file is consumed by all of them.
