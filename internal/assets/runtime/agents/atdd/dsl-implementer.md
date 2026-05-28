---
# Real DSL logic = system-semantics reasoning. Opus, but medium effort
# because the scope per dispatch is bounded to one DSL surface.
model: opus
effort: medium
---

The implement-dsl task replaces every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic and, when the DSL surface needs new behaviour, extends the driver port (`${driver-port}`) or external-system driver port (`${external-system-driver-port}`) in scope — either by adding matching prototype methods or by adding/changing fields on the DTOs those methods carry.

## Inputs

### Scope

${scope-block}

This task does not receive a substituted artifact input; the `TODO: DSL` prototypes the agent operates on are discovered from the files under its read-scope.

## Steps

1. Implement the DSL Core (`${dsl-core}`) for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver methods, add the method signature to the port (interface) and a throwing stub body to the corresponding adapter (impl class) so compilation works. Interfaces hold signatures only; the throwing-TODO body lives in the adapter:
   (a) Signature in the System Driver Port (`${driver-port}`); throwing `"TODO: System Driver"` stub in the System Driver Adapter (`${driver-adapter}`).
   (b) Signature in the External System Driver Port (`${external-system-driver-port}`); throwing `"TODO: External System Driver"` stub in the External System Driver Adapter (`${external-system-driver-adapter}`).
3. Before emitting outputs, list every file you wrote this invocation. For each port-changed flag, set it to `true` if **any** file in that list sits under the flag's port directory (see the flag-semantics table below). The flag is a question about the directory, not about methods — DTOs, enums, interfaces, and any other file under the port path all count. Setting `false` is a claim that you wrote zero files under that port path.
4. Emit the phase-output flag(s) listed under `## Outputs` for this invocation. Every flag listed for this invocation **MUST** be emitted before completing — unset is a bug, not a default `false`. The downstream gateway picks the next task from the flag values.

## Outputs

${expected-outputs}

Notes:

- Both required keys MUST be emitted — the downstream gateway treats *unset* as an error (no implicit default).
- Each `*-port-changed` flag is keyed on the port **directory**. List every file you wrote and set the flag `true` if any file sits under the flag's port directory (interface, DTO, enum — anything). `system-driver-port-changed` covers `${driver-port}/**`; `external-driver-port-changed` covers `${external-system-driver-port}/**`. The dispatcher's `validate-outputs-and-scopes` re-derives directory keying from `${changed-files}`, so an incorrect value mis-routes the cycle.
- `scope-exception-files` / `scope-exception-reason` are the envelope from the prepended `scope.md`. Emit only when you read or wrote outside scope.

## Additional Notes

- If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`), signature mismatch, typo) and fix it minimally. Do not change DSL Core (`${dsl-core}`) semantics.
