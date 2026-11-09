# 2026-11-09 19:09 UTC — Meta-plan: run all active top-level plans

## TL;DR

**Why:** 8 top-level plans are open (every one except `20261104-1120-meta-backlog-consolidation.md`, kept separate on purpose). Most wait on a user decision, so nothing moves unless the user is asked plan by plan.
**End result:** one `/execute-plan` run asks every open question below in a single round, then executes all the agent work in order without stopping. The child plans are closed or turned into implementation plans, and only the user-only items under **Verification** are left.

## ▶ Next executable step (resume here)

Run `/execute-plan plans/20261109-2009-meta-active-plans.md`. Its pre-flight asks Q1–Q9 below in one round; after that the run needs no user input.

## Plans covered

| Plan | State | Where it lands here |
|---|---|---|
| `20261104-1121-switch-shop-to-own-domain` | ⏳ deferred (walking-skeleton design not decided) | Verification only, not executed |
| `20261106-1305-scaffold-cloud-setup-guide` | only the live GCP run (user only) is left | Verification only, not executed |
| `20261107-1300-spec-parse-gherkin-criteria` | only the release is left | Step 2 (Q2) |
| `20261109-1219-improved-atdd-process-flow` | umbrella; children below | Step 7 (delete) |
| `20261109-1250-refactor-touched-use-case-tests` | discussion | Step 3 (Q3–Q5), Step 6 |
| `20261109-1258-ac-delete-and-generate` | one comment left | Step 1 |
| `20261109-1301-legacy-baseline-mode` | discussion | Step 4 (Q6–Q9), Step 6 |
| `20261109-1302-ux-ui-review` | decisions proposed, pending OK | Step 5 (Q1) |

## Open questions (all asked up front by `/execute-plan`)

Each has a recommendation; answer "all recommended" to accept every one.

1. **`1302` UX/UI review: accept its four proposed decisions?** (screenshots via a deterministic `capture-ui-screens` task plus the UI source; no CI; a human `APPROVE_VISUAL_CHECK` gate after restart; reuse the Code & Architecture Review loop.) **Recommended: accept as written**, because they copy the review that already shipped, so the least new machinery.
2. **`1300` spec parser: release now?** (push to `main` → acceptance pipeline cuts the release; then `gh optivem spec validate` in a read-only shop checkout must report 14 UC-001 criteria.) **Recommended: yes**, because shop's `spec-gate` stays off until this ships.
3. **`1250`: after a refactoring, run only the touched use case's acceptance tests?** **Recommended: yes, narrow locally and let the pipeline run the full suite**, which is what the article recommends; the cost (a break elsewhere caught after push) is covered by the pipeline.
4. **`1250`: how are the touched use cases picked?** Options: (a) the review plan names its UC IDs; (b) changed-file → use case mapping; (c) tags from the story's criterion IDs. **Recommended: (a) for review-driven refactors plus (c) for refactors inside a story, falling back to the whole suite when no ID is known**, because both inputs already exist and the fallback never under-tests.
5. **`1250`: apply narrowing to system and test refactors alike?** **Recommended: both**, because a test refactor can break other use cases' tests just as a system one can, and the same tag selection works for both.
6. **`1301` legacy baseline: shape?** (a) new kickoff mode; (b) extend `cover-system-behavior` with optional "generate narrative + criteria" steps per use case ticket; (c) prompts as docs only. **Recommended: (b)**, because it reuses the existing per-ticket legacy flow and keeps `internal/kickoff/` free while `1121` is deferred.
7. **`1301`: relation to kickoff?** **Recommended: kickoff stays greenfield-only; the baseline is a separate entry point**, so the two never share state.
8. **`1301`: how is the first test's "hand-holding" represented?** **Recommended: a human approve gate after each layer, for the first use case only**; later use cases run unattended.
9. **`1301`: does "fix the test, never the system" need a scope guard?** **Recommended: yes, the executor adds a scope guard making system code read-only in the baseline flow if the current `green-when-complete` fix loop can write it** (it checks first and skips the guard if system code is already out of scope).

Also confirmed by answering: commits and pushes to gh-optivem `main` per step via `/commit` (surgical `--paths`) are OK during this run.

## Steps

