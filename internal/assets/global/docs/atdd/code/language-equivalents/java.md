# Language Equivalents — Java

Per-language slice of the combined ATDD language-equivalents reference,
served to dispatches with `${language}=java`. See the
[README](README.md) for the multi-language overview.

## TODO Stubs

| Concept | Syntax |
|---------|--------|
| DSL stub | `throw new UnsupportedOperationException("TODO: DSL")` |
| Driver stub | `throw new UnsupportedOperationException("TODO: Driver")` |

## Test Disabling

| Operation | Syntax |
|-----------|--------|
| Disable a single test | `@Disabled("reason")` |
| Re-enable a test | Remove `@Disabled` |

## String Field Types

"String fields" means the nullable string type:

```java
private String sku;
```

The field type is `String`.

## DTO Boilerplate

| Layer | Syntax |
|-------|--------|
| Request DTOs | Lombok: `@Data @Builder @NoArgsConstructor @AllArgsConstructor` |
| Response DTOs | Same |

## Test File Naming

| Test type | Filename pattern |
|-----------|-----------------|
| Positive | `<UseCase>PositiveTest.java` |
| Negative | `<UseCase>NegativeTest.java` |

## Awaitable ShouldSucceed

Synchronous — no `await` needed.
