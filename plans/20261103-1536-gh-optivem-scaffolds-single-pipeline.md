# 2026-11-03 15:36 UTC — Make the gh-optivem scaffolds emit shop's single-pipeline workflow set


> Moved from shop `plans/move-to-gh-optivem/` on 2026-11-06 (the shop-side copy still needs deleting: the push was blocked by the auto-mode classifier). Originally: **Requires the `optivem/gh-optivem` repo.** The scaffold templates and their copy list live in gh-optivem, not in shop. Do this work in a gh-optivem checkout, and cut a gh-optivem release afterwards. Split out of shop plan `20261102-1855-single-pipeline-deploy-target.md` (its Step 7) on 2026-11-03, at the user's request: anything that needs another repo goes into its own plan.

## TL;DR

**Why:** Shop now has two acceptance tracks: a Docker gate on every commit (latest, plus a deletable legacy track) and a manual cloud acceptance stage that owns the RC tag. QA and prod run on cloud. Scaffolded repos still get the old workflow set (see Inventory Surprise 11 in the shop plan for what's broken today).
**End result:** `gh optivem` scaffolds emit the same set as shop: Docker acceptance (latest, and legacy unless `--no-legacy`), manual cloud acceptance, and QA/prod callers with cloud active. The scaffold README says the Docker gates work out of the box, while cloud acceptance, QA and prod need GCP set up (or the Docker block uncommented in QA/prod).

## ▶ Next executable step (resume here)

Only the release remains, and it is gated: **wait for shop to cut a `meta-v*` with the single-pipeline layout** (until then gh-optivem `main` refuses the default pin `meta-v1.0.223`), re-run the shop-backed scaffold test against that tag, then the user says go and the gh-optivem release is cut (the user wants it as the last thing; it also unblocks E2 in the meta-plan).

Done 2026-11-06: Steps 1–3 (`b684625e`), spec fix (`3a73ae55`: scaffolds ship `spec.yaml` + `docs/use-cases`). Rehearsal on shop `main` `2d663980`, throwaway repos deleted: monolith-java (legacy kept) commit-stage green, Docker legacy gate green (42225096172), Docker latest gate green after the spec fix (42228513314, spec coverage 13/14 = 92.9%); multitier-java `--no-legacy` backend + frontend commit stages and Docker latest gate green (42225102415). Cloud stages not run (user decision). Follow-ups: shop's `spec.yaml` `results` glob is Gradle-only, so .NET/TS `spec record` will miss results (shop-side, for the user's shop chat); the shop copy of this plan in `plans/move-to-gh-optivem/` still needs deleting from shop (user's shop chat); cloud setup guide is `plans/20261106-1305-scaffold-cloud-setup-guide.md`.

## Steps

- [ ] Step 4 — (rehearsal done 2026-11-06; release pending) Run gh-optivem's scaffold rehearsal, check that a scaffolded repo's Docker gates go green with no GCP, then release gh-optivem.

## Decisions

- 2026-11-02 (in the shop plan): option (a), scaffolds inherit cloud as is.
