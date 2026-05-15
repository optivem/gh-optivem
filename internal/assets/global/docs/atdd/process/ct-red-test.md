# CT - RED - TEST

## Purpose

Express the contract between the system and the real external system as executable tests. The contract tests are the *contract*: they must PASS against the real Test Instance and FAIL against the dockerized stub before the cycle is allowed to proceed.

## What it produces

- Commit `<Ticket> | CT - RED - TEST` containing the new contract tests, any DSL interface additions, and the `"TODO: DSL"` prototype impls that go with them — all written together in WRITE so the result compiles.
- Tests in state: contract tests disabled with reason `"CT - RED - TEST"`

## Conventions

- Suite selection (real vs stub): see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit message format: see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Onboarding pre-condition (Driver + Test Instance must exist): see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit handoff (the wrapping CLI commits, not the agent): see [cycles.md § Commit Handoff](cycles.md#commit-handoff).
- Phase progression and STOP semantics: see [shared-phase-progression.md](shared-phase-progression.md).
- `@Disabled` / skip syntax and "TODO: DSL" exception strings per language: see [language-equivalents.md](../code/language-equivalents.md).

## Example

A contract test calling a not-yet-implemented DSL method. The DSL interface declaration and `"TODO: DSL"` prototype impl that make the test compile are added in the same WRITE step.

```java
@Test
void promotion_endpoint_returns_default_no_promotion_state() {
    erp.promotion()
        .shouldHaveActive(false)
        .shouldHaveDiscount(1.0);
}
```

## CT - RED - TEST - WRITE

1. Write External System Contract Tests against the existing DSL surface.
   - If new DSL methods are needed, call them in the test as if they exist.
2. **Add the DSL stubs the tests reference.** For every new DSL method the tests call:
   - Add the method declaration to the DSL interface.
   - Implement a `"TODO: DSL"` not-implemented prototype (see [language-equivalents.md](../code/language-equivalents.md)). Minimum signature only — no behaviour.
   The result must compile. The RED state is proven later by runtime test failure, not by compile failure.
3. Verify the tests PASS against the Real External System (Test Instance):
   ```bash
   gh optivem test run --suite <suite-contract-real> --test <TestMethodName>
   ```
   If they do not pass, that is a real contract problem — ask the user for support and STOP. Do NOT continue.
4. Verify the tests FAIL against the dockerized Stub External System:
   ```bash
   gh optivem test run --suite <suite-contract-stub> --test <TestMethodName>
   ```
5. Mark the tests as disabled with reason `"CT - RED - TEST"` (see [language-equivalents.md](../code/language-equivalents.md)).

## Anti-patterns

- Skipping the real-instance verification "because the tests look right" — without `<suite-contract-real>` passing, you have no evidence the contract is real.
- Marking tests disabled before the real-vs-stub verification has run — that hides the contract from review.
- Implementing real DSL behavior here — that belongs in CT - RED - DSL. This phase only adds `"TODO: DSL"` prototypes (minimum signature, no behaviour) when needed to make tests compile.
- **Producing a non-compiling WRITE.** Compile-fail in RED is no longer the expected path; it trips a human review STOP. Always add the DSL stubs the tests reference in the same WRITE step.
- Adding fields or assertions that are not part of the contract being expressed — keep each test minimal.
