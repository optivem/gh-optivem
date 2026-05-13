[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

### GitHub CLI

[GitHub CLI](https://cli.github.com/) (`gh auth login`).

- Check your version: `gh --version`
- Install: `winget install GitHub.cli` (Windows), `brew install gh` (macOS), or `sudo apt install gh` / your distro's package manager (Linux)
- Upgrade: `winget upgrade GitHub.cli` (Windows), `brew upgrade gh` (macOS), or `sudo apt upgrade gh` / your distro's package manager (Linux)

### actionlint

[`actionlint`](https://github.com/rhysd/actionlint) — used by the `Verify scaffolded workflows` step.

- Check your version: `actionlint -version`
- Install or upgrade to the latest v1 release: `go install github.com/rhysd/actionlint/cmd/actionlint@v1`

### Required environment variables

`gh optivem init` reads six variables from your shell and writes them as Actions variables/secrets on the scaffolded repo (and on each component repo in multirepo). The scaffolded pipeline can't run without them, so the tool fails fast if any are missing. Pass `--dry-run` to skip the check.

- `DOCKERHUB_USERNAME` — your Docker Hub username. The scaffolded pipeline authenticates pulls of base images from Docker Hub so they don't hit the anonymous rate limit (shared CI runner IPs burn through it quickly).
- `DOCKERHUB_TOKEN` — a Docker Hub Personal Access Token paired with the username above. Same reason: authenticated pulls. Create at https://app.docker.com/settings/personal-access-tokens (read-only scope is enough since we only pull).
- `SONAR_TOKEN` — a SonarCloud token. Consumed by the local SonarCloud scan step (`--verify-level local`+) and by the commit stage's CI scan.
- `GHCR_TOKEN` — a GitHub Personal Access Token (classic) with `write:packages` + `read:packages`. The acceptance and prod stages tag images in GHCR. Create at https://github.com/settings/tokens.
- `WORKFLOW_TOKEN` — a GitHub PAT (classic) with `repo` + `workflow` scopes. The acceptance/QA/prod stages push release tags; the default `GITHUB_TOKEN` cannot push tags whose commits diff workflow files, which is why a separate PAT is needed.
- `REPO_TOKEN` — a GitHub PAT with `repo` scope, or a fine-grained PAT with `Contents:Read` on the component repos. In multitier+multirepo scaffolds, the system-level prod stage uses it to read each component repo's `VERSION` file via the GitHub API (cross-repo Contents read). Currently required for all scaffolds even though only multitier+multirepo consumes it.

```bash
export DOCKERHUB_USERNAME=...
export DOCKERHUB_TOKEN=...
export SONAR_TOKEN=...
export GHCR_TOKEN=...
export WORKFLOW_TOKEN=...
export REPO_TOKEN=...
```

## Installation

```bash
gh extension install optivem/gh-optivem
```

## Uninstalling

```bash
gh extension remove optivem
```

## Version

```bash
gh optivem --version
```

## Upgrading

```bash
gh optivem upgrade
```

Equivalent to `gh extension upgrade optivem` — either form works.

## Usage

Scaffolding is **two-phase**. First write `gh-optivem.yaml` — the file that carries every project-stable value (owner, repo, arch, langs, system name, license, deploy target, tier paths, project URL). Then run `gh optivem init` to create the GitHub repo(s) and apply the template.

### 1) Write `gh-optivem.yaml`

```bash
gh optivem config init --owner acme --repo page-turner --system-name "Page Turner" \
    --arch monolith --repo-strategy monorepo --monolith-lang java \
    --project-url https://github.com/orgs/acme/projects/1 \
    --system-path system --system-test-path system-test \
    --stubs-path external-systems/external-stub \
    --simulators-path external-systems/external-real-sim
```

Multitier:

```bash
gh optivem config init --owner acme --repo page-turner --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang java --frontend-lang typescript \
    --project-url https://github.com/orgs/acme/projects/1 \
    --backend-path backend --frontend-path frontend \
    --system-test-path system-test \
    --stubs-path external-systems/external-stub \
    --simulators-path external-systems/external-real-sim
```

Or run `gh optivem config init` interactively when the file is missing — `gh-optivem` prompts for owner/repo (auto-inferred from `git remote origin` when available), system-name, arch, repo-strategy, lang, and project-url; everything else is defaulted.

Review the generated `gh-optivem.yaml`, hand-edit if needed, then run `gh optivem config validate` to confirm. Once the sibling repos are cloned (multi-repo layouts) and the tier paths actually exist on disk, run `gh optivem config preflight` for the stronger "I'm about to run this for real" check — same schema validation plus an on-disk layout check that every declared repo and tier path resolves to a real directory. `preflight` is the same check `atdd implement-ticket` runs at startup.

### 2) Scaffold

```bash
gh optivem init
```

No flags needed — every project-stable value comes from `gh-optivem.yaml`. The `init` command accepts only per-invocation flags (dry-run, workdir, verify-level, no-*, log-file, …).

### Dry run

```bash
gh optivem init ... --dry-run
```

### Verification level

Control how deep pipeline verification goes after scaffolding:

```bash
gh optivem init ... --verify-level none          # skip all verification
gh optivem init ... --verify-level local         # local compilation + local tests + local SonarCloud scan (no CI)
gh optivem init ... --verify-level commit        # + commit stage CI
gh optivem init ... --verify-level acceptance    # + acceptance stage CI (latest + legacy in parallel)
gh optivem init ... --verify-level qa            # + QA stage + QA signoff
gh optivem init ... --verify-level release       # + production stage (default)
gh optivem init ... --no-legacy                  # skip legacy in local tests and acceptance
gh optivem init ... --no-local-tests             # skip the local system-tests step
gh optivem init ... --no-local-sonar             # skip the local SonarCloud scan step
gh optivem init ... --no-atdd                    # skip installing ATDD agents/commands/prompts from shop
gh optivem init ... --no-project                 # skip the "Ensure project board" step (no auto-create, no status-ensure)
```

### Pinning the shop template

By default `gh optivem init` clones the latest `meta-v*` release of `optivem/shop`. Pin a specific ref (tag, SHA, or branch) with `--shop-ref` — useful when reproducing a past scaffold or testing an unreleased shop change:

```bash
gh optivem init ... --shop-ref meta-v1.2.3
gh optivem init ... --shop-ref main
gh optivem init ... --shop-ref a1b2c3d
```

### Local cleanup

On a successful run the local scaffold dir is deleted — the end result is just the created GitHub repo(s) + SonarCloud project(s), which you can clone later. Pass `--keep-local` to keep the dir (e.g. for inspection). On failure the dir is always kept so the broken scaffold can be debugged.

### Unattended runs (CI)

Pass `--yes` (or `-y`) to skip all interactive confirmations — the existing-repo prompt and the `--report-bug` confirmation. This is the expected pattern for CI/automation:

```bash
gh optivem init ... --yes
```

### Logging flags

Available on `init`:

```bash
gh optivem init ... --verbose          # -v; debug output (retry/wait chatter, diagnostics)
gh optivem init ... --quiet            # -q; suppress info-level output (warnings + errors still shown)
gh optivem init ... --log-file run.log # also write a plain-text log (no ANSI colors, all levels)
```

### Deployment target

Only `--deploy docker` is currently supported (the default). `--deploy cloud-run` is in development and may be available in a future release.

### Running tests against a scaffolded project

`gh optivem` also provides runner subcommands for working with the system tests in a scaffolded project. Each lifecycle phase is its own verb (mirrors `docker compose`, `systemctl`, `kubectl`, `terraform`): the typical sequence is `test setup` (prepare the test harness — `npm ci`, compile test sources, etc.) → `system start` (bring the SUT up) → `test run` (run suites) → `system stop`.

```bash
gh optivem test setup                     # run setupCommands from tests.yaml (npm ci, restore, compile test sources, ...)

gh optivem test run                       # run every suite against the already-running system
gh optivem test run --suite smoke         # run only the suite with this id
gh optivem test run --suite acceptance-api --suite acceptance-ui   # multiple suites, repeatable
gh optivem test run --suite acceptance-api,acceptance-ui           # ...or comma-separated
gh optivem test run --test "MyTest"       # narrow execution to one test name (substituted into the suite's testFilter)
gh optivem test run --test T1 --test T2   # multiple names, repeatable
gh optivem test run --test T1,T2          # ...or comma-separated
gh optivem test run --sample              # use each suite's sampleTest field as the test name
gh optivem test run --list                # print suite ids from tests.yaml and exit

gh optivem system build                   # docker compose build for every entry in systems.yaml
gh optivem system build --rebuild         # force full rebuild (no layer cache reuse)

gh optivem system start                   # docker compose up + wait for health
gh optivem system start --restart         # force tear-down + restart
gh optivem system start --log-lines 200   # lines of compose logs to dump on health-probe failure (default 50)
gh optivem system start --up-timeout 10m  # per-attempt timeout for `docker compose up -d` (default 5m)

gh optivem system stop                    # docker compose down + container cleanup
gh optivem system clean                   # docker compose down -v --rmi local (delete volumes + locally-built images)

gh optivem compile                        # source-level compile of system + test tiers (halts on first failure)
gh optivem system compile                 # source-level compile of the system tier only
gh optivem test compile                   # source-level compile of the test tier only
```

Naming: `compile` is the source-level build (`dotnet build` / `./gradlew compileJava` / `npx tsc --noEmit`), dispatched per-tier by the `lang:` field in `gh-optivem.yaml`. `system build` is reserved for `docker compose build` (container image build). The two are distinct and must not be conflated.

`test run` health-probes every entry in `systems.yaml` first; if any aren't up, it errors out with "start it first with `gh optivem system start`" rather than silently starting them. Chain the verbs explicitly:

```bash
gh optivem test setup
gh optivem system start
gh optivem test run --suite smoke
gh optivem test run --suite acceptance-api
gh optivem system stop
```

The paths to `systems.yaml` / `tests.yaml` come from `gh-optivem.yaml`'s `system.config:` / `system_test.config:` fields — both are required by the runner commands (there is no built-in default-name fallback). Projects with non-default layouts (e.g. `docker/java/monolith/systems.yaml`) set the YAML fields once and forget; to pick an alternate variant ad hoc, select a different `gh-optivem.yaml` via the persistent `-c` / `--config` flag. See [Pointing at non-default configs](#pointing-at-non-default-configs) below.

Multi-test semantics depend on the suite's `testFilter` in `tests.yaml`. The runner combines multiple `--test` values per `testFilterJoin`: `"or"` (default) joins names with `|` and substitutes once — works for dotnet (`&DisplayName~T1|T2`) and playwright/jest (`--grep 'T1|T2'`); `"repeat"` substitutes the whole `testFilter` once per name and concatenates — required for gradle (`--tests T1 --tests T2`). Practical ceiling on Windows is ~600 typical test names per invocation (the OS caps each command line at 32K characters).

`system clean` is the analog of `dotnet clean` / `./gradlew clean` — it deletes build outputs (containers, named volumes, locally-built images) without touching the dependency cache (registry-pulled images are kept). Chain it explicitly for a fresh start: `gh optivem system clean && gh optivem test run`.

### Running the ATDD pipeline

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

### Verifying tokens

Before kicking off a CI matrix that fans out to every architecture × language combination, run a single up-front auth check:

```bash
gh optivem token verify
```

Reads `DOCKERHUB_USERNAME` / `DOCKERHUB_TOKEN` / `SONAR_TOKEN` / `GHCR_TOKEN` / `WORKFLOW_TOKEN` / `REPO_TOKEN` from the environment, runs a live auth call against each provider in parallel, and exits non-zero with an aggregated list of every missing or rejected credential. Read-only — no repos, secrets, or releases are mutated.

## Troubleshooting

### Auto-filed bug report (opt-in)

If you want the failure auto-filed to `optivem/gh-optivem` as an issue — including scaffold config — opt in with `--report-bug`:

```bash
gh optivem init ... --report-bug
```

Off by default. Filing a quick issue yourself is usually clearer and keeps the scaffold config private unless you decide to share it.

## Project config (`gh-optivem.yaml`)

Every scaffolded repo gets a `gh-optivem.yaml` at its root. The file declares five top-level keys:

- `project:` — the GitHub Projects board URL.
- `repo_strategy:` — `mono-repo` or `multi-repo`.
- `system:` — the system being built. Polymorphic by architecture: under `monolith`, `system:` carries flat `path:` / `repo:` / `lang:` directly; under `multitier`, it nests `backend:` and `frontend:` blocks (each with its own per-component language).
- `system_test:` — the acceptance-test suite that drives the system. Top-level (not nested under `system:`) because tests aren't part of the system; they drive it.
- `external_systems:` (optional) — vendored stand-ins for third-party dependencies. `stubs:` is the cycle-2 WireMock-style pattern; `simulators:` is the cycle-3 real-sim pattern.

Every populated tier carries the same `path:` (repo-relative) and `repo:` (slug from the participating repos) pair; system-tier blocks additionally carry `lang:`. The runtime preflight on `gh optivem atdd implement-ticket` validates that every declared path exists on disk before any agent runs, so a config / layout mismatch fails fast with a readable error rather than mid-pipeline.

For the canonical schema, see [`internal/projectconfig/config.go`](internal/projectconfig/config.go) — every YAML field is declared on the `Config` struct with its `yaml:` tag, and the `Validate` method spells out the cross-field rules (architecture exclusivity, repo-strategy consistency, per-tier completeness, SonarCloud presence).

### Pointing at non-default configs

`gh-optivem.yaml` is the single entry point for every `gh optivem` command — there is no default-name fallback for `systems.yaml` / `tests.yaml`. Three knobs decide *which* `gh-optivem.yaml` the tool reads, in ascending order of precedence — each overrides the one below:

```bash
# 1. One-shot flag (highest precedence) — selects which gh-optivem.yaml to read
gh optivem -c ./gh-optivem.shop-monolith.yaml test run

# 2. Shell-session env var (same role as --config)
export GH_OPTIVEM_CONFIG=./gh-optivem.shop-monolith.yaml
gh optivem test run

# 3. Default location: ./gh-optivem.yaml in the current working directory
gh optivem test run
```

Inside the selected `gh-optivem.yaml`, `system.config:` / `system_test.config:` point at the actual systems/tests config files:

```yaml
system:
  config: docker/systems.yaml
system_test:
  config: system-test/tests.yaml
```

Legacy `.json` files still work — the loader picks the parser from the file extension, and any in-flight repo carrying `systems.json` / `tests.json` keeps loading without changes.

`gh optivem init` auto-populates `system.config:` / `system_test.config:` to the paths it produces, so freshly scaffolded repos work without any flags. `gh optivem config init` (hand-rolled repos) leaves both fields empty — add them before invoking the runner commands.

If no `gh-optivem.yaml` is found, the runner commands hard-error with a hint pointing at `gh optivem config init` (to create one in place) and at `--config <path>` (to use one that lives elsewhere). If `gh-optivem.yaml` is present but `system.config:` / `system_test.config:` is unset, the runner commands hard-error pointing at the missing field plus the same `--config` escape hatch.

#### ATDD-specific overrides

The ATDD pipeline (`gh optivem atdd implement-ticket` / `manage-project`) reads four optional override fields from the same `gh-optivem.yaml`:

```yaml
process_flow: config/process-flow.yaml         # alternate process-flow YAML (default: embedded)
agent_prompts:                                  # swap one or more embedded agent prompts
  atdd-test: config/prompts/atdd-test.md
node_extras:                                    # appended to a node's prompt at dispatch
  AT_RED_DSL_WRITE: prefer record types
node_replacements:                              # replaces a node's prompt verbatim with this file body
  AT_RED_TEST_WRITE: config/prompts/at-red-test-write.md
```

All four fields are optional; absent means "use the embedded default." To experiment without committing a change to the project's `gh-optivem.yaml`, copy it to a side file and pass `--config ./gh-optivem.experimental.yaml`. There is no per-invocation flag for any of these — they are project-stable values by design.

## How it works

See [docs/how-it-works.md](docs/how-it-works.md) for a detailed walkthrough of the `main.go` logic, setup steps, and verification levels.

For the ATDD pipeline orchestration view, see the rendered [process diagram](docs/process-diagram.md). It is regenerated automatically whenever the canonical YAML at `internal/atdd/runtime/statemachine/process-flow.yaml` changes; do not edit the diagram by hand.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and release instructions.
