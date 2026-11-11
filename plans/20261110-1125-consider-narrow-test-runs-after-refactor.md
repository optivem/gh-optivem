# 2026-11-10 11:25 — Consider narrowing test runs after a refactoring

## TL;DR

**Why:** After a refactoring, the ATDD flow runs the whole acceptance suite. Running only the touched use cases' tests would be faster, but it needs a way to pick them, and the choice has not been made.
**End result:** a decision on whether, and how, to narrow the post-refactor test run, and (if yes) an implementation plan for it. Until then the whole suite keeps running.

## ▶ Next executable step (resume here)

All decisions taken (answer: narrow). Next: `/create-plan` an implementation plan from the sketch below, cross-referencing this plan; then delete this plan.

## Context

Decided 2026-11-10 (meta-plan `20261111-2359`, Q3): for now, keep running the whole suite after a refactoring. Narrowing is deferred to this plan. Source discussion: the former `20261109-1250-refactor-touched-use-case-tests` plan (deleted; see git history).

## Idea

After a refactoring, run only the touched use case's acceptance tests locally, and let the pipeline run the full suite after push. This is what the article recommends. Cost: a break in another use case is caught only after push.

## Open questions (all resolved 2026-11-11)

1. **Narrow at all?** ✅ **Resolved 2026-11-11 (user): yes.** Narrow locally; the pipeline runs the full suite after push; fall back to the whole suite when no use case ID is known.
2. **How are the touched use cases picked?**
   - (a) The review plan names its UC IDs (e.g. `UC-001`, `UC-003`); the runner turns them into a test filter. Works for review-driven refactors, not ad-hoc ones.
   - (b) Map changed files to use cases (static analysis or path convention). Needs no human input, but shared code (domain, utilities, config) maps to many use cases; fragile, most machinery, can silently under-test.
   - (c) Tags from the story's criterion IDs (e.g. `UC-001-AC3`). Works for refactors inside a story; nothing to key on outside one.
   - ✅ **Resolved 2026-11-11 (user): (a) for review-driven refactors plus (c) for refactors inside a story, with a whole-suite fallback when no ID is known.** (b) rejected: it can quietly miss a regression.
3. **Apply narrowing to system and test refactors alike?** ✅ **Resolved 2026-11-11 (user): both** — a test refactor can break other use cases' tests just as a system one can, and the same tag selection works for both.

## Preconditions to check

- Do the acceptance tests actually carry UC / criterion tags today? **Checked 2026-11-11: yes**, shop's acceptance tests carry criterion tags such as `@Tag("UC-001.3")` (Java, UC-001.1 to .16 seen). The .NET/TypeScript equivalents still need checking when the implementation plan is written.

## If the answer is "narrow" — implementation sketch

- BPMN `VERIFY_TESTS_PASS` `suite` params in `refactor-system-structure` / `refactor-test-structure`.
- Tag selection from UC IDs (review plan) or criterion IDs (story).
- Whole-suite fallback when no ID is known.
- Edits `internal/atdd/process/process-flow.yaml` — never run in parallel with another plan editing it.
