# `gh optivem init` auto-creates the GitHub Project board

## Motivation

`gh optivem init` already creates the GitHub repo(s), environments, secrets, and SonarCloud project — but it stops short of creating the **GitHub Project (v2) board** that the ATDD pipeline depends on. Today the operator has two choices:

- Pass `--project-url <url>` (defined at `internal/config/config.go:545`) pointing at a project they created by hand. The URL is baked into `gh-optivem.yaml` by `WriteOptivemYAML` (`internal/steps/optivem_yaml.go:56`).
- Leave it blank and fill in `gh-optivem.yaml` later by hand.

Both paths put manual project setup in the critical-path between scaffolding and a working ATDD cycle. The board itself is mechanical — title, status options, repo link — and there is no reason `init` cannot produce it.

A board created by hand via the GitHub UI also tends to ship with the **wrong status options** for the ATDD pipeline. The default new project has `Todo / In Progress / Done`. The pipeline reads `Ready` (`internal/atdd/runtime/board/board.go:189`), writes `In progress` (`board.go:415`), and writes `In acceptance` (`internal/atdd/runtime/actions/bindings.go:191`). Two of those three are missing by default. A user who runs `init`, manually creates a project via the UI, pastes the URL into the YAML, and tries to run `implement-ticket` will hit `ErrStatusFieldMissing` (`board.go:90`). Auto-creation is the right time to set the right options.

## Approach

Add a new step `CreateProjectBoard` in `internal/steps/project.go`, slotted into the **Setup repository** phase right after `CreateRepos` (so the repo exists when we link it). The step:

1. **Skips** if `cfg.ProjectURL` is already set — operator-supplied URL wins, no creation attempted. This preserves the existing escape hatch (re-using a shared department-wide project, for example).
2. Creates the project: `gh project create --owner <Owner> --title <Title> --format json` → parse `{url, number, id}`.
3. Customises the built-in **Status** field to the ATDD-expected options: `Backlog, Ready, In progress, In acceptance, Done`. The first preference is `gh project field-edit --single-select-options "..."`; if the gh CLI version available in CI/locally doesn't support editing the built-in Status field options that way, fall back to `gh api graphql` with the `updateProjectV2Field` mutation. Verify which path works against the pinned gh version before hard-coding.
4. Links the scaffolded repo(s) to the project: `gh project link <number> --owner <owner> --repo <fullRepo>`. For multirepo, links each component repo as well.
5. Writes the resulting URL into `cfg.ProjectURL` so the **existing** `WriteOptivemYAML` step (which already runs later in the same pipeline at `main.go:222` and reads `cfg.ProjectURL`) bakes it into `gh-optivem.yaml`. No separate file-update step needed.

The choice to write into `cfg.ProjectURL` rather than mutate the YAML directly is deliberate: `WriteOptivemYAML` is already the single source of truth for "render config into YAML" (the same function the existing `--project-url` path uses). Routing the auto-created URL through the same field keeps one write path.

Dry-run prints the planned commands without executing. Failures are hard errors, consistent with `CreateRepos` — a half-created scaffold (repo without project) is worse than a clean failure the user can re-run.

## Items

### 1. New step: `CreateProjectBoard`

**File:** `internal/steps/project.go` (new).

- `func CreateProjectBoard(cfg *config.Config, gh *shell.GitHub)` — signature matches the other `Setup*` steps.
- Early return when `cfg.ProjectURL != ""` with an info log (operator-supplied URL takes precedence).
- Honour `cfg.DryRun` — print the four `gh` commands that would run, no execution.
- Use `shell.RunCapture` for the create call so we can parse stdout JSON cleanly without log noise.
- Parse `{id, number, url}` out of the create response; fail-fast (`log.Fatalf`) on parse error or empty URL.
- Log `Created project board: <url>` on success (matches `logCreated` style in `github_setup.go:13`).

### 2. Status-field customisation

**File:** same as Item 1.

