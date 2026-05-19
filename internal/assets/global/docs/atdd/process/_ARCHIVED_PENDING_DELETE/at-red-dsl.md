# AT - RED - DSL

## Purpose

Replace the "TODO: DSL" prototypes from AT - RED - TEST with real DSL logic, and lock in which Driver interfaces (external and/or system) need to change as a consequence. Tests stay red — they will only go green once Drivers and the system implementation catch up.

## What it produces

- After WRITE: real DSL implementations, any Driver interface changes, and Driver "TODO: Driver" prototypes for any Driver interface that changed exist in the working tree.
- Flag set: `External System Driver Interface Changed = yes|no`.
- Flag set: `System Driver Interface Changed = yes|no`.
- Tests in state: change-driven scenarios disabled with reason `"AT - RED - DSL"`; legacy-coverage scenarios still enabled and passing.

## Conventions

- `@Disabled` / skip syntax and "TODO: Driver" prototype syntax per language: see [language-equivalents/](../code/language-equivalents/README.md).
- Definition of an "interface change" (DSL Interface, External System Driver, System Driver): see [glossary.md](glossary.md).
- DSL layout context: see [dsl-core.md](../architecture/dsl-core.md).

## Example

Before — DSL prototype produced by AT - RED - TEST:

```java
@Override
public ThenSuccess register() {
    throw new UnsupportedOperationException("TODO: DSL");
}
```

After — real DSL logic delegating to the Driver port:

```java
@Override
public ThenSuccess register() {
    var request = new RegisterCustomerRequest(email, name);
    var response = customerDriver.register(request);
    return new ThenSuccess(response);
}
```

If `customerDriver.register(...)` is a new method on the System Customer Driver port, the System Driver interface has changed — set the flag accordingly and add a Driver "TODO: Driver" prototype during WRITE.

## AT - RED - DSL - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - TEST"`.
2. Implement the DSL for real — replace each "TODO: DSL" prototype with actual logic.
3. Update the Driver interfaces as needed to support the new DSL behavior. Keep interface changes **minimal** — only what the new DSL actually calls.
4. **Add the Driver stubs the new DSL references.** For every new or changed Driver method:
   - Update the Driver interface.
   - Implement a `"TODO: Driver"` not-implemented prototype (see [language-equivalents/](../code/language-equivalents/README.md)). Minimum signature only — no behaviour.
   The result must compile.
5. Check whether any interface changes (see [glossary.md](glossary.md)) affect external-system Drivers. Set the flag: **External System Driver Interface Changed = yes/no**.
6. Check whether any interface changes affect system Drivers. Set the flag: **System Driver Interface Changed = yes/no**.

**Scope:** Only DSL code, Driver interfaces, `"TODO: Driver"` prototypes, and flag values. No system implementation, no test changes beyond the re-enable in step 1, no Driver bodies.

## Anti-patterns

- **Implementing Driver bodies in this phase.** Drivers are prototyped here (`"TODO: Driver"`); real Driver code belongs to CT - RED - EXTERNAL DRIVER and/or AT - RED - SYSTEM DRIVER.
- **Forgetting to set both flags.** Both `External System Driver Interface Changed` and `System Driver Interface Changed` must be set explicitly — an unset flag is a bug. They gate downstream phases.
- **Leaving "TODO: DSL" behind.** If any DSL method still throws `"TODO: DSL"` after this phase, the phase is not done.
- **Touching test files.** Re-enabling tests at WRITE is the only test-file activity here. Anything else (changing assertions, adding scenarios) means you're in the wrong phase.
