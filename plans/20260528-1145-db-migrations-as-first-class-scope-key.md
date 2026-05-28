# DB migrations as a first-class scope key

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-28T09:56:54Z`

## Context

During the 2026-05-28 rehearsal of issue #71 ("Gift-wrap an order"), the
`system-implementer` dispatch tripped a `VALIDATE_OUTPUTS_AND_SCOPES`
failure and routed into the `FIX` call-activity asking the operator to
approve remediation. Root cause:

1. The AC required persisting a `gift_wrap` boolean so the
   `isGiftWrapped()` assertion could round-trip through the DB.
2. The agent correctly identified that a new migration file under
   `system/db/migrations/` was required, and wrote
   `V20260528113000__add_gift_wrap.sql`.
3. `implement-system`'s scope (`process-flow.yaml:1461`) declares
   `write: [system-path]` only, where `system-path` resolves to
   `system/monolith/typescript`. The migration file is outside that set.
4. The agent's prompt told it to emit a `scope_exception` envelope via
   `gh optivem output write` before writing out of scope. It tried —
   twice — and got back:
   `"gh optivem output write" must run inside a gh-optivem agent dispatch (GH_OPTIVEM_OUTPUT_FILE is not set)`.
   The envelope channel was unreachable because `implement-system`
   declares no `outputs:` block (so the runtime doesn't export
   `GH_OPTIVEM_OUTPUT_FILE` — see
   `internal/atdd/runtime/clauderun/clauderun.go:1643-1649`).
5. The agent fell through to writing the migration anyway and
   flagging it in prose. The scope check then caught it.

There are two distinct things wrong here. The envelope-channel gap is
its own concern (the `implement-system` MID should declare
`scope-exception-*` outputs, matching the precedent at
`process-flow.yaml:1328-1337`); a separate ticket can address that.

**This plan owns the deeper question:** schema migrations are
**legitimate first-class output** of `implement-system` for any ticket
whose AC asserts persisted state. Routing them through
`scope_exception` permanently is the wrong default — exceptions exist
for genuine edge cases, not for a path the agent should be allowed to
write every time persistence changes.

## The infrastructure already exists

`system/db/migrations/` is not a hypothetical layer. The shop template
already ships it:

```
shop/system/db/migrations/
  README.md
  V20260514085249__init.sql
```

It is the **shared canonical migration set** consumed by all six SUT
implementations (3 languages × 2 architectures) via a Flyway sidecar.
The design is documented in the deferred plan
`plans/deferred/20260513-1530-shop-canonical-db-schema-via-migrations.md`
(Phase 1 of which has shipped — the directory and `init.sql` exist
today; the sidecar wiring is the deferred half).

Critically, `system/db/migrations/` is **architecture- and
language-agnostic**: one Flyway-style ordered set shared across every
SUT, sitting as a sibling of `system/monolith/` and `system/multitier/`,
not as a child of either. That asymmetry matters for the path-key
shape (see Item 1 below).

## Why the current scope is wrong for this case

The `implement-system` agent is dispatched to make a failing AT pass.
When the AT asserts persisted state (gift-wrapped flag, audit-log
entry, soft-delete tombstone, etc.), the only way to make it pass is
to:

1. Edit production code under `system-path` (the column read/write,
   the API field, the UI), AND
2. Add a migration under `system/db/migrations/` so the column exists
   when the test runs.

These two writes are a single logical change — not a violation of
separation of concerns, not an out-of-scope reach. The scope contract
should reflect that.

## Items

### Item 1 — Decide where the key lives in the config schema

The migration path is **user-owned in value** (an operator can put
migrations anywhere — even outside the gh-optivem-aware tree if they
have legacy SQL elsewhere) but **gh-optivem-owned in name**. That
fits the Family A/B distinction, but neither family is a clean home:

| Option                                                                 | Pros                                                                                       | Cons                                                                                                |
|------------------------------------------------------------------------|--------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| **A.** Family A — add `system.db-migration-path: system/db/migrations` and surface key `system-db-migration-path` | Matches `system-path` / `system-test-path` precedent (top-level system field → scope-eligible key). One source of truth in the config. | Family A is fixed-schema (one new top-level field). Requires `Validate` rule + migrate back-fill plumbing. |
| **B.** Family B — add `db-migration-path` to `CanonicalPathKeys()` under `system-test.paths:`                     | Lowest-touch — drop one entry into `CanonicalPathKeys()`, `DefaultPaths`, `pathStems`.    | Semantically wrong: the block is `system-test.paths:` but the value is under `system/`. Future-readers will wince. |
| **C.** New `system.paths:` block mirroring `system-test.paths:`                                                   | Cleanest grouping. Symmetric with the existing `system-test.paths:` shape. Future-proofs further system-tier layer keys (e.g. `system-config-path`). | Largest surface-area change: new block validation, new `DefaultPaths`-equivalent, new migrate back-fill, new path-keys doc section. |

**Decision: A.** The scope key is `system-db-migration-path`, matching
the existing `system-path` / `system-test-path` Family A naming (noun
+ `-path` suffix for a single physical location, not pluralised even
though the directory contains many files — the *path* names the
container, not the contents, mirroring how `system-path` names the
SUT root not its files). The cost is one new field in
`projectconfig.Config.System` plus one entry in the path-key
vocabulary doc. Option C is the architecturally clean answer but is
not justified by a single new key — defer C until a second
system-tier layer key needs a home.

### Item 2 — Add the key to the vocabulary

**Files:**

- `internal/projectconfig/config.go` — add `DbMigrationPath string \`yaml:"db-migration-path,omitempty"\`` to the `System` struct (or wherever `system.path` lives today).
- `internal/projectconfig/path-keys.md` — extend the Family A table at lines 30-35 with `system-db-migration-path` → `system.db-migration-path`. Add a row to the "path-shaped Family A keys eligible for per-phase scope" note (line 37-41) — `system-db-migration-path` joins `system-path` as scope-eligible.
- `internal/atdd/phase_scopes.go` — extend `FamilyAPathKeysInScope` to include `system-db-migration-path`.

