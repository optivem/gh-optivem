# AT - GREEN - SYSTEM

## Purpose

Take all change-driven acceptance tests from RED to GREEN by implementing the system (backend and frontend) — and only the system. Tests, DSL, and Drivers are frozen; if making them pass seems to require touching those layers, an earlier phase was wrong.

## What it produces

- Commit `<Ticket> | AT - GREEN - SYSTEM` containing backend implementation, frontend implementation, and the test re-enabling step from WRITE — all in a single commit.
- Tests in state: every change-driven scenario for the ticket is green. Legacy-coverage scenarios remain green.
- Issue moved to **TICKET STATUS - IN ACCEPTANCE** with the ticket's checklist items ticked.

## Conventions

- Backend and frontend ship in **one** commit. The agent has full-stack access; there is no per-layer commit split.
- When fixing failing acceptance tests, change only the system implementation — never tests, DSL, or Drivers.
- Legacy-coverage tests live alongside change-driven tests in the same test class (per the ordering rule in [at-red-test.md](at-red-test.md)). Once the cycle is green there is no special handling — they are just tests that pass.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`) and commit-message format: see [at-cycle-conventions.md](at-cycle-conventions.md).
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Commit handoff (the wrapping CLI commits, not the agent): see [cycles.md § Commit Handoff](cycles.md#commit-handoff).
- STOP semantics at REVIEW: see [shared-phase-progression.md](shared-phase-progression.md).
- Moving the ticket to IN ACCEPTANCE: see [shared-ticket-status-in-acceptance.md](shared-ticket-status-in-acceptance.md).

## Example

A representative slice — backend handler and frontend page changed together for a single feature — committed as one unit:

```diff
 // backend: shop/api/.../CustomerController.java
+@PostMapping("/customers")
+public ResponseEntity<RegisterCustomerResponse> register(@RequestBody RegisterCustomerRequest req) {
+    var result = registerCustomer.handle(req);
+    return ResponseEntity.ok(new RegisterCustomerResponse(result.id()));
+}

 // frontend: shop/ui/.../register-customer.page.tsx
+export function RegisterCustomerPage() {
+  const onSubmit = async (form) => {
+    await api.post("/customers", form);
+    navigate("/customers");
+  };
+  return <CustomerForm onSubmit={onSubmit} />;
+}
```

## AT - GREEN - SYSTEM - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - SYSTEM DRIVER"`. (This is the only "remove disabled annotation" step in this phase.)
2. Implement the backend:
   a. Implement the backend changes.
   b. Rebuild the SUT image, restart the running container, then run acceptance tests for the API channel:
      ```bash
      gh optivem system build --rebuild
      gh optivem system start --restart
      gh optivem test run --suite <acceptance-api> --test <TestMethodName>
      ```
   c. If tests fail, fix the backend until the tests pass.
   d. If you cannot get the tests to pass, ask the user. Do NOT change tests, DSL, or Drivers to work around it.
3. Implement the frontend:
   a. Implement the frontend changes.
   b. Rebuild the SUT image, restart the running container, then run acceptance tests for the UI channel:
      ```bash
      gh optivem system build --rebuild
      gh optivem system start --restart
      gh optivem test run --suite <acceptance-ui> --test <TestMethodName>
      ```
   c. If tests fail, fix the frontend until the tests pass.
   d. If you cannot get the tests to pass, ask the user. Do NOT change tests, DSL, or Drivers to work around it.
4. By now, all change-driven acceptance tests for the ticket are passing, and all legacy-coverage tests remain green.

## Anti-patterns

- **Changing tests, DSL, or Drivers to make tests pass.** Those layers are frozen by the time AT - GREEN - SYSTEM runs. If the system cannot satisfy the tests as written, the AC or the DSL is wrong — surface it to the user instead of patching around it.
- **Splitting backend and frontend into separate commits.** Both ship together as `<Ticket> | AT - GREEN - SYSTEM`. The AT cycle's terminal commit is full-stack.
- **Skipping the WRITE re-enable step.** The change-driven tests must be re-enabled before the implementation work begins, otherwise the test-runs in WRITE are silently skipping the disabled scenarios.
