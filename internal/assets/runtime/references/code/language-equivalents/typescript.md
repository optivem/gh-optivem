# Language Equivalents — TypeScript

Per-language slice of the combined ATDD language-equivalents reference,
served to dispatches with `${language}=typescript`. See the
[README](README.md) for the multi-language overview.

## TODO Stubs

| Concept | Syntax |
|---------|--------|
| DSL stub | `throw new Error("TODO: DSL")` |
| Driver stub | `throw new Error("TODO: Driver")` |

## Test Disabling

| Operation | Syntax |
|-----------|--------|
| Disable a single test | `test.skip(true, "reason")` |
| Re-enable a test | Remove `test.skip(...)` |

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
