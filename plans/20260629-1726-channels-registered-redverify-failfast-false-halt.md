> ⚠️ **PENDING — to be discussed.** This is a captured `/atdd-postmortem` diagnosis,
> not an agreed plan. The fix-layer decision (runner / config / BPMN) is still open —
> see **Open decision** below. Do not execute until the layer(s) are chosen.

# #76 — `VALIDATE_CHANNELS_REGISTERED` false halt (RED verify fail-fast skips the isolated partition)

Ticket: #76 — Bug: Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00
Machine: Valentina_Desk
Run: `.gh-optivem/runs/20260629-122532/` (rehearsal `20260629-142520-76-bug-order-cancellation-blackout-on-dec`)

## The halt

`validate-channels-registered` (`internal/atdd/process/actions/channel.go:104-133`) hard-errored:

> acceptance test(s) `cannotCancelAnOrderOn31stDecBetween2200And2300`,
> `shouldBeAbleToCancelOrderOutsideOfBlackoutPeriod31stDecBetween2200And2300` ran in
> none of the configured channels (api, ui)

Failing path: `IMPLEMENT_TICKET → CHANGE_SYSTEM_BEHAVIOR → WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS → WRITE_ACCEPTANCE_TESTS_AND_DSL → VALIDATE_CHANNELS_REGISTERED`.

## Root cause (pinned)

The two flagged tests are **correctly** registered for API (+UI first row) — they simply
**never executed**, so they're absent from the report the validator reads.

- **Primary — fail-fast abort:** `internal/build/runner/tests.go:131-133` — `RunTests`
  returns on the first suite's non-zero exit, before the presence check at line 151 and
  before the remaining partitions run. The RED filtered acceptance verify expanded
  `--suite=acceptance` to 4 partitions (declaration order: `acceptance-parallel-api`,
  `acceptance-parallel-ui`, `acceptance-isolated-api`, `acceptance-isolated-ui`). The log
  (`...142520...-76....log`) shows only `--- Running latest - Acceptance Parallel (stub) - API ---`
  then `BUILD FAILED in 14s (1 test completed, 1 failed)`. The first partition ran the one
  **non-isolated** ticket test (`cannotCancelAnOrderOn31stDecAt2245`), which **failed exactly
  as a RED verify intends**, gradle exited 1, and the runner aborted — never running the two
  `@Isolated` partitions where the two flagged tests live.

- **Compounding — report clobber:** all 4 acceptance partitions share one `testCountPath`
  in every language (`system-test/java/tests.yaml:70,77,84,91` → `build/test-results/test`;
  dotnet → `TestResults/testResults.trx`; typescript → `playwright-report/results.json`).
  Even if the runner didn't abort, sequential gradle runs overwrite that one dir, so the
  downstream `NamesInReport` (`channel.go:119`) would still see only the *last* partition.

The membership design's stated assumption — "the RED verify writes one report per partition"
(`channel.go:30-37`) — is false on **both** counts.

## Why only #76 (not #69/#71/#72)

#76 is the only rehearsal whose new acceptance tests span **both** a non-isolated class and
`@Isolated` classes (`CancelOrderNegativeIsolatedTest`, `CancelOrderPositiveIsolatedTest` —
clock-mutating, legitimately isolated). The non-isolated partition sorts first, fails RED,
and aborts the run before the isolated partition's report is written. In a RED verify *some*
partition is guaranteed to fail, so fail-fast + multi-partition tests = guaranteed incomplete
reports.

## Not an agent fault

The `acceptance-test-writer` correctly authored isolated clock-mutating tests with proper
channel registration. `@Isolated` and the channel scope are intentional and correct — no
agent change is warranted (do not propose weakening this).

## Open decision (to be discussed)

Which layer(s) should the prevention plan change? (multi-select)

1. **Runner — run all partitions** *(recommended, pairs with #2)* — `internal/build/runner/tests.go`:
   for a named acceptance verify, run ALL selected partitions to completion instead of
   returning on the first suite's non-zero exit, while preserving the RED (non-zero) exit so
   `verify-tests-fail` still sees the expected failure. Directly fixes the demonstrated cause.
   Note the exit-code tension: the current early `return` at :132 is what currently delivers the
   RED non-zero exit; the presence check at :151 returns nil when all names ran — a naive
   continue-on-error would convert RED into exit 0. The fix must accumulate the first error,
   run every partition, then return that error after the loop.

2. **Config — per-partition report paths** *(recommended, pairs with #1)* — shop scaffold
   `tests.yaml` (java/dotnet/typescript): give each acceptance partition its own `testCountPath`
   so partitions don't clobber and `NamesInReport` can union across all of them. Without this,
   fixing only the runner just **moves** the false-halt to whichever test gets overwritten. Lives
   in the **shop** repo, not gh-optivem.

3. **BPMN — robust membership source** *(alternative)* — `internal/atdd/process/actions/channel.go`:
   make `validate-channels-registered` (and `resolve-channel`) independent of the RED verify's
   leftover reports — run a dedicated continue-on-error enumeration of the ticket's tests across
   all partitions, or consume the runner's own presence-check verdict. Heavier (may re-run tests);
   `NamesExecutedIn` surfaces a failing test as an error, so a RED-tolerant path is needed.

Possible deeper simplification (out of scope unless raised): `validate-channels-registered` may
be partly redundant with the runner's own presence check (`tests.go:151-182`), which already
classifies "ran nowhere (fail loud)" vs "cross-channel skip."

## Next step

Decide the layer(s) above, then hand to `/create-plan` to draft the executable prevention plan.
