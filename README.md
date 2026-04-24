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
gh extension upgrade optivem
```

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
gh optivem init ... --exclude-legacy             # skip legacy in local tests and acceptance
gh optivem init ... --skip-local-tests           # skip Run-SystemTests.ps1 step
```

### Local cleanup

On a successful run the local scaffold dir is deleted — the end result is just the created GitHub repo(s) + SonarCloud project(s), which you can clone later. Pass `--keep-local` to keep the dir (e.g. for inspection). On failure the dir is always kept so the broken scaffold can be debugged.

### Deployment target

Only `--deploy docker` is currently supported (the default). `--deploy cloud-run` is in development and may be available in a future release.

## Troubleshooting

### Partial scaffold on failure

When scaffolding fails mid-run, the partial working tree is pushed to a `debug/<timestamp>` branch in the already-created remote repo so the state can be inspected and diffed. The default `main` branch is left untouched. The debug branch URL is printed in the summary.

To skip this push (e.g. for private repos or when you don't want any partial state on the remote):

```bash
gh optivem init ... --no-commit-on-failure
```

### Auto-filed bug report (opt-in)

If you want the failure auto-filed to `optivem/gh-optivem` as an issue — including scaffold config and the debug-branch URL — opt in with `--report-bug`:

```bash
gh optivem init ... --report-bug
```

Off by default. Filing a quick issue yourself is usually clearer and keeps the scaffold config private unless you decide to share it.

## How it works

See [docs/how-it-works.md](docs/how-it-works.md) for a detailed walkthrough of the `main.go` logic, setup steps, and verification levels.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and release instructions.
