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

## Scope-bound reads

Read only files you actually need for the work. The scope listed in the prompt's `scope:` frontmatter is the **complete** set of paths you may read or modify, with two narrow additions:

1. Files this prompt explicitly tells you to `Read` (e.g. lines like "Read `${references_root}/atdd/architecture/test.md`"). Such Reads are part of the prompt's contract; they are allowlisted by their explicit presence in the prompt body.
2. Files you must inspect to satisfy an explicit Step in the prompt body — e.g. when a Step says "implement the DSL Port layer", you may read files under that layer even if your phase doesn't list them, because the Step makes that read load-bearing.

**Greps and globs:** targeted greps for symbols named by the prompt or required by a Step are fine (e.g. "find the `CustomerService` class"). Open-ended greps ("look for related tests", "find similar code", "look around") are over-reading — treat as scope violations.

**Carve-outs that survive this rule** (each is its own contract, not a general license):

- `${changed_files}` and the working-tree state it describes are already-substituted context; you don't re-fetch them.
- The fix-* tasks' explicit `git diff`/`git show HEAD:<path>` exception (documented per-prompt under "Exception to the anti-rediscovery rule") stays in force for those tasks only.

If you cannot do the work without reading something outside scope and outside the two exceptions above, emit the scope-exception envelope via `gh optivem output write` (see `scope.md` for the exact call) and exit.

## Don't commit, don't summarise, don't ask

When the work is done, do not summarise and do not commit — exit cleanly. The orchestrator drives compile, test runs, disabling, and commits as separate service tasks; the agent must never run `git commit`, `git add`, `gh issue close`, the compile commands, or the test commands.

Do not present a plan and wait for approval inside the agent. The orchestrator gates approvals between phases; an agent that stops mid-dispatch to ask the operator something will hang the pipeline. If you genuinely cannot proceed (an ambiguous Acceptance Criterion, an out-of-scope edit required, a contradiction between two inputs), emit the appropriate structured exit (the scope-exception envelope via `gh optivem output write` per `scope.md`, or a task-specific `blocker:` block when defined) and exit.

## Edit cohesion

When you have multiple edits to the same file, make them in one `Write` or one `Edit` call with a larger context window rather than several sequential `Edit`s. Each tool round-trip costs latency and tokens; a file's interface additions, impl methods, and wiring are typically one cohesive change.

---
