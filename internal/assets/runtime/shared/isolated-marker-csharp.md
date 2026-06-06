Apply `[Collection("Isolated")]` **and** `[Trait("Category", "isolated")]` at the **class** level — the `[Collection]` is what forces the class to run serially; the `[Trait]` is what the isolated suite filters on. Keep each method's `[Theory]`/`[ChannelData(...)]` exactly as written:

```csharp
[Collection("Isolated")]
[Trait("Category", "isolated")]
public class PlaceOrderPositiveIsolatedTest : BaseAcceptanceTest
{
    [Theory]
    [ChannelData(ChannelType.UI, ChannelType.API)]
    public async Task ShouldXxx(Channel channel) { ... }
}
```
