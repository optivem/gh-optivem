# Layer paths in `TierSpec` (deferred)

## Status

**Deferred — pick up only when an actual incident motivates it.** This plan is filed in `plans/deferred/` rather than `plans/` because no real-world divergence has been observed yet; all motivation below is hypothetical. The trigger to move it back to `plans/` is the first time a scaffolded repo's renamed/moved layer directory causes an ATDD agent to wander or create a phantom directory.

## Context

The active plan `plans/20260505-100000-scope-paths-and-implement-ticket-preflight.md` introduces tier-level paths in `gh-optivem.yaml` (`system.path`, `system_test.path`, `system.backend.path`, `system.frontend.path`, plus `external_systems.{stubs,simulators}.path`). Those paths are injected into the agent prompt as "Allowed write roots" and validated by preflight before dispatch. They solve the cross-stack-bleed bug exposed by the 2026-05-05 atdd-rehearsal of `optivem/shop#61` (agent edited code across all four stacks despite scope being `monolith + java`).

Within each tier, the gh-optivem scaffold lays out conventional **layer** subdirectories that the per-agent prompts reference by name:

- `dsl-core/` — DSL Given/When/Then implementations and step classes (referenced by `atdd-dsl.md`)
- `driver-port/` — Driver interface declarations (referenced by `atdd-driver.md`, `atdd-dsl.md`)
- `driver-adapter/` — Driver interface implementations (referenced by `atdd-driver.md`)
- `external/` and `shop/` subtrees within `driver-port/` and `driver-adapter/` — split between external-system Drivers (CT cycle) and System Drivers (AT cycle), referenced across `atdd-driver.md` and `atdd-dsl.md`
- Test source roots (referenced by `atdd-test.md`)
- Stub configuration roots within `external_systems.stubs.path` (referenced by `atdd-stubs.md`)

Today the per-agent prompts hardcode these layer names as literal convention strings — e.g. `atdd-driver.md:97` says *"only files under `driver-port/` and `driver-adapter/` paths under `shop/`"*. The agent translates that convention into actual paths at dispatch time by Globbing/Reading the tier directory.

## Hypothetical motivation

Two failure modes the current plan does not address:

1. **Layer-rename divergence.** A student or team renames `dsl-core/` → `dsl/`, or relocates `driver-port/` to a non-standard parent. The agent's first Glob on the convention name misses; the agent wanders, possibly edits the wrong dir, or creates a *new* `dsl-core/` directory alongside the renamed one — the same class of bug as the cross-stack bleed, just at a finer grain. Estimated cost when it bites: ~10–20k tokens of exploration plus a wrong-path commit at the end.

2. **Token cost when the convention holds.** Even when the consumer hasn't diverged, each agent dispatch pays 1–3 Glob/Read calls to translate convention names into real paths. Multiplied across the per-cycle agents (`dsl`, `driver`, `test`, `system`/`backend`/`frontend`, `stubs`), this accumulates. Per-dispatch savings are modest (~2–5k tokens) but they recur on every ticket.

Both are real. Neither is a known incident yet — only (1) would have a clear before/after the way rehearsal #61 did.

## Why this is deferred, not part of the active plan

1. **No incident has occurred.** The active plan's tier-level fix is justified by a concrete rehearsal failure. The layer-level analogue is justified only by symmetry of reasoning. The repo guideline against speculative scope (CLAUDE.md spirit + the active plan's own "Out of scope" discipline) argues for waiting.