- After project creation, look up the built-in Status field via `gh project field-list <number> --owner <owner> --format json` to get its node ID.
- Try `gh project field-edit --id <fieldID> --project-id <projID> --single-select-options "Backlog,Ready,In progress,In acceptance,Done"`. If that returns "unsupported on built-in Status field" or similar, fall back to `gh api graphql` with `updateProjectV2Field` — the GraphQL path is documented and reliably supports the built-in field.
- The exact set of options is intentionally a **superset** of what ATDD reads/writes (`Ready`, `In progress`, `In acceptance`). `Backlog` and `Done` are familiar Kanban terms students will expect; including them keeps the board legible without affecting the runtime path. `equalStatus` (`board.go:382`) is case-insensitive, so casing within the option names is forgiving.

### 3. Repo linking

**File:** same as Item 1.

- `gh project link <number> --owner <owner> --repo <fullRepo>` for `cfg.FullRepo`.
- For `cfg.RepoStrategy == "multirepo"`:
  - `multitier`: link `cfg.BackendFullRepo` and `cfg.FrontendFullRepo`.
  - `monolith`: link `cfg.SystemFullRepo`.
- Linking allows `gh project item-list` to surface issues from the repo. Note: linking does NOT auto-add new issues to the board — it just establishes the relationship. See Open Questions below for the auto-add workflow decision.

### 4. Wire the step into the pipeline

**File:** `main.go`

- Add to `buildSteps` (around `main.go:209-212`), after `Create repositories` and before `Setup environments`:

```go
{name: "Create project board", phase: phaseSetupRepo, fn: func() { steps.CreateProjectBoard(cfg, gh) }},
```

- Update `printBanner`'s "Will create" block (`main.go:734-738`) to mention the project board when it'll be created (i.e. when `cfg.ProjectURL == ""`).

### 5. Update `printSummary` and `PrintRegistration`

**Files:** `main.go`, `internal/steps/registration.go`

- `printSummary` (`main.go:457`): print the project URL alongside `Repository:` / `Actions:` when `cfg.ProjectURL` is set (it always will be after this change unless creation was skipped via `--no-project`).
- `PrintRegistration` (`internal/steps/registration.go:14`): add a `Project:` line so the value lands on the registration form.

### 6. Optional opt-out flag: `--no-project`

**File:** `internal/config/config.go`

