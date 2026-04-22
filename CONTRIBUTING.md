# Contributing

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [GitHub CLI](https://cli.github.com/) (`gh auth login`)

## Install from source

```bash
cd gh-optivem
gh extension install .
```

## Build

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
