# 2026-09-17 17:58:44 UTC — Fix scaffolded TypeScript migrations-path rewrite (multitier)

## TL;DR

**Why:** gh-acceptance-stage run 35251477548 failed on the `multitier / monorepo / typescript` smoke. The scaffolded repo's backend Narrow Integration tests crashed with `ENOENT ... scandir '<parent-of-repo>/db/migrations'` (`test/support/migrations.ts:24`). Shop commit d5903324 (2026-09-17) moved the TS migration helper to `system/multitier/backend-typescript/test/support/migrations.ts` with a 4-up `MIGRATIONS_DIR`. gh-optivem's multitier rewrite (`internal/scaffolding/steps/apply_template.go:991`) still only matches the old 5-up literal, so it silently did nothing and the path now points above the repo root.
**End result:** Scaffolded TS backends (multitier monorepo and multirepo) and the monolith point `MIGRATIONS_DIR` at `<repo>/db/migrations` again. If a future shop move stops a rewrite from matching, the scaffold fails loudly and names the file, instead of shipping a broken repo.

## Outcomes

- The multitier TS backend's narrow-integration tests find `db/migrations` in the scaffolded repo, for both monorepo and multirepo.
- The monolith TS integration spec keeps working (same 4→3 rewrite).
- One arch-independent rewrite rule replaces the per-arch switch. The switch only existed to protect a 5-up string that shop no longer has.
- The unit tests check the rule against the literals shop actually uses today, not made-up strings.
- A drift guard runs after scaffolding. It fails with the file name and resolved path when a TS `path.resolve(__dirname, '.../db/migrations')` literal does not land on `<repo>/db/migrations`.

## ▶ Next executable step (resume here)

All five steps are implemented and `go build ./...` + `go test -p 2 ./internal/scaffolding/...` + `go test -p 2 .` pass. Nothing left for an agent. The only open work is the operator re-run under `## Verification`.

## Steps

- [x] Step 1: `internal/scaffolding/steps/apply_template.go`:
  - Replace `tsMigrationsPathReplacements(arch string)` with an arch-independent helper returning `{"'../../../../db/migrations'", "'../../../db/migrations'"}`. The rule is quote-anchored, so it is idempotent.
  - Rewrite the doc comment. Both shop literals are now 4 up:
    - multitier: `backend-typescript/test/support`
    - monolith: `monolith/typescript/src/__tests__`
  - Both scaffold targets need 3 up (`backend/test/support`, `system/src/__tests__`). Drop the "near-substring / per-arch" rationale.
  - Update all four call sites (monolith ≈288/406, multitier monorepo ≈485, multirepo `bDir` ≈638) and their comments.
- [x] Step 2: `internal/scaffolding/steps/replacements_test.go` — replace `TestTsMigrationsPathReplacementsRewritePerArch` (≈line 1086) with a test that checks three things:
  - the 4→3 rewrite of the exact current shop line text for both files (`const MIGRATIONS_DIR = path.resolve(__dirname, '../../../../db/migrations');`)
  - that the rewrite is idempotent
  - that unrelated `db/migrations` strings, such as the compose `../db/migrations` mount, are untouched
- [x] Step 3: add a fail-loud drift guard in `internal/scaffolding/steps/`:
  - After the TS migrations fixup on each path (monolith, multitier monorepo `repoDir`, multirepo `bDir`), walk the `.ts` files, skipping `node_modules`.
  - Match `path.resolve(__dirname, '<rel>/db/migrations')` literals and resolve `<rel>` against the file's directory.
  - If the result is not the scaffold's `db/migrations` dir (where `copyDbMigrations` puts it), return or propagate an error naming the file and resolved path.
  - Wire it into the step's existing error-return path. Don't just log it.
- [x] Step 4: unit-test the guard with a temp dir. Cover three cases:
  - a correct file, which passes
  - a file whose path escapes the repo, which fails and names the file
  - no matching files, which passes
- [x] Step 5: `go build ./...` and `go test -p 2 ./internal/scaffolding/...`. Never run unbounded `go test ./...` on Windows.

## Verification

- Guard proved on shop's real file: copied `shop/system/multitier/backend-typescript/test/support/migrations.ts` into a scaffold-shaped temp dir; the shipped 4-up literal resolved outside the repo (`exists: False`), and after the new 3-up rewrite it resolved to `<repo>/db/migrations` (`exists: True`).
- Operator re-runs gh-acceptance-stage and confirms `Smoke (ubuntu-latest, multitier, monorepo, typescript)` goes green. If possible, also run a multitier/multirepo/typescript combo, which uses the `bDir` call site and was not in this run's matrix.

## Notes

- Java is not affected: Flyway `filesystem:../../db/migrations` is rewritten by `flywayPathReplacements`, and the Java smoke jobs passed. .NET is not affected: it has no `db/migrations` literal in source.
- Classification: scaffold-template drift (gh-optivem rewrite out of sync with a shop refactor). This is not a bug in shop.
