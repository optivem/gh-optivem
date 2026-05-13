# Shop: canonical DB schema via a single migration set (Flyway-style)

> ⚠️ **Deferred.** Tactical alignment of all 6 ORM DDLs landed 2026-05-13 — the six implementations now emit identical schema *by convention*. This plan replaces convention with enforcement: schema ownership moves out of the apps into a single ordered migration set applied by a sidecar service, per Dave Farley's CD model for DB migrations.

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

- `shop/system/db/migrations/V001__init.sql` — the agreed DDL, extracted verbatim from the post-alignment `shop/system/monolith/typescript/src/lib/db.ts:14-42` block. One SQL file per migration thereafter (`V002__*.sql`, `V003__*.sql`).
- `shop/system/db/migrations/README.md` — pointer to this plan and the expand-contract runbook (Phase 4).

**New Compose service** (per the 12 compose files):

```yaml
db-migrate:
  image: flyway/flyway:10-alpine
  command: -url=jdbc:postgresql://postgres:5432/app -user=app -password=app -locations=filesystem:/migrations migrate
  volumes:
    - ../../../system/db/migrations:/migrations:ro
  depends_on:
    postgres:
      condition: service_healthy
```

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

Total: 24 compose files. The `db-migrate` service definition is identical across all of them — consider extracting to a `compose.db.yml` fragment and `extends:` it (TBD during implementation).

**Tool choice:** Flyway Community Docker image. SQL-only migrations, checksummed, baseline support, no JVM in the app image. Alternative `migrate/migrate` (Go) is lighter but lacks baseline and `repair` semantics.

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

**Test fixture impact:** Java's `application-test.yml` uses `create-drop` for test isolation. Switching to `validate` means tests rely on the migration set being applied. Either (a) the test setup invokes Flyway directly via Testcontainers, or (b) `application-test.yml` keeps `create-drop` and tests are explicitly out of scope for the migrate-sidecar contract. **Decision deferred to implementation.**

### Phase 3 — Pipeline integration

- Acceptance-stage workflows already drive `docker compose up`; the `depends_on: service_completed_successfully` chain means no workflow change is needed if compose orchestrates the migrate→app sequence correctly.
- Verify `acceptance-test/action.yml` (per-stack) does not bypass compose by `docker run`-ing app images directly.
- If any pipeline does direct image runs, prepend a `flyway/flyway:10-alpine migrate` step there too — same command, same migrations volume.

### Phase 4 — Expand-contract discipline (ongoing, lightweight)

**New file:** `shop/docs/operations/schema-changes.md` — runbook documenting the expand-contract pattern with worked examples:

- Rename column: add new → dual-write → migrate readers → migrate writers → drop old (5 migrations minimum)
- Drop column: stop writing → wait → stop reading → drop (3 migrations)
- Tighten constraint: add tolerant version → migrate data → tighten (3 migrations)

**PR template addition:** a checkbox `If this PR changes schema, does it preserve the previous app version's invariants?` in `shop/.github/pull_request_template.md` (file may not exist yet — create if so).

**CI smoke test:** add a job to the acceptance-stage workflow that boots the previous app version's image against the *current* migration set and runs that previous version's integration suite. Proves the rolling-deploy window is safe. Implementation: pull `ghcr.io/{owner}/{repo}:{previous-tag}` and run its existing suite — TBD whether previous-tag is "previous successful main" or "previous release."

### Phase 5 — Migration test fixture (when migration set grows non-trivial)

**New file:** `shop/system/db/fixtures/representative.sql` — anonymized prod-shape data covering edge cases (empty strings, nulls, max-length, unicode, large NUMERIC, timezone-spanning timestamps).

**New CI job:** applies `V001…Vn` to a clean Postgres + `representative.sql`, then runs the cross-language integration suite. Migration that breaks shape on representative data fails the build.

Defer Phase 5 until V003 or later — not worth the maintenance burden at V001.

## Scope (gh-optivem repo)

No changes expected. The shop scaffolder consumes shop as a template; once shop is updated, fresh scaffolds get the new layout automatically. Verify after Phase 1+2 land:

- `bash scripts/manual-test.sh --owner ... --arch monolith --backend-lang typescript ...` produces a scaffold whose `docker-compose.*.yml` files contain the `db-migrate` service and whose `db.ts` no longer has `ensureSchema()`.
- Same for `java` and `dotnet`, `monolith` and `multitier`.

If `gh-optivem/internal/steps/` hardcodes anything about app-level DDL (e.g., asserting `ddl-auto: create-drop` in templates or test fixtures), update accordingly. Quick scan needed during implementation: `grep -rn "ddl-auto\|EnsureCreated\|synchronize" internal/`.

## Coordination

- **Phases 1 and 2 must land in the same PR.** Phase 1 alone is harmless (adds a no-op sidecar). Phase 2 alone breaks every startup. They are inseparable.
- One PR, 6 stacks, 24 compose files, ~10 app-entry files, 1 new SQL file. Estimated 2-3 days including local verification across all 6 stack/arch combos.
- Phase 3 piggybacks on Phase 2's PR; Phases 4 and 5 are independent follow-ups.

Suggested implementation order:

1. Branch shop. Add `V001__init.sql` + `db-migrate` to one compose file (e.g., `dotnet/monolith/local.stub`). Verify locally end-to-end.
2. Disable EF Core auto-creation in that one stack. Verify.
3. Repeat steps 1-2 across all 6 stacks. One commit per stack for review-ability.
4. Last commit: extract the duplicated `db-migrate` service definition into a `compose.db.yml` fragment if the duplication is painful.
5. Run cross-language acceptance suite: Java monolith → Postgres tears down → TS monolith on same volume → both pass.
6. PR; verify all 6 SonarCloud projects' acceptance stages green.

## Verification

- Manual: `docker compose -f shop/docker/<lang>/<arch>/docker-compose.local.stub.yml up` boots cleanly for all 6 combos.
- Cross-language drift test: bring up Java monolith, run acceptance, tear down *apps only* (keep volume), bring up TS monolith, run acceptance. Both pass — no `null value in column` errors.
- `grep -rn "ddl-auto:\s*create-drop\|EnsureDeletedAsync\|EnsureCreatedAsync\|synchronize:\s*true\|CREATE TABLE IF NOT EXISTS" shop/system shop/docker` returns zero hits.
- Acceptance stage green on all 6 SonarCloud projects.
- `gh-optivem`: `bash scripts/manual-test.sh` scaffolds a working repo with the new layout for at least one matrix entry per architecture.

## Why deferred

The tactical alignment (2026-05-13) makes the verify suite pass and prevents the next drift bug *as long as developers remember to update all 6 ORM DDLs together when adding a column*. That's good enough to unblock today's release.

The structural fix is a 2-3 day cross-cutting change touching 24 compose files, 6 app entry points, the CI flow, and a new SQL migration file. Best done as a focused PR with a clean baseline, after the current release window — not as scope-creep on an issue-#61 verify fix.

## References

- Humble & Farley, *Continuous Delivery*, ch. 12 ("Managing Data")
- Sadalage & Fowler, *Refactoring Databases* — expand-contract pattern catalogue
- Origin incident: Issue #61 acceptance retry (rehearsal-20260513), `coupons.used_count` NULL constraint
- Tactical alignment: shop commit on `main`, 2026-05-13 (13 files across 6 stacks)
