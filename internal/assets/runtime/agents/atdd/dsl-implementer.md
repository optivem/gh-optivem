---
# Real DSL logic = system-semantics reasoning. Opus, but medium effort
# because the scope per dispatch is bounded to one DSL surface.
model: opus
effort: medium
---

Replace every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic. If new port-side behaviour is needed, you may add methods to the driver ports or add/change fields on the DTOs they carry.

This dispatch implements DSL for the **`${test-category}`** test layer.

> **CT-path guard.** If you are implementing CT-side DSL (`test-category=contract`), the System Driver port (`${system-driver-port}`) will **not** legitimately need new methods — do **not** add System Driver prototypes there and do **not** emit `system-driver-port-changed=true`. Contract tests stimulate the External-System Driver only (`${external-system-driver-port}`). On the AT path (`test-category=acceptance`) System Driver prototypes are fine.

## Inputs

### Scope

${scope-block}

No substituted input — discover `TODO: DSL` prototypes by reading the files under `${dsl-core}`.

## Steps

1. Implement the DSL Core (`${dsl-core}`) for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver methods, add the method signature to the port (interface) and a throwing stub body to the corresponding adapter (impl class) so compilation works. Interfaces hold signatures only; the throwing-TODO body lives in the adapter:
   (a) Signature in the System Driver Port (`${system-driver-port}`); throwing `"TODO: System Driver"` stub in the System Driver Adapter (`${system-driver-adapter}`).
   (b) Signature in the External System Driver Port (`${external-system-driver-port}`); throwing `"TODO: External System Driver"` stub in the External System Driver Adapter (`${external-system-driver-adapter}`).
3. Before emitting outputs, set each `*-port-changed` flag per the rules in the Notes section below.
4. Emit the phase-output flag(s) listed under `## Outputs` for this invocation. Every flag listed for this invocation **MUST** be emitted before completing — unset is a bug, not a default `false`. The downstream gateway picks the next task from the flag values.

## Outputs

${expected-outputs}

Notes:

- Both required keys MUST be emitted — the downstream gateway treats *unset* as an error (no implicit default).
- Each `*-port-changed` flag is keyed on the port **directory**. List every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). `system-driver-port-changed` covers `${system-driver-port}/**`; `external-driver-port-changed` covers `${external-system-driver-port}/**`. The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.

## Additional Notes

- ${re-entry-policy} For this agent the "broken/missing piece" is typically a forgotten driver stub in the System Driver port (`${system-driver-port}`) or External System Driver port (`${external-system-driver-port}`). Do not change DSL Core (`${dsl-core}`) semantics in the fix.
