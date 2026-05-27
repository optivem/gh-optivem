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

Emit each declared output by calling `gh optivem output write KEY=VAL`
from the `Bash` tool. The dispatcher reads the resulting per-invocation
JSONL file after you exit. Call once per dispatch (multiple `KEY=VAL`
args allowed in a single call); if you need to correct a value, call
again with the new value (last-write-wins).

${expected-outputs}

### Flag semantics

Both required keys MUST be emitted. The downstream gateway treats *unset* as an error (no implicit default).

Each flag is keyed on the port **directory**, not on what kind of file was touched. If you added or modified *any* file under the port path — interface, DTO, enum, anything — the flag is `true`. New method signatures are one trigger; new/changed DTO fields, new enum values, or any other port-surface change all count equally.

| Flag key (exact) | Meaning when `true` | Meaning when `false` |
|---|---|---|
| `system-driver-port-changed` | At least one file under `${driver-port}/**` was added or modified this invocation (interface, DTO, enum, anything). implement-system-driver-adapters must run to wire the new/changed surface end-to-end. | Zero files under `${driver-port}/**` were touched this invocation. |
| `external-driver-port-changed` | At least one file under `${external-system-driver-port}/**` was added or modified this invocation (interface, DTO, enum, anything). implement-external-system-driver-adapters must run to wire the new/changed surface end-to-end. | Zero files under `${external-system-driver-port}/**` were touched this invocation. |

`scope-exception-files` / `scope-exception-reason` (optional) are the
scope-exception envelope (see `${references-root}/scope.md`). Emit
them only when you had to read or write outside this MID's scope.

Example call:

```
gh optivem output write \
  system-driver-port-changed=false \
  external-driver-port-changed=false
```

## Additional Notes

- If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`), signature mismatch, typo) and fix it minimally. Do not change DSL Core (`${dsl-core}`) semantics.
