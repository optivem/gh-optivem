# 2026-11-03 15:36 UTC — Make the gh-optivem scaffolds emit shop's single-pipeline workflow set

> 🤖 **Picked up by agent** — `8091c04fb23f` at `2026-11-06T13:05:00Z`

> Moved from shop `plans/move-to-gh-optivem/` on 2026-11-06 (the shop-side copy still needs deleting: the push was blocked by the auto-mode classifier). Originally: **Requires the `optivem/gh-optivem` repo.** The scaffold templates and their copy list live in gh-optivem, not in shop. Do this work in a gh-optivem checkout, and cut a gh-optivem release afterwards. Split out of shop plan `20261102-1855-single-pipeline-deploy-target.md` (its Step 7) on 2026-11-03, at the user's request: anything that needs another repo goes into its own plan.

## TL;DR

**Why:** Shop now has two acceptance tracks: a Docker gate on every commit (latest, plus a deletable legacy track) and a manual cloud acceptance stage that owns the RC tag. QA and prod run on cloud. Scaffolded repos still get the old workflow set (see Inventory Surprise 11 in the shop plan for what's broken today).
**End result:** `gh optivem` scaffolds emit the same set as shop: Docker acceptance (latest, and legacy unless `--no-legacy`), manual cloud acceptance, and QA/prod callers with cloud active. The scaffold README says the Docker gates work out of the box, while cloud acceptance, QA and prod need GCP set up (or the Docker block uncommented in QA/prod).

## ▶ Next executable step (resume here)

Precondition met 2026-11-06: shop `main` (`2d663980`) has the final names (shared `_acceptance-stage{,-docker-compose}.yml`, `_qa-stage.yml`, `_prod-stage.yml`, `_deploy-stage.yml`, `.github/actions/{deploy-keycloak,build-shared-images}`, `docker/keycloak/{Dockerfile,acceptance-realm.jq,...}`, per-flavor `*-acceptance-stage{,-docker-compose,-docker-compose-legacy}.yml`). No shop `meta-v*` has this layout yet (latest `meta-v1.0.223`, 2026-10-31, old `-cloud` twins), so develop and rehearse against `--shop-ref main`. Do Step 1, replacing the interim guard `checkShopWorkflowLayout` (`internal/scaffolding/steps/prepare.go`). Rehearsal (Step 4) runs only commit-stage and the Docker acceptance stages, never cloud (user, 2026-11-06). Cloud setup for scaffold users is its own plan (`plans/20261106-1305-scaffold-cloud-setup-guide.md`).

## Steps

- [ ] Step 1 — In gh-optivem, update the scaffold copy list and templates to the final shop file names (after shop Step 6b's renames: `_deploy-stage.yml`, `deploy-keycloak`, `build-shared-images`, `docker/keycloak/Dockerfile`, `acceptance-realm.jq`, and the `*-acceptance-stage{,-docker-compose,-docker-compose-legacy}.yml` callers).
- [ ] Step 2 — Support `--no-legacy` by leaving out the `-docker-compose-legacy` caller and its `_prerelease-pipeline.yml` dispatch.
- [ ] Step 3 — Update the scaffold README text as described in the End result.
- [ ] Step 4 — Run gh-optivem's scaffold rehearsal, check that a scaffolded repo's Docker gates go green with no GCP, then release gh-optivem.

## Decisions

- 2026-11-02 (in the shop plan): option (a), scaffolds inherit cloud as is.
