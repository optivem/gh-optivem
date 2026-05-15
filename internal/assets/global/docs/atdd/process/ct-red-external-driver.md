# CT - RED - EXTERNAL DRIVER

## Purpose

Replace the `"TODO: Driver"` prototypes left behind by CT - RED - DSL with real External System Driver logic. The contract tests are still expected to fail at the end of this phase — the dockerized stub does not yet honor the new contract; that's CT - GREEN - STUBS.

## What it produces

- After WRITE: real External System Driver implementations exist in the working tree.
- Tests in state: contract tests disabled with reason `"CT - RED - EXTERNAL DRIVER"`

## Conventions

- Scope is strictly limited to files under `${external_driver_port}/` and `${external_driver_adapter}/`. All driver code lives in the test tree, not in `system/`. Files under the sibling `${sut_namespace}/` directories are off-limits in this phase. See [glossary.md](glossary.md).
- `@Disabled` syntax per language: see [language-equivalents/](../code/language-equivalents/README.md).

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
   - Only edit files under `${external_driver_port}/` and `${external_driver_adapter}/`.
   - Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract.
3. If your impl references a Driver method under `external/` that doesn't yet have a prototype, add the `"TODO: Driver"` stub in the same step (rare — reaching this usually means an interface was missed in CT - RED - DSL).

The result must compile.

## Anti-patterns

- Editing files under the sibling `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/` directories — those belong to System Drivers and the AT cycle, not the External System Driver phase.
- Reading external-system source code to figure out behavior — Drivers are written against the *contract* expressed by the contract tests and the published API, not against internal implementation details.
- Expecting the contract tests to pass at the end of this phase — they should still fail. The stub becomes contract-compatible in CT - GREEN - STUBS.
