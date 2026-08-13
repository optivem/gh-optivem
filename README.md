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

## Claude Code setup

Wire Claude Code up for the ATDD workflow:

```bash
gh optivem claude setup
```

Re-run it after each `gh extension upgrade optivem`. `gh optivem claude check` reports drift without writing anything.

## Generate your project

`gh optivem init` creates your GitHub repository, applies the project template, sets up the pipeline and waits for it to pass.

Run it with your project's values. Which language flags you pass depends on `--arch`.

**Multitier** — separate `backend/` and `frontend/` trees, each with its own language:

```bash
gh optivem init --owner <owner> --repo <repo> --system-name "<system-name>" --repo-strategy <repo-strategy> --arch multitier --backend-lang <backend-lang> --frontend-lang typescript --test-lang <test-lang>
```

**Monolith** — a single `system/` tree in one language, so there is no separate frontend flag:

```bash
gh optivem init --owner <owner> --repo <repo> --system-name "<system-name>" --repo-strategy <repo-strategy> --arch monolith --monolith-lang <monolith-lang> --test-lang <test-lang>
```

Either way, `--test-lang` is independent of the system language(s), and `--repo-strategy` works with both architectures.

**Flags:**

- `--owner` — your GitHub username or organization name, whichever owns the scaffolded repo(s).
- `--repo` — repo name (or monorepo root name for multi-repo layouts).
- `--system-name` — human-readable system name (e.g. `"Book Shop"`).
- `--repo-strategy` — `monorepo` | `multirepo`.
- `--arch` — `monolith` | `multitier`.
- `--monolith-lang` — system language, only when `--arch monolith`: `java` | `dotnet` | `typescript`.
- `--backend-lang` — backend language, only when `--arch multitier`: `java` | `dotnet` | `typescript`.
- `--frontend-lang` — frontend language, only when `--arch multitier`: currently only `typescript`.
- `--test-lang` — system-test language, independent of the system language(s): `java` | `dotnet` | `typescript`.

For example:

```bash
gh optivem init --owner valentinajemuovic --repo book-shop --system-name "Book Shop" --repo-strategy monorepo --arch multitier --backend-lang dotnet --frontend-lang typescript --test-lang java
```

**Expect this to run for a long time** — `init` watches the generated pipelines on GitHub Actions until they go green, which takes tens of minutes, printing a heartbeat every 5 minutes. Leave it running until it reports success; interrupting it stops the watching, not the pipeline.

## Clone project repository

Clone your new repository to work on it locally:

```bash
gh repo clone valentinajemuovic/book-shop
cd book-shop
```

*With `--repo-strategy multirepo`, `init` creates several repositories — clone them all side by side in the same parent directory, so the cross-repo commands can find them.*

## Verify your project

Once `init` completes, check the status badges in your new repository's README. All badges should be green.

Then, from your project's repo root, run one sample test per suite against a locally started system. `system-test setup` runs once per clone, and `system-test run` will not do it for you:

```bash
gh optivem system-test setup
gh optivem system start
gh optivem system-test run --sample
gh optivem system stop
```

# ATDD AI Implementation

Setup is a one-off. From here on, `gh optivem implement` is the day-to-day verb: it takes one GitHub issue — a User Story with Acceptance Criteria — and walks it through the ATDD pipeline, dispatching an AI agent at each step and running the real build, system, and test commands in between.

## Implement a ticket

First, create a ticket in your repository. It needs a description and Acceptance Criteria written as Gherkin scenarios — copy [optivem/shop#72](https://github.com/optivem/shop/issues/72) as a worked example of the shape the agents expect.

Then **add it to the GitHub Project board `init` created**, or `implement` fails with `issue #N not found on project`. That board's URL is in your repo's `gh-optivem.yaml` under the top-level `project:` key, shaped `https://github.com/users/<login>/projects/<number>`, or `/orgs/<org>/projects/<number>` for an organization.

Add the issue to it:

```bash
gh project item-add <number> --owner <login|org> --url https://github.com/<owner>/<repo>/issues/<issue_number>
```

If that comes back *missing required scopes*:

```bash
gh auth refresh -s project
```

Then, from your project's repo root — `56` is an example, use your actual issue number:

```bash
gh optivem implement 56
```

Every confirmation prompts by default. For an unattended run, opt into `--auto` and pick how much human approval to keep:

```bash
gh optivem --auto implement 56 --headless                         # truly autonomous: prompt only on human-tier sites (STOPs, fix-agents, release)
gh optivem --auto --confirm=prod-commit implement 56 --headless   # narrower: still confirm every commit
gh optivem --auto --confirm=prod-agent implement 56 --headless    # narrower still: also confirm every production-agent dispatch
gh optivem --auto --confirm=command implement 56 --headless       # narrowest: confirm everything except cheap build/test commands
```

`--confirm=<tier>` sets the auto-approve floor: that tier and everything above it still prompts, everything below auto-yeses. Tiers, low to high stakes:

| Tier | Covers |
|---|---|
| `command` | execute-command BPMN nodes (compile / build / start / test run). Cheap, no AI cost, no global state mutation. |
| `prod-agent` | execute-agent for production code (`implement-*`, `update-*`, `refactor-system`). AI cost; produces reviewable diffs. |
| `test-agent` | execute-agent for tests (`write-*-tests`, `refactor-tests`). Tests-as-contract — ranked above `prod-agent` because broken tests mask regressions. |
| `prod-commit` | commit node after a production-agent phase. Persistent git write. |
| `test-commit` | commit node after a test-agent phase. Persistent git write of the test contract. |
| `human` | fix-`*` agents, `refine-acceptance-criteria`, BPMN STOP nodes, release. Top tier — always prompts, cannot be auto-yes'd at any reachable floor. |

Default when `--auto` is set and `--confirm` is omitted: `human`. See [Auto-approve](docs/cli-reference.md#auto-approve) for env-var overrides and more detail.

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
