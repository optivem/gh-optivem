# Contributing

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [GitHub CLI](https://cli.github.com/) (`gh auth login`)

## Shop version in local builds

Local builds (`go run .`, `go build`, `gh extension install .`) have no shop ref baked in, so `gh optivem init` resolves `--shop-tag` as follows:

1. `--shop-tag vX.Y.Z` — use that exact tag (refuses `main`/`master`).
2. Otherwise — fetch the **latest `meta-v*` release** from `optivem/shop` via `gh api` and use that.

Released binaries (`gh extension install optivem/gh-optivem`) are pinned to the shop SHA baked in at release time and do **not** auto-upgrade to the latest `meta-v*` release.

For reproducible local testing, pass `--shop-tag meta-vX.Y.Z` explicitly.

## Run locally

Fastest iteration — compiles and runs in one step, no install needed:

```bash
go run . --version

# Dry-run a monolith scaffold (no side effects):
go run . init --owner YOUR_GH_USER --system-name "Page Turner" --repo page-turner \
    --arch monolith --repo-strategy monorepo --lang java --dry-run

# Dry-run with test mode + random suffix (preview repeated-run workflow):
go run . init --owner YOUR_GH_USER --system-name "Page Turner" --repo page-turner \
    --arch monolith --repo-strategy monorepo --lang java \
    --test --cleanup --random-suffix --dry-run
```

Replace `YOUR_GH_USER` with your GitHub username. Avoid `<angle-brackets>` in bash — the shell treats them as I/O redirection. Also avoid reserved words like `Test`, `Local`, `System` in `--system-name` (see [`isScaffoldReserved`](internal/config/config.go#L312) for the full list).

See [README.md](README.md#usage) for the full flag set and multitier examples.

## Install from source

Installs your local working copy as the `gh optivem` extension (replaces any previously-installed version). Use this when you want to invoke it as `gh optivem ...` — otherwise prefer `go run .`:

```bash
cd gh-optivem
gh extension install .
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

## Releasing

This project uses [semantic versioning](https://semver.org/). To create a new release:

```bash
git tag v1.2.3
git push origin v1.2.3
```

This triggers the Release workflow, which uses GoReleaser to build binaries for all platforms and publish a GitHub Release. Users who installed via `gh extension install` will get the new version on their next `gh extension upgrade`.
