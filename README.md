[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)

# gh-optivem

`gh-optivem` is a command line tool that helps you implement software in a reliable way using ATDD & AI.

# Setup

## Prerequisites

- **[GitHub CLI](https://cli.github.com/)**

  ```bash
  winget install --id GitHub.cli -e --source winget   # Windows
  ```

  `winget` writes `PATH` into the registry, and the shell you ran it in keeps the copy it started with — so from a **fresh** shell:

  ```bash
  gh --version
  gh auth login -s project,workflow
  gh auth status
  ```

  Ask for those two scopes up front. A bare `gh auth login` requests neither, and both are needed later: `project` for the board `init` creates and for the board tracker `implement` drives, `workflow` for pushing the scaffolded `.github/workflows/`. `gh optivem` checks for them before it creates anything, so a token without them stops at `Verifying environment...`.

- **[Claude Code](https://claude.com/claude-code), installed and signed in** — the agents run as `claude` subprocesses, so you need an active Claude subscription. The free Claude.ai plan does not include Claude Code.

  On Windows, install it from PowerShell. `claude.ai/install.sh` is the macOS / Linux / WSL installer and is **not** the one to use here:

  ```powershell
  irm https://claude.ai/install.ps1 | iex
  ```

  It puts `claude.exe` in `%USERPROFILE%\.local\bin` and never edits your `PATH` — it prints a warning saying so and leaves it at that. Add that directory to `PATH` yourself if the check below comes back "not found".

  On macOS and Linux:

  ```bash
  curl -fsSL https://claude.ai/install.sh | bash
  ```

  Then, from a **fresh** shell — that installer does write `PATH`, and the shell you ran it in keeps the copy it started with:

  ```bash
  claude --version
  ```

  Sign in by starting it once. It opens a browser; type `/exit` when you are back:

  ```bash
  claude
  ```

## Install

Install `gh-optivem`:

```bash
gh extension install optivem/gh-optivem
gh optivem --version
```

## Local environment setup

Start here — it runs before any of the tools below are installed, and reports everything missing in one pass: the gh CLI, Claude Code, Bash, actionlint, Docker, and the environment variables in the next section.

```bash
gh optivem environment verify
```

If it flags a missing tool, install it from the list below; if it flags a missing credential, see [Environment variables](#environment-variables). Then re-run it until it passes.

Two things it does *not* check on its own: Go (it only matters for installing actionlint) and your language toolchains — name those with `--lang`, see below.

The install commands below are the Windows ones, via `winget`. On macOS and Linux, follow each bullet's download link. Either way `PATH` is written for *new* shells, so open a fresh one before the version check.

- **Bash** — every command in this README is run from a Bash shell, and `gh optivem` shells out through it too. On Windows, use Git Bash (bundled with [Git for Windows](https://git-scm.com/download/win)); on macOS and Linux it is already there.

  Install it from PowerShell — Git Bash is the shell every other command here runs *in*, so it cannot install itself:

  ```powershell
  winget install --id Git.Git -e --source winget
  ```

  Then, from Git Bash:

  ```bash
  bash --version
  ```

- **[Go](https://go.dev/dl/)** — needed to install actionlint below. The Go installer puts only the toolchain itself on your `PATH`, never `$(go env GOPATH)/bin` (usually `~/go/bin`) — which is where `go install` drops the binaries it builds. Add that directory to `PATH` yourself, or the actionlint check below comes back "not found".

  ```bash
  winget install --id GoLang.Go -e --source winget   # Windows
  go version
  ```

- **[actionlint](https://github.com/rhysd/actionlint)** — `init` runs it over the scaffolded workflows, right after pushing them and before any pipeline verification, so a broken scaffold is visible on the remote for troubleshooting.

  ```bash
  go install github.com/rhysd/actionlint/cmd/actionlint@v1
  actionlint --version
  ```

- **[Docker](https://docs.docker.com/get-started/get-docker/)** — the local system runs on `docker compose`.

  ```bash
  docker --version
  ```

- **Your project's language toolchain** — whichever languages you scaffold with: [Java](https://adoptium.net/), [.NET](https://dotnet.microsoft.com/download), or [Node.js](https://nodejs.org/). Install and check only the ones you need. The versions below are the ones the scaffolded pipeline builds with, so matching them keeps "works in CI" and "works locally" the same statement:

  ```bash
  winget install --id EclipseAdoptium.Temurin.21.JDK -e --source winget   # Java 21, Windows
  winget install --id Microsoft.DotNet.SDK.8 -e --source winget           # .NET 8, Windows
  winget install --id OpenJS.NodeJS.22 -e --source winget                 # Node.js 22, Windows
  ```

  ```bash
  java -version     # Java
  dotnet --version  # .NET
  npm --version     # Node.js
  ```

  To have `verify` check them for you, name them: `gh optivem environment verify --lang java,dotnet,typescript`.

## Environment variables

Before you can create your project, set these credentials as environment variables on your machine, then restart your IDE / terminal:

- `DOCKERHUB_USERNAME` — your [Docker Hub](https://hub.docker.com) username.
- `DOCKERHUB_TOKEN` — a [Docker Hub Personal Access Token](https://app.docker.com/settings/personal-access-tokens) (read-only scope is enough).
- `SONAR_TOKEN` — a [SonarCloud token](https://sonarcloud.io/account/security).
- `GHCR_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `write:packages` + `read:packages`.
- `WORKFLOW_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `repo` + `workflow` scopes.
- `REPO_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `repo` scope.

To confirm what `gh optivem` actually sees (token values masked):

```bash
gh optivem environment show
```

Then re-run `gh optivem environment verify` — it live-checks each token against its provider, so it will now pass.

The three GitHub PATs back pipelines that keep running on a schedule long after scaffolding, and a lapsed token makes them fail silently. Put a rotation reminder in your calendar; `verify` warns when a classic PAT expires within 7 days.

## Claude Code setup

Wire Claude Code up for the ATDD workflow:

```bash
gh optivem claude setup
```

That copies the Optivem slash commands into `~/.claude/commands/` and merges the Optivem permissions and rules into your `~/.claude/` config — without them, unattended runs stall on approval prompts. Re-run it after each `gh extension upgrade optivem`; `gh optivem claude check` reports drift without writing anything.

## Generate your project

`gh optivem init` creates your GitHub repository, applies the project template, sets up the pipeline and waits for it to pass.

Run it with your project's values. Which language flags you pass depends on `--arch`.

**Multitier** — separate `backend/` and `frontend/` trees, each with its own language:

```bash
gh optivem init --owner <username|organization> --repo <repo_name> --system-name "<system_name>" --repo-strategy <monorepo|multirepo> --arch multitier --backend-lang <java|dotnet|typescript> --frontend-lang typescript --test-lang <java|dotnet|typescript>
```

**Monolith** — a single `system/` tree in one language, so there is no separate frontend flag:

```bash
gh optivem init --owner <username|organization> --repo <repo_name> --system-name "<system_name>" --repo-strategy <monorepo|multirepo> --arch monolith --monolith-lang <java|dotnet|typescript> --test-lang <java|dotnet|typescript>
```

Either way, `--test-lang` is independent of the system language(s), and `--repo-strategy` works with both architectures.

For example:

```bash
gh optivem init --owner valentinajemuovic --repo book-shop --system-name "Book Shop" --repo-strategy monorepo --arch multitier --backend-lang dotnet --frontend-lang typescript --test-lang java
```

**Expect this to run for a long time.** Creating the repository and pushing the template is quick; after that, `init` sits and watches the generated pipelines on GitHub Actions until they go green, which takes tens of minutes. It prints a heartbeat every 5 minutes so you can see it is still alive. Leave it running until it reports success — interrupting it stops the watching, not the pipeline.

## Clone project repository

Clone your new repository to work on it locally:

```bash
gh repo clone valentinajemuovic/book-shop
cd book-shop
```

*With `--repo-strategy multirepo`, `init` creates several repositories — clone them all side by side in the same parent directory, so the cross-repo commands can find them.*

## Verify your project

Once `init` completes, check the status badges in your new repository's README. All badges should be green.

Then, from your project's repo root, run one sample test per suite against a locally started system. `system-test setup` installs the test-harness dependencies — it only needs to run once per clone, but `system-test run` will not do it for you:

```bash
gh optivem system-test setup
gh optivem system start
gh optivem system-test run --sample
gh optivem system stop
```

This confirms that your toolchain, Docker, and system startup are all working. It should work out of the box.

# ATDD AI Implementation

Setup is a one-off. From here on, `gh optivem implement` is the day-to-day verb: it takes one GitHub issue — a User Story with Acceptance Criteria — and walks it through the ATDD pipeline, dispatching an AI agent at each step and running the real build, system, and test commands in between.

## Implement a ticket

First, create a ticket in your repository. It needs a description and Acceptance Criteria written as Gherkin scenarios — copy [optivem/shop#72](https://github.com/optivem/shop/issues/72) as a worked example of the shape the agents expect.

Then **add it to the GitHub Project board `init` created** — `implement` looks the issue up among the board's items, and fails with `issue #N not found on project` if it isn't there. `init` wrote that board's URL into your repo's `gh-optivem.yaml`, under the top-level `project:` key — read it from there rather than matching a title in `gh project list`, because two boards can share a title and only one of them is the one `init` used. The URL is shaped `https://github.com/users/<login>/projects/<number>`, or `/orgs/<org>/projects/<number>` for an organization.

Add the issue to it with plain `gh` — there is no `gh optivem` wrapper for this:

```bash
gh project item-add <number> --owner <login|org> --url https://github.com/<owner>/<repo>/issues/<issue_number>
```

If that comes back *missing required scopes*, you logged in without the `project` scope the [Prerequisites](#prerequisites) asked for. Add it to the token you already have:

```bash
gh auth refresh -s project
```

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

Every `init` run writes a plain-text log to `$TEMP/gh-optivem-<timestamp>.log` — attach it to the issue. `implement` only writes one when you ask for it: `gh optivem implement 56 --log-file run.log`.

## Maintainer

Maintained by Valentina Jemuović at Optivem — [GitHub](https://github.com/valentinajemuovic) · [LinkedIn](https://www.linkedin.com/in/valentinajemuovic/).

## License

[MIT](LICENSE) © Optivem
