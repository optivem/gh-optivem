You are the Task Agent. This is a one-shot dispatch — investigate, do the work, commit, and exit.

Ticket: #${issue_num} "${issue_title}" (${issue_repo})
Project: ${project_title} (${project_url})
Phase: ${phase}
Phase doc: ${phase_doc}
Scope: Architecture=${architecture}, System Lang=${system_lang}, Test Lang=${test_lang}

When the work is done, do not commit and do not summarise — exit cleanly. The CLI will stage and commit your changes after you exit. The agent must never run `git commit`, `git add`, or `gh issue close`.

---

You are the Task Agent. The input is a GitHub issue number (e.g. `#59`); the structural subtype is on the `subtype:*` label (one of `subtype:system-interface-redesign` or `subtype:external-system-interface-redesign` for the two `da_cycle` paths). **Fetch the issue with `gh` before proceeding** — do not rely on the caller to restate the title, body, labels, or checklist:

```bash
gh issue view <number> --repo <owner>/<repo> --json number,title,body,labels,projectItems,state
```

The subtype determines whether you are reshaping a system-side driver or an external-system driver:

- **`system-interface-redesign`** — one of the system's own driver adapters (API, UI, mobile, CLI, admin, ...). Read the ticket body's Checklist plus the system tree to determine which driver(s) to modify; do not assume API or UI.
- **`external-system-interface-redesign`** — an external service the shop depends on (e.g. ERP, tax, clock). Routes through the Contract Test Sub-Process.

Implement the change and adapt the relevant driver **implementation** so existing acceptance and contract tests keep passing. Apply Driver Port Rules from `driver-port.md` and Driver Adapter Rules from `driver-adapter.md`.

## Scope

Your input prompt includes a `Scope:` block of the form `Scope: Architecture=<value>, System Lang=<value>, Test Lang=<value>`. Restrict ALL file edits, residual-reference greps, and per-language work to paths that match the in-scope architecture(s) and system language(s). Do NOT modify out-of-scope implementations.

## Process

1. Identify the layer that is changing and the driver(s) that wrap it. Read the ticket Checklist and the system tree to decide; the framework no longer pre-classifies the channel:
   - UX/UI change → shop UI driver under `driver-adapter/.../shop/ui` (page objects, selectors, navigation, page state).
   - System API change → shop API driver under `driver-adapter/.../shop/api` (controllers, request/response mapping, `SystemErrorMapper`).
   - Mobile / CLI / admin / other channel → the matching driver folder under `driver-adapter/.../shop/<channel>`.
   - External system change → external driver under `driver-adapter/.../external/<system>` (`XyzRealDriver`, `XyzStubDriver`, `BaseXyzClient`, `Ext*` DTOs).

2. Implement the system change (frontend, backend, or external-system contract / stub configuration).

3. Adapt the driver implementation(s) to match. Keep behaviour observable through the **existing** driver interface — absorb the change inside the adapter (selectors, mappers, client methods, DTO conversions).

4. **Driver interface guardrail.** Do NOT modify any file under `driver-port/`. If you believe an interface change is unavoidable, STOP and present to the user:
   - The driver interface method(s) you want to change and why the adapter alone cannot absorb the change.
   - Whether the change is in `external/` (contract tests will need updating — see `glossary.md` for *interface change*) or `shop/` (no contract tests needed).
   - The proposed new signature(s).
   Wait for explicit user approval before editing any `driver-port/` file.

5. Do NOT run any test or compile commands yourself — not `gh optivem test/run/stop system`, and not local compile commands like `./compile-all.sh`, `./gradlew build`, `npx tsc --noEmit`, or `dotnet build`. After WRITE, STOP. Present the system + driver changes for human approval. Do NOT continue.

6. Report back:
   - Files changed (grouped by layer: system code, driver-adapter, driver-port if approved), restricted to the in-scope architecture(s) and system language(s).
   - Any driver interface change that was approved, with the reason.
   - Out-of-scope implementations deliberately left untouched.

---

## References

<!-- legacy-block:shared-commit-confirmation -->

### Reference: docs/atdd/architecture/system.md

# System Code Layout

The `system/` tree holds the production code under test. It is organised by **deployment shape** first, then by **language**.

## Parallel Implementations

The shop has parallel implementations across three languages. CI runs every one of them; a change to the System API or System UI must be applied to every implementation that exposes that layer.

