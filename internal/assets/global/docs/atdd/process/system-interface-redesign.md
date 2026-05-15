# SYSTEM INTERFACE REDESIGN

## Purpose

Reshape the system's surface — controllers, DTOs, routes, status codes, error format; or page structure, form fields, navigation, copy, selectors; or the equivalents for any other channel the student repo exposes (mobile, CLI, admin, …). The driver-adapter absorbs the surface change so DSL, Gherkin, and tests stay untouched.

The `subtype:system-interface-redesign` label routes a Task ticket through this cycle. The framework no longer pre-classifies the channel: the WRITE agent reads the ticket body's Checklist plus the system tree to decide which driver(s) — under `driver-port/.../shop/<channel>` and `driver-adapter/.../shop/<channel>` — to modify. Channel folders are listed by example, not exhaustively (e.g. `shop/api`, `shop/ui`, and any others the repo defines such as `shop/mobile`, `shop/cli`, `shop/admin`).

## What it produces

After WRITE: only `system/` and `driver-adapter/` edits (and, exceptionally, approved `driver-port/` edits). All in-scope parallel implementations updated (Java/.NET/TS × monolith/multitier — see [architecture/system.md](../architecture/system.md)).

## SYSTEM INTERFACE REDESIGN - WRITE

**Goal:** the System Driver(s) the ticket targets (interface + impl under `driver-port/.../shop/<channel>` and `driver-adapter/.../shop/<channel>`) reflect the new system surface; the system code under `system/` reflects the new surface; existing acceptance and contract tests still compile.

1. Read the Checklist and the system tree to decide which driver(s) the ticket targets, identified by the agent from the Checklist. Do NOT assume API or UI; let the Checklist and the system layout pick the channel(s). Examples: `shop/api` for a backend HTTP API, `shop/ui` for the web UI, `shop/mobile` for a mobile channel, `shop/cli` for a CLI, `shop/admin` for an admin UI — and so on for any other channel folder under `driver-adapter/.../shop/`.
2. Update the system surface under `system/` to match the ticket's Checklist. The shape of the change depends on the channel(s) identified in step 1:
   - For an API channel: controllers, request/response DTOs, routes, status codes, error format. Apply across **all parallel implementations** — see [architecture/system.md](../architecture/system.md) for the layout (Java/.NET/TS × monolith/multitier) and for where API URLs and their consumers live in each implementation. After editing the source of truth, grep the system tree for residual references (e.g. the old URL string) before moving on.
   - For a UI channel: page structure, form fields, navigation, copy, selectors.
   - For another channel: the channel-specific equivalents (commands/flags for CLI, screen/component layout for mobile, admin pages, …).
3. Update the matching System Driver implementation(s) (`driver-adapter/.../shop/<channel>`) to absorb the change. Prefer adapter-only changes — keep behaviour observable through the **existing** driver interface.
4. **Driver interface guardrail.** Do NOT modify any file under `driver-port/` casually. If an interface change is unavoidable, STOP separately at that boundary and present to the user: the method(s) you want to change, why the adapter alone cannot absorb the change, the proposed new signature(s). Wait for explicit user approval before editing any `driver-port/` file. (Such changes have no contract-test fallout because this is `shop/`, not `external/` — but they still touch the test surface and must be approved.)
5. Do not modify acceptance tests, DSL, Gherkin, or any code outside the system layer + its driver. `system-test/<lang>/.../Legacy/` is read-only course-reference material — leave it untouched.

## Anti-patterns

- Pre-classifying the channel (API vs UI vs …) instead of letting the Checklist + system tree drive it.
- Editing under `driver-port/` without an explicit guardrail STOP and approval.
- Changing tests / DSL / Gherkin instead of system code + adapter.
- Forgetting to grep for residual references to the old URL / selector / channel-specific strings.
- Touching `system-test/<lang>/.../Legacy/` (read-only).
- Updating one implementation but leaving the other parallel implementations drifting.
