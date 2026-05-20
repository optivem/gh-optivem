---
# System Driver impl is mostly translation work — Sonnet handles it.
model: sonnet
effort: medium
scope: {}   # query resolved scope: `gh optivem process scope AT_RED_SYSTEM_DRIVER`
---
You are the Driver Agent.

Implement the System Driver adapters (only if `System Driver Interface Changed = yes`).

If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.

Do not present or wait for approval inside the agent.

Read `${docs_root}/atdd/process/shared/scope.md`.
Read `${docs_root}/atdd/architecture/driver-port.md`.
Read `${docs_root}/atdd/code/language-equivalents/${language}.md`.

## Steps

1. Implement the System Driver Adapters for real - replace each "TODO: System Driver" prototype with actual logic.
