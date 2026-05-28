This is a one-shot dispatch. Read the context substituted into this prompt, do the work, and exit.

Ticket: #${issue-num} "${issue-title}"
Phase: ${phase}

## Trust the orchestrator's context — do not rediscover it

Every ticket and repo-state value you need is already substituted into this prompt. **Do not run** `gh issue view`, `git status`, `git log`, `git branch`, `git rev-parse`, or `git show <sha>` — the ticket body is in the AC / Checklist blocks below and the working-tree state is in `${changed-files}` (when populated).

## Scope-bound reads

Read only files in the prompt's `scope:` frontmatter, plus files an explicit Step makes load-bearing. Targeted greps for prompt-named symbols are fine; open-ended exploration is a scope violation.

`${changed-files}` is already-substituted context, not a read. The `fix-*` tasks' `git diff` / `git show HEAD:<path>` carve-out applies only to those tasks.

If the work needs a path outside scope, emit the scope-exception envelope (see `scope.md` below) and exit.

## Don't commit, don't summarise, don't ask

When the work is done, exit cleanly. The orchestrator drives test
runs, disabling, and commits — never run `git commit`, `git add`,
`gh issue close`, or test commands yourself.

Do not stop mid-dispatch to present a plan or ask for approval — the
orchestrator gates approvals between phases. If genuinely blocked (an
ambiguous AC, a required out-of-scope edit), emit the scope-exception
envelope (per `scope.md`) and exit.

## Edit cohesion

Batch all edits to the same file into one `Write` or `Edit` call.

---
