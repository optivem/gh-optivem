This is a one-shot dispatch. Read the context substituted into this prompt, do the work, and exit.

Ticket: #${issue-num} "${issue-title}"
Phase: ${phase}

## Trust the substituted context — do not rediscover it

Every ticket and repo-state value you need is already substituted into this prompt. **Do not run** `gh issue view`, `git status`, `git log`, `git branch`, `git rev-parse`, or `git show <sha>` — the ticket body is in the AC / Checklist blocks below and the working-tree state is in `${changed-files}` (when populated).

## Scope-bound reads

Read only files in the prompt's `scope:` frontmatter, plus files an explicit Step makes load-bearing. Targeted greps for prompt-named symbols are fine; open-ended exploration is a scope violation. Do this discovery yourself — **do not dispatch a scouting subagent** (`Explore`, `Task`, `general-purpose`) to map files or structure. A delegated scout returns a summary you cannot trust against the real tree and routes around this rule.

`${changed-files}` is already-substituted context, not a read. The `fix-*` tasks' `git diff` / `git show HEAD:<path>` carve-out applies only to those tasks.

If the work needs a path outside scope, emit the scope-exception envelope (see `scope.md` below) and exit.

## Don't commit, don't summarise, don't ask

When the work is done, exit cleanly. Disabling tests and commits are
handled downstream, not by you — never run `git commit`, `git add`, or
`gh issue close` yourself. The test suite is also a downstream
concern for author agents; compiling is their one exception (see below)
— it is not running the suite. The single role-scoped carve-out is the
`system-implementer`, which self-verifies by running its own acceptance
slice (see "System-running roles" below); that carve-out covers running
the suite **only**, never the commit / issue-close prohibition above.

Do not stop mid-dispatch to present a plan or ask for approval —
approvals are gated downstream, between phases. If genuinely blocked (an
ambiguous AC, a required out-of-scope edit), emit the scope-exception
envelope (per `scope.md`) and exit.

## Compile your slice; don't invent API surface

Before you exit, compile what you changed and fix every compile error you
introduced — `gh optivem test compile` for test-tier code (testkit, DSL,
tests), `gh optivem system compile` for production code under
`system-path`. Compiling is **not** running the suite: your output is
always expected to compile (a compile gate runs right after you), and the
intended red is a runtime assertion failure at the test-run step, never a
compile failure — so self-compiling never touches that red.

Call only methods that exist on a type — never fake a conventional-looking
call (`flatMap`, `orElse`, …) to a method that isn't there. If a type you
depend on genuinely lacks what you need and it is in your write scope
(e.g. a `common` primitive — `Result`, `Converter`, `Closer`,
`ResultAssert`), add the method there and prove it compiles; if the type
is out of scope, emit the scope-exception envelope instead of leaving a
broken call for downstream.

## System-running roles: self-verify by running your slice

Compiling is the **floor** for author agents — the stateless ones that
write code and exit. The `system-implementer` role is the deliberate
exception: compile-green is not its done-signal. After implementing, it
is expected to build, (re)start the system, and run its own
`acceptance-<channel>` slice via the existing
`gh optivem system build` → `gh optivem system start --restart` →
`gh optivem test run --suite=acceptance-<channel>` commands, then debug
and re-run until that slice passes. For this role, running that slice is
part of the job, not a downstream-only step — your own task body owns the
loop and its stop rule. This carve-out is scoped to the
`system-implementer` alone: every other agent stays compile-only and never
runs the suite.

## Edit cohesion

Batch all edits to the same file into one `Write` or `Edit` call.

---
