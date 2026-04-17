[![Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/commit-stage.yml)
[![Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/acceptance-stage.yml)
[![Release](https://github.com/optivem/gh-optivem/actions/workflows/release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/release-stage.yml)
[![Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/post-release-stage.yml)
[![Verify Release Chain](https://github.com/optivem/gh-optivem/actions/workflows/verify-release-chain.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/verify-release-chain.yml)

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
    --arch monolith --repo-strategy monorepo --lang java
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
gh optivem init ... --verify-level local          # local smoke + E2E tests only (no CI)
gh optivem init ... --verify-level commit        # only verify commit stage CI workflow
gh optivem init ... --verify-level acceptance    # commit + acceptance CI + full local system tests
gh optivem init ... --verify-level release       # full pipeline (default)
gh optivem init ... --exclude-legacy             # skip acceptance-stage-legacy
```

### Test mode

Scaffolds the project then cleans up automatically:

```bash
gh optivem init ... --test --cleanup --random-suffix
```

## How it works

See [docs/how-it-works.md](docs/how-it-works.md) for a detailed walkthrough of the `main.go` logic, setup steps, and verification levels.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and release instructions.
