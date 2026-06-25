# 2026-06-25 07:29:39 UTC — Rewrite the TypeScript migrations-path on scaffold flatten (fix ENOENT db/migrations)

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

Edit `internal/scaffolding/steps/apply_template.go`: add a `tsMigrationsPathReplacements(arch string)` helper (or two arch-specific helpers) next to `flywayPathReplacements()` (~line 904) that returns the per-arch source replacement — multitier: `'../../../../../db/migrations'` → `'../../../../db/migrations'`; monolith: `'../../../../db/migrations'` → `'../../../db/migrations'`. Then call `templates.FixupSourceFiles(<dir>, tsMigrationsPathReplacements(...))` in the 4 apply paths next to the existing `flywayPathReplacements()` calls: `applyMonolithMonorepo` (~264, repoDir), `applyMonolithMultirepo` (~376, sysDir — the system-repo copy), `applyMultitierMonorepo` (~448, repoDir), `applyMultitierMultirepo` (~583, bDir). Use `FixupSourceFiles` (not `FixupAllTextFiles`) — it covers `.ts` (`templates.go:243-244`). Per-arch scoping is collision-free because monolith/multitier are separate apply paths each touching only their own spec file (4-up is a suffix substring of 5-up, so a global replace would corrupt multitier). Then add the regression test (Step 2) and verify (Step 3).

## Steps

- [ ] **Step 1 — Add the rewrite to the scaffolder (`internal/scaffolding/steps/apply_template.go`).** Add `tsMigrationsPathReplacements` helper(s) near `flywayPathReplacements` (~904) with comments matching the Flyway-rewrite style (explain: migrations copied to repo root by `copyDbMigrations`; shop deep tree → flat scaffold reduces `../` depth). Apply via `templates.FixupSourceFiles` in all 4 apply paths, adjacent to each existing `flywayPathReplacements()` call:
  - `applyMonolithMonorepo` → `repoDir` (4→3 up)
  - `applyMonolithMultirepo` → `sysDir` (the system-repo code copy that carries the spec; 4→3 up)
  - `applyMultitierMonorepo` → `repoDir` (5→4 up)
  - `applyMultitierMultirepo` → `bDir` (the backend-repo copy; 5→4 up)
- [ ] **Step 2 — Regression test (`internal/scaffolding/steps/replacements_test.go`).** Mirror the existing Flyway-rewrite test (~920-926): assert the multitier rewrite (`'../../../../../db/migrations'` → `'../../../../db/migrations'`) and the monolith rewrite (`'../../../../db/migrations'` → `'../../../db/migrations'`), and assert the monolith pattern does **not** corrupt the multitier 5-up string when each is applied in its own arch path (guards the substring-collision reasoning).
- [ ] **Step 3 — Verify.** `go test -p 2 ./internal/scaffolding/...` green. (Never run unbounded `go test ./...` on Windows; use `-p 2`.)

## Verification

- `go test -p 2 ./internal/scaffolding/...` green (build + new regression test).
- Operator re-runs the **multitier-monorepo-typescript** Smoke (the original failing matrix cell) and ideally the **monolith-typescript** Smoke; confirms the ENOENT at `order.repository.integration.spec.ts:33` / `db.integration.spec.ts:27` is gone end-to-end.

## Notes / context (pinned root cause)

- Failure: gh-optivem Smoke run `28127099014`, job *Smoke (ubuntu-latest, multitier, monorepo, typescript)* → dispatched backend-commit-stage run `28127308512` failed at `npm run test:integration` with `ENOENT … scandir '/home/runner/work/test-app-.../db/migrations'`.
- Cause: `shop/system/multitier/backend-typescript/src/core/repositories/order.repository.integration.spec.ts:18` uses `path.resolve(__dirname, '../../../../../db/migrations')` (5-up). Correct for shop's deep tree (`…/backend-typescript/src/core/repositories` → 5-up = `system/`, giving `system/db/migrations`). The scaffolder flattens backend → `backend/` and copies migrations to the **repo root** (`copyDbMigrations`, `apply_template.go:88`), so 5-up overshoots past the root.
- Existing precedent: `flywayPathReplacements()` rewrites Java's `filesystem:../../db/migrations` → `filesystem:../db/migrations` (`apply_template.go:264/376/448/583`, helper ~904); Pact uses `FixupSourceFiles` + `contractsPathReplacements()` (~453). The Flyway comment (~262-263) explicitly notes it's a no-op on TS — the gap this plan closes.
- Parallel implementations: **Java** already handled (multitier-multirepo-java passed same run); **.NET** not affected (no relative-path migration loading in `backend-dotnet/Tests/Integration`); **monolith TS** has the same latent bug (`shop/system/monolith/typescript/src/__tests__/db.integration.spec.ts:12`, 4-up) — folded in as preventive.
- Scope: single repo (gh-optivem), scaffolder layer only. **No shop changes.**
