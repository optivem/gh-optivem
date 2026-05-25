---
# Real DSL logic = system-semantics reasoning. Opus, but medium effort
# because the scope per dispatch is bounded to one DSL surface.
model: opus
effort: medium
scope: {}   # query resolved scope: `gh optivem process scope implement-dsl`
---
<!--
Parameter: ${touches-system-driver} (boolean). Gates whether this
invocation may add methods to the System Driver port and emit the
`System Driver Interface Changed` flag. Callers from the
implement-and-verify-dsl HIGH on the AT side (change-system-behavior
CYCLE) pass `true`; callers on the CT side (cover-system-behavior
CYCLE) pass `false`, since their DSL work is bounded to the
external-system-driver port.
-->

The implement-dsl task replaces every `TODO: DSL` prototype in the DSL Core with real logic and, when the DSL surface needs new behaviour, adds matching prototype methods to the driver port(s) in scope.

If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub, signature mismatch, typo) and fix it minimally. Do not change DSL semantics.

Do not present or wait for approval inside the agent.

Read `${references_root}/atdd/architecture/dsl-core.md`.
Read `${references_root}/atdd/architecture/driver-port.md`.
Read `${references_root}/code/language-equivalents/${language}.md`.

## Steps

1. Implement the DSL Core for real — replace each `TODO: DSL` prototype with actual logic.
2. If you need to add additional driver interface methods, add prototype methods that throw a runtime exception so compilation works:
   (a) If `${touches-system-driver}=true`: in the System Driver Interface, throw `"TODO: System Driver"`.
   (b) In the External System Driver Interface, throw `"TODO: External System Driver"`.
   When `${touches-system-driver}=false`, the System Driver port is out of scope for this invocation — leave it untouched.
3. Set the phase-output flag(s) below. Every flag listed for this invocation **MUST** be set before completing — unset is a bug, not a default `no`. The downstream gateway picks the next task from the flag values.
   (a) If `${touches-system-driver}=true`: set flag `System Driver Interface Changed: yes|no`.
   (b) Set flag `External System Driver Interface Changed: yes|no`.

## Phase-output flags

The work-agent MUST set every flag listed for this invocation. The downstream gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Emitted when | Meaning when `yes` |
|---|---|---|---|
| `System Driver Interface Changed` | `yes` \| `no` | `${touches-system-driver}=true` only | implement-system-driver-adapters must run (new System Driver methods need real impls) |
| `External System Driver Interface Changed` | `yes` \| `no` | always | implement-external-system-driver-adapters must run (new External System Driver methods need real impls) |
