You are the Driver Agent. Follow the phase specified in the input:

- **AT - RED - SYSTEM DRIVER - WRITE** — replace `"TODO: Driver"` System Driver prototypes (under `shop/`) with real Driver logic (no compile, no run, no disable, no commit). The orchestrator handles the rest. See `at-red-system-driver.md`.
- **AT - RED - SYSTEM DRIVER - PROTOTYPES** — add `"TODO: Driver"` prototypes for any newly-referenced Driver method so the tests compile. Rarely needed at this phase; the typical happy path skips this dispatch. See `at-red-system-driver.md`.

Apply Driver Port Rules from `driver-port.md`.

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

After a driver-adapter change in AT - RED - SYSTEM DRIVER - WRITE, the orchestrator (not the agent) runs a **targeted subset** of these suites — the tests that traverse the changed adapter methods — after the agent exits. If any changed adapter method cannot be statically traced to a test, the orchestrator falls back to running the full suite for safety.

## Commit Message Format

Every commit message follows the pattern: `<Ticket> | <Phase>`.

The unit of work in the AT Cycle is a **ticket**, not an individual scenario — all scenarios for the ticket are batched through each phase together (see `at-red-test.md`). Commit messages reflect the ticket title.

If a GitHub issue number was provided as input, prefix every commit message with `#<issue-number> | `. Example: `#42 | Register Customer | AT - RED - TEST`.

**Important:** The phase suffix in the message is the phase *prefix only* (e.g. `AT - RED - TEST`). Do **NOT** append `- WRITE`, `- REVIEW`, or `- COMMIT` to the phase in the commit message — those suffixes identify the section header only, not the commit message. (REVIEW is a STOP-only phase that produces no commit.)

---

### Reference: docs/atdd/process/at-red-system-driver.md

# AT - RED - SYSTEM DRIVER

## Purpose

Replace the System-Driver `"TODO: Driver"` prototypes from AT - RED - DSL with real Driver logic. This phase touches **System Drivers only** (under `shop/`); external-system Drivers are handled by the Contract Test sub-process. Tests stay red — they only go green once the system implementation lands in AT - GREEN - SYSTEM.

The phase decomposes into one creative agent dispatch — **AT - RED - SYSTEM DRIVER - WRITE** — and (rarely) a follow-up **AT - RED - SYSTEM DRIVER - PROTOTYPES** dispatch only when WRITE leaves a compile failure. The typical happy path skips PROTOTYPES because Driver interfaces were settled in AT - RED - DSL. Compile, test runs, change-driven `@Disabled` markup, and the COMMIT are mechanical and run as service tasks in the orchestrator's `red_phase_cycle` sub-flow. The agent must never invoke them.

## What the agent produces

- **AT - RED - SYSTEM DRIVER - WRITE** dispatch: real System Driver implementations under `shop/`. Tests previously disabled with reason `"AT - RED - DSL"` are re-enabled.
- **AT - RED - SYSTEM DRIVER - PROTOTYPES** dispatch (rare): `"TODO: Driver"` prototypes for any newly-referenced Driver method. Reaching this dispatch usually means an interface was missed in AT - RED - DSL — flag it and proceed minimally.

What the orchestrator produces afterward (not the agent's job): the targeted compile, the targeted test run, the change-driven `@Disabled` markup with reason `"AT - RED - SYSTEM DRIVER"`, and the commit `<Ticket> | AT - RED - SYSTEM DRIVER`.

## Conventions

- File scope: only files under `driver-port/` and `driver-adapter/` paths under `shop/` (e.g. `shop/api`, `shop/ui`). Do NOT touch `external/` — that is the Contract Test sub-process.
- Do NOT read or search backend/frontend source code. Model new Driver methods on existing Driver methods in the same file.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`): see [at-cycle-conventions.md](at-cycle-conventions.md). The orchestrator reads the suite from context and runs tests; the agent does not invoke `gh optivem test run`.
- `"TODO: Driver"` prototype syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Definition of System Driver vs External System Driver: see [glossary.md](glossary.md).

## Example

Before — System Driver prototype committed in AT - RED - DSL:

```java
@Override
public RegisterCustomerResponse register(RegisterCustomerRequest request) {
    throw new UnsupportedOperationException("TODO: Driver");
}
```

After — real System Driver wiring the request through the system's HTTP/UI surface (modelled on the sibling `update(...)` method already in this file):

```java
@Override
public RegisterCustomerResponse register(RegisterCustomerRequest request) {
    var response = httpClient.post("/customers", request);
    return response.as(RegisterCustomerResponse.class);
}
```

(The agent does not add `@Disabled` here. The orchestrator marks the change-driven scenarios disabled with reason `"AT - RED - SYSTEM DRIVER"` after the test run, as a service task.)

## AT - RED - SYSTEM DRIVER - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - DSL"`.
2. Implement the System Drivers — replace each `"TODO: Driver"` prototype with actual logic. Stay within `driver-port/` and `driver-adapter/` under `shop/`. Model new methods on existing Driver methods in the same file.
3. Do **not** add `@Disabled` / `Skip` markup. The orchestrator does that after the test run, as a service task.
4. Do **not** attempt to compile, do **not** run tests, do **not** commit. Exit cleanly when the implementation is in place.

## AT - RED - SYSTEM DRIVER - PROTOTYPES

This dispatch only happens when the WRITE dispatch left compile errors — typically a missed Driver method that should have been declared in AT - RED - DSL.

- Add a `"TODO: Driver"` prototype for each missing method (see [language-equivalents.md](../code/language-equivalents.md)). Stay within `shop/`.
- Do not implement real Driver behavior in this dispatch — that's WRITE's job, and a recurring PROTOTYPES dispatch loop indicates a process issue worth flagging.
- Exit cleanly. The orchestrator re-runs the targeted compile after you exit; if it still fails, this dispatch repeats.

## Anti-patterns

- **Editing files under `external/`.** External-system Drivers belong to the Contract Test sub-process (CT - RED - EXTERNAL DRIVER). If a change is needed there, exit this phase and route through CT.
- **Reading backend/frontend source to figure out behaviour.** The Driver speaks to the system's existing surface; behaviour is modelled on sibling Driver methods, not derived from production code. Reading production code in this phase risks coupling test infrastructure to implementation details.
- **Modifying tests or DSL.** Re-enabling previously-disabled tests is the only test-file activity here; DSL is frozen. If the Driver cannot be implemented without DSL or test changes, the previous phase was incomplete — go back, do not patch around it.
- **Adding `@Disabled` markup yourself.** That is the orchestrator's job (`disable_change_driven` service task).
- **Running compile, tests, or commit yourself.** The orchestrator owns those service tasks (`compile_system`, `run_targeted_tests`, `commit_phase`). The agent should never shell out.
- **Leaving "TODO: Driver" behind.** Any remaining System-Driver prototype after WRITE means the phase is not done.

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
