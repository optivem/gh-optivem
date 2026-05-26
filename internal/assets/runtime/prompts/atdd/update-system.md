---
# Architectural surface reshape (Checklist drives per-channel updates across implementations). Opus high for cross-channel reasoning.
model: opus
effort: high
---
The update-system task reshapes the system surface to absorb a structural-redesign change. A Checklist parsed from the ticket body lists the changes to apply across affected channels.

Architecture: ${architecture}

Allowed write roots:
${allowed_roots}

Edit ONLY files under the "Allowed write roots" listed at the top of this prompt. External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise read-only context.

## Inputs

- `${architecture}` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `${allowed_roots}` — the paths under which this task is permitted to write.
- `${checklist}` — the parsed list of changes to apply across affected channels.

### Checklist

${checklist}

## Steps

1. Execute the Checklist on the system surface. Read the Checklist and the system tree to decide which channel(s) the ticket targets — do NOT pre-classify; let the Checklist + system layout pick it. Examples: `api`, `ui`, `mobile`, `cli`, `admin`. Update the system surface under `system/` to match:
   - **API**: controllers, request/response DTOs, routes, status codes, error format.
   - **UI**: page structure, form fields, navigation, copy, selectors.
   - **Other**: channel-specific equivalents (commands/flags for CLI, screens for mobile, admin pages, …).

   Apply across **all parallel implementations** (Java/.NET/TS × monolith/multitier — see [architecture/system.md](../../../architecture/system.md)). After editing the source of truth, grep the system tree for residual references (e.g. the old URL string) before moving on.
