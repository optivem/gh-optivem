# 2026-11-06 13:05 UTC — Cloud setup guide for scaffolded repos (Cloud Run acceptance, QA, prod)

## TL;DR

**Why:** A repo scaffolded from shop's single-pipeline layout gets Docker gates that run out of the box. Its manual cloud acceptance stage, QA and prod stages deploy to Google Cloud Run, though, and need GCP, Neon and GitHub setup that `gh optivem init` doesn't do. The scaffold doesn't even ship what `setup-gcp.sh` needs to run (the `terraform/` directory).
**End result:** A scaffold user can follow one guide (with a working copy of shop's setup script) to take a fresh repo to a green cloud acceptance run, QA deploy and prod deploy on their own GCP project and Neon account, with the cost capped by the kill-switch. They can also tear it all down again. The scaffold README links to this guide.

## ▶ Next executable step (resume here)

Decisions are settled (all six recommendations accepted, 2026-11-06). Do Steps 1–3 and the gh-optivem half of 4b; hand Step 4 and the shop half of 4b to the user's shop chat. The scaffolder port it builds on shipped in gh-optivem `v1.6.106` (README "Pipeline" section; `init` skips QA/prod verification without GCP vars). Don't start Step 5 (live GCP) without the user: it costs money and needs their accounts.

## Context

Shop ref researched: shop `main` @ `2d663980`. "Shop" paths below are relative to the shop repo root.

### Accounts and local tools

- **GCP account with an active billing account.** `setup-gcp.sh:129-142` fails without one. The script creates the GCP project itself (`terraform/main.tf:17-29`, ID `<system>-<6 hex>`), plus a per-Google-account Terraform state project and GCS bucket found by label (`terraform/state-backend.sh:4-18`).
- **Neon account** (free plan). Log in with `neonctl login` or set `NEON_API_KEY`, and set `NEON_ORG_ID` if the account has several orgs (`terraform/neon.sh:30-50`). Region `aws-us-east-2` (`terraform/neon.sh:9`).
- **GitHub:** `gh auth login` (`setup-gcp.sh:70-77`).
- **DNS: not needed today.** Every URL is `https://<service>-<project-number>.<region>.run.app` (`_deploy-stage.yml`, `url()` in Resolve Stack Settings, ~line 213). Caveat: an in-flight shop plan (`plans/20261104-0839-permanent-acc-e2e-environments.md:6,25,28`) adds custom domains (`shop-acc.optivem.com`) that fall back to `run.app` when unset. If it lands, DNS becomes an optional step in the guide.
- **Local tools:** `gcloud`, `gh`, `jq`, `terraform`, `node`/`npx` (for neonctl and for parsing the connection string, `setup-gcp.sh:258`), and `openssl` (`setup-gcp.sh:36-114`). Shop's devcontainer pins them all. Scaffolds get none of this unless they also copy the devcontainer (unverified whether they do).

### What `setup-gcp.sh` creates (Terraform plus scripted steps)

- **APIs:** run, artifactregistry, secretmanager, iamcredentials, iam, cloudresourcemanager, billingbudgets, cloudbilling, pubsub, cloudfunctions, cloudbuild, eventarc, storage (`terraform/main.tf:36-52`).
- **Workload Identity Federation:** a pool and a GitHub OIDC provider, restricted to the one repo (`attribute_condition`, `terraform/main.tf:61-81`). The `github-deployer` SA gets `run.admin`, `iam.serviceAccountUser` and `secretmanager.secretAccessor` (`terraform/main.tf:89-128`). There's a separate runtime SA (`:131`). The binding is per repo, not per environment. Renaming or transferring the repo breaks it until setup is re-run.
- **Secret Manager:** `<system>-db`, `<system>-keycloak-admin-password` and `<system>-monolith-client-secret` (`terraform/main.tf:142-160`). Their values are filled outside Terraform (`setup-gcp.sh:262-301`).
- **Neon:** one project named `<system>`, with databases `<sys>_{multitier,monolith}_{java,dotnet,typescript}_{real,stub,qa,production}` plus `<sys>_keycloak` (`setup-gcp.sh:222-245`). That's all 6 flavors. A scaffold only needs its own flavor's 4 databases plus Keycloak, so this is shop-specific but harmless.
- **Billing kill-switch:** a budget, Pub/Sub and a Cloud Function that unlinks billing above `BILLING_KILL_SWITCH_AMOUNT` (default 1 in the account's currency) (`terraform/billing-kill-switch.tf:1-30`, `terraform/README.md:164-170`).
- **GitHub repo variables:** `GCP_PROJECT_ID`, `GCP_REGION` (default `us-central1`, `terraform/variables.tf:1-4`), `GCP_WORKLOAD_IDENTITY_PROVIDER`, `GCP_SERVICE_ACCOUNT` and `SYSTEM_NAME` (`setup-gcp.sh:207-211`).
- **Check only:** the script warns if the secrets `GHCR_TOKEN` and `WORKFLOW_TOKEN` are missing (`setup-gcp.sh:303-337`).
- **Teardown:** `teardown-gcp.sh` destroys the GCP project (via the remote state), the Neon project and the 5 repo variables (`teardown-gcp.sh:4-80`).

### What each stage needs

| Stage | Vars (repo level) | Secrets | GitHub environment |
|---|---|---|---|
| Docker gates | `DOCKERHUB_USERNAME` | `DOCKERHUB_TOKEN`, `GHCR_TOKEN`, `SONAR_TOKEN` | none (unverified) |
| Cloud acceptance | the 5 GCP/SYSTEM vars (`_acceptance-stage.yml:121-136`) | `WORKFLOW_TOKEN`, `GHCR_TOKEN`, `SONAR_TOKEN` (`:100,167,729,817`) | `<arch>-<lang>-acceptance` (`:107,215,266,297,498`) |
| QA | the 5 vars (`_qa-stage.yml:60-77`) | `GHCR_TOKEN`, `WORKFLOW_TOKEN` | `<prefix>-qa` (`_qa-stage.yml:220`, plus `_deploy-stage.yml:69,146,178`) |
| Prod | the 5 vars (`_prod-stage.yml:66-83`) | `GHCR_TOKEN`, `WORKFLOW_TOKEN` | `<arch>-<lang>-production` (`_prod-stage.yml:263`) |

Plus the following:
- **Public GHCR packages.** Cloud Run pulls images anonymously, so every app image and every shared `shop-*` image must be public. `build-shared-images` fails early otherwise (`.github/actions/build-shared-images/action.yml:3-6,67-101`). The fix link it prints uses `/orgs/<owner>/packages/...` (`:101`), which may be wrong for a personal-account scaffold (unverified).
- **Keycloak.** One shared Keycloak on Cloud Run with realms `shop`, `shop-qa` and `shop-prod` (`.github/actions/deploy-keycloak/action.yml:3-6,74-81`). `shop-prod` has no users, so the user adds real users in the admin console (`terraform/README.md:177-179`). The admin password is in Secret Manager (`setup-gcp.sh:283-291`). Whether scaffolds rename the realms is unverified.
- **Docker-only QA/prod.** Shop documents an inactive Docker Compose block in QA/prod. Swapping it in needs only the DockerHub credentials and no GCP (`docs/pipeline/qa-and-prod-stages.md:23-35`).

### What gh-optivem `init` already does

- **Secrets:** `DOCKERHUB_TOKEN`, `SONAR_TOKEN`, `WORKFLOW_TOKEN`, `GHCR_TOKEN` and `REPO_TOKEN`. **Variable:** `DOCKERHUB_USERNAME` (`internal/scaffolding/steps/github_setup.go:91-104`). So every secret the cloud stages need is already set. None of the 5 GCP/SYSTEM variables is.
- **Environments:** bare `acceptance`, `qa` and `production` (`github_setup.go:49-61`). The template rewrite strips the `<arch>-<lang>-` prefix from `environment:` (comment at `:52-54`). The new `_*-stage.yml` files build names from `${{ inputs.architecture }}-${{ inputs.language }}-…` expressions, which a literal-prefix rewrite won't match. Check this in the sibling plan's Step 1 (unverified).
- **Scripts:** `copyCloudRunScripts` copies `setup-gcp.sh` and `teardown-gcp.sh` only when `--deploy cloud-run` (`internal/scaffolding/steps/apply_template.go:15,326-328,1115-1123`) and makes them executable (`finalize.go:330-334`). It does **not** copy `terraform/`, which both scripts `source` (`setup-gcp.sh:32-34`, `teardown-gcp.sh:29,43`). No Go file mentions `terraform`, so the scaffolded scripts fail at line 32. The sibling plan may drop `--deploy` entirely (shop plan `20261102-1855-single-pipeline-deploy-target.md:78`).
- **Docs:** `docs/setup.md` and `README.md` have no GCP, Cloud Run or Neon content (grep).

### Limits

- **SYSTEM_NAME.** Derived from the repo name: lowercased, non-`[a-z0-9-]` characters replaced with `-`, cut to 14 characters (`terraform/main.tf:7`, `teardown-gcp.sh:59`), or overridden with `SYSTEM_NAME` (4-14 characters, `terraform/variables.tf:12-19`).
- **Cloud Run's 49-character service-name limit.** Names have the form `<system>-<arch>-<lang>-<stage>[-<role>]`. A system name over 8 characters breaks multitier-typescript prod, and over 12 breaks multitier-dotnet (`setup-gcp.sh:6-7`, `terraform/main.tf:10-13`). `_deploy-stage.yml:203-223` fails early with the offending name. Typical scaffold repo names (`my-shop-multitier-java`) are longer than 8, so scaffold users will hit this. It's only a Terraform `check` warning, not an error.
- **Keycloak redirect URIs** are baked from the service names when the realm is imported, and an imported realm is never updated (`_deploy-stage.yml:203-206`). A user who changes `SYSTEM_NAME` after the first deploy needs a fresh Keycloak DB.

### Cost

About $0 a month in normal use: Cloud Run scales to zero with max 1-2 instances, and every cloud stage is `workflow_dispatch` only, with the cron commented out (`monolith-java-acceptance-stage.yml:10-12`). The kill-switch caps the worst case at about 1 unit of currency a month. Neon bills separately on its free plan (`terraform/README.md:144-188`). Uncommenting the cron burns through the free tier within days (`:181-183`).

## Decisions (user, 2026-11-06: all six recommendations accepted)

1. Doc plus the shop scripts; no `gh optivem cloud setup` command.
2. `init` always scaffolds `setup-gcp.sh`, `teardown-gcp.sh` and `terraform/` (minus `.terraform*`, `tfplan`, state), not gated on `--deploy`.
3. Cloud stages stay enabled (manual dispatch only, fail fast without vars).
4. Guide at gh-optivem `docs/cloud-setup.md`, linked from the scaffolded README; shop's `terraform/README.md` is copied along and linked as the deep reference.
5. Make `setup-gcp.sh` flavor-aware in shop (shop change: handed to the user's shop chat, not done from gh-optivem).
6. `init` warns when the derived `SYSTEM_NAME` is over 8 characters; it never auto-picks a name.

## Steps

- [ ] Step 1 — Write `docs/cloud-setup.md` in gh-optivem, covering:
  - prerequisites (accounts, tools, or open the devcontainer)
  - choosing `SYSTEM_NAME` (the 8/12-character rule)
  - `./setup-gcp.sh`
  - making the GHCR packages public
  - running cloud acceptance → QA → QA sign-off → prod
  - adding Keycloak `shop-prod` users
  - cost, kill-switch and restoring billing
  - the Docker-only QA/prod alternative
  - `./teardown-gcp.sh`
  - troubleshooting (WIF repo binding, missing variables, the 49-character error)

  Link it from `README.md` and `docs/setup.md`.
- [ ] Step 2 — Scaffold `setup-gcp.sh`, `teardown-gcp.sh` and `terraform/` unconditionally, minus local state files (per Q2). Coordinate with the sibling plan's Step 1, which owns the copy list. Add a test that a scaffolded repo contains `terraform/neon.sh` and `terraform/state-backend.sh`.
- [ ] Step 3 — Preflight in `init`: warn when the derived `SYSTEM_NAME` is over 8 characters (per Q6). Confirm the `environment:` rewrite matches the new `_*-stage.yml` expressions, and fix it or create `<arch>-<lang>-{acceptance,qa,production}` environments instead.
- [ ] Step 4 — (If Q5 is accepted) write a shop plan to make `setup-gcp.sh` flavor-aware and its "next steps" hint generic.
- [ ] Step 4b — Custom domains (shop P5, `setup-custom-domains.sh`, multitier only): scaffold the script for multitier flavors whenever `setup-gcp.sh` is scaffolded (today `setup-gcp.sh:306-308` calls `../setup-custom-domains.sh`, so setting `ACC_DOMAIN`/`E2E_DOMAIN` in a scaffold fails); rewrite its multitier-java hints (`multitier-java-acceptance-stage.yml` → `acceptance-stage.yml`) and check its service-name pattern `<system>-<architecture>-<language>-<stack>-frontend` matches the ported scaffold's Cloud Run names; ask shop to pass `acc-domain: ${{ vars.ACC_DOMAIN }}` / `e2e-domain: ${{ vars.E2E_DOMAIN }}` in the multitier-dotnet/typescript callers too (only multitier-java does); omit it for monolith (`_acceptance-stage.yml` rejects domains there). Guide section "Custom domains (optional)": verify the parent domain in Search Console for your gcloud account, CNAME → `ghs.googlehosted.com`, one cloud acceptance run, then `ACC_DOMAIN=… E2E_DOMAIN=… ./setup-custom-domains.sh` (sets the repo variables once mappings are Ready).
- [ ] Step 5 — **With the user (costs money, uses their GCP billing and Neon accounts):** scaffold a throwaway repo with a short name. Follow `docs/cloud-setup.md` word for word, and check that cloud acceptance creates an RC, QA deploys and is signed off, and prod deploys and publishes. Then run `./teardown-gcp.sh` and confirm that no GCP project, Neon project or repo variables remain. Fold any gaps back into the guide.

## Not in scope

- Porting the scaffolder to the single-pipeline layout, or the README pointer itself (shipped in `v1.6.106`).
- Changes to shop's scripts or Terraform. Those belong in a shop plan (Step 4 only writes that plan).
- Custom domains and DNS (shop plan `20261104-0839-permanent-acc-e2e-environments.md`). The guide adds it only once that lands.
- Cloud providers other than GCP, and replacing Neon.
- A Go `gh optivem cloud` command, unless Q1 is decided the other way.
