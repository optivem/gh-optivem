You are the DSL Agent. This is a one-shot dispatch — investigate, do the work, commit, and exit.

Ticket: #${issue_num} "${issue_title}" (${issue_repo})
Project: ${project_title} (${project_url})
Phase: ${phase}
Phase doc: ${phase_doc}

When the work is done, do not commit and do not summarise — exit cleanly. The CLI will stage and commit your changes after you exit. The agent must never run `git commit`, `git add`, or `gh issue close`.

---

You are the DSL Agent. Follow the phase specified in the input:

- **AT - RED - DSL - WRITE** (always falling through to the **AT - RED - DSL - REVIEW** STOP) or **AT - RED - DSL - COMMIT** — from `at-red-dsl.md`
- **CT - RED - DSL - WRITE** (falling through to **CT - RED - DSL - REVIEW** STOP) or **CT - RED - DSL - COMMIT** — from `ct-red-dsl.md`

Apply DSL Core Rules from `dsl-core.md` and Driver Port Rules from `driver-port.md`.

Report back exactly as the phase requires. After WRITE, fall through to REVIEW and STOP for human approval. STOP whenever a phase says STOP.

---

## References

### Reference: docs/atdd/process/shared-phase-progression.md

# Phase Progression

Proceed to the next phase automatically **unless** the current phase ends with **STOP**. When a phase ends with STOP, wait for the user to explicitly approve before continuing. If the user says something other than approval after a STOP, ask clarifying questions — do not execute the next phase.

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

### Reference: docs/atdd/process/at-red-dsl.md

# AT - RED - DSL

## Purpose

Replace the "TODO: DSL" prototypes from AT - RED - TEST with real DSL logic, and lock in which Driver interfaces (external and/or system) need to change as a consequence. Tests stay red — they will only go green once Drivers and the system implementation catch up.

## What it produces

- Commit `<Ticket> | AT - RED - DSL` containing real DSL implementations, any Driver interface changes, and Driver "TODO: Driver" prototypes for any Driver interface that changed.
- Flag set: `External System Driver Interface Changed = yes|no`.
- Flag set: `System Driver Interface Changed = yes|no`.
- Tests in state: change-driven scenarios disabled with reason `"AT - RED - DSL"`; legacy-acceptance-criteria scenarios still enabled and passing.

## Conventions

- Suite selection (`<acceptance-api>` / `<acceptance-ui>`) and commit-message format: see [at-cycle-conventions.md](at-cycle-conventions.md).
- `@Disabled` / skip syntax and "TODO: Driver" prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Definition of an "interface change" (DSL Interface, External System Driver, System Driver): see [glossary.md](glossary.md).
- Commit confirmation gate: see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- STOP semantics at REVIEW: see [shared-phase-progression.md](shared-phase-progression.md).
- DSL layout context: see [dsl-core.md](../architecture/dsl-core.md).

## Example

Before — DSL prototype committed in AT - RED - TEST:

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

If `customerDriver.register(...)` is a new method on the System Customer Driver port, the System Driver interface has changed — set the flag accordingly and add a Driver "TODO: Driver" prototype during COMMIT.

## AT - RED - DSL - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - TEST"`.
2. Implement the DSL for real — replace each "TODO: DSL" prototype with actual logic.
3. Update the Driver interfaces as needed to support the new DSL behavior.
4. Check whether any interface changes (see [glossary.md](glossary.md)) affect external-system Drivers. Set the flag: **External System Driver Interface Changed = yes/no**.
5. Check whether any interface changes affect system Drivers. Set the flag: **System Driver Interface Changed = yes/no**.

## AT - RED - DSL - REVIEW (STOP)

STOP. Present the DSL implementation, Driver interface changes, and both flags to the user and ask for approval. Do NOT continue.

**Review checklist:**
- "TODO: DSL" prototypes are gone — every change-driven DSL method has real logic.
- Driver interface changes are minimal: only what the new DSL actually calls.
- Both flags reflect reality: an external-driver port change means `External System Driver Interface Changed = yes`; a system-driver port change means `System Driver Interface Changed = yes`.
- No system implementation, no test edits, no Driver bodies — only DSL code, Driver interfaces, and flag values.

## AT - RED - DSL - COMMIT

1. **If any Driver interface changed** (either flag is `yes`):
   a. Implement Driver **prototypes** for the new/changed Driver methods — throw a `"TODO: Driver"` not-implemented exception in each (see [language-equivalents.md](../code/language-equivalents.md)).
2. Run the tests and verify they fail with a runtime error:
   ```bash
   gh optivem test system --suite <acceptance-api> --test <TestMethodName>
   gh optivem test system --suite <acceptance-ui> --test <TestMethodName>
   ```
3. Mark the tests as disabled with reason `"AT - RED - DSL"` (see [language-equivalents.md](../code/language-equivalents.md)).
4. Ensure that no test files are (accidentally) in the list of changed files.
5. COMMIT with message `<Ticket> | AT - RED - DSL`.

## Anti-patterns

- **Implementing Driver bodies in this phase.** Drivers are prototyped here (`"TODO: Driver"`); real Driver code belongs to CT - RED - EXTERNAL DRIVER and/or AT - RED - SYSTEM DRIVER.
- **Forgetting to set both flags.** Both `External System Driver Interface Changed` and `System Driver Interface Changed` must be set explicitly — an unset flag is a bug. They gate downstream phases.
- **Leaving "TODO: DSL" behind.** If any DSL method still throws `"TODO: DSL"` after this phase, the phase is not done.
- **Touching test files.** Re-enabling tests at WRITE and disabling them again at COMMIT is the only test-file activity here. Anything else (changing assertions, adding scenarios) means you're in the wrong phase.

