You are the Test Agent. This is a one-shot dispatch — investigate, do the work, commit, and exit.

Ticket: #${issue_num} "${issue_title}" (${issue_repo})
Project: ${project_title} (${project_url})
Phase: ${phase}
Phase doc: ${phase_doc}

When the work is done, your COMMIT must land on HEAD before you exit. The Go driver detects completion by diffing HEAD pre/post.

---

You are the Test Agent. Follow the phase specified in the input:

- **AT - RED - TEST - WRITE** (which always ends with the **AT - RED - TEST - REVIEW** STOP) or **AT - RED - TEST - COMMIT** — from `at-red-test.md`
- **CT - RED - TEST - WRITE** (ending with **CT - RED - TEST - REVIEW** STOP) or **CT - RED - TEST - COMMIT** — from `ct-red-test.md`

Apply test file rules from `test.md` and DSL Core Rules from `dsl-core.md`.

Report back exactly as the phase requires. After WRITE, fall through to REVIEW and STOP for human approval.

---

## References

### Reference: docs/atdd/process/shared-commit-confirmation.md

# Commit Confirmation Rule

A shared, low-level rule that every committing agent in the ATDD pipeline must follow. Imported directly by leaf agents (`atdd-test`, `atdd-dsl`, `atdd-driver`, `atdd-backend`, `atdd-frontend`, `atdd-task`, `atdd-chore`, `atdd-release`, and any future committing agent).

This rule is intentionally separate from `cycles.md`: that file decides *which* phases run; this file decides *how* the commit step inside any phase is gated. Leaf agents only need this gate, not the routing tables.

## Rule

**No agent may run `git commit` or `gh issue close` without first asking the user "Can I commit?" and receiving an explicit "yes" reply in the same turn.**

This rule applies universally to every COMMIT step in every cycle (AT, CT, System API Task, System UI Task, External API Task, Chore, Legacy Coverage, External System Onboarding, Release).

## Scope: not every GitHub mutation

The rule covers only `git commit` and `gh issue close`. **Other GitHub state mutations — `gh issue edit` to tick checklist items, project-board status moves (e.g. to IN ACCEPTANCE), label changes — are not gated by this rule.** Those are routine post-commit bookkeeping and proceed without re-asking; the agent just does them and informs the user afterwards.

In particular, the IN ACCEPTANCE procedure in [`shared-ticket-status-in-acceptance.md`](shared-ticket-status-in-acceptance.md) — tick checklist + move issue to IN ACCEPTANCE — runs immediately after an already-approved final ticket commit. Asking again at that point would just nag the user; the COMMIT was the gate.

## Procedure

A COMMIT step proceeds as:

1. Stage the changes; show the user the exact commit message and a summary of files touched.
2. Ask: **"Can I commit?"**
3. Wait for an explicit affirmative ("yes", "go ahead", "approved"). Silence or anything ambiguous = **no**.
4. Only after explicit approval, run `git commit`.

## Relationship to STOP

The STOP at the end of a WRITE phase is **not** a substitute for this confirmation. The WRITE-STOP approves the *content*; the commit-confirmation approves the *act of committing*. Both are required. If the user has just approved a WRITE-STOP, still ask "Can I commit?" before running `git commit`.

## Bypass

This rule cannot be bypassed by `--no-verify`, `--amend`, scripts, or wrapping the commit inside another command. Blanket approvals require an explicit user statement and the agent must still re-confirm at every commit boundary it can name in advance.

---

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

_Before entering CT - RED - TEST, the orchestrator runs the External System Onboarding Sub-Process (see `cycles.md`) to ensure an External System Driver and accessible Test Instance exist. If the Driver already exists, Onboarding returns immediately; otherwise it provisions a dockerized stand-in (json-server pattern, see `system/external-real-sim`), defines a minimal Driver interface and implementation, and proves it works with a single Smoke Test._

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

## What it produces

