# 2026-11-09 11:58 UTC — Acceptance criteria: delete on green, generate on demand

## TL;DR

**Why:** Each acceptance criterion and its acceptance test say the same thing, one to one. Kept by hand, they drift, and only the test is guaranteed current because it must pass. Today `change-system-behavior` keeps both and runs `RECORD_SPEC_FINGERPRINTS` / `COMMIT_SPEC_FINGERPRINTS` so CI's `spec check` reports later edits. The articles ("will drift, keep one", the coverage article's Step 2.5) say to delete the criteria once their tests pass, and to generate readable Gherkin from the tests when a PO or customer needs to read them.
**End result:** When a story's tests are green, its criteria are removed from the use case and the deletion is committed. A command regenerates readable criteria from the tagged tests. Spec fingerprints are retired.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md`, Q4 ("Change System Behavior" diagram).

## Outcomes

- `APPLY_AC_POLICY` + `COMMIT_AC_POLICY` replace `RECORD_SPEC_FINGERPRINTS` + `COMMIT_SPEC_FINGERPRINTS` in `change-system-behavior`.
- The use case keeps its narrative, and the business rules keep their own file. Each test still carries its `UC-NNN.M` ID.
- A `gh optivem spec generate`-style command renders Gherkin per use case from the tagged tests, for review or sign-off.
- The spec-fingerprint machinery is removed: the `spec check` CI step, the state file and `record-spec-fingerprints`. This follows the teaching-repo rule: no legacy shims.

## ▶ Next executable step (resume here)

Review this plan with the user. Settle the ordering with `plans/20261109-1256-use-case-format-and-story-link.md` first, because both touch the criteria format.

## Steps

- [ ] Step 1: Inventory everything that depends on spec fingerprints (`record-spec-fingerprints`, `spec check`, state file, CI wiring in shop/scaffold, docs under `docs/spec-gate/`). List it in this plan before deleting anything.
- [ ] Step 2: Replace the fingerprint nodes with `APPLY_AC_POLICY` (deterministic: delete the story's `merged-criteria` from the use case) and `COMMIT_AC_POLICY` (test-commit).
- [ ] Step 3: Design and build the generate command: input is the tagged tests per language, output is Gherkin per use case. Decide whether it parses the DSL chain or reuses the testselect parsing layer.
- [ ] Step 4: Remove the fingerprint machinery found in Step 1, including scaffolded CI steps.
- [ ] Step 5: Update the merger and refiner prompts that assume criteria text exists for already-implemented behaviour.
- [ ] Step 6: Tests.

## Open questions

- Generated output: printed to stdout, written to a file, or a doc artifact? Recommended: stdout or a file, never committed (committing it would re-create the second copy).
- How does the generator turn a DSL chain into Gherkin across Java/.NET/TypeScript? This is the hardest part; it may justify a separate plan.
- Before generation exists, is there a gap where nobody can read the criteria of implemented behaviour? Consider shipping the generator before deleting criteria.
