Give a high-level summary of a plan — the *why* and the *end result* — by reading the plan's `## TL;DR` block, or synthesizing one (and persisting it to the top of the plan) when it's missing. Read-only on the codebase; the only file it may touch is the plan itself.

## When to use which plan command

- `/explain-plan` (this one) — you want a **fast, high-level read** of what a plan is for and what the world looks like once it's done. No discussion, no code changes, no per-item walk. Cheapest of the plan commands: it reads one file.
- `/refine-plan` — talk through each item and rewrite the plan based on the conversation.
- `/review-plan` — check the plan against the current code state and prune done/obsolete items.
- `/execute-plan` — implement the plan (code changes + commits).
- `/update-plan` — batch sync the plan with decisions already made earlier in this conversation.

If the user invoked `/explain-plan` but the request sounds more like one of the others, point that out before starting.

## Input

The plan file is provided as `$ARGUMENTS`. If no argument is given, use the currently open file in the editor (from the IDE selection context). If no file is open either, ask the user which plan file to explain.

The plan file path can be:
- A filename in the current repo (e.g. `MIGRATION.md`)
- A relative path from the academy workspace root (e.g. `plans/20260530-1725-foo.md`)
- An absolute path

Resolve the academy workspace root dynamically:
```bash
ACADEMY_ROOT="$(cd "$(git rev-parse --show-toplevel)/.." && pwd)"
```

## What it does

This command is a **read**, not an analysis pass. Do not scan the codebase, run tests, or open files other than the plan. The whole point is that the summary is cheap.

### Phase 1 — Find or build the TL;DR

1. Read the plan file (one `Read`).
2. If a `## TL;DR` block (see format below) already exists near the top, that is the canonical summary — use it verbatim. **Do not re-derive it.**
3. If no `## TL;DR` block exists (an older plan, or one not yet executed), synthesize one from the plan body — primarily its `## Problem` / `## Goal` sections, falling back to the title and opening prose. Keep it to the two-line shape below; this is a *high-level* digest, not a restatement of the whole plan.

### Phase 2 — Present

Show the user:
- The plan title.
- The **Why** and **End result** lines.
- One short orientation line if useful — e.g. how many open items remain, whether the plan declares a dependency on another plan, or whether a pickup marker shows an agent is mid-execution. Keep this to a single line; the user can open the file for detail.

Do not dump the full plan. If the user wants more, they can ask or run `/refine-plan` / `/execute-plan`.

### Phase 3 — Persist (only if synthesized)

If you synthesized the TL;DR in Phase 1 (it wasn't already in the file), write it into the plan so the next `/explain-plan` is a pure read:

- Insert the `## TL;DR` block immediately after the H1 title and the pickup marker line (if present), and before the first detailed section (`## Problem`, `## Goal`, a blockquote dependency note, etc.).
- Make no other edits. Leave the change uncommitted; mention that `/commit` or raw `git` will pick it up. `/explain-plan` does not run `git` itself.

If the TL;DR already existed, make **no** file edits at all.

## TL;DR block format

```
## TL;DR

**Why:** <1–3 sentences — the problem/motivation: what's missing or broken today and who feels it.>
**End result:** <1–3 sentences — what is true once the plan is fully executed: the observable end state.>
```

Keep both fields tight. The detailed `## Problem` / `## Goal` sections carry the depth; the TL;DR is the elevator version.

## Rules

- **One file, read-mostly.** Read only the plan. The only write allowed is inserting a `## TL;DR` block that was missing (Phase 3). Never touch the codebase.
- **Don't re-derive an existing TL;DR.** If the block is present, surface it as-is. Re-synthesizing every run defeats the purpose and risks drift from the author's wording.
- **No commits.** Leave any TL;DR insertion as an uncommitted change for the user to review.
- **No per-item walk.** This is a summary, not a review. For item-level work use `/refine-plan`, `/review-plan`, or `/execute-plan`.
- **One plan file per invocation.**