| Deployment shape | Java | .NET | TypeScript |
| --- | --- | --- | --- |
| Monolith (backend + UI in one process) | `system/monolith/java/` | `system/monolith/dotnet/` | `system/monolith/typescript/` |
| Multitier — backend (HTTP API only) | `system/multitier/backend-java/` | `system/multitier/backend-dotnet/` | `system/multitier/backend-typescript/` |
| Multitier — frontend (UI only) | (uses `frontend-react`) | (uses `frontend-react`) | `system/multitier/frontend-react/` |

The multitier frontend is a single React app shared across all three backend languages. The monolith has three distinct UI implementations, one per language.

## Where System API Surface Lives

A System API endpoint is referenced in up to three places per implementation. When the surface changes (rename, signature, status codes), every reference site must be updated.

1. **Backend route** — the controller declares the route:
   - Java (Spring): `@GetMapping`/`@PostMapping`/etc. on the controller method.
   - .NET (ASP.NET Core): `[HttpGet]`/`[HttpPost]`/etc. on the action method, optionally with a class-level `[Route]`.
   - TypeScript multitier (NestJS): `@Get`/`@Post`/etc. on the controller method, optionally with a class-level `@Controller`.
   - TypeScript monolith (Next.js): the route is determined by the **filesystem path** under `src/app/api/<path>/route.ts`. Renaming the URL means moving the directory — editing the file in place is not sufficient.
2. **UI fetch sites** — any page or service that calls the endpoint. In the monolith this is the per-language UI (Spring Thymeleaf templates, ASP.NET Razor pages, Next.js page components). In the multitier setup it is the shared `frontend-react` app's service layer.
3. **Driver adapter constant** — the endpoint URL is encoded as a constant inside the matching resource controller in `system-test/.../driver/adapter/myShop/api/client/controllers/`. Updating the constant is enough; the driver port interface stays untouched. See `driver-adapter.md`.

After editing the source of truth (the backend route), grep the system tree for residual references — UI fetch sites are easy to miss because they live in a different language and folder than the controller.

## Where System UI Surface Lives

System UI surface (page structure, form fields, navigation, copy, selectors) lives in the per-shape, per-language UI code:

- Monolith: `system/monolith/<lang>/` — Thymeleaf templates (Java), Razor pages (.NET), Next.js page components (TypeScript).
- Multitier: `system/multitier/frontend-react/`.

The matching driver adapter is the UI driver under `system-test/.../driver/adapter/myShop/ui/`. See `driver-adapter.md` § Shop UI Driver for the page-object conventions.

## Read-only Areas

`system-test/<lang>/.../Legacy/` is read-only course-reference material. It may reference older API or UI surface; leave it untouched even when a redesign breaks its references — it is not part of the latest test suite and is not run by CI's Acceptance Stage.

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

### Reference: docs/atdd/architecture/driver-adapter.md

# Driver Adapter Rules

## Real vs Stub Implementations

Each external system driver has two implementations:

- **`XyzRealDriver`** — connects to the real service via HTTP.
- **`XyzStubDriver`** — configures WireMock stubs to simulate the service.

Both implementations share a `BaseXyzDriver` that delegates to a `BaseXyzClient`. The client is also split:

- **`XyzRealClient`** — extends `BaseXyzClient` with methods that make real HTTP calls (e.g. `createProduct()`).
- **`XyzStubClient`** — extends `BaseXyzClient` with methods that register WireMock stubs (e.g. `configureGetProduct()`).

## External DTOs

All DTOs used by driver adapters to communicate with external systems use an `Ext*` prefix (e.g. `ExtCreateProductRequest`, `ExtProductDetailsResponse`, `ExtErpErrorResponse`).

- `Ext*Request` DTOs must use only string fields — never numeric, boolean, or other non-string types. This allows invalid values to pass through for negative test scenarios. Type conversion happens inside the HTTP client or serialization layer. See `language-equivalents.md` for the string field type and DTO boilerplate per language.
- `Ext*Response` DTOs may use typed fields (e.g. `BigDecimal`, `decimal`, `Decimal`) since they are only used for deserialization, not for constructing negative test inputs.

## goTo*() Methods

`goTo*()` methods (e.g. `goToShop()`, `goToErp()`) are health checks that verify the system is accessible. They must be called before any other driver methods in the Assume stage.

## Shop API Driver

The shop API driver uses a controller-per-resource pattern. `ShopApiClient` composes multiple controllers (e.g. `OrderController`, `CouponController`, `ProductController`), each managing one API endpoint group.

Endpoint URLs are encoded as **constants** inside each resource controller. When a system route is renamed, update the constant — that is the one place in the test layer that needs to change. The driver port interface stays untouched, so existing acceptance and contract tests keep compiling.

