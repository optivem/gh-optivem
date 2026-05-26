---
# TBD placeholder — Sonnet until this task is fleshed out and benchmarked.
model: sonnet
effort: medium
---
**Ownership of this task is TBD** — this placeholder body exists so the dispatcher can route the `implement-external-system-stubs` task without a missing-prompt error. The operator who claims this task should fill in the specifics (any anti-patterns specific to the dockerized stub layer beyond what is captured below). Until then, follow the task description below — it is fully specified — and treat this prompt as the canonical guide.

Implement the dockerized External System stub changes so all change-driven contract tests pass.

## Inputs

- This task receives no per-invocation parameters — context is in the preamble + scope.

## Steps

1. Implement the stub — add or update routes, fixtures, or middleware so the dockerized stub honors the new contract. Stub data must reflect the real Test Instance's contract (same shapes, same status codes, same error semantics).
