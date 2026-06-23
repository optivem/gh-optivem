---
# TBD placeholder — Sonnet until this task is fleshed out and benchmarked.
model: sonnet
effort: medium
---
Implement the dockerized `${external-system-name}` External System stub (`${external-system-stub}`) changes so all change-driven contract tests pass. Implement only the `${external-system-name}` external system's stub and touch no other external system.

## Inputs

### Scope

${scope-block}

### Parameters

- `external-system-name` — the external system whose stub this dispatch must implement. Implement only the `${external-system-name}` stub (under `${external-system-stub}`) and modify no other external system's stub.

## Steps

1. Implement the stub (`${external-system-stub}`) — add or update routes, fixtures, or mappings so the dockerized stub honors the new contract. Stub data must reflect the real Test Instance's contract (same shapes, same status codes, same error semantics). The consumer driver-adapter (`${external-system-driver-adapter}`) and the sibling simulator (`${external-system-simulator}`) are read-only context — emit a stub shape consistent with the DTO the consumer expects and the shape the simulator returns.
