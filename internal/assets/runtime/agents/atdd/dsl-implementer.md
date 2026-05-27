---
# Real DSL logic = system-semantics reasoning. Opus, but medium effort
# because the scope per dispatch is bounded to one DSL surface.
model: opus
effort: medium
---

The implement-dsl task replaces every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic and, when the DSL surface needs new behaviour, adds matching prototype methods to the driver port (`${driver-port}`) or external-system driver port (`${external-system-driver-port}`) in scope.

## Inputs

### Scope

${scope-block}

This task does not receive a substituted artifact input; the `TODO: DSL` prototypes the agent operates on are discovered from the files under its read-scope.

## Steps

1. Implement the DSL Core (`${dsl-core}`) for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver methods, add the method signature to the port (interface) and a throwing stub body to the corresponding adapter (impl class) so compilation works. Interfaces hold signatures only; the throwing-TODO body lives in the adapter:
   (a) Signature in the System Driver Port (`${driver-port}`); throwing `"TODO: System Driver"` stub in the System Driver Adapter (`${driver-adapter}`).
   (b) Signature in the External System Driver Port (`${external-system-driver-port}`); throwing `"TODO: External System Driver"` stub in the External System Driver Adapter (`${external-system-driver-adapter}`).
3. Emit the phase-output flag(s) listed under `## Outputs` for this invocation. Every flag listed for this invocation **MUST** be emitted before completing — unset is a bug, not a default `false`. The downstream gateway picks the next task from the flag values.

## Outputs

Emit each declared output by calling `gh optivem output write KEY=VAL`
from the `Bash` tool. The dispatcher reads the resulting per-invocation
JSONL file after you exit. Call once per dispatch (multiple `KEY=VAL`
args allowed in a single call); if you need to correct a value, call
again with the new value (last-write-wins).

${expected-outputs}

### Flag semantics

Both required keys MUST be emitted. The downstream gateway treats *unset* as an error (no implicit default).

| Flag key (exact) | Meaning when `true` | Meaning when `false` |
|---|---|---|
| `system-driver-port-changed` | implement-system-driver-adapters must run (new System Driver (`${driver-port}`) methods need real impls) | the System Driver port (`${driver-port}`) was not changed this invocation. |
| `external-driver-port-changed` | implement-external-system-driver-adapters must run (new External System Driver (`${external-system-driver-port}`) methods need real impls) | the External System Driver port (`${external-system-driver-port}`) was not changed this invocation. |

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
