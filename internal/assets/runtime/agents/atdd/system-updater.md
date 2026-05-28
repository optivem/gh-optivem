---
# Architectural surface reshape (Checklist drives per-channel updates across implementations). Opus high for cross-channel reasoning.
model: opus
effort: high
---
The update-system task reshapes the system surface (`${system-path}`) to absorb a structural-redesign change. A Checklist parsed from the ticket body lists the changes to apply across affected channels.

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}

## Steps

1. Execute the Checklist on the system surface (`${system-path}`). Read the Checklist and the system tree (`${system-path}`) to decide which channel(s) the ticket targets — do NOT pre-classify; let the Checklist + system layout (`${system-path}`) pick it. Examples: `api`, `ui`, `mobile`, `cli`, `admin`. Update the system surface (`${system-path}`) under `system/` to match:
   - **API**: controllers, request/response DTOs, routes, status codes, error format.
   - **UI**: page structure, form fields, navigation, copy, selectors.
   - **Other**: channel-specific equivalents (commands/flags for CLI, screens for mobile, admin pages, …).

   Apply across **all parallel implementations** (Java/.NET/TS × monolith/multitier — see [architecture/system.md](../../../architecture/system.md)). After editing the source of truth, grep the system tree (`${system-path}`) for residual references (e.g. the old URL string) before moving on.
2. When the Checklist requires a schema change (e.g. extracting a column into a join table, renaming a persisted field, splitting a table along a new boundary), add a schema migration under the shared migration set (`${system-db-migration-path}`) — a single timestamped SQL file in the Flyway naming convention (`V{YYYYMMDDHHMMSS}__{description}.sql`, forward-only, no undo) following the expand-contract pattern so the previous app version's invariants stay satisfied across the deploy. Read the existing migrations first to see the current schema. The migration set is shared across every SUT (3 languages × 2 architectures); your one file is consumed by all of them.
