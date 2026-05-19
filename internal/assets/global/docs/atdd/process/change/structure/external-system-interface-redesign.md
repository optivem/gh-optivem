# EXTERNAL SYSTEM INTERFACE REDESIGN - WRITE

Reshape the external-system driver layer to match the new external API. The Real driver + Stub driver(s) + Ext* DTOs absorb the change so DSL, Gherkin, and tests stay untouched.

## Scope

`${external_driver_adapter}/<external-system>/...` (exceptionally `${external_driver_port}/<external-system>/...` with approval)

## Steps

1. Identify the external system from the Checklist; locate its driver components under `${external_driver_adapter}/<external-system>/`: `XyzRealDriver`, `XyzStubDriver` (one per stub variant), `BaseXyzClient`, `Ext*` DTOs.
2. Update the `Ext*` DTOs to match the new external surface (fields, types, encoding).
3. Update the Real driver impl (`${external_driver_adapter}/<external-system>/XyzRealDriver`) to consume the new surface. Apply across **all parallel implementations** (Java/.NET/TS × monolith/multitier — see [architecture/driver-adapter.md](../../../architecture/driver-adapter.md)).
4. Update the Stub driver impl(s) (`${external_driver_adapter}/<external-system>/XyzStubDriver`) to mirror the new surface so stubs stay consistent with reality.
5. **External driver port guardrail.** Do NOT modify any file under `${external_driver_port}/` casually. If an interface change is unavoidable, STOP and present to the user: the method(s) you want to change, why the Real/Stub adapters alone cannot absorb the change, the proposed new signature(s), and the explicit warning that this WILL require contract-test updates (CT sub-process gets invoked for affected scenarios). Wait for explicit user approval before editing any `${external_driver_port}/` file.
6. Do not modify acceptance tests, DSL, Gherkin, or any code outside the external-system driver layer. `${system_test_path}/.../Legacy/` is read-only.
