You are the Driver Agent. This is a one-shot dispatch — investigate, do the work, commit, and exit.

Ticket: #${issue_num} "${issue_title}" (${issue_repo})
Project: ${project_title} (${project_url})
Phase: ${phase}
Phase doc: ${phase_doc}

When the work is done, do not commit and do not summarise — exit cleanly. The CLI will stage and commit your changes after you exit. The agent must never run `git commit`, `git add`, or `gh issue close`.

---

You are the Driver Agent. Follow the phase specified in the input:

- **AT - RED - SYSTEM DRIVER - WRITE** (always falling through to the **AT - RED - SYSTEM DRIVER - REVIEW** STOP) or **AT - RED - SYSTEM DRIVER - COMMIT** — from `at-red-system-driver.md`
- **CT - RED - EXTERNAL DRIVER - WRITE** (falling through to **CT - RED - EXTERNAL DRIVER - REVIEW** STOP) or **CT - RED - EXTERNAL DRIVER - COMMIT** — from `ct-red-external-driver.md`

Apply Driver Port Rules from `driver-port.md`.

Report back exactly as the phase requires. After WRITE, fall through to REVIEW and STOP for human approval.

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

### Reference: docs/atdd/process/ct-cycle-conventions.md

# CT Cycle Conventions

_The Contract Test sub-process is only triggered when the AT cycle's DSL phase reports **external system interfaces changed = yes** — i.e. new methods were added to interfaces under `external/` (e.g. `driver-port/.../external/erp`). It is initiated by the orchestrator as defined in `cycles.md`._

_Before entering CT - RED - TEST, the orchestrator runs the External System Onboarding Sub-Process (see `cycles.md`) to ensure an External System Driver and accessible Test Instance exist. If the Driver already exists, Onboarding returns immediately; otherwise it provisions a dockerized stand-in (json-server pattern, see `system/external-real-sim`), defines a minimal Driver interface and implementation, and proves it works with a single Smoke Test._

## Suite Selection

Each contract test runs against two parallel suites — the real external-system Test Instance and the dockerized stub. Use the matching suite placeholder throughout all CT phases:

- `<suite-contract-real>` — the contract suite executed against the **real external system** (Test Instance).
- `<suite-contract-stub>` — the contract suite executed against the **dockerized stub** External System.

A CT phase that names only one of these placeholders is exercising one side of the real-vs-stub contract pair; CT - RED - TEST runs both, CT - RED - DSL and CT - GREEN - STUBS run the stub side only.

After a driver-adapter change in CT - RED - EXTERNAL DRIVER - WRITE, the orchestrator (not the agent) runs a **targeted subset** of `<suite-contract-stub>` — the tests that traverse the changed adapter methods — after the agent exits. If any changed adapter method cannot be statically traced to a test, the orchestrator falls back to running the full stub suite for safety.

## Commit Message Format

Every commit message follows the pattern: `<Ticket> | <Phase>`.

The unit of work in the CT sub-process is a **ticket**, not an individual scenario — CT is entered once per ticket when AT's DSL phase reports external system interfaces changed = yes, and all CT phases run as a single per-ticket pass (mirroring the AT Cycle batching rule). Commit messages reflect the ticket title.

If a GitHub issue number was provided as input, prefix every commit message with `#<issue-number> | `. Example: `#42 | Register Customer | CT - RED - TEST`.

**Important:** The phase suffix in the message is the phase *prefix only* (e.g. `CT - RED - TEST`). Do **NOT** append `- WRITE`, `- REVIEW`, or `- COMMIT` to the phase in the commit message — those suffixes identify the section header only, not the commit message. (REVIEW is a STOP-only phase that produces no commit.)

---

### Reference: docs/atdd/process/at-red-system-driver.md

# AT - RED - SYSTEM DRIVER

## Purpose

Replace the System-Driver "TODO: Driver" prototypes from AT - RED - DSL with real Driver logic. This phase touches **System Drivers only** (under `shop/`); external-system Drivers are handled by the Contract Test sub-process. Tests stay red — they only go green once the system implementation lands in AT - GREEN - SYSTEM.

## What it produces

- Commit `<Ticket> | AT - RED - SYSTEM DRIVER` containing real System Driver implementations under `shop/`.
- Tests in state: change-driven scenarios disabled with reason `"AT - RED - SYSTEM DRIVER"`; legacy-acceptance-criteria scenarios still enabled and passing.

## Conventions

- File scope: only files under `driver-port/` and `driver-adapter/` paths under `shop/` (e.g. `shop/api`, `shop/ui`). Do NOT touch `external/` — that is the Contract Test sub-process.
- Do NOT read or search backend/frontend source code. Model new Driver methods on existing Driver methods in the same file.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`) and commit-message format: see [at-cycle-conventions.md](at-cycle-conventions.md).
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Definition of System Driver vs External System Driver: see [glossary.md](glossary.md).
- Commit confirmation gate: see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- STOP semantics at REVIEW: see [shared-phase-progression.md](shared-phase-progression.md).

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

## AT - RED - SYSTEM DRIVER - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - DSL"`.
2. Implement the System Drivers — replace each "TODO: Driver" prototype with actual logic. Stay within `driver-port/` and `driver-adapter/` under `shop/`. Model new methods on existing Driver methods in the same file.
3. Do NOT run the tests yourself. The orchestrator runs a targeted subset of `<acceptance-api>` / `<acceptance-ui>` after you exit, based on which adapter methods you changed; an unmapped change triggers a full-suite fallback. Exit cleanly when the implementation is in place.

