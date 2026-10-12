# 2026-10-12 10:42:00 UTC — Replace stale `commit.sh` references in embedded Claude commands

## TL;DR

**Why:** Several embedded Claude commands tell the agent to run `bash ".../github-utils/scripts/commit.sh" --repo <name> "<msg>"`. That script no longer exists (`academy/github-utils/` now holds only `LICENSE` and `README.md`; the logic was ported to `gh optivem commit`). `gh optivem claude install` copies these commands to `~/.claude/commands/`, so every user gets instructions pointing at a missing file. They also conflict with the global rule "always use the `/commit` skill, never ad-hoc commands".
**End result:** No embedded command, doc, or code comment references `commit.sh`. Commands that commit use `gh optivem commit --repo <name> "<msg>"` (the same thing `/commit` runs), and `gh optivem claude check` reports no drift after `gh optivem claude setup`.

## Findings already established (so we don't re-derive)

Checked 2026-10-12 with `grep -rn "commit\.sh\|github-utils"` in `gh-optivem`:

| File | Line(s) | Kind |
|---|---|---|
| `internal/claude/assets/commands/execute-plan.md` | 191, 224 | Instruction (Step-by-step Phase 3, Batch Phase 3) |
| `internal/claude/assets/commands/create-plan.md` | 101 | Instruction (commit new plan) |
| `internal/claude/assets/commands/fix-sonar-warnings.md` | 186, 190 | Instruction + a note about `commit.sh --paths` |
| `docs/gh-monitoring-process.md` | 65 | Doc example |
| `cross_repo_commands.go` | 43, 431, 1193, 1209 | Comments ("verbatim from commit.sh", "mirrors commit.sh:201") |
| `internal/devworkflow/workspace/workspace.go` | 98 | Comment ("commit.sh:183 behavior") |
| `cleanup_commands.go`, `internal/devworkflow/ghbulk/ghbulk.go`, `internal/devworkflow/sonar/sonar.go` | header comments | Mention `github-utils/scripts/*.sh` ports |

- `gh optivem commit` signature (`cross_repo_commands.go:102`): `commit [--repo <name>] [--paths "<paths>"] [--yes] [--all] [--include-untracked] ["<message>"]`. `--yes` is required without a TTY.
- `issue-triager.md:16` lists `../github-utils/` as a tooling directory. The directory still exists, so that line is not stale.

## Outcomes

- The three command files above use `gh optivem commit --repo <repo-name> ["--paths ..."] "<message>"` (or the `/commit` skill) instead of `commit.sh`.
- `docs/gh-monitoring-process.md` example updated the same way.
- Comments that cite `commit.sh` line numbers are removed or reworded so each file is self-contained (global rule: no cross-project references in comments).
- After `gh optivem claude setup`, `~/.claude/commands/` has the updated files and `gh optivem claude check` exits 0.

## ▶ Next executable step (resume here)

Step 1: in `internal/claude/assets/commands/execute-plan.md` lines 191 and 224, replace the `bash ".../commit.sh" --repo ...` blocks with the `gh optivem commit --repo <repo-name> "<message>"` form chosen in the open question below. Then repeat in `create-plan.md:101` and `fix-sonar-warnings.md:186-190`.

## Steps

1. **Command files.** Edit `execute-plan.md`, `create-plan.md`, `fix-sonar-warnings.md` as in Outcomes. In `fix-sonar-warnings.md` also reword the `commit.sh --paths` note to `gh optivem commit --paths`.
2. **Docs.** Update `docs/gh-monitoring-process.md:65`.
3. **Comments.** Reword the Go comments listed above so they don't cite `commit.sh` / `sync.sh` line numbers. Comment-only; no behavior change.
4. **Verify.** `go build ./...` and `go test ./...` in `gh-optivem`. `grep -rn "commit\.sh" .` returns nothing outside `plans/`.
5. **Propagate.** Run `gh optivem claude setup`, then `gh optivem claude check` (expect exit 0). Start a new Claude session to load the updated commands.
6. **Commit** with `/commit`.

## Open questions

- In the command files, should the commit instruction be the CLI form (`gh optivem commit --repo <name> "<msg>"`) or "invoke the `/commit` skill"? Recommended: the CLI form with `--repo` (and `--yes` where there is no TTY), because the commands need per-repo scoping and `/commit` is the interactive wrapper over the same primitive; mention `/commit` only where the user is driving.
- Should the comment cleanups (step 3) ride in this plan or be dropped? Recommended: include them; they are one-line rewordings and leave no dangling references to a deleted script.
