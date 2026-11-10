# 2026-11-10 11:25 — Consider narrowing test runs after a refactoring

## TL;DR

**Why:** After a refactoring, the ATDD flow runs the whole acceptance suite. Running only the touched use cases' tests would be faster, but it needs a way to pick them, and the choice has not been made.
**End result:** a decision on whether, and how, to narrow the post-refactor test run, and (if yes) an implementation plan for it. Until then the whole suite keeps running.

## ▶ Next executable step (resume here)

Design/decision only: run `/refine-plan plans/20261110-1125-consider-narrow-test-runs-after-refactor.md` to resolve the open questions below, then `/create-plan` the implementation plan if the answer is "narrow".

## Context

Decided 2026-11-10 (meta-plan `20261110-2159`, Q3): for now, keep running the whole suite after a refactoring. Narrowing is deferred to this plan. Source discussion: the former `20261109-1250-refactor-touched-use-case-tests` plan (deleted; see git history).

## Idea

After a refactoring, run only the touched use case's acceptance tests locally, and let the pipeline run the full suite after push. This is what the article recommends. Cost: a break in another use case is caught only after push.

## Open questions

1. **Narrow at all?** Recommended: yes, narrow locally and let the pipeline run the full suite.
2. **How are the touched use cases picked?**
   - (a) The review plan names its UC IDs (e.g. `UC-001`, `UC-003`); the runner turns them into a test filter. Works for review-driven refactors, not ad-hoc ones.
   - (b) Map changed files to use cases (static analysis or path convention). Needs no human input, but shared code (domain, utilities, config) maps to many use cases; fragile, most machinery, can silently under-test.
   - (c) Tags from the story's criterion IDs (e.g. `UC-001-AC3`). Works for refactors inside a story; nothing to key on outside one.
   - **Recommended: (a) for review-driven refactors plus (c) for refactors inside a story, with a whole-suite fallback when no ID is known** — both inputs already exist, and the fallback never under-tests. (b) is the option that can quietly miss a regression.
3. **Apply narrowing to system and test refactors alike?** Recommended: both — a test refactor can break other use cases' tests just as a system one can, and the same tag selection works for both.

## Preconditions to check

- Do the acceptance tests actually carry UC / criterion tags today? If not, the first implementation step is adding them.

## If the answer is "narrow" — implementation sketch

- BPMN `VERIFY_TESTS_PASS` `suite` params in `refactor-system-structure` / `refactor-test-structure`.
- Tag selection from UC IDs (review plan) or criterion IDs (story).
- Whole-suite fallback when no ID is known.
- Edits `internal/atdd/process/process-flow.yaml` — never run in parallel with another plan editing it.
