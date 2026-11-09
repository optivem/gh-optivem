# 2026-11-09 11:50 UTC — Discuss: run only the touched use case's acceptance tests after a refactoring

## TL;DR

**Why:** After a refactoring, ATDD verification runs the whole acceptance suite (`VERIFY_TESTS_PASS` with `suite: ""` in `refactor-and-verify-tests` / `refactor-system-structure`). That is slow and produces long test output. The article "After a refactoring, run the acceptance tests you touched" (`substack/articles/drafts/next/2026-11-06-FREE-ATDD-run-the-acceptance-tests-you-touched.md`) recommends running only the touched use case's tests locally (e.g. `./gradlew test --tests '*PlaceOrderPositiveTest'` or by `@UC-NNN` tag) and letting the pipeline run the full suite.
**End result:** A decision on whether gh-optivem narrows refactor verification. If it does, the decision includes how the touched use cases are picked.

Parked from Q5 of `plans/20261109-1219-improved-atdd-process-flow.md`. Until this is decided, refactor verification keeps running the whole suite.

## Outcomes

- A recorded decision: narrow or keep the whole suite.
- If narrow: a chosen selection mechanism and the follow-up implementation plan.

## ▶ Next executable step (resume here)

Discussion only. Run `/refine-plan plans/20261109-1250-refactor-touched-use-case-tests.md` to settle the open questions. There is nothing to execute yet.

## Steps

- [ ] Step 1: Settle the open questions below with the user.
- [ ] Step 2: If the decision is "narrow", write the implementation plan (BPMN `VERIFY_TESTS_PASS` params in the refactor processes, test selection by tag).

## Open questions

- **Narrow at all?** Trade-off: faster, cheaper refactor loops vs. a break in another use case only being caught by the pipeline, after the push.
- **How to pick the touched use cases?** Options:
  - The Code & Architecture Review plan names the use case IDs it touches.
  - A changed-file → use case mapping.
  - Tag-based selection from the story's criterion IDs (only covers opportunistic refactors inside a story).
- **Which refactors?** Should narrowing apply to `refactor-system-structure` and `refactor-test-structure` alike, or only to system refactors?
