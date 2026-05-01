# Contributing

## ATDD Testing

valen_4rjvn9e@Valentina_Desk MINGW64 /c/GitHub/optivem/academy/shop (main)


cd shop
./scripts/atdd-rehearsal-start.sh atdd-cli
cd ..
cd rehearsal-atdd-cli
go -C ../gh-optivem build -o gh-optivem.exe .
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue https://github.com/optivem/shop/issues/61
./scripts/atdd-rehearsal-end.sh atdd-cli

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [GitHub CLI](https://cli.github.com/) (`gh auth login`)

## Shop version in local builds

Local builds (`go run .`, `go build`, `gh extension install .`) have no shop ref baked in, so `gh optivem init` resolves `--shop-ref` as follows:

1. `--shop-ref <ref>` — use that exact ref (tag, SHA, or branch — e.g. `meta-v1.2.3`, `main`, `a1b2c3d`).
2. Otherwise — fetch the **latest `meta-v*` release** from `optivem/shop` via `gh api` and use that.

Released binaries (`gh extension install optivem/gh-optivem`) are pinned to the shop SHA baked in at release time and do **not** auto-upgrade to the latest `meta-v*` release. Users can still override with `--shop-ref`, but the default (baked-in SHA) is what you want in almost all cases.

For reproducible local testing, pass `--shop-ref meta-vX.Y.Z` explicitly. To test scaffolding against unreleased shop changes, pass `--shop-ref main` (or a specific SHA).

## Run locally

Fastest iteration — compiles and runs in one step, no install needed:

```bash
go run . --version

# Dry-run a monolith scaffold (no side effects):
go run . init --owner valentinajemuovic --system-name "Page Turner" --repo page-turner \
    --arch monolith --repo-strategy monorepo --monolith-lang java --dry-run
```

For a real manual test run (random repo name; on success the local dir is deleted by init and the GitHub repos + Sonar projects are deleted by scripts/cleanup-orphans.sh; on failure everything is kept for debugging):

```bash
# FULL
bash scripts/manual-test.sh --no-cleanup --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang react --test-lang typescript \
    --shop-ref main

bash scripts/manual-test.sh --no-cleanup --owner valentinajemuovic --system-name "Page Turner" \
    --arch monolith --repo-strategy monorepo \
    --monolith-lang typescript --test-lang typescript \
    --shop-ref main


# SKIP LOCAL & LEGACY (MONOLITH)
bash scripts/manual-test.sh --no-cleanup --owner valentinajemuovic --system-name "Page Turner" \
    --arch monolith --repo-strategy monorepo \
    --monolith-lang typescript --test-lang typescript \
    --shop-ref main \
    --no-local-tests --no-local-sonar --no-legacy


# SKIP LOCAL & LEGACY (MULTIREPO)
bash scripts/manual-test.sh --no-cleanup --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang react --test-lang typescript \
    --shop-ref main \
    --no-local-tests --no-legacy





# Keep the repo after a successful run (for inspection):
bash scripts/manual-test.sh --no-cleanup --owner valentinajemuovic ...
```

See [README.md](README.md#usage) for the full flag set and multitier examples.

## Install from source

Installs your local working copy as the `gh optivem` extension (replaces any previously-installed version). Use this when you want to invoke it as `gh optivem ...` — otherwise prefer `go run .`:

```bash
cd gh-optivem
gh extension install .
```

## Test the released version (real-user flow)

Smoke-test what an end user actually gets — useful after publishing a release, or to reproduce a user-reported issue against the same binary they're running. Unlike `go run .` or `gh extension install .` (which use whatever is on disk), this exercises the released binary and the shop SHA baked in at release time.

```bash
# Install the published extension (or upgrade if already installed)
gh extension install optivem/gh-optivem
# gh optivem upgrade

# Confirm the version matches the latest release
gh optivem --version

# Run the same scaffold a user would (no --shop-ref — uses the baked-in SHA)
gh optivem init --owner valentinajemuovic --system-name "Page Turner" --repo page-turner \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang react --test-lang typescript

# Clean up the extension when done
gh extension remove optivem
```

## Build

Produces a standalone `gh-optivem` binary. Not needed for local testing (`go run .` handles that) — use this only to ship an artifact or sanity-check that the code compiles:

```bash
go build ./...
```

## Running Tests

### Unit tests

```bash
go test ./... -v
```

### System tests

Run all system tests:

```bash
go test -tags=system ./... -v
```

Run a quick subset locally:

```bash
bash scripts/test-system.sh
```

Run a single test locally (e.g. monolith monorepo java dotnet):

```bash
export TEST_OWNER=valentinajemuovic
export DOCKERHUB_USERNAME=valentinajemuovic
export DOCKERHUB_TOKEN=...
export SONAR_TOKEN=...
export GHCR_TOKEN=...
export WORKFLOW_TOKEN=...
go test -tags=system ./internal/config/ -v -timeout 2h \
    -run "TestValidMonolithConfigurations/monolith_monorepo_java_dotnet"
```

## Testing the ATDD driver

The ATDD driver walks `docs/atdd/process/process-flow.yaml` against a real GitHub issue, dispatching service tasks inline. At each user-task node it shells out to the `claude` CLI (auto-launching the matching Claude Code subagent in your terminal); when the subprocess exits and a fresh commit lands on HEAD, the engine advances. The YAML is read from the **current working directory**, so smoke-tests run from inside a consumer repo (typically `shop`), not from `gh-optivem`.

The dispatch is interactive by default — Claude Code's full UI runs in your terminal so you can observe tool calls and interject in chat. Use `--autonomous` to run agents headless via `claude -p`, or `--manual-agents` to fall back to the v1 two-window workflow (driver pauses, you launch the agent in a separate Claude Code session, press Enter to advance). `--manual-agents` is the right choice when you want to bisect "did v2 misroute the agent?" against "did v1 see the commit?".

### Smoke-test against a single ticket

`go run` won't work from the consumer repo (it needs a module in cwd), so the iteration loop is build-once-then-invoke. `go -C` builds in `../gh-optivem` without moving your shell, and the resulting binary, when run from `shop`, sees `shop` as its cwd:

```bash
cd ../shop

# Build (rerun after any gh-optivem code change):
go -C ../gh-optivem build -o gh-optivem.exe .

# Run (accepts bare number or full GitHub issue URL):
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue https://github.com/optivem/shop/issues/42

# Pick the top Ready item from the project board instead of supplying --issue:
../gh-optivem/gh-optivem.exe atdd manage-project

# Disambiguate the project explicitly when multiple match the repo:
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42 \
    --project https://github.com/orgs/optivem/projects/20
```

Or, for the closer-to-real-user flow, install once as a gh extension and use `gh optivem` directly (rerun the install after code changes):

```bash
(cd ../gh-optivem && gh extension install --force .)
cd ../shop
gh optivem atdd implement-ticket --issue 42
```

The driver pre-resolves the project item, moves it to **In Progress**, then walks the flow node by node, auto-dispatching each agent via the `claude` CLI. `--autonomous` skips human-approval STOPs and runs agents headless. `--manual-agents` reverts to v1 two-window dispatch. `--extra NODE="text"` and `--replace NODE="text"` (both repeatable) shape the prompt for a specific YAML node ID; `--interactive` previews the constructed prompt and reads stdin for last-minute additions before each dispatch.

```bash
# Tune one phase's prompt without editing the template:
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42 \
    --extra AT_RED_DSL_WRITE="prefer record types"

# Diagnose a misroute by reverting to v1 manual dispatch:
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42 --manual-agents
```

### Debug a single phase in isolation

The hidden `gh optivem atdd debug …` subcommands exercise individual runtime packages standalone — useful for reproducing one phase without rerunning the whole pipeline. Flag shapes here are not part of the stable API.

```bash
# What would manage-project pick? (no move)
gh optivem atdd debug pick-top-ready

# Classify a ticket via the deterministic fast path:
gh optivem atdd debug classify --issue 42

# Which edge would nextEdge pick from GATE_DSL under a synthetic state?
gh optivem atdd debug next-phase --node GATE_DSL --state dsl_interface_changed=true

# Evaluate one gateway binding standalone:
gh optivem atdd debug gate dsl_changed

# Replay the release primitives (regex strip + commit + close), dry-run first:
gh optivem atdd debug release --issue 42 --dry-run
```

Run `gh optivem atdd debug --help` to list them all.

### Running ATDD on CI

The default v2 dispatcher shells out to the `claude` CLI for every user-task agent dispatch. The CLI looks up credentials from `~/.claude/` — that directory is empty in a fresh CI runner unless someone has already authenticated as the executing user. Without this, the failure surface is a confusing `clauderun: <agent> exited non-zero: ...` deep into the first dispatch.

The driver runs a pre-flight `claude --no-update-check --version` at startup (skipped under `--manual-agents`) so this class of failure surfaces before any flow-walking work happens. If you see `claude CLI pre-flight failed`, the binary is missing or unauthenticated — fix one of the bootstrap paths below before re-running.

Bootstrap options:

- **Bake credentials into a CI image.** Run `claude /login` once locally as the user the image will execute as, then copy `~/.claude/` into the image at build time. Simplest path for a self-hosted runner.
- **Mount credentials at job start.** Store the contents of `~/.claude/` (typically `credentials.json`) as an encrypted secret and write it before invoking `gh optivem atdd …`:

  ```bash
  mkdir -p ~/.claude
  printf '%s' "$CLAUDE_CREDENTIALS" > ~/.claude/credentials.json
  chmod 600 ~/.claude/credentials.json
  ```

- **Fall back to `--manual-agents`.** When credentials aren't available, use the v1 two-window workflow — the driver pauses, a human launches each agent in a separate Claude Code session, then presses Enter to advance. This bypasses the CLI subprocess entirely and is the right choice when you want CI to walk the gates / actions but not the agent dispatches.

Rate-limit / quota failures during a long autonomous run surface as `rate limit hit on Claude subscription; weekly cap likely exhausted — re-run after the next reset window or upgrade your plan`. Mid-run credential expiry surfaces as `claude CLI is not authenticated — run `claude /login` …`. Both are detected from the runner's stderr signature.

## Releasing

This project uses [semantic versioning](https://semver.org/). To create a new release:

```bash
git tag v1.2.3
git push origin v1.2.3
```

This triggers the Release workflow, which uses GoReleaser to build binaries for all platforms and publish a GitHub Release. Users who installed via `gh extension install` will get the new version on their next `gh extension upgrade`.