2. **Schema cost is non-trivial.** Each layer path needs validation (path is relative, no `..`, language-appropriate), preflight checks (directory exists under the tier's host repo), scaffold-mapping table entries (per architecture × language), and round-trip tests. The active plan already pays this cost six times over (system, system_test, backend, frontend, stubs, simulators); doubling or tripling it for layer paths is sizable.

3. **It is non-breakingly addable later.** `TierSpec` can grow optional `layers:` sub-fields without any migration cost — existing configs would continue to validate untouched, since every new field would be optional. This is the key reason deferral is safe: the active plan is not closing this door.

4. **Layer naming is a scaffold convention, not a consumer-tunable one.** Every layer name (`dsl-core`, `driver-port`, `driver-adapter`, `external`, `shop`) appears verbatim in per-agent prompts AND in the cycles documentation in the `shop` repo. Encoding them in user config implies the consumer can rename them — but if they do, prompts and docs are also stale and would need a separate fix. Solving only the config side gives a misleading half-fix.

## What this plan would do, when picked up

Sketch only — the eventual plan will need its own full design pass, but the shape is:

1. **Extend `TierSpec` with optional layer paths.** Add a `Layers` sub-struct to `TierSpec`, populated only when present in YAML:

   ```go
   type TierSpec struct {
       Path   string     `yaml:"path,omitempty"`
       Repo   string     `yaml:"repo,omitempty"`
       Lang   string     `yaml:"lang,omitempty"`
       Layers TierLayers `yaml:"layers,omitempty"`
   }

   type TierLayers struct {
       DSL                  string `yaml:"dsl,omitempty"`
       DriverPortShop       string `yaml:"driver_port_shop,omitempty"`
       DriverPortExternal   string `yaml:"driver_port_external,omitempty"`
       DriverAdapterShop    string `yaml:"driver_adapter_shop,omitempty"`
       DriverAdapterExternal string `yaml:"driver_adapter_external,omitempty"`
       Test                 string `yaml:"test,omitempty"`
   }
   ```

   Each field is optional. When absent, the runtime falls back to scaffold convention (`<tier.path>/dsl-core`, `<tier.path>/driver-port/shop`, etc.). When present, the explicit override wins.

   The five field names above are illustrative — the eventual plan will reconcile them with the actual scaffold layout (which language conventions live where, whether `driver_port_shop` and `driver_adapter_shop` are needed separately or collapse, whether the `external` split applies to all tiers or only `system_test`).

2. **Per-agent prompt rendering uses layer paths.** `renderAllowedRoots` (currently rendered once for the whole task) gains per-agent variants that emit only the layers relevant to that agent's scope:

   - `atdd-dsl.md` template gets a `${dsl_root}` and `${driver_port_root}` substitution.
   - `atdd-driver.md` template gets `${driver_port_shop_root}` and `${driver_adapter_shop_root}` for AT phases, `${driver_port_external_root}` and `${driver_adapter_external_root}` for CT phases.
   - `atdd-test.md` template gets `${test_root}`.
   - `atdd-stubs.md` template gets `${stubs_root}` (already implicitly available via `external_systems.stubs.path`, but layer-aware to surface the per-stub-collection structure if any).

   Substitutions are computed by a small renderer that consults the tier's `Layers` field if set, otherwise falls back to convention paths.

3. **Preflight extends to layer paths.** When `TierLayers` fields are explicitly set, preflight verifies each named directory exists under its host repo. When not set (convention path), preflight does NOT check — the agent's first action will surface a missing convention directory clearly enough, and adding a stat per convention is brittle if convention drifts (e.g. for languages the scaffold supports later).

4. **Scaffold continues to emit the convention layout silently.** `buildOptivemYAML` in `internal/steps/optivem_yaml.go` does NOT emit a `layers:` block — convention is enough at scaffold time. Operators fill in `layers:` manually only when they diverge. This keeps the freshly-scaffolded `gh-optivem.yaml` short and readable.

5. **Per-agent prompt rewrites.** Replace literal convention strings in `atdd-dsl.md`, `atdd-driver.md`, `atdd-test.md`, `atdd-stubs.md` with the substituted placeholders. The convention strings remain as fallbacks in the renderer, not in the prompts.

## Out of scope (within this deferred plan)

- **A `gh optivem config layers detect` command** that scans a tier directory and writes the corresponding `layers:` block. Probably not worth automating — divergence is rare, and operators editing a config to capture a rename can do it by hand.
- **Per-language layer naming.** `.NET` and TypeScript scaffolds may use different layer names than Java. Whether the override is per-tier-per-language or shared across languages is a question for the live plan, not this deferred sketch.
- **Migration tooling.** Adding `layers:` is non-breaking; configs without it continue to validate. No migrate command needed.

## Trigger to move this back to `plans/`

Any of:

1. A real divergence incident — a student renamed `dsl-core/` (or any other layer dir) and the ATDD agent wandered or created a phantom directory.
2. A measured, recurring token-cost concern from layer-finding Globs across the per-cycle agents that justifies the schema cost on its own (e.g. the per-agent dispatch budget tightens enough that 2–5k tokens per agent matters).
3. The scaffold gains a frontend framework variant beyond React, or a non-conventional layer (e.g. a `verification/` layer added to DSL), that requires per-tier overrides anyway — at which point the schema work this plan describes is partially needed for unrelated reasons and may as well land complete.

Until one of these triggers, ship the active scope-paths plan as-is and revisit this only when reality pushes back.
