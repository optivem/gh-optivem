# 2026-06-25 07:29:39 UTC — Rewrite the TypeScript migrations-path on scaffold flatten (fix ENOENT db/migrations)

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-25T07:33:23Z`

## TL;DR

**Why:** Scaffolded TypeScript backends fail `npm run test:integration` with `ENOENT … scandir '<repo>/db/migrations'`. The integration spec's `MIGRATIONS_DIR` is a hardcoded relative path tuned to shop's deep tree; the scaffolder flattens the layout and copies migrations to the repo root, but only rewrites the **Java** Flyway path on flatten — the TS path is left untouched (`apply_template.go:262-263` says "No-op on .NET / TS") and overshoots past the repo root.
**End result:** The scaffolder rewrites the TS spec's `MIGRATIONS_DIR` relative path per-architecture (multitier 5→4 `../`, monolith 4→3 `../`) so it resolves to the repo-root `db/migrations` the scaffolder actually creates. Scaffolded monolith & multitier TS integration tests find their migrations; shop source is unchanged (shop CI keeps its deep-tree path).

## Outcomes

What we get out of this — the goals and deliverables:

- A scaffolded multitier-TypeScript repo's `order.repository.integration.spec.ts` resolves `MIGRATIONS_DIR` to `<repo>/db/migrations`, so `npm run test:integration` passes the migration scan (the original failure in run 28127099014 / backend run 28127308512 is gone).
- A scaffolded monolith-TypeScript repo's `db.integration.spec.ts` likewise resolves to `<repo>/db/migrations` (preventive — same latent defect, not in the failing run's matrix).
- The fix lives in the scaffolder layer (`internal/scaffolding/steps`), mirroring the existing Java Flyway and Pact-contracts rewrites — **shop source is not modified** (shop's own CI needs the deep-tree relative path).
- A regression test pins both rewrites (multitier 5→4, monolith 4→3), mirroring the existing Flyway-rewrite test.
- `go test -p 2 ./internal/scaffolding/...` is green.

## ▶ Next executable step (resume here)

All code steps are done (helper + 4 call sites + regression test, `go test -p 2 ./internal/scaffolding/...` green). The only remaining work is operator verification (see `## Verification`): re-run the **multitier-monorepo-typescript** Smoke (the original failing matrix cell) and ideally the **monolith-typescript** Smoke, confirming the ENOENT at `order.repository.integration.spec.ts:33` / `db.integration.spec.ts:27` is gone end-to-end.

## Verification

- `go test -p 2 ./internal/scaffolding/...` green (build + new regression test).
- Operator re-runs the **multitier-monorepo-typescript** Smoke (the original failing matrix cell) and ideally the **monolith-typescript** Smoke; confirms the ENOENT at `order.repository.integration.spec.ts:33` / `db.integration.spec.ts:27` is gone end-to-end.

## Notes / context (pinned root cause)

- Failure: gh-optivem Smoke run `28127099014`, job *Smoke (ubuntu-latest, multitier, monorepo, typescript)* → dispatched backend-commit-stage run `28127308512` failed at `npm run test:integration` with `ENOENT … scandir '/home/runner/work/test-app-.../db/migrations'`.
- Cause: `shop/system/multitier/backend-typescript/src/core/repositories/order.repository.integration.spec.ts:18` uses `path.resolve(__dirname, '../../../../../db/migrations')` (5-up). Correct for shop's deep tree (`…/backend-typescript/src/core/repositories` → 5-up = `system/`, giving `system/db/migrations`). The scaffolder flattens backend → `backend/` and copies migrations to the **repo root** (`copyDbMigrations`, `apply_template.go:88`), so 5-up overshoots past the root.
- Existing precedent: `flywayPathReplacements()` rewrites Java's `filesystem:../../db/migrations` → `filesystem:../db/migrations` (`apply_template.go:264/376/448/583`, helper ~904); Pact uses `FixupSourceFiles` + `contractsPathReplacements()` (~453). The Flyway comment (~262-263) explicitly notes it's a no-op on TS — the gap this plan closes.
- Parallel implementations: **Java** already handled (multitier-multirepo-java passed same run); **.NET** not affected (no relative-path migration loading in `backend-dotnet/Tests/Integration`); **monolith TS** has the same latent bug (`shop/system/monolith/typescript/src/__tests__/db.integration.spec.ts:12`, 4-up) — folded in as preventive.
- Scope: single repo (gh-optivem), scaffolder layer only. **No shop changes.**
