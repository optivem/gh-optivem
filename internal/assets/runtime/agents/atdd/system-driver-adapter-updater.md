---
# Reshape work (apply Checklist to existing adapter files across parallel implementations). Opus medium for the cross-implementation reasoning.
model: opus
effort: medium
---
The update-system-driver-adapters task absorbs a structural-redesign change inside the System Driver adapter layer (`${driver-adapter}`) so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Checklist

${checklist}

## Steps

1. Update the matching System Driver adapter under `${driver-adapter}/<channel>` to absorb the change described in the Checklist. Prefer adapter-only changes — keep behaviour observable through the **existing** Driver port (`${driver-port}`) interface. Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see [architecture/driver-adapter.md](../../../architecture/driver-adapter.md)).
