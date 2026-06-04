# Configurable per-channel System Driver adapter folders

**Created:** 2026-06-04 09:55 CEST
**Status:** Design locked 2026-06-04 (refine pass) — **explicit nested
per-channel block** (`system-driver-adapter-channels`), scaffolder-written
per-language, members tied to `channels:`. Ready to execute **after** its
prerequisite rename lands. Spun off from
`plans/20260530-1725-scoped-implement-by-layer-channel.md` during execution of
its Items 2b–2d (the git-state resume detector needs the per-channel adapter
footprint).

**Depends on:** `plans/20260604-1106-rename-driver-keys-to-system-driver.md` —
this plan's new key is `system-driver-adapter-channels`, so the
`driver-* → system-driver-*` rename must land first. Execute the rename plan,
then this one.

## TL;DR

**Why:** The System Driver test adapters are physically split by channel folder
— `driver/adapter/api`, `driver/adapter/ui`, … — each owned by a *different
team*. But gh-optivem models the adapter layer as a single `system-driver-adapter`
path key, so the config cannot express the per-team folder ownership, and
scope/footprint resolution narrows to a channel's subtree by a runtime
`path.Join` string convention in `scoped.go` rather than by a configured path —
which cannot carry per-language folder casing/structure.
**End result:** a nested `system-driver-adapter-channels:` map of
`channel → fully-resolved adapter path`, written per-language by the scaffolder
at `init` and operator-owned afterward, sitting alongside the retained
whole-layer `system-driver-adapter` root. Per-channel scope, the
scoped-`implement` resume detector (1725 Items 2c/2d), and per-team ownership all
key on a real, configured, correctly-cased path — and the runtime `path.Join`
smell is removed, restoring the resolve-at-scaffold / read-verbatim doctrine the
other path keys already follow.

## Decision (locked 2026-06-04)

**Explicit nested per-channel block, encoding (A).** Keep `system-driver-adapter`
as the flat whole-layer string and add a sibling nested
`system-driver-adapter-channels:` map (`channel → path`). The scaffolder writes
the correctly-cased per-language value for each channel at `init` (extend
`pathStems`); `migrate` does not back-fill; a validator ties the members to
`channels:`. `scoped.go` and the per-channel write-scope read the configured
member **verbatim** — no runtime path construction.

```yaml
system-test:
  channels: [api, ui]
  paths:
    system-driver-port:             system-test/typescript/src/testkit/driver/port/shop
    # Whole adapter layer (root). Broad writers (DSL stub-writer IMPL_DSL,
    # implement-system, update-system) scope here; shared/common adapter code
    # lives directly under it — the residual, NOT a declared member.
    system-driver-adapter:          system-test/typescript/src/testkit/driver/adapter/shop
    # Per-channel members — the narrow `--channel` write-scope + resume footprint.
    # Each owned by its channel team. Members must match `channels:`.
    system-driver-adapter-channels:
      api:                          system-test/typescript/src/testkit/driver/adapter/shop/api
      ui:                           system-test/typescript/src/testkit/driver/adapter/shop/ui
    external-system-driver-port:    system-test/typescript/src/testkit/external/port/shop
    external-system-driver-adapter: system-test/typescript/src/testkit/external/adapter/shop
    at-test:                        system-test/typescript/tests/latest/acceptance
    dsl-port:                       system-test/typescript/src/testkit/dsl/port/shop
    dsl-core:                       system-test/typescript/src/testkit/dsl/core/shop
    ct-test:                        system-test/typescript/tests/latest/contract
```

For .NET the members are written **project-shaped** (e.g.
`Testkit.Driver.Adapter.Api`), pinned against the shop template — NOT a lowercase
`<root>/<channel>` join.

**Why explicit, not derived (rejected).** Per-language folder *casing and
structure* make `<root>/<channel>` non-derivable: TS/Java use lowercase
subfolders, but .NET is PascalCase and **project**-based, and a team may
restructure. A runtime `path.Join` cannot carry that shape — only scaffold-time
resolution can, which is the doctrine the other keys follow (`path-keys.md`:
resolve fully at `init`, store, read verbatim). Deriving would also perpetuate
the runtime-join smell the Problem section exists to remove.

