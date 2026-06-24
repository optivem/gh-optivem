Execute a plan file item by item, either step-by-step (with per-item approval gates) or batch-then-review (execute everything, then one review-and-commit gate at the end).

## Always surface a recommended answer (applies to every question)

**Whenever you ask the user a question in this command — mode choice, scope choice, an approval gate, a clarification, anything — always show which option you recommend and one sentence of why.** Never present a neutral menu and make the user choose blind.

- When using the `AskUserQuestion` tool, make the recommended option the **first** in the list and append `(Recommended)` to its label.
- When asking inline (plain prose), lead with the recommendation: *"I'd go with X because <reason> — or do you want Y?"*
- If the user just says "what do you recommend" / "go with your recommendation" / "you decide", take that as approval of your stated recommendation and proceed; don't re-ask.

## Input

The plan file is provided as `$ARGUMENTS`. If no argument is given, use the currently open file in the editor (from the IDE selection context). If no file is open either, ask the user which plan file to execute.

The plan file path can be:
- A filename in the current repo (e.g. `MIGRATION.md`)
- A relative path from the academy workspace root (e.g. `starter/MIGRATION.md`)
- An absolute path

Resolve the academy workspace root dynamically:
```bash
ACADEMY_ROOT="$(cd "$(git rev-parse --show-toplevel)/.." && pwd)"
```

## Plan format expectations

The plan file contains numbered items (e.g. `## Step 1:`, `### 1.`, `- [ ] Step 1`, or similar). Parse whatever numbering/format is used.

## Pre-flight: resolve open questions (gate — do this first)

**Before anything else — before advising on mode, marking pickup, or touching code — scan the plan for unresolved open questions and decisions.** Look for an `## Open questions` section, inline `TBD` / `TODO(decide)` / `???` / "needs user decision" markers, and any step whose action is conditional on a choice that hasn't been made.

- **If unresolved questions remain**, stop and present them to the user as a numbered list, with your recommendation for each, and ask them to resolve before you start. Do **not** begin execution against an under-specified plan — a guessed decision mid-execution is the most expensive kind of rework.
- **If the section explicitly says they're all resolved** (e.g. "Open questions: none — all resolved before execution"), proceed.
- **If there is no Open questions section at all**, do a quick judgment pass for hidden ambiguity; if the plan reads as fully specified, proceed.

This gate exists because plans are often drafted in one session and executed in another — the executor must confirm every decision the author left open is actually closed, not silently pick one.

## Token-efficient advice (always surface)

Before picking a mode, **always advise the user which option is most token-efficient for this specific plan and situation**, and recommend one. Don't just list the options — make a recommendation with one sentence of why.

The dimensions to consider:

