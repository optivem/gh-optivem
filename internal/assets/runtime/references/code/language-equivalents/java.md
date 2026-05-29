# Language Equivalents — Java

Per-language slice of the combined ATDD language-equivalents reference,
served to dispatches with `${language}=java`. See the
[README](README.md) for the multi-language overview.

## TODO Stubs

| Concept | Syntax |
|---------|--------|
| DSL stub | `throw new UnsupportedOperationException("TODO: DSL")` |
| Driver stub | `throw new UnsupportedOperationException("TODO: Driver")` |

## WIP Gate

The acceptance-test-writer prepends a permanent env-var gate to every AT
method. Feature-branch CI, local `mvn test`, and IDE runs leave
`GH_OPTIVEM_RUN_WIP_TESTS` unset and silently skip the work-in-progress
test; the ATDD orchestrator sets it to `1` at verify time to run it. The
gate is never removed — no enable/disable step.

| Operation | Syntax |
|-----------|--------|
| WIP gate (above `@Test`) | `@EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", matches = "1", disabledReason = "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run")` |
| Import | `import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;` |

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
