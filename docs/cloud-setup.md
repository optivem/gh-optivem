# Cloud setup (Google Cloud Run + Neon)

A repo scaffolded by `gh optivem init` runs its commit stage and Docker Compose acceptance gates out of the box. Three stages deploy to Google Cloud Run and need the one-time setup described here:

| Workflow | Trigger | What it does |
|---|---|---|
| `acceptance-stage.yml` | manual | Cloud acceptance: deploys the stub and real stacks to Cloud Run, runs the acceptance suites, and creates the release candidate (`v<x.y.z>-rc.N`). |
| `qa-stage.yml` | manual | Deploys an RC to QA and runs the smoke tests. |
| `qa-signoff.yml` | manual | Approves or rejects the RC deployed to QA. |
| `prod-stage.yml` | manual | Deploys the latest QA-approved RC to production, publishes the release (`v<x.y.z>`) and bumps the patch version. |

Until you finish this guide, those stages fail at their first GCP step, and `init` skips verifying them (see [Troubleshooting](#troubleshooting)).

The setup is scripted. `init` copies the scripts and their Terraform into your repo:

- `setup-gcp.sh`, which creates everything below.
- `teardown-gcp.sh`, which removes it all again.
- `terraform/`, the Terraform configuration plus helper scripts (`neon.sh`, `state-backend.sh`) that both scripts source. Its `terraform/README.md` is the deep reference for naming, state, cost and troubleshooting.
- `setup-custom-domains.sh`, multitier flavors only (see [Custom domains](#custom-domains-optional-multitier-only)).

Run every command below from the root of the repo that holds `acceptance-stage.yml`.

## 1. Prerequisites

**Accounts**

- A **Google Cloud account with an active billing account.** The script creates the GCP project for you. You don't create one by hand. If no billing account is found, the script stops and points you at <https://console.cloud.google.com/billing>.
- A **Neon account** (the free plan is enough): <https://console.neon.tech>.
- **GitHub CLI logged in** to an account that can set variables on the repo.

**Tools**: `bash`, `gcloud`, `gh`, `jq`, `terraform`, `node` (with `npx`) and `openssl`. The script checks for the first four and prints an install command for your OS. Neon's CLI, `neonctl`, doesn't need installing: the scripts run a pinned version through `npx` if it's missing.

The scaffold does **not** ship a devcontainer. If you'd rather not install the tools, shop's devcontainer (`.devcontainer/` in [optivem/shop](https://github.com/optivem/shop)) pins all of them. You can copy it into your repo yourself.

**Log in** (once per machine):

```bash
gh auth login
gcloud auth login --no-launch-browser
npx -y neonctl login          # or: export NEON_API_KEY=<key>  (Neon console → Account settings → API keys)
```

If your Neon account belongs to several organizations, the script asks you to choose one. Set `NEON_ORG_ID=<org-id>` before running it.

The two GitHub secrets the cloud stages use, `GHCR_TOKEN` and `WORKFLOW_TOKEN`, were already set by `init`. `setup-gcp.sh` checks they exist and tells you if one is missing.

## 2. Choose the system name

Everything the script creates is named after a **system name**. That includes the GCP project (`<system>-<6 hex>`), the Neon project (`<system>`), the secrets (`<system>-db`, …) and every Cloud Run service (`<system>-<architecture>-<language>-<stage>[-<role>]`).

By default, the name comes from the **GitHub repo name**, not from `init --system-name`. The repo name is lowercased, every character other than `a-z`, `0-9` and `-` becomes `-`, and the result is cut to 14 characters. To override it, pass `SYSTEM_NAME` (4–14 characters: lowercase letters, digits and hyphens, starting with a letter and not ending with a hyphen):

```bash
SYSTEM_NAME=bookshop ./setup-gcp.sh
```

**Keep it short.** Cloud Run limits service names to 49 characters, and the longest name is the production frontend:

| Your flavor (`architecture`/`language` inputs in `.github/workflows/acceptance-stage.yml`) | Max system name |
|---|---|
| multitier + typescript | **8** |
| multitier + dotnet | **12** |
| multitier + java, any monolith | 14 |

`init` warns when the name derived from your repo is over 8 characters. Terraform also warns at plan time. Neither is an error: the first deploy that hits the limit fails with the offending service name (see [Troubleshooting](#troubleshooting)).

**Choose the name once.** Re-runs of `setup-gcp.sh` must use the same name. If you overrode it, pass it again on every re-run, for example `SYSTEM_NAME=$(gh variable get SYSTEM_NAME) ./setup-gcp.sh`. Otherwise Terraform plans to replace the project. Changing the name later means a teardown and a fresh setup (see [Troubleshooting](#troubleshooting)).

## 3. Run `./setup-gcp.sh`

```bash
./setup-gcp.sh            # add --yes to apply plans that change or delete resources without asking
```

Optional environment variables:

- `SYSTEM_NAME`: see above.
- `BILLING_KILL_SWITCH_AMOUNT`: a whole number, default `1`. See [Cost](#7-cost-and-the-billing-kill-switch).
- `KC_ADMIN_PASSWORD`: the Keycloak admin password. If you don't set it, one is generated.

The script is idempotent. Re-run it to resume after a failure or to restore billing. It creates the following:

- **A Terraform state project** per Google account (`tfstate-<random>`, labelled `purpose=terraform-state`), with a versioned bucket `gs://<state-project>-tfstate`. This repo's state is stored under the prefix `<owner>/<repo>`. Every system you set up from the same Google account shares this project. Teardown keeps it.
- **The GCP project** `<system>-<6 hex>`, linked to your billing account. It contains:
  - the required APIs (Cloud Run, Secret Manager, IAM, billing and budgets, Pub/Sub, Cloud Functions, Cloud Build, Eventarc, Artifact Registry, Storage);
  - **Workload Identity Federation**: a pool and a GitHub OIDC provider that only accepts tokens from **this repo**, plus a `github-deployer` service account (Cloud Run admin, service account user, secret accessor) that the workflows impersonate (no JSON keys);
  - a `runtime` service account that the Cloud Run services run as;
  - three **Secret Manager** secrets, `<system>-db` (Neon credentials), `<system>-keycloak-admin-password` and `<system>-monolith-client-secret`. Their values are written by the script, never by Terraform;
  - the **billing kill-switch**: a budget, a Pub/Sub topic and a Cloud Function.
- **A Neon project** named `<system>` in `aws-us-east-2`, with one database per stack and stage, plus `<system>_keycloak` for Keycloak.
  - *Note:* the script currently creates the databases for all six shop flavors (`<sys>_{multitier,monolith}_{java,dotnet,typescript}_{real,stub,qa,production}`), not only yours. The extra ones are empty and harmless.
- **GitHub repo variables:**

| Variable | Value |
|---|---|
| `GCP_PROJECT_ID` | the new project's ID |
| `GCP_REGION` | `us-central1` (the Terraform default) |
| `GCP_WORKLOAD_IDENTITY_PROVIDER` | the WIF provider's resource name |
| `GCP_SERVICE_ACCOUNT` | the `github-deployer` service account's email |
| `SYSTEM_NAME` | the system name |

When it finishes, the script prints a "Next steps" hint naming `multitier-java-acceptance-stage.yml`. That's shop's file name. In your repo the file is `acceptance-stage.yml`.

## 4. Run the cloud acceptance stage, then make the GHCR packages public

Cloud Run pulls your images from GHCR **anonymously**, so every image it deploys must be a **public** package:

- your app images (`ghcr.io/<owner>/<repo>/...`), pushed by the commit stage;
- the shared images the cloud acceptance stage builds on its first run: `<repo>/shop-db-migrate`, `<repo>/shop-keycloak`, `<repo>/shop-simulators` and `<repo>/shop-stubs`.

The shared images don't exist until the first run, so the order is:

1. Start the cloud acceptance stage:

   ```bash
   gh workflow run acceptance-stage.yml                      # latest commit-stage images
   gh workflow run acceptance-stage.yml -f commit-sha=<sha>  # or a specific commit
   ```

2. If any package is private, the **Check Images Are Publicly Pullable** step fails and names each one.
3. Make each named package public: open the package on GitHub (your profile or org → **Packages**), then **Package settings → Danger Zone → Change visibility → Public**.
   - *Unverified:* the fix link the step prints has the form `https://github.com/orgs/<owner>/packages/...`. For a repo under a personal account, the settings page is probably under `https://github.com/users/<owner>/packages/container/<package>/settings` instead. Going through **Packages** on your profile always works.
   - Packages in a public repo may already be public, because new packages can inherit the repo's visibility. The check step tells you either way.
4. Re-run the stage.

A green run deploys the acceptance (`stub`) and E2E (`real`) environments, the shared Keycloak, and creates the release candidate `v<x.y.z>-rc.N`.

## 5. QA, QA sign-off, production

Every step is manual:

```bash
gh workflow run qa-stage.yml                           # deploys the latest RC to QA (or: -f version=v1.0.0-rc.1)
gh workflow run qa-signoff.yml -f result=approved      # or result=rejected; signs off the RC currently in QA
gh workflow run prod-stage.yml                         # deploys the latest QA-approved RC, publishes v<x.y.z>, bumps the patch version
```

QA and production each get their own Cloud Run services and database. All environments share one Keycloak service (`<system>-keycloak`), with one realm per stage: `shop` (acceptance), `shop-qa` and `shop-prod`.

`init` has already created the bare GitHub environments `acceptance`, `qa` and `production`. Add protection rules to them (for example, required reviewers on `production`) if you want them.

## 6. Add production users in Keycloak

The `shop-prod` realm has no test users. Add real users in the Keycloak admin console:

```bash
PROJECT=$(gh variable get GCP_PROJECT_ID); REGION=$(gh variable get GCP_REGION); SYSTEM=$(gh variable get SYSTEM_NAME)
gcloud run services describe "$SYSTEM-keycloak" --region "$REGION" --project "$PROJECT" --format='value(status.url)'   # Keycloak URL
gcloud secrets versions access latest --secret="$SYSTEM-keycloak-admin-password" --project="$PROJECT"             # admin password
```

Open `<keycloak-url>/admin/`, log in as `admin`, switch to the **shop-prod** realm, and add users under **Users**.

## 7. Cost and the billing kill-switch

- **About $0 a month in normal use.** Every service scales to zero (min instances 0, max 1–2). Every cloud stage is manual, and the cloud acceptance cron in `acceptance-stage.yml` is commented out. Images are pulled from public GHCR, so nothing is stored in Artifact Registry except the kill-switch's own build image. Neon is billed separately, on its free plan.
- **The kill-switch** caps the worst case. A budget on the project, set at `BILLING_KILL_SWITCH_AMOUNT` (default `1`, in your billing account's currency), triggers a Cloud Function that **unlinks the project from billing** once the month's cost passes the budget. Budget data lags usage by hours, so the cost can overshoot by a few cents. After that, Cloud Run stops serving and the cloud stages fail until billing is restored.
- **To restore billing**, re-run `./setup-gcp.sh` and confirm the billing change (or pass `--yes`). Raise the limit first (`BILLING_KILL_SWITCH_AMOUNT=5 ./setup-gcp.sh`) or wait for the next month. Otherwise the next budget notification unlinks billing again.
- **What would start costing money:** uncommenting the hourly cron in `acceptance-stage.yml` (it uses up the Cloud Run free tier within days, and then the kill-switch fires), `min-instances ≥ 1`, or "CPU always allocated" billing.

Details are in the Cost section of `terraform/README.md` (also on [GitHub](https://github.com/optivem/shop/blob/main/terraform/README.md#cost)).

## 8. Alternative: QA and production on Docker Compose

`.github/workflows/_qa-stage.yml` and `_prod-stage.yml` each contain an inactive block, `# === Docker Compose deploy (inactive) ===` … `# === End of Docker Compose deploy ===`. To deploy QA or production with Docker Compose instead of Cloud Run:

1. Delete the active `deploy` job above the block.
2. Uncomment the block.

The block needs `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`, which `init` already set, and none of the GCP variables.

Two caveats:

- **It's only a zero-cost stand-in.** The stack runs on the GitHub runner and is thrown away when the job ends, so there is nothing for a tester or user to log in to. The release steps still run.
- **It doesn't remove the need for GCP.** QA deploys only release candidates, and only the cloud acceptance stage (`acceptance-stage.yml`, Cloud Run) creates them. The Docker Compose acceptance gates create none. So you still need the cloud setup for acceptance, and a fully GCP-free pipeline isn't supported today.

## Custom domains (optional, multitier only)

By default, every URL is a Cloud Run URL: `https://<service>-<project-number>.<region>.run.app`. In a multitier repo you can put the acceptance (`stub`) and E2E (`real`) frontends on your own domains. QA and production stay on `run.app`, and monolith repos don't support custom domains (`_acceptance-stage.yml` rejects them).

1. **Verify the parent domain.** In [Search Console](https://search.google.com/search-console), verify the parent domain (for example `example.com`) for the Google account `gcloud` uses. Check it with `gcloud domains list-user-verified --project=$(gh variable get GCP_PROJECT_ID)`.
2. **Add the DNS records.** Add a CNAME for each subdomain (for example `bookshop-acc`, `bookshop-e2e`) pointing to `ghs.googlehosted.com`.
3. **Deploy once.** Run `gh workflow run acceptance-stage.yml` once, so the frontend services to map exist.
4. **Map the domains.** Run the script below. It uses `gcloud beta`, which may prompt you to install the beta component.

   ```bash
   ACC_DOMAIN=bookshop-acc.example.com E2E_DOMAIN=bookshop-e2e.example.com ./setup-custom-domains.sh
   ```

   It maps `<system>-multitier-<language>-stub-frontend` and `-real-frontend` to the domains. It sets the repo variables `ACC_DOMAIN` and `E2E_DOMAIN` only once each mapping is Ready, meaning its certificate has been issued, usually 15–60 minutes after the CNAME resolves. Re-run it with the same variables until both report ready.
   - You can also pass `ACC_DOMAIN`/`E2E_DOMAIN` to `./setup-gcp.sh`, which calls this script. Before the first acceptance run, though, the services don't exist yet, so step 4 is still needed.
5. **Check the caller passes the domains.** `.github/workflows/acceptance-stage.yml` must pass the domains to `_acceptance-stage.yml`, under `with:`:

   ```yaml
   acc-domain: ${{ vars.ACC_DOMAIN }}
   e2e-domain: ${{ vars.E2E_DOMAIN }}
   ```

   Shop's multitier-java caller has these lines. Its multitier-dotnet and multitier-typescript callers don't yet, so add them if they're missing. From the next run, the UI tests use `https://<domain>`, and the domain is added to the backend's allowed origins and to the Keycloak `shop` realm's redirect URIs.

`teardown-gcp.sh` removes the mappings (they go with the GCP project) and the two variables. Remove the CNAME records yourself.

## 9. Tear it all down

```bash
./teardown-gcp.sh
```

It finds the GCP project through the remote Terraform state, so it works from any clone. Before deleting anything it shows what it will remove and asks you to type the GCP project ID. It then deletes:

1. the GCP project, with all its services, secrets and the kill-switch;
2. the Neon project and all its databases;
3. the repo variables `GCP_PROJECT_ID`, `GCP_REGION`, `GCP_WORKLOAD_IDENTITY_PROVIDER`, `GCP_SERVICE_ACCOUNT`, `SYSTEM_NAME`, `ACC_DOMAIN` and `E2E_DOMAIN`.

It's safe to re-run: when nothing is left, it prints "Nothing to tear down — already clean." The shared Terraform state project is kept. GHCR packages and the GitHub environments are not touched.

Use `./teardown-gcp.sh`, not a bare `terraform destroy`. A bare destroy removes only the GCP side and leaves the Neon project and the variables behind.

## Troubleshooting

**`init` says it skipped QA, QA sign-off, production and cloud acceptance.** `init` (with `--verify-level qa` or `release`) checks for the five variables `setup-gcp.sh` sets. If any is missing, it logs a warning naming them and stops verifying after the Docker acceptance gates. That's expected on a fresh scaffold. Run `./setup-gcp.sh`, then dispatch the stages yourself (steps 4–5). Missing variables in a workflow run mean the same thing: setup hasn't run, or teardown removed them.

**`Cloud Run service name '<name>' is N characters (max 49). Use a shorter SYSTEM_NAME`.** Your system name is too long for your flavor (see [step 2](#2-choose-the-system-name)). Run `./teardown-gcp.sh`, then `SYSTEM_NAME=<shorter> ./setup-gcp.sh`, and re-run the stages from cloud acceptance.

**Changing `SYSTEM_NAME` after the first deploy.** Don't change it in place. The Keycloak realms are imported once, with redirect URIs built from the service names, and an imported realm is never updated. So a new name needs a fresh Keycloak database as well as new services. The clean way is `./teardown-gcp.sh`, then `SYSTEM_NAME=<new> ./setup-gcp.sh`, which creates a new GCP project and a new Neon project, and with it a new Keycloak database. Run teardown *first*: the Neon project is found by name, so after a rename the old one would be left behind.

**Workflows fail to authenticate to GCP after you renamed or transferred the repo.** Workload Identity Federation only trusts the exact `<owner>/<repo>` the setup ran for. The Terraform state is also stored under that `<owner>/<repo>` prefix, so running `./setup-gcp.sh` after a rename would find no state and create a *second* project. Safest order:

1. Rename the repo back, or run teardown before you rename.
2. Run `./teardown-gcp.sh`.
3. Rename or transfer the repo.
4. Run `./setup-gcp.sh` again.
5. Make the new GHCR packages public and run the stages again.

*(Unverified alternative: copy the state objects in `gs://<state-project>-tfstate/<old-owner>/<old-repo>/` to the new prefix, then re-run `./setup-gcp.sh` with the old `SYSTEM_NAME`. Terraform should then update the WIF binding in place.)*

**`No active billing account found`.** Set one up at <https://console.cloud.google.com/billing> and re-run.

**Cloud stages fail after working before, and Cloud Run returns errors about billing.** The kill-switch probably fired. See [Cost](#7-cost-and-the-billing-kill-switch).

**`Not authenticated to Neon` / `several organizations`.** Run `npx -y neonctl login` or export `NEON_API_KEY`, then set `NEON_ORG_ID` as the script tells you.

**`gcloud` asks you to log in again every day.** That's a Google Workspace session policy. The fix is in the Troubleshooting section of `terraform/README.md`.

## Reference

- `terraform/README.md` in your repo (also on [GitHub](https://github.com/optivem/shop/blob/main/terraform/README.md)) covers naming, variables, remote state, the full cost model, the kill-switch and troubleshooting.
- Shop's docs on [environments](https://github.com/optivem/shop/blob/main/docs/environments.md), the [acceptance stage](https://github.com/optivem/shop/blob/main/docs/pipeline/acceptance-stage.md), and the [QA and production stages](https://github.com/optivem/shop/blob/main/docs/pipeline/qa-and-prod-stages.md).
