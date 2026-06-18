Apply `@Isolated` at the **class** level (the whole class runs serially), keeping each method's `@TestTemplate` and `@Channel(...)` annotations exactly as written:

```java
import com.optivem.testing.Isolated;

@Isolated("mutates the cancellation-blackout clock; parallel runs would be flaky")
class PlaceOrderPositiveIsolatedTest extends BaseAcceptanceTest {
    @TestTemplate
    @Channel({ChannelType.UI, ChannelType.API})
    void shouldXxx() { ... }
}
```

The reason string is **optional free text** (the annotation's `value`). Lift it **verbatim** from an adjacent Gherkin comment / scenario-description line on the source scenario (e.g. a `# isolated: …` line above `Scenario:`) when one is present; emit bare `@Isolated()` when none is present. **Never invent a reason** — if the scenario carries no reason line, the annotation stays `@Isolated()`.

Add `import com.optivem.testing.Isolated;` next to the other testkit imports if it's not already present.
