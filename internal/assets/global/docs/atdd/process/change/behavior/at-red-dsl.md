# AT - RED - DSL

Implement the DSL Core for real; set the two driver-interface-changed flags.

## Scope

This phase touches the `dsl_core`, `driver_port` layers (bare layer
names; resolved physical paths live in `gh-optivem.yaml paths:` —
inspect with `gh optivem process scope AT_RED_DSL`).

See [the scope rule](../../shared/scope.md).

## Steps

1. Implement the DSL Core for real — replace each "TODO: DSL" prototype with actual logic.
2. If you need add additional Driver interface methods:
   (a) In the System Driver Interface: implement prototype methods by throwing `"TODO: System Driver"` exception
   (b) In the External System Driver Interface: implement prototype methods by throwing `"TODO: External System Driver"` exception
3. Set both phase-output flags (see below). Both **MUST** be set before completing the phase — unset is a bug, not a default `no`. The next phase is chosen downstream based on the flag values.
   (a) Set flag: `System Driver Interface Changed: yes|no`
   (b) Set flag: `External System Driver Interface Changed: yes|no`

## Phase-output flags

The work-agent MUST set both flags below. They are read by the post-RED-DSL gateway to branch onto the right next phase; the gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Meaning when `yes` |
|---|---|---|
| `System Driver Interface Changed` | `yes` \| `no` | RED-SYSTEM-DRIVER phase must run (new System Driver methods need real impls) |
| `External System Driver Interface Changed` | `yes` \| `no` | Hand off to the CT cycle (external driver belongs to the CT sub-process) |
