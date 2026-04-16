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
7. **Bug report** — on failure, automatically creates a GitHub issue in `optivem/gh-optivem` with scaffolding details (unless `--no-bug-report` is set).
8. **Cleanup** — in test mode, deletes created resources; skipped on failure so the repo can be inspected.

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
| 13 | Enable GitHub Pages | Enables Pages on the repo |
| 14 | Verify compilation | Ensures the scaffolded project compiles |

## Verification steps

After setup, optional verification steps run based on `--verify-level`:

| Level | What runs |
|-------|-----------|
| `none` | Nothing — skip all verification |
| `local` | Local smoke tests only (no CI) |
| `commit` | Verify commit stage CI workflow passes |
| `acceptance` | Commit stage + acceptance stage CI + acceptance stage legacy (unless `--exclude-legacy`) + local system tests |
| `release` | All of the above + QA stage + QA signoff + production stage |

After verification, a final step prints project registration info.
