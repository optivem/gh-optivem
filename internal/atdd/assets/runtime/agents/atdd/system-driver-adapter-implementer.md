---
# Translation work (fill TODO markers under driver-adapter). Sonnet: shallow per-channel
# translation, hard-gated by the acceptance-${channel} suite (low blast radius). Trialled
# down from opus 2026-06-17.
model: sonnet
effort: medium
---
The implement-system-driver-adapters task fills in real adapter logic for the System Driver port (`${system-driver-port}`) — replace each `TODO: System Driver` prototype with actual logic for one delivery channel, the `${channel}` channel.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the target architecture for the System Driver adapter (`${system-driver-adapter}`).
- `channel` — the delivery channel whose driver adapter this dispatch must implement. Implement only the `${channel}` channel's adapter (under `${system-driver-adapter}`) and modify no other channel's adapter.

## Steps

1. Implement the `${channel}` System Driver adapter (the `${channel}` adapter under `${system-driver-adapter}`) for real — replace each `TODO: System Driver` prototype in that channel's adapter with actual logic, and touch no adapter other than the `${channel}` one. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten `${channel}` Driver stub under `${system-driver-adapter}`.