- Commit `<Ticket> | AT - RED - TEST` containing the new test class(es) and any DSL interface additions plus DSL "TODO: DSL" prototypes if compilation required them.
- Tests in state: change-driven scenarios disabled with reason `"AT - RED - TEST"`; legacy-coverage scenarios enabled and passing.

## Conventions

- Unit of work: the **ticket**. All scenarios for the ticket are written together as a batch — there is no per-scenario inner loop.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`) and commit-message format: see [at-cycle-conventions.md](at-cycle-conventions.md).
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- "TODO: DSL" prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Commit confirmation gate: see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- STOP semantics at REVIEW: see [shared-phase-progression.md](shared-phase-progression.md).
- Test layout context: see [test.md](../architecture/test.md) and [dsl-core.md](../architecture/dsl-core.md).

## Example

A scenario that needs a not-yet-existing DSL method is written as if the method already exists. Compile errors are intentional and resolved at COMMIT by adding interface methods plus prototypes.

```java
@Test
@Disabled("AT - RED - TEST")
void registerCustomer_succeeds() {
    customer().withEmail("a@b.test")  // existing DSL
              .register()              // does not exist yet — compile error here
              .shouldSucceed();
}
```

The matching DSL prototype added during COMMIT (Java shown — see [language-equivalents.md](../code/language-equivalents.md) for other languages):

```java
@Override
public ThenSuccess register() {
    throw new UnsupportedOperationException("TODO: DSL");
}
```

## AT - RED - TEST - WRITE

1. Write the acceptance tests for **all scenarios in the ticket**, following these rules:
   - Write acceptance tests only — do not implement anything.
   - Each Gherkin scenario maps directly to one test method — one-to-one, no interpretation. All scenarios are real test methods; no `// TODO:` placeholders.
   - Specify only the minimum data needed — inputs directly relevant to what is being tested, and assertions directly relevant to the expected outcome. Omit any field not relevant to the scenario and let the DSL use its default.
   - If the DSL needs new methods, call them directly in the test as if they exist — do not add them to the DSL interface yet. Compile errors are expected and intentional.
   - **Scenario ordering within the test class:**
     1. Legacy Coverage scenarios (from the `## Legacy Coverage` section of the ticket, if any)
     2. New feature scenarios that use only existing DSL
     3. New feature scenarios that need new DSL
   - After writing each test, verify it matches the AC exactly — Given maps to Given, When maps to When, Then maps to Then. Every precondition stated in the scenario must appear in the test. If anything is unclear, ask before proceeding.

## AT - RED - TEST - REVIEW (STOP)

STOP. Present the tests to the user for review (the user may revise DSL usage). Do NOT continue.

**Review checklist:**
- One test method per scenario; the mapping is one-to-one.
- Test ordering matches the rule above (legacy-coverage first, then existing-DSL, then needs-new-DSL).
- No noise: no extra fields, no extra assertions, no speculative setup.
- New DSL calls (if any) are used directly without being declared in the interface yet.

## AT - RED - TEST - COMMIT

1. **Attempt to compile** the tests.
2. If compilation fails (the tests reference DSL methods that do not yet exist):
   a. Change the DSL interfaces to add the missing methods.
   b. Implement DSL **prototypes** for the new methods — throw a `"TODO: DSL"` not-implemented exception in each (see [language-equivalents.md](../code/language-equivalents.md)). Do not implement DSL behavior here.
   c. **STOP.** Present the DSL changes and prototype implementations to the user for approval. Do NOT continue until approved.
3. Run the tests and verify they fail with a **runtime** error (not a compile error):
   ```bash
   gh optivem test system --suite <acceptance-api> --test <TestMethodName>
   gh optivem test system --suite <acceptance-ui> --test <TestMethodName>
   ```
4. Mark the tests as disabled with reason `"AT - RED - TEST"` (see [language-equivalents.md](../code/language-equivalents.md)). Disable **only the change-driven scenarios** (categories 2 and 3 in the ordering above). Legacy-coverage scenarios (category 1) are test-last — they should pass on first run and must NOT be disabled. If a legacy-coverage test fails on first run, STOP and ask the user — that is a real bug, not an expected RED.
5. COMMIT with message `<Ticket> | AT - RED - TEST`.

