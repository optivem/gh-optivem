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

Provide these credentials one of two ways:

- **OS environment variables** — set them on your machine the usual way. After setting them, restart your IDE / terminal for the changes to take effect (the env snapshot is taken when the process launches).
- **A portable `.env` file** (no restart needed) — copy [`.env.example`](.env.example), fill in the values, and drop it at the user-level path `gh optivem` loads at startup:
  - Windows: `%AppData%\gh-optivem\.env`
  - Linux/macOS: `~/.config/gh-optivem/.env`
  - Or keep it in a synced folder (Dropbox/OneDrive) and point at it with `GH_OPTIVEM_ENV_FILE=/abs/path/to/.env`.

  Edit the file any time — already-open shells pick up the new values on the next `gh optivem` run, with no terminal restart. A real exported environment variable always wins; the file only fills variables that are currently unset. Your filled-in copy is never committed (`.env` is gitignored; only the `.env.example` template is tracked).

The credentials, either way:

- `DOCKERHUB_USERNAME` — your Docker Hub username.
- `DOCKERHUB_TOKEN` — Docker Hub Personal Access Token (read-only scope is enough). Create at https://app.docker.com/settings/personal-access-tokens.
- `SONAR_TOKEN` — SonarCloud token. Create at https://sonarcloud.io/account/security.
- `GHCR_TOKEN` — GitHub PAT (classic) with `write:packages` + `read:packages`. Create at https://github.com/settings/tokens.
- `WORKFLOW_TOKEN` — GitHub PAT (classic) with `repo` + `workflow` scopes. Create at https://github.com/settings/tokens.
- `REPO_TOKEN` — GitHub PAT with `repo` scope (classic) or `Contents: Read` on each component repo (fine-grained). Create at https://github.com/settings/tokens or https://github.com/settings/personal-access-tokens.

_These are read from your local environment at scaffold time and then propagated as variables and secrets onto the GitHub repos that `gh optivem init` creates, so the pipelines it generates can pull base images from Docker Hub under the authenticated rate limit (rather than the much lower anonymous one), publish and pull pipeline images to/from GHCR, send analysis to SonarCloud, and dispatch cross-repo workflows — all without you having to set each secret in the GitHub UI afterwards._

_Tokens are read from env vars rather than passed as CLI flags so they don't end up in shell history or `ps` output, so a single set persists across `init`, `environment show`/`verify`, and re-runs, and so the local input contract matches how GitHub Actions exposes the same secrets to the generated pipelines._

To confirm what your shell is actually exporting (token values masked):

```bash
gh optivem environment show
```

To live-check each token is also accepted by its provider before scaffolding:

```bash
gh optivem environment verify
gh optivem environment verify --lang typescript,dotnet,java  # also check compilers for the listed languages
gh optivem environment verify --deploy docker                # also check the docker CLI is on PATH
```

`--lang` (comma-separated or repeated; values: `java`, `dotnet`, `typescript`) opts in to per-language compiler-presence checks. `--deploy` (value: `docker`) opts in to the deploy-target-conditional tool check. Both are opt-in so a CI preflight job can pin one matrix combo without coupling this command to the project-config schema.

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
- `--project-url` — URL of the GitHub Project board to attach. When omitted, `init` auto-creates the board and writes the URL back into `gh-optivem.yaml`.
- `--license` — SPDX-like license key: `mit` (default), `apache-2.0`, `gpl-3.0`, `bsd-2-clause`, `bsd-3-clause`, or `unlicense`. Drives the scaffolded `LICENSE` file and README badge.
- `--deploy` — deployment target: `docker` (default). `cloud-run` is in development and not yet usable.
- Tier paths — `--system-path`, `--system-test-path`, `--backend-path`, `--frontend-path`. Repo-relative paths to the corresponding tier. Pass these only to point the YAML at a non-flat existing repo; the flat scaffold layout `init` itself produces is the default.

Per-invocation flags — not written to `gh-optivem.yaml`; pass them on each `init` run:

