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

   **Implement every port method the contract suite exercises.** The contract tests under verification call a specific set of External System Driver port methods; **every** one of those must have a real implementation before you declare complete. A leftover `TODO: External System Driver` / `throw` stub on a method the contract suite calls is an incomplete implementation that halts the next run — not a tolerable default.
2. **Done-check (verify-path scope only).** Before declaring complete, grep the concrete driver(s) under `${external-system-driver-adapter}` for remaining `TODO: External System Driver` markers and throwing stubs, and confirm **none sit on a port method the contract suite calls**. Treat any such remaining stub on the verify path as incomplete and implement it. Scope is the verify path only — port methods the contract suite does **not** exercise are out of scope and may keep their stubs; do not implement them.
