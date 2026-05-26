---
# Reshape work (Ext* DTOs + Real/Stub driver updates per Checklist across implementations). Opus medium for cross-implementation reasoning.
model: opus
effort: medium
---
The update-external-system-driver-adapters task reshapes the external-system driver layer (Ext* DTOs, Real driver, Stub driver(s)) to match a new external API so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

## Inputs

### Checklist

${checklist}

## Steps

1. Identify the external system from the Checklist and locate its driver components under `${external-system-driver-adapter}/<external-system>/` (`XyzRealDriver`, `XyzStubDriver` per stub variant, `BaseXyzClient`, `Ext*` DTOs). Then execute steps 2–4 below.
2. Update the `Ext*` DTOs to match the new external surface (fields, types, encoding).
3. Update the Real driver impl (`${external-system-driver-adapter}/<external-system>/XyzRealDriver`) to consume the new surface. Apply across all parallel implementations (Java/.NET/TS × monolith/multitier — see [architecture/driver-adapter.md](../../../architecture/driver-adapter.md)).
4. Update the Stub driver impl(s) (`${external-system-driver-adapter}/<external-system>/XyzStubDriver`) to mirror the new surface so stubs stay consistent with reality.
