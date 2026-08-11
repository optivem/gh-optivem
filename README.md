[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)

# gh-optivem

A GitHub CLI extension that scaffolds a ready-to-go system test project with a GitHub Actions pipeline — a complete, working project from the first run.

You then use it to implement your GitHub tickets. Write a User Story with Acceptance Criteria as an issue, and `gh optivem` takes it from start to end: RED (acceptance tests, DSL and drivers) through GREEN (backend and frontend implementation).

# Setup

## Prerequisites

- **[GitHub CLI](https://cli.github.com/), authenticated** — `gh optivem` shells out to it to create repos, set secrets, and dispatch workflows. Check with `gh auth status`; log in with `gh auth login`.
- **[actionlint](https://github.com/rhysd/actionlint)** on your `PATH` — `init` runs it over the scaffolded workflows before anything is pushed. Install with `go install github.com/rhysd/actionlint/cmd/actionlint@v1` (needs a [Go toolchain](https://go.dev/dl/)).
- **[Docker](https://docs.docker.com/get-started/get-docker/)** — the local system runs on `docker compose`.
- **Your project's language toolchain** — `java`, `dotnet`, or `npm`, whichever you scaffold with.

## Install

```bash
gh extension install optivem/gh-optivem
```

Check that it worked:

```bash
gh optivem --version
```

## Environment variables

`gh optivem init` reads these credentials from your local environment and sets them as secrets and variables on the repos it creates, so the generated pipelines can pull base images, publish to GHCR, and scan with SonarCloud.

Set them on your machine the usual way, then restart your IDE / terminal — the environment snapshot is taken when the process launches, so a shell opened before you set them won't see them.

- `DOCKERHUB_USERNAME` — your Docker Hub username.
- `DOCKERHUB_TOKEN` — a [Docker Hub Personal Access Token](https://app.docker.com/settings/personal-access-tokens) (read-only scope is enough).
- `SONAR_TOKEN` — a [SonarCloud token](https://sonarcloud.io/account/security).
- `GHCR_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `write:packages` + `read:packages`.
- `WORKFLOW_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `repo` + `workflow` scopes.
- `REPO_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `repo` scope.

To confirm what your shell is actually exporting (token values masked):

```bash
gh optivem environment show
```

To live-check each token is also accepted by its provider before scaffolding — and that the prerequisites above are in place:

```bash
gh optivem environment verify --lang dotnet,typescript --deploy docker
```

`--lang` (any of `java`, `dotnet`, `typescript`) adds the compiler checks for the languages you scaffold with; `--deploy docker` adds the Docker CLI check. Without them, `verify` covers the gh CLI, actionlint, and the five tokens.

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

# ATDD AI Implementation

Setup is a one-off. From here on, `gh optivem implement` is the day-to-day verb: it takes one GitHub issue — a User Story with Acceptance Criteria — and walks it through the ATDD pipeline, dispatching an AI agent at each step and running the real build, system, and test commands in between.

## Prerequisites

- **[Claude Code](https://claude.com/claude-code), installed and signed in** — the agents run as `claude` subprocesses, so you need an active Claude subscription. Nothing in Setup depends on it; only `implement` does.

## Implement a ticket

First, create a ticket in your repository. It needs a description and Acceptance Criteria written as Gherkin scenarios — copy [optivem/shop#72](https://github.com/optivem/shop/issues/72) as a worked example of the shape the agents expect.

Then, from your project's repo root, run it against that ticket — `56` here is just an example, replace it with your actual issue number:

```bash
gh optivem implement 56
```

That walks the issue from start to end: RED (acceptance tests, DSL and drivers) through GREEN (backend and frontend implementation), committing as it goes.

Every confirmation prompts by default. For an unattended run:

```bash
gh optivem --auto implement 56 --headless
```

# Reference

## Everything else

See the [full CLI reference](docs/cli-reference.md) — credentials, every `init` flag, the `implement` flags and team-handoff slices, the system and test runners, cross-repo ops, and cleanup.

## Upgrade

```bash
gh extension upgrade optivem
```

## Uninstall

```bash
gh extension remove optivem
```

## Support

Something went wrong? [Open an issue](https://github.com/optivem/gh-optivem/issues).

If `gh optivem init` itself fails, add `--report-bug` and it files the issue for you, with your scaffold config and log file attached:

```bash
gh optivem init --report-bug ...
```

Either way, a run always writes a plain-text log to `$TEMP/gh-optivem-<timestamp>.log` — attach it to the issue.

## Maintainer

Maintained by Valentina Jemuović at Optivem — [GitHub](https://github.com/valentinajemuovic) · [LinkedIn](https://www.linkedin.com/in/valentinajemuovic/).

## License

[MIT](LICENSE) © Optivem
