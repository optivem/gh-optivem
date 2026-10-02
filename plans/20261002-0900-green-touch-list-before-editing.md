# 2026-10-02 09:00:02 UTC — GREEN-phase "touch list" before editing

## TL;DR

**Why:** GREEN implementer agents go straight to editing with no declared scope, so scope misses are only caught after the fact by `scope-diff-fixer`. A full plan-then-execute pass would cost too many tokens, and RED is already pinned down by the acceptance criteria.
**End result:** Each GREEN implementer emits a short "touch list" (files it will change, one line of reason each, plus the test command) in the same turn before editing. No approval gate and no separate plan document. `scope-diff-fixer` can compare the actual diff against this declared scope.

## Outcomes

What we get out of this:

- GREEN implementers declare their intended scope up front in a few lines per run, so the token overhead stays small.
- `scope-diff-fixer` has a declared scope to compare against, so scope misses surface earlier and with a clearer cause.
- RED agents are unchanged.
- The touch list lives in one canonical place, not copy-pasted across six prompts.

## ▶ Next executable step (resume here)

Only design remains, so run `/refine-plan` on this file first. Resolve the three open questions below, each with a recommendation: (1) preamble vs per-agent, (2) declared BPMN output vs prose-only, (3) how `scope-diff-fixer` consumes the list. Then replace Steps 2–5 with concrete edits. Step 1 (read the current prompts) can be done at any time.

## Steps

- [ ] Step 1: Read the six GREEN implementer prompts under `internal/atdd/assets/runtime/agents/atdd/` (`system-implementer`, `dsl-implementer`, `system-driver-adapter-implementer`, `external-system-driver-adapter-implementer`, `external-system-stub-implementer`, `external-system-real-simulator-implementer`), plus `shared/preamble.md` and `scope-diff-fixer.md`, to see the current Outputs sections and scope handling.
- [ ] Step 2: Settle the open questions below (via `/refine-plan`).
- [ ] Step 3: Add the touch-list instruction at the chosen location (shared chunk or per-agent). Specify the format: `path — reason` per line, then `test: <command>`.
- [ ] Step 4: If it becomes a declared output, add it to the relevant nodes in `internal/atdd/process/process-flow.yaml` and to any Go binding or validator that checks declared outputs.
- [ ] Step 5: Update `scope-diff-fixer` to read the touch list and use it as the declared scope when diagnosing a scope diff.
- [ ] Step 6: Add or update tests covering prompt assembly and, if applicable, output validation. Run the existing suite.

## Open questions

- (1) **Shared preamble vs per-agent prompt.** Recommendation: a dedicated shared chunk included only by the six GREEN implementers. It avoids duplication, and putting it in `preamble.md` would wrongly apply it to RED agents and the fixers.
- (2) **Declared BPMN output vs prose-only.** Recommendation: prose-only first. A declared output adds YAML, Go and validator surface for an unproven benefit. Promote it only if `scope-diff-fixer` needs a machine-readable list.
- (3) **How `scope-diff-fixer` consumes it.** Recommendation: it reads the touch list from the implementer's transcript or output as context. It does not hard-fail on mismatch, because it already handles scope diffs and this only improves its diagnosis. This depends on (2).
- Inferred, not stated: whether the stub and simulator implementers need the touch list or are small enough to skip. Recommendation: include them for consistency, since the cost is a few lines.
