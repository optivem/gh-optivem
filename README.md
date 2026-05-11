[![gh Commit Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-commit-stage.yml)
[![gh Acceptance Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-acceptance-stage.yml)
[![gh Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-release-stage.yml)
[![gh Post-Release Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-post-release-stage.yml)
[![gh Local Stage](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml/badge.svg)](https://github.com/optivem/gh-optivem/actions/workflows/gh-local-stage.yml)

# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

- [GitHub CLI](https://cli.github.com/) (`gh auth login`) — `gh-optivem` is tested against `gh` ≥ 2.92.0; older releases may work but are unsupported. `gh optivem init` and `gh optivem atdd …` will refuse to run on older versions and point you at the upgrade. Update with `winget upgrade GitHub.cli` (Windows), `brew upgrade gh` (macOS), or your distro's package manager.
- [`actionlint`](https://github.com/rhysd/actionlint) v1.7.7 — used by the `Verify scaffolded workflows` step. Install: `go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.7` or `bash <(curl -fsSL https://raw.githubusercontent.com/rhysd/actionlint/v1.7.7/scripts/download-actionlint.bash) 1.7.7`

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

### Scaffold a monolith project

```bash
gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
    --arch monolith --repo-strategy monorepo --monolith-lang java
```

The repo name can also be passed positionally — `gh optivem init page-turner --owner acme ...` — in which case `--repo` is optional.

### Scaffold a multitier project

```bash
gh optivem init --owner acme --system-name "Page Turner" --repo page-turner \
    --arch multitier --repo-strategy multirepo \
    --backend-lang java --frontend-lang react
```

### Other init flags

```bash
gh optivem init ... --license apache-2.0          # mit (default), apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense
gh optivem init ... --test-lang typescript         # override; defaults to --monolith-lang or --backend-lang
gh optivem init ... --workdir ./scratch            # custom clone dir; defaults to a temp dir
gh optivem init ... --shop-ref meta-v1.2.3         # pin optivem/shop to a tag, SHA, or branch
gh optivem init ... --no-atdd                      # skip installing ATDD agents/commands/prompts from shop
```

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

`gh optivem` also provides runner subcommands for working with the system tests in a scaffolded project. Inspired by `dotnet test` and `./gradlew test`, `test system` builds and starts the system implicitly — you don't need to run `build` or `run` first.

```bash
gh optivem test system                       # build (incremental) + start (if needed) + run tests
gh optivem test system --no-build            # skip the explicit build step
gh optivem test system --rebuild             # force full rebuild in the implicit build step (ignored with --no-build)
gh optivem test system --no-start            # skip the start step (system must already be up)
gh optivem test system --restart             # force tear-down + restart before tests
gh optivem test system --suite smoke         # run only the suite with this id
gh optivem test system --suite acceptance-api --suite acceptance-ui   # multiple suites, repeatable
gh optivem test system --suite acceptance-api,acceptance-ui           # ...or comma-separated
gh optivem test system --test "MyTest"       # narrow execution to one test name (substituted into the suite's testFilter)
gh optivem test system --test T1 --test T2   # multiple names, repeatable
gh optivem test system --test T1,T2          # ...or comma-separated
gh optivem test system --sample              # use each suite's sampleTest field as the test name

gh optivem build system                      # docker compose build for every entry in systems.yaml
gh optivem build system --rebuild            # force full rebuild (no layer cache reuse)

gh optivem run system                        # docker compose up + wait for health
gh optivem run system --restart              # force tear-down + restart
gh optivem run system --log-lines 200        # lines of compose logs to dump on health-probe failure (default 50)

gh optivem stop system                       # docker compose down + container cleanup
gh optivem clean system                      # docker compose down -v --rmi local (delete volumes + locally-built images)
```

All runner subcommands accept `--system-config <path>` and `test system` additionally accepts `--test-config <path>` for projects where these files live elsewhere. The paths are resolved through three knobs in ascending order of permanence — flag → `gh-optivem.yaml` field → built-in default. See [Pointing at non-default configs](#pointing-at-non-default-configs) below.

Multi-test semantics depend on the suite's `testFilter` in `tests.yaml`. The runner combines multiple `--test` values per `testFilterJoin`: `"or"` (default) joins names with `|` and substitutes once — works for dotnet (`&DisplayName~T1|T2`) and playwright/jest (`--grep 'T1|T2'`); `"repeat"` substitutes the whole `testFilter` once per name and concatenates — required for gradle (`--tests T1 --tests T2`). Practical ceiling on Windows is ~600 typical test names per invocation (the OS caps each command line at 32K characters).

`clean system` is the analog of `dotnet clean` / `./gradlew clean` — it deletes build outputs (containers, named volumes, locally-built images) without touching the dependency cache (registry-pulled images are kept). Chain it explicitly for a fresh start: `gh optivem clean system && gh optivem test system`.

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

For the canonical reference, see the four sample configs (mono-repo × multi-repo crossed with monolith × multitier) in [`plans/20260505-100000-scope-paths-and-implement-ticket-preflight.md`](plans/20260505-100000-scope-paths-and-implement-ticket-preflight.md).

### Pointing at non-default configs

Three knobs decide which `gh-optivem.yaml` / `systems.yaml` / `tests.yaml` the tool reads, in ascending order of permanence. Each knob overrides everything below it:

```bash
# 1. One-shot flag (highest precedence)
gh optivem -c ./gh-optivem.shop-monolith.yaml test system \
    --system-config docker/java/monolith/systems.yaml \
    --test-config system-test/java/tests-latest.yaml

# 2. Shell-session env var (for gh-optivem.yaml only)
export GH_OPTIVEM_CONFIG=./gh-optivem.shop-monolith.yaml
gh optivem test system

# 3. Per-project default baked into gh-optivem.yaml (lowest precedence)
system_config: docker/systems.yaml
test_config:   system-test/tests-latest.yaml
```

Legacy `.json` files still work — the loader picks the parser from the file extension, and any in-flight repo carrying `systems.json` / `tests-latest.json` keeps loading without changes.

`gh optivem init` auto-populates `system_config:` / `test_config:` to the paths it produces, so freshly scaffolded repos work without any flags. `gh optivem config init` (hand-rolled repos) leaves both fields empty — add them once your layout is settled and drop the per-command flags.

## How it works

See [docs/how-it-works.md](docs/how-it-works.md) for a detailed walkthrough of the `main.go` logic, setup steps, and verification levels.

For the ATDD pipeline orchestration view, see the rendered [process diagram](docs/process-diagram.md). It is regenerated automatically whenever the canonical YAML at `internal/atdd/runtime/statemachine/process-flow.yaml` changes; do not edit the diagram by hand.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing, and release instructions.