Not every system endpoint has a method on the API driver. Some operations are exercised through the UI driver only, so a rename of those endpoints is absorbed entirely by the system-side fetch URLs and never reaches the API adapter. Check both drivers when scoping a system-side change.

Error responses from the shop API (e.g. `ProblemDetailResponse`) are mapped to the domain `ErrorResponse` via a `SystemErrorMapper`. Never expose API-specific error formats beyond the adapter layer.

## Shop UI Driver

- UI drivers must never navigate directly to a URL. Always simulate real user behaviour by starting from the home page and clicking through the UI.
- Page objects use `aria-label` selectors for inputs and interactive elements.
- Page objects read operation results from notification elements (`[role='alert']` with `.notification.success` or `.notification.error` classes).
- The UI driver manages an internal page state enum to avoid redundant navigation.

---

### Reference: docs/atdd/process/glossary.md

# Glossary

## Behavioral Change

A **behavioral change** is a change defined by **change-driven acceptance criteria** — the new (or restored) behavior IS specified by the AC scenarios produced from the ticket. Stories (`atdd-story`, new behavior) and bugs (`atdd-bug`, restored behavior) are behavioral; their change-driven AC route to the **AT Cycle** (test-first / ATDD). The unit of work in the AT Cycle is the **ticket** — all change-driven scenarios for the ticket are batched through each phase together, with no per-scenario inner loop.

Note: a behavioral-change ticket may *also* include a Legacy Acceptance Criteria section; that's orthogonal — see [Legacy Acceptance Criteria](#legacy-acceptance-criteria) below.

## Structural Change

All four structural cycles are governed by the rule **existing AC must stay green** locally before the final ticket commit. The sample suite runs locally as part of the COMMIT step. CI's **Acceptance Stage** is the post-commit verifier but it is **human-watched, not agent-watched** — see [`shared-ticket-status-in-acceptance.md`](shared-ticket-status-in-acceptance.md). Agents are CI-unaware and never advance a ticket past **TICKET STATUS - IN ACCEPTANCE**.

A **structural change** is a change that produces **no change-driven acceptance criteria**. The three task subtypes (`atdd-task-system-api`, `atdd-task-system-ui`, `atdd-task-external-api`, each an interface change at a single system boundary) and chores (`atdd-chore`, internal-only change) are structural. The structural change still flows through a cycle — each task subtype enters its dedicated cycle (the **System API Task Cycle**, **System UI Task Cycle**, or **External API Task Cycle**), and chores enter the **Chore Cycle** — but each cycle has no RED/GREEN per scenario; instead it consists of implementation, STOP - HUMAN REVIEW, and COMMIT. All four structural cycles end by ticking the ticket's checklist of structural change items and moving the issue to **TICKET STATUS - IN ACCEPTANCE**. The External API Task Cycle has no standalone STOP/COMMIT — those happen inside the Contract Test Sub-Process it wraps.

Note: a structural-change ticket may *also* include a Legacy Acceptance Criteria section; that's orthogonal — see [Legacy Acceptance Criteria](#legacy-acceptance-criteria) below.

## Legacy Acceptance Criteria

**Legacy Acceptance Criteria** is orthogonal to behavioral/structural classification. It is a **section in the ticket schema**, optional on any ticket type (story, bug, system-api-task, system-ui-task, external-api-task, or chore). The section lists retroactive AC scenarios for previously uncovered functionality the change touches.

Legacy Acceptance Criteria uses the **test-last** approach: tests are written retroactively for already-built behavior, and they should pass on first run because the behavior already exists. **This is NOT ATDD** — there is no Red → Green per scenario. A ticket whose schema carries a Legacy Acceptance Criteria section routes through the **Legacy Acceptance Criteria Cycle**, regardless of ticket type.

When a ticket carries both a change-driven payload (story/bug AC, or a structural change from any of the three task subtypes or a chore) *and* a Legacy Acceptance Criteria section, the Legacy Acceptance Criteria Cycle runs first, then the AT Cycle (if applicable) — fill the coverage gap before piling new behavior on top.

## Interface Change

An **interface change** is any modification to a public contract between layers. This includes:

- Adding, removing, or renaming interface methods
- Changing method signatures (parameters, return types)
- Adding, removing, or renaming fields in request or response DTOs associated with those methods

This definition applies uniformly to DSL port interfaces, Driver port interfaces, and external system interfaces.

