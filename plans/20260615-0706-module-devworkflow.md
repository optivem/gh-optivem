# 2026-06-15 07:06 UTC — Module extraction: dev-workflow

**Child of** `20260615-0548-gh-optivem-modular-monolith-parent.md`. First (easiest) module cut in the modular-monolith decomposition.

## What changed

Physically nested the dev-workflow packages under a new `internal/devworkflow/` parent (pure move + import-path updates; no logic changes):

- `internal/ghbulk` → `internal/devworkflow/ghbulk`
- `internal/sonar` → `internal/devworkflow/sonar`
- `internal/workspace` → `internal/devworkflow/workspace`

Import paths updated in 4 root files (CLI surface, which stays put): `main.go`, `cross_repo_commands.go`, `cleanup_commands.go`, `root_cmd_test.go`.

## Verification

- Baseline before move: `go build ./...` ✓, `go test ./...` ✓ (all green).
- After move: `go build ./...` ✓, `go vet ./internal/devworkflow/...` ✓, `go test ./...` ✓ (no new failures).

## Status

Done. This module had the lowest coupling (only the shared kernel + `projectconfig`), so it was a clean cut with no seam inversion required.
