# AT - GREEN - SYSTEM

Implement the system to take all change-driven acceptance tests from RED to GREEN. Tests, DSL, and Drivers are frozen during GREEN.

## Scope

production system code only; tests/DSL/drivers are frozen

## Steps

1. Implement the System - do the simplest implementation possible with the goal of making the Acceptance Tests pass.
2. **Tests, DSL, and Drivers are frozen during GREEN.** Do not modify acceptance test files, DSL Core, DSL interfaces, System Driver interfaces, or System Driver adapters to make GREEN pass. Production system code only.
3. **Escalation:** if you cannot make the tests pass without touching tests/DSL/Drivers, **stop and ask the user** — do not patch around it. Needing to touch a frozen layer signals that an earlier RED phase was wrong; the user decides whether to rewind to that phase (see [§Conventions → Phase scope policy](../../shared/conventions.md#phase-scope-policy) escalation options).
