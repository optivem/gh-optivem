# Plan: migrate the shop template configs to the per-system `external-systems:` map

## TL;DR

**Why:** Plan `20260606-1356` moved the gh-optivem schema from the flat two-tier
`external-systems.{stubs,simulators}` model to a per-system map (`external-systems.<name>`
carrying `real-kind` + `stub` + optional `simulator`). The 12 checked-in shop template
configs in the **sibling `shop` repo** still use the old flat shape, so once the shop picks
up the new gh-optivem binary, `gh optivem` config-load/validate rejects them and shop CI
goes red.
**End result:** all 12 `shop/gh-optivem-<arch>-<lang>.yaml` files carry a per-system
`external-systems:` map that loads + validates clean under the new schema; shop CI is green.

## Why

The pre-1356 shape (still in every shop config today):

```yaml
external-systems:
    stubs:
        path: external-systems/stubs
        repo: optivem/shop
    simulators:
        path: external-systems/simulators
        repo: optivem/shop
```

The 1B schema (locked in 1356) replaces the two shared tiers with one entry **per external
system**, keyed by name:

```yaml
external-systems:
    <name>:
        real-kind: simulator        # test-instance | simulator
        stub:
            path: external-systems/stubs/<name>     # or the operator's chosen path
            repo: optivem/shop
        simulator:                    # present iff real-kind: simulator
            path: external-systems/simulators/<name>
            repo: optivem/shop
```

`ExternalSystems` is now `map[string]ExternalSystem` (`internal/projectconfig/config.go`),
and `Validate` enforces: `real-kind` required + in `{test-instance, simulator}`; `stub`
always present; `simulator` present **iff** `real-kind: simulator`. The old `stubs:` /
`simulators:` keys are no longer recognised, so they must be rewritten, not just left.

## Scope — cross-repo (sibling `shop` repo)

These files live in `../shop/`, **not** in gh-optivem. 12 files:

- `gh-optivem-monolith-{dotnet,java,typescript}.yaml`
- `gh-optivem-monolith-{dotnet,java,typescript}-legacy.yaml`
- `gh-optivem-multitier-{dotnet,java,typescript}.yaml`
- `gh-optivem-multitier-{dotnet,java,typescript}-legacy.yaml`

All 12 currently carry the old flat block (confirmed 2026-06-06). The commit lands in the
**shop repo**; coordinate the merge/release timing with the gh-optivem version the shop pins.

## Open question — system name(s) + real-kind (needs shop-domain input)

The old flat shape carries **no per-system name**, so the migration cannot be purely
mechanical — someone has to name the external system(s) the shop actually talks to and
declare each one's `real-kind`. Resolve before executing:

1. **What external system(s) does the shop integrate?** (one map entry each.) The old block
   had a single shared stubs+simulators tier, suggesting **one** external system — but
   confirm against the shop's testkit (`shop/external-systems/...`, the driver-adapter
   `external/<name>/` folders).
2. **`real-kind` per system.** The old shape declared *both* a stubs path and a simulators
   path, which maps to `real-kind: simulator` (simulator block present). If a system is
   actually backed by a live vendor sandbox, it is `real-kind: test-instance` and its
   `simulator:` block is omitted.
3. **Paths.** Decide whether to keep the flat `external-systems/stubs` +
   `external-systems/simulators` roots (then per-system subdirs) or restructure. The schema
   only requires each `stub`/`simulator` to have `path` + `repo`; no implicit subdir
   convention is imposed (1B decision).

## Steps

1. Resolve the open question above with whoever owns the shop testkit (name + real-kind +
   paths per external system).
2. For each of the 12 configs, rewrite the `external-systems:` block from the flat two-tier
   shape to the per-system map. Map old → new: `stubs.{path,repo}` →
   `<name>.stub.{path,repo}`; `simulators.{path,repo}` → `<name>.simulator.{path,repo}`; add
   `real-kind:`. (The `-legacy` variants take the same map — there is no legacy-specific
   external-systems shape.)
3. Validate each file against the **new** gh-optivem binary (built from this repo's `main`
   at/after commit `b59aed9`).

## Verification

- For each config: `gh optivem <validate-or-config-load> --config <path>` (or the parity
  harness the shop CI uses) loads clean — no "unknown field stubs/simulators", no
  "real-kind required", no "simulator present iff" violation.
- Shop CI green once it pins the gh-optivem version carrying the 1B schema.

## Cross-references

- Parent plan (schema + CT flow): `plans/20260606-1356-external-system-real-kind-simulator-ct.md`
  (the residual "Shop template config (cross-repo)" item this plan spins out).
- Schema SSoT: `internal/projectconfig/config.go` (`ExternalSystems`, `ExternalSystem`,
  `RealKind`, `Validate`).
- Old-shape reference: `../shop/gh-optivem-monolith-typescript.yaml` (lines ~34–40 at time
  of writing).
