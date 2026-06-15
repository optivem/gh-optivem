---
# TBD placeholder — Sonnet until this task is fleshed out and benchmarked.
model: sonnet
effort: medium
---
Implement the dockerized External System real simulator (`${external-system-driver-adapter}`) changes so all change-driven contract tests pass against the real system.

## Inputs

### Scope

${scope-block}

## Steps

1. Implement the simulator (`${external-system-driver-adapter}`) — add or update routes, fixtures, or middleware so the dockerized simulator (`${external-system-driver-adapter}`) honors the new contract. The simulator stands in for the real Test Instance, so it must reflect the published contract exactly (same shapes, same status codes, same error semantics).
