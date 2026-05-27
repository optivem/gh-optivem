# Plan: Three switchable Claude Code approval modes

## Context

Today the shop repo enforces human-gates (commit, push, issue-close, local system tests) through **memory entries** — `feedback_ask_before_commit.md` and `feedback_ask_before_local_system_tests.md`. These are unconditional: Claude always asks.

The user wants the same gates to be **switchable** on demand, so a single session can be cautious during exploration and autonomous during routine cleanup without rewriting memory each time.

Three target modes:

| Mode             | Behavior |
|------------------|----------|
| `cautious`       | Ask approval before every mutation. Default when no marker file exists. |
| `commits-only`   | Auto-work on edits/reads/refactors; ask only before commit / push / issue-close / local system tests. |
| `autonomous`     | Proceed without asking. Escalate to human only when blocked, ambiguous, or about to do something destructive while uncertain. |

Mechanism: a per-project marker file `.claude/current-mode.md` that CLAUDE.md imports via `@`. Three slash commands at user-global level write the marker; one command reads it back.

The behavioral marker is the primary mechanism (since `Bash(*)` is already in the allow list, permission rules are not what gates Claude today — memory is). The `autonomous` command additionally flips `permissions.defaultMode` to `bypassPermissions` in `.claude/settings.local.json` as belt-and-braces.

## Files to create (4 user-global slash commands)

### 1. `C:\Users\valen_4rjvn9e\.claude\commands\mode-cautious.md`

```markdown
Switch this project to **cautious** approval mode.

Steps:
1. Write `.claude/current-mode.md` (overwrite) with:

       # Approval Mode: cautious

       Ask the user for explicit approval before EVERY action that mutates
       state: commits, pushes, issue close/comment, file deletion, system
       tests, builds, destructive shell commands, dependency installs.

       When in doubt, ask. Default to stopping.

2. Unset `permissions.defaultMode` in `.claude/settings.local.json` if and only if its current value is `"bypassPermissions"` (only what `/mode-autonomous` set; leave any operator-chosen value alone):
   - If the file does not exist, do nothing.
   - Parse the file as JSON (strict — fail loudly on parse error).
   - If `permissions.defaultMode == "bypassPermissions"`, delete that key. If `permissions` then becomes `{}`, delete `permissions` too.
   - Leave `permissions.allow`, `permissions.deny`, `permissions.ask`, and every other key untouched.
   - Write back with 2-space indent and a trailing newline.

3. Reply in ONE line: `Mode: cautious (ask before every mutation).`
```

### 2. `C:\Users\valen_4rjvn9e\.claude\commands\mode-commits-only.md`

```markdown
Switch this project to **commits-only** approval mode.

Steps:
1. Write `.claude/current-mode.md` (overwrite) with:

       # Approval Mode: commits-only

       Auto-proceed on read, edit, write, refactor, search, plan, test design.
       PAUSE and ask explicit approval before any action in these categories:

         a. Producing or rewriting commits (anything that creates a commit
            object): git commit, git commit --amend, git revert, git
            cherry-pick, git rebase, gh optivem commit, /commit. Staging
            (git add) when a commit is the obvious next step also counts.

         b. Publishing to a remote or to shared trackers: git push, gh issue
            create/comment/close/reopen/edit/lock, gh pr
            create/comment/close/reopen/merge/review, gh release
            create/edit/delete, any post to Slack/email/external service.

         c. Long-running local builds and system tests that consume the
            terminal for minutes: gh optivem test/run/stop system,
            ./test-all.sh, ./compile-all.sh, ./gradlew build,
            npx tsc --noEmit, dotnet build. (Read-only --sample flags and
            compile-only commands are NOT exempt — they still pause.)

         d. Destructive or hard-to-reverse operations (the principle is
            "would I want a human to confirm this before I do it on their
            machine?"): rm -rf, git reset --hard, git push --force, git
            branch -D, dropping a database, history rewrites, mass file
            deletion, force-overwriting uncommitted work.

         e. Dependency or environment mutations: npm install/uninstall,
            pip install, dotnet add package, brew install, modifications
            to lockfiles, Dockerfile rebuilds, mise/asdf tool installs.

2. Unset `permissions.defaultMode` in `.claude/settings.local.json` using the same JSON-safe procedure as `/mode-cautious` step 2 (delete the key only if its current value is `"bypassPermissions"`).

3. Reply in ONE line: `Mode: commits-only (auto-work, ask before commit/push/system-tests).`
```

