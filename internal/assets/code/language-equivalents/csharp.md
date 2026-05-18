# Language Equivalents — C# (.NET)

Per-language slice of the combined ATDD language-equivalents reference,
served to dispatches with `${language}=csharp`. See the
[README](README.md) for the multi-language overview.

## TODO Stubs

| Concept | Syntax |
|---------|--------|
| DSL stub | `throw new NotImplementedException("TODO: DSL")` |
| Driver stub | `throw new NotImplementedException("TODO: Driver")` |

## Test Disabling

| Operation | Syntax |
|-----------|--------|
| Disable a single test | `[Fact(Skip = "reason")]` or `[Theory(Skip = "reason")]` |
| Re-enable a test | Remove `Skip = "reason"` |

## String Field Types

"String fields" means the nullable string type:

```csharp
public string? Sku { get; set; }
```

The field type is `string?`.

## DTO Boilerplate

| Layer | Syntax |
|-------|--------|
| Request DTOs | Auto-properties: `public string? Field { get; set; }` |
| Response DTOs | `required` modifier for non-nullable IDs: `public required string Id { get; set; }` |

## Test File Naming

| Test type | Filename pattern |
|-----------|-----------------|
| Positive | `<UseCase>PositiveTest.cs` |
| Negative | `<UseCase>NegativeTest.cs` |

## Awaitable ShouldSucceed

`IThenSuccess` implements `GetAwaiter()` — use `await ...ShouldSucceed()`.
