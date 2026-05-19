# CT - GREEN - STUBS

Implement the dockerized External System stub changes to take all change-driven contract tests from RED to GREEN. Tests, DSL, and Drivers are frozen during GREEN.

## Scope

This phase touches the `external_driver_adapter` layer (bare layer
name; resolved physical path lives in `gh-optivem.yaml paths:` —
inspect with `gh optivem process scope CT_GREEN_STUBS`). Dockerized
External System stub (routes, fixtures, middleware) only;
tests/DSL/drivers are frozen.

See [the scope rule](../../shared/scope.md).

## Steps

1. Implement the stub — add or update routes, fixtures, or middleware so the dockerized stub honors the new contract. Stub data must reflect the real Test Instance's contract (same shapes, same status codes, same error semantics).
2. **Tests, DSL, and Drivers are frozen during GREEN.** Do not modify contract test files, DSL Core, DSL interfaces, External System Driver interfaces, or External System Driver adapters to make GREEN pass. Stub code only.
3. **Escalation:** if you cannot make the tests pass without touching tests/DSL/Drivers, **stop and ask the user** — do not patch around it. Needing to touch a frozen layer signals that an earlier RED phase was wrong; the user decides whether to rewind to that phase (see [§Conventions → Phase scope policy](../../../shared/conventions.md#phase-scope-policy) escalation options).
