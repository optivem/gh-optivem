# 2026-11-03 15:36 UTC — Make the gh-optivem scaffolds emit shop's single-pipeline workflow set

> 🤖 **Picked up by agent** — `8091c04fb23f` at `2026-11-06T13:05:00Z`

> Moved from shop `plans/move-to-gh-optivem/` on 2026-11-06 (the shop-side copy still needs deleting: the push was blocked by the auto-mode classifier). Originally: **Requires the `optivem/gh-optivem` repo.** The scaffold templates and their copy list live in gh-optivem, not in shop. Do this work in a gh-optivem checkout, and cut a gh-optivem release afterwards. Split out of shop plan `20261102-1855-single-pipeline-deploy-target.md` (its Step 7) on 2026-11-03, at the user's request: anything that needs another repo goes into its own plan.

## TL;DR

**Why:** Shop now has two acceptance tracks: a Docker gate on every commit (latest, plus a deletable legacy track) and a manual cloud acceptance stage that owns the RC tag. QA and prod run on cloud. Scaffolded repos still get the old workflow set (see Inventory Surprise 11 in the shop plan for what's broken today).
**End result:** `gh optivem` scaffolds emit the same set as shop: Docker acceptance (latest, and legacy unless `--no-legacy`), manual cloud acceptance, and QA/prod callers with cloud active. The scaffold README says the Docker gates work out of the box, while cloud acceptance, QA and prod need GCP set up (or the Docker block uncommented in QA/prod).

## ▶ Next executable step (resume here)

Steps 1–3 done 2026-11-06 (shared `_*.yml` copied and path-rewritten via `sharedWorkflowRewrites` in `internal/scaffolding/steps/shared_pipeline.go`; only the new layout is supported — refs without it, incl. the current default `meta-v1.0.223`, are refused with a pointer to `--shop-ref main`; scaffolded legacy Docker gate gets an hourly schedule; README "Pipeline" section with a placeholder "Cloud setup" heading). Offline proof: `GH_OPTIVEM_SHOP_PATH=<shop clone> go test -run Shop ./internal/scaffolding/steps/` — 25 runs (6 flavors × 2 repo strategies × ±`--no-legacy` + mixed-language) pass with actionlint.

Do Step 4 now: `gh optivem init --shop-ref main` into a throwaway repo (monolith-java, then multitier-java), with and without `--no-legacy`; check commit-stage and the Docker acceptance gates go green with no GCP. Never run cloud acceptance/QA/prod (user, 2026-11-06). Delete the repos after. Hold the gh-optivem release until shop cuts a `meta-v*` with the new layout (until then the default pin is refused) and the user says go (release is the last thing in the run).

Known leftovers: shop-path comments in `.sh` scripts and `system-test/README.md` (already present at `meta-v1.0.223`) may be flagged by the post-push leftover check.

## Steps

- [ ] Step 4 — Run gh-optivem's scaffold rehearsal, check that a scaffolded repo's Docker gates go green with no GCP, then release gh-optivem.

## Decisions

- 2026-11-02 (in the shop plan): option (a), scaffolds inherit cloud as is.
