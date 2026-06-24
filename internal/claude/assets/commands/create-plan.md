Turn a rough idea into a new plan file, then discuss it with the user and refine the plan in place. Creates the file from the idea, leads with the outcomes (goals + what you get), fills in steps below, and only commits when the user confirms. No code changes — this builds the *plan*, not the codebase.

## When to use which plan command

- `/create-plan` (this one) — you have an **idea but no plan file yet**. Creates the file from scratch, then iterates with you. Outputs a new `plans/*.md` in the **current repo** (the repo of the working directory this chat is in).
- `/refine-plan` — a plan file **already exists**; walk it item by item and rewrite based on discussion.
- `/review-plan` — check an existing plan against the **current code state** and prune done/obsolete items.
- `/execute-plan` — **implement** an existing plan (code changes + commits).
- `/update-plan` — batch sync an existing plan with decisions already made earlier in this conversation.
- `/explain-plan` — fast, high-level read of an existing plan's *why* and *end result*.

If the user invoked `/create-plan` but a plan file for this idea already exists, point that out and offer `/refine-plan` instead.

## Input

The idea is provided as `$ARGUMENTS` (free-form: a sentence, a paragraph, a brain-dump). If no argument is given, ask the user for the idea in one line, then proceed.

## Where the plan is saved

Plans live in the **current repo's own `plans/` directory** — the repo of the working directory this chat is in, *not* a hardcoded central repo. A plan about a repo's code belongs with that code, so it travels with the repo and is found by anyone working there. Named `YYYYMMDD-HHMM-<slug>.md` (match the repo's existing plan-naming convention if its `plans/` already uses a different shape).

Resolve the path and timestamp dynamically (never hardcode):
```bash
REPO_ROOT="$(git rev-parse --show-toplevel)"
PLANS_DIR="$REPO_ROOT/plans"
mkdir -p "$PLANS_DIR"   # plans/ may not exist yet in this repo
TS="$(date -u +%Y%m%d-%H%M)"
```

Derive `<slug>` from the idea: 3–6 kebab-case words capturing the topic (e.g. `add-dark-mode-toggle`). Don't ask the user to approve the filename — derive it, state it, and move on. The user can rename later.

## Plan structure

The file leads with outcomes and puts steps below. Use this skeleton:

```markdown
# <YYYY-MM-DD HH:MM:SS UTC> — <Human-readable title>

## TL;DR

**Why:** <1–3 sentences — the problem/motivation behind the idea.>
**End result:** <1–3 sentences — what is true once the plan is fully executed.>

## Outcomes

What we get out of this — the goals and deliverables:

- <concrete outcome 1 — observable, not a task>
- <concrete outcome 2>
- ...

## ▶ Next executable step (resume here)

<The single next concrete, executable unit of work — grounded enough that a fresh agent running `/execute-plan <this file>` can act without re-deriving it: what to change, which files/commands, the gate to stop at, and what it unblocks. For a simple linear plan this just restates Step 1. For a multi-session or coordination plan it names the specific next move. If only design/planning remains (not a mechanical edit), say so explicitly and point at the plan to draft/refine — so the executor switches to `/create-plan` or `/refine-plan` instead of hunting for edits.>

## Steps

- [ ] Step 1: <action>
- [ ] Step 2: <action>
- ...

## Open questions

- <anything unresolved that the discussion still needs to settle>
```

- **`## TL;DR`** is mandatory and goes first — `/explain-plan` and `/execute-plan` key off this exact block, so populating it here means a later `/explain-plan` is a pure read.
- **`## Outcomes`** is the user's headline ask: goals and what you walk away with, stated as results ("dark mode persists across reloads"), not tasks ("add a toggle"). This is what the user reviews first.
- **`## ▶ Next executable step (resume here)`** is the **resume contract**: it lets the user re-enter the plan any time with just `/clear` + `/execute-plan <this file>` — no custom prompt. It always names the *single* next concrete unit, fully grounded. `/execute-plan` keeps it current (replaces it as each unit completes). Mandatory on every plan; for a trivial linear plan it can simply mirror the first open Step.
- **`## Steps`** is the how. Keep steps coarse at creation time; the discussion sharpens them.
- Drop **`## Open questions`** if there are none.

## Phase 1 — Draft from the idea

1. Read `$ARGUMENTS`. Do **not** scan the codebase or run tools unless the idea is too vague to draft outcomes from — creating the skeleton should be cheap. If the idea is genuinely ambiguous, ask **one** clarifying question (via `AskUserQuestion`), then draft.
2. Write the skeleton above to `$PLANS_DIR/$TS-<slug>.md` in **one `Write`**. Fill in TL;DR, Outcomes, and a first coarse pass at Steps. Mark anything you inferred (rather than the user stated) in `## Open questions` so it surfaces for discussion.
3. Show the user the **Outcomes** and **Open questions** in chat (not the whole file) and invite discussion: *"Drafted `<filename>`. Here's what I think you get out of it — what's off?"*

## Phase 2 — Discuss & refine (token-efficient)

This is a conversation. The user reacts; you adjust the plan. **Edit strategy matters for token cost:**

- **Hold fluid changes in the conversation; write through only when a point settles.** While a section is still churning (the user is thinking out loud, changing their mind), keep the working state in the chat — do **not** Edit the file on every exchange. The moment a decision is firm, apply **one targeted `Edit`** for just that section.
- **Always `Edit`, never re-`Write`.** Once the skeleton exists, every change is a surgical `Edit` of the affected lines — `Edit` sends only the diff; re-`Write` resends the whole file (~50× the tokens for a small change). Even several `Edit`s beat one full rewrite.
- **Batch settled changes per turn.** If a single reply from the user settles three things, apply them as a few `Edit`s in that turn — don't spread them across turns, and don't wait until the very end to write everything (a mid-discussion interruption would lose it).
- **Move resolved open questions** out of `## Open questions` and into the relevant Outcome/Step as they're answered.

The net rule: *don't rewrite the file on every message, and don't defer all writes to one giant update at the end — write each chunk through the moment it's settled, as a targeted `Edit`.*

Keep iterating until the user signals the plan is good ("looks right", "that's it", "done").

## Phase 3 — Commit (only on confirmation)

When the user signals the plan is ready, **do not commit automatically.** Ask:

> Plan ready. Commit `<filename>` to the `<current repo>` repo? (yes / not yet)

- **yes** → commit only the current repo (the one the plan file lives in), mirroring `/execute-plan`'s scoped commit:
  ```bash
  REPO="$(basename "$(git rev-parse --show-toplevel)")"
  bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh" --repo "$REPO" "Add plan: <human-readable title>"
  ```
- **not yet** → leave the file uncommitted and stop. Mention the user can run `/commit` later, or resume refining.

## Rules

- **No code changes.** This command produces a plan file only. If the discussion surfaces work, capture it as a Step — don't implement it here. Use `/execute-plan` afterward.
- **Outcomes first, steps second.** The file must lead with TL;DR + Outcomes. Steps live below. Don't bury the goals under a task list.
- **TL;DR block is mandatory and exact.** Use the `## TL;DR` / **Why** / **End result** shape verbatim so the rest of the plan tooling can read it.
- **`## ▶ Next executable step (resume here)` block is mandatory.** Every plan carries it so it can be resumed with bare `/clear` + `/execute-plan <file>`. Ground it enough to act on without re-derivation; for a trivial linear plan it may just mirror Step 1.
- **Write the skeleton once, then `Edit`.** One `Write` to create the file; every later change is a targeted `Edit`. Never re-`Write` the whole file to make a small change.
- **Write through settled chunks; don't batch-to-the-end.** Apply each firm decision as an `Edit` when it settles — so an interruption can't lose work — but don't thrash the file on still-fluid discussion.
- **Don't commit without confirmation.** Phase 3 is an explicit gate. Scope the commit to the current repo only (the one the plan file lives in).
- **Current repo, not a central one.** Save the plan in the current repo's `plans/` dir (`$(git rev-parse --show-toplevel)/plans`), never a hardcoded `courses/plans`. The plan lives with the code it concerns.
- **Dynamic paths only.** Resolve the repo root and timestamp at runtime; never hardcode a local path or date.
- **One plan per invocation.** If the user has several ideas, run the command once per idea.
