# CT - RED - EXTERNAL SYSTEM DRIVER

Implement the External System Driver adapters (only if `External System Driver Interface Changed = yes`).

## Scope

This phase touches the `external_system_driver_port`, `external_system_driver_adapter`
layers (bare layer names; resolved physical paths live in
`gh-optivem.yaml paths:` — inspect with
`gh optivem process scope CT_RED_EXTERNAL_SYSTEM_DRIVER`).

See [the scope rule](../../shared/scope.md).

## Steps

1. Implement the External System Driver Adapters for real — replace each "TODO: External System Driver" prototype with actual logic.
2. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract.
