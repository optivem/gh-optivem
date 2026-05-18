# CHORE - WRITE

Internal refactor / rename / move / dependency upgrade / build tweak / dead-code removal / internal abstraction change inside `system/`; no boundary or behavioral impact.

## Scope

`system/` only; drivers, tests, DSL, Gherkin untouched.

## Steps

1. Implement the change as described in the ticket's checklist of refactor / upgrade steps.
2. Drivers — interfaces (`${driver_port}/`) and implementations (`${driver_adapter}/`) — are untouched. If the work turns out to require driver changes, STOP and reclassify the ticket: `subtype:system-implementation-change` by definition does not change boundaries; relabel as `subtype:system-interface-redesign` (or `subtype:external-system-interface-redesign` for an external-system change).
3. Tests, DSL, and Gherkin are untouched. If the work turns out to require behavioral test changes, STOP and reclassify the ticket as a Story or Bug.
