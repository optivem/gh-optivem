# 2026-07-08 09:09:00 UTC — Fix `gh optivem commit` pull/rebase failing in multi-worktree repos

> **Triggered by** a real failure this session: `gh optivem commit --repo shop --yes --all` committed successfully (`fad5d866`) but then aborted the sync with `git pull --rebase in C:\GitHub\optivem\academy\shop: exit status 128` / `fatal: Cannot rebase onto multiple branches.` The commit landed but was **not pushed** — the operator had to push with a one-off `git push origin main`.

## Investigation (this session, verified)

- The commit flow is in `cross_repo_commands.go`. It commits FIRST, then runs a **bare `git pull --rebase`**, then pushes (`pushWithRebaseRetry`). See:
  - line **219**: `runGit(repo, "pull", gitFlagRebase)` (main commit path) — error wrapped at line **220** (`"git pull --rebase in %s: %w"`, the exact message seen).
  - line **456**: same bare pull in the clean-repo sync path.
  - line **~1008**: same bare pull inside `pushWithRebaseRetry` (push-rejection retry).
- The failing repo has **15 git worktrees** with many `rehearsal/*` branches checked out (`git worktree list` → 15).
- **A bare `git pull --rebase` run manually in the same repo, same state (ahead 1, behind 13), SUCCEEDED** ("Successfully rebased and updated refs/heads/main"). So the failure is **not deterministically reproducible** with the bare command — it is state/environment-sensitive.
- `git config --get-all branch.main.merge` = single `refs/heads/main`; `remote.origin.fetch` = `+refs/heads/*:refs/remotes/origin/*`; single `origin` remote. Nothing obviously malformed.
- `Cannot rebase onto multiple branches` is emitted by git's `pull` only when, after fetch, **more than one merge head** is present in `FETCH_HEAD` and mode is rebase. So at the moment the skill ran, `git pull --rebase` resolved **multiple** branches to rebase onto — most plausibly a transient `FETCH_HEAD` / multi-worktree interaction, not a stable config fault.

## Root-cause hypothesis

A bare `git pull --rebase` lets git *infer* the rebase target from `FETCH_HEAD`/upstream config. In a multi-worktree repo (or after a partial/prior fetch), that inference can resolve to **more than one** branch → the fatal error. The command is ambiguous; the fix is to make the target **explicit and singular** so git never has a choice to get wrong.

## Design decisions

1. **Keep rebase — do NOT switch to merge.** The skill implements Scaled-TBD (`docs/tbd.md`), where a linear trunk via rebase-on-pull is intentional. The bug is *ambiguity* (multiple rebase targets), not rebase itself.

   **On the "always merge" rule and why it does not govern here:** the operator's global instruction (`~/.claude/CLAUDE.md`) reads — *"Always use plain `git pull` (merge), never `git pull --rebase`. Rebase can silently drop commits on conflict — merge is safer."* That rule is a conservative safety default aimed at **feature-branch / divergent-history** pulls where incoming commits *conflict* with local ones: a merge lands the whole reconciliation in one reviewable merge commit, whereas an automated rebase replays commit-by-commit and a botched conflict resolution can reorder/drop a commit unnoticed.

   **This is incompatible with TBD as the skill practices it.** On a single shared trunk you want history to stay **linear** — merge-on-pull would pepper `main` with a merge commit on every sync, which is exactly what TBD avoids. And the risk the merge rule guards against barely applies here: trunk syncs are normally the automated `Bump VERSION …` commits, which don't touch the operator's files, so rebases are conflict-free (as this session's 13-commit sync was). So: **the "always merge" rule is scoped to divergent feature branches; the `gh optivem commit` trunk-sync path keeps rebase** — made *robust* (explicit single target) rather than switched to merge. Surface this scoping to the operator so the global rule and the TBD skill are understood as non-contradictory (different contexts), not as one overriding the other.
2. **Make the pull target explicit** at all three sites: replace bare `git pull --rebase` with an unambiguous single-target form, e.g. `git pull --rebase origin <currentBranch>` — or the more explicit `git fetch origin <currentBranch>` then `git rebase origin/<currentBranch>`. Either guarantees a single rebase head, eliminating "Cannot rebase onto multiple branches" regardless of `FETCH_HEAD` state.
3. **Resolve `<currentBranch>` per repo**, not hard-coded `main` — the skill already runs in feature branches (`branch_commands.go`, `branch_refresh.go`). Use the repo's actual current branch / its configured upstream.
4. **Fail loud, but don't strand a committed-not-pushed repo silently.** On pull/rebase error the skill currently returns and aborts the whole run. Ensure the summary makes it unmistakable which repos are **committed-but-unpushed** so the operator knows to finish the push (today it printed a generic error mid-list).

## Steps

- [ ] **Step 1 — Reproduce reliably.** In a multi-worktree checkout, script the exact sequence the skill runs (commit → bare `git pull --rebase`) and capture the multiple-merge-head condition (inspect `FETCH_HEAD` after the internal fetch). Confirm whether the trigger is worktree count, a stale `FETCH_HEAD`, or a config quirk. Report findings.
- [ ] **Step 2 — Make all three pull sites explicit-target** (lines ~219, ~456, ~1008 of `cross_repo_commands.go`). Use `git pull --rebase origin <branch>` (or fetch+rebase) with `<branch>` = the repo's current branch/upstream. Add a helper so the three sites share one implementation.
- [ ] **Step 3 — Harden the committed-not-pushed reporting.** When a pull/rebase or push fails after a successful local commit, surface a distinct, actionable outcome ("committed, NOT pushed — run `gh optivem commit` again or `git push`") rather than a bare wrapped error; keep the run going for remaining repos where safe, or at minimum name the stranded repo in the summary.
- [ ] **Step 4 — Test.** Cover: single-worktree repo (regression), multi-worktree repo (the failing case), feature-branch upstream, and push-rejection retry (line ~1008 path). Verify no "multiple branches" error and that the commit is pushed end-to-end.
- [ ] **Step 5 — Commit via `/commit`; delete this plan.** (Dogfoods the fix.)

## Notes

- Assets vs. binary: this is the **Go CLI** (`cross_repo_commands.go`), not a Claude asset under `internal/claude/assets/` — the `/commit` Claude skill merely shells out to `gh optivem commit`, so the fix is in the Go source and requires a rebuild/release of `gh optivem`.
- Related bare-rebase sites to sanity-check while here: `branch_commands.go:58`, `branch_refresh.go:46` (feature-branch rituals) — same explicit-target hardening may apply.
