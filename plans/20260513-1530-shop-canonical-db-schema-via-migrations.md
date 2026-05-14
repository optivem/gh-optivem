# Shop: canonical DB schema via a single migration set (Flyway)

> ✅ **Decisions locked 2026-05-14.** Tactical alignment of all 6 ORM DDLs landed 2026-05-13 — the six implementations now emit identical schema *by convention*. This plan replaces convention with enforcement: schema ownership moves out of the apps into a single ordered Flyway migration set applied by a sidecar service, per Dave Farley's CD model for DB migrations.
>
> **Tool choice:** Flyway Community (SQL-first, forward-only, polyglot-friendly). See "Decisions" section below for all locked answers.

## Decisions (locked 2026-05-14)

| # | Question | Decision |
|---|---|---|
| 1 | Migration file naming | Timestamped: `V{YYYYMMDDHHMMSS}__{description}.sql` (avoids merge conflicts between parallel branches) |
| 2 | Flyway image tag | Pinned semver: `flyway/flyway:10.21.0-alpine` (or current minor at PR time); Renovate/Dependabot bumps it |
| 3 | First-rollout strategy | Drop volumes (`docker compose down -v`) before first `up` after merge. No baseline — shop is greenfield from Flyway's perspective. Document the one-time `down -v` step in shop README. |
| 4 | Test isolation under `validate` | Testcontainers + Flyway across all 3 stacks. `application-test.yml` (Java) switches from `ddl-auto: create-drop` to `validate`. Cost ~2-3s/test class, real schema. |
| 5 | Failure semantics | Forward-only. No `U__*.sql` files. `FLYWAY_CLEAN_DISABLED=true` on the sidecar. **App rollbacks ≠ schema rollbacks** — schema stays forward; previous app version runs against current schema by expand-contract discipline. Recovery from a bad migration is a new forward migration. Documented in `schema-changes.md`. |
| 6 | Compose duplication | Duplicate the `db-migrate` block in all 24 compose files. No `extends:` / `include:` / anchors — scaffolder copies files verbatim per language/architecture, so each scaffolded repo gets one clean, complete compose file. Drift prevented by a shop-side CI check (`yq '.services.db-migrate' docker-compose.*.yml \| sort -u` yields one entry). |
| 7 | Cross-language drift CI | Add `monolith-drift` and `multitier-drift` acceptance jobs (additive — existing per-stack jobs untouched). Sequence: Java up → suite → stop apps → TS up on same volume → suite. Stub mode only. .NET third-leg deferred to follow-up. |
| 8 | Previous-version smoke test | **Deferred entirely.** Shop has no production deploys yet; rolling-deploy safety isn't a real concern. Phase 4 keeps a one-line TODO in `schema-changes.md` ("When shop adopts release tagging for deploys, add a CI job that runs the previous release's image against the current migration set.") |
| 9 | Cross-repo sequencing | Shop-first → manual-test gh-optivem `--shop-ref main` → patch gh-optivem `internal/` if scaffolder hardcodes DDL assumptions → tag new shop `meta-v*` → bump gh-optivem's pinned shop ref via release. Existing released gh-optivem binaries stay frozen on the old shop SHA — no surprise breakage in the field. |
| 10 | Canary stack | TS monolith `local.stub` first. It's where Issue #61 surfaced, spins up fastest (~10-15s), has the largest before/after diff (`ensureSchema()` deletion), and its `IF NOT EXISTS` tolerance is the only path that can demonstrate the original failure mode. |

## Background

The shop repo contains 6 implementations of the same SUT (3 languages × 2 architectures):

- `system/monolith/{typescript,java,dotnet}`
- `system/multitier/backend-{typescript,java,dotnet}`

All three monolith stacks share one Postgres volume (Compose project `my-shop-stub`); all three multitier stacks share their own. Each stack manages schema differently:

