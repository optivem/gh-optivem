# Sonar naming — how to tackle the `optivem_shop` → `my-company_my-shop` rename

Context for a carve-out of the shop template's placeholder rename. Everything else in [shop/.plans/PLACEHOLDER-RENAME.md](../../shop/.plans/PLACEHOLDER-RENAME.md) (`.sln` names, Java packages, docker-compose DB names, etc.) is local-effect and can be renamed freely. SonarCloud is different: the projectKey is a pointer into a remote service, so renaming the string in the repo without first reconciling the remote state **breaks CI**.

## What happened (2026-04-24)

PLACEHOLDER-RENAME Phase 1 was executed across the shop template, including the SonarCloud identifiers. Commit stages for monolith-typescript and monolith-dotnet then failed on `main` with:

```
Detected project binding: NONEXISTENT
HttpException: Error 404 on https://api.sonarcloud.io/analysis/analyses
```

The `sonar.organization=my-company` / `sonar.projectKey=my-company_my-shop-*` values point at a SonarCloud org/projects that don't exist.

The Sonar identifiers were reverted to the pre-rename values in [optivem/shop@fcc00f9](https://github.com/optivem/shop/commit/fcc00f9cbb152d5f1203a35bf7905bc6d5573fec):

> Revert Sonar project keys/org to optivem_shop-* (SonarCloud projects under my-company_my-shop-* don't exist yet)

Reverted files (Sonar-only; `.sln`/gradle-group/description renames kept):

- `.github/workflows/monolith-typescript-commit-stage.yml`
- `.github/workflows/monolith-dotnet-commit-stage.yml`
- `.github/workflows/multitier-backend-typescript-commit-stage.yml`
- `.github/workflows/multitier-backend-dotnet-commit-stage.yml`
- `.github/workflows/multitier-frontend-react-commit-stage.yml`
- `system/monolith/java/build.gradle`
- `system/multitier/backend-java/build.gradle`
- `system-test/dotnet/Run-Sonar.ps1`
- `README.md` (SonarCloud badge links)

The corresponding plan item is still listed at [shop/.plans/PLACEHOLDER-RENAME.md:132](../../shop/.plans/PLACEHOLDER-RENAME.md#L132) and should be marked deferred to this plan.

## Why this carve-out lives in `gh-optivem`, not in the shop repo

`gh optivem init` creates SonarCloud projects during scaffolding ([main.go:198](../main.go#L198) via `steps.CreateSonarCloudProjects`), using `cfg.OwnerLower` as the org and the rewritten suffixes documented in [MAPPING.md:199-227](../MAPPING.md#L199-L227). Whatever the template commits as the Sonar projectKey in `.github/workflows/*.yml` and `build.gradle` files:

1. is static text the scaffolder rewrites via MAPPING.md when projecting the template onto a user's repo, **and**
2. must match a SonarCloud project that exists (created by the scaffolder, or pre-existing for the template's own CI).

So the template's own Sonar keys (`optivem_shop-*`) and the scaffolder's rewrite rules need to be planned together — this is the right repo for that.

## Three questions to answer before changing anything

### 1. What is the template's own Sonar identity?

Today: org `optivem`, keys `optivem_shop-*`. These exist on SonarCloud and are what CI pushes analyses to.

Options:

- **(a) Keep `optivem_shop-*` — recommended.** The template repo is publisher-real (`optivem/shop`), same as we decided to keep the GitHub repo name (see [shop/.plans/PLACEHOLDER-RENAME.md:158](../../shop/.plans/PLACEHOLDER-RENAME.md#L158)). Scaffolded outputs are what the placeholder rename targets, not the template. This matches the existing decision and requires no SonarCloud work.
- (b) Rename to `my-company_my-shop-*`. Only makes sense if we also rename the GitHub repo and adopt `my-company` as the template's publisher identity — which contradicts the "template stays publisher-real" decision. Also needs a new SonarCloud org `my-company` and 7 new projects created before any CI run can succeed.

If we pick (a), **no rename work is needed in the shop repo's Sonar config**. The PLACEHOLDER-RENAME Sonar item collapses to "no-op for the template; scaffolder already rewrites for user output."

### 2. What does the scaffolder produce for user output?

Today (MAPPING.md): org `{owner}`, keys `{owner}_{repo}-{system|backend|frontend}`. The scaffolder already:

- Reads `--owner` and lowercases it for the Sonar org.
- Rewrites template suffixes `-monolith-{lang}` / `-backend-{lang}` / `-frontend-{lang}` → `-system` / `-backend` / `-frontend`.
- Creates the projects via the SonarCloud API.

Gap to close: the scaffolder must also rewrite the **template's literal `optivem_` prefix** to `{owner-lower}_` in every file it copies. Verify by grepping the scaffolded output for `optivem_` — should be zero hits.

Action items:

- Audit `shell.SonarCloud` and `steps.CreateSonarCloudProjects` to confirm the projectKey prefix rewrite runs on copied workflow files and gradle files.
- Add a post-scaffold assertion: after file copy, grep for `optivem_` in workflow/gradle content and fail if any remain outside comments.
- Update MAPPING.md's Sonar section to state the prefix rewrite explicitly (currently it documents only the suffix rewrite).

### 3. What's the SonarCloud setup order for a fresh scaffold?

Confirm the scaffolder:

1. creates the SonarCloud org binding (if the user's `{owner}` org doesn't exist on SonarCloud yet — likely a manual prerequisite, document clearly);
2. creates each project *before* the first CI run that would try to push analysis to it;
3. fails fast with a clear error if `SONAR_TOKEN` is missing or the token lacks org-admin scope.

Current code path appears correct (step `CreateSonarCloudProjects` runs in `phaseApplyTemplate`, before any CI dispatch) — just re-verify and write a dry-run test that scaffolds against a throwaway org.

## Recommendation

Pick **option (a)** for the template. The Sonar rename was the wrong scope of PLACEHOLDER-RENAME: the template's Sonar identity is publisher-real, same as the GitHub repo name. Close out the Sonar item in `shop/.plans/PLACEHOLDER-RENAME.md:132` with a note pointing here.

Then do the three scaffolder verifications above (`optivem_` prefix rewrite, post-scaffold grep assertion, MAPPING.md doc update, fresh-scaffold dry-run). That's the real work — and it belongs here, not in the shop template.

## Out of scope for this plan

- Renaming the GitHub repo `optivem/shop` (decided: no — see [shop/.plans/PLACEHOLDER-RENAME.md:158](../../shop/.plans/PLACEHOLDER-RENAME.md#L158)).
- The non-Sonar parts of PLACEHOLDER-RENAME (`.sln`, Java packages, compose DB names, etc.) — those are local-effect and already proceeding in the shop repo.
- SonarCloud cleanup scripting — already covered by [scripts/cleanup-orphans.sh](../scripts/cleanup-orphans.sh).
