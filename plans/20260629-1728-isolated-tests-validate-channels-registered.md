<!-- PENDING TO BE DISCUSSED -->

# Plan: `validateChannelsRegistered` falsely flags isolated tests as orphan channels

## TL;DR

**Why:** `validateChannelsRegistered` in `channel.go` only checks `acceptance-<ch>` suite reports, but `@isolated` tests never appear in those reports — they run in `acceptance-isolated-<ch>` suites. The RED verify step also only runs `--suite=acceptance`, so no `acceptance-isolated-*` reports exist on disk when the validator checks, causing a hard halt on any ticket that produces isolated tests.
**End result:** After the three fixes land, the validator will also look in `acceptance-isolated-*` reports (Fix A), the RED verify will produce those reports by running `--suite=acceptance-isolated` as well (Fix B), and the acceptance-test-writer prompt will document that `@isolated` tests must not use `forChannels()` (Fix C) — eliminating this false-halt class for all future isolated-test-producing tickets.

Ticket: #76 — Bug: Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00
Machine: ValentinaLaptop

## Summary

Run `20260629-124124` halted at `VALIDATE_CHANNELS_REGISTERED` after the
acceptance-test-writer produced two isolated tests (`@isolated` describe block)
that the validator treated as registering for an unconfigured channel.

The underlying defect: `validateChannelsRegistered` only checks the
`acceptance` (non-isolated) suite reports, but isolated tests are excluded from
those reports by `--grep-invert '@isolated'`. They never appear there and are
always flagged as orphans — even though they are valid tests that will run in
`acceptance-isolated-*` suites.

## Root cause (pinned)

**File:** `internal/atdd/process/actions/channel.go`

| Line | Code | Problem |
|------|------|---------|
| 118 | `suiteIDs := a.acceptanceSuiteIDs("acceptance", channels)` | Only expands to non-isolated `acceptance-<ch>` suites |
| 119 | `names, err := runner.NamesInReport(...)` | So the resulting `names` set never contains isolated test names |
| 123–128 | `for _, t := range ticketTests { if !names[t] { orphans = ... } }` | Any `@isolated` test is flagged as an orphan |
| 130 | Hard-error emitted | Run halts before channel unroll even starts |

**Affected tests:**
- `cannotCancelAnOrderOn31stDecBetween2200And2300_`
  (`cancel-order-negative-isolated-test.spec.ts:19`)
- `shouldBeAbleToCancelOrderOutsideOfBlackoutPeriod31stDecBetween2200And2300_`
  (`cancel-order-positive-isolated-test.spec.ts:13`)

Both are inside `test.describe('@isolated', ...)` blocks and correctly do NOT
use `forChannels()` — isolated tests are channel-agnostic by design.

**What `shouldRejectCancellationAt2245OnDec31` did right:** it lives in
`cancel-order-negative-test.spec.ts:11` inside `forChannels(ChannelType.UI, ChannelType.API)(...)`,
ran in the `acceptance-api` suite, and appeared in the report → not flagged.

## Defect classification

Mixed **orchestration mis-step** + **service task bug**:

- The acceptance-test-writer agent produced correct isolated tests (no
  `forChannels()` is right for `@isolated` blocks).
- The `validateChannelsRegistered` service task has the wrong assumption: it
  treats "not in any `acceptance-<ch>` report" as "orphan", but isolated tests
  live in `acceptance-isolated-<ch>` reports, not `acceptance-<ch>` reports.
- Additionally, the RED verify step (`WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE`)
  only runs `--suite=acceptance`, so no `acceptance-isolated-*` reports exist
  on disk when the validator runs.

## Candidate fixes

### Fix A — service task: include isolated suite reports in the lookup (recommended)

**File:** `internal/atdd/process/actions/channel.go`

In `validateChannelsRegistered`, after collecting names from `acceptance` suite
reports, also collect names from `acceptance-isolated` suite reports. A test
found in either set is not an orphan.

```go
// current
suiteIDs := a.acceptanceSuiteIDs("acceptance", channels)
names, err := runner.NamesInReport(...)

// proposed
suiteIDs := a.acceptanceSuiteIDs("acceptance", channels)
isolatedIDs := a.acceptanceSuiteIDs("acceptance-isolated", channels)
names, err := runner.NamesInReport(a.deps.TestsConfig, a.deps.TestsCwd,
    append(suiteIDs, isolatedIDs...))
```

This requires Fix B (isolated suite must also run) so reports actually exist.

**Tradeoff:** Narrows only to the lookup fix; doesn't change orchestration beyond what Fix B adds. Lowest blast radius.

### Fix B — BPMN / process flow: also run `--suite=acceptance-isolated` in the RED verify

**File:** `internal/atdd/process/process-flow.yaml` (the `write-and-verify-acceptance-test-code` or equivalent sub-process) and the `gh optivem system-test run` command invocation.

The RED verify step currently runs:
```
gh optivem system-test run --suite=acceptance --test=<names>
```

It should also run:
```
gh optivem system-test run --suite=acceptance-isolated --test=<names>
```

This ensures `acceptance-isolated-*` reports exist on disk before `VALIDATE_CHANNELS_REGISTERED` reads them, making Fix A effective.

**Tradeoff:** Adds a second run command to the RED verify; isolated tests run
in serial mode so it may add wall-clock time. However the tests must eventually
run anyway, so this is the correct place for them.

### Fix C — agent: teach `acceptance-test-writer` that isolated tests are valid

**File:** `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md`

Add a rule clarifying:
- Isolated tests (`@isolated` describe, serial mode) do NOT use `forChannels()`.
- The validator will accept them provided Fixes A + B are in place.

This is documentation/instruction hardening, not a standalone fix. Prevents
the agent from incorrectly adding `forChannels()` to isolated tests in a
future attempt to avoid the error.

**Tradeoff:** Narrow agent fix; harmless on its own but needed to prevent a
confused agent from "fixing" isolated tests by wrapping them in `forChannels()`.

## Recommended scope

All three fixes together form a coherent prevention:

- Fix A (channel.go) + Fix B (process-flow.yaml / command invocation) eliminate
  the false halt for this entire class of isolated-test-producing tickets.
- Fix C (acceptance-test-writer.md) prevents the agent from working around the
  validator incorrectly in a future run.

If only one fix can land: **Fix A + Fix B** (neither is meaningful without the
other; Fix C is documentation only).

## Prevention goal

After these changes, a future ticket where the acceptance-test-writer produces
`@isolated` tests will:
1. Run both `acceptance` and `acceptance-isolated` suites in the RED verify.
2. Have `validateChannelsRegistered` find the isolated test names in the
   `acceptance-isolated-*` reports and not flag them.
3. Proceed to the channel unroll normally.

## Steps

- [ ] **A.** `internal/atdd/process/actions/channel.go` — in
  `validateChannelsRegistered`, extend the suite ID list to include
  `acceptance-isolated` suite IDs and re-read `NamesInReport` over the
  combined list; update `channel_test.go` with a case for an isolated test.
- [ ] **B.** `internal/atdd/process/process-flow.yaml` (or the underlying
  command construction) — the RED acceptance verify step must invoke
  `gh optivem system-test run` twice: once with `--suite=acceptance` and once
  with `--suite=acceptance-isolated` (both filtered to `--test=<names>`).
- [ ] **C.** `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md`
  — add a rule: isolated tests (`@isolated`, serial, no channel fixture
  dependency) must NOT use `forChannels()`; they are verified via the
  `acceptance-isolated` suite, not the per-channel `acceptance` suite.
