# Plan: `system-driver-adapter-shared` scope key (writable shared test-transport foundation)

## Why

A per-channel `implement-system-driver-adapters` dispatch (channel `ui`) legitimately
needed to add a Playwright primitive (`PageClient.setChecked`) to the shared transport
client at `…/driver/adapter/shared/client/playwright/PageClient.java`. The edit is
correct and load-bearing — `NewOrderPage.inputGiftWrap` calls it, and a gift-wrap
checkbox has no existing primitive. But it tripped the scope checker and was routed to
the `scope-diff-fixer` halt.

Root cause: `narrowAdapterScopeByChannel` (`internal/atdd/runtime/actions/bindings.go:571`)
narrows the whole-layer `system-driver-adapter` write (`driver/adapter`) down to the
channel member (`driver/adapter/ui`). `driver/adapter/shared/**` belongs to no channel,
so the narrowed dispatch can't write it — even though extending the shared transport is
normal, additive, in-spec work.

Design discussion conclusion: the shared transport wrappers are a **foundation layer**
consumed by many independently-scoped adapters (UI page objects, the API driver, *and*
external adapters — an external system with no API can only be driven via Playwright; HTTP
wrappers are shared across api + every external adapter). Placement follows architectural
layer, not the current consumer set. Adapter implementers should be allowed to *extend*
that foundation directly, without a halt — via an explicit write scope, not by punching a
hole in the channel-narrowing logic and not by routing legitimate work through the
exception path.

## ⚠️ Doctrine change being made

`internal/projectconfig/path-keys.md` currently states (lines ~81-83, ~117-123):

> The whole-layer `system-driver-adapter` key … names that root … (where `shared` lives).
> `shared` adapter code is the residual under the root, not a member — **no phase scopes
> to "shared only", so it earns no schema slot.**

This plan overturns that: `shared` becomes a first-class, **explicitly stored** Family B
key (`system-driver-adapter-shared`) with its own phase scope. Flagging per the
rule-conflict convention: deliberate doctrine change, decided in discussion 2026-06-05.

## Decisions (locked in discussion)

- **Explicit, stored Family B key — NOT derived.** It joins `system-test.paths:` like
  every other layer key, so a developer can rename or relocate the shared folder. This
  matches the project's strongest doctrine — *"users own VALUES; explicit only; runtime
  never falls back to derived values"* (`path-keys.md`) — and the `system-driver-adapter-channels`
  precedent (stored, not derived, precisely because a team may restructure). A derived
  token was investigated and **rejected** (see below): it would be the one path a developer
  couldn't customize, and the "no runtime fallback" doctrine forbids the stored-optional-
  with-derived-fallback middle ground.
- **Token name** `system-driver-adapter-shared` (position-named, parallel to
  `external-system-driver-adapter` which is likewise a path-subset of `driver/adapter`).
- **Token path** the whole `driver/adapter/shared` subtree, NOT `…/shared/client`:
  scoping to `…/shared/client` only would re-create the same halt one level down — the base
  `Client` interface, shared DTOs, serialization, config live directly under `shared/`,
  outside `shared/client/`. Treat the foundation as one coherent owned area.
- **Un-narrowed.** `narrowAdapterScopeByChannel` narrows only the exact `system-driver-adapter`
  entry, so this key is never narrowed. Every channel + the external dispatch can read AND
  extend the foundation. `driver/adapter/ui` and `driver/adapter/shared` are disjoint
  subtrees, so per-channel ownership and resume footprints (keyed on the channel member)
  are unaffected.
- **Write, not just read.** Adapter implementers get the key in BOTH `read:` and `write:`.
- **No file moves required.** `PageClient` already lives under `driver/adapter/shared/…`;
  this is scope wiring + a new path key, not a tree reshape. (Confirm the HTTP/RestClient
  location during execution — expected to already be under `shared/` too.)

## Existing-config handling (Rule 22a consequence) — DECIDED: B1

`Validate` Rule 22a (`config.go:776`) requires *every* `CanonicalPathKeys()` entry present
once `system.architecture` is set. Adding a 9th canonical key means existing configs must
gain the line. The "no runtime fallback" doctrine (`path-keys.md:150-156`) rules out a
stored-optional-with-derived-default escape hatch.

**Chosen: B1 — no migrate change.** Existing configs surface the standard Rule 22a
"missing key" error (which names the gap and points at `path-keys.md`); the operator adds
the one line. This is exactly how the system already treats any missing canonical key — no
new doctrine. Friction = one line per project (in practice, the shop config).

Not chosen — **B2** (extend `gh optivem config migrate` to back-fill the key from the
derived default, the `db-migration-path` lifecycle): zero operator friction, but it would
**overturn** `config_commands.go:371` ("migrate does NOT back-fill Family B keys"). Kept on
record in case the one-line manual add is later judged too costly; not part of this plan.

## Rejected alternatives

