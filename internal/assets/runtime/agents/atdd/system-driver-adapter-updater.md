---
# Reshape work (apply Checklist to existing adapter files across parallel implementations). Opus medium for the cross-implementation reasoning.
model: opus
effort: medium
---
The update-system-driver-adapters task absorbs a structural-redesign change inside the System Driver adapter layer (`${system-driver-adapter}`) so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}

## Steps

1. Update the matching System Driver adapter under `${system-driver-adapter}/<channel>` to absorb the change described in the Checklist. Prefer adapter-only changes — keep behaviour observable through the **existing** Driver port (`${system-driver-port}`) interface. Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see [architecture/driver-adapter.md](../../../architecture/driver-adapter.md)).
