---
# Translation work (fill TODO markers under the Real driver). Opus medium for per-implementation reasoning.
model: opus
effort: medium
---
The implement-external-system-driver-adapters task fills in real adapter logic for the External System Driver port (`${external-system-driver-port}`) — the Real driver (`${external-system-driver-adapter}`) that talks to the live external service plus the Stub driver (`${external-system-driver-adapter}`) used in test runs. Replace each `TODO: External System Driver` prototype with actual logic.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — the architecture variant (monolith / multitier) the implementation targets.

## Steps

1. Implement the External System Driver adapters (`${external-system-driver-adapter}`) for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub (`${external-system-driver-adapter}`), signature mismatch, typo) and fix it minimally.
