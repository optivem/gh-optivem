Mark the **class** with `[Collection("Isolated")]` (so xUnit runs it serially, never concurrently with another isolated class) and `[Trait("Category", "isolated")]` (so the isolated suite filters it into the isolated partition). Put `[Isolated("reason")]` on each **method**, alongside its `[Theory]`/`[ChannelData(...)]`, exactly as written:

```csharp
using Optivem.Testing;

[Collection("Isolated")]
[Trait("Category", "isolated")]
public class PlaceOrderPositiveIsolatedTest : BaseAcceptanceTest
{
    [Theory]
    [Isolated("mutates the cancellation-blackout clock; parallel runs would be flaky")]
    [ChannelData(ChannelType.UI, ChannelType.API)]
    public async Task ShouldXxx(Channel channel) { ... }
}
```

The reason string is **optional free text** (the `IsolatedAttribute(string reason)` ctor argument). Lift it **verbatim** from an adjacent Gherkin comment / scenario-description line on the source scenario (e.g. a `# isolated: …` line above `Scenario:`) when one is present; emit bare `[Isolated]` when none is present. **Never invent a reason** — if the scenario carries no reason line, the attribute stays `[Isolated]`.

Add `using Optivem.Testing;` next to the other testkit usings if it's not already present.
