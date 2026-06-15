Add the gate annotation directly above each Acceptance Test method, keeping its existing `@TestTemplate` and `@Channel(...)` annotations exactly as written:

```java
@EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", matches = "1", disabledReason = "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run")
@TestTemplate
@Channel({ChannelType.UI, ChannelType.API})
void shouldXxx() { ... }
```

Add `import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;` next to the other JUnit imports if it's not already present. Do not replace `@TestTemplate`/`@Channel` with `@Test` — the gate is purely additive.
