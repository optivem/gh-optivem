---
# TBD placeholder — Sonnet until this task is fleshed out and benchmarked.
model: sonnet
effort: medium
scope: {}   # query resolved scope: `gh optivem process scope implement-external-system-stubs`
---
**Ownership of this task is TBD** — this placeholder body exists so the dispatcher can route the `implement-external-system-stubs` task without a missing-prompt error. The operator who claims this task should fill in the specifics (any anti-patterns specific to the dockerized stub layer beyond what is captured below). Until then, follow the task description below — it is fully specified — and treat this prompt as the canonical guide. This task is called from the HIGH orchestration `implement-and-verify-external-system-driver-adapters-contract-tests`, which wraps stub-implementation within the `change-system-behavior` CYCLE (see `process-flow.yaml`).

Implement the dockerized External System stub changes so all change-driven contract tests pass. Tests, DSL, and Drivers are frozen during the implement-external-system-stubs task.

Dockerized External System stub (routes, fixtures, middleware) only; tests/DSL/drivers are frozen.

## Steps

1. Implement the stub — add or update routes, fixtures, or middleware so the dockerized stub honors the new contract. Stub data must reflect the real Test Instance's contract (same shapes, same status codes, same error semantics).
2. **Tests, DSL, and Drivers are frozen during stub implementation.** Do not modify contract test files, DSL Core, DSL interfaces, External System Driver interfaces, or External System Driver adapters to make the tests pass. Stub code only.
3. **Escalation:** if you cannot make the tests pass without touching tests/DSL/Drivers, **stop and ask the user** — do not patch around it. Needing to touch a frozen layer signals that an earlier task in the calling CYCLE (the `write-contract-tests` or `implement-dsl` step) was wrong; the user decides whether to rewind to that task (see the scope rule's escalation options).