## AT - RED - SYSTEM DRIVER - REVIEW (STOP)

STOP. Present the Driver implementation to the user and ask for approval. Do NOT continue.

**Review checklist:**
- All edits live under `driver-port/` and `driver-adapter/` paths under `shop/`. Nothing under `external/`.
- "TODO: Driver" is gone for every System Driver method affected by this ticket.
- New methods follow the shape of existing methods in the same file — no novel patterns invented from backend/frontend source.
- No test, DSL, system, or external-driver edits.

## AT - RED - SYSTEM DRIVER - COMMIT

1. Mark the tests as disabled with reason `"AT - RED - SYSTEM DRIVER"` (see [language-equivalents.md](../code/language-equivalents.md)).
2. Ensure no test files are (accidentally) in the list of changed files.
3. COMMIT with message `<Ticket> | AT - RED - SYSTEM DRIVER`.

## Anti-patterns

- **Editing files under `external/`.** External-system Drivers belong to the Contract Test sub-process (CT - RED - EXTERNAL DRIVER). If a change is needed there, exit this phase and route through CT.
- **Reading backend/frontend source to figure out behaviour.** The Driver speaks to the system's existing surface; behaviour is modelled on sibling Driver methods, not derived from production code. Reading production code in this phase risks coupling test infrastructure to implementation details.
- **Modifying tests or DSL.** Tests are disabled/enabled here, nothing more; DSL is frozen. If the Driver cannot be implemented without DSL or test changes, the previous phase was incomplete — go back, do not patch around it.
- **Leaving "TODO: Driver" behind.** Any remaining System-Driver prototype means the phase is not done.

---

### Reference: docs/atdd/process/ct-red-external-driver.md

# CT - RED - EXTERNAL DRIVER

## Purpose

Replace the `"TODO: Driver"` prototypes left behind by CT - RED - DSL with real External System Driver logic. The contract tests are still expected to fail at the end of this phase — the dockerized stub does not yet honor the new contract; that's CT - GREEN - STUBS.

## What it produces

- Commit `<Ticket> | CT - RED - EXTERNAL DRIVER` containing real External System Driver implementations
- Tests in state: contract tests disabled with reason `"CT - RED - EXTERNAL DRIVER"`
- GitHub issue comment summarising Driver interface changes (if an issue number was provided as input)

## Conventions

- Scope is strictly limited to files under `external/` (e.g. `driver-port/.../external/...`, `driver-adapter/.../external/...`). Files under `shop/` are off-limits in this phase. See [glossary.md](glossary.md).
- Suite selection (real vs stub): see [ct-cycle-conventions.md](ct-cycle-conventions.md). This phase exercises the stub side only.
- Commit message format: see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit gate ("Can I commit?"): see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- Phase progression and STOP semantics: see [shared-phase-progression.md](shared-phase-progression.md).
- `@Disabled` syntax per language: see [language-equivalents.md](../code/language-equivalents.md).

## Example

Replace the `"TODO: Driver"` prototype with a real HTTP call to the external system. The Driver translates between the DSL's needs and the external API's wire shape.

```diff
 public PromotionResponse getPromotion() {
-    throw new UnsupportedOperationException("TODO: Driver");
+    HttpResponse<String> response = httpClient.send(
+        HttpRequest.newBuilder()
+            .uri(URI.create(baseUrl + "/erp/api/promotion"))
+            .GET()
+            .build(),
+        BodyHandlers.ofString());
+    return objectMapper.readValue(response.body(), PromotionResponse.class);
 }
```

## CT - RED - EXTERNAL DRIVER - WRITE

1. Enable the tests marked disabled with reason `"CT - RED - DSL"`.
2. Implement the External System Drivers — replace each `"TODO: Driver"` prototype with actual logic.
   - Only edit files under `external/` (driver-port and driver-adapter).
   - Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract.
3. Do NOT run the tests yourself. The orchestrator runs a targeted subset of `<suite-contract-stub>` after you exit, based on which adapter methods you changed; an unmapped change triggers a full-suite fallback. Exit cleanly when the implementation is in place.

## CT - RED - EXTERNAL DRIVER - REVIEW (STOP)

STOP. Present the Driver implementation to the user and ask for approval. Do NOT continue.

**Review checklist:**

- All changes are confined to files under `external/` — nothing under `shop/` was touched.
- No `"TODO: Driver"` strings remain.
- Tests fail with a runtime error against `<suite-contract-stub>` (still RED — that's expected).

## CT - RED - EXTERNAL DRIVER - COMMIT

1. Mark the tests as disabled with reason `"CT - RED - EXTERNAL DRIVER"` (see [language-equivalents.md](../code/language-equivalents.md)).
2. COMMIT with message `<Ticket> | CT - RED - EXTERNAL DRIVER`.
3. If a GitHub issue number was provided as input, post a comment on the issue summarising the Driver interface changes (new methods added, interfaces updated).

## Anti-patterns

- Editing files under `shop/` — those belong to System Drivers and the AT cycle, not the External System Driver phase.
- Reading external-system source code to figure out behavior — Drivers are written against the *contract* expressed by the contract tests and the published API, not against internal implementation details.
- Expecting the contract tests to pass at the end of this phase — they should still fail. The stub becomes contract-compatible in CT - GREEN - STUBS.
- Skipping the issue comment when an issue number was provided — it's the audit trail of the Driver change.

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
