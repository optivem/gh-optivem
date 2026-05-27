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

Playwright's `test.skip(title, body)` overload defines a skipped test; the overload has no reason parameter, so the reason rides in a `//`-comment line directly above the test.

| Operation | Syntax |
|-----------|--------|
| Disable a single test | Prepend `// <reason>` above the test, then change `test(...)` to `test.skip(...)`. |
| Re-enable a test | Delete the `// <reason>` line and change `test.skip(...)` back to `test(...)`. |

Do not use the imperative `test.skip(condition, description)` form (that runs inside a test body and skips conditionally at runtime) — the disable/enable agents transform the definition-time form only.

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
