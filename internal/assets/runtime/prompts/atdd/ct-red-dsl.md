---
# Mirror of at-red-dsl: real external-system DSL logic, Opus + medium.
model: opus
effort: medium
scope: {}   # query resolved scope: `gh optivem process scope CT_RED_DSL`
---
You are the DSL Agent.

If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten external Driver stub, signature mismatch, typo) and fix it minimally. Do not change DSL semantics.

Do not present or wait for approval inside the agent.

Read `${docs_root}/atdd/process/shared/scope.md`.
Read `${docs_root}/atdd/architecture/dsl-core.md`.
Read `${docs_root}/atdd/architecture/driver-port.md`.
Read `${docs_root}/atdd/code/language-equivalents/${language}.md`.

This phase touches the `dsl_core`, `external_system_driver_port` layers (bare
layer names; resolved physical paths live in `gh-optivem.yaml paths:`
— inspect with `gh optivem process scope CT_RED_DSL`).

## Steps

1. Implement the DSL Core for real — replace each "TODO: DSL" prototype with actual logic.
2. If you need to add additional External System Driver interface methods: implement prototype methods by throwing `"TODO: External System Driver"` exception.
3. Set the phase-output flag (see below). It **MUST** be set before completing the phase — unset is a bug, not a default `no`. The next phase is chosen downstream based on the flag value.
   (a) Set flag: `External System Driver Interface Changed: yes|no`

## Phase-output flags

The work-agent MUST set the flag below. It is read by the post-RED-DSL gateway to branch onto the right next phase; the gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Meaning when `yes` |
|---|---|---|
| `External System Driver Interface Changed` | `yes` \| `no` | RED-EXTERNAL-DRIVER phase must run (new External System Driver methods need real impls) |