### 3. `C:\Users\valen_4rjvn9e\.claude\commands\mode-autonomous.md`

```markdown
Switch this project to **autonomous** approval mode.

Steps:
1. Write `.claude/current-mode.md` (overwrite) with:

       # Approval Mode: autonomous

       Proceed without asking. Escalate to the human ONLY when:
         1. Intent is genuinely ambiguous and a wrong guess is costly.
         2. About to perform a destructive op (force push, hard reset, mass
            delete, history rewrite) and you are NOT certain it matches user
            intent.
         3. A test/build fails and two fix attempts have not resolved it.
         4. Hitting auth, network, or quota errors you cannot resolve.
         5. Discovering scope creep that materially changes the original
            request.
         6. About to commit code that does not compile or has failing tests.

       Otherwise: commit, push, close issues, run system tests freely.

2. Set `permissions.defaultMode = "bypassPermissions"` in `.claude/settings.local.json`, preserving every other key:
   - If the file does not exist, start from `{}`.
   - Parse the file as JSON (strict — fail loudly on parse error rather than overwriting).
   - If `permissions` is missing, add it as `{}`.
   - Set `permissions.defaultMode = "bypassPermissions"`. Do not touch `permissions.allow`, `permissions.deny`, `permissions.ask`, or any other top-level key.
   - Write back with 2-space indent and a trailing newline.

3. Reply in ONE line: `Mode: autonomous (escalate only when blocked or uncertain).`
```

### 4. `C:\Users\valen_4rjvn9e\.claude\commands\mode.md`

```markdown
Read `.claude/current-mode.md` and report the current mode in one line.

If the file does not exist, reply: `Mode: cautious (default — no marker file).`
```

## Files to edit (6)

### 5. `C:\GitHub\optivem\academy\shop\CLAUDE.md`

Append after the existing "Fixing Failing Workflows" section:

```markdown
## Approval mode

This repo supports three switchable approval modes. The active mode is stored
at `.claude/current-mode.md` (gitignored, per-developer-machine state).

- **cautious** (default if no marker) — ask before every mutation.
- **commits-only** — auto-work, ask before commit / push / issue-close / local system tests.
- **autonomous** — proceed; escalate only when blocked or uncertain.

Switch with `/mode-cautious`, `/mode-commits-only`, `/mode-autonomous`. Inspect with `/mode`.

@.claude/current-mode.md
```

The `@.claude/current-mode.md` line auto-imports the marker into Claude's context at session start (when the file exists).

### 6. `C:\GitHub\optivem\academy\shop\.gitignore`

Under the existing `# Claude` block (line 18), add line 21:

```
.claude/current-mode.md
```

### 7. Two memory entries (make conditional on mode)

**`C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-shop\memory\feedback_ask_before_commit.md`** — rewrite body to:

> Before any `git commit`, `git push`, or `gh issue close`/`gh issue comment`, check `.claude/current-mode.md`:
>
> - **cautious** / **commits-only** / marker missing → ASK "Can I commit?" with the proposed message + summary of staged changes; wait for explicit yes.
> - **autonomous** → proceed without asking; still escalate if the commit would include non-compiling code or failing tests.
>
> **Why:** original incident — Claude auto-committed without a user gate, leading to a noisy revert. The gate is now conditional on mode rather than unconditional, so autonomous workflows can opt out explicitly.
> **How to apply:** every commit/push/issue-close action; read the marker first.

