# Fix unfiltered workflow-status badges across `gh-optivem`, the scaffolding template, and the rest of the academy

## Motivation

GitHub's `actions/workflows/<file>.yml/badge.svg` endpoint, when called with no query string, shows the latest run **on the default branch for any event the workflow declares**. For workflows whose only trigger is `workflow_dispatch` or `workflow_call`, that run never happens via a "default" event, and the badge degrades to "no status" forever. For workflows that mix push + schedule + dispatch, the badge flips between unrelated runs and stops being a reliable signal.

The shop repo just shipped a fix for this (commit `d9b44ce2`, `optivem/shop`): commit-stage badges got `?event=push`, scheduled meta-pipelines got `?event=schedule`, and dispatch-only "Per-language pipeline drivers" were deleted from the README outright. The same blind spot exists in `gh-optivem`'s own README, in its scaffolding template (so every newly scaffolded repo inherits the bug), and in five other academy repos. This plan brings them in line with the shop convention.

The convention, applied uniformly:

- `on: push` (with or without `pull_request`) → append `?event=push`.
- `on: schedule` (with or without `workflow_dispatch`) → append `?event=schedule`.
- `on: workflow_dispatch` only, or `on: workflow_call` only, or both with no recurring trigger → **delete the badge**. There is no event filter that yields a meaningful status, and an unfiltered badge will read "no status" indefinitely.

## Items

Three independent change sets, ship-able separately. Item 2 is a code change in this repo; Items 1 and 3 are README-only edits split across repos.

### 1. `gh-optivem`/README.md — drop dead badges, filter the survivor

Five badges currently sit at the top of `README.md`. Four of them point to dispatch-only or `workflow_call`-only workflows and will read "no status" forever; one points to a push workflow but lacks `?event=push`.

| Line | Workflow file | Trigger declared | Action |
|---|---|---|---|
| 1 | `gh-commit-stage.yml` | `push` + `pull_request` | append `?event=push` |
| 2 | `gh-acceptance-stage.yml` | `workflow_dispatch` only (schedule cron commented out) | **delete badge** |
| 3 | `gh-release-stage.yml` | `workflow_dispatch` only | **delete badge** |
| 4 | `gh-post-release-stage.yml` | `workflow_call` + `workflow_dispatch` | **delete badge** |
| 5 | `gh-local-stage.yml` | `workflow_dispatch` only | **delete badge** |

Resulting README header: a single `gh-commit-stage` badge with `?event=push`.

If the commented-out cron in `gh-acceptance-stage.yml` is intended to be re-enabled soon, revisit then and add the badge back with `?event=schedule`. Until then, drop it — a permanently-grey badge in the README is worse than no badge.

**Files:** `README.md` (this repo).

### 2. `gh-optivem`/`internal/steps/readme.go` — make the scaffolding template emit filtered badges and drop dead pipeline-stage badges

The badge format string is hardcoded at line 19:

```go
badgeFmt = "[![%s](%s/badge.svg)](%s)\n"
```

It is consumed by five generation paths (`pipelineBadges`, `writeMonolithMultirepoReadme`, `writeMultitierMultirepoReadme`, `writeComponentReadme`, `generateBadges`), all of which produce unfiltered URLs. Every scaffolded repo therefore inherits the same blind spot the shop fix just addressed for one of its READMEs.

Mapping the generated badges to the shop template's actual triggers (which the scaffolding copies into each new repo):

| Generated badge | Trigger in shop's template | Treatment |
|---|---|---|
| `commit-stage`, `backend-commit-stage`, `frontend-commit-stage` | `push` (+ PR) | `?event=push` |
| `acceptance-stage`, `acceptance-stage-legacy` | `schedule` + `workflow_dispatch` | `?event=schedule` |
| `qa-stage`, `qa-signoff`, `prod-stage` | `workflow_dispatch` only | **drop from `pipelineBadges`** |

**Implementation outline:**

1. Replace the single `badgeFmt` constant with a per-stage event tag. Concrete shape: turn the `[2]string` pairs in `pipelineBadges` / `generateBadges` / `writeMonolithMultirepoReadme` / `writeMultitierMultirepoReadme` / `writeComponentReadme` into `[3]string`s (`url`, `label`, `event`), and have `writeBadges` format `"[![%s](%s/badge.svg?event=%s)](%s)\n"`. Keep an empty-event fallback that emits no `?event=` if some future caller wants an unfiltered badge.
2. In `pipelineBadges`, drop `qa-stage`, `qa-signoff`, and `prod-stage`. The `acceptance-stage` and `acceptance-stage-legacy` entries get `event: "schedule"`. The constants `qaStage`, `qaSignoff`, `prodStage` become unused and can be removed alongside.
3. In `generateBadges`, `writeMonolithMultirepoReadme`, `writeMultitierMultirepoReadme`, `writeComponentReadme`: every `commit-stage`-family entry gets `event: "push"`.
4. Smoke-test by running the scaffolding against a throwaway repo (or `internal/scenarios/...` if a fixture exists) and inspecting the generated `README.md`.

