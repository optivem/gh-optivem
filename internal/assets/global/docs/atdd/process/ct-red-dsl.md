# CT - RED - DSL

## Purpose

Replace the `"TODO: DSL"` prototypes left behind by CT - RED - TEST with real DSL logic for the external system, and surface whether the work changes any External System Driver interfaces.

## What it produces

- Commit `<Ticket> | CT - RED - DSL` containing the real DSL implementation, any updated Driver interfaces, and `"TODO: Driver"` prototypes for new/changed Driver methods
- Flag set: `external_system_driver_interface_changed = yes|no`
- Tests in state: contract tests disabled with reason `"CT - RED - DSL"`

## Conventions

- Suite selection (real vs stub): see [ct-cycle-conventions.md](ct-cycle-conventions.md). This phase exercises the stub side only.
- Commit message format: see [ct-cycle-conventions.md](ct-cycle-conventions.md).
- Commit handoff (the wrapping CLI commits, not the agent): see [cycles.md § Commit Handoff](cycles.md#commit-handoff).
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
4. **Add the Driver stubs the new DSL references.** For every new or changed External System Driver method:
   - Update the Driver interface under `external/`.
   - Implement a `"TODO: Driver"` not-implemented prototype (see [language-equivalents.md](../code/language-equivalents.md)). Minimum signature only — no behaviour.
   The result must compile.
5. Determine whether any interface changes affect files under an `external/` package and set `external_system_driver_interface_changed = yes|no`.

## Anti-patterns

- Implementing External System Drivers here — Driver bodies belong in CT - RED - EXTERNAL DRIVER. Only Driver *prototypes* (`"TODO: Driver"`) are added in this phase.
- Leaving `"TODO: DSL"` strings behind in the committed code — every prototype must be replaced with real logic.
- Editing files outside `external/` to "fix" failing contract tests — the contract is between the system and the external boundary, not internal code.
