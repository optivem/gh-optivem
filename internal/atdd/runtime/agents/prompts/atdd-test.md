You are the Test Agent. This is a one-shot dispatch — investigate, do the work, and exit.

Ticket: #${issue_num} "${issue_title}" (${issue_repo})
Project: ${project_title} (${project_url})
Phase: ${phase}
Phase doc: ${phase_doc}

When the work is done, do not summarise — exit cleanly. The orchestrator drives compile, test runs, disabling, and commits as separate service tasks; the agent must never run `git commit`, `git add`, `gh issue close`, the compile commands, or the test commands.

---

You are the Test Agent. Follow the phase specified in the input:

- **AT - RED - TEST - WRITE** — write tests only (no compile, no run, no disable, no commit). The orchestrator handles the rest. See `at-red-test.md`.
- **AT - RED - TEST - PROTOTYPES** — extend DSL interfaces with the missing methods and implement `"TODO: DSL"` prototypes so the tests compile. The orchestrator re-runs compile after you exit. See `at-red-test.md`.
- **CT - RED - TEST - WRITE** — write contract tests only. The orchestrator verifies them against the real Test Instance and the dockerized stub. See `ct-red-test.md`.
- **CT - RED - TEST - PROTOTYPES** — extend DSL interfaces with the missing methods and implement `"TODO: DSL"` prototypes so the contract tests compile. See `ct-red-test.md`.

Apply test file rules from `test.md` and DSL Core Rules from `dsl-core.md`.

After WRITE the orchestrator runs the REVIEW STOP — do not present or wait for approval inside the agent.

---

## References

### Reference: docs/atdd/process/at-cycle-conventions.md

# AT Cycle Conventions

## Suite Selection

Each acceptance test is annotated with a channel. Use the matching suite placeholder throughout all phases:
- `<acceptance-api>` — for tests annotated with `@Channel(API)`
- `<acceptance-ui>` — for tests annotated with `@Channel(UI)`

If a test covers both channels, run both suites.

## Commit Message Format

Every commit message follows the pattern: `<Ticket> | <Phase>`.

The unit of work in the AT Cycle is a **ticket**, not an individual scenario — all scenarios for the ticket are batched through each phase together (see `at-red-test.md`). Commit messages reflect the ticket title.

If a GitHub issue number was provided as input, prefix every commit message with `#<issue-number> | `. Example: `#42 | Register Customer | AT - RED - TEST`.

**Important:** The phase suffix in the message is the phase *prefix only* (e.g. `AT - RED - TEST`). Do **NOT** append `- WRITE`, `- REVIEW`, or `- COMMIT` to the phase in the commit message — those suffixes identify the section header only, not the commit message. (REVIEW is a STOP-only phase that produces no commit.)

---

### Reference: docs/atdd/process/ct-cycle-conventions.md

# CT Cycle Conventions

_The Contract Test sub-process is only triggered when the AT cycle's DSL phase reports **external system interfaces changed = yes** — i.e. new methods were added to interfaces under `external/` (e.g. `driver-port/.../external/erp`). It is initiated by the orchestrator as defined in `cycles.md`._

_Before entering CT - RED - TEST, the orchestrator runs the External System Onboarding Sub-Process (see `cycles.md`) to ensure an External System Driver and accessible Test Instance exist. If the Driver already exists, Onboarding returns immediately; otherwise it provisions a dockerized stand-in (json-server pattern, see `external-systems/external-real-sim`), defines a minimal Driver interface and implementation, and proves it works with a single Smoke Test._

## Suite Selection

Each contract test runs against two parallel suites — the real external-system Test Instance and the dockerized stub. Use the matching suite placeholder throughout all CT phases:

- `<suite-contract-real>` — the contract suite executed against the **real external system** (Test Instance).
- `<suite-contract-stub>` — the contract suite executed against the **dockerized stub** External System.

A CT phase that names only one of these placeholders is exercising one side of the real-vs-stub contract pair; CT - RED - TEST runs both, CT - RED - DSL and CT - GREEN - STUBS run the stub side only.

