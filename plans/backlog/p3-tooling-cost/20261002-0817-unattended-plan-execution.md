# 2026-10-02 08:17:00 UTC — Unattended, token-efficient plan execution

## TL;DR

**Why:** Executing a plan with `/execute-plan` currently needs the user online to `/clear` and re-run whenever context fills up. That is manual babysitting and wastes the user's time.
**End result:** `/execute-plan` (autonomous by default) runs a whole plan without the user present: each item is done in a fresh, minimal context, progress is persisted in the plan file, and the user only returns for real decisions or the final result.

## Outcomes

- The user starts execution once and does not have to `/clear` and re-run between items.
- Each item runs in a fresh context (no accumulated history), so token cost per item stays flat instead of growing.
- Progress survives interruption: the plan file is the single source of truth (items removed as done, `▶ Next executable step` kept current), so a crash or stop resumes cleanly.
- Genuine blockers/decisions are surfaced (recorded in the plan and reported at the end), not silently guessed or swallowed.
- Commits go through the commit skill/script only, incrementally per verified item; no raw git.

## Decisions

- Autonomous execution is the **default** mode of `/execute-plan` (no flag needed): ask all questions upfront, work via subagents, commit incrementally, stop mid-run only for important decisions that are expensive to reverse, hand back at the end (asking for test/review if needed). Step-by-step and batch-then-review remain available on request. The skill text for this is already written; the remaining steps harden it.
- Permissions: pre-allow a **narrow** set (Read/Grep/Glob, Edit/Write in the repo, repo build/test commands, `/commit`) via project settings (e.g. `/fewer-permission-prompts`); no blanket bypass. Anything outside the set halts the run and is recorded in the plan (fail loud, no guessing).
- Commit: **incrementally, one commit per verified item** (per repo), and only after the item's checks pass, so an unattended run never pushes partial or failing work.

## ▶ Next executable step (resume here)

Step 5: configure the narrow pre-allow list in project settings (`/fewer-permission-prompts`) and verify an out-of-set tool call halts the run with a recorded reason. The Autonomous-mode contract, rails and authoring rules are already in `execute-plan.md` and `create-plan.md`.

## Steps

- [ ] Step 5: Configure the narrow pre-allow list in project settings and verify an out-of-set tool call halts the run with a recorded reason.
- [ ] Step 8 (remainder): independent fresh-context reviewer subagent per diff; append one line per item (item, outcome, files, tokens) to a run-log file.
- [ ] Step 10 (remainder): read-only/dry-run pass for risky items; calibrated end-of-run report; after first runs, compare run-log numbers across options A/B and tune.
- [ ] Step 11: Document the mode in `docs/rules` Plan Processing; dry-run on a small existing plan to verify token usage and resume behavior.

## Open questions

- **Q-9 (resolved 2026-11-02):** Option A (an orchestrator plus one subagent per item) is the default mode; B (headless `claude -p` loop) is the escape hatch for very long plans. C and D are not pursued now.

## Later (not in scope now)

- Dry-run mode: workers print intended changes without applying them; useful for the first `--unattended` runs.
- Idempotent, resumable items: write items as "make X true", so re-running a half-finished item is safe.
- Escalation queue: ambiguous decisions queued in the plan while the run continues with unblocked items.
- Notify on finish or halt (`PushNotification`).
- Explicitly out: agent self-modifying its allowlist or the skill mid-run; parallel workers on overlapping files.