**Out of scope:** updating `shop`'s README to match. The d9b44ce2 commit fixed shop's commit-stage and meta-* badges but left the dispatch-only `qa-stage` / `qa-signoff` / `prod-stage` badges in place. Reconciling shop's README to the new template output is a separate decision (it'd remove badges users may currently click); flag it as a follow-up rather than bundling here.

**Files:** `internal/steps/readme.go` (and any test that asserts badge content — none observed at audit time, but check during implementation).

### 3. Academy-wide README sweep

Six repos with badges, plus one bonus bug. Each is an independent README edit; group them however you like (one PR per repo, or one mass sweep — pick what's easier to review).

#### `eshop` — broken paths **and** missing filter

🚨 The README references `frontend-commit-stage.yml` / `backend-commit-stage.yml`, but the actual files in `.github/workflows/` are `commit-stage-frontend.yml` / `commit-stage-backend.yml`. Both badges are 404 today, regardless of any event filter.

- Fix the workflow paths first, then append `?event=push` (both workflows are push-triggered — verify during the edit).

#### `eshop-tests` — six schedule-driven badges

All six acceptance-stage workflows (`{java,dotnet,typescript}-acceptance-stage{,-legacy}.yml`) are `schedule` + `workflow_dispatch`.

- Append `?event=schedule` to all six.

#### `optivem-testing` — five badges, mixed

- `java-commit-stage.yml`, `dotnet-commit-stage.yml`, `typescript-commit-stage.yml` → `push` + PR + dispatch → `?event=push`.
- `acceptance-stage.yml` → `schedule` + dispatch → `?event=schedule`.
- `release-stage.yml` → `workflow_dispatch` only → **delete badge**.

#### `actions` — one badge

- `commit-stage.yml` → `push` + PR + dispatch → `?event=push`.

#### `hub` — one badge with four triggers

`dashboard.yml` is unusual: `workflow_run` (chained from auto-* workflows), `push` on main, `schedule` every 30 min, `workflow_dispatch`. There is no single "right" filter:

- `?event=push` shows the result of the most recent main-branch deploy — most user-meaningful for a docs/dashboard signal.
- `?event=schedule` shows the recurring health check.
- Leaving it unfiltered means the badge flips between unrelated triggers but is rarely "no status" because something runs every 30 min anyway.

**Recommendation:** `?event=push`, on the same logic as commit-stage badges elsewhere. But this one is a judgment call — flag it for the user before changing.

#### Repos with no badges (no action)

`github-utils`, `courses`, `claude`, `substack` — none of their READMEs reference `badge.svg`.

## Sequencing

These three items are independent. Suggested order:

1. **Item 1** first — smallest, lowest-risk, makes the host repo consistent with the rule before propagating it. Single file, single repo.
2. **Item 3** in parallel with Item 2 — the README sweep is mechanical and doesn't depend on the template change. Doing it first means the academy is consistent before any new scaffolding lands.
3. **Item 2** last — code change with the largest blast radius (all future scaffolded repos). After this lands, optionally reconcile shop's README to match (deferred follow-up).

## Verification

For each modified badge, after the edit:

- Open the rendered README on github.com (badges are not rendered usefully on a local preview — the PNG fetch only happens against the live API).
- Confirm each badge displays a current passing/failing state, not "no status" and not the icon for a different event class.

For Item 2 specifically, run the scaffolding locally against a throwaway target and grep the generated README for `badge.svg?event=` — every remaining badge URL should match, and no `qa-stage` / `qa-signoff` / `prod-stage` badge should appear.

## Out of scope / explicitly not doing

- **Reconciling `shop`'s pipeline-stage badges** (`qa-stage`, `qa-signoff`, `prod-stage`) with the new template output. The shop README still has them; the d9b44ce2 commit didn't touch them. Decide separately.
- **Adding badges where there are none today** (`github-utils`, `courses`, `claude`, `substack`). Those repos chose not to surface workflow status, and that's fine.
- **Adding `?branch=main`** alongside `?event=*`. The default-branch behaviour of the unfiltered endpoint is already correct for these repos; explicit `?branch=` would only matter if we cared about non-main branches, which we don't.