## Commit Message Format

Every commit message follows the pattern: `<Ticket> | <Phase>`.

The unit of work in the CT sub-process is a **ticket**, not an individual scenario — CT is entered once per ticket when AT's DSL phase reports external system interfaces changed = yes, and all CT phases run as a single per-ticket pass (mirroring the AT Cycle batching rule). Commit messages reflect the ticket title.

If a GitHub issue number was provided as input, prefix every commit message with `#<issue-number> | `. Example: `#42 | Register Customer | CT - RED - TEST`.

**Important:** The phase suffix in the message is the phase *prefix only* (e.g. `CT - RED - TEST`). Do **NOT** append `- WRITE`, `- REVIEW`, or `- COMMIT` to the phase in the commit message — those suffixes identify the section header only, not the commit message. (REVIEW is a STOP-only phase that produces no commit.)

---

### Reference: docs/atdd/process/at-red-test.md

# AT - RED - TEST

## Purpose

Turn the ticket's change-driven AC into compiling, runtime-failing acceptance tests in a single batch. This is the entry point of the AT Cycle: it locks in test intent before any DSL, Driver, or system code is written.

The phase decomposes into two creative agent dispatches — **AT - RED - TEST - WRITE** (always) and **AT - RED - TEST - PROTOTYPES** (only when WRITE leaves a compile failure). Compile, test runs, change-driven `@Disabled` markup, and the COMMIT are mechanical and run as service tasks in the orchestrator's `red_phase_cycle` sub-flow. The agent must never invoke them.

## What the agent produces

- **AT - RED - TEST - WRITE** dispatch: the new test class(es) only.
- **AT - RED - TEST - PROTOTYPES** dispatch (only when needed): the DSL interface additions plus `"TODO: DSL"` prototype implementations.

