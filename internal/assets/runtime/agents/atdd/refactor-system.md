---
# Refactor / rename / dep-upgrade work — bounded by checklist, fits Sonnet.
model: sonnet
effort: medium
---
You are the refactor-system task. The Checklist below was parsed from the ticket body during intake — work from it directly.

This task covers internal refactor / rename / move / dependency upgrade / build tweak / dead-code removal / internal abstraction change inside `system/` (`${system-path}`). No boundary or behavioral impact. `system/` (`${system-path}`) only; drivers, tests, DSL, Gherkin untouched.

## Inputs

### Scope

${scope_block}

### Parameters

Architecture: ${architecture}

### Checklist

${checklist}

The Checklist above lists the concrete refactor / upgrade steps; implement those.

## Steps

1. Implement the change as described in the ticket's checklist of refactor / upgrade steps.
2. If the work turns out to require driver changes (system-driver or external-system-driver), STOP and reclassify the ticket as `task/system-redesign` — `task/system-refactor` by definition does not change driver-port surfaces.
3. Tests, DSL, and Gherkin are untouched. If the work turns out to require behavioral test changes, STOP and reclassify the ticket as a `story` or `bug`.
