# AT - RED - TEST

## Purpose

Turn the ticket's change-driven AC into compiling, runtime-failing acceptance tests in a single batch. This is the entry point of the AT Cycle: it locks in test intent before any DSL, Driver, or system code is written.

## What it produces

- Commit `<Ticket> | AT - RED - TEST` containing the new test class(es), any DSL interface additions, and the `"TODO: DSL"` prototype impls that go with them — all written together in WRITE so the result compiles.
- Tests in state: change-driven scenarios disabled with reason `"AT - RED - TEST"`; legacy-coverage scenarios enabled and passing.

## Conventions

- Unit of work: the **ticket**. All scenarios for the ticket are written together as a batch — there is no per-scenario inner loop.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`) and commit-message format: see [at-cycle-conventions.md](at-cycle-conventions.md).
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- "TODO: DSL" prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Commit handoff (the wrapping CLI commits, not the agent): see [cycles.md § Commit Handoff](cycles.md#commit-handoff).
- STOP semantics at REVIEW: see [shared-phase-progression.md](shared-phase-progression.md).
- Test layout context: see [test.md](../architecture/test.md) and [dsl-core.md](../architecture/dsl-core.md).

## Example

A scenario that needs a not-yet-existing DSL method is written as if the method already exists. The DSL interface declaration and `"TODO: DSL"` prototype impl that make the test compile are added in the same WRITE step.

```java
@Test
@Disabled("AT - RED - TEST")
void registerCustomer_succeeds() {
    customer().withEmail("a@b.test")  // existing DSL
              .register()              // does not exist yet — compile error here
              .shouldSucceed();
}
```

The matching DSL prototype added in the same WRITE step (Java shown — see [language-equivalents.md](../code/language-equivalents.md) for other languages):

```java
@Override
public ThenSuccess register() {
    throw new UnsupportedOperationException("TODO: DSL");
}
```

## AT - RED - TEST - WRITE

1. Write the acceptance tests for **all scenarios in the ticket**, following these rules:
   - Each Gherkin scenario maps directly to one test method — one-to-one, no interpretation. All scenarios are real test methods; no `// TODO:` placeholders.
   - Specify only the minimum data needed — inputs directly relevant to what is being tested, and assertions directly relevant to the expected outcome. Omit any field not relevant to the scenario and let the DSL use its default.
   - If the DSL needs new methods, call them in the test as if they exist.
   - **Scenario ordering within the test class:**
     1. Legacy Coverage scenarios (from the `## Legacy Coverage` section of the ticket, if any)
     2. New feature scenarios that use only existing DSL
     3. New feature scenarios that need new DSL
   - After writing each test, verify it matches the AC exactly — Given maps to Given, When maps to When, Then maps to Then. Every precondition stated in the scenario must appear in the test. If anything is unclear, ask before proceeding.
2. **Add the DSL stubs the tests reference.** For every new DSL method the tests call:
   - Add the method declaration to the DSL interface.
   - Implement a `"TODO: DSL"` not-implemented prototype (see [language-equivalents.md](../code/language-equivalents.md)). Minimum signature only — no behaviour.
   The result must compile. The RED state is proven later by runtime test failure, not by compile failure.

## AT - RED - TEST - REVIEW (STOP)

STOP. Present the tests to the user for review (the user may revise DSL usage). Do NOT continue.

**Review checklist:**
- One test method per scenario; the mapping is one-to-one.
- Test ordering matches the rule above (legacy-coverage first, then existing-DSL, then needs-new-DSL).
- No noise: no extra fields, no extra assertions, no speculative setup.
- New DSL calls (if any) are used directly without being declared in the interface yet.

## AT - RED - TEST - COMMIT

1. Run the tests and verify they fail with a **runtime** error (not a compile error — WRITE was responsible for producing compiling code):
   ```bash
   gh optivem test run --suite <acceptance-api> --test <TestMethodName>
   gh optivem test run --suite <acceptance-ui> --test <TestMethodName>
   ```
2. Mark the tests as disabled with reason `"AT - RED - TEST"` (see [language-equivalents.md](../code/language-equivalents.md)). Disable **only the change-driven scenarios** (categories 2 and 3 in the ordering above). Legacy-coverage scenarios (category 1) are test-last — they should pass on first run and must NOT be disabled. If a legacy-coverage test fails on first run, STOP and ask the user — that is a real bug, not an expected RED.
3. COMMIT with message `<Ticket> | AT - RED - TEST`.

## Anti-patterns

- **Hand-coding DSL bodies in this phase.** WRITE adds DSL stubs (`"TODO: DSL"`) — minimum signature, no behaviour. Real DSL logic belongs to AT - RED - DSL.
- **Producing a non-compiling WRITE.** Compile-fail in RED is no longer the expected path; it trips a human review STOP. Always add the DSL stubs the tests reference in the same WRITE step.
- **Disabling a legacy-coverage scenario.** Legacy coverage is test-last; it must pass on first run. A failing legacy-coverage test signals a real bug — surface it, do not paper over it with `@Disabled`.
- **Adding "noise" assertions or fields.** Anything not directly tied to Given/When/Then for the scenario is noise. Trust the DSL defaults.
