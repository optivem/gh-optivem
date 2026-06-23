---
# Reshape work (Ext* DTOs + Real/Stub driver updates per Checklist). Opus medium for the structural-reshape reasoning.
model: opus
effort: medium
---
The update-external-system-driver-adapters task reshapes the external-system driver layer (`${external-system-driver-adapter}`) (Ext* DTOs (`${external-system-driver-adapter}`), Real driver (`${external-system-driver-adapter}`), Stub driver (`${external-system-driver-adapter}`)) to match a new external API so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}

## Steps

1. Identify the external system from the Checklist and locate its driver components (`${external-system-driver-adapter}`) under `${external-system-driver-adapter}/<external-system>/` (`XyzRealDriver`, `XyzStubDriver` per stub variant, `BaseXyzClient`, `Ext*` DTOs). Then execute steps 2–4 below.
2. Update the `Ext*` DTOs (`${external-system-driver-adapter}`) to match the new external surface (fields, types, encoding).
3. Update the Real driver impl (`${external-system-driver-adapter}/<external-system>/XyzRealDriver`) to consume the new surface.
4. Update the Stub driver impl (`${external-system-driver-adapter}/<external-system>/XyzStubDriver`) to mirror the new surface so stubs stay consistent with reality.
