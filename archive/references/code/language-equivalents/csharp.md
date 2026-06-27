# Language Equivalents — C# (.NET)

Per-language slice of the combined ATDD language-equivalents reference,
served to dispatches with `${language}=csharp`. See the
[README](README.md) for the multi-language overview.

## TODO Stubs

| Concept | Syntax |
|---------|--------|
| DSL stub | `throw new NotImplementedException("TODO: DSL")` |
| Driver stub | `throw new NotImplementedException("TODO: Driver")` |

## WIP Gate

The acceptance-test-writer prepends a permanent env-var gate to every AT
method. Feature-branch CI, local `dotnet test`, and IDE runs leave
`GH_OPTIVEM_RUN_WIP_TESTS` unset and silently skip the work-in-progress
test; the ATDD orchestrator sets it to `1` at verify time to run it. The
gate is never removed — no enable/disable step.

| Operation | Syntax |
|-----------|--------|
| Attribute | `[SkippableFact]` in place of `[Fact]` (from the `Xunit.SkippableFact` package) |
| Guard (first body line) | `Skip.IfNot(Environment.GetEnvironmentVariable("GH_OPTIVEM_RUN_WIP_TESTS") == "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");` |
| Imports | `using Xunit;` (for `Skip`) and `using System;` (for `Environment`) |

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

## Rule Grouping

Scenarios grouped under a Gherkin `Rule:` (see the architecture
[Test File Rules](../../atdd/architecture/test.md) and the runtime `ac-format.md`)
become a `// Rule: <name>` comment block plus a shared PascalCase method-name
prefix — no nested class, composing with the `[Theory]`/`[ChannelData]` wrapper:

```csharp
// Rule: Shipping is charged at $0.10 per kg per unit
[Theory]
[ChannelData(ChannelType.UI, ChannelType.API)]
public async Task ShippingPerKgPerUnit_SingleItem(Channel channel) { ... }

[Theory]
[ChannelData(ChannelType.UI, ChannelType.API)]
public async Task ShippingPerKgPerUnit_ScalesWithQuantity(Channel channel) { ... }
```
