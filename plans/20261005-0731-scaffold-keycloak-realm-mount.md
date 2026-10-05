# 2026-10-05 07:31:00 UTC — Scaffolded projects must carry the Keycloak realm file and a working compose mount

## TL;DR

**Why:** `gh-acceptance-stage` (run 37165938260, SHA 92da003) fails: all Smoke jobs die at "Acceptance Test" because `sky-travel-stub-keycloak-1` exits 1. Shop's compose files mount `../../keycloak/shop-realm.json`, which resolves only inside shop; the scaffolder copies `docker/<lang>/<arch>` but never `docker/keycloak/`, so generated projects have no realm file and a broken path.
**End result:** Scaffolded projects (monolith/multitier, monorepo/multirepo) are self-contained: they include the realm file, the compose mount points at it, and a fail-hard scaffold step stops any future drift before commit/push. `gh-acceptance-stage` Smoke is green again.

## Outcomes

- Every scaffolded project contains `docker/keycloak/shop-realm.json` and its compose files mount `./keycloak/shop-realm.json`.
- If shop changes the mount line so the rewrite no longer matches, scaffolding fails loudly at scaffold time (naming file, resolved path, expected path), not via a CI round-trip.
- Tests assert the rewrite against shop's real compose line text, not synthetic strings.
- `gh-acceptance-stage` Smoke matrix is green, unblocking the gh-optivem RC release.

## ▶ Next executable step (resume here)

Step 1: read `internal/scaffolding/steps/apply_template.go` (`copySystemTests` ~141-152, docker-dir handling ~747 and ~876), `names.go`, `verify.go`, `replacements_test.go`; then implement Steps 1-3 following precedents c3054718 (`simulatorContextPathReplacements`) and e220eed9 (`VerifyMigrationsPaths`). Gate: `go test ./...` green before anything is committed or dispatched.

## Steps

- [ ] Step 1: In `copySystemTests` also copy shop `docker/keycloak` to `<repo>/docker/keycloak`; add `Names.ShopKeycloakDir` in `names.go`. Check the multirepo/monorepo paths (~747, ~876) so every layout gets it.
- [ ] Step 2: Add `keycloakRealmPathReplacements()` rewriting `../../keycloak/shop-realm.json` to `./keycloak/shop-realm.json` in scaffolded `docker-compose.*.yml` (local/pipeline/stub/real variants). Wire it into every apply path (monolith + multitier, monorepo + multirepo).
- [ ] Step 3: Add a fail-hard verify step (like `VerifyMigrationsPaths` in `verify.go`, wired in `main.go` before commit/push): every scaffolded compose file's keycloak realm bind mount must resolve to an existing file in the generated repo; fail naming file, resolved path, expected path.
- [ ] Step 4: Tests in `replacements_test.go` / `verify_test.go` using shop's real compose line text from the shop checkout, not synthetic strings.
- [ ] Step 5: Update `docs/how-it-works.md`.
- [ ] Step 6: Run `go test ./...`. Then ask the author before re-dispatching `gh-acceptance-stage`. If Keycloak still exits 1, read `docker logs sky-travel-stub-keycloak-1` for secondary causes (`KC_HOSTNAME`, healthcheck port 9000).
- [ ] Step 7: Before any release, read shop `meta-release-stage.yml` and check whether `gh-acceptance-stage` uses shop `main` or a pinned shop version (decides whether a shop-side change is also needed). Release actions need author approval.

## Open questions

- Option B (shop-side layout change so one relative path works in both shop and scaffolds) is deferred as a follow-up. Recommendation: do Option A now, revisit B later.
- Does `docker-compose.pipeline.stub.yml` (or any workflow) use the same relative path? Step 2 must confirm.