**`C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-shop\memory\feedback_ask_before_local_system_tests.md`** — rewrite body to:

> Before running `gh optivem test/run/stop system`, `./test-all.sh`, `./compile-all.sh`, `./gradlew build`, `npx tsc --noEmit`, or `dotnet build`, check `.claude/current-mode.md`:
>
> - **cautious** / **commits-only** / marker missing → emit `About to run <cmd> locally — ~<N> min. Approve? (yes/no)` and wait.
> - **autonomous** → run freely; escalate only if the command fails repeatedly or hangs.
>
> **Why:** local system-test runs take minutes and consume the user's terminal. The gate is now conditional on mode rather than unconditional.
> **How to apply:** every local test/build invocation; read the marker first. (Read-only `--sample` flags and compile-only `tsc --noEmit` still count — they still block.)

### 8. `C:\GitHub\optivem\academy\gh-optivem\CLAUDE.md`

Append the same `## Approval mode` section used in shop (verbatim — `cautious` / `commits-only` / `autonomous` description plus `@.claude/current-mode.md` import). The slash commands are user-global so they already work in this repo; what's missing is the marker-file import and the conditional memory rewrite below. Without these two edits, modes silently no-op in gh-optivem.

### 9. `C:\GitHub\optivem\academy\gh-optivem\.gitignore`

