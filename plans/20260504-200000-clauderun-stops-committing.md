# clauderun stops committing — outer CLI takes over

## Motivation

Two CLIs are layered on top of the agent today: `clauderun` (this repo's Go binary, embedded in the `gh optivem atdd` commands) and an outer wrapper the operator runs around it. The 2026-04-30 plan (`20260430-171111-cli-owns-commit-not-agent.md`) moved the commit step from the agent into `clauderun`, on the premise that `clauderun` was the outermost CLI. That premise no longer holds.

The outer wrapper now runs its own post-dispatch flow ("STOP — press Enter to continue", "TEST mode?", "Can I commit?"). With `--cli-commits=true` as the default, `clauderun` commits at agent exit before the wrapper's gate fires, which is why the wrapper's "Can I commit?" prompt now reports `Staged changes: (none)` — the commit already happened one layer down.

The fix is to push the commit responsibility one more layer out. `clauderun` (and the agent it dispatches) should never run `git add` or `git commit`. The outer wrapper owns staging and committing because it owns the post-run human gates that should come before a commit.

This plan undoes the gh-optivem half of the 2026-04-30 design and obsoletes the 2026-05-02 pre-commit-hook plan that depended on it. The "agent never commits" guarantee survives — that part of the previous plan was correct and stays. What's removed is `clauderun`'s commit step.

## Status

Items 1–6 (gh-optivem code + tests + obsoleted plans) landed in commit `clauderun: hand commit ownership to the outer wrapper`. Only item 7 (shop process docs) remains; it lives in a different repo and ships as a separate commit there.

## Items

### 7. Update `shop` process docs

- The 2026-04-30 plan deferred deleting `shop/docs/atdd/process/shared-commit-confirmation.md` until after rehearsals soaked. With this plan landing, the legacy-mode reason for keeping it is gone — but the *new* reason ("agent never commits, outer CLI does") still wants documentation somewhere.
- Decide: rewrite `shared-commit-confirmation.md` to describe the new flow, or delete it and add a short paragraph to `cycles.md` pointing at the outer CLI as the commit owner. Lean toward delete-and-replace; the existing filename is misleading under the new design.
- Cross-ref grep targets stay as before: `cycles.md`, `shared-ticket-status-in-acceptance.md`, `task-and-chore-cycles.md`.

## Out of scope

- The outer CLI's commit logic. That lives outside this repo and isn't this plan's concern.
- Sign-off / GPG. Same as the prior plan.
- Pre-commit hook enforcement at the gh-optivem layer. If hook-level enforcement is wanted later, it belongs in the outer CLI's worktree setup.
- Permission deny lists in agent prompts (`Bash(git commit:*)` etc.). Cleaner agent UX if the agent tries something it shouldn't, but the wording in the new preamble carries the prohibition; deny lists are a follow-up.