| Stack | Mechanism | Behavior |
|---|---|---|
| TS monolith | raw `pg` + `ensureSchema()` | `CREATE TABLE IF NOT EXISTS` — tolerant follower |
| TS multitier | TypeORM | `synchronize: true` — idempotent ALTERs |
| Java (both) | Hibernate | `ddl-auto: create-drop` — drops + recreates on startup |
| .NET (both) | EF Core | `EnsureDeletedAsync()` + `EnsureCreatedAsync()` — same |

The live schema is whichever stack created it first. The tolerant TS monolith then no-ops via `IF NOT EXISTS` and silently inherits a possibly-incompatible shape. Manifested as: `null value in column "used_count"` (Issue #61 acceptance retry, 2026-05-13) — the table existed from a prior Java run that mapped `used_count` as `NOT NULL` without a SQL `DEFAULT`.

**Tactical fix already landed** (commit on shop `main`, 2026-05-13): all 6 ORM definitions hand-aligned to emit identical DDL for `coupons` and `orders` via `@ColumnDefault` (Hibernate), `HasDefaultValueSql` (EF Core), and `default:` (TypeORM). The TS monolith `insertCoupon` was also patched to set `used_count` explicitly so it doesn't rely on a DEFAULT existing in the live table.

That's correct *as long as nobody adds a column without remembering to update all 6 ORM DDLs in lockstep.* This plan removes that fragility.

## Scope (shop repo: `../shop`)

### Phase 1 — Add canonical migration set + sidecar

**New files:**

- `shop/system/db/migrations/V{YYYYMMDDHHMMSS}__init.sql` — the agreed DDL, extracted verbatim from the post-alignment `shop/system/monolith/typescript/src/lib/db.ts:14-42` block. Timestamp from the moment of file creation (e.g. `V20260514103045__init.sql`). One SQL file per migration thereafter, same naming scheme.
- `shop/system/db/migrations/README.md` — pointer to this plan and to `shop/docs/operations/schema-changes.md` (Phase 4 runbook).

**New Compose service** (duplicated in each of the 24 compose files, per decision #6):

```yaml
db-migrate:
  image: flyway/flyway:10.21.0-alpine
  command: -url=jdbc:postgresql://postgres:5432/app -user=app -password=app -locations=filesystem:/migrations migrate
  environment:
    FLYWAY_CLEAN_DISABLED: "true"
  volumes:
    - ../../../system/db/migrations:/migrations:ro
  depends_on:
    postgres:
      condition: service_healthy
```

`FLYWAY_CLEAN_DISABLED=true` blocks `flyway clean` so a misconfigured CI step can never wipe a schema. Pin the image tag to a specific minor (rebased deliberately via Renovate/Dependabot).

App services in each compose file gain:

```yaml
depends_on:
  db-migrate:
    condition: service_completed_successfully
```

**Files to edit:**

- `shop/docker/typescript/monolith/docker-compose.{local,pipeline}.{real,stub}.yml` (4 files)
- `shop/docker/typescript/multitier/docker-compose.{local,pipeline}.{real,stub}.yml` (4 files)
- `shop/docker/java/monolith/docker-compose.{local,pipeline}.{real,stub}.yml` (4 files)
- `shop/docker/java/multitier/docker-compose.{local,pipeline}.{real,stub}.yml` (4 files)
- `shop/docker/dotnet/monolith/docker-compose.{local,pipeline}.{real,stub}.yml` (4 files)
- `shop/docker/dotnet/multitier/docker-compose.{local,pipeline}.{real,stub}.yml` (4 files)

Total: 24 compose files with an identical `db-migrate` block (decision #6). Drift between the 24 copies is prevented by a new shop-side CI check that asserts the YAML-serialized service definitions are identical across all 24 files.

**Why not extract a shared fragment?** `extends:` / `include:` / YAML anchors all complicate scaffolding — the gh-optivem scaffolder copies compose files verbatim per language/architecture, and indirection in the template forces either inline transformation at copy time or fragile relative paths in the new repo's layout. Duplication keeps the scaffolder a `cp`.

**Tool choice:** Flyway Community Docker image. SQL-only migrations, checksummed, no JVM in the app image. SQL-first is required because the shop is polyglot (3 ORMs sharing schemas) — ORM-owned migrations (EF Core Migrations, Hibernate `ddl-auto`) can't be the source of truth when three ORMs share a database.

### Phase 2 — Disable app-level DDL

Apps become schema-validators, not schema-creators. Each must boot clean against a Postgres already migrated by the sidecar.

**Files to edit:**

- `shop/system/monolith/typescript/src/lib/db.ts` — delete `ensureSchema()` and the `schemaPromise` plumbing (lines 11-44 in the post-alignment file). Remove every `await ensureSchema();` call (one per exported function, ~7 sites).
- `shop/system/multitier/backend-typescript/src/app.module.ts:42` — `synchronize: true` → `synchronize: false`.
- `shop/system/monolith/java/src/main/resources/application.yml:17` — `ddl-auto: create-drop` → `ddl-auto: validate`.
- `shop/system/monolith/java/src/test/resources/application-test.yml:9` — same.
- `shop/system/multitier/backend-java/src/main/resources/application.yml:17` — same.
- `shop/system/multitier/backend-java/src/test/resources/application-test.yml:9` — same.
- `shop/system/monolith/dotnet/Program.cs:57-62` — delete the `using (var scope = ...)` block that calls `EnsureDeletedAsync()` + `EnsureCreatedAsync()`.
- `shop/system/multitier/backend-dotnet/Program.cs` — same (line ~76).

**Test fixture impact (decision #4):** Java's `application-test.yml` uses `create-drop` for test isolation. We switch to `validate` and use **Testcontainers + Flyway** across all three stacks — a fresh `PostgreSQLContainer` per integration test class, with the Flyway migration set applied at container start. Spring Boot 3.x wires this via `@DynamicPropertySource` (Java); `Testcontainers.PostgreSql` covers .NET; the `testcontainers` npm package covers TS. Cost: ~2-3s per test class. Benefit: tests run against the same schema generator as prod, eliminating the "tests pass on `create-drop` shape, prod breaks on migration shape" failure mode that motivated this whole plan.

Fast unit-shaped tests that only need transactional rollback (Spring `@Transactional`) can keep that pattern. The Testcontainers requirement is for integration/acceptance suites — i.e., the ones that exercise actual schema concerns.

### Phase 3 — Pipeline integration

- Acceptance-stage workflows already drive `docker compose up`; the `depends_on: service_completed_successfully` chain means no workflow change is needed if compose orchestrates the migrate→app sequence correctly.
- Verify `acceptance-test/action.yml` (per-stack) does not bypass compose by `docker run`-ing app images directly.
- If any pipeline does direct image runs, prepend a `flyway/flyway:10.21.0-alpine migrate` step there too — same command, same migrations volume.
- **Add two new acceptance-stage jobs** (decision #7): `monolith-drift` and `multitier-drift`. Each runs Java up → suite → stop apps (keep volume) → TS up on same volume → suite. Proves the migration set produces a schema all stacks can read. Stub mode only. .NET third-leg deferred.
- **Add a CI drift check** for compose duplication (decision #6): `yq '.services.db-migrate' docker-compose.*.yml | sort -u` must yield exactly one entry.

### Phase 4 — Forward-only doctrine + expand-contract discipline

**New file:** `shop/docs/operations/schema-changes.md` — runbook with three sections in this order:

**1. Forward-only doctrine (decision #5).**
- No `U__*.sql` files. The Flyway `undo` feature is treated as if it doesn't exist.
- Recovery from a bad migration is a new forward migration, never a reversal.
- `FLYWAY_CLEAN_DISABLED=true` is set on the sidecar. Don't unset it.
- **Why:** down migrations don't reverse data changes — `DROP COLUMN` deletes the data the previous app version wrote. Forward-only forces every migration to be safe-by-construction.

**2. App rollback ≠ schema rollback.**
- Apps roll back via the platform (kubectl rollout undo, compose with previous image, blue/green flip). Seconds, no data risk.
- The schema does not roll back. The previous app version runs against the current schema by design.
- This only works if every migration preserved the previous app's invariants — see section 3.
- Rollback-safety table:

  | Change type | Wrong (couples to one app version) | Right (rollback-safe) |
  |---|---|---|
  | Add column | `ADD COLUMN x NOT NULL` (old code can't write x) | `ADD COLUMN x DEFAULT 0` (old code keeps working) |
  | Rename column | `RENAME COLUMN a TO b` (old code can't find a) | Add b → dual-write → migrate readers → drop a (multi-step) |
  | Drop column | `DROP COLUMN a` (old code still reads a) | Stop writing → wait → stop reading → drop (multi-step) |
  | Tighten constraint | `ALTER ... SET NOT NULL` (old code writes NULL) | Add tolerant default → backfill → tighten |

**3. Expand-contract pattern.** Worked examples:
- Rename column: add new → dual-write → migrate readers → migrate writers → drop old (5 migrations minimum)
- Drop column: stop writing → wait → stop reading → drop (3 migrations)
- Tighten constraint: add tolerant version → backfill data → tighten (3 migrations)

**PR template addition:** a checkbox `If this PR changes schema, does it preserve the previous app version's invariants?` in `shop/.github/pull_request_template.md` (file may not exist yet — create if so).

**TODO (deferred — decision #8):** when shop adopts release tagging for deploys, add a CI job that runs the previous release's image against the current migration set, to prove the rolling-deploy window is safe. Not built today because shop has no production deploys — every CI environment is fresh. Re-evaluate when the deploy model exists.

### Phase 5 — Migration test fixture (when migration set grows non-trivial)

**New file:** `shop/system/db/fixtures/representative.sql` — anonymized prod-shape data covering edge cases (empty strings, nulls, max-length, unicode, large NUMERIC, timezone-spanning timestamps).

**New CI job:** applies the full migration set to a clean Postgres + `representative.sql`, then runs the cross-language integration suite. A migration that breaks shape on representative data fails the build.

Defer Phase 5 until the migration set has at least 3 entries — not worth the maintenance burden at the initial migration.

## Scope (gh-optivem repo)

The gh-optivem scaffolder pins to a specific shop SHA at release time (`.goreleaser.yml` bakes `ShopRef` via ldflags). Local dev builds fall back to the latest `meta-v*` tag; `--shop-ref main` is the explicit opt-in for testing unreleased shop changes. This means **existing released gh-optivem binaries stay frozen on the old shop SHA** and won't break in the field when shop's main moves.

Cross-repo sequencing (decision #9):

1. Shop PR (Phases 1+2 inseparable) lands on shop's main.
2. Shop's own CI passes — all 6 per-stack acceptance jobs + the 2 new drift jobs (decision #7) + the compose drift check (decision #6).
3. In gh-optivem, run `bash scripts/manual-test.sh --shop-ref main` for each of the 6 stack/arch combos. Confirm: `docker-compose.*.yml` contains `db-migrate`; `db.ts` has no `ensureSchema()`; `application.yml` has `ddl-auto: validate`; `Program.cs` has no `EnsureCreatedAsync()`.
4. Grep gh-optivem `internal/` for DDL assumptions and patch as needed:
   ```
   grep -rn "ddl-auto\|EnsureCreated\|EnsureDeleted\|synchronize\|CREATE TABLE IF NOT EXISTS" internal/
   ```
   Likely targets: anything in `internal/steps/` that asserts the old DDL behavior in template files or test fixtures.
5. Tag a new `meta-v*` release on shop pointing at the Flyway commit.
6. Cut a new gh-optivem release that bumps `SHOP_SHA` / `SHOP_TAG` ldflags to the new shop SHA. After this, fresh `gh extension install optivem/gh-optivem` users scaffold Flyway-enabled repos.

**Limitation:** cross-repo verification (step 3) is manual today — no automated test in gh-optivem CI proves it tracks shop's evolution. That's a pre-existing gap; addressing it is out of scope for this plan.

## Coordination

- **Phases 1 and 2 must land in the same PR.** Phase 1 alone is harmless (adds a no-op sidecar). Phase 2 alone breaks every startup. They are inseparable.
- One PR, 6 stacks, 24 compose files, ~10 app-entry files, 1 new SQL file. Estimated 2-3 days including local verification across all 6 stack/arch combos.
- Phase 3 piggybacks on Phase 2's PR; Phases 4 and 5 are independent follow-ups.

Suggested implementation order (canary first, then fan out):

1. Branch shop. Add `V{YYYYMMDDHHMMSS}__init.sql` + `db-migrate` service to **`docker/typescript/monolith/docker-compose.local.stub.yml`** (canary — decision #10). TS monolith chosen because: it's where Issue #61 surfaced, spins up fastest, and its `IF NOT EXISTS` tolerance is the only path that can demonstrate the original failure mode.
2. Delete `ensureSchema()` and the `schemaPromise` plumbing from `shop/system/monolith/typescript/src/lib/db.ts`. Remove every `await ensureSchema();` call. Verify locally end-to-end with `docker compose down -v && docker compose up`.
3. Add `db-migrate` to the remaining three TS-monolith compose files (`local.real`, `pipeline.stub`, `pipeline.real`). Verify each.
4. Repeat the pattern on `java/monolith` (4 compose files, `ddl-auto: validate`, Testcontainers in test setup) and `dotnet/monolith` (4 compose files, delete `EnsureDeletedAsync`/`EnsureCreatedAsync` block, Testcontainers in test setup).
5. Repeat on the 3 multitier stacks (12 more compose files).
6. Add the shop-side CI drift check for the 24 duplicated `db-migrate` blocks (decision #6).
7. Add the `monolith-drift` and `multitier-drift` acceptance jobs (decision #7).
8. Add `shop/docs/operations/schema-changes.md` (Phase 4 doctrine + runbook).
9. PR; verify all 6 SonarCloud projects' acceptance stages green + both new drift jobs green.
10. After merge: tag shop `meta-v*`, then bump gh-optivem's pinned shop SHA per "Scope (gh-optivem repo)" sequence above.

One commit per stack/mode pairing for review-ability.

## Verification

- Manual local: `docker compose -f shop/docker/<lang>/<arch>/docker-compose.local.stub.yml up` boots cleanly for all 6 combos after `down -v`.
- Cross-language drift test (now automated as `monolith-drift` / `multitier-drift` jobs — see Phase 3): bring up Java, run acceptance, stop *apps only* (keep volume), bring up TS on same volume, run acceptance. Both pass — no `null value in column` errors.
- `grep -rn "ddl-auto:\s*create-drop\|EnsureDeletedAsync\|EnsureCreatedAsync\|synchronize:\s*true\|CREATE TABLE IF NOT EXISTS" shop/system shop/docker` returns zero hits.
- Compose drift check: `yq '.services.db-migrate' shop/docker/**/docker-compose.*.yml | sort -u` returns exactly one entry.
- Acceptance stage green on all 6 SonarCloud projects + both new drift jobs green.
- `gh-optivem`: `bash scripts/manual-test.sh --shop-ref main` scaffolds a working Flyway-enabled repo for at least one matrix entry per architecture (preferably all 6 before tagging the gh-optivem release).

## References

- Humble & Farley, *Continuous Delivery*, ch. 12 ("Managing Data")
- Sadalage & Fowler, *Refactoring Databases* — expand-contract pattern catalogue
- Origin incident: Issue #61 acceptance retry (rehearsal-20260513), `coupons.used_count` NULL constraint
- Tactical alignment: shop commit on `main`, 2026-05-13 (13 files across 6 stacks)