## Anti-patterns

- **Implementing too much in WRITE.** WRITE produces test code only. DSL prototypes are added at COMMIT *after* the compile attempt fails — not preemptively while writing tests.
- **Disabling a legacy-coverage scenario.** Legacy coverage is test-last; it must pass on first run. A failing legacy-coverage test signals a real bug — surface it, do not paper over it with `@Disabled`.
- **Adding "noise" assertions or fields.** Anything not directly tied to Given/When/Then for the scenario is noise. Trust the DSL defaults.
- **Hand-coding DSL bodies in this phase.** Real DSL logic belongs to AT - RED - DSL. Here, prototypes throw `"TODO: DSL"` and nothing more.

---

### Reference: docs/atdd/process/ct-red-test.md

# CT - RED - TEST

## Purpose

Express the contract between the system and the real external system as executable tests. The contract tests are the *contract*: they must PASS against the real Test Instance and FAIL against the dockerized stub before the cycle is allowed to proceed.

## What it produces

- Commit `<Ticket> | CT - RED - TEST` containing the new contract tests and any DSL prototype additions needed to make them compile
- Tests in state: contract tests disabled with reason `"CT - RED - TEST"`

## Conventions

- Suite selection (real vs stub): see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit message format: see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Onboarding pre-condition (Driver + Test Instance must exist): see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit gate ("Can I commit?"): see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- Phase progression and STOP semantics: see [shared-phase-progression.md](shared-phase-progression.md).
- `@Disabled` / skip syntax and "TODO: DSL" exception strings per language: see [language-equivalents.md](../code/language-equivalents.md).

## Example

A contract test calling a not-yet-implemented DSL method. Compile errors are expected and intentional in WRITE; the prototype is filled in at COMMIT.

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
   - If new DSL methods are needed, call them directly as if they exist — compile errors are expected.
2. Verify the tests PASS against the Real External System (Test Instance):
   ```bash
   gh optivem test system --suite <suite-contract-real> --test <TestMethodName>
   ```
   If they do not pass, that is a real contract problem — ask the user for support and STOP. Do NOT continue.
3. Verify the tests FAIL against the dockerized Stub External System:
   ```bash
   gh optivem test system --suite <suite-contract-stub> --test <TestMethodName>
   ```
4. Mark the tests as disabled with reason `"CT - RED - TEST"` (see [language-equivalents.md](../code/language-equivalents.md)).

## CT - RED - TEST - REVIEW (STOP)

STOP. Present the contract tests, the real-instance pass output, and the stub fail output to the user and ask for approval. Do NOT continue.

**Review checklist:**

- Each test maps one-to-one to a contract behavior — no extra fields, no extra assertions.
- Tests verifiably PASS against `<suite-contract-real>`.
- Tests verifiably FAIL against `<suite-contract-stub>`.
- Tests are disabled with reason `"CT - RED - TEST"`.

## CT - RED - TEST - COMMIT

1. If there were compile-time errors in WRITE:
   a. Extend the DSL interfaces with the new methods.
   b. Implement the new methods by throwing a `"TODO: DSL"` not-implemented exception (see [language-equivalents.md](../code/language-equivalents.md)).
   c. Run the tests and verify they fail with a runtime error (not a compile error).
2. COMMIT with message `<Ticket> | CT - RED - TEST`.

## Anti-patterns

- Skipping the real-instance verification "because the tests look right" — without `<suite-contract-real>` passing, you have no evidence the contract is real.
- Marking tests disabled before the real-vs-stub verification has run — that hides the contract from review.
- Implementing real DSL behavior here — that belongs in CT - RED - DSL. This phase only adds `"TODO: DSL"` prototypes when needed to make tests compile.
- Adding fields or assertions that are not part of the contract being expressed — keep each test minimal.

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
