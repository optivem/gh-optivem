# Setup

One-time, before you can create your first project. See the [README](../README.md) for the actual day-to-day workflow — creating a project and running the ATDD loop on it.

- [Prerequisites](#prerequisites)
- [Install](#install)
- [Local environment setup](#local-environment-setup)
- [Environment variables](#environment-variables)
- [Claude Code setup](#claude-code-setup)

## Prerequisites

- **[GitHub CLI](https://cli.github.com/)**

  ```bash
  winget install --id GitHub.cli -e --source winget   # Windows
  ```

  Then, from a **fresh** shell:

  ```bash
  gh --version
  gh auth status
  ```

  `gh auth status` tells you which of the next two you need. It prints a `Token scopes:` line — `project` and `workflow` both have to appear in it.

  Not logged in yet:

  ```bash
  gh auth login -s project,workflow
  ```

  Logged in, but missing scopes:

  ```bash
  gh auth refresh -s project,workflow
  ```

- **[Claude Code](https://claude.com/claude-code), installed and signed in** — you need an active Claude subscription; the free Claude.ai plan does not include Claude Code.

  On Windows, install it from PowerShell — `claude.ai/install.sh` is the macOS / Linux / WSL installer:

  ```powershell
  irm https://claude.ai/install.ps1 | iex
  ```

  It puts `claude.exe` in `%USERPROFILE%\.local\bin` without editing your `PATH` — add that directory yourself if the check below comes back "not found".

  On macOS and Linux:

  ```bash
  curl -fsSL https://claude.ai/install.sh | bash
  ```

  Then, from a **fresh** shell:

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

See [the full reference](cli-reference.md#install) for upgrade/uninstall and the `actionlint` PATH note.

## Local environment setup

Start here — it reports every missing tool and credential in one pass:

```bash
gh optivem environment verify
```

If it flags a missing tool, install it from the list below; if it flags a missing credential, see [Environment variables](#environment-variables). Then re-run it until it passes.

Two things it does *not* check on its own: Go, and your language toolchains — name those with `--lang`, see below.

The install commands below are the Windows ones, via `winget`. On macOS and Linux, follow each bullet's download link. Either way `PATH` is written for *new* shells, so open a fresh one before the version check.

- **Bash** — on Windows, use Git Bash (bundled with [Git for Windows](https://git-scm.com/download/win)); on macOS and Linux it is already there.

  Install it from PowerShell:

  ```powershell
  winget install --id Git.Git -e --source winget
  ```

  Then, from Git Bash:

  ```bash
  bash --version
  ```

- **[Go](https://go.dev/dl/)** — add `$(go env GOPATH)/bin` (usually `~/go/bin`) to your `PATH` yourself; the Go installer does not, and the actionlint check below comes back "not found" without it.

  ```bash
  winget install --id GoLang.Go -e --source winget   # Windows
  go version
  ```

- **[actionlint](https://github.com/rhysd/actionlint)**

  ```bash
  go install github.com/rhysd/actionlint/cmd/actionlint@v1
  actionlint --version
  ```

- **[Docker](https://docs.docker.com/get-started/get-docker/)**

  ```bash
  docker --version
  ```

- **Your project's language toolchain** — [Java](https://adoptium.net/), [.NET](https://dotnet.microsoft.com/download), or [Node.js](https://nodejs.org/). Install and check only the ones you scaffold with; these are the versions the pipeline builds with:

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

To confirm what `gh optivem` sees (values masked):

```bash
gh optivem environment show
```

Then re-run `gh optivem environment verify`.

Put a rotation reminder for the three GitHub PATs in your calendar — `verify` warns when a classic PAT expires within 7 days.

See [the full reference](cli-reference.md#environment-variables) for the portable `.env` file, which lets you edit credentials without restarting your shell.

## Claude Code setup

Wire Claude Code up for the ATDD workflow:

```bash
gh optivem claude setup
```

Re-run it after each `gh extension upgrade optivem`. `gh optivem claude check` reports drift without writing anything. See [the full reference](cli-reference.md#claude-code-setup) for what `setup` actually does.

Once this passes, head back to the [README](../README.md) to create your project.