---

### Reference: docs/atdd/process/ct-red-dsl.md

# CT - RED - DSL

## Purpose

Replace the `"TODO: DSL"` prototypes left behind by CT - RED - TEST with real DSL logic for the external system, and surface whether the work changes any External System Driver interfaces.

## What it produces

- Commit `<Ticket> | CT - RED - DSL` containing the real DSL implementation, any updated Driver interfaces, and `"TODO: Driver"` prototypes for new/changed Driver methods
- Flag set: `external_system_driver_interface_changed = yes|no`
- Tests in state: contract tests disabled with reason `"CT - RED - DSL"`
- GitHub issue comment summarising DSL interface changes (if an issue number was provided as input)

## Conventions

- Suite selection (real vs stub): see [ct-cycle-conventions.md](ct-cycle-conventions.md). This phase exercises the stub side only.
- Commit message format: see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit gate ("Can I commit?"): see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- Phase progression and STOP semantics: see [shared-phase-progression.md](shared-phase-progression.md).
- `"TODO: Driver"` exception string and `@Disabled` syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Definitions of DSL Interface and External System Driver: see [glossary.md](glossary.md).

## Example

Replace the `"TODO: DSL"` prototype with real DSL logic. Driver methods stay as `"TODO: Driver"` prototypes — they get implemented in CT - RED - EXTERNAL DRIVER.

```diff
 public PromotionResult promotion() {
-    throw new UnsupportedOperationException("TODO: DSL");
+    PromotionResponse response = erpDriver.getPromotion();
+    return new PromotionResult(response.isActive(), response.getDiscount());
 }
```

## CT - RED - DSL - WRITE

1. Enable the tests marked disabled with reason `"CT - RED - TEST"`.
2. Implement the DSL for real — replace each `"TODO: DSL"` prototype with actual logic.
3. Update the External System Driver interfaces as needed.
4. Determine whether any interface changes affect files under an `external/` package and set `external_system_driver_interface_changed = yes|no`.

## CT - RED - DSL - REVIEW (STOP)

STOP. Present the DSL implementation, Driver interface changes, and the `external_system_driver_interface_changed` flag to the user and ask for approval. Do NOT continue.

**Review checklist:**

- DSL methods now contain real logic — no `"TODO: DSL"` strings remain.
- Driver interface changes (if any) are confined to files under `external/`.
- The `external_system_driver_interface_changed` flag accurately reflects whether any Driver interface changed.

## CT - RED - DSL - COMMIT

1. For any new or changed External System Driver methods, add `"TODO: Driver"` prototypes that throw a not-implemented exception (see [language-equivalents.md](../code/language-equivalents.md)).
2. Run the contract tests and verify they fail with a runtime error:
   ```bash
   gh optivem test system --suite <suite-contract-stub> --test <TestMethodName>
   ```
3. Mark the tests as disabled with reason `"CT - RED - DSL"` (see [language-equivalents.md](../code/language-equivalents.md)).
4. COMMIT with message `<Ticket> | CT - RED - DSL`.
5. If a GitHub issue number was provided as input, post a comment on the issue summarising the DSL interface changes (new methods added, interfaces updated).

## Anti-patterns

- Implementing External System Drivers here — Driver bodies belong in CT - RED - EXTERNAL DRIVER. Only Driver *prototypes* (`"TODO: Driver"`) are added in this phase.
- Forgetting to post the GitHub-issue comment when an issue number was provided — it's the audit trail of the DSL interface change.
- Leaving `"TODO: DSL"` strings behind in the committed code — every prototype must be replaced with real logic.
- Editing files outside `external/` to "fix" failing contract tests — the contract is between the system and the external boundary, not internal code.

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

### Reference: docs/atdd/architecture/driver-port.md

# Driver Port Rules

## External vs Internal Systems

Driver interfaces are split into two categories by directory:

- **`external/`** — external systems the shop depends on (e.g. `external/clock`, `external/erp`, `external/tax`). These require contract tests when the interface changes (see `glossary.md` for the definition of *interface change*).
- **`shop/`** — the shop itself (API + UI drivers). No contract tests needed.

This convention applies identically in `driver-port/`, `driver-adapter/`, and `dsl-core/src/.../usecase/`.

- Response DTOs must not repeat fields that were already in the request. Only include fields generated by the system (e.g. IDs, timestamps).
- In response DTOs, the ID field(s) must be declared first, before any other fields.
- Request DTOs must use only string fields — never numeric, boolean, or other non-string types. This applies to all request DTOs across all layers: driver-port DTOs, driver-adapter client DTOs (e.g. `Ext*Request`), and any other request objects. This allows invalid values (e.g. `"lala"` for a numeric field, `"yes"` for a boolean) to be passed through for negative test scenarios. Type conversion must happen inside the HTTP client or serialization layer, not in the DTO itself. See `language-equivalents.md` for the string field type in each language.
- Driver interface methods for fetching a single resource must use only that resource's own ID — never add extra navigation context to a `getX(id)` method. If a resource has no standalone ID (e.g. a coupon applied to an order), model it as a sub-resource and access it through the parent (e.g. `getCoupon(orderNumber)`). If a UI driver needs additional context to navigate, it must manage that internally.
- UI drivers must never navigate directly to a URL (e.g. `page.navigate(baseUrl + "/orders/" + id)`). Always simulate real user behaviour by starting from the home page and clicking through the UI.

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
