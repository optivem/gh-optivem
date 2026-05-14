# Plan: migrate `github-utils/scripts/*` into `gh-optivem` as native subcommands

> **Status: DEFERRED.** This is a larger change than fits the current cycle — postponed pending dedicated time. Do not start execution without re-confirming scope with the user.
>
> Step 1 (`internal/workspace/` package + tests) was carved out and landed on 2026-05-14 (see `internal/workspace/`). Steps 2–13 remain deferred.

## Context

`academy/github-utils/scripts/` hosts bash scripts the user runs daily:

| Script | What it does |
|---|---|
| `commit.sh` | Iterates every repo in `academy/*.code-workspace`, stages, commits with a required message, pulls, pushes. Supports `--repo`, `--paths`, `--yes`, `--include-untracked`. Interactive y/N prompt against `/dev/tty`. |
| `sync.sh` | Iterates every workspace repo and runs `git pull && git push`. No commit. |
| `check-actions-all.sh` | Iterates every workspace repo and reports the latest run of every workflow, with failure details. |
| `delete-releases.sh` | Bulk-deletes GitHub releases + tags across a list of `owner/repo` arguments. `DRY_RUN` env var. |
| `delete-packages.sh` | Bulk-deletes GitHub packages (handles the public→private requirement). `DRY_RUN`. |
| `delete-repos.sh` | Bulk-deletes GitHub repos. `DRY_RUN`. |
| `delete-sonar-projects.sh` | Bulk-deletes SonarCloud projects. `DRY_RUN`. |
| `test-pipeline-templates.sh` | Operational test for the pipeline templates. |
| `common.sh` | `load_workspace_folders` (reads `*.code-workspace` via `node -e ...`), `wait_for_rate_limit`, `gh_api_or_stop`. |
| `gh-retry.sh` | 4-attempt exponential-backoff wrapper around `gh`. |
| `check-rate-limits.sh` | Standalone rate-limit reporter. |

These work, but the user can only call them from `academy/github-utils/` (or via the bash wrappers in `~/.claude/commands/`) because they discover the workspace by walking up from their own script location. The user wants to invoke them **from anywhere on disk, without going through AI** — which means the entry point must be a globally-installed binary on `PATH`. `gh optivem` already fits that bill (installed via `gh extension install optivem/gh-optivem`).

Additional pressure to migrate:

- **`commit.sh` fails on `/dev/tty` in Claude Code's non-TTY shell** ([[feedback_commit_script_tty]]). A native Go implementation can use `golang.org/x/term.IsTerminal` and degrade cleanly without the `/dev/tty` dance.
- **Windows fragility.** Every script depends on Git Bash + `node` + `cygpath` to parse `*.code-workspace`. The Go port replaces all three with `encoding/json`.
- **Duplicated gh-retry logic.** `gh-optivem/internal/shell/ghretry.go` already implements the exact same policy as `github-utils/scripts/gh-retry.sh` — porting the scripts lets them share one source of truth.