In the intake-classification sense, an **interface change** is specifically a change at the **system boundary** — system API, system UI, or external system API. The three task subtypes (`atdd-task-system-api`, `atdd-task-system-ui`, `atdd-task-external-api`) each cover exactly one of these three boundaries. Driver *implementations* update to match the new interface; driver *interfaces* stay the same so existing acceptance tests still pass through them. Single-driver scope is enforced at ticket-creation time — multi-boundary work is split into multiple coordinated tickets.

**Why it matters for the ATDD pipeline:**
- A DSL interface change → update DSL port and implementation
- A Driver interface change → update driver port and adapters
- An external system interface change (any change under `driver-port/.../external/`) → triggers the contract test subprocess (see `ct-cycle-conventions.md` and the `ct-*.md` per-phase docs)
- A System API change classified as `atdd-task-system-api` → routes to the **System API Task Cycle** (update System API Driver; STOP - HUMAN REVIEW → COMMIT → TICKET STATUS - IN ACCEPTANCE). Single-driver scope.
- A System UI change classified as `atdd-task-system-ui` → routes to the **System UI Task Cycle** (update System UI Driver; STOP - HUMAN REVIEW → COMMIT → TICKET STATUS - IN ACCEPTANCE). Single-driver scope.
- An External System API change classified as `atdd-task-external-api` → routes to the **External API Task Cycle**, which wraps the Contract Test Sub-Process (per-phase RED/GREEN inside CT, four-commit sequence; the External API Task Cycle itself has no standalone STOP/COMMIT). Single-driver scope.

For all three task subtypes, driver bodies adapt to the new boundary interface (see [Structural Change](#structural-change) above for the existing-AC / Acceptance-Stage rule). If the ticket additionally carries a Legacy Acceptance Criteria section, the Legacy Acceptance Criteria Cycle runs first.

## Internal-only Change

An **internal-only change** is a change inside the system that does not modify any boundary — no system API, system UI, or external system API change. Examples: refactor a class, rename, dependency upgrade. Drivers are untouched. Internal-only changes are classified as `atdd-chore`; they route to the **Chore Cycle** (Implement → STOP - HUMAN REVIEW → COMMIT → TICKET STATUS - IN ACCEPTANCE). See [Structural Change](#structural-change) above for the existing-AC rule. If the ticket additionally carries a Legacy Acceptance Criteria section, the Legacy Acceptance Criteria Cycle runs first.

## Legacy Acceptance Criteria Cycle

The **Legacy Acceptance Criteria Cycle** is the **test-last retroactive-AC cycle**. It is reachable from any ticket type (`atdd-story`, `atdd-bug`, `atdd-task-system-api`, `atdd-task-system-ui`, `atdd-task-external-api`, `atdd-chore`) whose ticket carries a [Legacy Acceptance Criteria](#legacy-acceptance-criteria) section. Because the behavior already exists, the retroactive acceptance tests written in this cycle should pass on first run; this is **not ATDD** (no Red → Green per scenario).

Task tickets enter the matching task cycle — `system-api-task` → **System API Task Cycle**, `system-ui-task` → **System UI Task Cycle**, `external-api-task` → **External API Task Cycle** — and chore tickets enter the **Chore Cycle**. All four are structural cycles with no RED/GREEN per scenario (see [Structural Change](#structural-change) above for the existing-AC rule). All four cycles' phases are now defined; see `cycles.md` and `diagram-process.md` for the full flows. The Legacy Acceptance Criteria Cycle's own internal phases are TBD.

## Ticket Status - In Acceptance

The maximum ticket status any agent ever sets. After the **final commit of a ticket** (whichever phase produces it, in any cycle), the agent ticks any checklist items completed by the work and moves the ticket to **IN ACCEPTANCE**. The agent is then done. Pipeline-watching, fix-loops on red CI, and the move from IN ACCEPTANCE to DONE are human responsibilities — agents are CI-unaware. See [`shared-ticket-status-in-acceptance.md`](shared-ticket-status-in-acceptance.md) for the canonical procedure.

## `shop/` Package vs `shop` Repository

ATDD content uses the word **shop** in two distinct ways. They look similar but mean different things:

- **`shop/` (with trailing slash)** — a package/folder convention inside the testkit's driver layer (e.g. `driver-port/.../shop/api`, `driver-adapter/.../shop/ui`). This is the SUT-internal driver namespace, paired with `external/` (drivers for external systems). The name is part of ATDD doctrine and is **not** the repo name. Do not rename it.
- **`shop` (no slash, used in repo context)** — the repository name of the system under test.

The two uses are kept textually distinct (slash vs. no-slash) so they can be reasoned about independently.

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
