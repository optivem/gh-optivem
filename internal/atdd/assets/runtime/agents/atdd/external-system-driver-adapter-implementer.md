---
# Translation work (fill TODO markers under the Real driver). Opus medium for per-implementation reasoning.
model: opus
effort: medium
---
The implement-external-system-driver-adapters task fills in real adapter logic for the External System Driver port (`${external-system-driver-port}`) — the Real and Stub drivers under `${external-system-driver-adapter}`. Replace each `TODO: External System Driver` prototype with actual logic.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the architecture variant (monolith / multitier) the implementation targets.

## Steps

1. Implement the External System Driver adapters (`${external-system-driver-adapter}`) for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten Driver stub under `${external-system-driver-adapter}`.