**Why nested members, not flat per-channel keys (rejected).** `channels:` is a
variable subset (an api-only project has no `ui`), so a *fixed* flat
`CanonicalPathKeys` set cannot hold variable members; a map keyed by channel can.

**Why root retained + `shared` residual.** Broad writers scope to the entire
adapter layer (where shared/common code lives), so the whole-layer
`system-driver-adapter` root is mandatory. `shared` is the residual under it —
no phase scopes to "shared only", so it earns no schema slot
(`feedback_schema_fields_earn_slot`); its team-ownership note is a YAML comment,
not a key.

**Encoding (B) — a unified `system-driver-adapter: {base, api, ui}` map — is the
documented fallback** if a single string root proves insufficient (e.g. a .NET
layout with no containing folder); rejected as default because it changes the key
from string→map, rippling through the config struct, validator, and the flat
`PlaceholderMap`.

**Scope boundaries:** `external` is **deferred** to `reconcile-defaultpaths`
(Notes + Related); the `driver-* → system-driver-*` rename is a **separate
prerequisite plan** (above).

## Problem

`channels.go` documents channels as distinguished by *class name*
(`MyShopApiDriver` / `MyShopUiDriver`). In the actual testkit tree they are
distinguished by *folder*:

```
.../driver/adapter/api        # API team
.../driver/adapter/ui         # UI team
.../driver/adapter/shared     # common to all channels (residual)
```

The single `system-driver-adapter` key resolves to the parent `driver/adapter`;
the per-channel subfolders are an undocumented convention the config cannot see.
Consequences:

- **Scope can't be per-channel.** `implement-system-driver-adapters`'s
  `write: [system-driver-adapter]` (process-flow.yaml) covers *all* channel
  subfolders at once, so the per-channel `--target driver-adapter --channel <ch>`
  slice (1725) cannot assert it wrote only its own channel's folder via the
  existing `check-phase-scope` path-prefix machinery.
- **Resume detection narrows by string-join, not config.** `driverAdapterFootprint`
  (scoped.go) computes `path.Join(paths["system-driver-adapter"], ch)` — a
  convention baked into code rather than declared, and one that cannot carry
  per-language folder casing/structure.
- **Ownership is invisible.** The folder-per-team split is the real ownership
  boundary, but nothing in config records which folder belongs to which team.

## Items

5. **process-flow scope narrowing.** Narrow `implement-system-driver-adapters`'s
   `write: [system-driver-adapter]` so the channel-split dispatch resolves its
   write-scope to the channel's member, enabling the scope checker to assert it
   wrote only its own channel's folder. Broad/non-channel writers keep the
   whole-layer scope.

   **Pin (decided during 2026-06-04 execution — needs operator sign-off before
   implementing):**
   - **Live consumer is `validate-outputs-and-scopes`** (`bindings.go` ~1049),
     which resolves `Engine.Scope(taskName).write` via `ResolveLayerPaths(write,
     cfg)` and prefix-checks the dispatch's diff. `check-phase-scope` is
     **dormant** (no `phase-id` / `check_phase_scope` node wired in
     `process-flow.yaml`), so it is not a live consumer today.
   - **Shared-node constraint.** The `implement-system-driver-adapters` process
     node carries a single static `write:` list and is reused by BOTH the
     channel-split dispatch (`ctx.Params["channel"]` set, via the
     `UnrollSystemDriverAdapterChannels` clone) AND the no-`channels:` full run
     (no channel param). One static list must serve both, so narrowing MUST be
     conditional on the runtime channel — it cannot be expressed by rewriting the
     list to a member token (the no-channel run would then fail to resolve
     `${channel}`).
   - **Two candidate mechanisms** (operator to choose):
     - **(i) Implicit narrowing in the resolver.** Keep `write:
       [system-driver-adapter]`. Teach `validate-outputs-and-scopes` (and
       dormant `check-phase-scope`) to substitute the channel's member for the
       `system-driver-adapter` path when `ctx.Params["channel"]` is non-empty.
       Localized; no process-flow edit; preflight's static scope sweep
       (`runScopeResolutionChecks`, no channel) is unaffected.
     - **(ii) Parameterized member token.** Add a
       `system-driver-adapter-channels.${channel}` token plus a params-aware
       `ResolveLayerPaths` variant that substitutes `${channel}` from
       `ctx.Params` then looks up the member (already emitted by
       `PlaceholderMap`). Requires the preflight static sweep to **skip**
       `${`-containing tokens (mirrors how it already skips templated
       task-names), and still needs a whole-layer fallback for the no-channel
       run — so it does not actually remove the shared-node conditional.
   - Recommendation: **(i)** — it respects the shared-node constraint with the
     least surface area and touches no static scope list. **(ii)**'s explicit
     token reads nicer but doesn't escape the conditional and adds a sweep
     special-case.
   - **Lower urgency:** the live scope check only tightens (today it allows the
     whole adapter layer for a channel dispatch, which is permissive, not
     wrong); the resume guard (Item 4, landed) already keys on the real member.

