[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

### GitHub CLI

[GitHub CLI](https://cli.github.com/) is required to install this extension.

Check your version:

```bash
gh --version
```

If the command isn't found or the version is too old, install or upgrade:

- Install: `winget install GitHub.cli` (Windows), `brew install gh` (macOS), or `sudo apt install gh` / your distro's package manager (Linux)
- Upgrade: `winget upgrade GitHub.cli` (Windows), `brew upgrade gh` (macOS), or `sudo apt upgrade gh` / your distro's package manager (Linux)

### GitHub CLI authentication

You must be logged in to GitHub via the CLI before installing this extension.

Verify you're logged in:

```bash
gh auth status
```

If the command reports you're not logged in, log in:

```bash
gh auth login
```

### actionlint

[`actionlint`](https://github.com/rhysd/actionlint) — used by the `Verify scaffolded workflows` step.

Check your version:

```bash
actionlint -version
```

If the command isn't found or the version is too old, install or upgrade to the latest v1 release:

```bash
go install github.com/rhysd/actionlint/cmd/actionlint@v1
```

## Installation

```bash
gh extension install optivem/gh-optivem
```

Verify the install:

```bash
gh optivem --version
```

Upgrade:

```bash
gh extension upgrade optivem
```

Uninstall:

```bash
gh extension remove optivem
```

## Environment Variables

Create these environment variables on your local machine. After setting them, restart your IDE / terminal for the changes to take effect.

- `DOCKERHUB_USERNAME` — your Docker Hub username.
- `DOCKERHUB_TOKEN` — Docker Hub Personal Access Token (read-only scope is enough). Create at https://app.docker.com/settings/personal-access-tokens.
- `SONAR_TOKEN` — SonarCloud token. Create at https://sonarcloud.io/account/security.
- `GHCR_TOKEN` — GitHub PAT (classic) with `write:packages` + `read:packages`. Create at https://github.com/settings/tokens.
- `WORKFLOW_TOKEN` — GitHub PAT (classic) with `repo` + `workflow` scopes. Create at https://github.com/settings/tokens.
- `REPO_TOKEN` — GitHub PAT with `repo` scope (classic) or `Contents: Read` on each component repo (fine-grained). Create at https://github.com/settings/tokens or https://github.com/settings/personal-access-tokens.

To confirm what your shell is actually exporting (token values masked):

```bash
gh optivem environment show
```

To live-check each token is also accepted by its provider before scaffolding:

```bash
gh optivem environment verify
```

## Scaffolding

```bash
gh optivem init
```

Project-stable values — prompted on first run and written to `gh-optivem.yaml` (or passed as flags for non-interactive runs):

- `--owner` — GitHub owner (user or org) for the scaffolded repo(s).
- `--repo` — repo name (or monorepo root name for multi-repo layouts).
- `--system-name` — human-readable system name (e.g. `"Page Turner"`).
- `--arch` — system architecture: `monolith` or `multitier`.
- `--repo-strategy` — `monorepo` or `multirepo`.
- Implementation language — which flag applies depends on `--arch`:
  - `--monolith-lang` — system language when `--arch monolith`: `java`, `dotnet`, or `typescript`.
  - `--backend-lang` — backend language when `--arch multitier`: `java`, `dotnet`, or `typescript`.
  - `--frontend-lang` — frontend language when `--arch multitier` (currently only `typescript`).
- `--test-lang` — system-test language: `java`, `dotnet`, or `typescript`. Independent of the system language(s).
- `--project-url` — URL of the GitHub Project board to attach.

<!--
TODO: document the standalone `gh optivem config init` retrofit flow
(writing gh-optivem.yaml into a hand-rolled, non-scaffolded repo, with
the full set of --system-path / --backend-path / ... tier-path overrides)
once the UX is validated.
-->

## Usage

`gh optivem` provides runner subcommands to build the system, run the system, and run the tests.

### System

#### Compile system

Source-level compile of the system tier (`dotnet build` / `./gradlew compileJava` / `npx tsc --noEmit`), dispatched per-tier by the `lang:` field in `gh-optivem.yaml`.

```bash
gh optivem system compile                 # system tier only
gh optivem compile                        # shortcut: system + test tiers (halts on first failure)
```

`compile` is the source-level build — distinct from `system build` (`docker compose build` / container image build). The two must not be conflated.

#### Build system

`docker compose build` for every entry in `systems.yaml`.

```bash
gh optivem system build
gh optivem system build --rebuild         # force full rebuild (no layer cache reuse)
```

#### Start system

`docker compose up` + wait for health.

```bash
gh optivem system start
gh optivem system start --restart         # force tear-down + restart
gh optivem system start --log-lines 200   # lines of compose logs to dump on health-probe failure (default 50)
gh optivem system start --up-timeout 10m  # per-attempt timeout for `docker compose up -d` (default 5m)
```

#### Stop system

`docker compose down` + container cleanup.

```bash
gh optivem system stop
```

#### Clean system

`docker compose down -v --rmi local` — delete volumes + locally-built images. Analog of `dotnet clean` / `./gradlew clean`: deletes build outputs without touching the dependency cache (registry-pulled images are kept). Chain it explicitly for a fresh start: `gh optivem system clean && gh optivem test run`.

```bash
gh optivem system clean
```

### System tests

#### Setup tests

Run `setupCommands` from `tests.yaml` (`npm ci`, restore, compile test sources, ...).

```bash
gh optivem test setup
```

#### Compile tests

Source-level compile of the test tier only.

```bash
gh optivem test compile
```

#### Run tests

> [!WARNING]
> The system must already be running (`gh optivem system start`). `test run` health-probes every entry in `systems.yaml` first; if any aren't up, it errors out with "start it first with `gh optivem system start`" rather than silently starting them.

Run all tests:

```bash
gh optivem test run                       # run every suite against the already-running system
```

Run specific suites:

```bash
gh optivem test run --suite smoke         # run only the suite with this id
gh optivem test run --suite acceptance-api --suite acceptance-ui   # multiple suites, repeatable
gh optivem test run --suite acceptance-api,acceptance-ui           # ...or comma-separated
gh optivem test run --list                # print suite ids from tests.yaml and exit
```

Run specific tests:

```bash
gh optivem test run --test "MyTest"       # narrow execution to one test name (substituted into the suite's testFilter)
gh optivem test run --test T1 --test T2   # multiple names, repeatable
gh optivem test run --test T1,T2          # ...or comma-separated
gh optivem test run --sample              # use each suite's sampleTest field as the test name
```

Multi-test semantics depend on the suite's `testFilter` in `tests.yaml`. The runner combines multiple `--test` values per `testFilterJoin`: `"or"` (default) joins names with `|` and substitutes once — works for dotnet (`&DisplayName~T1|T2`) and playwright/jest (`--grep 'T1|T2'`); `"repeat"` substitutes the whole `testFilter` once per name and concatenates — required for gradle (`--tests T1 --tests T2`). Practical ceiling on Windows is ~600 typical test names per invocation (the OS caps each command line at 32K characters).

<!-- TODO: revisit implementation pipeline section — commented out for now
## Running the implementation pipeline

Once a scaffolded project carries a valid `gh-optivem.yaml` and the sibling repos are cloned next to it, the `implement` subcommand walks the configured process-flow state machine for one ticket:

```bash
gh optivem implement --issue 42                                # walk the pipeline for a specific issue
gh optivem implement --issue https://github.com/optivem/shop/issues/42
gh optivem implement                                           # pick the top Ready ticket and walk the pipeline
```

`implement` accepts the same per-invocation flags whether or not `--issue` was passed:

```bash
... --autonomous          # skip human-approval STOPs and dispatch agents headless via `claude -p`
... --manual-agents       # v1 fallback: pause at each user-task node and let the operator launch the agent manually
... --log-file run.log    # mirror stdout/stderr to this file
... --keep-runs 10        # max prompt-log run dirs to keep under .gh-optivem/runs/ (0 = never prune; default 10)
... --show-prompt         # dump each agent's full rendered prompt before dispatch (default: summary banner only)
... --workspace <path>    # override the default workspace root (parent directory of CWD; each clone dir must be named after the repo-name component of its slug)
```

Project-stable overrides (process flow, agent prompts, per-node text) live in `gh-optivem.yaml` — see [pipeline overrides](#pipeline-overrides).

To inspect the embedded process-flow diagram without running the pipeline:

```bash
gh optivem process show                            # print the canonical Mermaid markdown to stdout
gh optivem process show > docs/process-diagram.md  # regenerate the committed diagram
```
-->

