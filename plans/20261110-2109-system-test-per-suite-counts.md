# 2026-11-10 21:09 — `system-test run` prints passed/failed/skipped per suite

## TL;DR

**Why:** You can't read a `gh optivem system-test run` (including `--sample`) off at a glance. Each runner prints its own native output: a Gradle tree, dotnet console lines, a Playwright list, or Playwright JSON stats for some TypeScript suites. The closing `Suite Results:` table only shows `PASSED`/`FAILED` and a duration. Local gates and subagent reports have to dig the counts out of runner noise, and they get it wrong (shop plan `20261110-1207-system-test-reliability-followups.md`, idea #16).
**End result:** Each suite ends with one uniform line, `<suite>: passed N, failed M, skipped K`, in every language and runner. The closing table gets `Passed | Failed | Skipped` columns and a totals row. Counts come from the machine-readable reports the runner already reads (TRX, JUnit XML, Playwright JSON), and they are reported for **failed suites too**, not only clean ones.

## ▶ Next executable step (resume here)

Run `/refine-plan plans/20261110-2109-system-test-per-suite-counts.md` to resolve the open questions, then do Step 1.

## Current behaviour (verified 2026-11-10 against source)

| Piece | What it does today | Code |
|---|---|---|
| Per-suite run | Prints `--- Running <suite> ---`, runs the command, and prints `Test report: <path>` when `testReportPath` is set. There is no count line. | `internal/build/runner/tests.go` `runOneSuite` |
| Count parsing | `countExecutedTests(path)` returns only a single **executed** int: TRX `Counters@executed`, JUnit `tests - skipped` (a file or a gradle dir of files), Playwright `stats.expected+unexpected+flaky`. It does not separate passed, failed and skipped. | `internal/build/runner/testcount.go` |
| When counts are read | Only after the suite command **exits 0**, and only when `testCountPath` is set. A failed suite returns before the report is read. | `runOneSuite` (the early `return 0, false, nil, err` on `runShell` failure) |
| Closing table | `SuiteResult{Name, Status, Duration}` → `  <name> <PASSED/FAILED> <mm:ss.mmm>`. There are no counts and no totals. | `tests.go` `SuiteResult`, `printSummary` |
| Failure extraction | `FailuresInReports` already reads the same reports with a freshness cutoff (`reportFreshnessSlack`), so a leftover report from an earlier run is ignored. | `internal/build/runner/testfailures.go` |
| Component tests | A separate `printSummary` with its own result type. | `internal/build/componenttest/run.go:359` |

Which shop suites have a `testCountPath` (the source the counts would come from):

| Config | Suites | With `testCountPath` | Missing |
|---|---|---|---|
| `system-test/java/tests.yaml` | 11 | 7 | smoke-stub, smoke-real, e2e-* |
| `system-test/dotnet/tests.yaml` | 11 | 11 | — |
| `system-test/typescript/tests.yaml` | 11 | 7 | smoke-stub, smoke-real, e2e-* |
| `*/tests.legacy.yaml` (all three) | 26 each | 0 | all |

So the gh-optivem change alone gives full coverage for .NET latest only. Without a shop follow-up (Step 4), the Java/TS smoke and e2e suites and every legacy suite would print `counts unavailable`.

## Open questions

1. **Count source: reports only, or also parse console output?**
   - (a) Reports only (TRX / JUnit / Playwright JSON via `testCountPath`). A suite without one prints `<suite>: counts unavailable (no testCountPath)`.
   - (b) Also scrape each runner's console output as a fallback.
   - **Recommended: (a).** The reports are structured and already parsed. Console formats vary by reporter and version, and scraping them is exactly the brittleness this plan removes. Close the gap in the shop configs instead (Step 4).
2. **What counts as "skipped"?** TRX has `notExecuted` (plus `inconclusive`, `timeout`, `aborted`, `error`), JUnit has `skipped` + `failures` + `errors`, and Playwright has `skipped` + `unexpected` + `flaky`.
   - **Recommended:** failed = TRX `failed+error+timeout+aborted`, JUnit `failures+errors`, Playwright `unexpected`. Skipped = TRX `notExecuted`, JUnit `skipped`, Playwright `skipped`. Passed = the rest of executed. A Playwright `flaky` test counts as passed, and the line also shows `(flaky F)` when F > 0, because a flaky pass is worth seeing.
3. **Reading the report of a failed suite:** a crash can leave a stale report from an earlier run.
   - **Recommended:** reuse the freshness cutoff from `testfailures.go`, with the suite's start time as `since`. A stale or missing report on a failed suite prints `<suite>: counts unavailable (no fresh report)`, never old numbers.
4. **TS contract suites share one report path** (`playwright-report/results.json` for contract-stub, contract-stub-isolated and contract-real). It's fine sequentially, because each suite is read right after it runs. **Recommended:** keep it, and rely on Q3's freshness check. Add a note in the shop config that the path is per-run, not per-suite.
5. **Also apply to `component-test run`?** It has its own `printSummary`. **Recommended: yes, as a later Step 5**, reusing the same counts type, so all `*-test run` verbs read the same way. Keep it out of the first pass.

## Implementation (after the open questions are resolved)

- [ ] **Step 1 — counts type.** In `testcount.go`, add `type TestCounts struct{ Passed, Failed, Skipped, Flaky int }` and `readTestCounts(path) (TestCounts, error)` for TRX (extend `trxCounters` with the Q2 attributes), JUnit file/dir (add `failures`, `errors`), and Playwright (add `skipped`). Reimplement `countExecutedTests` as `Passed+Failed` (plus Flaky inside Passed) so the zero-count guard keeps its exact semantics. Extend `testcount_test.go` with fixtures that have failures and skips for each format.
- [ ] **Step 2 — per-suite line.** In `runOneSuite`, read the counts after the command **whether it passed or failed** (failed: freshness-gated per Q3). Print `<suite>: passed N, failed M, skipped K` right after the runner output, or the `counts unavailable (...)` variant. Return the counts alongside the existing values. Don't change the empty-selection or presence-check logic: failed suites still never feed `tally.totalExecuted`.
- [ ] **Step 3 — closing table.** Add `Counts *TestCounts` to `SuiteResult` (nil = unavailable). `printSummary` gets `Passed`, `Failed` and `Skipped` columns (`-` when nil) and a `Total` row that sums the known counts and says how many suites had no counts. Add unit tests in `tests_test.go` for the table rendering (capture stdout).
- [ ] **Step 4 — shop follow-up (separate shop commit).** Add `testCountPath` (and the reporter output each needs) to the Java/TS smoke and e2e suites and to all `tests.legacy.yaml` suites. Java: `build/test-results/<task>`. TS: a per-suite `--reporter=json` output file. .NET legacy: a per-suite `--logger 'junit;LogFilePath=TestResults/junit/<suite>.xml'`, as latest already does. Don't count from the shared `testResults.trx`, which several suites overwrite. Check each by running `gh optivem system-test run --sample` per language (ask before running; system tests need a booted stack).
- [ ] **Step 5 — component tests (optional, per Q5).** Use `TestCounts` in `internal/build/componenttest/run.go` `printSummary`.
- [ ] Update `README.md` / `docs/` wherever the `Suite Results:` output is shown or described (grep `Suite Results`).

## Context

- Raised in shop `plans/20261110-1207-system-test-reliability-followups.md`, idea #16 (TypeScript `--sample` runs: only some suites print JSON stats, the acceptance suites print a Playwright list).
- Don't run this in parallel with another plan that edits `internal/build/runner/tests.go` or `testcount.go`.
