# Reconcile `DefaultPaths` emission with shop-template disk layout

**Filed:** 2026-05-26 (deferred from the optivem/shop yaml-correction round)
**Revised:** 2026-06-04 — the original framing (wrong SUT-namespace *leaf* casing, `shop` vs `myShop`) is obsolete. The per-channel driver-adapter decomposition (gh-optivem commit `c1ba55b`, "implement(scoped): per-channel driver-adapter decomposition") removed the SUT-namespace leaf from the template entirely. The mismatch is now *structural*, not a casing fix. Problem statement, options, and open questions rewritten against the current on-disk template.

> **At-a-glance (2026-06-04 review):** `gh optivem init` clones the shop template but `DefaultPaths` computes the `paths:` block from scratch (repo slug + per-language stems) without reading the cloned tree — so six (TS/.NET) / eight (Java) keys point at directories that don't exist, and the scope checker rejects the first real edit. After the per-channel decomposition the divergence is now *structural* (spurious `/shop` leaf, wrong `external/*` nesting, missing Java `com/<org>/<sut>/` package prefix, spurious `.NET Testkit.` prefix), not a casing nit. Options narrowed to two: **C** — `init` reads the template's own yaml as SSoT, `DefaultPaths` demoted to fallback (recommended); **A′** — rewrite `pathStems` to mirror the template's structural shape statically.
>
> **Verdict: KEEP — still live.** Verified `paths_defaults.go` still appends `sutNamespace` and hard-codes the `testkit/`/`Testkit.` prefixes; unresolved in code. **Caveat:** answer the plan's own open question first — *is `gh optivem init` even invoked anymore, or has the template's checked-in yaml made it dead code?* If init is effectively dead, this drops to near-zero priority.

## Cross-references

- **`internal/projectconfig/path-keys.md`** ("Ownership: scaffold-authoritative at `init`, operator-owned afterwards", lines 76–101 + the worked TS example lines 119–126) — the doctrine this plan stress-tests. The example still shows the obsolete `…/driver/adapter/shop` leaf.
- **`internal/projectconfig/paths_defaults.go::DefaultPaths` / `pathStems`** — the emission point. Still appends `sutNamespace` as a trailing directory segment to the six testkit keys, and still hard-codes a `testkit/` prefix (Java) and `Testkit.` project prefix (.NET).
- **`internal/projectconfig/paths_defaults_test.go`** — pins the buggy shape today (`…/driver/adapter/shop` etc., lines 12–19 / 30–37 / 48–55). Must be rewritten alongside the body.
- **`internal/steps/optivem_yaml.go::BuildOptivemYAML`** (lines 191–204) — the only caller in the binary. Derives `sutNamespace := lastSlashSegment(systemRepoSlug(cfg))` (line 191) and passes it to `DefaultPaths` (line 204). *Verified accurate as of this revision.*
- **`plans/20260530-1725-scoped-implement-by-layer-channel.md`** — the per-channel `--target driver-adapter --channel <ch>` work. Confirms `driver-adapter` stays a **single** Family-B path key; the channel (`api`/`ui`) is a runtime sub-dimension scoped *under* that path, not a new key. So the reconciled `driver-adapter` value must point at the channel-agnostic parent dir (`…/driver/adapter`), with the per-channel children (`api`, `ui`, `external`, `shared`) sitting beneath it.

## Problem

The "scaffold-authoritative" doctrine in `path-keys.md` claims: *"the scaffolder owns both the YAML and the directory tree the YAML points at, so the values are authoritative initial values matching the just-created tree."*

This is false in practice. `gh optivem init` does not materialise the testkit tree from scratch — it clones the `optivem/shop` template (via `copySystemTests`). `DefaultPaths` then computes the `paths:` block *from scratch* (repo slug + per-language stems), without reading the tree it just cloned. The two have diverged, and the divergence is now **structural**, not a casing nit.

### Current template tree (verified on disk, 2026-06-04)

The template decomposes the driver layers **by channel/concern** (`api` / `ui` / `external` / `shared`) and carries **no SUT-namespace leaf** anywhere:

