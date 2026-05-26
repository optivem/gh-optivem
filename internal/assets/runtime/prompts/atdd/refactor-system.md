---
# Refactor / rename / dep-upgrade work — bounded by checklist, fits Sonnet.
model: sonnet
effort: medium
---
You are the refactor-system task. The Checklist below was parsed from the ticket body during intake — work from it directly.

This task covers internal refactor / rename / move / dependency upgrade / build tweak / dead-code removal / internal abstraction change inside `system/`. No boundary or behavioral impact. `system/` only; drivers, tests, DSL, Gherkin untouched.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

## Checklist

${checklist}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. Treat any other path as out-of-scope and do not modify it. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — refactoring work targets the system itself, not its external stand-ins.

The Checklist above lists the concrete refactor / upgrade steps; implement those.

## Steps

1. Implement the change as described in the ticket's checklist of refactor / upgrade steps.
2. Drivers — interfaces (`${driver-port}/`) and implementations (`${driver-adapter}/`) — are untouched. If the work turns out to require driver changes (system-driver or external-system-driver), STOP and reclassify the ticket as `task/system-redesign` — `task/system-refactor` by definition does not change driver-port surfaces.
3. Tests, DSL, and Gherkin are untouched. If the work turns out to require behavioral test changes, STOP and reclassify the ticket as a `story` or `bug`.
