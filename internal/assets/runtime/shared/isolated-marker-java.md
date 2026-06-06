Apply `@Isolated` at the **class** level (the whole class runs serially), keeping each method's `@TestTemplate` and `@Channel(...)` annotations exactly as written:

```java
import com.optivem.testing.Isolated;

@Isolated
class PlaceOrderPositiveIsolatedTest extends BaseAcceptanceTest {
    @TestTemplate
    @Channel({ChannelType.UI, ChannelType.API})
    void shouldXxx() { ... }
}
```

Add `import com.optivem.testing.Isolated;` next to the other testkit imports if it's not already present.
