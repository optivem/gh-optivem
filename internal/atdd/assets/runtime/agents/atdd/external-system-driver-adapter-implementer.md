---
# Translation work (fill TODO markers under the Real driver). Sonnet: shallow translation
# against a published API contract, double-gated by contract-real + contract-stub suites.
# Trialled down from opus 2026-06-17.
model: sonnet
effort: medium
---
The implement-external-system-driver-adapters task fills in real adapter logic for the `${external-system-name}` External System Driver port (`${external-system-driver-port}`) — the Real and Stub drivers under `${external-system-driver-adapter}`. Replace each `TODO: External System Driver` prototype with actual logic. Implement only the `${external-system-name}` external system's adapters and touch no other external system.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the architecture variant (monolith / multitier) the implementation targets.
- `external-system-name` — the external system whose driver adapters this dispatch must implement. Implement only the `${external-system-name}` adapters (under `${external-system-driver-adapter}`) and modify no other external system's adapters.

## Steps

1. Implement the External System Driver adapters (`${external-system-driver-adapter}`) for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten Driver stub under `${external-system-driver-adapter}`.
