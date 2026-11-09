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

Step 1 is a read-only confirmation and can run right away via `/execute-plan`. It checks that the gap is real end to end before any code changes.

## Steps

- [ ] Step 1: Confirm the gap. Find a rehearsal branch or run where ATDD-written tests were committed, and check that (a) they still carry the gate on the final commit and (b) the shop/scaffold pipeline does not set `GH_OPTIVEM_RUN_WIP_TESTS`. Record the evidence here. If the gap is not real, stop and report.
- [ ] Step 2: Implement the per-language strip (Java annotation line + import if unused, .NET guard line, TypeScript `test.skip(...)` line), keyed on the exact snippets in `wip-gate-*.md` so they can't drift. Fail loudly if a listed test doesn't carry the gate.
- [ ] Step 3: Add the nodes to `process-flow.yaml`. Find out how "last touched channel" is known after `UnrollSystemChannels` (it may be better placed in `change-system-behavior` right after `IMPLEMENT_AND_VERIFY_SYSTEM`, which runs once after all channels).
- [ ] Step 4: Check `cover-system-behavior` (expected-pass tests): do covered tests get gated at all? If yes, they need the same removal.
- [ ] Step 5: Update the `clauderun.go` / `release.go` comments and the unit/statemachine tests. Run with `-p 2`.

## Verification

- Operator: rehearsal on shop. After Green, the story's tests have no gate line, and the next pipeline run executes them.

## Open questions

- Placement: inside `implement-and-verify-system` after the last channel, or once in `change-system-behavior` after `IMPLEMENT_AND_VERIFY_SYSTEM`? The second looks simpler; Step 3 confirms.
