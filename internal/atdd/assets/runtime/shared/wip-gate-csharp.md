Add the gate to each Acceptance Test method without disturbing its channel parameterization. Change the test attribute from `[Theory]` to `[SkippableTheory]`, keep every `[ChannelData(...)]` / `[ChannelInlineData(...)]` attribute and the `(Channel channel)` parameter exactly as written, and make `Skip.IfNot(...)` the first statement in the body:

```csharp
[SkippableTheory]
[ChannelData(ChannelType.UI, ChannelType.API)]
public async Task ShouldXxx(Channel channel)
{
    Skip.IfNot(Environment.GetEnvironmentVariable("GH_OPTIVEM_RUN_WIP_TESTS") == "1", "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run");
    ...
}
```

`[SkippableTheory]` comes from the `Xunit.SkippableFact` package. Add `using Xunit;` (for `Skip`) and `using System;` (for `Environment`) if not already present. Do not replace `[Theory]`/`[ChannelData(...)]` with `[Fact]`/`[SkippableFact]` — that would drop the channel parameterization.
