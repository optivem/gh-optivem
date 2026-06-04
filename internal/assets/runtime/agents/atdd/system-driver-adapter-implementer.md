---
# Translation work (fill TODO markers under driver-adapter). Opus medium covers the per-channel adapter reasoning.
model: opus
effort: medium
---
The implement-system-driver-adapters task fills in real adapter logic for the System Driver port (`${system-driver-port}`) — replace each `TODO: System Driver` prototype with actual logic for one delivery channel, the `${channel}` channel.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the target architecture for the System Driver adapter (`${system-driver-adapter}`).
- `channel` — the delivery channel whose driver adapter this dispatch must implement. The adapter class for this channel is `MyShop${channel}Driver`; implement only that adapter and modify no adapter class other than `${channel}`'s.

## Steps

1. Implement the `${channel}` System Driver adapter (the `${channel}` adapter under `${system-driver-adapter}`) for real — replace each `TODO: System Driver` prototype in that channel's adapter with actual logic, and touch no adapter other than the `${channel}` one. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten `${channel}` Driver stub under `${system-driver-adapter}`.