- **TS** — `system-test/typescript/src/testkit/driver/adapter/{api,external,shared,ui}`, `…/driver/port/{dtos,external}`, `…/dsl/{core,port}/…`; tests at `tests/latest/{acceptance,contract}`.
- **Java** — `system-test/java/src/main/java/com/mycompany/myshop/testkit/driver/adapter/{api,external,shared,ui}` (note `com/mycompany/myshop/` is a **package** segment, *not* a trailing leaf; `testkit/` lives **under** the package, not directly under `src/main/java/`); tests at `src/test/java/com/mycompany/myshop/systemtest/latest/{acceptance,contract}` (note the `systemtest` segment).
- **.NET** — `system-test/dotnet/Driver.Adapter/{Api,External,Shared,Ui}`, `Driver.Port/{Dtos,External}`, `Dsl.Core/…`, `Dsl.Port/…` (**no `Testkit.` prefix**, no `MyShop` leaf, **no top-level `External.*` project** — externals nest under `Driver.Adapter/External` and `Driver.Port/External`); tests at `SystemTests/Latest/{AcceptanceTests,ExternalSystemContractTests}`.

### What `DefaultPaths` emits today, per key

For `sutNamespace = "shop"` (derived from `optivem/shop`), measured against the tree above:

| Key | DefaultPaths emits (TS) | Template actual (TS) | Status |
|---|---|---|---|
| `driver-port` | `…/src/testkit/driver/port/shop` | `…/src/testkit/driver/port` | ✗ spurious `/shop` leaf |
| `driver-adapter` | `…/src/testkit/driver/adapter/shop` | `…/src/testkit/driver/adapter` | ✗ spurious `/shop` leaf |
| `external-system-driver-port` | `…/src/testkit/external/port/shop` | `…/src/testkit/driver/port/external` | ✗ wrong nesting **and** leaf |
| `external-system-driver-adapter` | `…/src/testkit/external/adapter/shop` | `…/src/testkit/driver/adapter/external` | ✗ wrong nesting **and** leaf |
| `at-test` | `…/tests/latest/acceptance` | `…/tests/latest/acceptance` | ✓ correct |
| `dsl-port` | `…/src/testkit/dsl/port/shop` | `…/src/testkit/dsl/port` | ✗ spurious `/shop` leaf |
| `dsl-core` | `…/src/testkit/dsl/core/shop` | `…/src/testkit/dsl/core` | ✓ shape, ✗ `/shop` leaf |
| `ct-test` | `…/tests/latest/contract` | `…/tests/latest/contract` | ✓ correct |

The `external-system-driver-*` rows expose a second, independent stem bug the original plan missed: `DefaultPaths` models externals as a sibling `external/{port,adapter}` dir, but every language nests them **under** the driver layer (`driver/port/external`, `driver/adapter/external`). In .NET there is no `External.*` project to point at at all.

Language-specific extra drift:

- **Java** — emits `src/main/java/testkit/driver/adapter/shop`; template is `src/main/java/com/mycompany/myshop/testkit/driver/adapter` (missing the `com/<org>/<sut>/` package prefix, spurious `/shop` leaf). `at-test`/`ct-test` emit `src/test/java/shop/latest/…`; template is `src/test/java/com/mycompany/myshop/systemtest/latest/…` (missing package prefix + the `systemtest` segment).
- **.NET** — emits `Testkit.Driver.Adapter/shop`; template is `Driver.Adapter` (spurious `Testkit.` prefix + `/shop` leaf). `at-test`/`ct-test` (`SystemTests/Latest/AcceptanceTests`, `…/ExternalSystemContractTests`) are correct.

### Net effect

For any user who runs `gh optivem init` against a fresh repo backed by the shop template,

- their generated `gh-optivem-*.yaml` names six (TS/.NET) or eight (Java) `paths:` keys that don't exist on disk,
- the runtime scope checker (`validate-outputs-and-scopes` → `resolveLayerPaths` → `pathInScope`) fails with `path(s) outside scope` on the first phase that edits a real file,
- the per-channel `--target driver-adapter --channel <ch>` flow (plan 1725) resolves `driver-adapter` to a nonexistent `…/adapter/shop`, so the channel subdir (`…/adapter/shop/api`?) is doubly wrong.

`at-test`/`ct-test` for TS and .NET are the only keys that happen to land correctly, because `DefaultPaths` already skips the SUT-leaf append for those two keys.

## How it was discovered (historical, pre-decomposition)

