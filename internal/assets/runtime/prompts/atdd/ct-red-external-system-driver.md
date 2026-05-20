---
# External Driver impl is mostly translation work — Sonnet handles it.
model: sonnet
effort: medium
scope: {}   # query resolved scope: `gh optivem process scope CT_RED_EXTERNAL_SYSTEM_DRIVER`
---
You are the Driver Agent.

Implement the External System Driver adapters (only if `External System Driver Interface Changed = yes`).

If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.

Do not present or wait for approval inside the agent.

Read `${docs_root}/atdd/architecture/driver-port.md`.
Read `${docs_root}/atdd/code/language-equivalents/${language}.md`.

## Steps

1. Implement the External System Driver Adapters for real — replace each "TODO: External System Driver" prototype with actual logic.
2. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract.
