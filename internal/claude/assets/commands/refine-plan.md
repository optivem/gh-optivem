Resolve a plan's **open questions** (each with a recommendation), then synthesize the resulting **target end-state** and write it as a summary at the top of the plan. No code changes are made — this command refines the *plan*, not the codebase. It does **not** walk every item.

## When to use which plan command

- `/refine-plan` (this one) — the plan has **open questions / unresolved decisions** you want settled. It asks you only those (each led by a recommendation), folds the answers in, then writes the resulting **target end-state** to the top of the plan. It does *not* present every item for Keep/Edit/Split.
- `/review-plan` — you want to check the plan against the **current code state** and prune items that are done/obsolete. No discussion per item.
- `/execute-plan` — you want to **implement** the plan. Code changes + commits.
- `/update-plan` — batch sync the plan with decisions already made earlier in *this* conversation. No per-item walk.
- `/explain-plan` — you want a **fast, high-level read** of the plan's *why* and *end result*. No discussion, no edits to items.

If the user invoked `/refine-plan` but the request sounds more like one of the others, point that out before starting.

## Input

The plan file is provided as `$ARGUMENTS`. If no argument is given, use the currently open file in the editor (from the IDE selection context). If no file is open either, ask the user which plan file to refine.

The plan file path can be:
- A filename in the current repo (e.g. `MIGRATION.md`)
- A relative path from the academy workspace root (e.g. `plans/20260518-foo.md`)
- An absolute path

Resolve the academy workspace root dynamically:
```bash
ACADEMY_ROOT="$(cd "$(git rev-parse --show-toplevel)/.." && pwd)"
```

## Plan format expectations

The plan file is free-form but typically has an H1 title, a TL;DR / Outcomes block near the top, numbered/bulleted items, and often an `## Open questions` section. Preserve the file's existing structure. Refinement touches only two things: (a) the open questions you resolve, and (b) the top-of-plan target-state summary it writes. Everything else (item bodies, phase narrative, cross-references) is left untouched unless resolving a question forces a downstream edit.

## Mark plan as picked up

Before starting, add a marker at the top of the plan file so anyone viewing it can see an agent is working on it:

> 🤖 **Picked up by agent (refine)** — `<hostname>` at `<ISO-8601 UTC timestamp>`

Obtain the values with:
- Hostname: `hostname`
- Timestamp: `date -u +%Y-%m-%dT%H:%M:%SZ`

Insert the marker immediately after the H1 title (or as the first line if there is no title). If a previous marker is already present (from `/execute-plan` or a prior `/refine-plan` run), replace it. Remove the marker when refinement finishes.

## Phase 1: Collect open questions

Scan the whole plan for unresolved decisions — not just one section:

- An explicit `## Open questions` (or `## Decisions to make`, `## TBD`) section.
- Inline markers anywhere in the body: `TODO`, `TBD`, `⏳ Deferred:`, `?`-tagged design forks, and prose like "decide during", "decide before", "settle during encoding", "to be confirmed", "pending <name> confirmation", or an unresolved `VJ:` / `AUTHOR:` question.

Present the collected list back to the user as a short numbered index, e.g.:

```
Found 2 open questions in the plan:
  1. Engine Context key names — string keys vs typed accessor
  2. ${attempt-block} wording — single pre-rendered block vs raw placeholders
I'll ask each one (with a recommendation), then write the target end-state to the top.
```

**If there are no open questions**, say so plainly and skip straight to Phase 3 (synthesis) — the plan is already decided; all that's left is to write/refresh the target-state summary.

## Phase 2: Resolve, one at a time

For each open question, in order:

