---
# TBD placeholder — Sonnet until this task is fleshed out and benchmarked.
model: sonnet
effort: medium
---
Implement the dockerized `${external-system-name}` External System real simulator (`${external-system-driver-adapter}`) changes so all change-driven contract tests pass against the real system. Implement only the `${external-system-name}` external system's simulator and touch no other external system.

## Inputs

### Scope

${scope-block}

### Parameters

- `external-system-name` — the external system whose simulator this dispatch must implement. Implement only the `${external-system-name}` simulator (under `${external-system-driver-adapter}`) and modify no other external system's simulator.

## Steps

1. Implement the simulator (`${external-system-driver-adapter}`) — add or update routes, fixtures, or middleware so the dockerized simulator (`${external-system-driver-adapter}`) honors the new contract. The simulator stands in for the real Test Instance, so it must reflect the published contract exactly (same shapes, same status codes, same error semantics).
