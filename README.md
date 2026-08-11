[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension that scaffolds a full delivery pipeline and then drives tickets through it with AI agents.

## Install

```bash
gh extension install optivem/gh-optivem
```

Check that it worked:

```bash
gh optivem --version
```

## Generate your project

`gh optivem init` creates your GitHub repository, applies the project template, sets up the pipeline and waits for the pipeline to wait.

Run it with your project's values:

```bash
gh optivem init --owner <username|organization> --repo <repo_name> --system-name "<system_name>" --arch multitier --repo-strategy <monorepo|multirepo> --backend-lang <java|dotnet|typescript> --frontend-lang typescript --test-lang <java|dotnet|typescript>
```

For example:

```bash
gh optivem init --owner valentinajemuovic --repo book-shop --system-name "Book Shop" --arch multitier --repo-strategy monorepo --backend-lang dotnet --frontend-lang typescript --test-lang java
```

## Verify your setup

Once `init` completes, check the status badges in your new repository's README. All badges should be green.

Then, from your project's repo root, run one sample test per suite against a locally started system:

```bash
gh optivem system start
gh optivem system-test run --sample
gh optivem system stop
```

This confirms that your toolchain, Docker, and system startup are all working. It should work out of the box.

## Everything else

See the [full CLI reference](docs/cli-reference.md) — credentials, every `init` flag, `gh optivem implement` (the day-to-day verb that walks a ticket through the ATDD pipeline), the system and test runners, cross-repo ops, and cleanup.

## Upgrade

```bash
gh extension upgrade optivem
```

## Uninstall

```bash
gh extension remove optivem
```
