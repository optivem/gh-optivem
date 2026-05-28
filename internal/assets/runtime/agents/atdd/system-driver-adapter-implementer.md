---
# Translation work (fill TODO markers under driver-adapter). Opus medium covers the per-channel adapter reasoning.
model: opus
effort: medium
---
The implement-system-driver-adapters task fills in real adapter logic for the System Driver port (`${driver-port}`) — replace each `TODO: System Driver` prototype with actual logic.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the target architecture for the System Driver adapter (`${driver-adapter}`).

## Steps

1. Implement the System Driver adapter (`${driver-adapter}`) for real — replace each `TODO: System Driver` prototype with actual logic. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten Driver stub under `${driver-adapter}`.