A 2026-05-26 rehearsal of `implement-system-driver-adapters` edited a real file under the driver-adapter UI subtree, then the scope checker rejected it because the yaml's `driver-adapter` value carried a leaf the tree didn't have. At that time the template still had a `myShop` SUT-namespace leaf (`…/driver/adapter/myShop/ui/client/pages/NewOrderPage.ts`); the per-channel decomposition has since removed it, so the same file now lives at `…/driver/adapter/ui/client/pages/NewOrderPage.ts`. The class of bug (DefaultPaths emits a path the tree doesn't have) is unchanged; only the specific shape moved.

## Decision needed

Reconcile `DefaultPaths`'s output with the shop template's actual layout. The original four options (A/B/C/D) were all keyed to a SUT-namespace *leaf* and are obsolete — there is no leaf to emit, configure, or rename. The live choice is now between two:

### Option C — derive from the template (recommended)

`gh optivem init`, after cloning the template, **reads** the template's checked-in `gh-optivem-*.yaml` `paths:` block (or, failing that, derives from the cloned tree) and uses it as the source of truth. `DefaultPaths` becomes a fallback for partial configs only (e.g. legacy migrations).

- **Pro:** the template controls layout truth completely — including per-channel nesting, the Java package prefix, the asymmetric external nesting, and any future restructuring — without a `gh-optivem` code change each time. Aligns the "scaffold-authoritative" doctrine with reality (the scaffolder copies a tree it does not author the *shape* of).
- **Con:** more I/O at init time; requires the template to ship a correct, parseable yaml. (The shop template's yamls were corrected template-side in the 2026-05-26 round, so this precondition now holds.)

### Option A′ — static per-language stems mirroring the template

Rewrite `pathStems` to emit the channel-agnostic parent paths directly, **dropping the `sutNamespace` trailing append for all testkit keys**, fixing the external nesting (`driver/{port,adapter}/external`), adding the Java `com/<org>/<sut>/` package prefix (+ the `systemtest/latest/…` test path), and removing the `.NET Testkit.` prefix.

- **Pro:** no init-time I/O; the derivation stays a pure function.
- **Con:** hard-codes the template's structural shape (channel decomposition, `com/mycompany/myshop` package, `Driver.Adapter` project names) into `gh-optivem`. Any future template restructuring needs a matching `pathStems` edit — exactly the drift this plan is cleaning up, re-armed. The Java package prefix still needs `org`/`sut` inputs, partially re-opening the question Option B was retired to close.

## Recommendation

**Option C.** The structural complexity the decomposition introduced (per-channel children, language-specific package/project prefixes, asymmetric external nesting) is precisely what a from-scratch derivation keeps getting wrong. Letting the template own its own layout truth — and reducing `DefaultPaths` to a partial-config fallback — is the only option that doesn't re-arm the drift on the next template change. This is still a real architectural call; confirm at pickup.

## Work this plan would do (sketch — pickup-time)

1. **Rewrite `paths_defaults.go` + `paths_defaults_test.go`** — under Option C, demote `DefaultPaths` to a fallback and add the template-yaml read at `init`; under Option A′, rewrite `pathStems` per the corrected per-key shapes above. Either way the test must stop asserting the `…/adapter/shop` shape (lines 12–19 / 30–37 / 48–55).
2. **Update `internal/projectconfig/path-keys.md`** — restate (C) or remove (A′) the "scaffolder owns both the YAML and the directory tree" claim, and replace the worked TS example (lines 119–126) — it still shows the obsolete `/shop` leaf.
3. **Smoke-test against the shop template** — `gh optivem init` against a fresh empty workspace; confirm the emitted yaml's `paths:` block matches the template's `gh-optivem-*.yaml` and that every value resolves to a real directory in the cloned tree.
4. **Cross-language + channel verification** — repeat for Java + .NET, and confirm `--target driver-adapter --channel api|ui` (plan 1725) resolves to real channel subdirs under the reconciled `driver-adapter` value.

## Open questions to resolve at pickup

- Does any repo besides `optivem/shop` get scaffolded by `gh optivem init`? If so, Option A′'s hard-coded structural assumptions become harder to justify — pushing further toward C.
- Is `gh optivem init` invoked by anyone today, or has the shop template's checked-in yamls made it dead code in practice? If dead, downgrade priority.
- Under Option C, what is the fallback contract when the cloned template ships no `paths:` block (partial/legacy)? `DefaultPaths`-derived-from-tree, or hard error?
