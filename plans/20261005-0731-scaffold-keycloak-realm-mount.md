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

Steps 1-6 are done: `gh-acceptance-stage` run 37290144876 (`debug-shop-head`) is fully green after three fixes (realm copy + mount rewrite 2c67722c; runner exports `KEYCLOAK_URL_<label>` 44dcc8fe; shop backend-typescript problem-type URIs from one base constant d0cee415). Remaining: Step 7, the gh-optivem release, which needs explicit author approval.

## Steps

- [ ] Step 7: Release of gh-optivem needs author approval. Before it, note that the green run tested shop `main`, not a pinned release: the pinned `meta-v*` release (what shop's `meta-release-stage.yml` dispatches via `shop-tag`) predates shop fix d0cee415, so multitier/typescript scaffold lint may fail there until shop cuts a new `meta-v*`. Consider a normal (no `debug-shop-head`) dispatch after the next shop release.

## Open questions

- Option B (shop-side layout change so one relative path works in both shop and scaffolds) is deferred as a follow-up. Recommendation: do Option A now, revisit B later.
- Resolved: only `docker-compose.*.yml` files carry the `../../keycloak/shop-realm.json` mount (24 occurrences across all lang/arch/variants); no workflow uses it.
