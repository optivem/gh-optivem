# How it works

The entry point (`main.go`) handles CLI argument parsing and orchestrates the `init` subcommand.

## Startup

1. **Version check** — `--version` / `-v` prints the version and exits immediately.
2. **Subcommand dispatch** — requires a subcommand (`init` is the only one); unknown commands print usage and exit.
3. **Update check** — queries the latest GitHub release; if a newer version exists, prints a warning to stderr (non-blocking, fails silently if offline).

## Init pipeline

`runInit()` drives the scaffolding process:

1. **Parse & validate config** — reads all CLI flags, validates required fields, and builds a `Config` struct.
2. **Initialize clients** — creates a `GitHub` shell wrapper and a `SonarCloud` client from the config.
3. **Print banner** — logs owner, repo, architecture, languages, and mode settings.
4. **Build steps** — assembles an ordered list of setup steps (see below).
5. **Execute steps** — runs each step sequentially, timing each one. If a step panics, the error is caught via `recover()`, logged, and execution stops immediately.
6. **Print summary** — reports success/failure, total duration, and links (repo, actions, docs, backend/frontend repos if multirepo).
7. **Bug report** — off by default. Pass `--bug-report` to auto-create a GitHub issue in `optivem/gh-optivem` with scaffolding details and the debug-branch URL on failure. Filing yourself is usually clearer and keeps scaffold config private unless you decide to share it.
8. **Cleanup** — on success, deletes the local scaffold dir (skip with `--keep-local`) so the user is left with just the remote repo(s) + SonarCloud project(s). On failure the dir is always kept so the broken scaffold can be inspected. The GitHub repos + SonarCloud projects are never deleted by the CLI itself; use [scripts/cleanup-orphans.sh](../scripts/cleanup-orphans.sh) for that.

## Setup steps

| # | Step | Description |
|---|------|-------------|
| 1 | Create repositories | Creates the GitHub repo(s) via `gh` |
| 2 | Setup environments | Configures GitHub deployment environments |
| 3 | Setup secrets and variables | Sets repo secrets and variables |
| 4 | Clone repos | Clones the created repo(s) locally |
| 5 | Apply template | Copies the project template files |
| 6 | Replace repository references | Updates repo URLs/names in template files |
| 7 | Replace namespaces | Substitutes namespace placeholders |
| 8 | Replace system name | Substitutes system-name placeholders |
| 9 | Update README | Generates the scaffolded project's README |
| 10 | Write project config | Writes `.optivem.yml` or similar config |
| 11 | Create SonarCloud projects | Registers projects in SonarCloud |
| 12 | Commit and push | Commits all changes and pushes to remote |
| 13 | Validate no leftover system names | Fails if old template name still appears in the pushed repo |

## Verification steps

After setup, verification steps run based on `--verify-level`, in this fixed order:

| # | Step | Description |
|---|------|-------------|
| 1 | Verify local compilation | Compiles system/backend/frontend/tests locally to catch broken imports, type errors |
| 2 | Verify local testing | Runs `Run-SystemTests.ps1` (latest + legacy). Skipped when `--skip-local-tests` is set |
| 3 | Verify commit stage | Watches the commit stage CI workflow |
| 4 | Verify acceptance stage | Triggers and watches acceptance stage **latest + legacy in parallel** (legacy dropped when `--exclude-legacy`). Captures the RC version. |
| 5 | Verify QA stage | Triggers QA stage, then QA signoff |
| 6 | Verify production stage | Triggers and watches the production stage |

`--verify-level` picks the cutoff; every step at or below that rank runs (except local tests when `--skip-local-tests` is set):

| Level | Steps that run |
|-------|----------------|
| `none` | Nothing — skip all verification |
| `local` | 1 + 2 |
| `commit` | 1 + 2 + 3 |
| `acceptance` | 1 + 2 + 3 + 4 |
| `qa` | 1 + 2 + 3 + 4 + 5 |
| `release` | 1 + 2 + 3 + 4 + 5 + 6 (default) |

`--exclude-legacy` applies to step 2 (local tests) and step 4 (acceptance stage). `--skip-local-tests` drops step 2 regardless of level.

After verification, a final step prints project registration info.
