# gh-optivem — full reference

A GitHub CLI extension that scaffolds a full delivery pipeline and then drives tickets through it with AI agents.

Two commands carry the workflow:

```bash
gh optivem init              # scaffold the project: repos, pipelines, SonarCloud, project board
gh optivem implement 42      # walk ticket #42 through the ATDD pipeline, agent by agent
```

`init` is a one-time setup per project. `implement` is the day-to-day verb — it walks a BPMN process-flow state machine for one ticket, dispatching a Claude agent at each node (write acceptance tests → implement → refactor → commit) and running the real build, system, and test commands in between. Everything else is a supporting verb.

`gh optivem <command> --help` is always authoritative; this document adds the rationale that doesn't fit in a flag description. For the short version, see the [README](../README.md).

- [Install](#install)
- [Claude Code setup](#claude-code-setup)
- [Environment variables](#environment-variables)
- [Scaffolding (`init`)](#scaffolding-init)
- [Managing `gh-optivem.yaml`](#managing-gh-optivemyaml)
- [Implement a ticket](#implement-a-ticket)
- [System verbs](#system-verbs)
- [Tests](#tests)
- [Auto-approve](#auto-approve)
- [Cross-repo operations](#cross-repo-operations)
- [Cleanup](#cleanup)
- [Inspecting a past run](#inspecting-a-past-run)
- [Diagrams](#diagrams)
- [Trunk-based development helpers](#trunk-based-development-helpers)
- [Methodology assets](#methodology-assets)

## Install

[GitHub CLI](https://cli.github.com/) is required, and you must be logged in:

```bash
gh --version        # install: winget install GitHub.cli / brew install gh / apt install gh
gh auth status      # log in with: gh auth login
```

Then:

```bash
gh extension install optivem/gh-optivem     # install
gh optivem --version                        # verify
gh extension upgrade optivem                # upgrade
gh extension remove optivem                 # uninstall
```

`actionlint` must also be on `PATH` before you run `init` — the `Verify scaffolded workflows` step shells out to it. `go install` writes it to `$(go env GOPATH)/bin` (usually `~/go/bin`) without touching `PATH`, so add that directory yourself:

```bash
go install github.com/rhysd/actionlint/cmd/actionlint@v1
actionlint -version
```

## Claude Code setup

`gh optivem` ships the Optivem Claude slash commands and global configuration rules embedded in the binary. `gh optivem claude setup` does the whole job; the pieces are also available individually.

```bash
gh optivem claude install     # copy the embedded slash commands to ~/.claude/commands/
gh optivem claude configure   # merge Optivem settings.json permissions + CLAUDE.md rules into ~/.claude/
gh optivem claude setup       # both of the above
gh optivem claude check       # report drift without writing anything
```

`install` skips files already up to date, overwrites changed ones, and prints a summary. `configure` is non-destructive — it never removes entries you added yourself. Re-run `setup` any time after upgrading the extension to pick up new or updated commands and configuration rules.

`check` reports, for each embedded slash command, whether it is missing from `~/.claude/commands/`, differs in content, or is in sync; then whether `settings.json` is missing any Optivem permissions or hooks and whether `CLAUDE.md` is missing any Optivem rule section. It exits non-zero when any drift is found, so it can gate CI.

## Environment variables

Provide these credentials one of two ways:

- **OS environment variables** — set them on your machine the usual way. After setting them, restart your IDE / terminal for the changes to take effect (the env snapshot is taken when the process launches).
- **A portable `.env` file** (no restart needed) — copy [`.env.example`](../.env.example), fill in the values, and drop it at the user-level path `gh optivem` loads at startup:
  - Windows: `%AppData%\gh-optivem\.env`
  - macOS: `~/Library/Application Support/gh-optivem/.env`
  - Linux: `~/.config/gh-optivem/.env` (or `$XDG_CONFIG_HOME/gh-optivem/.env` when that variable is set)
  - Or keep it in a synced folder (Dropbox/OneDrive) and point at it with `GH_OPTIVEM_ENV_FILE=/abs/path/to/.env`.

  Edit the file any time — already-open shells pick up the new values on the next `gh optivem` run, with no terminal restart. A real exported environment variable always wins; the file only fills variables that are currently unset. Your filled-in copy is never committed (`.env` is gitignored; only the `.env.example` template is tracked).

The credentials, either way:

- `DOCKERHUB_USERNAME` — your [Docker Hub](https://hub.docker.com) username.
- `DOCKERHUB_TOKEN` — a [Docker Hub Personal Access Token](https://app.docker.com/settings/personal-access-tokens) (read-only scope is enough).
- `SONAR_TOKEN` — a [SonarCloud token](https://sonarcloud.io/account/security).
- `GHCR_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `write:packages` + `read:packages`.
- `WORKFLOW_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `repo` + `workflow` scopes.
- `REPO_TOKEN` — a [GitHub PAT (classic)](https://github.com/settings/tokens) with `repo` scope.

The three GitHub PATs (`GHCR_TOKEN`, `WORKFLOW_TOKEN`, `REPO_TOKEN`) back cron-scheduled pipelines (e.g. `<arch>-<lang>-acceptance-stage-legacy.yml`, which runs hourly) that keep running indefinitely once scaffolded — when one of these tokens expires, the schedule starts failing silently, with no scaffold-time signal. Put a reminder in your calendar to rotate them, and re-run `gh optivem environment verify` after each rotation; it warns when a classic PAT's expiration is within 7 days.

These are read from your local environment at scaffold time and then propagated as variables and secrets onto the GitHub repos that `gh optivem init` creates, so the pipelines it generates can pull base images from Docker Hub under the authenticated rate limit (rather than the much lower anonymous one), publish and pull pipeline images to/from GHCR, send analysis to SonarCloud, and dispatch cross-repo workflows — all without you having to set each secret in the GitHub UI afterwards.

Tokens are read from env vars rather than passed as CLI flags so they don't end up in shell history or `ps` output, so a single set persists across `init`, `environment show`/`verify`, and re-runs, and so the local input contract matches how GitHub Actions exposes the same secrets to the generated pipelines.

To confirm what your shell is actually exporting (token values masked):

```bash
gh optivem environment show
```

To live-check each token is also accepted by its provider before scaffolding:

```bash
gh optivem environment verify
gh optivem environment verify --lang typescript,dotnet,java  # also check compilers for the listed languages
```

Always checked: the gh CLI (installed and authenticated), `actionlint`, `bash`, `docker`, `claude`, `DOCKERHUB_USERNAME`, and the five tokens. `bash` and `docker` are unconditional rather than gated on the deploy target — every scaffold shells out through bash and runs its system on `docker compose`, whatever the project deploys to. `claude` is presence-only (the CLI exposes no non-interactive sign-in probe) and is the one check with an opt-out: set `GH_OPTIVEM_SKIP_CLAUDE_CHECK=1` on GitHub-hosted runners, which scaffold but never reach `implement`. The skip is logged, so a green verify never hides a silently dropped check.

`--lang` (comma-separated or repeated; values: `java`, `dotnet`, `typescript`) opts in to per-language compiler-presence checks. It is opt-in so a CI preflight job can pin one matrix combo without coupling this command to the project-config schema.

## Scaffolding (`init`)

```bash
gh optivem init
```

See [how-it-works.md](how-it-works.md) for the phased step list `init` executes and how `--verify-level` selects the verification steps.

Project-stable values — prompted on first run and written to `gh-optivem.yaml` (or passed as flags for non-interactive runs):

- `--owner` — GitHub owner (user or org) for the scaffolded repo(s).
- `--repo` — repo name (or monorepo root name for multi-repo layouts).
- `--system-name` — human-readable system name (e.g. `"Page Turner"`).
- `--repo-strategy` — `monorepo` or `multirepo`.
- `--arch` — system architecture: `monolith` or `multitier`. (A third architecture, `microservices`, is YAML-authored only — declare it directly in `gh-optivem.yaml` under `system.backend-services:`, not via this flag.)
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
- `--no-atdd` — no-op retained for backward compatibility; ATDD assets ship embedded in the binary (see [Methodology assets](#methodology-assets)), not installed per-repo.
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

## Managing `gh-optivem.yaml`

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
    --repo-strategy monorepo --arch monolith --monolith-lang java \
    --test-lang java --project-url https://github.com/orgs/acme/projects/1
gh optivem -c ./gh-optivem.myrepo.yaml config validate
gh optivem config preflight --workspace /abs/path/to/workspace
```

## Implement a ticket

Once a scaffolded project carries a valid `gh-optivem.yaml` and the sibling repos are cloned next to it, `implement` walks the configured process-flow state machine for one ticket:

```bash
gh optivem implement 42                                        # walk the pipeline for a specific issue
gh optivem implement --issue 42                                # ...or the flag form (supply exactly one)
gh optivem implement --issue https://github.com/myorg/myrepo/issues/42
gh optivem implement                                           # pick the top Ready ticket and walk from START
```

### Team handoff (`--target` / `--channel`)

With no `--target`, `implement` walks the whole pipeline start to end — the fullstack-developer default. A ticket can instead be produced in contiguous slices that different teams own, handed off via commit:

| `--target` | Slice | `--channel` |
|---|---|---|
| `test` | The shared, channel-agnostic contract (acceptance tests + DSL + driver ports + external system) the whole team mobs. Ends RED by design. | rejected |
| `driver-adapter` | One channel's test-side System Driver adapter. | required |
| `system` | One channel's system (the first channel also builds the channel-agnostic common layer). | required |

```bash
gh optivem implement 42 --target test                          # whole team: shared RED contract
gh optivem implement 42 --target driver-adapter --channel api  # API team: its driver adapter
gh optivem implement 42 --target system --channel api          # API team: its system (channel green)
```

`--channel` is validated against the project's `channels:` list. There is no resume status file — a slice reads how far the ticket got from the committed tree, so it refuses to start until its upstream slice is committed.

### Flags

| Flag | Effect |
|---|---|
| `--target <slice>` | Scope the run to one pipeline slice: `test`, `driver-adapter`, or `system`. Default: walk the whole pipeline. |
| `--channel <ch>` | Channel for a channel-split `--target`. Required for `driver-adapter` / `system`, rejected for `test`. |
| `--headless` | Run the claude subprocess headless (`claude -p`); a structured JSON envelope is captured for the exit banner. |
| `--manual-agents` | v1 fallback: pause at each user-task node and let the operator launch the agent manually. |
| `--show-prompt` | Dump each agent's full rendered prompt before dispatch. Default: summary banner only. |
| `--verbose` / `-v` | Stream the full firehose to the terminal (subprocess output, agent body, prompt-prep banners). |
| `--log-file <path>` | Mirror everything stdout/stderr emit during the run to this file. |
| `--log-level <lvl>` | Narrow the `--log-file` capture to `phase` (BPMN trace + prompts) instead of `detail`. Pairs with `--log-file`. |
| `--keep-runs <n>` | Max prompt-log run dirs to keep under `.gh-optivem/runs/` (`0` = never prune; default `10`). |
| `--workspace <path>` | Override the default workspace root (parent directory of CWD). Each clone dir must be named after the repo-name component of its slug. |
| `--autonomous` | **[Deprecated]** alias for `--auto --headless`; will be removed in a future release. |

### Output levels

`implement` separates terminal output from `--log-file` content via two levels:

- **Phase** — `[phase] start / end …` boundary banners, BPMN trace lines (`[trace …] > NODE_ID …`), approval / STOP prompts, errors, `[agent] enter / exit / FAIL …` lines. The headline channel the operator needs to follow the run.
- **Detail** — subprocess byte streams (gradle, docker, gh CLI, agent body), `[agent] prep` summary, the `$ <command>` echo, internal banners. The firehose used for forensic dig.

Each sink subscribes to a maximum level. Defaults: terminal = Phase (clean), `--log-file` = Detail (firehose). Independently configurable via `--verbose` (terminal up to Detail) and `--log-level=phase|detail` (log file).

### Unattended runs

By default every confirmation prompts. `--auto` (a root-level flag, so it goes *before* `implement`) auto-approves everything below the `--confirm` threshold tier:

```bash
gh optivem --auto implement 42 --headless                # typical unattended invocation
gh optivem --auto --confirm=system-commit implement 42   # ...but still confirm every commit
```

The default floor is `human`, so BPMN STOP nodes and fix-agent dispatch always prompt. See [Auto-approve](#auto-approve) for the full tier ladder.

Project-stable overrides (`process_flow:`, `task_prompts:`, `node_extras:`, `node_replacements:`) live in `gh-optivem.yaml` and are read at startup.

## System verbs

### Compile system

Source-level compile of the system tier (`dotnet build` / `./gradlew compileJava` / `npx tsc --noEmit`), dispatched per-tier by the `lang:` field in `gh-optivem.yaml`.

```bash
gh optivem system compile                 # system tier only
gh optivem compile                        # shortcut: system + component-test + system-test tiers (halts on first failure)
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
gh optivem system start --restart         # recreate changed services (keeps the database running — no full down/up)
gh optivem system start --log-lines 200   # lines of compose logs to dump on health-probe failure (default 50)
gh optivem system start --up-timeout 10m  # per-attempt timeout for `docker compose up -d` (default 5m)
```

### Probe system status

Snapshot probe of every component + external-system URL in `systems.yaml`. Prints `OK` or `DOWN` per entry and exits non-zero if any are DOWN, so it can be used in shell pipelines.

```bash
gh optivem system status
gh optivem system status --timeout 5s    # per-URL probe timeout (default 2s)
```

No retries, no waiting — pair with `system start` for the lifecycle ("did it come up?").

### Stop and clean

```bash
gh optivem system stop     # docker compose down + container cleanup
gh optivem system clean    # docker compose down -v --rmi local
```

`clean` deletes volumes + locally-built images. Analog of `dotnet clean` / `./gradlew clean`: deletes build outputs without touching the dependency cache (registry-pulled images are kept). Chain it explicitly for a fresh start: `gh optivem system clean && gh optivem system-test run`.

## Tests

Tests are organised as two **levels**, each with its own noun and the same `run` / `setup` / `compile` verb set:

- **`component-test`** — the commit-stage suites declared in each component's `component-tests.yaml`. No compose, no system lifecycle, no health probe: each suite's command runs in the component directory.
- **`system-test`** — the suites declared in `tests.yaml`, run against an already-running system.

The bare verb **`gh optivem test`** is the for-all aggregate: it walks every level cheapest-first, halting on the first failure.

### Run everything (pyramid order)

```bash
gh optivem test                           # component-test suites → system start → system-test run → system stop
gh optivem test --assume-running          # CI: skip the start/stop, the caller owns the system lifecycle
```

`test` manages the system lifecycle itself by default. CI already starts and stops the system explicitly around its acceptance stage, so it passes `--assume-running` to avoid double-managing it.

### Component tests

```bash
gh optivem component-test run                                  # every suite × every component
gh optivem component-test run --suite unit                     # narrow to one suite id (repeatable, comma-separated, or a suiteGroups alias)
gh optivem component-test run --suite component --component backend   # narrow to one component (repeatable, comma-separated)
gh optivem component-test run --test T1,T2                     # narrow to given test names
gh optivem component-test run --sample                         # use each suite's sampleTest field as the test name
gh optivem component-test run --list                           # print each component's suite ids and exit
gh optivem component-test setup [--component backend]          # run each component's setupCommands
gh optivem component-test compile [--component backend]        # compile the component-test tier
```

`--suite` / `--component` narrow the run for local feedback only; CI gates on the full run. Pending suites print a notice and pass; suites that need Docker fail fast when no daemon is reachable.

### System tests

```bash
gh optivem system-test setup     # run setupCommands from tests.yaml (npm ci, restore, compile test sources, ...)
gh optivem system-test compile   # source-level compile of the system-test tier only
```

> [!WARNING]
> The system must already be running (`gh optivem system start`). `system-test run` health-probes every entry in `systems.yaml` first; if any aren't up, it errors out with "start it first with `gh optivem system start`" rather than silently starting them.

```bash
gh optivem system-test run                       # every suite against the already-running system
gh optivem system-test run --suite smoke         # run only the suite with this id
gh optivem system-test run --suite acceptance-api --suite acceptance-ui   # multiple suites, repeatable
gh optivem system-test run --suite acceptance-api,acceptance-ui           # ...or comma-separated
gh optivem system-test run --suite acceptance    # group alias: expands to every acceptance-* suite
gh optivem system-test run --list                # print suite ids from tests.yaml and exit
gh optivem system-test run --test "MyTest"       # narrow to one test name (substituted into the suite's testFilter)
gh optivem system-test run --test T1 --test T2   # multiple names, repeatable
gh optivem system-test run --test T1,T2          # ...or comma-separated
gh optivem system-test run --sample              # use each suite's sampleTest field as the test name
```

`--test` cannot be combined with multiple `--suite` values — a test name is substituted into one suite's `testFilter`, so applying it across suites is ambiguous. Narrow to a single `--suite`, or run the command once per suite.

Multi-test semantics depend on the suite's `testFilter` in `tests.yaml`. The runner combines multiple `--test` values per `testFilterJoin`: `"or"` (default) joins names with `|` and substitutes once — works for playwright/jest (`--grep 'T1|T2'`) where `|` is alternation at the value level; `"repeat"` substitutes the whole `testFilter` once per name and concatenates — required for gradle (`--tests T1 --tests T2`) where the flag itself must repeat; `"fragment-or"` (for `&`-prefixed injection fragments) substitutes per name, joins with `|`, wraps in `( ... )`, and injects as one expression — required for dotnet (`&(DisplayName~T1|DisplayName~T2)`) whose `--filter` parser ORs full property terms, not bare values. Practical ceiling on Windows is ~600 typical test names per invocation (the OS caps each command line at 32K characters).

## Auto-approve

`gh optivem` prompts on every confirmation by default. To run unattended, opt into auto-approve policy with `--auto`:

```bash
gh optivem --auto implement 42                           # truly autonomous: prompt only on human-tier sites
gh optivem --auto --confirm=system-commit implement 42   # narrower: still prompt on both commit tiers (and human)
gh optivem --auto --confirm=system-agent implement 42    # narrower still: prompt from production-agent dispatch upward
```

`--auto` is a root-level persistent flag. `--confirm` takes a **single tier**, which becomes the *threshold floor*: sites at or above the floor still prompt, sites strictly below it auto-yes. The tiers, ordered low-to-high by stakes:

| Tier | Covers |
|---|---|
| `command` | execute-command BPMN nodes (compile / build / start / test run). Cheap, no AI cost, no global state mutation. |
| `system-agent` | execute-agent for production code (`implement-*`, `update-*`, `refactor-system`). AI cost; produces reviewable diffs. |
| `test-agent` | execute-agent for tests (`write-*-tests`, `refactor-tests`). Tests-as-contract — ranked above `system-agent` because broken tests mask regressions. |
| `system-commit` | commit node after a production-agent phase. Persistent git write. |
| `test-commit` | commit node after a test-agent phase. Persistent git write of the test contract. |
| `human` | fix-`*` agents, `refine-acceptance-criteria`, BPMN STOP nodes, release. Top tier — always prompts, cannot be auto-yes'd at any reachable floor. |

Default `--confirm` when `--auto` is set and `--confirm` is omitted: `human` — i.e. truly autonomous, everything below the human tier auto-yeses. Pass a lower tier to keep prompting on it and everything above it.

> [!WARNING]
> `--confirm` is a single tier, not a list: a comma-separated value (`--confirm=commit,fix`) is rejected. An explicit empty `--confirm=` resolves the floor to the lowest tier, which makes *every* site prompt — the opposite of autonomous.

Environment variables:

- `GH_OPTIVEM_AUTO=true` — same as `--auto`.
- `GH_OPTIVEM_CONFIRM=<tier>` — same as `--confirm=<tier>`.

Flag overrides env; both override default. A one-line banner is emitted to stderr at command start showing the resolved policy and where each part came from:

```
Auto: true (auto-source: flag, confirm-source: default → floor=human)
```

The per-command `--yes` flag on `commit` skips the per-repo confirmation directly, independent of `--auto`. The two do *not* compose: the cross-repo `commit` prompt is registered at the `human` tier, so `gh optivem --auto commit "msg"` still asks per repo — `--yes` is the only way to skip it. Because `--yes` removes the human review of the staged file list, it refuses a blanket `git add -A` unless you opt in with `--all` (or scope the stage with `--paths`) — and refuses untracked files unless you add `--include-untracked`. This keeps unrelated working-tree changes (e.g. parallel-agent WIP) from being swept into a scripted commit.

## Cross-repo operations

`gh optivem commit`, `sync`, and `actions status` infer scope from the environment. The cascade resolves to one of three modes:

- **Workspace** — a `*.code-workspace` file is reachable (via the `--workspace <dir>` flag, the `$GH_OPTIVEM_WORKSPACE` env var, or a walk-up from the current directory); the verb iterates every folder in the workspace file.
- **Project** — no workspace file is reachable, but CWD walks up to a `gh-optivem.yaml` with a non-empty `repos:` list; the verb iterates every listed local repo (used for multitier projects whose tiers live in sibling clones).
- **Single repo** — neither of the above; the verb acts on the cwd repo only.

`rate-limit` is a single API call with no scope.

```bash
gh optivem commit "Update settings"                                     # stage, commit, pull, push every dirty repo in scope
gh optivem commit --repo myrepo "Fix bug"                               # only operate on the named repo (workspace mode)
gh optivem commit --repo myrepo --paths "system/monolith/java" "fix"    # stage only the listed space-separated paths (requires --repo) — surgical, no sweep
gh optivem commit --yes --all "Sync .claude"                            # skip the y/N confirmation; --all opts in to the blanket git add -A sweep
gh optivem commit --yes "Sync .claude"                                  # ERROR: --yes refuses a blanket stage without --all (use --paths or --all)
gh optivem sync                                                         # pull + push every repo in scope (no commit)
gh optivem actions status                                               # latest run of every workflow in every repo in scope
gh optivem rate-limit                                                   # current GitHub API rate limits and reset times
```

Each run prints a `Mode:` banner showing the resolved scope — `Mode: workspace (5 repos from page-turner.code-workspace)`, `Mode: project (3 repos from gh-optivem.yaml)`, or `Mode: single repo (shop)`.

`commit --yes` refuses to stage untracked (`??`) files unless `--include-untracked` is also passed — the stray-file foot-gun is opt-in for scripted callers.

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

## Inspecting a past run

`gh optivem run summary` replays the per-agent summary table from a run's on-disk sidecar (`.gh-optivem/runs/<ts>/summary.jsonl`), written one line per dispatch as the run progresses — so a binary crash mid-run still leaves every row that completed before the bust.

```bash
gh optivem run summary                     # most-recently-modified run under <cwd>/.gh-optivem/runs/
gh optivem run summary 20260528-150000     # a specific run-timestamp directory
gh optivem run summary --markdown          # the human run digest (summary.md) instead of the table
```

Columns: agent, model, effort, elapsed, in / out tokens, `$` cost. A step-execution table follows when `steps.jsonl` is present.

## Diagrams

```bash
gh optivem process show                            # print the process-flow Mermaid markdown to stdout
gh optivem process show > docs/process-diagram.md  # regenerate the committed diagram
gh optivem process show --expanded > docs/process-diagram-expanded.md  # call-activities inlined as subgraphs

gh optivem architecture show                                   # print the architecture Mermaid markdown to stdout
gh optivem architecture show > docs/architecture-diagram.md    # regenerate the committed diagram
```

To see which paths each agent phase is allowed to touch (`process-flow.yaml` × `gh-optivem.yaml`):

```bash
gh optivem process scope                              # every phase
gh optivem process scope write-acceptance-tests       # one phase
```

Without a `gh-optivem.yaml` in scope, layer names print bare; with one, they resolve to physical paths.

## Trunk-based development helpers

`gh optivem doctor`, `branch`, `pr`, and `hooks` encapsulate the trunk-based development rituals from [tbd.md](tbd.md) so the operator runs one command instead of three.

```bash
gh optivem doctor                              # verify the three global git config keys tbd.md mandates
gh optivem doctor --fix                        # set any missing or wrong keys to the required values
gh optivem doctor --orphans                    # sweep for orphan claude subprocesses from crashed `implement` runs
gh optivem branch start feature/payments       # checkout main, pull --rebase, checkout -b <name> off latest origin/main
gh optivem branch refresh                      # fetch origin, rebase current branch onto origin/main, push --force-with-lease (refuses on main)
gh optivem pr merge                            # squash-merge a PR via `gh pr merge` (TBD-safe: never a merge commit)
gh optivem pr merge 123 --rebase               # rebase-merge instead
gh optivem pr merge --auto --squash --delete-branch
gh optivem hooks install                       # install a pre-push hook that refuses non-fast-forward pushes to main
```

`pr merge` defaults to `--squash`; `--rebase` is opt-in and the two are mutually exclusive. The `--merge` mode is intentionally not exposed because merge commits on `main` break the linear-trunk invariant. Pass any other `gh pr merge` flags directly to the underlying CLI.

`doctor --orphans` runs the orphan-recovery sweep *instead of* the git-config check (invoke the command twice to run both). It lists the `claude` subprocesses tracked by per-dispatch PID markers under the user-level state dir and classifies each as stale (child already dead — silently cleaned), live (parent still running — left alone), or orphan (child alive, parent dead — prompts y/n to kill). Orphans are what a force-cancelled `implement` run leaves behind: Ctrl+C in the parent terminal, terminal closed, kernel kill, or a panic in a child.

## Methodology assets

`gh optivem` ships its ATDD methodology assets (the per-phase agent prompts and the shared preamble) embedded in the binary. They are fed to the `claude -p` subprocess via argv at dispatch time and are never written to disk in consumer repos — scaffolded projects hold zero ATDD assets locally, and updates propagate simply by upgrading the `gh-optivem` binary.

## Further reading

- [Process diagram](process-diagram.md) — committed Mermaid diagram of the implementation process flow `implement` walks (regenerate with `gh optivem process show`).
- [BPMN process design](bpmn-process-design.md) — the *why* behind the five-level process model (TOP / CYCLE / HIGH / MID / LOW): primitives, doctrine decisions, and the ticket-to-cycle mapping.
- [Architecture diagram](architecture-diagram.md) — committed Mermaid diagram of the ATDD layered architecture (regenerate with `gh optivem architecture show`).
- [How it works](how-it-works.md) — what `gh optivem init` actually does: startup, the phased scaffolding step list, and how `--verify-level` selects the verification steps.
- [Trunk Based Development (TBD)](tbd.md) — how to work with `main` in this repo and the repos it scaffolds.