Decision (locked from this session's open-questions walk):

1. **End state: replace entirely.** Port to gh-optivem; delete the bash scripts; retire the `~/.claude/commands/{commit,sync,…}.md` wrappers (or rewrite them to invoke `gh optivem`).
2. **Scope: all** scripts listed above.
3. **Workspace discovery cascade** mirrors the existing `projectconfig.ResolvePath` shape: `--workspace <path>` flag > `$GH_OPTIVEM_WORKSPACE` env var > walk up from CWD looking for `*.code-workspace`.
4. **Command layout (mainstream `gh`-extension noun-verb, matching gh-optivem's existing pattern):**
   ```
   gh optivem workspace {commit, sync, check-actions, test-pipeline-templates, rate-limit}
   gh optivem cleanup   {releases, packages, repos, sonar-projects}
   ```
   `workspace` = iterates the `.code-workspace` folders. `cleanup` = bulk-destructive ops keyed by `owner/repo` args. The split is honest about the two scripts having different inputs (workspace file vs explicit targets) and isolates destructive verbs behind a discoverable namespace. This is the one deliberate departure from strict `gh`-core parity (`gh release delete` is single-repo; these are multi-repo bulk).

This plan does not add capabilities the scripts don't already have — see [[feedback_materialize_dont_expand]]. The port is a port.

## Critical files

**Sources to port (read-only references):**

- `academy/github-utils/scripts/commit.sh` — workspace commit semantics (lines 116–254 are the loop body).
- `academy/github-utils/scripts/sync.sh` — workspace sync (whole file, 47 lines).
- `academy/github-utils/scripts/check-actions-all.sh` — workflow-status report (lines 36–131).
- `academy/github-utils/scripts/delete-releases.sh` — bulk delete + DRY_RUN + tag cleanup.
- `academy/github-utils/scripts/delete-packages.sh` — public→private + delete.
- `academy/github-utils/scripts/delete-repos.sh` — bulk repo delete.
- `academy/github-utils/scripts/delete-sonar-projects.sh` — SonarCloud project bulk delete.
- `academy/github-utils/scripts/test-pipeline-templates.sh` — pipeline-template operational test.
- `academy/github-utils/scripts/common.sh` — `load_workspace_folders`, `wait_for_rate_limit`, `gh_api_or_stop`.
- `academy/github-utils/scripts/check-rate-limits.sh` — standalone reporter.

**Targets to add in `gh-optivem/`:**

- `workspace_commands.go` — `newWorkspaceCmd()` Cobra group + `commit`, `sync`, `check-actions`, `test-pipeline-templates`, `rate-limit` subcommands. Mirrors the existing `system_commands.go` shape (e.g. `Use: "system"` at `system_commands.go:27` → `Use: "workspace"`).
- `cleanup_commands.go` — `newCleanupCmd()` group + `releases`, `packages`, `repos`, `sonar-projects` subcommands.
- `internal/workspace/` — new package: `Resolve(flag string) (path string, folders []string, err error)` implementing the flag→env→walk-up cascade and the `*.code-workspace` JSON parse. **No external `node` shell-out** — `encoding/json` only.
- `internal/ghbulk/` — new package: paginated `gh api` helpers for releases / packages / repos / sonar-projects with `DryRun bool`, integrated rate-limit guard, and ghretry under the hood.
- `internal/sonar/` — new package for SonarCloud REST calls (delete-projects script's only external dep — minimal HTTP client).

**Targets to update in `gh-optivem/`:**

- `main.go:95-104` — `cmd.AddCommand(...)` list. Add `newWorkspaceCmd()` and `newCleanupCmd()`.
- `README.md` — add `gh optivem workspace` and `gh optivem cleanup` sections.
- `internal/shell/ghretry.go` — already implements the retry policy from `gh-retry.sh`. Confirm the new bulk callers route through it; no changes needed if the package exports a public entry point already (audit during implementation).

**Targets to delete after migration:**

- All scripts under `academy/github-utils/scripts/`.
- `academy/github-utils/README.md` — collapse to a deprecation note pointing at `gh optivem workspace --help` and `gh optivem cleanup --help`. (Do not delete the repo or LICENSE; keep the dir as a tombstone for users with old shell history.)

**Targets to update outside `gh-optivem/`:**

- `~/.claude/commands/commit.md`, `sync.md`, `check-actions.md`, `github-commit-push-all.md`, `github-sync-all.md`, `github-check-actions-all.md` — rewrite each to call `gh optivem workspace <verb>` instead of `bash .../github-utils/scripts/<verb>.sh`. (These are user-level slash-command files; the same skill names should keep working.)

## Reuse references

- **`gh-optivem/internal/shell/ghretry.go:11-37`** — Go port of `gh-retry.sh` already exists. Same 4-attempt / 5s→15s→45s policy, same transient/hard-fail regexes. The new bulk-delete commands route through this; no parallel implementation.
- **`gh-optivem/internal/projectconfig/` `ResolvePath`** — the canonical flag > env > CWD cascade. The new `internal/workspace.Resolve` mirrors it shape-for-shape: takes a flag value, falls back to `GH_OPTIVEM_WORKSPACE`, then walks up from CWD looking for `*.code-workspace`. Same return-explicit-bool convention.
- **`gh-optivem/internal/promptio/promptio.go`** — existing y/N prompt helper. `commit.sh:135-151`'s `/dev/tty` confirmation is replaced by a `promptio` call that already handles TTY detection. Resolves [[feedback_commit_script_tty]] for free.
- **`gh-optivem/internal/shell/github.go`** — existing wrapper around `gh` CLI invocations. Used by `check-actions` (workflow listing) and the bulk-delete commands (release/package/repo lookups). Already integrates with `ghretry`.
- **`gh-optivem/main.go:79-106`** `newRootCmd` — model for adding the two new `AddCommand` entries.
- **`gh-optivem/system_commands.go:27-46`** `newSystemCmd` — model for the `workspace` and `cleanup` group commands (parent with no `Run`, subcommands underneath).
- **`gh-optivem/test_commands.go:28-58`** `newTestCmd` + `run` — model for nested subcommands that share flags.

## Out of scope

- **New capabilities.** The plan ports existing behaviour 1:1. Do not add a "commit only modified, not new files" mode, a parallel-pull mode, a Slack notifier, etc. Per [[feedback_materialize_dont_expand]], scope discipline. New verbs go in a follow-up plan.
- **Workspace-file schema changes.** `*.code-workspace` is consumed read-only. The plan does not introduce a `gh-optivem`-specific workspace format.
- **The `optivem-testing/` and `rehearsal-*/` directories.** They live alongside the workspace but are not part of the `folders[]` list. Out of scope unless `load_workspace_folders` already picks them up (in which case parity is preserved).
- **Restructuring `~/.claude/commands/*.md`.** The plan rewrites the contents so they call `gh optivem ...`, but leaves the filenames intact so the user's muscle-memory `/commit`, `/sync`, etc. keep working. A separate dedupe pass (e.g. `commit` vs `github-commit-push-all` may be the same skill) is a different task.
- **Cross-platform installer changes for `gh-optivem`.** The extension already installs via `gh extension install` on Win/Mac/Linux; nothing to update there.
- **CI-side `gh-retry`.** `actions/shared/gh-retry.sh` is referenced from GitHub Actions workflows, not by the user locally. It stays — it's not in `github-utils/scripts/`. (Per [[feedback_ci_mirrors_user_flow]], CI keeps mirroring the real user flow, but the "real user flow" here is `gh optivem ...` running locally; CI doesn't invoke these commands.)
- **Renaming `github-utils` the repo.** The repo stays as a tombstone with a deprecation README. Renaming or deleting the GitHub repo is a follow-up — the user may want a redirect period.

## Steps

### 2. Add `gh optivem workspace commit`

Create `workspace_commands.go`:

```go
func newWorkspaceCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "workspace",
        Short: "Operate on every repo in the academy workspace",
    }
    cmd.AddCommand(
        newWorkspaceCommitCmd(),
        newWorkspaceSyncCmd(),
        newWorkspaceCheckActionsCmd(),
        newWorkspaceTestPipelineTemplatesCmd(),
        newWorkspaceRateLimitCmd(),
    )
    cmd.PersistentFlags().String("workspace", "", "Path to a directory containing a *.code-workspace file (default: $GH_OPTIVEM_WORKSPACE or walk up from CWD)")
    return cmd
}
```

`newWorkspaceCommitCmd` ports `commit.sh` 1:1. Flags:

- `--repo <name>` (single-repo scope)
- `--paths "<paths>"` (requires `--repo`)
- `--yes` (skip confirmation; refuses untracked unless `--include-untracked`)
- `--include-untracked` (no-op without `--yes`)
- Positional: commit message (required when any iterated repo is dirty)

Logic:

1. `workspace.Resolve` for folders.
2. For each folder with `.git/`, check `git status --short`. If dirty:
   - Print status.
   - If `--yes`, gate untracked-file refusal (port `commit.sh:227-236`).
   - Otherwise call `promptio.ConfirmYN(...)`. On a non-TTY without `--yes`, error out with the same "stdin is not a TTY" message as the bash version.
   - If confirmed: `git add` (`-A` or `-- <paths>`), `git commit -m "<msg>\n\nCo-Authored-By: ..."`, increment `committed`.
   - If declined and `--paths` was used: `git reset -- <paths>` to restore staging (port `commit.sh:213-216`).
3. Always run `git pull && git push` per repo (mirrors `commit.sh:250-253`).
4. Print the same `committed / synced / skipped` summary line.

**Co-author trailer**: keep the existing `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>` trailer for now (parity); flagging it for review in a follow-up since gh-optivem itself isn't a Claude session.

Regression test: a `workspace_commands_test.go` that drives `runWorkspaceCommit` against a temp dir with two fake git repos (one dirty, one clean) and asserts the dirty repo got a commit with the supplied message and the clean repo did not.

### 3. Add `gh optivem workspace sync`

Direct port of `sync.sh` (47 lines, no flags). Walks folders, runs `git pull && git push` on each repo with a remote tracking branch, prints `synced / skipped` summary. No tests beyond a smoke test — too thin to be worth more.

### 4. Add `gh optivem workspace check-actions`

Port `check-actions-all.sh`. Inputs: workspace folders. For each repo:

- `gh workflow list --all` (routed through `internal/shell/ghretry`).
- For each workflow, `gh run list --workflow <id> --limit 1`.
- For failures, `gh run view --log-failed` and grep for `##[error]`.

Same emoji-prefixed output (✅ / ❌ / ⏭). Pre-existing `internal/shell/github.go` already wraps the `gh` shell-out; this command just orchestrates calls.

Add a small unit test for the parsing layer (TSV record → struct) but skip an end-to-end test that hits the real `gh` CLI — too brittle for CI, manual verification covers it.

### 5. Add `gh optivem workspace test-pipeline-templates` and `rate-limit`

Port `test-pipeline-templates.sh` and `check-rate-limits.sh` as straightforward shell-outs through `internal/shell`. Both are simple — fewer than ~50 lines of bash each. No tests; smoke-verify in step 11.

### 6. Add `internal/ghbulk/` for paginated bulk operations

`ghbulk.go` exposes:

```go
type Options struct {
    DryRun bool
    PageSize int
    RateLimitThreshold int  // default 50, env override GH_OPTIVEM_RATELIMIT_THRESHOLD
}

// ForEachRelease lists all releases for owner/repo (paginated) and invokes fn
// for each. fn is invoked even when DryRun is true (so callers can log "would
// delete <tag>"); the destructive op itself respects DryRun internally.
func ForEachRelease(owner, repo string, opt Options, fn func(rel Release) error) error
// ...likewise for Package, Repo
```

Internals use `gh api ... --paginate` via `internal/shell/github.go` (`gh_retry`-equivalent already integrated). A pre-flight `wait_for_rate_limit` mirrors `common.sh:49-65`: read `gh api rate_limit`, if remaining < threshold, sleep until reset.

### 7. Add `gh optivem cleanup releases/packages/repos/sonar-projects`

Create `cleanup_commands.go`:

```go
func newCleanupCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "cleanup",
        Short: "Bulk-delete remote artifacts (releases, packages, repos, SonarCloud projects)",
        Long:  "Destructive operations. Pass --dry-run first to preview.",
    }
    cmd.AddCommand(
        newCleanupReleasesCmd(),
        newCleanupPackagesCmd(),
        newCleanupReposCmd(),
        newCleanupSonarProjectsCmd(),
    )
    cmd.PersistentFlags().Bool("dry-run", false, "Print what would be deleted; do not delete")
    return cmd
}
```

Each subcommand takes positional `owner/repo` args (1+). Behaviour matches the matching bash script — page through, optionally make-private (packages only), delete, sleep between deletes (port `common.sh:21-23` `DELAY_BETWEEN_DELETES`).

`--dry-run` is a flag, not an env var (DRY_RUN). The env var is the bash idiom; flags are the gh-optivem idiom. **Do not** support both — pick the flag, document the change in the deprecation README in step 10.

SonarCloud: `internal/sonar/` wraps the SonarCloud `api/projects/delete` endpoint using `SONAR_TOKEN` from env (the same env var the existing scaffolder uses — keep parity). Auth header `Authorization: Bearer $SONAR_TOKEN`.

Regression tests: table tests for the argument parser (`owner/repo` validation, rejects bare `repo`, rejects `owner/repo/extra`). Skip live-API tests; smoke-verify in step 11.

### 8. Wire the new groups into the root command

In `main.go:95`, extend `cmd.AddCommand(...)`:

```go
cmd.AddCommand(
    newInitCmd(),
    newConfigCmd(),
    newSystemCmd(),
    newTestCmd(),
    newCompileCmd(),
    newImplementCmd(),
    newProcessCmd(),
    newEnvironmentCmd(),
    newWorkspaceCmd(),   // new
    newCleanupCmd(),     // new
)
```

### 9. Rewrite the slash-command wrappers

For each of `~/.claude/commands/{commit, sync, check-actions, github-commit-push-all, github-sync-all, github-check-actions-all}.md`, replace the bash invocation:

**Before** (e.g. `commit.md`):
```bash
bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh" $ARGUMENTS
```

**After:**
```bash
gh optivem workspace commit $ARGUMENTS
```

Verify each slash command's documented `$ARGUMENTS` semantics map cleanly to the new flag surface. If a slash command documents a flag the new Go version doesn't support, surface that mismatch and adjust the slash-command file's docs (not the Go code — the Go code is the canonical surface).

### 10. Tombstone `github-utils/scripts/`

- Delete every file under `academy/github-utils/scripts/`.
- Rewrite `academy/github-utils/README.md` to ~10 lines:
  ```markdown
  # github-utils (deprecated)

  The scripts in this directory were ported to the `gh optivem` CLI on 2026-05-XX.

  - `commit.sh` → `gh optivem workspace commit`
  - `sync.sh` → `gh optivem workspace sync`
  - `check-actions-all.sh` → `gh optivem workspace check-actions`
  - `delete-releases.sh` → `gh optivem cleanup releases`
  - `delete-packages.sh` → `gh optivem cleanup packages`
  - `delete-repos.sh` → `gh optivem cleanup repos`
  - `delete-sonar-projects.sh` → `gh optivem cleanup sonar-projects`
  - `test-pipeline-templates.sh` → `gh optivem workspace test-pipeline-templates`
  - `check-rate-limits.sh` → `gh optivem workspace rate-limit`

  Install: `gh extension install optivem/gh-optivem`
  ```

Note: this is a deletion + README rewrite in the **`github-utils` repo**, not in `gh-optivem`. The commit lives in `academy/github-utils/`. Per [[feedback_new_plan_not_extend]], a parallel commit in that repo's history references this plan.

### 11. End-to-end verification

From an arbitrary directory (e.g. `C:\Users\valen_4rjvn9e\`):

```powershell
$env:GH_OPTIVEM_WORKSPACE = "C:\GitHub\optivem\academy"
gh optivem workspace sync
gh optivem workspace commit "test message"   # with no dirty repos: should be a no-op + sync
gh optivem workspace check-actions
gh optivem cleanup releases optivem/nonexistent-repo --dry-run   # should error cleanly with 404
```

Then `Remove-Item Env:\GH_OPTIVEM_WORKSPACE` and re-run from `C:\GitHub\optivem\academy\shop\` (walk-up should find the workspace).

Negative cases:

- No workspace anywhere: clear error message naming the three cascade options.
- Malformed `*.code-workspace`: error names the file and the JSON failure.
- `commit` against a dirty repo with no message and `--yes`: error matches the bash version's wording.
- `commit --yes` against an untracked file without `--include-untracked`: refuses with the stray-file warning.
- `cleanup releases foo` (missing slash): rejected by argument parser, not by the API.

### 12. Update gh-optivem README

Add a short "Workspace operations" + "Cleanup" section to `README.md` after the existing command tables, with one example per verb.

### 13. (Optional) Update `MEMORY.md`

The [[feedback_commit_script_tty]] memory documents a workaround for a script that no longer exists after this migration. After step 9 ships and the slash commands invoke `gh optivem workspace commit`, that memory entry should be removed or updated to "ported to gh optivem; /dev/tty issue resolved by promptio". Flag it but do not edit yet — wait until the migration lands.

## Verification

The plan is complete when:

1. From any directory on the user's machine (with `gh extension install optivem/gh-optivem` done and `$GH_OPTIVEM_WORKSPACE` exported or CWD inside the academy tree), every verb in the table below works without invoking AI:

   | Old | New |
   |---|---|
   | `bash …/github-utils/scripts/commit.sh "msg"` | `gh optivem workspace commit "msg"` |
   | `bash …/github-utils/scripts/sync.sh` | `gh optivem workspace sync` |
   | `bash …/github-utils/scripts/check-actions-all.sh` | `gh optivem workspace check-actions` |
   | `DRY_RUN=1 bash …/delete-releases.sh owner/repo` | `gh optivem cleanup releases owner/repo --dry-run` |
   | …packages / …repos / …sonar-projects | `gh optivem cleanup {packages,repos,sonar-projects} owner/repo` |

2. The `~/.claude/commands/{commit,sync,…}.md` slash commands now invoke `gh optivem ...` instead of bash, and running each `/commit`, `/sync`, etc. in Claude Code produces the same outcome as before.

3. `academy/github-utils/scripts/` is empty (or removed) and the README is a tombstone.

4. The [[feedback_commit_script_tty]] failure mode (`/dev/tty` interactive prompt in Claude Code's shell) no longer reproduces — the new `gh optivem workspace commit --yes` works unattended without the stray-file foot-gun.

5. `gh optivem workspace --help` and `gh optivem cleanup --help` list every verb with one-line descriptions. Top-level `gh optivem --help` lists `workspace` and `cleanup` alongside the existing `config`, `system`, `test`, etc.

6. The new commands route every `gh` API call through `internal/shell/ghretry` (no parallel retry loop), and the bulk-delete commands pre-check `gh api rate_limit` before each repo just like `common.sh:wait_for_rate_limit`.
