# 2026-11-09 12:00 UTC — Green: remove the WIP gate from the story's tests once they pass

## TL;DR

**Why:** The AT writer adds a permanent WIP gate to every new or changed acceptance test (Java: `@EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", ...)`; see `internal/atdd/assets/runtime/shared/wip-gate-*.md`). Only the orchestrator's own verify runs set the variable (`internal/atdd/process/actions/command.go`). Per `clauderun.go` and `release.go`, nothing ever removes the gate, so regular CI, local and IDE runs skip these tests forever, even after the feature is built. ATDD-written tests therefore never give regression protection in the pipeline.
**End result:** After the story's tests pass on the last touched channel, gh-optivem deterministically removes the gate line from exactly those tests and commits that. From then on the pipeline runs them like any other test. Red stays as today: tests are gated while red, so trunk stays green.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md`, Q7 + Q8 ("Implement and Verify System" diagram). Q8: a changed test is edited in place and follows the same gate path.

## Outcomes

- New nodes in `implement-and-verify-system`: `GATE_LAST_CHANNEL → REMOVE_WIP_GATE → COMMIT_WIP_GATE_REMOVAL` (test-commit), after the last touched channel's `COMMIT_SYSTEM`.
- `REMOVE_WIP_GATE` is a deterministic service task, not the old enabler agent. Per language it strips the gate snippet from the tests in the AT writer's `test-names` list.
- The comments in `clauderun.go` and `release.go` no longer say "permanent".
- Bugs (also `change-system-behavior`) get the same removal.

## ▶ Next executable step (resume here)

The gap is confirmed (see `## Evidence`). Next is Step 2: implement the per-language gate strip, keyed on the `wip-gate-*.md` snippets. Review this plan with `/refine-plan` first if the placement question (Step 3) should be settled before coding.

## Steps

- [ ] Step 2: Implement the per-language strip (Java annotation line + import if unused, .NET guard line, TypeScript `test.skip(...)` line), keyed on the exact snippets in `wip-gate-*.md` so they can't drift. Fail loudly if a listed test doesn't carry the gate.
- [ ] Step 3: Add the nodes to `process-flow.yaml`. Find out how "last touched channel" is known after `UnrollSystemChannels` (it may be better placed in `change-system-behavior` right after `IMPLEMENT_AND_VERIFY_SYSTEM`, which runs once after all channels).
- [ ] Step 4: Check `cover-system-behavior` (expected-pass tests): do covered tests get gated at all? If yes, they need the same removal.
- [ ] Step 5: Update the `clauderun.go` / `release.go` comments and the unit/statemachine tests. Run with `-p 2`.

## Evidence (gap confirmed 2026-11-09)

- **Gate survives Green.** On shop rehearsal branches, the final commit (the last `SYSTEM` commit) still carries the WIP gate on the ATDD-written tests:
  - `rehearsal/71-gift-wrap-an-order`: tip `9056c76b` "SYSTEM (ui)"; `GiftWrapOrderPositiveTest.java` has the gate on both `shouldMarkOrderAsGiftWrapped` and `shouldAddGiftWrappingFeeToOrderTotal`.
  - `rehearsal/69-reject-order-with-line-quantity-of-100`, `rehearsal/72-charge-shipping-based-on-product-weight` and `rehearsal/76-bug-order-cancellation-blackout-on-dec`: same, with gated test files at the tip.
- **No pipeline lifts it.** `GH_OPTIVEM_RUN_WIP_TESTS` is set only in `internal/atdd/process/actions/command.go:196` (orchestrator verify runs, restored afterwards). It does not appear anywhere in shop `.github/`, in the `actions` repo, or in gh-optivem scaffolding. Shop's `system-test/java/build.gradle` only forwards the variable when it is already set.
- Consequence: once merged, these tests would be skipped by every acceptance-stage pipeline run.

## Verification

- Operator: rehearsal on shop. After Green, the story's tests have no gate line, and the next pipeline run executes them.

## Open questions

- Placement: inside `implement-and-verify-system` after the last channel, or once in `change-system-behavior` after `IMPLEMENT_AND_VERIFY_SYSTEM`? The second looks simpler; Step 3 confirms.
