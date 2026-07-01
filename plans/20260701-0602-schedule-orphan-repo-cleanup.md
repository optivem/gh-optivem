# 2026-07-01 06:02:00 UTC — Schedule automated orphan-repo cleanup to stop stale test-app-* CI failures

## TL;DR

**Why:** `test-app-483d46fd-b658b5d0eb089d86`'s `acceptance-stage` workflow has failed every hour since 2026-06-27 because its GHCR preflight check can't get a token for a package that was never published. The repo is an orphaned ephemeral test fixture that nothing ever cleaned up — the cleanup mechanism exists but only runs on manual `workflow_dispatch`, never on a schedule.
**End result:** `gh-cleanup-orphans.yml` runs automatically on a schedule with a safe age cutoff, so orphaned `test-app-*` (and other prefixed) repos get swept before they can spam hourly CI failures for days, without risk of deleting a repo mid-active-test-run.

## Diagnosis (for context — no code changes needed here)

Failure: https://github.com/valentinajemuovic/test-app-483d46fd-b658b5d0eb089d86/actions/runs/28356216413 — `preflight` job fails: "Failed to obtain GHCR registry token for valentinajemuovic/test-app-483d46fd-b658b5d0eb089d86/system. Token missing or invalid" (from `optivem/actions`' `check-ghcr-packages-exist@v1`, `check.sh:35-39` — a sibling repo, not this one).

Root cause, pinned to this repo:
- `test-app-483d46fd-b658b5d0eb089d86` is an ephemeral test fixture scaffolded by gh-optivem's system tests (`internal/config/config_system_test.go:126` builds the `test-app-<hex>-<hex>` name; `internal/scaffolding/steps/github_setup.go:24-46` calls `gh.CreateRepo()`; `github_setup.go:91` seeds the `GHCR_TOKEN` secret from the operator's local env var via `internal/config/environment.go:32`).
- No GHCR container package was ever published for it (confirmed via `gh api users/valentinajemuovic/packages`), and/or its `GHCR_TOKEN` PAT has since gone stale. Confirmed empirically that GHCR's `/token` endpoint returns 200 for any validly-authenticated caller regardless of package existence — an empty bearer result means the credential itself is bad, not merely "package unpublished yet."
- Its hourly `acceptance-stage` cron (`schedule: '0 * * * *'`) has failed every run since 2026-06-27 (4+ days) because nothing ever cleaned it up.
- The cleanup mechanism already exists: `scripts/cleanup-orphans.sh` (default prefixes include `test-app-`) wired into `.github/workflows/gh-cleanup-orphans.yml`. But that workflow (`gh-cleanup-orphans.yml:3-4`) only has a `workflow_dispatch` trigger — no `schedule:` — so orphans only get swept when a human remembers to run it manually.
- This is a CI/workflow automation gap, not a code bug. `check.sh`'s hard-fail-on-indeterminate-token behavior is correct per this shop's check-* action fail-loud policy and must not change.

## Outcomes

- `gh-cleanup-orphans.yml` runs on its own on a recurring schedule (no human needs to remember to dispatch it).
- The scheduled run only deletes repos/resources older than a safe cutoff (e.g. 24h), so it can never delete a repo mid-active local test session.
- Existing manual `workflow_dispatch` behavior (all modes: `all`, `before-1h`..`before-12h`, `until-yesterday`, `custom-date`) is unchanged.
- `test-app-483d46fd-b658b5d0eb089d86` (and any other accumulated orphans) get deleted, stopping the hourly failure noise.

## ▶ Next executable step (resume here)

Edit `.github/workflows/gh-cleanup-orphans.yml`: add a `schedule:` trigger alongside the existing `workflow_dispatch:` trigger (e.g. daily cron, off-peak UTC hour), and extend the "Resolve cutoff date" step so that when the run is schedule-triggered (`github.event_name == 'schedule'`), it defaults to `--before 24h ago` and runs all targets (`--all --delete`) — without touching the existing `workflow_dispatch` input handling. Then validate the YAML (`actionlint`/`yamllint` if available in this repo's tooling, otherwise review by eye) and confirm the existing manual-dispatch modes are untouched by diff review.

## Steps

- [ ] Step 1: Add a `schedule:` cron trigger to `.github/workflows/gh-cleanup-orphans.yml` (daily, off-peak UTC).
- [ ] Step 2: Update the "Resolve cutoff date" step so a `schedule`-triggered run computes a safe default cutoff (e.g. 24h ago) instead of requiring `mode` input (which only exists for `workflow_dispatch`); keep all existing `workflow_dispatch` mode branches unchanged.
- [ ] Step 3: Update the "Run cleanup" step's env/args if needed so it works correctly for both trigger types (`github.event.inputs.mode` is only present for `workflow_dispatch` — guard against it being empty on `schedule` runs).
- [ ] Step 4: Validate the workflow YAML (lint/actionlint if available; otherwise careful manual review) and confirm no regressions to existing manual dispatch modes.
- [ ] Step 5 (manual, operator-run, not part of this plan's code diff): trigger `gh-cleanup-orphans` (or delete directly) to remove `test-app-483d46fd-b658b5d0eb089d86` and check for other accumulated orphans older than 24h.

## Open questions

- Preferred daily cron time (any off-peak UTC hour is fine — defaulting to `0 3 * * *` unless the user wants a different slot).
- Whether the scheduled run should include all targets (`--sonar --repos --docker --tmp`, i.e. `--all`) or just `--repos` — defaulting to `--all` to match the manual "all" default mode, since Sonar/Docker/tmp orphans are the same category of unattended cleanup problem.
