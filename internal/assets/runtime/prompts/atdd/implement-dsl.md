---
# Real DSL logic = system-semantics reasoning. Opus, but medium effort
# because the scope per dispatch is bounded to one DSL surface.
model: opus
effort: medium
---

The implement-dsl task replaces every `TODO: DSL` prototype in the DSL Core (`${dsl-core}`) with real logic and, when the DSL surface needs new behaviour, adds matching prototype methods to the driver port (`${driver-port}`) or external-system driver port (`${external-system-driver-port}`) in scope.

## Inputs

### Scope

${scope_block}

This task does not receive a substituted artifact input; the `TODO: DSL` prototypes the agent operates on are discovered from the files under its read-scope.

### Parameters

- `${touches-system-driver}` — boolean, `true` | `false`. Controls whether the System Driver port (`${driver-port}`) is in scope for this invocation.
  - When `true`: the System Driver Interface (`${driver-port}`) is in scope; the agent may add prototype methods there (throwing `"TODO: System Driver"`) and MUST set the `System Driver Interface Changed` flag.
  - When `false`: the System Driver port (`${driver-port}`) is out of scope; the agent leaves it untouched and does not set the `System Driver Interface Changed` flag.

## Steps

1. Implement the DSL Core (`${dsl-core}`) for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver interface methods, add prototype methods that throw a runtime exception so compilation works:
   (a) In the System Driver Interface (`${driver-port}`), throw `"TODO: System Driver"` (only when `${touches-system-driver}=true`; otherwise skip).
   (b) In the External System Driver Interface (`${external-system-driver-port}`), throw `"TODO: External System Driver"`.
3. Set the phase-output flag(s) listed under `## Outputs` for this invocation. Every flag listed for this invocation **MUST** be set before completing — unset is a bug, not a default `no`. The downstream gateway picks the next task from the flag values.

## Outputs

### Phase-output flags

The work-agent MUST set every flag listed for this invocation. The downstream gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Emitted when | Meaning when `yes` |
|---|---|---|---|
| `System Driver Interface Changed` | `yes` \| `no` | `${touches-system-driver}=true` only | implement-system-driver-adapters must run (new System Driver (`${driver-port}`) methods need real impls) |
| `External System Driver Interface Changed` | `yes` \| `no` | always | implement-external-system-driver-adapters must run (new External System Driver (`${external-system-driver-port}`) methods need real impls) |

## Additional Notes

- If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`), signature mismatch, typo) and fix it minimally. Do not change DSL Core (`${dsl-core}`) semantics.
