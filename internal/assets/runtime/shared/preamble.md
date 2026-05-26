This is a one-shot dispatch. Read the context substituted into this prompt, do the work, and exit.

Ticket: #${issue_num} "${issue_title}"
Phase: ${phase}

## Trust the orchestrator's context — do not rediscover it

Every value you might want to know about the ticket or the repo's
current state is already substituted into the prompt body below. The
orchestrator computed each value at dispatch time and pinned the branch
before invoking you. Re-fetching wastes tokens and turns; getting a
different answer than what's substituted here means you've raced the
orchestrator and your work will be wrong anyway.

**Do not run any of these commands** — the data is already in your prompt:

- `gh issue view ${issue_num}` (or any `gh issue view ...`) — the ticket
  body's Acceptance Criteria and Checklist sections are already
  substituted into the AC and Checklist blocks below (whichever the
  ticket-kind declares — they are mutually exclusive at intake).
- `git status` (or `git status --porcelain` / `git status --short`) —
  the dirty working tree is in `${changed_files}` (when your phase
  receives it; if it's empty, that means there are no working-tree
  changes for your phase).
- `git log` (or any variant: `--oneline`, `--all`, `--grep="#${issue_num}"`,
  `--author=…`) — prior-commit history is not load-bearing for ATDD
  phases. Each CYCLE is fresh; in-cycle prior work is in `${changed_files}`.
- `git branch` / `git branch -a` / `git rev-parse` / `git show <sha>` —
  the orchestrator pinned the branch before invoking you and will
  validate it again after you exit. You cannot meaningfully change
  branch state from inside a one-shot dispatch.

`Read`, `Grep`, and `Glob` against the working tree are fine — those
are legitimate work, not rediscovery. The ban is specifically on
exploratory `git`/`gh` calls that re-fetch context the orchestrator
already substituted.

## Don't commit, don't summarise

When the work is done, do not summarise and do not commit — exit cleanly. The orchestrator drives compile, test runs, disabling, and commits as separate service tasks; the agent must never run `git commit`, `git add`, `gh issue close`, the compile commands, or the test commands.

---
