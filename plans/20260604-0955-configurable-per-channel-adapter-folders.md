# Configurable per-channel System Driver adapter folders

**Created:** 2026-06-04 09:55 CEST
**Status:** Idea / not started. Spun off from
`plans/20260530-1725-scoped-implement-by-layer-channel.md` during execution of
its Items 2b–2d (the git-state resume detector needs to know the per-channel
adapter footprint).

## TL;DR

**Why:** The System Driver test adapters are physically split by channel folder
— `driver/adapter/api`, `driver/adapter/ui`, `driver/adapter/external`,
`driver/adapter/shared` — and each channel folder is owned by a *different
team* (API team, UI team, external-system team; `shared` is common to all). But
gh-optivem models the adapter layer as a single `driver-adapter` path key
(`internal/projectconfig/paths_defaults.go`, `CanonicalPathKeys`), so the
config cannot express the per-team folder ownership, and scope/footprint
resolution cannot narrow to one channel's adapter subtree by name.
**End result:** A way to declare the per-channel adapter folders in
`gh-optivem.yaml` (or derive them deterministically from the `channels:` SSoT
under the `driver-adapter` root), so per-channel scope, the scoped-`implement`
resume detector (1725 Items 2c/2d), and per-team ownership all key on a real,
configured path instead of a string convention.

## Problem

`channels.go` documents channels as distinguished by *class name*
(`MyShopApiDriver` / `MyShopUiDriver`). In the actual testkit tree they are
distinguished by *folder*:

```
.../driver/adapter/api        # API team
.../driver/adapter/ui         # UI team
.../driver/adapter/external   # external-system team
.../driver/adapter/shared     # common to all channels
```

The single `driver-adapter` path key resolves to the parent `driver/adapter`;
the per-channel and `shared` subfolders are an undocumented convention the
config cannot see. Consequences:

- **Scope can't be per-channel.** `implement-system-driver-adapters`'s
  `write: [driver-adapter]` covers *all* channel subfolders at once, so the
  per-channel `--target driver-adapter --channel <ch>` slice (1725) cannot
  assert it wrote only its own channel's folder via the existing
  `check-phase-scope` path-prefix machinery.
- **Resume detection narrows by string-join, not config.** The 1725 footprint
  detector currently has to compute `path.Join(paths["driver-adapter"], ch)`
  and assume the `shared` / `external` siblings exist — a convention baked into
  code rather than declared.
- **Ownership is invisible.** The folder-per-team split is the real ownership
  boundary, but nothing in config records which folder belongs to which team.

## Sketch of options (to weigh later)

1. **Derive by rule from `channels:` + a `shared`/`external` convention.** Keep
   one `driver-adapter` root key; define per-channel folders as
   `<driver-adapter>/<ch>` plus the fixed `shared` (+ `external`) siblings.
   Cheapest; encodes the convention once, in one resolver, instead of scattered.
   Deterministic-path-construction doctrine applies (no guessing — pin against
   the shop template).
2. **Explicit per-channel keys in `system-test.paths:`.** e.g.
   `driver-adapter-api`, `driver-adapter-ui`, `driver-adapter-shared`,
   `driver-adapter-external`. Fully configurable, ownership explicit, but
   multiplies the canonical key set and the `DefaultPaths` writer.
3. **A nested `driver-adapter:` block** mapping channel → folder, with `shared`
   / `external` reserved members. Configurable and structured, but a new YAML
   shape the validator + `PlaceholderMap` must learn.

## Notes / constraints

- The `external` adapter already has its *own* canonical key
  (`external-system-driver-adapter` → `src/testkit/external/adapter` in the
  default scaffold), so the testkit's `driver/adapter/external` folder and the
  canonical `external/adapter` key may be two different things — reconcile the
  actual tree against `CanonicalPathKeys` before choosing an option.
- Honour the scaffold-authoritative-then-operator-owned doctrine
  (`internal/projectconfig/path-keys.md`): if new keys are added, `init` writes
  them once and `migrate` does not back-fill.
- Relationship to enforcement: per-team folder ownership is the informal split
  1725 expresses — configured per-channel folders are the shared substrate it
  needs.

## Related

- `plans/20260530-1725-scoped-implement-by-layer-channel.md` — the scoped
  `implement` work whose resume detector surfaced this; channel narrowing keys
  on the per-channel adapter subtree.
- `internal/projectconfig/paths_defaults.go` / `path-keys.md` — the canonical
  path-key vocabulary this would extend or refine.
