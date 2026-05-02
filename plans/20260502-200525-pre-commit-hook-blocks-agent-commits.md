# Pre-commit hook blocks agent commits

## Motivation

Item 4 of `20260430-171111-cli-owns-commit-not-agent.md` switched the prompts to "do not commit; the CLI will." That's instruction, not enforcement. A misbehaving agent — stale context, an unexpected tool call, an operator override — can still run `git commit` and there's nothing at the git layer to stop it.

The CLI's `clauderun.commitChanges` is the *only* path that should produce a commit on a dispatch branch. We can express that invariant with a `pre-commit` hook in the worktree that refuses to commit unless an env var set by `commitChanges` is present.

This is the enforcement layer that sits behind item 4's documentation layer. Both want the same thing: agents never touch `git commit`.

## Approach

- Install `.git/hooks/pre-commit` in the dispatch worktree. The hook reads `CLAUDERUN_CLI_COMMIT`; if it isn't `1`, the hook prints a one-line explanation and exits non-zero.
- `clauderun.commitChanges` sets `CLAUDERUN_CLI_COMMIT=1` for its own `git commit` invocation only.
- Agent dispatches never see that env var, so any `git commit` they attempt — directly via the Bash tool, indirectly via a script, anything — is rejected at the git layer.

## Items

### 5. Manual rehearsal verification

After landing items 1–4, run a rehearsal with `--cli-commits` and confirm:

- The CLI's own commit lands as expected.
- Manually invoking `git commit` inside the worktree (e.g. via a separate shell) fails with the hook's message.
- A rehearsal *without* `--cli-commits` runs unchanged (no hook installed, agent commits work).

## Out of scope

- Claude Code permission deny list (`permissions.deny: ["Bash(git commit:*)", ...]`). Cleaner failure UX for the agent, but the hook is the actual enforcement boundary; perms are belt-and-suspenders. Add later if rehearsals show agents confused by hook rejections.
- Hooks for other repos (this is dispatch-specific).
- Multi-hook coexistence (`core.hooksPath`). Not needed until something else wants to install hooks.
- Signing / GPG. Same rollout discussion as the parent plan.

## Order of operations

1. ~~Land items 1–4 in `gh-optivem`.~~ **Done (2026-05-02).**
2. Run item 5 (manual rehearsal verification) to confirm the enforcement works end-to-end.
3. After parent plan's step 5 (remove `--agent-commits`), drop the `--cli-commits` gate from item 3 — hook installs unconditionally on every dispatch.
