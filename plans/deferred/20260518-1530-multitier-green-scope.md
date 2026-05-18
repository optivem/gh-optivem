# Plan (deferred): Multitier GREEN scope in phase-scopes.yaml

**Date filed:** 2026-05-18

**Filed from:** [SSoT phase-scope plan (20260518-1530)](../20260518-1530-atdd-phase-scope-ssot.md), item 1's AT-GREEN handling refinement.

## Why deferred

The SSoT phase-scope plan introduces `internal/atdd/phase-scopes.yaml` as the doctrinal source of per-phase allowed-paths. `internal/atdd/runtime/statemachine/process-flow.yaml` has three GREEN `user_task` nodes:

- `AT_GREEN_SYSTEM` — monolith GREEN (touches `system.path`)
- `AT_GREEN_BACKEND` — multitier GREEN, backend tier (touches `system.backend.path`)
- `AT_GREEN_FRONTEND` — multitier GREEN, frontend tier (touches `system.frontend.path`)

The SSoT plan encodes scope only for `AT_GREEN_SYSTEM`. The other two are knowingly unmapped, with an explicit allowlist entry in the build-time cross-validator (item 11 of the SSoT plan) citing this plan.

The reason for deferral: encoding `AT_GREEN_BACKEND: [system_path]` would be factually wrong (multitier projects have no `system.path` — they have `system.backend.path` and `system.frontend.path`). Doing it right requires deciding the layer-key vocabulary for the multitier path-shaped Family A keys, which is its own design problem that touches Family A schema rules, BPMN gating, and scope rules together.

## Open design questions to resolve when this plan is picked up

1. **Layer-key names.** Two options surfaced during refinement:
   - (a) Add `system_backend_path` and `system_frontend_path` as new Family A path-shaped keys. Symmetric with `system_path`; explicit.
   - (b) Reuse `system_path` with architecture-conditional resolution: monolith resolves it from `system.path`; multitier resolves it from whichever tier the current phase is scoped to (BACKEND vs FRONTEND). One key, dual meaning — risky for `check_phase_scope`'s diff matching.
   - (c) Something else (e.g. namespacing under `tier_path:{name}`).
2. **`PlaceholderMap()` impact.** `Config.PlaceholderMap()` (in `internal/projectconfig/config.go`) currently emits `system_path` only for monolith. Multitier emits an empty `system_path`. Either fix the map (per option a above) or change the resolver in `check_phase_scope` (per option b).
3. **Predecessor plans to consult.** [`plans/deferred/20260505-110000-layer-paths-in-tier-spec.md`](20260505-110000-layer-paths-in-tier-spec.md) is the obvious sibling — it's been parked separately and may already have done some of this thinking. Cross-reference before designing anew.
4. **`system_test` tier.** `gh-optivem.yaml` already has a `SystemTest TierSpec` field separate from `system.backend` / `system.frontend`. Does it need its own scope row, or is it covered by the AT-RED-* `at_test` / `ct_test` Family B keys? Inspect at pickup.

## Pre-requisites

- SSoT phase-scope plan ([20260518-1530](../20260518-1530-atdd-phase-scope-ssot.md)) must land first. This plan amends, not replaces.
- Inspect `check_phase_scope` action implementation (per SSoT plan's item 8 rewiring) to understand the current resolver shape before adding architecture-conditional logic.

## Out of scope for this deferred plan

- Re-litigating the standalone vs folded location for `phase-scopes.yaml` (decided in SSoT plan: standalone).
- Re-litigating the fully-resolved-paths migration (decided in SSoT plan: fully resolved, no runtime `${...}` substitution).
- Microservices — `architecture:` enum currently supports only `monolith` and `multitier`. If/when a third architecture is added, that's a further extension.

## Hand-off

Pick up when: (a) SSoT phase-scope plan has landed and stabilised, AND (b) someone needs the multitier scope check to actually work (today the `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` paths flow through `check_phase_scope` without per-tier guarding).
