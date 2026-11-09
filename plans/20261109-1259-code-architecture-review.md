# 2026-11-09 11:59 UTC — Refactor & Review: code and architecture review, review-first

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-11-09T15:35:52Z`

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

Step 4, once `internal/atdd/process/gates/bindings.go` has no uncommitted edits from another session. It was dirty for the whole first run, so the gateway registration could not be added, and the YAML cycle can't land without it: an unregistered binding fails `Bind` for every run. The code is written and parked in the repo:
- `internal/atdd/process/gates/code_review.go`: the four gates plus `registerCodeReview` (committed, not yet called).
- `plans/20261109-1259-step4-wip/review-code-and-architecture.yaml`: the cycle block.
- `plans/20261109-1259-step4-wip/code_review_loop_test.go.txt`: the loop test (7 tests).

## Steps

Done (first commit): `${guidelines-path}` (optional `system-test.paths.guidelines-path`, default `docs/guidelines`), `code-architecture-reviewer` and `guidelines-updater` agents with their MIDs and scopes (`review-code-architecture`, `update-guidelines`), the generic/Red split of the adversarial-review chunk, the refactorers' Checklist now also carrying an approved plan, and the review's three actions.

- [ ] Step 4: Wire the Code & Architecture Review into the menu:
  1. In `bindings.go`: call `registerCodeReview(r, b)` in `RegisterAll`, and add `review-code-and-architecture` to `refactorTypeChoice`'s `allowed` list and prompt string (before `none`).
  2. In `process-flow.yaml`: paste the parked cycle block above `refactor-system-structure`'s doc-block (`# Step 1 calls HIGH ...`). In `refactor`, add the `REVIEW_CODE_AND_ARCHITECTURE` call-activity (process `review-code-and-architecture`, name "Code & Architecture Review"), add the edge `GATE_REFACTOR_TYPE_CHOICE -> REVIEW_CODE_AND_ARCHITECTURE` with `when: "refactor-type-choice == review-code-and-architecture"` and the back-edge to `GATE_REFACTOR_TYPE_CHOICE`, change "emits the 5" to "the 6" in the gateway comment, and name the review in the menu doc-block.
  3. Tests: move the parked test to `internal/atdd/process/code_review_loop_test.go`. In `transitions_test.go`, add `review-code-and-architecture` to the CYCLE list and add two entries to `TestBackEdgesAreAudited`: `refactor: REVIEW_CODE_AND_ARCHITECTURE -> GATE_REFACTOR_TYPE_CHOICE` and `review-code-and-architecture: COMMIT_GUIDELINES -> HUMAN_REVIEW` (capped by max-visits 3 on UPDATE_GUIDELINES). In `gates/bindings_test.go`, add the four binding names to `TestRegisterAll_AllBindingsRegistered` and `review-code-and-architecture` to `TestRefactorTypeChoice`'s answers. The parked work's loop audit is done (see the test's doc comment). Run `go test -p 2 -timeout 300s ./internal/atdd/... ./internal/engine/statemachine ./internal/kernel/...`.
  4. Delete `plans/20261109-1259-step4-wip/`, then this plan, and mark the 1259 row done in the parent.

## Decisions (accepted by the user 2026-11-09)

- **Guidelines live in per-area files under `docs/guidelines/`** in the scaffolded repo (e.g. `architecture.md`, `naming.md`, `testing.md`), addressed by one `${guidelines-path}` placeholder. Not `CLAUDE.md`: it is loaded into every agent and grows unbounded, which is exactly the degradation the article warns about. Per-area files keep each one small and prunable, and the reviewer reads only the folder.
- **Offered through the `refactor` menu only** (opportunistic-inside-change and ad-hoc operator entry). The `task/system-refactor` subtype bypasses the menu (`GATE_TASK_SUBTYPE` → `REFACTOR_SYSTEM_STRUCTURE` directly), and a ticket subtype for review can be added later if teams ask; the review loop needs the human present anyway, which fits the interactive menu.
- **`HUMAN_REVIEW` captures issues as free text, one issue per line** (same convention as the Red reviewer's findings — the output CLI comma-splits lists). An empty answer means "no issues" and ends the loop.
- **Step 3: `EXECUTE_PLAN` reuses `system-refactorer` / `test-refactorer`**, passing the approved plan through a `${refactoring-plan}` placeholder.

Made during execution (2026-11-09; override any):
- **Step 3 is implemented through the existing `${checklist}`, not a new `${refactoring-plan}` placeholder.** EXECUTE_SYSTEM_PLAN / EXECUTE_TEST_PLAN pass `checklist: ${system-refactoring-plan}` / `${test-refactoring-plan}` as a call-activity param, and the refactorer prompts say the Checklist is either the ticket's or the approved plan. This needed no renderer change: a ticket Checklist is never present on the menu path, and `clauderun.go` was being edited by another session.
- **`guidelines-path` is optional with a default.** It is accepted under `system-test.paths:`, not required by Rule 22a, and not checked on disk by preflight. When unset it resolves to `docs/guidelines`. A required key would break shop's 12 configs on the next release (see `path-keys.md`, "Release ordering").
- **The reviewer emits a required `refactoring-needed` bool**, with optional `system-refactoring-plan`, `test-refactoring-plan` and `findings` strings, one item per line. A reviewer that emits nothing then fails validation instead of reading as "nothing to refactor", and a verdict that contradicts the plans fails at SUMMARIZE_REFACTORING_PLAN.
- **The guidelines loop goes back to HUMAN_REVIEW, not to the reviewer**, and is capped at 3 rounds. Each round runs the full suite and makes its own `GUIDELINES` commit. The human enters issues one prompt per line, and an empty line ends the list.
- **The adversarial-review chunk is split.** The generic reviewer stance is shared by every `*-reviewer`. The Red reviewers' `${changed-files}` and blocking/non-blocking verdict moved to `red-review-verdict.md`.
- **Tiers:** the reviewer is opus/medium with `command` category, and the human approves the plan at APPROVE_PLAN (`human` category). The updater is opus/medium with `system-agent` category.

## Verification

- Operator: on shop, run the `refactor` menu → `review-code-and-architecture`. Approve the plan, list one issue at HUMAN_REVIEW, and confirm that `docs/guidelines/<area>.md` gains a rule, that other occurrences are fixed, and that a `GUIDELINES` commit lands.
