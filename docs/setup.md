# Setup

One-time, before you can create your first project. See the [README](../README.md) for the actual day-to-day workflow — creating a project and running the ATDD loop on it.

- [Prerequisites](#prerequisites)
- [Install](#install)
- [Local environment setup](#local-environment-setup)
- [Environment variables](#environment-variables)
- [Claude Code setup](#claude-code-setup)

## Prerequisites

- **[GitHub CLI](https://cli.github.com/)** — the one thing needed before the extension exists.

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

## Install

Install `gh-optivem`:

```bash
gh extension install optivem/gh-optivem
gh optivem --version
```

See [the full reference](cli-reference.md#install) for upgrade/uninstall and the `actionlint` PATH note.

## Local environment setup

One command installs everything installable and fixes your `PATH`:

```bash
gh optivem environment setup --lang java,dotnet,typescript
```

Name only the languages you scaffold with — each one installs a multi-gigabyte toolchain. Omit `--lang` to install just the language-agnostic tools.

It installs, when missing: Git Bash, Go, actionlint, [Claude Code](https://claude.com/claude-code) and the `--lang` toolchains (Java 21, .NET 8, Node.js 22). Where an upstream installer declines to edit `PATH` — Claude Code's `%USERPROFILE%\.local\bin`, Go's `$(go env GOPATH)/bin` — setup adds the directory to your user `PATH` itself and prints exactly what it changed.

Automated on Windows, via `winget`. On macOS and Linux it installs nothing and prints each missing tool's install pointer. Open a fresh shell afterwards so every program sees the new `PATH`.

It then runs `verify` and lists what is left, which is only what a human can do:

- **Docker** — install and start [Docker Desktop](https://docs.docker.com/get-started/get-docker/). Setup never installs it: it needs a licence acceptance, WSL2 and a reboot.
- **Claude Code sign-in** — you need an active Claude subscription; the free Claude.ai plan does not include Claude Code. Start it once, which opens a browser, and type `/exit` when you are back:

  ```bash
  claude
  ```

  Setup cannot tell whether you have already signed in — `claude` has no non-interactive sign-in check — so it lists this every time.
- **GitHub sign-in**, if the Prerequisites step above is not done.
- **Credentials** — see [Environment variables](#environment-variables).

Re-check at any time, without changing anything:

```bash
gh optivem environment verify
gh optivem environment verify --lang java,dotnet,typescript
```

`setup` writes to your machine; `verify` only reads.

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
