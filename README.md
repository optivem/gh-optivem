[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)

# gh-optivem

`gh-optivem` is a tool that helps you implement functionality with AI, using the ATDD process.

# Setup

## Install

Install the [GitHub CLI](https://cli.github.com/) first. Check with `gh --version`, then log in with `gh auth login` and confirm with `gh auth status`.

```bash
gh extension install optivem/gh-optivem
```

Check that it worked:

```bash
gh optivem --version
```


## Prerequisites

Run this first — it reports everything that is missing, both the tools below and the environment variables in the next section:

```bash
gh optivem environment verify
```

It reports every problem in one pass. If it flags a missing tool, install it from the list below; if it flags a missing credential, see [Environment variables](#environment-variables). Then re-run it until it passes.

- **Bash** — every command below is run from a Bash shell, and `gh optivem` shells out through it too. On Windows, use Git Bash (bundled with [Git for Windows](https://git-scm.com/download/win)); on macOS and Linux it is already there. Check with `bash --version`.
- **[Go](https://go.dev/dl/)** — needed to install actionlint below. Check with `go version`.
- **[actionlint](https://github.com/rhysd/actionlint)** — `init` runs it over the scaffolded workflows before anything is pushed. Install with `go install github.com/rhysd/actionlint/cmd/actionlint@v1`, then check with `actionlint --version`. That drops the binary in `~/go/bin`, which Go does not add to your `PATH` — add it yourself if the check comes back "not found".
- **[Docker](https://docs.docker.com/get-started/get-docker/)** — the local system runs on `docker compose`. Check with `docker --version`.
- **Your project's language toolchain** — whichever languages you scaffold with: [Java](https://adoptium.net/) (check with `java -version`), [.NET](https://dotnet.microsoft.com/download) (`dotnet --version`), or [Node.js](https://nodejs.org/) (`npm --version`). To have `verify` check them for you, name them: `gh optivem environment verify --lang java,dotnet,typescript`.


## Environment variables

Before you can create your project, set these environment variables on your machine, then restart your IDE / terminal.

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

Then re-run `gh optivem environment verify` — it live-checks each token against its provider, so it will now pass.

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

## Clone project repository

Clone your new repository to work on it locally:

```bash
gh repo clone valentinajemuovic/book-shop
cd book-shop
```

*With `--repo-strategy multirepo`, `init` creates several repositories — clone them all side by side in the same parent directory, so the cross-repo commands can find them.*

## Verify your project

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

Then wire Claude Code up for the ATDD workflow:

```bash
gh optivem claude setup
```

That copies the Optivem slash commands into `~/.claude/commands/` and merges the Optivem permissions and rules into your `~/.claude/` config — without them, unattended runs stall on approval prompts. Re-run it after each `gh extension upgrade optivem`; `gh optivem claude check` reports drift without writing anything.

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
