---
# Translation work (fill TODO markers under driver-adapter). Opus medium covers the per-channel adapter reasoning.
model: opus
effort: medium
---
The implement-system-driver-adapters task fills in real adapter logic for the System Driver port (`${driver-port}`) — replace each `TODO: System Driver` prototype with actual logic for one delivery channel, the `${channel}` channel.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the target architecture for the System Driver adapter (`${driver-adapter}`).
- `channel` — the delivery channel whose driver adapter this dispatch must implement (e.g. `api`, `ui`). Each channel has its own adapter class (e.g. `MyShop${channel}Driver`); implement only the `${channel}` one and leave the other channels' adapters untouched.

## Steps

1. Implement the `${channel}` System Driver adapter (the `${channel}` adapter under `${driver-adapter}`) for real — replace each `TODO: System Driver` prototype in that channel's adapter with actual logic, leaving the other channels' adapter stubs as they are. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten `${channel}` Driver stub under `${driver-adapter}`.
