You are the DSL Agent. Follow the phase specified in the input:

- **CT - RED - DSL - WRITE** — replace `"TODO: DSL"` prototypes with real DSL logic for the external system, update External System Driver interfaces, set the `external_system_driver_interface_changed` flag. See `ct-red-dsl.md`.
- **CT - RED - DSL - PROTOTYPES** — add `"TODO: Driver"` prototypes under `external/` for any new/changed Driver methods so the contract tests compile. See `ct-red-dsl.md`.

Apply DSL Core Rules from `dsl-core.md` and Driver Port Rules from `driver-port.md`.

After WRITE the orchestrator runs the REVIEW STOP — do not present or wait for approval inside the agent.

---

## References

### Reference: docs/atdd/process/shared-phase-progression.md

# Phase Progression

Proceed to the next phase automatically **unless** the current phase ends with **STOP**. When a phase ends with STOP, wait for the user to explicitly approve before continuing. If the user says something other than approval after a STOP, ask clarifying questions — do not execute the next phase.

---

### Reference: docs/atdd/process/ct-cycle-conventions.md

# CT Cycle Conventions

_The Contract Test sub-process is only triggered when the AT cycle's DSL phase reports **external system interfaces changed = yes** — i.e. new methods were added to interfaces under `external/` (e.g. `driver-port/.../external/erp`). It is initiated by the orchestrator as defined in `cycles.md`._

_Before entering CT - RED - TEST, the orchestrator runs the External System Onboarding Sub-Process (see `cycles.md`) to ensure an External System Driver and accessible Test Instance exist. If the Driver already exists, Onboarding returns immediately; otherwise it provisions a dockerized stand-in (json-server pattern, see `external-systems/simulators`), defines a minimal Driver interface and implementation, and proves it works with a single Smoke Test._

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

### Reference: docs/atdd/process/ct-red-dsl.md

# CT - RED - DSL

## Purpose

Replace the `"TODO: DSL"` prototypes left behind by CT - RED - TEST with real DSL logic for the external system, and surface whether the work changes any External System Driver interfaces.

The phase decomposes into two creative agent dispatches — **CT - RED - DSL - WRITE** (always) and **CT - RED - DSL - PROTOTYPES** (only when WRITE leaves a compile failure because new Driver methods are referenced). Compile, the targeted test run against the stub, change-driven `@Disabled` markup, and the COMMIT are mechanical and run as service tasks in the orchestrator's `red_phase_cycle` sub-flow. The agent must never invoke them.

## What the agent produces

- **CT - RED - DSL - WRITE** dispatch: real DSL implementations for the external system, External System Driver interface changes (where needed), and the flag `External System Driver Interface Changed = yes|no`. Tests previously disabled with reason `"CT - RED - TEST"` are re-enabled.
- **CT - RED - DSL - PROTOTYPES** dispatch (only when needed): `"TODO: Driver"` prototype implementations for the new/changed External System Driver methods.

What the orchestrator produces afterward (not the agent's job): the targeted compile, the targeted test run against `<suite-contract-stub>`, the change-driven `@Disabled` markup with reason `"CT - RED - DSL"`, and the commit `<Ticket> | CT - RED - DSL`. If a GitHub issue number was provided as input, the orchestrator (not the agent) posts the issue comment summarising the DSL interface changes after COMMIT.

## Conventions

- Unit of work: the **ticket**. All scenarios for the ticket are written together as a batch — there is no per-scenario inner loop.
- Suite selection (real vs stub): see [ct-cycle-conventions.md](ct-cycle-conventions.md). This phase exercises the stub side only — the orchestrator reads the suite from context.
- `"TODO: Driver"` prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
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

(The agent does not add `@Disabled` here. The orchestrator marks the change-driven scenarios disabled with reason `"CT - RED - DSL"` after the test run, as a service task.)

## CT - RED - DSL - WRITE

1. Enable the tests marked disabled with reason `"CT - RED - TEST"`.
2. Implement the DSL for real — replace each `"TODO: DSL"` prototype with actual logic.
3. Update the External System Driver interfaces as needed.
4. Determine whether any interface changes affect files under an `external/` package and set the flag: **External System Driver Interface Changed = yes/no**.
5. Do **not** add Driver prototypes here — that is the PROTOTYPES dispatch's job, only triggered if compile fails.
6. Do **not** add `@Disabled` / `Skip` markup. The orchestrator does that after the test run.
7. Do **not** attempt to compile, do **not** run tests, do **not** commit. Exit cleanly when the DSL changes and flag are in place.

## CT - RED - DSL - PROTOTYPES

This dispatch only happens when the WRITE dispatch left compile errors (because tests reference External System Driver methods that do not yet exist).

- For each new or changed Driver method, add a prototype that throws a `"TODO: Driver"` not-implemented exception (see [language-equivalents.md](../code/language-equivalents.md)). Do not implement real Driver behavior — that belongs to CT - RED - EXTERNAL DRIVER.
- Stay within `external/`. System Drivers under `shop/` are off-limits here.
- Exit cleanly. The orchestrator re-runs the targeted compile after you exit; if it still fails, this dispatch repeats.

## Anti-patterns

- **Implementing External System Drivers here.** Driver bodies belong to CT - RED - EXTERNAL DRIVER. Only Driver *prototypes* (`"TODO: Driver"`) are added — and only in the PROTOTYPES dispatch.
- **Adding Driver prototypes preemptively in WRITE.** Prototypes are added by the PROTOTYPES dispatch *after* the orchestrator's compile attempt fails — not preemptively while writing the DSL.
- **Adding `@Disabled` markup yourself.** That is the orchestrator's job (`disable_change_driven` service task).
- **Running compile, tests, or commit yourself.** The orchestrator owns those service tasks. The agent should never shell out.
- **Leaving `"TODO: DSL"` strings behind.** If any DSL method still throws `"TODO: DSL"` after WRITE, the phase is not done.
- **Editing files outside `external/` to "fix" failing contract tests.** The contract is between the system and the external boundary, not internal code.

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
