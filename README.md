[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension that scaffolds a full delivery pipeline and then drives tickets through it with AI agents.

## Install

Check whether you already have it:

```bash
gh optivem --version
```

If not installed:

```bash
gh extension install optivem/gh-optivem
```

If already installed, upgrade to the latest version:

```bash
gh extension upgrade optivem
```

To uninstall:

```bash
gh extension remove optivem
```

## Generate your project

Run `gh optivem init` — it will create your GitHub repository, apply the project template, set up CI/CD, and verify the pipeline.

- **Recommended: new project** — run `gh optivem init` to scaffold a fresh project. This is a huge time saver.
- **Alternative: existing project** — you still need to run `gh optivem init`, then manually transfer the relevant parts into your project.

Once complete, verify that your pipeline is green by checking the status badges in your new repository's README. All badges should be green.

## Verify your toolchain

From your project's repo root, run one sample test per suite against a locally started system:

```bash
gh optivem system start
gh optivem system-test run --sample
gh optivem system stop
```

This confirms that your toolchain, Docker, and system startup are all working. It should work out of the box.

## Everything else

See the [full CLI reference](docs/cli-reference.md) — credentials, every `init` flag, `gh optivem implement` (the day-to-day verb that walks a ticket through the ATDD pipeline), the system and test runners, cross-repo ops, and cleanup.
