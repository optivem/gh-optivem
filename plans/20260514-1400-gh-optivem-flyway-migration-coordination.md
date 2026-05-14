# gh-optivem: scaffold Flyway migrations + repair paths after shop's `meta-v1.0.89`

> Cross-repo follow-up for the shop Flyway adoption (see
> `plans/20260513-1530-shop-canonical-db-schema-via-migrations.md`). Shop's
> `meta-v1.0.89` tag is the first release where `system/db/migrations/V*.sql`
> is the canonical source of schema, `ddl-auto: validate` is on, and a
> `db-migrate` Flyway sidecar is required at pipeline-time. gh-optivem's
> scaffolder was never updated, so the first acceptance run that resolved
> the shop ref to `meta-v1.0.89` failed across **all four smoke jobs**
> (run `25856832645`, 2026-05-14).
>
> The cross-repo coordination step listed in the shop plan ("grep gh-optivem
> `internal/` for DDL assumptions") undersold the surface area. This plan
> is the concrete fix.

## Problem

Scaffolded repos contain `db-migrate:` (copied through verbatim from shop's
compose) and apps configured with `ddl-auto: validate`, but **no migration
files**. Flyway logs at startup:

```
db-migrate-1 | WARNING: No migrations found. Are your locations set up correctly?
db-migrate-1 | Current version of schema "public": << Empty Schema >>
```

The app then boots, Hibernate schema-validates, and dies on the first missing
table:

```
SchemaManagementException: Schema-validation: missing table [coupons]
```

Two root causes, both in `internal/steps/apply_template.go`:

1. **Nothing copies `<shop>/system/db/migrations/`** into the scaffolded
   repo. `grep -rn "db/migrations\|system/db\|flyway" internal/` returns zero
   hits — there is no code path that moves these files.
2. **Even if they were copied, the paths inside the templates wouldn't
   resolve.** The scaffolder flattens shop's `docker/{testLang}/{arch}/` →
   `<repo>/docker/` and `system/monolith/{lang}/` → `<repo>/system/`. The
   relative paths inside the templates assume shop's deeper layout:
   - `docker-compose.pipeline.real.yml` mounts
     `../../../system/db/migrations:/migrations:ro` — from
     `<repo>/docker/` that overshoots two levels above the repo.
   - `application-test.yml` configures
     `spring.flyway.locations: filesystem:../../db/migrations` — from
     `<repo>/system/` (the Gradle project root post-flatten) that
     overshoots one level above the repo.

Only Java backends use Flyway in commit-stage tests (verified):
- Monolith Java + multitier Java backend → `application-test.yml`
  with `filesystem:../../db/migrations`.
- .NET monolith tests use EF Core `UseInMemoryDatabase` (no Flyway).
- TS backend setup not yet inspected; assume no Flyway today and
  no-op the replacement (FixupAllTextFiles only edits files that match).

`db-migrate` itself runs in every compose variant regardless of language
(it's a separate Flyway container, not language-specific), so the
**compose-side fix is universal**; the **test-config fix is Java-only** but
applying it everywhere is harmless.

## Target layout

After scaffolding, migrations live at the **root** of the repo that owns
the compose / Java code, in a `db/migrations/` directory at the same depth
as `system/` (monolith) or `backend/` (multitier). This mirrors shop's
sibling relationship between `system/db/` and `system/monolith/java/` and
gives the **same `../db/migrations` relative path** from every consuming
directory — `docker/`, `system/`, `backend/`. One rule covers all four
combos.

| Scaffold combo                       | Root repo                                       | Component repo                                   |
| ------------------------------------ | ----------------------------------------------- | ------------------------------------------------ |
| monolith / monorepo                  | `<repo>/db/migrations/` + compose + system code | —                                                |
| monolith / multirepo                 | `<repo>/db/migrations/` (compose only)          | `<sysDir>/db/migrations/` (system code)          |
| multitier / monorepo                 | `<repo>/db/migrations/` + compose + backend     | —                                                |
| multitier / multirepo                | `<repo>/db/migrations/` (compose only)          | `<bDir>/db/migrations/` (backend); frontend none |

Two replacements apply uniformly across all combos:
- Compose: `../../../system/db/migrations` → `../db/migrations`
- Java test yml: `filesystem:../../db/migrations` → `filesystem:../db/migrations`

## Changes

### 1. `internal/steps/apply_template.go` — new helper

```go
// copyDbMigrations copies shop/system/db/migrations/ to <dst>/db/migrations/.
// Called for every scaffolded repo that contains either a docker-compose
// referencing the db-migrate sidecar or Java backend code that runs
// Flyway+Testcontainers in its commit stage.
func copyDbMigrations(shop, dst string) {
    src := filepath.Join(shop, "system", "db", "migrations")
    if _, err := os.Stat(src); err == nil {
        files.CopyDir(src, filepath.Join(dst, "db", "migrations"))
    }
}
```

The `os.Stat` guard keeps the scaffolder compatible with pre-`meta-v1.0.89`
shop refs — anyone passing `--shop-ref meta-v1.0.88` (or an older SHA) still
scaffolds successfully; the directory just isn't there to copy.

### 2. Call sites

In `apply_template.go`:

| Function                  | Add call                                                       |
| ------------------------- | -------------------------------------------------------------- |
| `applyMonolithMonorepo`   | `copyDbMigrations(shop, repoDir)` after `copyExternals`        |
| `applyMonolithMultirepo`  | `copyDbMigrations(shop, repoDir)` for root; `copyDbMigrations(shop, sysDir)` after the system-code copy |
| `applyMultitierMonorepo`  | `copyDbMigrations(shop, repoDir)` after `copyExternals`        |
| `applyMultitierMultirepo` | `copyDbMigrations(shop, repoDir)` for root; `copyDbMigrations(shop, bDir)` after the backend-code copy |

For multirepo, copy to the component repo unconditionally (not gated on
language). It's a single small directory; the slight pollution in .NET /
TS repos costs nothing and keeps the call-site logic uniform.

### 3. Compose path replacement

In `monolithDockerComposeReplacements` (apply_template.go:637) and
`multitierDockerComposeReplacements` (apply_template.go:742), append:

```go
// Flyway migrations volume mount: shop's compose lives at docker/<lang>/<arch>/
// so ../../../system/db/migrations reaches shop's system/db/migrations.
// Scaffolder flattens to docker/ and lifts migrations to <repo>/db/migrations,
// so the mount becomes ../db/migrations.
{shopSystemPrefix + "db/migrations", "../db/migrations"},
```

`shopSystemPrefix` is already defined as `"../../../system/"` at line 36,
so this expands to `"../../../system/db/migrations" → "../db/migrations"`.

### 4. Java test-config path replacement

Add a new replacement set targeting `application-test.yml`. Options
considered:

- **Option A** (chosen): extend `FixupAllTextFiles` calls with a small
  Flyway-path replacement list, applied in all four `applyXxx` functions
  after the system-code copy. `FixupAllTextFiles` already walks `.yml` /
  `.yaml` files, so this is a one-line addition per call site.
- Option B: add a dedicated `FixupFlywayPaths` helper. Cleaner but
  introduces a new template function for a one-line transformation —
  premature factoring.

Replacement list (defined once, shared across helpers):

```go
// flywayPathReplacements rewrites Spring's filesystem:../../db/migrations
// (shop layout: system/monolith/java/ → up 2 → system/ → db/migrations)
// to filesystem:../db/migrations (scaffold layout: system/ → up 1 → root →
// db/migrations). Same shape works for multitier (backend/ → up 1 → root).
func flywayPathReplacements() [][2]string {
    return [][2]string{
        {"filesystem:../../db/migrations", "filesystem:../db/migrations"},
    }
}
```

Call sites:

- `applyMonolithMonorepo`, `applyMultitierMonorepo`: add
  `templates.FixupAllTextFiles(repoDir, flywayPathReplacements())` after
  the system/backend code copy.
- `applyMonolithMultirepo`: apply to `sysDir` (after copying system code
  to system repo). Root repo doesn't contain `application-test.yml`, so
  no need there.
- `applyMultitierMultirepo`: apply to `bDir` (backend repo). Root has no
  Java code; frontend repo has no Flyway.

### 5. Tests

`internal/config/config_system_test.go` is the system test that drove the
discovery (`TestValidMonolithConfigurations` etc.). It already exercises
the four combos end-to-end via real `docker compose up`, so a green run
after the fix is the integration signal.

Unit-level coverage to add in `internal/steps/replacements_test.go`:

- `Test_monolithDockerComposeReplacements_includesFlywayPath` — assert
  the new compose replacement is present and applies to a sample input.
- `Test_flywayPathReplacements` — assert
  `filesystem:../../db/migrations` is rewritten and
  `filesystem:../db/migrations` is left untouched (idempotency).

No changes needed to `internal/steps/apply_template_test.go` (if it
exists; haven't checked) beyond extending fixtures if they assert on
copied-file lists.

## Sequencing

1. Land scaffolder fix on a branch (single PR; all four combos in one
   change).
2. Local verification: `bash scripts/manual-test.sh --shop-ref meta-v1.0.89`
   for each of the six stack/arch combos. Confirm:
   - `<repo>/db/migrations/V*.sql` is present.
   - `<repo>/docker/docker-compose.pipeline.real.yml` mounts
     `../db/migrations:/migrations:ro`.
   - For Java scaffolds (mono + multitier), the relevant
     `application-test.yml` shows `filesystem:../db/migrations`.
3. Run `gh-acceptance-stage` workflow_dispatch with the default
   (smoke=full, acceptance=condensed). All four smoke jobs should pass.
4. Cut a gh-optivem release that bumps the baked `SHOP_SHA` /
   `SHOP_TAG` ldflags to `meta-v1.0.89` (or the latest meta-v* at that
   point). Existing released binaries remain on `meta-v1.0.88` and keep
   working — no surprise breakage in the field.

## Risks / open questions

- **Pre-`meta-v1.0.89` shop refs**: covered by the `os.Stat` guard in
  `copyDbMigrations`. Verified: `meta-v1.0.88` had no `system/db/`
  directory at all, so the scaffolder will skip the copy and the
  generated compose won't reference `db-migrate` (it came from
  `meta-v1.0.88` which didn't have it either).
- **TS backend Flyway config** (multitier with TS backend, if such a
  combo exists): not inspected. If TS backend ever adopts Flyway with a
  similar relative path, the replacement set already covers it (no-op
  today, future-compatible).
- **Layout drift signal**: if shop ever moves `system/db/migrations/`
  elsewhere, the replacement breaks silently (compose mount points at a
  missing dir, Flyway reports "no migrations"). The shop plan calls out
  a `compose-drift` CI check on the shop side. Consider an analogous
  scaffold-drift integration test that, post-scaffold, greps the
  generated compose for `../../../` paths and fails — would catch this
  class of bug for any future shop layout change. Out of scope for this
  PR.

## References

- Failing run: <https://github.com/optivem/gh-optivem/actions/runs/25856832645>
- Last passing run (still on `meta-v1.0.88`):
  <https://github.com/optivem/gh-optivem/actions/runs/25849462851>
- Shop Flyway plan:
  `plans/20260513-1530-shop-canonical-db-schema-via-migrations.md`
- Origin incident (shop side): Issue #61 acceptance retry rehearsal,
  `coupons.used_count` NULL constraint, 2026-05-13.
