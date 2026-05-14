# Shop: canonical DB schema via Flyway — remaining work

> ✅ **Phases 1–4 landed in shop on 2026-05-14.** The canonical migration set, the
> `db-migrate` Flyway sidecar across all 24 compose files, the app-level DDL
> deletions (TS `ensureSchema`, TS `synchronize`, Java `ddl-auto: validate`,
> .NET `EnsureCreatedAsync` removal), the Testcontainers + Flyway wiring for
> the Java test profiles, the `compose-drift` CI check, the `drift` (monolith
> + multitier Java→TS schema-interop) jobs **integrated into
> `_meta-prerelease-pipeline.yml` as a sibling of `cross-lang` (gates
> `tag-meta-rc` via the `!failure()` aggregate)**, and
> `docs/operations/schema-changes.md` are all in shop's `main`.
>
> Verified end-to-end by meta-prerelease run `25854297005` on 2026-05-14:
> all 7 commit stages + 6 pipelines + 12 cross-lang + 2 drift jobs green,
> meta-rc tag `meta-v1.0.89-rc.321` minted.
>
> What remains is the cross-repo coordination after shop's PR ships and Phase 5,
> deferred per decision in the original plan.

## Cross-repo coordination (post-merge of shop)

Triggered after the shop changes are merged and shop's CI is green (per-stack
acceptance + `compose-drift` + `drift` — the latter now a pipeline-level gate
on `tag-meta-rc`).

1. **Manual-test against shop main.** In gh-optivem, run
   `bash scripts/manual-test.sh --shop-ref main` for each of the six stack/arch
   combos. Confirm:
   - The scaffolded `docker-compose.*.yml` contains the `db-migrate` sidecar.
   - The scaffolded `db.ts` has no `ensureSchema()` plumbing.
   - The scaffolded `application.yml` has `ddl-auto: validate`.
   - The scaffolded `Program.cs` has no `EnsureDeletedAsync()`/`EnsureCreatedAsync()`.
2. **Grep gh-optivem `internal/` for DDL assumptions.** Patch as needed:
   ```bash
   grep -rn "ddl-auto\|EnsureCreated\|EnsureDeleted\|synchronize\|CREATE TABLE IF NOT EXISTS" internal/
   ```
   Likely targets: anything in `internal/steps/` that asserts the old DDL
   behavior in template files or test fixtures.
3. **Tag a new `meta-v*` release on shop** pointing at the Flyway commit.
4. **Cut a new gh-optivem release** that bumps `SHOP_SHA` / `SHOP_TAG` ldflags
   to the new shop SHA. After this, fresh `gh extension install optivem/gh-optivem`
   users scaffold Flyway-enabled repos.

Existing released gh-optivem binaries stay frozen on the old shop SHA — no
surprise breakage in the field.

## Deferred follow-ups

- ⏳ **Phase 5 — migration test fixture.** New file
  `shop/system/db/fixtures/representative.sql` (anonymised prod-shape data
  covering nulls, max-length, unicode, large NUMERIC, timezone-spanning
  timestamps) + a CI job that applies the migration set to a clean Postgres
  plus this fixture, then runs the cross-language integration suite. Defer
  until the migration set has at least three entries — not worth the
  maintenance burden at the initial migration.
- ⏳ **Previous-version smoke test** (decision #8 in the original plan).
  When shop adopts release tagging for deploys, add a CI job that runs the
  previous release's image against the current migration set. Shop has no
  production deploys today, so this is theoretical until the deploy model
  exists.

## References

- The decision log and full design history live in the shop changes
  themselves: `system/db/migrations/README.md` and
  `docs/operations/schema-changes.md` in the shop repo.
- Origin incident: Issue #61 acceptance retry (rehearsal-20260513),
  `coupons.used_count` NULL constraint.
