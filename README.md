# gh-optivem

A GitHub CLI extension for scaffolding pipeline projects.

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [GitHub CLI](https://cli.github.com/) (`gh auth login`)

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
