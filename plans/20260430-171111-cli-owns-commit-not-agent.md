# CLI owns commit, agent never touches git

## Motivation

Today's design has a contradiction between the CLI side and the agent side of dispatch.

- **CLI side** — `internal/atdd/runtime/clauderun/clauderun.go` polls `git HEAD` to detect agent completion, and `prompt.tmpl` line 14–16 instructs the agent: *"your COMMIT must land on HEAD before you exit."* The runtime is built on the assumption that the agent commits autonomously.
- **Agent side** — leaf agent definitions (e.g. `atdd-task.md` step 5) say *"After WRITE, STOP. Do NOT continue,"* and they import `docs/atdd/process/shared-commit-confirmation.md`, whose rule is *"No agent may run `git commit` … without first asking the user 'Can I commit?'"* The agent is built on the assumption that a human approves every commit interactively.

This contradiction surfaced during a v2b rehearsal on issue #61 (`rehearsal/atdd-cli` branch, 2026-04-30): the `atdd-task` agent finished WRITE, stopped at the commit gate, and asked the operator. The CLI's HEAD-poll never advanced because the agent (correctly per its own rules) never committed.

The clean resolution is to move the commit step out of the agent entirely:

1. The agent window is purely creative work — write, human reviews, agent reworks, loop until the human is satisfied.
2. The human exits the agent window when satisfied. Exit *is* the approval signal.
3. The CLI then stages the working-tree changes and commits with a templated message built from known context (phase, ticket, agent, diff stat).

Why this is the right split:

- **No agent intelligence is needed for the commit message.** Phase, ticket number/title, and agent name are already in `clauderun.Options`; `git diff --stat` supplies the file list. Asking the agent to compose a commit message is paying tokens for a mechanical step.
- **Single point of control.** Staging policy, message format, and (later) sign-off / hook handling live in one place instead of drifting across N leaf agents.
- **No `shared-commit-confirmation.md` rule to maintain.** That rule exists to gate a thing the agent shouldn't be doing in the first place.
- **Human gate stays where it belongs.** The WRITE-STOP inside the agent window already gives the human review/rework loop. We don't need a *second* "Can I commit?" gate after the human has already exited the window — exit is the gate.

## Items

Ordered by dependency. Items 1–4 are the core flip; 5–6 are coordination with shop / rehearsal-atdd-cli where the agent definitions live; 7 is cleanup.

### 1. Flip the completion signal in `clauderun.Dispatch`

**File:** `internal/atdd/runtime/clauderun/clauderun.go`

Today `Dispatch` (lines ~146–199):

1. Records `headBefore`.
2. Runs the subprocess.
3. Records `headAfter`.
4. Errors with `"exited cleanly but produced no commit"` if HEAD is unchanged.
5. Returns `CommitInfo{SHA, Subject}` from the agent's commit.

New behaviour:

1. Run the subprocess. Subprocess exit = human approval (interactive) or autonomous completion (`-p`).
2. After exit, check `git status --porcelain` for staged + unstaged changes.
3. If there are changes: stage everything tracked + new files in scope (see item 3 for scope policy), build the templated commit message (item 2), run `git commit`, return `CommitInfo` from the new HEAD.
4. If there are no changes: return a `CommitInfo{}` with a "no-op phase" marker — do **not** error. Some phases legitimately produce no diff (verify, agent decides nothing was needed).

The "HEAD unchanged" error path goes away. The HEAD-before/HEAD-after comparison goes away.

**Risk:** the agent could leave the working tree in a partial state mid-rework when the human exits early. Mitigation: in interactive mode, `clauderun` already streams to a TTY — the human is in the loop and chose to exit. We commit what's there. If they wanted to discard, they should have said so before exiting; we don't try to second-guess.

**Estimated effort:** half a day plus rewriting `clauderun_test.go`'s commit-detection cases.

### 2. Build a templated commit message in the CLI

**File:** `internal/atdd/runtime/clauderun/clauderun.go` (new helper) + a small template file alongside `prompt.tmpl`.

Inputs available from `Options`: `Agent`, `IssueNum`, `IssueTitle`, `IssueRepo`, `NodeDescription`. Plus `git diff --stat` of staged changes after item 1's staging step.

Proposed format:

```
<agent>(#<IssueNum>): <IssueTitle>

Phase: <NodeDescription>

<git diff --stat output>
```

