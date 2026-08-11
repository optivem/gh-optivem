# How it works

`main.go` builds the root Cobra command and attaches every subcommand (`init`, `config`, `system`, `system-test`, `component-test`, `compile`, `test`, `doctor`, `commit`, `sync`, `actions`, `rate-limit`, `implement`, `run`, `process`, `architecture`, `environment`, `branch`, `pr`, `hooks`, `cleanup`, `claude`, `output`). Cobra handles `--help` at every level, `--version` at the root, shell completion, and unknown-subcommand suggestions.

This document walks the **`init` scaffolding pipeline** specifically — the longest and least self-evident of those commands. For the others, `gh optivem <cmd> --help` is the reference.

## Startup

1. **Load the user-level `.env`** — before Cobra runs, `main()` reads the user-level env file (`%AppData%\gh-optivem\.env`, `~/.config/gh-optivem/.env`, or `$GH_OPTIVEM_ENV_FILE`). A missing file is a silent no-op; a read error is a non-fatal stderr note. A real exported environment variable always wins — the file only fills variables that are currently unset.
2. **Resolve the approval policy** — the root `PersistentPreRunE` resolves `--auto` / `--confirm` (flag > env > default) into a policy snapshot stashed on the command context, and emits the `Auto: …` banner to stderr when auto is on.
3. **Dispatch** — Cobra routes to the subcommand. `--version` prints the version and exits; note that `-v` is deliberately *not* a version shorthand, so `init -v` still means `--verbose`.

## Init pipeline

`runInit()` drives the scaffolding process:

1. **Load `gh-optivem.yaml`** — reads the project config from `--config` / `$GH_OPTIVEM_CONFIG` / `./gh-optivem.yaml`. On a TTY a missing file drops into the same interactive prompt as `gh optivem config init`.
2. **Merge and validate** — YAML values fill any unset flags, then `ParseAndValidate` validates the combined input and builds a `Config`.
3. **Initialize logging** — honours `--verbose` / `--quiet` / `--log-file` (a plain-text log mirror is always written).
4. **Update check** — queries the latest GitHub release and prints a notice to stderr if a newer version exists. Notify-only, non-blocking; the user upgrades explicitly via `gh extension upgrade optivem`.
5. **Initialize clients** — creates a `GitHub` shell wrapper and a `SonarCloud` client from the config.
6. **Print banner** — logs owner, repo, architecture, languages, and mode settings.
7. **Build steps** — assembles the ordered step list, grouped into phases (see below). The verify steps are appended by `buildVerifySteps` according to `--verify-level`.
8. **Execute steps** — runs each step in order, timing it and printing a phase header when the phase changes. A step that panics is recovered, logged, and marked failed. After a failure, remaining steps are skipped — except steps marked `alwaysRun` (Commit and push), so a partial scaffold still lands on the remote where it can be inspected. A step marked `failHard` additionally skips every subsequent step including the always-run ones.
9. **Print summary** — reports success/failure, total duration, and links (repo, actions, docs, backend/frontend repos if multirepo).
10. **Bug report** — off by default. Pass `--report-bug` to auto-create a GitHub issue in `optivem/gh-optivem` with scaffolding details on failure. Filing yourself is usually clearer and keeps scaffold config private unless you decide to share it.
11. **Cleanup** — on success, deletes the local scaffold dir (skip with `--keep-local`) so the user is left with just the remote repo(s) + SonarCloud project(s). On failure the dir is always kept so the broken scaffold can be inspected. The GitHub repos + SonarCloud projects are never deleted by the CLI itself; use [scripts/cleanup-orphans.sh](../scripts/cleanup-orphans.sh) or `gh optivem cleanup` for that.

## Setup steps

Steps are grouped into phases; `executeSteps` prints a header each time the phase changes.

| Phase | Step | Description |
|---|---|---|
| Prepare | Clone shop template | Clones `optivem/shop` at the pinned `meta-v*` release (or `--shop-ref`) |
| Setup repository | Create repositories | Creates the GitHub repo(s) via `gh` |
| Setup repository | Ensure project board | Auto-creates the Project board when `--project-url` is omitted; ensures statuses on a supplied one. Skipped entirely with `--no-project` |
| Setup repository | Setup environments | Configures GitHub deployment environments |
| Setup repository | Seed subtype labels | Creates the `subtype:*` ticket labels the ATDD pipeline reads |
| Setup repository | Setup variables and secrets | Sets repo secrets and variables from the local environment |
| Apply template | Clone repos | Clones the created repo(s) locally |
| Apply template | Apply template | Copies the project template files |
| Apply template | Replace repository references | Updates repo URLs/names in template files |
| Apply template | Replace namespaces | Substitutes namespace placeholders |
| Apply template | Replace system name | Substitutes system-name placeholders |
| Apply template | Update README | Generates the scaffolded project's README |
| Apply template | Write gh-optivem.yaml | Writes the per-project config the ATDD pipeline reads at runtime |
| Apply template | Write LICENSE | Writes the `LICENSE` file for the `--license` key |
| Apply template | Create SonarCloud projects | Registers projects in SonarCloud |
| Apply template | Verify push path filters | Fails hard if a scaffolded commit-stage workflow's `on: push: paths:` filter matches nothing in the repo — the filter would silently never fire |
| Push scaffold | Commit and push | Commits all changes and pushes to remote. Always runs, even after an earlier failure, so a partial scaffold is inspectable on the remote |
| Lint scaffold | Verify scaffolded workflows | Runs `actionlint` over the scaffolded workflows. Runs *after* push (so broken output is visible remotely) and fails hard, skipping the verify phases. Runs at every `--verify-level`, including `none` |
| Finalize | Print project registration | Prints the registration info for the new project |

## Verification steps

After the scaffold is pushed and linted, verification steps run based on `--verify-level`, in this fixed order:

| # | Step | Description |
|---|------|-------------|
| 1 | Compile system + Compile tests | Compiles system/backend/frontend and tests locally to catch broken imports and type errors |
| 2 | Local system lifecycle | Build system → setup tests → start system → run tests (latest, then legacy) → stop system → clean system. Mirrors the CLI verb order. Skipped entirely when `--no-local-tests` is set; the legacy run is dropped by `--no-legacy` |
| 3 | Verify local SonarCloud scan | Per-component `run-sonar.sh` against the SonarCloud project. Skipped when `--no-local-sonar` is set |
| 4 | Verify commit stage | Watches the commit stage CI workflow |
| 5 | Verify acceptance stage | Triggers and watches acceptance stage **latest + legacy in parallel** (legacy dropped when `--no-legacy`). Captures the RC version |
| 6 | Verify QA stage | Triggers QA stage, then QA signoff |
| 7 | Verify production stage | Triggers and watches the production stage |
| 8 | Verify cleanup workflow | Dry-run of `cleanup.yml`, which otherwise only fires on schedule — so a syntax error or stale action reference surfaces now rather than the next night |

`--verify-level` picks the cutoff; every step at or below that rank runs:

| Level | Steps that run |
|-------|----------------|
| `none` | Nothing — skip all verification |
| `local` | 1 + 2 + 3 |
| `commit` | 1 + 2 + 3 + 4 |
| `acceptance` | 1 + 2 + 3 + 4 + 5 |
| `qa` | 1 + 2 + 3 + 4 + 5 + 6 |
| `release` | 1 + 2 + 3 + 4 + 5 + 6 + 7 + 8 (default) |

Steps 1–3 run at every level above `none`. `--no-local-tests` drops step 2 and `--no-local-sonar` drops step 3, regardless of level. `--no-legacy` applies to the legacy test run inside step 2 and to the legacy leg of step 5.