- **Derived token (no storage), synthesized in `PlaceholderMap`:** non-breaking, but not
  operator-editable (can't rename/move shared) and leans on a runtime derivation the
  project's doctrine explicitly forbids. Rejected in favor of explicit.
- **Widen by smearing `shared/` into the channel member** (the scope-diff-fixer's idea):
  pollutes `narrowAdapterScopeByChannel`, muddies per-channel ownership / resume footprints.
- **Halt → scope-fixer for every shared edit:** mischaracterizes legitimate additive work
  as agent failure; adds latency/cost.
- **Colocate transports into `ui/` / `api/`:** fails — consumer sets aren't stable
  (browser-driven external systems need Playwright; HTTP is multi-consumer).
- **Split `shared` into ui/api, or tag external clients ui/api:** misapplies the inbound
  channel axis to a foundation / to outbound external deps.

## Edits

### 1. `internal/projectconfig/paths_defaults.go`
- `CanonicalPathKeys()` — append `"system-driver-adapter-shared"` (9th key).
- `pathStems()` — append the matching stem **in the same position** in all three language
  branches (DefaultPaths is index-aligned with CanonicalPathKeys):
  - TypeScript: `src/testkit/driver/adapter/shared`
  - Java: `path.Join(main, "driver/adapter/shared")`
  - .NET: `Driver.Adapter/Shared`
- Update the doc comments that say "eight keys" / enumerate the set → nine + new key.

### 2. `internal/projectconfig/path-keys.md`
- Add `system-driver-adapter-shared` to the Family B key table (§"Family B").
- Rewrite the "`shared` … earns no schema slot" passage (~line 123) and the
  `system-driver-adapter` root description (~line 81): `shared` is now its own stored key,
  writable by adapter implementers, un-narrowed.
- Add the new key to the TypeScript default-values example block.

### 3. `internal/atdd/runtime/statemachine/process-flow.yaml`
Add `system-driver-adapter-shared` to read+write on the four adapter call-activities:
- `implement-system-driver-adapters` (~1572): `read:` += token, `write:` += token.
- `update-system-driver-adapters` (~1605): same.
- `implement-external-system-driver-adapters` (~1629): `read:` += token, `write:` += token.
- `update-external-system-driver-adapters` (~1657): same.
- **Decide during execution:** `implement-external-system-stubs` (~1680) — if stubs call
  the shared HTTP wrapper, add the token here too (likely yes).
- **Review (likely no change):** the combined reshape/other nodes already listing
  `system-driver-adapter` (~1485, ~1524, ~1558, ~1719, ~1744, ~1768) run un-narrowed
  whole-layer scope, which already covers `driver/adapter/shared`. Confirm none are
  channel-narrowed.

### 4. `narrowAdapterScopeByChannel` (`internal/atdd/runtime/actions/bindings.go:571`)
- **No code change.** It narrows only the exact `system-driver-adapter` entry, so the new
  key is never narrowed. Confirm with a test (below).

### 5. Shop template (cross-repo)
- Add `system-driver-adapter-shared` to the checked-in `gh-optivem-<arch>-<lang>.yaml`
  `paths:` blocks so the `pathStems`/`DefaultPaths` parity tests stay green, and ensure the
  `driver/adapter/shared` folder exists in the scaffolded tree (it does today — `PageClient`
  lives there). Coordinate so the parity test and shop CI land together.

## Tests to update / add
- `internal/projectconfig/paths_defaults_test.go` — key count 8→9; new key present per
  language with correct stem.
- `internal/projectconfig/config_test.go` — Rule 22a over the new key; canonical-key set.
- `internal/steps/optivem_yaml_test.go` — emitted YAML includes the new key.
- `internal/atdd/runtime/actions/bindings_test.go` — `system-driver-adapter-shared`
  resolves via the layer resolver and is NOT narrowed by a `channel` param; a write under
  `driver/adapter/shared` passes scope for a channel-`ui` dispatch (the gift-wrap repro).
- `internal/atdd/runtime/statemachine/transitions_test.go` / `channels_test.go` — scope
  expectations for the four adapter activities if they assert read/write sets.
- `internal/atdd/phase_scopes_test.go` — `TestPhaseScopes_LayersAreCanonical` passes for
  free (canonical key); the four adapter MIDs now carry the token.
- If B2: `config_commands_test.go` — migrate back-fills the key + idempotency.

## Immediate unblock (independent of this plan)
For the in-flight gift-wrap run: approve `PageClient.setChecked` as a one-off scope
exception (the edit is correct; do NOT revert). This plan removes the need for future
exceptions of this shape.

## Verification
- `go build ./...`
- `go test ./internal/projectconfig/... ./internal/atdd/... ./internal/steps/... ./...`
- `gh optivem process scope implement-system-driver-adapters` (and the external variant)
  shows `driver/adapter/shared` in the resolved write set.
