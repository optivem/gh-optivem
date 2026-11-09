# 2026-11-09 11:59 UTC — Refactor & Review: code and architecture review, review-first

## TL;DR

**Why:** Today the `refactor` menu runs `system-refactorer` / `test-refactorer` directly. Nothing reviews the code first, the human never approves a plan, and nothing captures the issues a human finds. The executor/reviewer/approver article and the Circle session describe a review-first Refactor:
1. A reviewer agent produces findings and a plan, without changing anything.
2. The human approves the plan.
3. The agent executes it.
4. The tests run.
5. The human reviews the result.
6. Every issue the human finds goes into the guidelines, and the agent fixes other occurrences across the codebase.

**End result:** The `refactor` menu offers "Code & Architecture Review", which runs that loop. Guideline files grow from what humans catch, so over time the reviewer catches more and the human less.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md` ("Refactor" and "Review Code & Architecture" diagrams). Decided there:
- Q5: verification keeps the full suite; narrowing is parked in `plans/20261109-1250-refactor-touched-use-case-tests.md`.
- Q6: UX/UI review is a separate, later plan.

## Outcomes

- A new `code-architecture-reviewer` agent: read-only, reads the guidelines files, outputs findings plus a refactoring plan.
- A new process `review-code-and-architecture`: `RUN_REVIEWER → APPROVE_PLAN → EXECUTE_PLAN → build/start → verify (unit + full AT suite) → COMMIT_REFACTOR → HUMAN_REVIEW → (issues?) UPDATE_GUIDELINES → loop`.
- `GATE_REFACTOR_TYPE_CHOICE` gets a "Code & Architecture Review" option.
- A guidelines feedback agent step: fix the issue, update the guidelines file, find and fix other occurrences.

## ▶ Next executable step (resume here)

Review this plan with the user, especially the guidelines-file question below. After that, Step 1 can run via `/execute-plan`.

## Steps

- [ ] Step 1: Decide where the guidelines live (see open questions) and add a `${...}` placeholder for them in gh-optivem.yaml.
- [ ] Step 2: Write `code-architecture-reviewer.md` (reuse the adversarial-review shared chunk from `plans/20261109-1257-red-reviewer-agents.md`) and pin its read-only scope.
- [ ] Step 3: Decide whether `EXECUTE_PLAN` reuses `system-refactorer` / `test-refactorer` with the approved plan as input, or uses a new executor. Recommended: reuse, passing the plan through a placeholder.
- [ ] Step 4: Add the `review-code-and-architecture` process and the menu option in `process-flow.yaml`. Cap the human-issue loop with `max-visits`, and audit the loop for statemachine deadlocks before running tests.
- [ ] Step 5: Add the guidelines-update step (agent) with write scope limited to the guidelines files plus the occurrences it fixes.
- [ ] Step 6: Tests. Run with `-p 2`.

## Open questions

- Where do the guidelines live? Options: the scaffolded repo's `CLAUDE.md`, a dedicated `docs/guidelines/` folder, or per-area files (architecture, naming). The article warns that big guideline files degrade results, so they must be prunable.
- Is this offered only inside a story's opportunistic refactor, or also as a `task/system-refactor` ticket subtype?
- How does `HUMAN_REVIEW` capture "issues found" in a structured way the next agent can consume (free text vs. a list)?