- Add `--no-project` (default `false`) following the convention of `--no-legacy` / `--no-local-tests` / `--no-local-sonar` / `--no-atdd`.
- When set, skip the entire `CreateProjectBoard` step (the step itself checks `cfg.NoProject` and early-returns with an info log).
- Mutually compatible with `--project-url`: passing the URL also skips creation, so the new flag is for the "no project at all" case (rare, but useful for CI smoke tests of the scaffolder that don't want to litter the org with throwaway projects).

### 7. Tests

**Files:** new tests in `internal/steps/project_test.go` and additions to existing test files where helpful.

- Pure-logic test: title derivation from `cfg` (whatever Open Question 1 lands on), and the JSON-parse path against a captured `gh project create --format json` sample.
- Skip-when-URL-set: `cfg.ProjectURL = "https://github.com/orgs/x/projects/9"` → step is a no-op (no shell calls).
- Skip-when-dry-run: prints the planned commands, no execution.
- Skip-when-`--no-project`: assert no shell calls.
- End-to-end-ish: stub the shell layer (the package already uses `shell.Run` / `shell.RunCapture`; tests can intercept via the existing test seams used in `github_setup` tests, if any — otherwise extract a small `Runner` interface for this step).
- For the Status-field customisation: assert the option list passed to `field-edit` is exactly the documented set, regardless of casing input.

### 8. Auto-add workflow (depends on Open Question 3)

**Files** (if pursued): `templates/shop` (or wherever the scaffold template lives) — add a `.github/workflows/auto-add-to-project.yml` using the `actions/add-to-project@v1` action, parameterised on the project URL.

- Parameter for the action's `project-url` input would need to be set per-repo. Since `gh-optivem.yaml` already has the URL, the workflow could read it via a small step that parses the YAML — or the URL could be substituted into the workflow at scaffold time by `apply_template.go`'s replacement pass.
- This is the only way to keep the board populated automatically; without it, every issue needs `gh project item-add` by hand or via another mechanism.

## Open questions

- **Project title.** Options:
  - `cfg.SystemName` (e.g. "Page Turner") — simplest, matches the repo's display name.
  - `cfg.SystemName + " ATDD"` — matches the `clauderun_test.go:189` convention ("Shop ATDD"), signals the board's purpose, but couples the title to a single workflow.
  - Configurable via `--project-title <title>` flag. Most flexible, more surface area.
  - **Lean:** `cfg.SystemName`. Students get a board named after their system; the ATDD lineage is implicit. Easy to rename in the UI later.
- **Auto-add workflow (Item 8).** Without it, linked repos still require manual `gh project item-add` per issue. Three paths:
  - Drop a `auto-add-to-project.yml` into the scaffold during `init` (Item 8 above). Pros: one-and-done; matches the "scaffolder produces a working pipeline" promise. Cons: extra workflow file in every scaffolded repo.
  - Document the manual step in the README and stop. Pros: no extra workflow. Cons: every new issue is a friction point — ATDD board-mode picks from `Ready`, so an empty board means the cycle has nothing to do.
  - Use the project's built-in workflow rules (configurable via the GitHub UI on the project itself) and have `init` set those up via GraphQL. Pros: no per-repo workflow file. Cons: the GraphQL surface for project workflow rules is more involved than the action-based path; needs verification.
  - **Lean:** include the auto-add workflow file (path 1). The scaffolder's promise is "running this gives you a working pipeline"; an empty board violates that.
- **Status options.** The required set for ATDD is `Ready / In progress / In acceptance`. Question is whether to also include `Backlog` and `Done`:
  - **Lean:** include both. They cost nothing at runtime (ATDD ignores statuses it doesn't recognise) and match standard Kanban vocabulary students will expect.
- **Idempotency on re-run.** What happens if the user re-runs `init` against an existing repo (`CreateRepo` already handles "exists" gracefully — `internal/shell/github.go:323`)? Two paths:
  - Mirror `CreateRepo`: list existing projects (`gh project list --owner <owner> --format json`), skip creation if a project with the same title exists, log a warning, and reuse the existing URL.
  - Always create a new project (each `init` run produces a fresh board). Simpler but pollutes the org with duplicates if the user is iterating.
  - **Lean:** mirror `CreateRepo` — list-first, reuse on title match. Aligns with the rest of `init`'s "rerunnable scaffold" contract.
- **`field-edit` vs `gh api graphql` for Status options.** The CLI flag may or may not work against built-in Status fields depending on gh version. Need to verify against the pinned gh version (whatever CI uses) and possibly document a minimum gh version. Alternative: skip the CLI attempt and go straight to GraphQL, which is more verbose but version-stable.

## Out of scope

- **Templated project views / saved filters.** Out for v1. The default board view is fine for ATDD board-mode.
- **Custom fields beyond Status.** ATDD only reads Status. Adding `Priority`, `Estimate`, etc. is a v2 nicety.
- **Cross-repo project sharing.** Each `init` produces its own project, even when multiple init runs share an owner. Multi-repo-per-project is a v2 use case.
- **Migrating existing scaffolded repos to a new auto-created project.** The `--project-url` opt-out covers the "I already have a board" case.
- **Deleting the project on init failure.** `cfg.KeepLocal = true` on error (`main.go:179`) — we keep the local scaffold for inspection; same logic should keep the remote project so the user can inspect it. Cleanup is the user's call.
- **A `gh optivem project` subcommand for board management.** The board CRUD that ATDD does internally is enough; a dedicated CLI surface is a separate plan if it ever becomes useful.

## Order of operations

1. Resolve the Open Questions above.
2. Verify the `gh project field-edit --single-select-options` path against the pinned gh version. If it works on built-in Status fields, use it; otherwise commit to the GraphQL path.
3. Land Items 1, 2, 3, 4 together — the step + its pipeline wiring is one coherent change.
4. Land Item 5 (summary / registration print) in the same PR (small, related).
5. Land Item 6 (`--no-project` flag) in the same PR (one-line addition to the flag set + one-line check in the step).
6. Land Item 7 (tests) in the same PR.
7. Land Item 8 (auto-add workflow) — only if Open Question 2 lands on path 1; otherwise drop this item.
8. Manual rehearsal: run `init` end-to-end against a throwaway test repo, verify the project is created with the right options, verify the repo is linked, verify a new issue appears on the board (if Item 8 lands), verify `implement-ticket` resolves the URL from `gh-optivem.yaml` and the ATDD cycle picks/moves cards correctly.