1. **Present it verbatim** (the actual text from the plan) with its number, plus any inline annotation (`VJ:`, `AUTHOR:`) so the user remembers their own past notes.
2. **Give your read and a recommendation.** One or two sentences: what the tradeoff is and which way you lean.
3. **Ask one `AskUserQuestion`** — never batch, one per turn (standing preference; see `feedback_open_questions_one_at_a_time` in memory). Lead with your recommended answer as the **first option**, labelled with `(Recommended)` and a one-sentence rationale; offer the genuine alternative(s); always leave room for the user to dictate something else via "Edit".
4. **Write through immediately.** Once the user decides, fold the resolution into the plan with one targeted `Edit`: move the question into `## Resolved decisions` (create the section if absent), and delete it from `## Open questions` / remove the inline `TODO`/`Deferred`/"decide during" marker it came from. If resolving it forces a downstream edit to an item (the item contradicts the choice), make that edit now. Always `Edit`, never re-`Write` the whole file. Write per question — don't defer to the end — so a mid-walk interruption can't lose prior decisions.

**Shortcut signals.** If the user says "accept your recommendations", "pick the best long-term for all", or similar mid-resolution, stop the per-question `AskUserQuestion` prompts and **batch-resolve the remaining questions** in one pass using your recommended answer for each, writing the rationale into `## Resolved decisions` (see `feedback_autonomous_best_long_term` in memory). Then go to Phase 3.

When the `## Open questions` section is emptied, remove the now-empty heading.

## Phase 3: Synthesize and write the target end-state

This is the deliverable. Once every open question is resolved (or there were none):

1. **Derive the target end-state** from the now-fully-resolved plan — not a restatement of the steps, but the *end behavior*: what the system does differently when the work is done, what the user will observe (concrete: prompts, output, summaries, UI), and what is explicitly **unchanged**. Ground it in the resolved decisions.
2. **Show it to the user** in the chat response.
3. **Write it at the top of the plan** as the canonical target-state summary:
   - If the plan already has a top summary (`## TL;DR`, `## Outcomes`, an "End result:" line), **update that in place** so the resolved end-state is reflected there — do not create a second competing summary (duplicate top summaries are a drift source).
   - If the plan has no top summary, **insert a new `## Target state` section** immediately after the H1 title (and after the pickup marker, which is removed at wrap-up).
   - Keep it tight: the end logic / end outcome / what-the-user-sees / what's-unchanged. It should let a future reader understand where the plan lands without reading every step.

## Phase 4: Wrap-up

1. Remove the pickup marker.
2. Summarize what changed: open questions resolved (one line each, with the chosen answer), and that the target-state summary at the top was written/updated.
3. **Do not commit.** Leave the refined plan file as an uncommitted change so the user can review the diff. Mention that `/commit` or raw `git` will pick it up — and that `/refine-plan` does not run `git` itself.
4. Offer the natural next step: `/clear` then `/execute-plan <file>`.

## Rules

- **Open questions only, then synthesize.** Do not walk every item asking Keep/Edit/Split. The job is: resolve unresolved decisions, then write the target end-state. If the user wants to rework a specific item that isn't an open question, do it — but that's on request, not the default sweep.
- **One question per turn.** Never batch `AskUserQuestion`. Lead with a recommendation. (See `feedback_open_questions_one_at_a_time` and `feedback_flag_non_token_efficient` in memory.)
- **No code changes.** This command refines the plan only. If resolving a question surfaces something that needs a code change, note it in the plan (new item or annotation) — don't fix it here. Use `/execute-plan` afterward.
- **Write through, not batched.** Apply each resolution as a targeted `Edit` the moment the user decides — never re-`Write` the whole file, and don't defer all writes to the end (a mid-walk interruption must not lose prior decisions). Don't thrash either: hold still-fluid discussion in the chat until a point settles. (Same plan-file write policy as `/create-plan`.)
- **Preserve author voice and structure.** Inline annotations (`VJ:`, `AUTHOR:`) survive unless the user rewrites them. Headings, phase sections, and item bodies are not refined unless resolving a question forces it. The two sections refinement owns are the resolved open questions and the top-of-plan target-state summary.
- **No duplicate top summaries.** Update an existing TL;DR/Outcomes rather than adding a parallel `## Target state` beside it.
- **One plan file per invocation.** If the user wants to refine several, run the command once per file.
- **No `/clear` advice mid-resolution.** The resolution pass is interactive and stateful; clearing loses position. Save and exit first, then `/clear`.