What the orchestrator produces afterward (not the agent's job): the targeted compile, the targeted test run, the change-driven `@Disabled` markup, and the commit `<Ticket> | AT - RED - TEST`.

## Conventions

- Unit of work: the **ticket**. All scenarios for the ticket are written together as a batch — there is no per-scenario inner loop.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`): see [at-cycle-conventions.md](at-cycle-conventions.md). The orchestrator reads the suite from context and runs tests; the agent does not invoke `gh optivem test system`.
- "TODO: DSL" prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Test layout context: see [test.md](../architecture/test.md) and [dsl-core.md](../architecture/dsl-core.md).

## Example

A scenario that needs a not-yet-existing DSL method is written as if the method already exists. Compile errors are intentional — they trigger the orchestrator's compile gate, which dispatches the PROTOTYPES phase to add interface methods plus prototypes.

```java
@Test
void registerCustomer_succeeds() {
    customer().withEmail("a@b.test")  // existing DSL
              .register()              // does not exist yet — compile error here
              .shouldSucceed();
}
```

(The agent does not add `@Disabled` here. The orchestrator marks the change-driven scenarios disabled with reason `"AT - RED - TEST"` after the test run, as a service task.)

The matching DSL prototype added during the PROTOTYPES dispatch (Java shown — see [language-equivalents.md](../code/language-equivalents.md) for other languages):

```java
@Override
public ThenSuccess register() {
    throw new UnsupportedOperationException("TODO: DSL");
}
```

## AT - RED - TEST - WRITE

Write the acceptance tests for **all scenarios in the ticket**, following these rules:

- Write acceptance tests only — do not implement anything else.
- Each Gherkin scenario maps directly to one test method — one-to-one, no interpretation. All scenarios are real test methods; no `// TODO:` placeholders.
- Specify only the minimum data needed — inputs directly relevant to what is being tested, and assertions directly relevant to the expected outcome. Omit any field not relevant to the scenario and let the DSL use its default.
- If the DSL needs new methods, call them directly in the test as if they exist — do not add them to the DSL interface yet. Compile errors are expected and intentional; the orchestrator will dispatch PROTOTYPES if needed.
- **Scenario ordering within the test class:**
  1. Legacy Acceptance Criteria scenarios (from the `## Legacy Acceptance Criteria` section of the ticket, if any)
  2. New feature scenarios that use only existing DSL
  3. New feature scenarios that need new DSL
- After writing each test, verify it matches the AC exactly — Given maps to Given, When maps to When, Then maps to Then. Every precondition stated in the scenario must appear in the test. If anything is unclear, ask before proceeding.
- Do **not** add `@Disabled` / `Skip` markup. The orchestrator does that after the test run, as a service task.
- Do **not** attempt to compile, do **not** run tests, do **not** commit. Exit cleanly when the tests are written.

## AT - RED - TEST - PROTOTYPES

This dispatch only happens when the WRITE dispatch left compile errors (because tests reference DSL methods that do not yet exist).

- Extend the DSL interfaces with the methods the tests are calling.
- Implement each new method as a prototype that throws a `"TODO: DSL"` not-implemented exception (see [language-equivalents.md](../code/language-equivalents.md)). Do not implement real DSL behavior.
- Exit cleanly. The orchestrator re-runs the targeted compile after you exit; if it still fails, this dispatch repeats.

## Anti-patterns

- **Implementing too much in WRITE.** WRITE produces test code only. DSL prototypes are added by the PROTOTYPES dispatch *after* the orchestrator's compile attempt fails — not preemptively while writing tests.
- **Adding `@Disabled` markup yourself.** That is the orchestrator's job (`disable_change_driven` service task), driven by the language and the change-driven scenario list. Doing it in the agent risks disabling legacy-acceptance-criteria scenarios that should run.
- **Running compile or tests yourself.** The orchestrator owns those service tasks (`compile_system_tests`, `run_targeted_tests`). The agent should never shell out to compile or test commands.
- **Adding "noise" assertions or fields.** Anything not directly tied to Given/When/Then for the scenario is noise. Trust the DSL defaults.
- **Hand-coding DSL bodies in PROTOTYPES.** Real DSL logic belongs to AT - RED - DSL. Here, prototypes throw `"TODO: DSL"` and nothing more.

---

### Reference: docs/atdd/process/ct-red-test.md

# CT - RED - TEST

## Purpose

Express the contract between the system and the real external system as executable tests. The contract tests are the *contract*: they must PASS against the real Test Instance and FAIL against the dockerized stub before the cycle is allowed to proceed.

The phase decomposes into two creative agent dispatches — **CT - RED - TEST - WRITE** (always) and **CT - RED - TEST - PROTOTYPES** (only when WRITE leaves a compile failure). Compile, the real-vs-stub verification runs, change-driven `@Disabled` markup, and the COMMIT are mechanical and run as service tasks in the orchestrator's `red_phase_cycle` sub-flow. The agent must never invoke them.

## What the agent produces

- **CT - RED - TEST - WRITE** dispatch: the new contract test class(es) only.
- **CT - RED - TEST - PROTOTYPES** dispatch (only when needed): the DSL interface additions plus `"TODO: DSL"` prototype implementations.

What the orchestrator produces afterward (not the agent's job): the targeted compile, the real-suite verification (`<suite-contract-real>` must PASS — runtime contract gate), the targeted test run against the stub (`<suite-contract-stub>` must fail at runtime), the change-driven `@Disabled` markup with reason `"CT - RED - TEST"`, and the commit `<Ticket> | CT - RED - TEST`.

## Conventions

- Unit of work: the **ticket**. All scenarios for the ticket are written together as a batch — there is no per-scenario inner loop.
- Suite selection (`<suite-contract-real>` / `<suite-contract-stub>`): see [ct-cycle-conventions.md](ct-cycle-conventions.md). The orchestrator reads the suites from context/params and invokes the runner; the agent does not invoke `gh optivem test system`.
- Onboarding pre-condition (Driver + Test Instance must exist): see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- "TODO: DSL" prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).

## Example

A contract test calling a not-yet-implemented DSL method. Compile errors are intentional — they trigger the orchestrator's compile gate, which dispatches the PROTOTYPES phase to add interface methods plus prototypes.

```java
@Test
void promotion_endpoint_returns_default_no_promotion_state() {
    erp.promotion()
        .shouldHaveActive(false)
        .shouldHaveDiscount(1.0);
}
```

(The agent does not add `@Disabled` here. The orchestrator marks the change-driven scenarios disabled with reason `"CT - RED - TEST"` after the test runs, as a service task.)

## CT - RED - TEST - WRITE

Write the contract tests for **all scenarios in the ticket**, following these rules:

- Write contract tests only — do not implement anything else.
- Each test maps one-to-one to a contract behaviour — no extra fields, no extra assertions. Trust the DSL defaults.
- If new DSL methods are needed, call them directly as if they exist — compile errors are expected and intentional. The orchestrator dispatches PROTOTYPES if needed.
- Do **not** add `@Disabled` / `Skip` markup. The orchestrator does that after the test run.
- Do **not** attempt to compile, do **not** run tests, do **not** commit. Exit cleanly when the tests are written.

## CT - RED - TEST - PROTOTYPES

This dispatch only happens when the WRITE dispatch left compile errors (because tests reference DSL methods that do not yet exist).

- Extend the DSL interfaces with the methods the tests are calling.
- Implement each new method as a prototype that throws a `"TODO: DSL"` not-implemented exception (see [language-equivalents.md](../code/language-equivalents.md)). Do not implement real DSL behavior.
- Exit cleanly. The orchestrator re-runs the targeted compile after you exit; if it still fails, this dispatch repeats.

## Anti-patterns

- **Implementing too much in WRITE.** WRITE produces test code only. DSL prototypes are added by the PROTOTYPES dispatch *after* the orchestrator's compile attempt fails — not preemptively while writing tests.
- **Adding `@Disabled` markup yourself.** That is the orchestrator's job (`disable_change_driven` service task). Doing it in the agent risks disabling tests that should run.
- **Running the real-vs-stub suites yourself.** The orchestrator owns those service tasks (`verify_real_suite_passes` against `<suite-contract-real>` and `run_targeted_tests` against `<suite-contract-stub>`). The agent should never shell out to compile or test commands.
- **Hand-coding DSL bodies in PROTOTYPES.** Real DSL logic belongs to CT - RED - DSL. Here, prototypes throw `"TODO: DSL"` and nothing more.
- **Adding fields or assertions that are not part of the contract being expressed.** Keep each test minimal.

---

### Reference: docs/atdd/architecture/test.md

# Test File Rules

## Positive vs Negative Test Classes

Each use case has two test files (see `language-equivalents.md` for the file extension per language):

- **`<UseCase>PositiveTest`** — scenarios where `Then` asserts **success** (e.g. `shouldSucceed()`, resource is returned, state is correct).
- **`<UseCase>NegativeTest`** — scenarios where `Then` asserts **failure** (e.g. `shouldFailWith(...)`, error message returned).

When writing a first scenario and leaving the rest as `// TODO:` comments:
- If the first scenario is positive, put its `// TODO:` siblings in the **positive** file only if they are also positive; put negative `// TODO:` lines in the **negative** file.
- If new DSL is needed and only one test method is written, the remaining `// TODO:` lines must go into the correct file based on this rule — never mix positive and negative TODOs in the same file.

---

### Reference: docs/atdd/architecture/dsl-core.md

# DSL Core Rules

- In DSL step classes (Given/When/Then implementations), set all variable defaults only in the constructor. Never reference constants directly in `execute()` or other methods — use the fields instead.
- Every Given and When step class must have a public `with<FieldName>(value)` method for every field it declares. This is required even if the field is only used internally (e.g. in a `given` setup). Additional overloads for typed parameters (e.g. numeric, boolean, enum) are permitted alongside the string version. The port interface must also declare matching `with` methods.
- Given/When/Then step classes should have only string fields (never numeric or boolean types — see `language-equivalents.md` for the string type in each language). This allows passing invalid values (e.g., `"lala"` for a numeric field) for negative test scenarios. Accordingly, step method parameters for configuring data should also be string. Convert when calling the use case if needed.
- Use case classes should have only string fields (never numeric or boolean types). This allows passing invalid values (e.g., `"lala"` for a numeric field) for negative test scenarios. Convert to the needed type inside the use case if required.
- For resource-creation When steps, system-generated IDs (e.g. `orderNumber`) must be declared as alias fields with a default (e.g. `DEFAULT_ORDER_NUMBER`), just like `orderNumber` in `WhenPlaceOrderImpl`. The alias is passed to the use case, which stores the system-generated value from the response into context under that alias. The ID must never be sent as part of the request — only the alias string is used to register where the result is stored.
- Verification classes should have only public methods that return their own type. For example, all public methods in `PlaceOrderVerification` should return `PlaceOrderVerification`. Never add getter methods (e.g., `getOrderNumber()`) that return primitive or non-verification types.

---

### Reference: docs/atdd/code/language-equivalents.md

# Language Equivalents

Use this table when implementing any ATDD phase. All concepts are language-agnostic; this file shows the concrete syntax for each supported language.

## TODO Stubs

| Concept | Java | .NET (C#) | TypeScript |
|---------|------|-----------|------------|
| DSL stub | `throw new UnsupportedOperationException("TODO: DSL")` | `throw new NotImplementedException("TODO: DSL")` | `throw new Error("TODO: DSL")` |
| Driver stub | `throw new UnsupportedOperationException("TODO: Driver")` | `throw new NotImplementedException("TODO: Driver")` | `throw new Error("TODO: Driver")` |

## Test Disabling

| Operation | Java | .NET (C#) | TypeScript |
|-----------|------|-----------|------------|
| Disable a single test | `@Disabled("reason")` | `[Fact(Skip = "reason")]` or `[Theory(Skip = "reason")]` | `test.skip(true, "reason")` |
| Re-enable a test | Remove `@Disabled` | Remove `Skip = "reason"` | Remove `test.skip(...)` |

## String Field Types

"String fields" means the nullable string type in the target language:

| Language | String field type | Example |
|----------|------------------|---------|
| Java | `String` | `private String sku;` |
| .NET (C#) | `string?` | `public string? Sku { get; set; }` |
| TypeScript | `Optional<string>` | `private sku: Optional<string>;` |

## DTO Boilerplate

| Language | Request DTOs | Response DTOs |
|----------|-------------|---------------|
| Java | Lombok: `@Data @Builder @NoArgsConstructor @AllArgsConstructor` | Same |
| .NET (C#) | Auto-properties: `public string? Field { get; set; }` | `required` modifier for non-nullable IDs: `public required string Id { get; set; }` |
| TypeScript | `interface` with optional fields: `field?: Optional<string>` | `interface` with required fields: `field: string` |

## Test File Naming

| Language | Positive test file | Negative test file |
|----------|-------------------|-------------------|
| Java | `<UseCase>PositiveTest.java` | `<UseCase>NegativeTest.java` |
| .NET (C#) | `<UseCase>PositiveTest.cs` | `<UseCase>NegativeTest.cs` |
| TypeScript | `<UseCase>Positive.spec.ts` | `<UseCase>Negative.spec.ts` |

## Awaitable ShouldSucceed

The terminal `ShouldSucceed()` / `shouldSucceed()` assertion is awaitable in .NET and TypeScript:

| Language | Mechanism |
|----------|-----------|
| Java | Synchronous — no `await` needed |
| .NET (C#) | `IThenSuccess` implements `GetAwaiter()` — use `await ...ShouldSucceed()` |
| TypeScript | `ThenSuccessPort extends PromiseLike<void>` — use `await ...shouldSucceed()` |
