# Plan: migrate `github-utils/scripts/*` into `gh-optivem` as native subcommands

> **Status:** all items landed on 2026-05-14 except Step 5a (`test-pipeline-templates`), which is deferred and tracked below as the only remaining work. The migration of `commit`, `sync`, `check-actions`, `rate-limit`, and `cleanup {releases,packages,repos,sonar-projects}` shipped; the bash scripts (except those used by `test-pipeline-templates`) are removed; `~/.claude/commands/{commit,sync,check-actions,…}` and `academy/claude/.claude/commands/{commit,sync,check-actions}.md` now invoke `gh optivem workspace …`; the `feedback_commit_script_tty` memory was removed.

## Context (for the deferred Step 5a)

`academy/github-utils/scripts/test-pipeline-templates.sh` is a 327-line bash operational test for the pipeline templates. It orchestrates parallel stages (commit-stage fan-out → wait → acceptance/QA/signoff/prod per-repo fan-out) against a hard-coded greeter-repo list. It still sources `common.sh` and `gh-retry.sh` (kept in `academy/github-utils/scripts/` solely as its dependencies); these are the last files left under that directory.

## Reuse references

- `gh-optivem/internal/shell/ghretry.go` — already implements the retry policy from `gh-retry.sh`.
- `gh-optivem/internal/shell/github.go` — `RunWithRetry` / rate-limit guard for parallel `gh` calls.
- `gh-optivem/workspace_commands.go` — pattern for adding a new `workspace` subcommand (Cobra group + nested cmd).
- `gh-optivem/internal/workspace.Resolve` — workspace-file folder discovery (already used by `commit`, `sync`, `check-actions`, `rate-limit`).

## Out of scope for Step 5a

- New capabilities. The port is 1:1 with the bash script's behavior — same greeter-repo list, same workflow names, same parallel stage ordering.
- Tombstoning `test-pipeline-templates.sh`, `common.sh`, `gh-retry.sh`: deletion happens once Step 5a ships and a release is cut.

## Steps

- [ ] **Step 5a: Add `gh optivem workspace test-pipeline-templates`** — ⏳ Deferred (2026-05-14). The script is 327 lines with parallel orchestration (commit-stage fan-out → wait → acceptance/QA/signoff/prod per-repo fan-out), not "<50 lines" as the original plan estimated. Needs a dedicated session with goroutines + sync.WaitGroup. Inputs: workspace folders + hard-coded greeter repo list + workflow names. Reuse: `internal/shell.RunWithRetry`, `shell.CheckRateLimit`. Once shipped, delete `scripts/test-pipeline-templates.sh`, `scripts/common.sh`, `scripts/gh-retry.sh`, and update `academy/github-utils/README.md` to drop the "Not yet ported" section.

## Verification

Step 5a is complete when:

1. `gh optivem workspace test-pipeline-templates --help` documents the same inputs and behavior as the bash script.
2. Running `gh optivem workspace test-pipeline-templates` from any directory in the academy tree produces the same per-repo per-workflow pass/fail output as the bash script for the same input set.
3. The parallel orchestration ordering matches: commit-stage triggered for all repos first, then per-repo acceptance/QA/signoff/prod fan-out only after the commit stage completes.
4. `academy/github-utils/scripts/` is empty and `academy/github-utils/README.md` no longer has a "Not yet ported" section.
