# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [GitHub CLI](https://cli.github.com/) (`gh auth login`)

## Installation

```bash
gh extension install optivem/gh-optivem
```

To install from a local clone instead:

```bash
cd gh-optivem
gh extension install .
```

## Version

Check the installed version:

```bash
gh optivem --version
```

## Upgrading

Upgrade to the latest release:

```bash
gh extension upgrade optivem
```

Upgrade all extensions at once:

```bash
gh extension upgrade --all
```

## Releasing

This project uses [semantic versioning](https://semver.org/). To create a new release:

```bash
git tag v1.2.3
git push origin v1.2.3
```

This triggers the Release workflow, which uses GoReleaser to build binaries for all platforms and publish a GitHub Release. Users who installed via `gh extension install` will get the new version on their next `gh extension upgrade`.

## Usage

Monolith:

```bash
gh optivem --owner acme --system-name "Page Turner" --repo page-turner \
    --arch monolith --repo-strategy monorepo --lang java
```

Multitier:

```bash
gh optivem --owner acme --system-name "Page Turner" --repo page-turner \
    --arch multitier --repo-strategy multirepo \
    --backend-lang java --frontend-lang react
```

Dry run:

```bash
gh optivem ... --dry-run
```

Test mode (scaffolds then cleans up automatically):

```bash
gh optivem ... --test --cleanup --random-suffix
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
export OPTIVEM_STARTER_PATH=/path/to/starter

go test -tags=system ./internal/config/ -v -timeout 2h \
    -run "TestValidMonolithConfigurations/monolith_monorepo_java_dotnet"
```

### Acceptance Stage Monitoring Process

When working on gh-optivem, follow this loop to verify changes:

1. **Trigger** the acceptance stage workflow via GitHub Actions (`workflow_dispatch`).
2. **Monitor** the run (check every 5 minutes).
3. **If a job fails:**
   - Investigate the failure logs (`gh run view <id> --log-failed`).
   - Fix the issue locally.
   - Run **only the one failing test** locally (see above for single-test syntax). Do not run the full suite.
   - Repeat fix-and-test until that test passes locally.
4. **Commit** the fix (use the `/commit` skill).
5. **Re-trigger** the acceptance stage and go back to step 2.
6. **Repeat** until the acceptance stage passes.

**Stop condition:** If a test fails due to an external issue not under your control (e.g. subscription limits, third-party service outage, rate limiting), stop the loop and wait for the user to investigate.
