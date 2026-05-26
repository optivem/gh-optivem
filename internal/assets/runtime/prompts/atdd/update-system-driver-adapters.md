---
# Reshape work (apply Checklist to existing adapter files across parallel implementations). Opus medium for the cross-implementation reasoning.
model: opus
effort: medium
---
The update-system-driver-adapters task absorbs a structural-redesign change inside the System Driver adapter layer so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt.

## Inputs

### Checklist

${checklist}

## Steps

1. Update the matching System Driver adapter(s) under `${driver-adapter}/<channel>` to absorb the change described in the Checklist. Prefer adapter-only changes — keep behaviour observable through the **existing** driver interface. Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see [architecture/driver-adapter.md](../../../architecture/driver-adapter.md)).
