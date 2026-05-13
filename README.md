[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

### GitHub CLI

[GitHub CLI](https://cli.github.com/) (`gh auth login`) is required to install this extension.

- Check your version: `gh --version`
- Install: `winget install GitHub.cli` (Windows), `brew install gh` (macOS), or `sudo apt install gh` / your distro's package manager (Linux)
- Upgrade: `winget upgrade GitHub.cli` (Windows), `brew upgrade gh` (macOS), or `sudo apt upgrade gh` / your distro's package manager (Linux)

### actionlint

[`actionlint`](https://github.com/rhysd/actionlint) — used by the `Verify scaffolded workflows` step.

- Check your version: `actionlint -version`
- Install or upgrade to the latest v1 release: `go install github.com/rhysd/actionlint/cmd/actionlint@v1`

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

After installing the extension, configure these once before running `gh optivem init` for the first time.

`gh optivem init` reads six variables from your local shell environment and writes them as Actions variables/secrets on the scaffolded repo (and on each component repo in multirepo). The scaffolded pipeline can't run without them, so the tool fails fast if any are missing.

- `DOCKERHUB_USERNAME` — your Docker Hub username. The scaffolded pipeline authenticates pulls of base images from Docker Hub so they don't hit the anonymous rate limit (shared CI runner IPs burn through it quickly).
- `DOCKERHUB_TOKEN` — a Docker Hub Personal Access Token paired with the username above. Same reason: authenticated pulls. Create at https://app.docker.com/settings/personal-access-tokens (read-only scope is enough since we only pull).
- `SONAR_TOKEN` — a SonarCloud token. Consumed by the local SonarCloud scan step (`--verify-level local`+) and by the commit stage's CI scan.
- `GHCR_TOKEN` — a GitHub Personal Access Token (classic) with `write:packages` + `read:packages`. The acceptance and prod stages tag images in GHCR. Create at https://github.com/settings/tokens.
- `WORKFLOW_TOKEN` — a GitHub PAT (classic) with `repo` + `workflow` scopes. The acceptance/QA/prod stages push release tags; the default `GITHUB_TOKEN` cannot push tags whose commits diff workflow files, which is why a separate PAT is needed. Create at https://github.com/settings/tokens.
- `REPO_TOKEN` — a GitHub PAT used by the system-level prod stage in multitier+multirepo scaffolds to read each component repo's `VERSION` file via the GitHub API (cross-repo Contents read). Currently required for all scaffolds even though only multitier+multirepo consumes it. Create at https://github.com/settings/tokens (classic PAT with `repo` scope) or https://github.com/settings/personal-access-tokens (fine-grained PAT with `Contents: Read` on every component repo of the scaffolded system).

```bash
export DOCKERHUB_USERNAME=...
export DOCKERHUB_TOKEN=...
export SONAR_TOKEN=...
export GHCR_TOKEN=...
export WORKFLOW_TOKEN=...
export REPO_TOKEN=...
```

To confirm what your shell is actually exporting (token values masked):

```bash
gh optivem environment show
```

To live-check each token is also accepted by its provider before running `init`:

```bash
gh optivem environment verify
```

Reads `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` / `SONAR_TOKEN` / `GHCR_TOKEN` / `WORKFLOW_TOKEN` / `REPO_TOKEN` from the environment, runs a live auth call against each provider in parallel, and exits non-zero with an aggregated list of every missing or rejected value. `DOCKERHUB_USERNAME` is an account name rather than a token — not all environment variables are tokens, which is why the command is `environment verify` not `token verify`. Read-only — no repos, secrets, or releases are mutated. Run this once up front before kicking off a CI matrix that fans out to every architecture × language combination.

## Configuration

Scaffolding is **two-phase**. First write `gh-optivem.yaml` — the file that carries every project-stable value (owner, repo, arch, langs, system name, license, deploy target, tier paths, project URL). Then run `gh optivem init` to create the GitHub repo(s) and apply the template.

For freshly scaffolded repos, `gh optivem init` writes the YAML for you — skip ahead to [Scaffolding](#scaffolding). The `gh optivem config init` flow below is for hand-rolled repos adopting `gh-optivem` after the fact; see [CONTRIBUTING.md](CONTRIBUTING.md#install-from-source) for full flag examples.

Tier paths default to the flat scaffold layout (`system` / `backend` / `frontend` / `system-test` / `external-systems/external-stub` / `external-systems/external-real-sim`) — the same layout `gh optivem init` itself produces. Override with `--system-path`, `--backend-path`, `--frontend-path`, `--system-test-path`, `--stubs-path`, `--simulators-path` only when writing the YAML for a non-flat existing repo.

Or run `gh optivem config init` interactively when the file is missing — `gh-optivem` prompts for owner/repo (auto-inferred from `git remote get-url origin` when the current directory already has a github.com origin configured), system-name, arch, repo-strategy, lang, and project-url; everything else is defaulted.

Review the generated `gh-optivem.yaml`, hand-edit if needed, then run `gh optivem config validate` to confirm. Once the sibling repos are cloned (multi-repo layouts) and the tier paths actually exist on disk, run `gh optivem config preflight` for the stronger "I'm about to run this for real" check — same schema validation plus an on-disk layout check that every declared repo and tier path resolves to a real directory. `preflight` is the same check `atdd implement-ticket` runs at startup.

## Scaffolding

```bash
gh optivem init
```

No flags needed — every project-stable value comes from `gh-optivem.yaml`. The `init` command accepts only per-invocation flags (workdir, verify-level, no-*, log-file, …).

### Logging flags

Available on `init`:

```bash
gh optivem init ... --verbose          # -v; debug output (retry/wait chatter, diagnostics)
gh optivem init ... --quiet            # -q; suppress info-level output (warnings + errors still shown)
gh optivem init ... --log-file run.log # override the log file path (default: $TEMP/gh-optivem-<timestamp>.log)
```

## Usage

`gh optivem` provides runner subcommands for the system + tests lifecycle in a scaffolded project. Each phase is its own verb (mirrors `docker compose`, `systemctl`, `kubectl`, `terraform`): the typical sequence is `compile` (source-level sanity check) → `test setup` (prepare the test harness) → `system start` (bring the SUT up) → `test run` (run suites) → `system stop`.

```bash
gh optivem compile
gh optivem test setup
gh optivem system start
gh optivem test run --suite smoke
gh optivem test run --suite acceptance-api
gh optivem system stop
```

The paths to `systems.yaml` / `tests.yaml` come from `gh-optivem.yaml`'s `system.config:` / `system_test.config:` fields — both are required by the runner commands (there is no built-in default-name fallback). Projects with non-default layouts (e.g. `docker/java/monolith/systems.yaml`) set the YAML fields once and forget; to pick an alternate variant ad hoc, select a different `gh-optivem.yaml` via the persistent `-c` / `--config` flag. See [Pointing at non-default configs](CONTRIBUTING.md#pointing-at-non-default-configs).

### Compile system

Source-level compile of the system tier (`dotnet build` / `./gradlew compileJava` / `npx tsc --noEmit`), dispatched per-tier by the `lang:` field in `gh-optivem.yaml`.

```bash
gh optivem system compile                 # system tier only
gh optivem compile                        # shortcut: system + test tiers (halts on first failure)
```

`compile` is the source-level build — distinct from `system build` (`docker compose build` / container image build). The two must not be conflated.

### Build system

`docker compose build` for every entry in `systems.yaml`.

```bash
gh optivem system build
gh optivem system build --rebuild         # force full rebuild (no layer cache reuse)
```

### Start system

`docker compose up` + wait for health.

```bash
gh optivem system start
gh optivem system start --restart         # force tear-down + restart
gh optivem system start --log-lines 200   # lines of compose logs to dump on health-probe failure (default 50)
gh optivem system start --up-timeout 10m  # per-attempt timeout for `docker compose up -d` (default 5m)
```

### Stop system

`docker compose down` + container cleanup.

```bash
gh optivem system stop
```

### Clean system

`docker compose down -v --rmi local` — delete volumes + locally-built images. Analog of `dotnet clean` / `./gradlew clean`: deletes build outputs without touching the dependency cache (registry-pulled images are kept). Chain it explicitly for a fresh start: `gh optivem system clean && gh optivem test run`.

```bash
gh optivem system clean
```

### Setup tests

Run `setupCommands` from `tests.yaml` (`npm ci`, restore, compile test sources, ...).

```bash
gh optivem test setup
```

### Compile tests

Source-level compile of the test tier only.

```bash
gh optivem test compile
```

### Run tests

`test run` health-probes every entry in `systems.yaml` first; if any aren't up, it errors out with "start it first with `gh optivem system start`" rather than silently starting them.

```bash
gh optivem test run                       # run every suite against the already-running system
gh optivem test run --suite smoke         # run only the suite with this id
gh optivem test run --suite acceptance-api --suite acceptance-ui   # multiple suites, repeatable
gh optivem test run --suite acceptance-api,acceptance-ui           # ...or comma-separated
gh optivem test run --test "MyTest"       # narrow execution to one test name (substituted into the suite's testFilter)
gh optivem test run --test T1 --test T2   # multiple names, repeatable
gh optivem test run --test T1,T2          # ...or comma-separated
gh optivem test run --sample              # use each suite's sampleTest field as the test name
gh optivem test run --list                # print suite ids from tests.yaml and exit
```

Multi-test semantics depend on the suite's `testFilter` in `tests.yaml`. The runner combines multiple `--test` values per `testFilterJoin`: `"or"` (default) joins names with `|` and substitutes once — works for dotnet (`&DisplayName~T1|T2`) and playwright/jest (`--grep 'T1|T2'`); `"repeat"` substitutes the whole `testFilter` once per name and concatenates — required for gradle (`--tests T1 --tests T2`). Practical ceiling on Windows is ~600 typical test names per invocation (the OS caps each command line at 32K characters).

<!-- TODO: revisit ATDD pipeline section — commented out for now
## Running the ATDD pipeline

Once a scaffolded project carries a valid `gh-optivem.yaml` and the sibling repos are cloned next to it, the ATDD subcommands walk the canonical process-flow state machine for one ticket:

```bash
gh optivem atdd implement-ticket --issue 42                    # walk the pipeline for a specific issue
gh optivem atdd implement-ticket --issue https://github.com/optivem/shop/issues/42
gh optivem atdd manage-project                                 # pick the top Ready ticket and walk the pipeline
```

Both commands accept the same per-invocation flags:

```bash
... --autonomous          # skip human-approval STOPs and dispatch agents headless via `claude -p`
... --manual-agents       # v1 fallback: pause at each user-task node and let the operator launch the agent manually
... --log-file run.log    # mirror stdout/stderr to this file
... --keep-runs 10        # max prompt-log run dirs to keep under .gh-optivem/runs/ (0 = never prune; default 10)
... --show-prompt         # dump each agent's full rendered prompt before dispatch (default: summary banner only)
```

`implement-ticket` additionally takes `--workspace <path>` to override the default workspace root (parent directory of CWD; each clone dir must be named after the repo-name component of its slug). Project-stable overrides (process flow, agent prompts, per-node text) live in `gh-optivem.yaml` — see [ATDD-specific overrides](#atdd-specific-overrides).

To inspect the embedded process-flow diagram without running the pipeline:

```bash
gh optivem atdd show diagram                            # print the canonical Mermaid markdown to stdout
gh optivem atdd show diagram > docs/process-diagram.md  # regenerate the committed diagram
```
-->