**Default value (scaffold-authoritative):** `system/db/migrations`. This
matches the shop template's existing tree and is correct for every
language/architecture combo (the path is shared infrastructure).

### Item 3 — Wire the default into `init`

**File:** `internal/steps/optivem_yaml.go` (the `BuildOptivemYAML`
function called from `gh optivem init`).

Per the scaffold-authoritative doctrine
(`internal/projectconfig/path-keys.md:64-89` — "only `init` can
legitimately write a `paths:` block from a derived layout"),
`db-migration-path: system/db/migrations` is emitted once at `init`
time, then operator-owned at every other layer (`Validate`, `migrate`,
runtime).

Mirror the existing `system.path` handling: `init` writes the default;
`Validate` rejects a missing value (Rule 22a equivalent for the new
field); `gh optivem config migrate` does not back-fill it (operators
of pre-this-plan configs add it themselves).

### Item 4 — Widen `implement-system` scope

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

```yaml
implement-system:
  ...
  - id: EXECUTE_AGENT
    ...
    read:  [at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, external-system-driver-port, external-system-driver-adapter, system-path, system-db-migration-path]
    write: [system-path, system-db-migration-path]
```

The read addition matters too: the agent should see existing
migrations before adding a new one (so it doesn't redeclare a column,
collide on a timestamp, etc.).

Update the block-comment at `process-flow.yaml:1442-1446` to call out
the `system-db-migration-path` shared infrastructure asymmetry — every
arch/language SUT writes into the same migration set.

### Item 5 — Mirror the widening on `update-system`

**File:** same.

`update-system` (`process-flow.yaml:1477+`) is the reshape variant of
`implement-system` and faces the same friction: a reshape that
requires a schema change (e.g. extracting a column into a join table)
needs the same legitimate write path. Add `system-db-migration-path` to
both its `read:` and `write:` sets.

### Item 6 — Decide whether `refactor-system` also gets the write

**Open question to resolve during implementation:** does
`refactor-system` (`process-flow.yaml:1605+`) legitimately need
`system-db-migration-path` on the write side? Pure refactors should not
change schema, by definition. But the boundary blurs: a column rename
is a refactor *and* a schema change.

**Tentative answer: read-only on this phase.** A refactor that needs
a schema change is the trigger for a new ticket (or a `behavior-change`
verb dispatch), not a write authority widening. If this turns out to
be too strict in practice, revisit.

Add `system-db-migration-path` to `refactor-system`'s `read:` set only,
not `write:`.

### Item 7 — Update the system-implementer agent prompt

**File:** `internal/assets/runtime/agents/atdd/system-implementer.md`

Currently the prompt's Step 2 says "Do the simplest implementation
possible under the system surface (`system/monolith/typescript`) with
the goal of making the acceptance test pass." Extend with a one-line
note:

> When the AT asserts persisted state, also add a schema migration
> under `system-db-migration-path` — a single timestamped SQL file in the
> Flyway naming convention (`V{YYYYMMDDHHMMSS}__{description}.sql`).
> Read the existing migrations first to see the current schema; do not
> redeclare columns that already exist.

Mirror the addition in `update-system`'s agent prompt
(`system-updater.md` if it exists, or wherever the update-system body
lives).

### Item 8 — Backfill the shop template's `gh-optivem.yaml`

**File:** `shop/gh-optivem.yaml` (or wherever the canonical template
config lives).

Add `db-migration-path: system/db/migrations` to the `system:` block.
This is the one config file an operator does **not** scaffold-generate,
so it needs manual editing.

### Item 9 — Test coverage

- `internal/projectconfig/paths_defaults_test.go` (or equivalent) —
  cover the new `DefaultPaths`-equivalent for the new field.
- `internal/projectconfig/config_test.go` — `Validate` rejects missing
  value; accepts the doctrine default.
- `internal/atdd/phase_scopes_test.go` — `system-db-migration-path`
  resolves correctly and is scope-eligible for the widened phases.
- `internal/atdd/runtime/statemachine/transitions_test.go` (or scope
  test) — `implement-system` and `update-system` accept writes to
  the new path; `refactor-system` rejects them.

### Item 10 — Re-run the issue #71 rehearsal as the acceptance check

After Items 1-9 land, re-dispatch `gh optivem rehearsal` on issue #71.
The `system-implementer` dispatch should now write **both**
`system/monolith/typescript/...` files and the migration without
tripping `VALIDATE_OUTPUTS_AND_SCOPES`. No `FIX` activity should be
offered. If the gate still fires, the scope widening was incomplete.

## Out of scope

- **The `implement-system` outputs-block gap.** That `gh optivem output write` failed because `implement-system` declares no `outputs:` block is a separate defect — covered by a sibling plan (TBD). This plan does not require fixing it, but the fix would compose: even with `system-db-migration-path` in scope, *other* out-of-scope edits (e.g. a driver-port change) should still have a working envelope path.
- **The deferred Flyway sidecar wiring** (`plans/deferred/20260513-1530-shop-canonical-db-schema-via-migrations.md` Phase 1.2+ — the 24 docker-compose edits + Phase 2 app-level DDL disable). This plan adds the **scope-key** infrastructure; whether or not the sidecar is wired up, the migration files are already a real artefact today.
- **External-system schema migrations.** If an external stand-in (e.g. a stubbed payment provider) needs its own migrations, that is a separate scope-key decision under `external-system-driver-adapter` — not this plan.
- **A separate `db-migrator` agent.** Routing migrations through a dedicated agent is one possible future shape (cleaner separation, narrower scope per dispatch). But it would split the single logical "make the AT pass" change into two dispatches, costing latency and breaking the BPMN sequence. Defer until evidence that the implementer-writes-migration default produces bad migrations.

## Decisions

1. **Key name: `system-db-migration-path`** (resolved 2026-05-28).
   Matches the existing `system-path` / `system-test-path` Family A
   naming. Family B Option B was rejected on semantic grounds — the
   block is `system-test.paths:` but the value lives under `system/`.
2. **Migrate-time back-fill: yes, exactly once** (resolved 2026-05-28).
   `gh optivem config migrate` back-fills
   `system.db-migration-path: system/db/migrations` for existing
   consumer repos that lack the field, then never again. This is
   consistent with the one-shot SSoT-join precedent for
   `sut-namespace`. Operators who deliberately set a non-default
   value are unaffected (back-fill applies only when the field is
   absent).

## Deferred to a separate plan

- **Should `update-system` get the same AT-layer read widening that
  `implement-system` got in commit `454eb64`?** Decided 2026-05-28:
  **dropped** — separate plan was opened
  (`20260528-1155-update-system-at-layer-read-widening.md`) and then
  deleted during refinement. Not pursued; may revisit if a concrete
  reshape failure surfaces.

## References

- `internal/projectconfig/paths_defaults.go:94-105` — `CanonicalPathKeys()`, the Family B key set.
- `internal/projectconfig/path-keys.md` — the vocabulary doc that needs the new Family A row.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1447-1468` — `implement-system` MID with current `read: [...] / write: [system-path]`.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1325-1337` — `implement-and-verify-acceptance-tests` MID — precedent for `scope-exception-*` outputs (which `implement-system` lacks; out-of-scope for this plan).
- `internal/atdd/runtime/clauderun/clauderun.go:1629-1654` — `subprocessEnv()`, the gate that exports `GH_OPTIVEM_OUTPUT_FILE` only when the MID declares outputs.
- `plans/deferred/20260513-1530-shop-canonical-db-schema-via-migrations.md` — the upstream design for the canonical migration set (Phase 1 already shipped).
- `shop/system/db/migrations/README.md` — runtime contract for the migration set (Flyway naming, forward-only, expand-contract).
- `plans/20260527-1507-widen-implement-system-read-scope.md` (deleted in commit `454eb64`, viewable via `git show`) — precedent for widening `implement-system`'s read scope; this plan extends the same pattern to add a *write* layer.
- `plans/upcoming/20260526-1430-reconcile-defaultpaths-with-shop-template-layout.md` — the in-flight rework of `DefaultPaths`; coordinate with that plan's owner if both touch `BuildOptivemYAML`.