- **Mode.** Batch-then-review is almost always cheaper than step-by-step: each per-item gate replays the prefix from the start of the conversation, so 10 gates = 10× the cached-prefix replay. Recommend step-by-step only when items are genuinely high-risk or high-ambiguity (the user's judgment is needed mid-flow, not just at the end).
- **Scope.** If the plan is large, recommend executing only one natural seam this session and finishing the rest in a fresh `/clear`-ed session. Cached prefixes grow with every read/edit; splitting on natural seams (engine → integration → driver → docs, or one repo at a time) keeps each session's prefix small. Look for explicit chunking guidance inside the plan first — many plans pre-declare "execute in N separate sessions"; respect that.
- **Parallelization.** When items are independent (e.g. "edit 9 different files, no cross-cutting changes"), recommend dispatching subagents in parallel rather than doing them sequentially in the main session. Subagent context is isolated from the main conversation, so the main session stays small while the work fans out.
- **Re-read budget.** Read each file once; use `Edit` afterward instead of re-reading. If the plan demands repeatedly re-checking the same large file, flag that as a token cost and propose either splitting the work or having a subagent own that file.

Phrasing template: *"Most token-efficient = batch-then-review, scoped to <seam>, with <N> parallel subagents. Sound good?"* The user can accept, ask for an alternative, or override.

## Hand-off at end of session (always surface)

Before exiting **any** session — whether all items finished, a stop-gate was hit, or you scoped to a subset — surface an explicit hand-off block to the user if any unresolved items remain in the plan:

```
Next session:
1. /clear
2. /execute-plan <relative-path-to-plan>
```

If the plan declares its own chunk ordering, name the next chunk explicitly: *"Next session is the `<name>` chunk; type `/clear` then `/execute-plan plans/foo.md` and I'll resume there."* Always include the literal slash-commands the user needs to type — don't make them remember.

If only deferred items remain, say so plainly and skip the hand-off block: deferred items are a backlog signal, not a next-session signal.

The hand-off block goes at the very end of the final response, after the per-repo commit summary.

## Execution modes

Three possible modes. **Before starting work, pick a mode** (after surfacing the token-efficient advice above):

1. **Auto-detect "Execute approved" first.** Scan the plan for decision annotations (`⏭️ Skipped`, `❌ Rejected`, `✏️ Modified`, `⏳ Deferred`). If any exist, assume a prior review pass happened and switch to **Mode: Execute approved** (see below). Do not ask the user in this case.

2. **Otherwise, ask the user which mode** — but lead with the recommendation, not a neutral menu:

   > For this plan, batch-then-review is most token-efficient because <one-sentence reason>. Sound good, or do you want step-by-step?

   Accept short answers: "step", "step-by-step", "one by one" → **Step-by-step**. "batch", "all", "everything", "batch-then-review", "yes", "sounds good" → **Batch-then-review**. If the user has already indicated a preference in their invocation message (e.g. "execute everything and ask me to review at the end", "whatever is most token efficient"), treat that as the answer and don't re-ask.

3. Respect **pre-approved items** in either mode (see below).

---

## Mark plan as picked up

Before starting execution in any mode (and after the mode is chosen), add a marker at the top of the plan file so anyone viewing the file can see an agent is working on it:

> 🤖 **Picked up by agent** — `<hostname>` at `<ISO-8601 UTC timestamp>`

Obtain the values with:
- Hostname: `hostname`
- Timestamp: `date -u +%Y-%m-%dT%H:%M:%SZ`

Insert the marker as the first line of the file (or immediately after the H1 title, if one exists). If a previous marker is already present, replace it with the new one.

Remove the marker when execution finishes — either the plan file is deleted (all items done) or only deferred items remain (delete just the marker line).

---

## Ensure the TL;DR block

After marking pickup, check whether the plan has a `## TL;DR` block near the top:

```
## TL;DR

**Why:** <1–3 sentences — the problem/motivation: what's missing or broken today and who feels it.>
**End result:** <1–3 sentences — what is true once the plan is fully executed: the observable end state.>
```

- **If it already exists, leave it untouched.** Do not re-derive it each run — that wastes tokens and risks drifting from the author's wording.
- **If it's missing,** synthesize one from the plan body (primarily its `## Problem` / `## Goal` sections) and insert it immediately after the H1 title and the pickup marker line, before the first detailed section. Keep it to the two-line shape above — a high-level digest, not a restatement of the plan.

This is the same block `/explain-plan` reads, so populating it here means a later `/explain-plan` is a pure read. The insertion rides along with the plan-file edits you commit during execution; no separate commit step is needed.

---

## Keep the ▶ Next executable step block current (resume contract)

Every plan should carry a `## ▶ Next executable step (resume here)` block so the user can re-enter it with bare `/clear` + `/execute-plan <file>` — no custom prompt. This block always names the **single** next concrete, executable unit, grounded enough to act on without re-deriving it.

- **If the block is missing,** synthesize one from the next incomplete item (its files/commands, the gate, what it unblocks) and insert it after `## Outcomes` (or after TL;DR if there are no Outcomes). Don't skip this because the plan "looks obvious" — the block is what spares the *next* session the reasoning you just did.
- **Keep it current as you go.** Whenever a unit completes and you remove its item, **replace the block's body with the new next unit** in the same edit. The block must never describe already-done work.
- **When only design/planning remains** (the next move is to draft or refine a child plan, not make a mechanical edit), rewrite the block to say so explicitly and name the plan to draft/refine — so the next executor switches to `/create-plan` or `/refine-plan` instead of hunting for edits. This is exactly the case that otherwise forces a confused back-and-forth on resume.
- **When the plan is fully done** (file about to be deleted), the block goes with it. If only deferred items remain, point the block at them or note that the rest is deferred.

The block's edits ride along with the plan-file changes you already commit during execution — no separate commit step.

---

## Pre-approved items

Before any approval gate, check whether the item in the plan file already contains a clear author decision — for example, an inline author comment (`VJ:`, `AUTHOR:`, `APPROVED`, etc.) with an explicit instruction like "create a ticket", "add a TODO in X", "yes do it", "reject", "skip". If the decision is unambiguous and the required action is obvious from that decision, treat the item as pre-approved:

- In **Step-by-step mode**: skip both the Phase 1 approval gate and the Phase 2 commit gate. State what you will do in one sentence, execute, summarize, commit.
- In **Batch-then-review mode**: no change — pre-approved items are executed alongside the rest; the single end-of-run review still applies to the batch as a whole.

Only fall back to normal approval gates when:
- The author decision is unclear ("not sure", "maybe", a question back to you, no comment at all).
- The task is ambiguous enough that multiple reasonable approaches exist and the author didn't pick one.
- Something unexpected comes up during execution that changes the plan.

When in doubt, ask. But don't re-ask for approval on something the author already decided.

---

## Mode: Step-by-step

For each incomplete item in the plan:

### Phase 1: Propose
1. Read and display the item description to the user.
2. Analyze what needs to be done: which files to change, in which repo, and how.
3. Present a concrete proposed solution (file changes, commands, etc.).
4. If the item is pre-approved, proceed to Phase 2 without asking. Otherwise ask: **"Approve this approach? (yes / modify / skip)"** and wait for the user's response. If "modify", incorporate their feedback and re-propose. If "skip", move to the next item.

### Phase 2: Execute
1. Implement the approved changes.
2. If the item involves code, run any relevant tests or validation to confirm correctness.
3. Show the user a summary of what was done (changed files, test results).
4. If the item is pre-approved, proceed directly to Phase 3 (commit) without asking. Otherwise ask: **"Review complete. Approve and commit? (yes / redo / skip)"** and wait for the user's response. If "redo", ask what to change and re-execute. If "skip", leave changes uncommitted and move on.

### Phase 3: Commit
1. Determine which repo the changes belong to (from the file paths modified).
2. Commit only that repo using the commit script with `--repo`:
   ```bash
   bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh" --repo <repo-name> "<item description>"
   ```
3. **Delete the item from the plan file immediately after committing.** This is mandatory — never move to the next item without removing the completed item first.
4. Report success, then move to the next item.

---

## Mode: Batch-then-review

Execute all incomplete items in sequence without asking per-item. Commit at the very end after one review gate.

### Phase 1: Execute all items
For each incomplete item:
1. Read the item, determine the work (files, commands), and execute the changes directly.
2. Run any relevant tests/validation to confirm correctness. If tests fail, try to fix within the scope of the item. If unfixable, stop and ask the user.
3. **Do not commit yet.** Leave all changes staged in the working tree.
4. **Delete the item from the plan file as soon as its work is done** — do not wait for the commit gate. The plan-file edit is itself an uncommitted change; if the user later chooses `discard`, revert the plan-file deletions along with the code changes.
5. If an item explicitly says "stop and ask user" (or equivalent — e.g. in Open Questions, in Cleanup sections marked destructive, in items marked "needs user decision"), stop at that point, present what you've done so far, and wait for the user. Do not continue past that item without approval.

### Phase 2: Present for review
Once all items in scope are executed (or you hit a stopping point):
1. Summarize by repo: list every file modified, grouped by repository.
2. For each repo, include: item IDs addressed (reference these from the summary, since they've already been removed from the plan file), key changes (not full diffs), test/build results.
3. Mention any items skipped or deferred and why.
4. Ask: **"Review complete. Approve to commit all changes? (yes / redo / ask-per-item / discard)"**
   - `yes` → proceed to Phase 3
   - `redo <item ID>` → roll that item's changes back and re-execute (this includes restoring the item to the plan file and re-deleting it once the redo work completes)
   - `ask-per-item` → drop into Step-by-step mode for the remaining commit gates (still one commit per repo, but ask per commit)
   - `discard` → revert all uncommitted changes, including the plan-file deletions

### Phase 3: Commit all
1. For each repo that has changes, run the commit script once with a message summarizing the items addressed for that repo:
   ```bash
   bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh" --repo <repo-name> "<summary of items>"
   ```
   The plan file's deletions (made in Phase 1) are part of these commits — no separate plan-file step is needed here.
2. Report per-repo commit results and summarize what's left in the plan (if anything — e.g. deferred items or items past a stop-gate that weren't executed).

---

## Mode: Execute approved

Only items annotated as approved-but-unimplemented (still `- [ ]` with no rejection/skip/defer marker) are executed. Everything else is skipped. Within this mode, follow **Step-by-step** semantics for each approved item (Phase 1/2/3 with gates, unless pre-approved).

---

## Tracking decisions

After each item is resolved (by any outcome), update the plan file to record what happened:

| Outcome | How to mark |
|---------|-------------|
| Approved & committed | **Delete the item** from the plan file |
| Modified & committed | **Delete the item** |
| Skipped | **Delete the item** |
| Rejected | **Delete the item** — but if the rejection creates new work (e.g. "do the opposite"), add a new item for that work |
| Deferred | `- [ ] Item N: ... — ⏳ Deferred: <reason>` |

**Resolved items are deleted** — the plan file should only show what's left to do. The git history is the record of what was done. Only deferred items remain visible.

Deletions happen **as soon as each item's work is done**, in both modes — Step-by-step deletes after each item's commit; Batch-then-review deletes during Phase 1, before the final commit gate.

**Whenever you delete a resolved item, update the `## ▶ Next executable step (resume here)` block** to name the new next unit (see "Keep the ▶ Next executable step block current" above) — in the same edit, so the plan is always resumable.

## Rules

- **Resolve open questions before starting** — run the pre-flight gate above; never begin execution while the plan has unresolved questions/decisions. Surface them with recommendations and wait for the user.
- **Keep the `## ▶ Next executable step` block current** — it is the plan's resume contract (bare `/clear` + `/execute-plan <file>`). Refresh it every time an item is resolved; when only design work remains, rewrite it to say so and point at the plan to draft/refine.
- Only commit the specific repo that was modified, never all repos. (Batch mode may still commit multiple repos — one commit per repo with changes, still using `--repo` each time.)
- Never bypass a gate a user would reasonably want: explicit "stop and ask user" markers in the plan, destructive operations (release deletion, force-push, dropping data), or actions visible to third parties (published releases, GitHub comments).
- If a item affects multiple repos, handle each repo with its own commit.
- If execution fails and cannot be fixed within the scope of the item, stop and explain the error. Do not auto-fix; propose a solution and wait for approval.
- After the last item, summarize what was completed and what was skipped.
