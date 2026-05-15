# AT - RED - SYSTEM DRIVER

## Purpose

Replace the System-Driver "TODO: Driver" prototypes from AT - RED - DSL with real Driver logic. This phase touches **System Drivers only** (under `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/`); external-system Drivers are handled by the Contract Test sub-process. Tests stay red — they only go green once the system implementation lands in AT - GREEN - SYSTEM.

## What it produces

- After WRITE: real System Driver implementations exist under `${driver_adapter}/${sut_namespace}/`.
- Tests in state: change-driven scenarios disabled with reason `"AT - RED - SYSTEM DRIVER"`; legacy-coverage scenarios still enabled and passing.

## Conventions

- File scope: only files under `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/<channel>` (e.g. `${driver_adapter}/${sut_namespace}/api`, `${driver_adapter}/${sut_namespace}/ui`). All driver code lives in the test tree, not in `system/`. Do NOT touch `external/` siblings — that is the Contract Test sub-process.
- Do NOT read or search backend/frontend source code. Model new Driver methods on existing Driver methods in the same file.
- `@Disabled` / skip syntax per language: see [language-equivalents/](../code/language-equivalents/README.md).
- Definition of System Driver vs External System Driver: see [glossary.md](glossary.md).

## Example

Before — System Driver prototype produced by AT - RED - DSL:

```java
@Override
public RegisterCustomerResponse register(RegisterCustomerRequest request) {
    throw new UnsupportedOperationException("TODO: Driver");
}
```

After — real System Driver wiring the request through the system's HTTP/UI surface (modelled on the sibling `update(...)` method already in this file):

```java
@Override
public RegisterCustomerResponse register(RegisterCustomerRequest request) {
    var response = httpClient.post("/customers", request);
    return response.as(RegisterCustomerResponse.class);
}
```

## AT - RED - SYSTEM DRIVER - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - DSL"`.
2. Implement the System Drivers — replace each "TODO: Driver" prototype with actual logic. Stay within `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/`. Model new methods on existing Driver methods in the same file.

**Scope:** Only System Driver code. No test, DSL, system, or external-driver edits.

## Anti-patterns

- **Editing files under `external/`.** External-system Drivers belong to the Contract Test sub-process (CT - RED - EXTERNAL DRIVER). If a change is needed there, exit this phase and route through CT.
- **Reading backend/frontend source to figure out behaviour.** The Driver speaks to the system's existing surface; behaviour is modelled on sibling Driver methods, not derived from production code. Reading production code in this phase risks coupling test infrastructure to implementation details.
- **Modifying tests or DSL.** Tests are disabled/enabled here, nothing more; DSL is frozen. If the Driver cannot be implemented without DSL or test changes, the previous phase was incomplete — go back, do not patch around it.
- **Leaving "TODO: Driver" behind.** Any remaining System-Driver prototype means the phase is not done.
