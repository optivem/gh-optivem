---
# Real DSL logic = system-semantics reasoning. Opus, but medium effort
# because the scope per dispatch is bounded to one DSL surface.
model: opus
effort: medium
---

The implement-dsl task replaces every `TODO: DSL` prototype in the DSL Core with real logic and, when the DSL surface needs new behaviour, adds matching prototype methods to the driver port(s) in scope.

## Inputs

This task does not receive a substituted artifact input; the `TODO: DSL` prototypes the agent operates on are discovered from the files under its read-scope.

### Parameters

- `${touches-system-driver}` — boolean, `true` | `false`. Controls whether the System Driver port is in scope for this invocation.
  - When `true`: the System Driver Interface is in scope; the agent may add prototype methods there (throwing `"TODO: System Driver"`) and MUST set the `System Driver Interface Changed` flag.
  - When `false`: the System Driver port is out of scope; the agent leaves it untouched and does not set the `System Driver Interface Changed` flag.

## Steps

1. Implement the DSL Core for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver interface methods, add prototype methods that throw a runtime exception so compilation works:
   (a) In the System Driver Interface, throw `"TODO: System Driver"` (only when `${touches-system-driver}=true`; otherwise skip).
   (b) In the External System Driver Interface, throw `"TODO: External System Driver"`.
3. Set the phase-output flag(s) listed under `## Outputs` for this invocation. Every flag listed for this invocation **MUST** be set before completing — unset is a bug, not a default `no`. The downstream gateway picks the next task from the flag values.

## Outputs

### Phase-output flags

The work-agent MUST set every flag listed for this invocation. The downstream gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Emitted when | Meaning when `yes` |
|---|---|---|---|
| `System Driver Interface Changed` | `yes` \| `no` | `${touches-system-driver}=true` only | implement-system-driver-adapters must run (new System Driver methods need real impls) |
| `External System Driver Interface Changed` | `yes` \| `no` | always | implement-external-system-driver-adapters must run (new External System Driver methods need real impls) |

## Additional Notes

- If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub, signature mismatch, typo) and fix it minimally. Do not change DSL semantics.
