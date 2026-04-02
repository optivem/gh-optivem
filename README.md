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