- [ ] **Step 1 — `1258` Step 7.** In `internal/scaffolding/steps/apply_template.go` (~line 158) replace "ATDD's record-spec-fingerprints both read ./spec.yaml" with "`gh optivem spec generate` both read ./spec.yaml". Direct edit, no subagent. Then delete `plans/20261109-1258-ac-delete-and-generate.md` (its last item). One commit.
- [ ] **Step 2 — `1300` release (if Q2 = yes).** Follow that plan's Step 5: release, then in a scratch shop clone run `gh optivem spec validate` and confirm 14 UC-001 criteria. Write the hand-off note for shop Track D into the plan, then delete it (the shop-side copy is deleted from a shop session, not here).
- [ ] **Step 3 — `1250` decisions.** Write the Q3–Q5 answers into the plan. If "narrow": write a fresh implementation plan `plans/<local-time>-refactor-scoped-verification.md` (BPMN `VERIFY_TESTS_PASS` `suite` params in `refactor-system-structure` / `refactor-test-structure`, tag selection, whole-suite fallback) and delete `1250`. If "keep whole suite": record that and delete `1250`.
- [ ] **Step 4 — `1301` decisions.** Write the Q6–Q9 answers into the plan, draw the chosen flow in `1219`'s colour legend, write a fresh implementation plan `plans/<local-time>-legacy-baseline-impl.md`, and delete `1301`. (If Q6 = (c), write the docs-only plan instead.)
- [ ] **Step 5 — `1302` UX/UI review.** Record Q1 in the plan, then execute its Steps 1–3 (`capture-ui-screens` service task, `ux-ui-reviewer` agent + `review-ux-ui` process + `GATE_REFACTOR_TYPE_CHOICE` option, tests). Sole owner of `internal/atdd/process/process-flow.yaml` while it runs. Delete `1302` when done.
- [ ] **Step 6 — implementation plans from Steps 3 and 4.** Execute them one at a time (both edit `process-flow.yaml`; never in parallel), each to completion and deletion. Questions they raise that Q3–Q9 don't answer: stop and ask (the only allowed mid-run stop).
- [ ] **Step 7 — close the umbrella.** Delete `plans/20261109-1219-improved-atdd-process-flow.md` once `1250`, `1258`, `1301`, `1302` are gone; move any operator rehearsals it still lists into **Verification** below first.
- [ ] **Step 8 — delete this meta-plan** once Steps 1–7 are done and Verification is handed to the user.

## Run rules

- **Mode: autonomous** (`/execute-plan` default). After the one up-front question round the user is away: no per-step approval gates, one hand-back at the end with a summary and the Verification list.
- **Main session = orchestrator only.** It reads this plan, dispatches, checks each worker's result, ticks the step and commits. It does not read child plans or code itself beyond what a check needs.
- **Token-efficient dispatch:**
  - Step 1 is a one-line edit: do it directly in the main session, no subagent.
  - Steps 2, 3, 4 each go to one `general-purpose` subagent, **dispatched in parallel in a single message** (disjoint files: release / `1250` / `1301` plan files). Brief each with its step text, the Q-answers it needs, and the child plan path; let it read the plan itself.
  - Step 5 and each Step 6 plan get one subagent each, **sequential** (shared `process-flow.yaml`). Inside them, the subagent runs that plan's own steps; it may fan out further only for independent files.
  - Workers return a short result (files changed, tests run, pass/fail, anything blocked), not file dumps.
  - Never `isolation: "worktree"`.
- **Commits:** one surgical commit per step (`gh optivem commit --repo gh-optivem --paths "<files>" "<msg>"`), made by the orchestrator after the step's check passes; parallel workers never commit.
- A step that fails its check twice: record it under a `## Blocked` heading here and continue with the next step that doesn't depend on it.
- Pre-execute: `grep -l "PICKUP\|claimed by\|Picked up by agent" plans/*.md plans/*/*.md plans/*/*/*.md` (`in-flight` gives false positives in `1305` and the meta-backlog); `git status` clean on `apply_template.go` and `process-flow.yaml`.
- Go tests on Windows: `scripts/test.sh` or `-p 2`, never unbounded.
- Diagrams under `docs/process-diagram*` are regenerated by CI; don't regenerate locally.

## Verification (user, not executed by the run)

- `1305` Step 5: live GCP run with a short repo name, following `docs/cloud-setup.md`; settle its unverified bits (GHCR `/orgs/` vs `/users/` link, public-package default, multirepo script location).
- `1121`: waits on the walking-skeleton decision (meta-backlog Phase B); not part of this run.
- `1258` operator check: read `gh optivem spec generate` output on shop and judge if it reads well to a PO.
- `1219` operator rehearsals (Align loop, Use Case Criteria body section, AT reviewer, Code & Architecture Review, WIP-gate removal), once shop/Docker is free.
- `1302`: one attended `refactor` → UX/UI Review run on shop.
