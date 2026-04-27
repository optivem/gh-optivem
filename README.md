[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh auth login`)

## Installation

```bash
gh extension install optivem/gh-optivem
```

## Uninstalling

```bash
gh extension remove optivem
```

## Version

```bash
gh optivem --version
```

## Upgrading

```bash
gh optivem upgrade
```

Equivalent to `gh extension upgrade optivem` — either form works.

## Usage

### Scaffold a monolith project

```bash
gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
    --arch monolith --repo-strategy monorepo --monolith-lang java
```

### Scaffold a multitier project

```bash
gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
    --arch multitier --repo-strategy multirepo \
    --backend-lang java --frontend-lang react
```

### Dry run

```bash
gh optivem init ... --dry-run
```

### Verification level

Control how deep pipeline verification goes after scaffolding:

```bash
gh optivem init ... --verify-level local          # local compilation + local tests only (no CI)
gh optivem init ... --verify-level commit        # + commit stage CI
gh optivem init ... --verify-level acceptance    # + acceptance stage CI (latest + legacy in parallel)
gh optivem init ... --verify-level qa            # + QA stage + QA signoff
gh optivem init ... --verify-level release       # + production stage (default)
gh optivem init ... --no-legacy                  # skip legacy in local tests and acceptance
gh optivem init ... --no-local-tests             # skip the local system-tests step
```

### Local cleanup

On a successful run the local scaffold dir is deleted — the end result is just the created GitHub repo(s) + SonarCloud project(s), which you can clone later. Pass `--keep-local` to keep the dir (e.g. for inspection). On failure the dir is always kept so the broken scaffold can be debugged.

### Unattended runs (CI)

Pass `--yes` (or `-y`) to skip all interactive confirmations — the existing-repo prompt and the `--report-bug` confirmation. This is the expected pattern for CI/automation:

```bash
gh optivem init ... --yes
```

### Deployment target

Only `--deploy docker` is currently supported (the default). `--deploy cloud-run` is in development and may be available in a future release.

### Running tests against a scaffolded project

`gh optivem` also provides runner subcommands for working with the system tests in a scaffolded project. Inspired by `dotnet test` and `./gradlew test`, `test system` builds and starts the system implicitly — you don't need to run `build` or `run` first.

```bash
gh optivem test system                 # build (incremental) + start (if needed) + run tests
gh optivem test system --no-build      # skip the explicit build step
gh optivem test system --no-start      # skip the start step (system must already be up)
gh optivem test system --restart       # force tear-down + restart before tests
gh optivem test system --suite smoke   # run only the suite with this id

gh optivem build system                # docker compose build for every entry in system.json
gh optivem build system --rebuild      # force full rebuild (no layer cache reuse)

gh optivem run system                  # docker compose up + wait for health
gh optivem run system --restart        # force tear-down + restart

gh optivem stop system                 # docker compose down + container cleanup
gh optivem clean system                # docker compose down -v --rmi local (delete volumes + locally-built images)
```

`clean system` is the analog of `dotnet clean` / `./gradlew clean` — it deletes build outputs (containers, named volumes, locally-built images) without touching the dependency cache (registry-pulled images are kept). Chain it explicitly for a fresh start: `gh optivem clean system && gh optivem test system`.

## Troubleshooting

### Auto-filed bug report (opt-in)

If you want the failure auto-filed to `optivem/gh-optivem` as an issue — including scaffold config — opt in with `--report-bug`:

```bash
gh optivem init ... --report-bug
```

Off by default. Filing a quick issue yourself is usually clearer and keeps the scaffold config private unless you decide to share it.

## How it works

See [docs/how-it-works.md](docs/how-it-works.md) for a detailed walkthrough of the `main.go` logic, setup steps, and verification levels.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and release instructions.
