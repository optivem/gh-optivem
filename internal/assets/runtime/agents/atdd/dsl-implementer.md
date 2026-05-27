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

### Parameters

- `touches-system-driver` (current value: `${touches-system-driver}`) — boolean, `true` | `false`. Controls whether the System Driver port (`${driver-port}`) is in scope for this invocation.
  - When `true`: the System Driver Interface (`${driver-port}`) is in scope; the agent may add prototype methods there (throwing `"TODO: System Driver"`) and emits `system-driver-ports-changed: true` iff it did so.
  - When `false`: the System Driver port (`${driver-port}`) is out of scope; the agent leaves it untouched and emits `system-driver-ports-changed: false`.

## Steps

1. Implement the DSL Core (`${dsl-core}`) for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver interface methods, add prototype methods that throw a runtime exception so compilation works:
   (a) In the System Driver Interface (`${driver-port}`), throw `"TODO: System Driver"` (only when `${touches-system-driver}=true`; otherwise skip).
   (b) In the External System Driver Interface (`${external-system-driver-port}`), throw `"TODO: External System Driver"`.
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
| `system-driver-ports-changed` | implement-system-driver-adapters must run (new System Driver (`${driver-port}`) methods need real impls) | the System Driver port (`${driver-port}`) was not changed this invocation. When `${touches-system-driver}=false` the port is out of scope and this flag is always `false`. |
| `external-driver-ports-changed` | implement-external-system-driver-adapters must run (new External System Driver (`${external-system-driver-port}`) methods need real impls) | the External System Driver port (`${external-system-driver-port}`) was not changed this invocation. |

`scope-exception-files` / `scope-exception-reason` (optional) are the
scope-exception envelope (see `${references-root}/scope.md`). Emit
them only when you had to read or write outside this MID's scope.

Example call:

```
gh optivem output write \
  system-driver-ports-changed=false \
  external-driver-ports-changed=false
```

## Additional Notes

- If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`), signature mismatch, typo) and fix it minimally. Do not change DSL Core (`${dsl-core}`) semantics.
