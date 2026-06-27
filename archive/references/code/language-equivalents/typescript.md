# Language Equivalents — TypeScript

Per-language slice of the combined ATDD language-equivalents reference,
served to dispatches with `${language}=typescript`. See the
[README](README.md) for the multi-language overview.

## TODO Stubs

| Concept | Syntax |
|---------|--------|
| DSL stub | `throw new Error("TODO: DSL")` |
| Driver stub | `throw new Error("TODO: Driver")` |

## WIP Gate

The acceptance-test-writer prepends a permanent env-var gate to every AT
method. Feature-branch CI, local `npx playwright test`, and IDE runs
leave `GH_OPTIVEM_RUN_WIP_TESTS` unset and silently skip the
work-in-progress test; the ATDD orchestrator sets it to `1` at verify
time to run it. The gate is never removed — no enable/disable step.

| Operation | Syntax |
|-----------|--------|
| WIP gate (first body line) | `test.skip(process.env.GH_OPTIVEM_RUN_WIP_TESTS !== "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");` |

This is Playwright's runtime `test.skip(condition, description)` overload — it skips conditionally at runtime, which is exactly what the env-var gate needs. It is distinct from the definition-time `test.skip(title, body)` overload. No import change.

## String Field Types

"String fields" means the nullable string type:

```typescript
private sku: Optional<string>;
```

The field type is `Optional<string>`.

## DTO Boilerplate

| Layer | Syntax |
|-------|--------|
| Request DTOs | `interface` with optional fields: `field?: Optional<string>` |
| Response DTOs | `interface` with required fields: `field: string` |

## Test File Naming

| Test type | Filename pattern |
|-----------|-----------------|
| Positive | `<UseCase>Positive.spec.ts` |
| Negative | `<UseCase>Negative.spec.ts` |

## Awaitable ShouldSucceed

`ThenSuccessPort extends PromiseLike<void>` — use `await ...shouldSucceed()`.

## Rule Grouping

Scenarios grouped under a Gherkin `Rule:` (see the architecture
[Test File Rules](../../atdd/architecture/test.md) and the runtime `ac-format.md`)
become a `// Rule: <name>` comment block plus a shared test-title prefix — no
native `describe(...)`, composing with the `forChannels(...)` wrapper:

```typescript
// Rule: Shipping is charged at $0.10 per kg per unit
forChannels(ChannelType.UI, ChannelType.API)(() => {
    test('shippingPerKgPerUnit — single item', async ({ scenario }) => { ... });
    test('shippingPerKgPerUnit — scales with quantity', async ({ scenario }) => { ... });
});
```