**Correction to the Decision (found during execution):** the actual shop
template writes the .NET channel split as **PascalCase subfolders inside one
`Driver.Adapter` project** (`Driver.Adapter/Api`, `Driver.Adapter/Ui`), **not**
as separate `Testkit.Driver.Adapter.Api` projects as the Decision's example
claims. All three languages are therefore a `<root>/<channel>` join differing
only by **casing** (TS/Java lowercase, .NET PascalCase) — which is what the
landed scaffolder (`DefaultSystemDriverAdapterChannels`) encodes, deriving each
member from the configured `system-driver-adapter` root so they track it
through the pending `reconcile-defaultpaths` root fix. The "explicit not derived"
decision still stands (per-language casing genuinely can't be a single lowercase
join, and a team may restructure); only the project-vs-subfolder example was
wrong.

## Notes / constraints

- **Prerequisite rename.** All key names above assume
  `plans/20260604-1106-rename-driver-keys-to-system-driver.md` has landed. If the
  rename is dropped, substitute `driver-adapter` / `driver-adapter-channels`.
- **`external` is out of scope and entangled with `reconcile-defaultpaths`.** The
  archived `testkit-architecture-rules.md` puts the external-system adapter at
  `driver/adapter/external/<system>/…` (nested), but current `pathStems` puts it
  at `src/testkit/external/adapter` (a sibling, its own canonical key). Which the
  shop template actually uses is exactly the open question of
  `plans/backlog/20260526-1430-reconcile-defaultpaths-with-shop-template-layout.md`.
  Do **not** resolve it here; coordinate.
- **`shared` is a residual, not a member** — see Decision.
- Honour scaffold-authoritative-then-operator-owned (`path-keys.md`): `init`
  writes members once, `migrate` does not back-fill.
- The `channels:` ↔ members tie is a two-place join (a known drift source);
  Item 3's validator keeps them consistent.
- **Nested members live outside the flat `Paths` map / `CanonicalPathKeys()`**
  (true for both encodings A and B). Every consumer that iterates those —
  preflight existence (Item 6), the validator (Item 3), scope resolution
  (Item 5) — must be explicitly taught about the members; none picks them up for
  free. This is the main implementation tax of going nested vs. flat keys.

## Related

- `plans/20260604-1106-rename-driver-keys-to-system-driver.md` — prerequisite
  rename.
- `plans/20260530-1725-scoped-implement-by-layer-channel.md` — the scoped
  `implement` work whose resume detector surfaced this.
- `plans/backlog/20260526-1430-reconcile-defaultpaths-with-shop-template-layout.md`
  — semantic partner per the 2026-06-04 triage; owns the `external` layout
  question. Run **with** this plan, not against it.
- `plans/20260604-1024-backlog-conflict-triage-vs-1725-scoped-implement.md` — the
  triage that ruled these coordinate (🟡 tier).
- `internal/projectconfig/paths_defaults.go` / `path-keys.md` — the canonical
  path-key vocabulary this extends.