Example (for the rehearsal #61 case):

```
atdd-task(#61): Redesigning New Order UI

Phase: SYSTEM UI TASK - WRITE

 monolith-typescript/.../home.tsx           | 2 +-
 monolith-typescript/.../new-order.tsx      | 2 +-
 ...
```

This is intentionally boring. If a phase needs richer prose, that's a sign the phase is doing too much, not a sign the template needs an LLM in the loop.

**Risk:** none. Worst case the message is terse but accurate.

**Estimated effort:** 2 hours including a unit test that asserts the rendered message for a fixed set of inputs.

### 3. Decide the staging policy

**File:** `internal/atdd/runtime/clauderun/clauderun.go`

`git status --porcelain` returns four kinds of paths after the agent runs:

- Modified tracked files (`M`) — always stage.
- New untracked files (`??`) — always stage. The agent created them on purpose.
- Deleted tracked files (`D`) — always stage.
- Pre-existing dirty paths from before dispatch — must NOT be picked up.

The dispatcher must snapshot `git status --porcelain` *before* running the subprocess and stage only the *delta* after. This already aligns with the v2b operational-hardening plan's item 2 ("detect leftover untracked files / wrong-branch commits") — the snapshot machinery is shared.

**Risk:** if items 1 and 2 of `20260430-144514-v2b-operational-hardening.md` land before this plan, reuse their snapshot. If after, build the snapshot here and let v2b item 2 reuse it. Either order works; just don't duplicate.

**Estimated effort:** 2–3 hours including tests covering each `porcelain` shape.

### 4. Update `prompt.tmpl`

**File:** `internal/atdd/runtime/clauderun/prompt.tmpl`

Remove lines 14–16:

```
When the agent finishes, do not summarise — exit cleanly. The Go driver
detects completion by polling HEAD; your COMMIT must land on HEAD before
you exit.
```

Replace with:

```
When the agent finishes, do not summarise and do not commit — exit
cleanly. The CLI will stage and commit your changes after you exit.
The agent must never run `git commit`, `git add`, or `gh issue close`.
```

The "do not commit" instruction is belt-and-suspenders alongside the agent-definition edits in items 5–6 — it covers the case where the host session hasn't yet pulled the latest agent definition.

**Estimated effort:** 30 minutes including the prompt-rendering unit test.

### 5. Strip commit gating from leaf agent definitions in `shop`

**Files (in the `shop` repo):**

- `.claude/agents/atdd/atdd-task.md`
- `.claude/agents/atdd/atdd-test.md` (if present)
- `.claude/agents/atdd/atdd-dsl.md`
- `.claude/agents/atdd/atdd-driver.md`
- `.claude/agents/atdd/atdd-backend.md`
- `.claude/agents/atdd/atdd-frontend.md`
- `.claude/agents/atdd/atdd-chore.md`
- `.claude/agents/atdd/atdd-release.md`
- (verify against `shared-commit-confirmation.md`'s import list — single source of truth)

For each leaf agent:

- Remove the `@docs/atdd/process/shared-commit-confirmation.md` import line.
- Keep the WRITE-STOP step (e.g. atdd-task.md step 5: *"After WRITE, STOP. Present the system + driver changes for human approval."*) — that's the human review point, still valid.
- Remove any agent-internal procedural language about staging, commit messages, or `git commit` (replace with a single line: *"Do not run git commands. The CLI commits your changes after you exit."*).

**Estimated effort:** 2–3 hours across all leaf agents, mostly mechanical.

### 6. Mirror item 5 in `rehearsal-atdd-cli`

**Files (in the `rehearsal-atdd-cli` repo):**

- Same paths as item 5.

`rehearsal-atdd-cli` is a frozen rehearsal checkout, so this only matters if a future rehearsal will run against it. If the rehearsal is one-shot, document the divergence in the rehearsal's plan rather than editing.

Decide which: either edit (≈1 hour) or document (10 minutes).

### 7. Delete `shared-commit-confirmation.md`

**File:** `docs/atdd/process/shared-commit-confirmation.md` (in `shop` and `rehearsal-atdd-cli`)

The file exists to enforce the "agent asks before committing" rule. Once the agent doesn't commit, the rule is gone — there's nothing left to confirm. Keeping a file by that name to describe "how the CLI commits" is a misleading filename, which is worse than no file.

Steps:

- Delete the file in `shop` and `rehearsal-atdd-cli`.
- Grep `cycles.md` and other process docs for references to `shared-commit-confirmation.md` and remove or rewrite them. The expected mentions are short pointers like "see shared-commit-confirmation.md" — these can be deleted outright since the new flow needs no equivalent gate doc.
- If a CLI commit policy doc proves valuable later (operator confusion, audit requirement), write it fresh under an accurate name — e.g. `cli-commit-policy.md` — or fold a short paragraph into `cycles.md`. Do not pre-emptively create one as part of this plan.

**Estimated effort:** 30 minutes including the grep-fix pass.

## Out of scope

- The WRITE-STOP gate inside the agent window. That stays; this plan is about who runs `git commit`, not whether the human reviews.
- Phase boundary gates between agents (e.g. "human confirms before the next agent starts"). If we want those, that's a separate plan in the CLI orchestrator, not here.
- Sign-off, GPG signing, hooks. The CLI commits without `--no-verify` by default; whatever pre-commit hooks the repo defines will run. Sign-off / GPG is a follow-up.

## Order of operations for landing this

1. Land items 1–4 in `gh-optivem` behind a `--cli-commits` flag (default off) so existing rehearsals keep working.
2. Land item 5 in `shop`.
3. Flip the default in `gh-optivem` (`--cli-commits` becomes default-on, with `--agent-commits` as the legacy escape hatch).
4. Land item 7 (delete / repurpose the shared doc) once the default has flipped and rehearsals have run green.
5. Remove `--agent-commits` after one full soak window.
