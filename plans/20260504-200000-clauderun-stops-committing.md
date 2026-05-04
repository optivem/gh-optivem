# clauderun stops committing — outer CLI takes over

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-04T14:51:51Z`

## Motivation

Two CLIs are layered on top of the agent today: `clauderun` (this repo's Go binary, embedded in the `gh optivem atdd` commands) and an outer wrapper the operator runs around it. The 2026-04-30 plan (`20260430-171111-cli-owns-commit-not-agent.md`) moved the commit step from the agent into `clauderun`, on the premise that `clauderun` was the outermost CLI. That premise no longer holds.

The outer wrapper now runs its own post-dispatch flow ("STOP — press Enter to continue", "TEST mode?", "Can I commit?"). With `--cli-commits=true` as the default, `clauderun` commits at agent exit before the wrapper's gate fires, which is why the wrapper's "Can I commit?" prompt now reports `Staged changes: (none)` — the commit already happened one layer down.

The fix is to push the commit responsibility one more layer out. `clauderun` (and the agent it dispatches) should never run `git add` or `git commit`. The outer wrapper owns staging and committing because it owns the post-run human gates that should come before a commit.

This plan undoes the gh-optivem half of the 2026-04-30 design and obsoletes the 2026-05-02 pre-commit-hook plan that depended on it. The "agent never commits" guarantee survives — that part of the previous plan was correct and stays. What's removed is `clauderun`'s commit step.

## Approach

- `clauderun.Dispatch` reduces to: render prompt → run subprocess → snapshot pre/post repo state for *informational* purposes → exit. Process exit code is the completion signal. Working-tree delta (if any) is left intact for the outer wrapper.
- All commit-related flags, env vars, helpers, and prompt-gating code paths are deleted, not soft-disabled. There is no flag-gated rollout this time — the previous plan's flag (`--cli-commits`) is going away with the code.
- Embedded agent prompts ship with a single, fixed preamble: *"do not commit, exit cleanly."* The dual-preamble rewrite in `applyCommitGating` is gone.
- The exit banner switches from `committed <sha>` to a changed-file count, so the operator sees the dispatch's effect without claiming a commit happened.

## Items

### 1. Strip commit logic from `internal/atdd/runtime/clauderun/clauderun.go`

**Remove:**

- `Options.CLICommits` field (`clauderun.go:104-115`).
- The `if opts.CLICommits { commitChanges(...) }` branch in `Dispatch` (`:280-288`).
- The "subprocess clean but HEAD unchanged" error path and `errNoCommit` sentinel (`:290-294`, `:521`). With no commit expected, the only success criterion is subprocess exit zero plus no mid-run branch switch.
- The "honor agent's commit if HEAD moved" tail of `Dispatch` (`:296-307`) — the agent isn't supposed to commit, and if it does, that's an outer-wrapper problem to flag.
- `commitChanges` (`:326-357`).
- `runCLICommit` and the `CLAUDERUN_CLI_COMMIT` env-var dance (`:359` onward).
- `readCommitSubject`, `renderCommitMessage`, `diffDirty` if no other caller remains. Audit and delete.
- `installPreCommitHook` and any caller (added by `20260502-200525-pre-commit-hook-blocks-agent-commits.md`). Search for `pre-commit` in the package and remove the install path entirely.
- `applyCommitGating` and the dual-preamble logic (`:566-...`). Embedded prompts get edited in place to their final form (item 3 below).

**Keep:**

- `repoState` snapshot, the `rev-parse HEAD` calls, and the `--abbrev-ref HEAD` branch check. These remain useful as informational signals (banner, diff stats) and as a safety guard against agents switching branches mid-run. Just stop *gating* on them.
- `CommitInfo` — repurpose to `RunInfo` (or similar) carrying `ChangedFiles []string` and elapsed time, since downstream callers consume the return value. Rename + reshape; don't keep a misleading struct name.

**Banner:** Update `writeExitBanner` (`clauderun.go:883-...`) to show changed-file count instead of `committed <sha>`. Suggested form: `✅ EXITED AGENT: 7 file(s) changed (3m21s)` and `✅ EXITED AGENT: no changes (3m21s)` for the no-op case.

### 2. Strip commit-related flags from `atdd_commands.go`

- Delete `--cli-commits` and `--agent-commits` flag definitions on both `atdd dispatch` (`atdd_commands.go:153-154`) and `atdd manage-project` (`:208-209`).
- Delete `resolveCommitMode` (`:228-...`) and its two call sites (`:131`, `:188`).
- Delete the `cliCommits` / `agentCommits` package-level vars feeding those flags.

### 3. Edit embedded agent prompts to their final preamble

Prompts live at `internal/atdd/runtime/agents/prompts/*.md`. The 2026-04-30 plan landed them in their CLI-commits *target* state, with `applyCommitGating` reverse-substituting the legacy preamble at render time when `--cli-commits=off`. Now that the legacy mode is gone, the target state ships unconditionally.

- Confirm each prompt's preamble matches the new wording: *"When the work is done, do not commit and do not summarise — exit cleanly. The agent must never run `git commit`, `git add`, or `gh issue close`. The wrapping CLI handles staging and committing after you exit."*
- Remove any residual reference to `${commit-confirmation-block}` markers or the legacy-commit-confirmation include path.
- Delete `internal/atdd/runtime/agents/shared/legacy-commit-confirmation.md`. It only existed to support `applyCommitGating`'s legacy-mode swap.

### 4. Rewrite tests

`internal/atdd/runtime/clauderun/clauderun_test.go` and `internal/atdd/runtime/driver/driver_test.go` have many cases that exercise the commit path. Two patterns to convert:

- **Cases that asserted "CLI staged + committed delta X with message Y":** rewrite to assert "working tree contains delta X, HEAD unchanged, exit code zero." The fake `GitRunner` should no longer expect `git add`/`git commit` calls.
- **Cases that asserted `errNoCommit` when subprocess clean + HEAD unchanged:** delete or invert. Subprocess exit zero with HEAD unchanged is now a legitimate no-op, not an error.

Search for `--cli-commits`, `CLICommits`, `errNoCommit`, `runCLICommit`, `commitChanges`, `applyCommitGating` in test files and rewrite/delete each match.

### 5. Audit other callers and process-flow YAML

- `internal/atdd/runtime/actions/bindings.go` — referenced `atdd-task` per earlier grep. Confirm no commit-related action bindings.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — confirm no node references a commit gate that no longer exists.
- `internal/atdd/runtime/intake/sections.go` — check whether the intake flow expects post-dispatch HEAD changes.

These are likely no-op confirmations, but worth a pass to catch implicit dependencies.

### 6. Delete obsoleted plan files

- `plans/20260430-171111-cli-owns-commit-not-agent.md` — design superseded by this plan.
- `plans/20260502-200525-pre-commit-hook-blocks-agent-commits.md` — depended on `commitChanges` setting `CLAUDERUN_CLI_COMMIT=1`. With `commitChanges` gone, the hook has nothing to gate against, and the only legitimate caller it was protecting against is also gone.

If hook-level enforcement is wanted later (belt-and-suspenders against a misbehaving agent invoking `git commit` directly), it belongs in the outer CLI's worktree setup, not here. Out of scope for this plan.

### 7. Update `shop` process docs

- The 2026-04-30 plan deferred deleting `shop/docs/atdd/process/shared-commit-confirmation.md` until after rehearsals soaked. With this plan landing, the legacy-mode reason for keeping it is gone — but the *new* reason ("agent never commits, outer CLI does") still wants documentation somewhere.
- Decide: rewrite `shared-commit-confirmation.md` to describe the new flow, or delete it and add a short paragraph to `cycles.md` pointing at the outer CLI as the commit owner. Lean toward delete-and-replace; the existing filename is misleading under the new design.
- Cross-ref grep targets stay as before: `cycles.md`, `shared-ticket-status-in-acceptance.md`, `task-and-chore-cycles.md`.

## Out of scope

- The outer CLI's commit logic. That lives outside this repo and isn't this plan's concern. This plan only ensures `clauderun` hands off cleanly: subprocess exits, working tree has the delta, HEAD untouched.
- Sign-off / GPG. Same as the prior plan.
- Pre-commit hook enforcement (see item 6 rationale).
- Permission deny lists in agent prompts (`Bash(git commit:*)` etc.). Cleaner agent UX if the agent tries something it shouldn't, but the wording in item 3 carries the prohibition; deny lists are a follow-up.
- Phase boundary gates between agents. The outer CLI's STOP / TEST / commit prompts cover this surface; if more granular gating is wanted, that's a separate plan there.

## Order of operations

1. Land items 1–4 in `gh-optivem` (the code change, atomic). After this commit, `clauderun` no longer touches git beyond `rev-parse`.
2. Item 5 (audit). Should be a no-op pass; if it surfaces a hidden caller, fold the fix into the same commit or a follow-up.
3. Item 6 (delete plan files). Can ride along with item 1's commit or land separately — either is fine, the plans are doc-only.
4. Item 7 (shop docs) — separate commit in the `shop` repo. Land after items 1–3 so the docs describe the as-shipped behavior.
5. Smoke-test with one rehearsal of `gh optivem atdd dispatch` end-to-end: confirm the agent runs, exits, leaves the working tree dirty, the outer CLI's gates fire and `Can I commit?` actually has staged changes to act on.

## Outer CLI handoff

Confirmed (2026-05-04): the outer CLI does its own `git status` and needs nothing from `clauderun`. No JSON summary, no `--summary-out` flag, no stdout contract beyond what's already there. `clauderun` exits, the wrapper inspects the worktree, full stop.

This means item 1's `RunInfo` rename is for *internal* call-site clarity only. If no in-repo caller actually consumes the changed-file list, drop the field and make `Dispatch` return `(time.Duration, error)` or just `error`. Decide while editing.

## Open questions

- **Banner colour / format.** The current green ✅ banner reads as "all good, committed cleanly." Once it just reports file count, is green still right? Yellow when files changed (signaling "the wrapper has work to do"), green only on no-changes? Decide during item 1.
