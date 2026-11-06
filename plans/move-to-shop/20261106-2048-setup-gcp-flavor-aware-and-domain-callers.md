# 2026-11-06 20:48 UTC — setup-gcp.sh flavor-aware; domain inputs in all multitier callers

> **Requires the `optivem/shop` repo.** Move this plan into shop and do the work there (from the user's shop chat). Split out of gh-optivem `plans/20261106-1305-scaffold-cloud-setup-guide.md` (Decision 5, Step 4, and the shop half of Step 4b).

## TL;DR

**Why:** Scaffolded repos copy shop's `setup-gcp.sh`. Today it creates Neon databases for all 6 flavors and ends with a hint naming `multitier-java-acceptance-stage.yml`, which is wrong for a single-flavor scaffold. Only the multitier-java acceptance caller passes the custom-domain inputs, so custom domains don't work for multitier-dotnet or multitier-typescript.
**End result:** `setup-gcp.sh` creates only the selected flavors' databases (all 6 by default) and prints a generic next-step hint. All three multitier acceptance callers pass the domain inputs.

## ▶ Next executable step (resume here)

Step 1 in shop.

## Steps

- [ ] Step 1 — Make `setup-gcp.sh` flavor-aware. Read the flavor list from an env var (or from the repo's workflows), defaulting to all 6, so it creates only those flavors' Neon databases plus Keycloak. Also make the closing "Trigger workflow" hint generic instead of naming `multitier-java-acceptance-stage.yml`.
- [ ] Step 2 — Pass `acc-domain: ${{ vars.ACC_DOMAIN }}` and `e2e-domain: ${{ vars.E2E_DOMAIN }}` in the multitier-dotnet and multitier-typescript acceptance callers, as multitier-java already does. Monolith stays without them, because `_acceptance-stage.yml` rejects domains there.
- [ ] Step 2b — `setup-custom-domains.sh` is stored as mode 100644 but `setup-gcp.sh` executes it directly (`setup-gcp.sh:308`), so shop's own call likely fails: `git update-index --chmod=+x setup-custom-domains.sh`.
- [ ] Step 2c — `setup-gcp.sh` and `setup-custom-domains.sh` refer to `docs/environments.md`, which scaffolds don't receive. Point them at a doc that travels with the scripts (e.g. `terraform/README.md`) or tell gh-optivem to copy `docs/environments.md`.
- [ ] Step 3 — Cut a shop `meta-v*` release so gh-optivem scaffolds pick the changes up (the scaffolder pins the latest release). Then tell the gh-optivem side, so its scaffold rewrites of these hints can be simplified or dropped.
