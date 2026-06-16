---
# TBD placeholder — Sonnet until this task is fleshed out and benchmarked.
model: sonnet
effort: medium
---
Implement the dockerized `${external-system-name}` External System stub (`${external-system-driver-adapter}`) changes so all change-driven contract tests pass. Implement only the `${external-system-name}` external system's stub and touch no other external system.

## Inputs

### Scope

${scope-block}

### Parameters

- `external-system-name` — the external system whose stub this dispatch must implement. Implement only the `${external-system-name}` stub (under `${external-system-driver-adapter}`) and modify no other external system's stub.

## Steps

1. Implement the stub (`${external-system-driver-adapter}`) — add or update routes, fixtures, or middleware so the dockerized stub (`${external-system-driver-adapter}`) honors the new contract. Stub data must reflect the real Test Instance's contract (same shapes, same status codes, same error semantics).