- `--verify-level` — `none`, `local`, `commit`, `acceptance`, `qa`, or `release` (default `release`). Each level runs every step up to and including its named stage.
- `--no-legacy` — exclude legacy from local tests and acceptance stage.
- `--no-local-tests` — skip the local-test verification step.
- `--no-local-sonar` — skip the local SonarCloud scan step.
- `--no-project` — skip the `Ensure project board` step entirely (no auto-create, no status-ensure on a supplied `--project-url`).
- `--no-atdd` — no-op retained for backward compatibility; ATDD assets are sourced from the per-user sync (see [Methodology assets](#methodology-assets)), not installed per-repo.
- `--shop-ref` — pin `optivem/shop` to a specific ref (tag, SHA, or branch). Default: latest `meta-v*` release.
- `--workdir` — working directory for local clones (default: temp dir).
- `--keep-local` — keep the local scaffolded clone dir on success instead of deleting it.
- `--report-bug` — on failure, auto-create a GitHub issue in `optivem/gh-optivem` with scaffold config. Off by default.
- `--yes` / `-y` — skip all interactive confirmations (existing-repo prompt, bug-report confirmation). Expected for CI/unattended runs.
- `--log-file` — override path for the plain-text log mirror (default: `$TEMP/gh-optivem-<timestamp>.log`; always written).
- `--verbose` / `-v` — enable debug output (retry/wait chatter, diagnostics).
- `--quiet` / `-q` — suppress info-level output (warnings and errors still shown).

### Where `gh-optivem.yaml` lands

- **Default path** (no `--config`, no `$GH_OPTIVEM_CONFIG`): `gh optivem init` writes `gh-optivem.yaml` only inside the scaffolded repo on GitHub. Nothing is materialized in the current working directory.
- **Explicit path** (`--config /some/path.yaml` or `$GH_OPTIVEM_CONFIG=/some/path.yaml`): `init` writes/updates the YAML at the path you named, and the scaffolded repo still gets its own copy (rendered with the auto-created Project URL).
- **Pre-existing `<CWD>/gh-optivem.yaml`**: respected as operator-authored input. Loaded and used as-is; the scaffolded repo still gets its own rendered copy.

### Managing `gh-optivem.yaml` standalone

`gh optivem config` reads or writes `gh-optivem.yaml` outside of a full `init` run — useful for retrofitting a hand-rolled repo, validating a hand-edited file, or migrating an older config to the current schema.

```bash
gh optivem config init       # write a fresh gh-optivem.yaml from CLI flags (or interactive prompt on a TTY)
gh optivem config validate   # parse the YAML and validate it against the schema
gh optivem config preflight  # validate + check every declared repo and tier path exists on disk
gh optivem config migrate    # idempotently back-fill required fields (project.provider, repos:) on a pre-schema-bump file
```

`config init` accepts the same YAML-affecting flags as `gh optivem init` (`--owner`, `--repo`, `--system-name`, `--arch`, `--repo-strategy`, `--monolith-lang` / `--backend-lang` / `--frontend-lang`, `--test-lang`, `--project-url`, `--license`, `--deploy`, plus the `--system-path` / `--system-test-path` / `--backend-path` / `--frontend-path` tier-path overrides). On a TTY with no required flags set, it drops into the same interactive prompt the `init` command uses. Extra flags: `--force` (overwrite an existing file) and `--dir <dir>` (target directory; ignored when `--config` is set).

`config preflight` accepts `--workspace <dir>` to point at a non-default workspace root (default: parent directory of CWD). `config validate` and `config migrate` take no flags beyond the root-level `--config`.

```bash
gh optivem config init --owner acme --repo page-turner \
    --arch monolith --repo-strategy monorepo --monolith-lang java \
    --test-lang java --project-url https://github.com/orgs/acme/projects/1
gh optivem -c ./gh-optivem.myrepo.yaml config validate
gh optivem config preflight --workspace /abs/path/to/workspace
```

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

#### Probe system status

Snapshot probe of every component + external-system URL in `systems.yaml`. Prints `OK` or `DOWN` per entry and exits non-zero if any are DOWN, so it can be used in shell pipelines.

```bash
gh optivem system status
gh optivem system status --timeout 5s    # per-URL probe timeout (default 2s)
```

No retries, no waiting — pair with `system start` for the lifecycle ("did it come up?"). After a successful `gh optivem implement --issue N`, the same block is printed automatically as an `=== System endpoints ===` banner.

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

Multi-test semantics depend on the suite's `testFilter` in `tests.yaml`. The runner combines multiple `--test` values per `testFilterJoin`: `"or"` (default) joins names with `|` and substitutes once — works for playwright/jest (`--grep 'T1|T2'`) where `|` is alternation at the value level; `"repeat"` substitutes the whole `testFilter` once per name and concatenates — required for gradle (`--tests T1 --tests T2`) where the flag itself must repeat; `"fragment-or"` (for `&`-prefixed injection fragments) substitutes per name, joins with `|`, wraps in `( ... )`, and injects as one expression — required for dotnet (`&(DisplayName~T1|DisplayName~T2)`) whose `--filter` parser ORs full property terms, not bare values. Practical ceiling on Windows is ~600 typical test names per invocation (the OS caps each command line at 32K characters).

## Cross-repo operations

`gh optivem commit`, `sync`, and `actions status` infer scope from the environment. The cascade resolves to one of three modes:

- **Workspace** — a `*.code-workspace` file is reachable (via the `--workspace <dir>` flag, the `$GH_OPTIVEM_WORKSPACE` env var, or a walk-up from the current directory); the verb iterates every folder in the workspace file.
- **Project** — no workspace file is reachable, but CWD walks up to a `gh-optivem.yaml` with a non-empty `repos:` list; the verb iterates every listed local repo (used for multitier projects whose tiers live in sibling clones).
- **Single repo** — neither of the above; the verb acts on the cwd repo only.

`rate-limit` is a single API call with no scope.

```bash
gh optivem commit "Update settings"                                     # stage, commit, pull, push every dirty repo in scope
gh optivem commit --repo myrepo "Fix bug"                               # only operate on the named repo (workspace mode)
gh optivem commit --repo myrepo --paths "system/monolith/java" "fix"    # stage only the listed space-separated paths (requires --repo)
gh optivem commit --yes "Sync .claude"                                  # skip the y/N confirmation (required without a TTY)
gh optivem sync                                                         # pull + push every repo in scope (no commit)
gh optivem actions status                                               # latest run of every workflow in every repo in scope
gh optivem rate-limit                                                   # current GitHub API rate limits and reset times
```

Each run prints a `Mode:` banner showing the resolved scope — `Mode: workspace (5 repos from page-turner.code-workspace)`, `Mode: project (3 repos from gh-optivem.yaml)`, or `Mode: single repo (shop)`.

`commit --yes` refuses to stage untracked (`??`) files unless `--include-untracked` is also passed — the stray-file foot-gun is opt-in for scripted callers.

## Auto-approve

`gh optivem` prompts on every confirmation by default. To run unattended, opt into auto-approve policy with `--auto`:

```bash
gh optivem --auto implement --issue 42          # skip everything except commit + fix (default exclusion)
gh optivem --auto --confirm= implement          # truly autonomous: prompt only on human STOP nodes
gh optivem --auto --confirm=fix implement       # narrower: prompt only on fix-agent dispatch (and human)
```

`--auto` is a root-level persistent flag. When set, every confirmation auto-yeses *except* the categories listed in `--confirm`:

| Category | Covers |
|---|---|
| `commit` | git commit confirmations (lands on GitHub history) |
| `fix` | ATDD approve nodes wrapping `fix-*` agent dispatch (recovery flow) |
| `release` | ATDD release confirmer |
| `prompt` | low-stakes interactive prompts (init walks, doctor, bug-report, ATDD non-fix approve) |
| `human` | ATDD `agent: human` STOP nodes — always confirmed, cannot be auto-yes'd |

Default `--confirm` when `--auto` is set and `--confirm` is omitted: `commit,fix` (plus implicit `human`). This protects the two expensive failure modes (publishing the wrong commit; auto-rewriting files in a recovery flow) by default. Override with an explicit `--confirm=<categories>` to narrow or broaden.

Environment variables:

- `GH_OPTIVEM_AUTO=true` — same as `--auto`.
- `GH_OPTIVEM_CONFIRM=<categories>` — same as `--confirm=<categories>`.

Flag overrides env; both override default. A one-line banner is emitted to stderr at command start showing the resolved policy and where each part came from:

```
Auto: true (auto-source: flag, confirm-source: default → commit,fix)
```

The per-command `--yes` flag on `commit` is unchanged — `gh optivem commit --yes "msg"` still skips the per-repo confirmation directly, independent of `--auto`. The two compose: `gh optivem --auto commit "msg"` also commits without prompting unless `commit` is in the confirm set (it is, by default).

## Cleanup

`gh optivem cleanup <verb>` bulk-deletes remote artifacts. Each subcommand pre-flights `gh api rate_limit` before every destructive call and sleeps `--delay-seconds` (default 10) after each delete to stay under GitHub's 80-mutating-calls/minute secondary limit. Always pass `--dry-run` first to preview.

```bash
gh optivem cleanup releases optivem/greeter-java --dry-run
gh optivem cleanup releases optivem/greeter-java optivem/greeter-dotnet
gh optivem cleanup packages myorg/myrepo --before-date 2026-01-01
gh optivem cleanup repos valentinajemuovic --prefix course-tester- --dry-run
gh optivem cleanup sonar-projects myorg --prefix myorg_course-tester- --dry-run
```

`cleanup releases` and `cleanup packages` take one or more positional `owner/repo` slugs; `cleanup repos` and `cleanup sonar-projects` take a single positional `<owner>` (or `<organization>`) followed by either `--prefix <prefix>`, explicit names/keys, or both. `cleanup sonar-projects` requires `$SONAR_TOKEN` (the same token the scaffolder reads).

## Running the implementation pipeline

Once a scaffolded project carries a valid `gh-optivem.yaml` and the sibling repos are cloned next to it, the `implement` subcommand walks the configured process-flow state machine for one ticket:

```bash
gh optivem implement --issue 42                                # walk the pipeline for a specific issue
gh optivem implement --issue https://github.com/myorg/myrepo/issues/42
gh optivem implement                                           # pick the top Ready ticket and walk the pipeline from START
```

`implement` accepts the same per-invocation flags whether or not `--issue` was passed:

```bash
... --headless            # run the claude subprocess headless (claude -p); structured JSON envelope captured for the exit banner
... --autonomous          # [Deprecated] alias for --auto --headless; will be removed in a future release
... --manual-agents       # v1 fallback: pause at each user-task node and let the operator launch the agent manually
... --log-file run.log    # mirror everything stdout/stderr emit during the run to this file (always Detail = firehose by default)
... --log-level phase     # narrow the --log-file capture to phase only (BPMN trace + prompts); pairs with --log-file
... --verbose / -v        # stream the full firehose to the terminal (subprocess output, agent body, prompt-prep banners). Default: terminal shows only BPMN trace, agent enter/exit banners, and prompts
... --keep-runs 10        # max prompt-log run dirs to keep under .gh-optivem/runs/ (0 = never prune; default 10)
... --show-prompt         # dump each agent's full rendered prompt before dispatch (default: summary banner only)
... --workspace <path>    # override the default workspace root (parent directory of CWD; each clone dir must be named after the repo-name component of its slug)
```

#### Output levels

`implement` separates terminal output from `--log-file` content via two levels:

- **Phase** — `[phase] start / end …` boundary banners, BPMN trace lines (`[trace …] > NODE_ID …`), approval / STOP prompts, errors, `[agent] enter / exit / FAIL …` lines. The headline channel the operator needs to follow the run.
- **Detail** — subprocess byte streams (gradle, docker, gh CLI, agent body), `[agent] prep` summary, the `$ <command>` echo, internal banners. The firehose used for forensic dig.

Each sink subscribes to a maximum level. Defaults: terminal = Phase (clean), `--log-file` = Detail (firehose). Independently configurable via `--verbose` (terminal up to Detail) and `--log-level=phase|detail` (log file).

For skipping confirmation prompts during a pipeline run (or any other `gh optivem` invocation), see [Auto-approve](#auto-approve) — `gh optivem --auto implement --headless` is the typical unattended invocation.

Project-stable overrides (`process_flow:`, `task_prompts:`, `node_extras:`, `node_replacements:`) live in `gh-optivem.yaml` and are read at startup.

### Process diagram

To inspect the configured process-flow Mermaid diagram without running the pipeline:

```bash
gh optivem process show                            # print the canonical Mermaid markdown to stdout
gh optivem process show > docs/process-diagram.md  # regenerate the committed diagram
```

## Trunk-based development helpers

`gh optivem doctor`, `branch`, `pr`, and `hooks` encapsulate the trunk-based development rituals from [docs/tbd.md](docs/tbd.md) so the operator runs one command instead of three.

```bash
gh optivem doctor                              # verify the three global git config keys docs/tbd.md mandates
gh optivem doctor --fix                        # set any missing or wrong keys to the required values
gh optivem branch start feature/payments       # checkout main, pull --rebase, checkout -b <name> off latest origin/main
gh optivem branch refresh                      # fetch origin, rebase current branch onto origin/main, push --force-with-lease (refuses on main)
gh optivem pr merge                            # squash-merge a PR via `gh pr merge` (TBD-safe: never a merge commit)
gh optivem pr merge 123 --rebase               # rebase-merge instead
gh optivem pr merge --auto --squash --delete-branch
gh optivem hooks install                       # install a pre-push hook that refuses non-fast-forward pushes to main
```

`pr merge` defaults to `--squash`; `--rebase` is opt-in and the two are mutually exclusive. The `--merge` mode is intentionally not exposed because merge commits on `main` break the linear-trunk invariant. Pass any other `gh pr merge` flags directly to the underlying CLI.

## Methodology assets

`gh optivem` ships its ATDD methodology assets (the per-phase agent prompts and the shared preamble) embedded in the binary. They are fed to the `claude -p` subprocess via argv at dispatch time and are never written to disk in consumer repos — scaffolded projects hold zero ATDD assets locally, and updates propagate simply by upgrading the `gh-optivem` binary.

## Further reading

- [Trunk Based Development (TBD)](docs/tbd.md) — how to work with `main` in this repo (and the repos it scaffolds), the role of `pull --rebase`, when to use short-lived PRs, and why the version-bump bot is just another committer.
- [Process diagram](docs/process-diagram.md) — committed Mermaid diagram of the configured implementation process flow (regenerate with `gh optivem process show`).
- [BPMN process design](docs/bpmn-process-design.md) — the *why* behind the five-level process model (TOP / CYCLE / HIGH / MID / LOW): primitives, doctrine decisions, and the ticket-to-cycle mapping.