Add `.claude/current-mode.md` under the existing Claude-related gitignore block (or create one if none exists, mirroring shop's layout).

### 10. `C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-gh-optivem\memory\feedback_no_commit_without_approval.md`

Rewrite the body to be mode-conditional, mirroring the shop rewrite in item 7:

> Before any `git commit`, `git commit --amend`, `gh optivem commit`, `/commit` skill, or any commit-producing command (including pre-commit `git add` when a commit is the obvious next step), check `.claude/current-mode.md`:
>
> - **cautious** / **commits-only** / marker missing → ASK with a one-line "ready to commit — approve?" prompt + the proposed message; wait for an explicit yes that names the commit action.
> - **autonomous** → proceed without asking. Still escalate if the commit would include non-compiling code, failing tests, or scope creep beyond the original request.
>
> **Why:** original incident — autonomous commits bypassed user review control. The gate is now mode-conditional so autonomous workflows can opt out explicitly.
> **How to apply:** every commit-producing action across all repos in this workspace (gh-optivem, shop, scaffolded repos, rehearsal worktrees); read the marker first.
> Related: [[feedback_use_commit_skill]] still governs *which* commit mechanism to use once approval is given.

## Default state

When `.claude/current-mode.md` does not exist, behavior defaults to **cautious** — matching today's hardcoded memory gates. The two memory rewrites above explicitly handle the "marker missing" case as cautious, so removing the marker safely restores current behavior.

## Verification

Smoke-test each mode in isolation, in this order:

1. **default (no marker)** — delete `.claude/current-mode.md` if present, run `/mode`. Expect: `Mode: cautious (default — no marker file).`

2. **cautious** — run `/mode-cautious`, then ask Claude to "fix a typo in README.md and commit it". Expect: edit applied, then explicit "Can I commit?" prompt with the staged diff before any `git commit`.

3. **commits-only** — run `/mode-commits-only`, then "rename variable `foo` to `bar` across `src/`, compile, then commit". Expect: edits applied without asking, **pause** before `dotnet build` / `npx tsc --noEmit`, **pause** before `git commit`.

4. **autonomous** — run `/mode-autonomous`, then "add a `notes` field to the Customer entity, compile all three languages, and commit". Expect: edit + compile + commit + push with no prompts. Then trigger an escalation: ask Claude to "force-push main to overwrite remote" — expect refusal/escalation (criterion #2 in the marker).

5. **persistence across sessions** — switch to `commits-only`, exit Claude Code, restart, run `/mode`. Expect: `Mode: commits-only`. (Confirms CLAUDE.md `@` import works.)

6. **settings.local.json belt-and-braces** — after `/mode-autonomous`, inspect `.claude/settings.local.json` — confirm `permissions.defaultMode: "bypassPermissions"` is present and the three existing mkdir/mv/rmdir allow rules are preserved. After `/mode-cautious`, confirm `defaultMode` is removed and the three allow rules still present.

## Resolutions (refine pass 2026-05-27, autonomous best-long-term)

Four under-specified areas in the original draft were resolved in this pass. Recorded here so the rationale survives — revert by deleting the relevant edits if you disagree.

1. **JSON merge algorithm specified** (items 1, 2, 3). The original "Merge into `.claude/settings.local.json`" left the procedure implicit, which invited slash-command implementations that string-mangle JSON, overwrite operator keys, or fail to preserve `permissions.allow`. Now each command spells out: parse-strict, mutate one key only, write back with stable formatting. `/mode-cautious` and `/mode-commits-only` only unset `defaultMode` when its current value is `"bypassPermissions"` — so an operator who set `defaultMode: "default"` or `"acceptEdits"` by hand keeps their choice.

2. **Destructive-ops list reframed as principle + examples** (item 2). The original `rm / git reset --hard / git push --force / branch -D` enumeration would have rotted as new dangerous commands appeared. Now `mode-commits-only.md` carries five labelled categories (a–e) and states the underlying principle ("would I want a human to confirm this on their machine?"), with the original commands as examples. This is also where `git commit --amend`, `git revert`, `git cherry-pick`, `git rebase`, and pre-commit `git add` were added — they all produce/rewrite commits and were missing.

3. **GitHub-mutation list extended** (item 2). The original `gh issue close / comment` missed `create`, `reopen`, `edit`, `lock`, and the entire `gh pr` and `gh release` surface. The extended list now covers all state-mutating GitHub commands the workspace actually uses.

4. **Scope extended to gh-optivem** (new items 8, 9, 10). The original plan only edited `shop/`. But gh-optivem has its own `feedback_no_commit_without_approval.md` that would still gate unconditionally — so the user could not switch modes when working in this repo, including when working on this very plan. The three new items mirror the shop pattern in gh-optivem (CLAUDE.md @-import, .gitignore entry, conditional memory rewrite). If you prefer to keep modes shop-only, delete items 8–10 and the corresponding Critical-files entries before `/execute-plan`.

Deferred (not resolved this pass): nothing. All four implicit ambiguities are addressed above.

## Critical files

- `C:\Users\valen_4rjvn9e\.claude\commands\mode-cautious.md` (new)
- `C:\Users\valen_4rjvn9e\.claude\commands\mode-commits-only.md` (new)
- `C:\Users\valen_4rjvn9e\.claude\commands\mode-autonomous.md` (new)
- `C:\Users\valen_4rjvn9e\.claude\commands\mode.md` (new)
- `C:\GitHub\optivem\academy\shop\CLAUDE.md` (append `## Approval mode` section + `@.claude/current-mode.md` import)
- `C:\GitHub\optivem\academy\shop\.gitignore` (add `.claude/current-mode.md` under `# Claude`)
- `C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-shop\memory\feedback_ask_before_commit.md` (conditional rewrite)
- `C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-shop\memory\feedback_ask_before_local_system_tests.md` (conditional rewrite)
- `C:\GitHub\optivem\academy\gh-optivem\CLAUDE.md` (append `## Approval mode` section + `@.claude/current-mode.md` import)
- `C:\GitHub\optivem\academy\gh-optivem\.gitignore` (add `.claude/current-mode.md`)
- `C:\Users\valen_4rjvn9e\.claude\projects\C--GitHub-optivem-academy-gh-optivem\memory\feedback_no_commit_without_approval.md` (conditional rewrite)
