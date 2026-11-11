# 2026-11-11 09:23:45 UTC — Narrow post-refactor test runs to the touched use cases

Source: decisions in `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md` (all questions resolved 2026-11-11). Delete that plan once this one is committed.

## TL;DR

**Why:** After a refactoring (system or test), the ATDD flow runs the whole acceptance suite locally. Most refactors touch one or two use cases, so the full local run is slow for little gain; the pipeline runs the full suite after push anyway.
**End result:** The refactor cycles run only the acceptance tests tagged with the touched use case / criterion IDs (from the review plan, or from the story in flight), and fall back to the whole suite when no ID is known. The pipeline still runs everything after push.

## Outcomes

- A system refactor or test refactor inside a story runs only that story's tagged acceptance tests locally.
- A review-driven refactor (code-review loop) runs only the tests for the UC IDs the review plan names.
- When no ID is known (ad-hoc refactor, review plan without UC IDs), the whole suite runs. Narrowing never silently under-tests: an ID selection that matches zero tests fails loud rather than passing.
- Works for Java, .NET and TypeScript stacks (tags exist in all three, or are added).
- The pipeline after push still runs the full suite (unchanged).

## ▶ Next executable step (resume here)

Step 1: check whether shop's .NET and TypeScript acceptance tests carry the same UC / criterion tags as Java (`@Tag("UC-001.3")`), and how each stack's runner filters on them (gradle `-PincludeTags` / JUnit tag expressions, `dotnet test --filter Category=…` / `Trait`, TypeScript runner grep or tag). Record the result in this plan. Read-only; no code changes. It decides the shape of Step 2.

## Steps

- [ ] Step 1: Precondition check — tags + per-stack tag-filter syntax in shop's Java / .NET / TypeScript acceptance tests (see resume block).
- [ ] Step 2: Runner support for tag selection — add a tag/ID selection to `gh optivem system-test run` (new flag, e.g. `--tag`, next to `--test` in `internal/build/runner/tests.go`), mapped per stack via the suite config (like `appendTestFilter` does for names). Keep the empty-selection guard: a tag filter that executes 0 tests is an error.
- [ ] Step 3: Source the IDs.
  - (c) Story path: the story's use case / criterion IDs are already in ctx.State from ticket parsing — expose them as a param.
  - (a) Review path: the review plan (`SUMMARIZE_REFACTORING_PLAN`) names UC IDs — add a structured output (e.g. `refactor-use-case-ids`) to the reviewer/summarizer contract.
- [ ] Step 4: BPMN wiring in `internal/atdd/process/process-flow.yaml` — bind the new tag param in `refactor-system-structure` (`IMPLEMENT_AND_VERIFY_SYSTEM`, currently `suite: ""` / `test-names: ""`) and in `refactor-test-structure` (`refactor-and-verify-tests`), plus the guideline-round `VERIFY_TESTS_PASS` in the code-review process. Empty IDs → whole suite. Update the "full regression" comments. **Never run in parallel with another plan editing `process-flow.yaml`.**
- [ ] Step 5: Tests — runner tag-filter per stack; BPMN binding tests (IDs present → tag filter; IDs absent → whole suite); empty-tag-selection fails loud.
- [ ] Step 6: Delete `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`, update meta-plan `20261110-2159`.

## Verification

- Operator: rehearse one story with a system refactor and one review-driven refactor; confirm the local run executes only the tagged tests and the pipeline runs the full suite.

## Decisions (2026-11-11, user)

- **Redesign path stays on the full suite.** `redesign-system-structure` is out of scope: a redesign is broad by nature, so the whole suite is the right check. Leave its `suite: ""` / `test-names: ""` and comment unchanged.
- **Narrow by use case, not by criterion.** A story or review ID `UC-001` (or criterion `UC-001.3`) selects all `UC-001.*` tests — a refactor inside a use case can break any of its criteria, and running a whole UC is cheap. Criterion IDs are reduced to their UC prefix before filtering.
- **Review plan without UC IDs → whole suite.** A cross-cutting refactor where the reviewer names no UC IDs runs the full suite.
