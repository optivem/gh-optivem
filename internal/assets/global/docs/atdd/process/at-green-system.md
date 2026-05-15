# AT - GREEN - SYSTEM

## Purpose

Take all change-driven acceptance tests from RED to GREEN by implementing the system (backend and frontend) — and only the system. Tests, DSL, and Drivers are frozen; if making them pass seems to require touching those layers, an earlier phase was wrong.

## What it produces

- After WRITE: backend implementation, frontend implementation, and the test re-enabling step from WRITE exist together in the working tree.
- Tests in state: every change-driven scenario for the ticket is green. Legacy-coverage scenarios remain green.

## Conventions

- When fixing failing acceptance tests, change only the system implementation — never tests, DSL, or Drivers.
- Legacy-coverage tests live alongside change-driven tests in the same test class (per the ordering rule in [at-red-test.md](at-red-test.md)). Once the cycle is green there is no special handling — they are just tests that pass.
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).

## Example

A representative slice — backend handler and frontend page changed together for a single feature:

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
2. Implement the backend changes. If you cannot make them pass without touching tests / DSL / Drivers, ask the user. Do NOT change tests, DSL, or Drivers to work around it.
3. Implement the frontend changes. If you cannot make them pass without touching tests / DSL / Drivers, ask the user. Do NOT change tests, DSL, or Drivers to work around it.

## Anti-patterns

- **Changing tests, DSL, or Drivers to make tests pass.** Those layers are frozen by the time AT - GREEN - SYSTEM runs. If the system cannot satisfy the tests as written, the AC or the DSL is wrong — surface it to the user instead of patching around it.
- **Skipping the WRITE re-enable step.** The change-driven tests must be re-enabled before the implementation work begins, otherwise the test-runs in WRITE are silently skipping the disabled scenarios.
