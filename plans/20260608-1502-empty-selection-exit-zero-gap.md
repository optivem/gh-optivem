# Plan: close the empty-test-selection guard for runners that exit 0 ({20260608-1502})

Follow-up to `plans/20260608-1240-empty-test-selection-guard.md` (committed as
`02b09bc` + `b392149`). That plan closed the empty-selection hole for runners
that **exit non-zero** on a zero-match filter (Gradle, Maven `failIfNoTests`,
Playwright) by classifying their "no tests" stderr as `infra` →
`TESTS_INFRA_HALT`. This plan closes the **complement**: runners that exit `0`
on a zero-match filter, which the 1240 guard cannot see.

Related (historical): plan `20260606-1458` (infra-vs-red classifier), plan
`20260608-1240` (the non-zero half of this guard).

---

## TL;DR

**Why:** The 1240 empty-selection guard is exit-code-gated — `runCommand` only
classifies on a non-zero exit (`bindings.go:964`). dotnet (and any runner that
treats "no matching tests" as success) exits `0` with zero tests run, so
`test-outcome` is stamped `pass`. A *pass*-expecting verify greens on zero tests;
a *fail*-expecting verify routes to the passing-tests fixer on a non-signal. Same
silent hole as 1240, opposite exit code.
**End result:** An empty selection is detected regardless of the tool's exit
code — surfaced as the same `infra` / `TESTS_INFRA_HALT` outcome the 1240 guard
uses — so no verify can pass (or spin a fixer) without exercising a real test on
any supported runner.

---

## The latent gap

The 1240 guard lives in `classifyShellErr`, which `runCommand` only calls on
`!succeeded` (`internal/atdd/runtime/actions/bindings.go:964`). By contract the
classifier returns `classOK` whenever `err == nil` (`verify_classify.go:138`) —
a deliberate choice so a *passing* run with noisy stderr is never misread as
infra.

dotnet `test --filter` exits `0` and prints `No test matches the given testcase
filter …` as a **warning**, not an error (confirmed during 1240 execution; it is
why the 1240 dotnet classifier row can never fire in practice). The same is true
of any runner whose default is "no tests = success." For those, a zero-match
selection becomes `test-outcome = pass`:

- a `success`-expecting verify (`verify-tests-pass` / `VERIFY_TESTS_PASS_*`)
  greens having exercised nothing — the exact regression-masking 1240 set out to
  kill, just on the other polarity;
- a `failure`-expecting verify (`verify-tests-fail` / `VERIFY_TESTS_FAIL_*`)
  reads `pass` and routes to `FIX_UNEXPECTED_PASSING_TESTS`, spinning a fixer on
  a non-signal.

The 1240 per-tool stderr patterns do not help here: on a zero-exit run cobra
prints no error, so the marker (if any) lands only in the live stream
(`result.Stdout`), and the classifier is never invoked.

---

## Recommended approach (resolve before encoding)

Two strategies were considered; **Strategy A is recommended** — it unifies all
runners under one mechanism and removes reliance on each tool's idiosyncratic
"no tests" wording.

**Strategy A — runner-side "zero executed" → non-zero exit (recommended).**
Make `gh optivem test run` itself fail when zero tests executed across the
selected suites. The runner already knows each suite's report path
(`internal/runner/tests.go:162-171`, `suite.TestReportPath`); after a suite
exits 0, parse the report's executed-test count and, if the total across all
selected suites is 0, return an error with a uniform gh-optivem marker (e.g.
`ERROR: 0 tests executed for the given selection`). This drops the empty case
into the existing non-zero path, where one classifier row matching the
gh-optivem marker catches it cross-runner — no per-tool string matching, dotnet
included. The 1240 per-tool stderr patterns become a redundant safety net.

**Strategy B — scan-on-success for the empty marker (lighter, not recommended).**
Add a narrow check in `runCommand` that, even on `succeeded`, scans
`result.Stdout`+`result.Stderr` for the empty-selection marker family and, on a
match, overrides `test-outcome` to `infra`. Cheaper (no report parsing) but
keeps the brittle per-tool string dependency the recommended approach removes,
and must be carefully scoped so only the empty-selection family — never the
launch-failure patterns — can override a success.

---

## Items

### 1. [internal/runner] Detect zero-executed tests and fail the run

**Where:** `internal/runner/tests.go` — the per-suite run path (~`:155-174`) and
the suite-summary aggregation (~`:300-313`).

**Change (Strategy A):** after a suite's command exits 0, parse its
`TestReportPath` for the executed-test count. Aggregate across all selected
suites; if the total is 0, return an error carrying a uniform, gh-optivem-owned
marker string. Report formats span the supported languages — JUnit XML
(Gradle/Maven), TRX (dotnet), JSON (Playwright/Jest); confirm each suite
declares a `TestReportPath` and pick a parse path per format.

**Blast radius:** changes the exit-code contract of `gh optivem test run` (now
fails on zero-executed). Audit any non-verify caller of `test run` that might
legitimately select zero. **Gate for review.**

### 2. [actions/verify_classify.go] Match the uniform zero-executed marker

**Where:** `internal/atdd/runtime/actions/verify_classify.go` — the
`infraPatterns` table (the 1240 `"empty test selection"` row).

**Change:** add (or fold into the existing row) a pattern matching the
gh-optivem marker from Item 1, so the now-non-zero empty run classifies `infra`
exactly as the Gradle case does today. Add a `verify_classify_test.go` case and
a `bindings_test.go` `runCommand` wiring case mirroring the 1240 pair.

**Blast radius:** one classifier row + tests. Inherits the existing
`test-outcome == infra → TESTS_INFRA_HALT` routing — no BPMN change (confirmed
in 1240 Item 3 that every verify site already carries the infra branch).

### 3. [audit] Enumerate exit-0-on-empty runners and confirm coverage

**Where:** the per-language suite definitions / runner configs and Item 1's
report-count path.

**Change:** confirm which supported runners exit 0 on a zero-match filter
(dotnet confirmed; verify Maven without `failIfNoTests`, Jest `--passWithNoTests`,
Playwright). Confirm Item 1's report-count detection fires for each, and that no
suite legitimately runs zero tests by design. Expectation: dotnet is the primary
gap; the count-based check should cover all of them uniformly.

---

## Verification

(Operator-driven; not agent `## Items` work.)

- Run a verify with a deliberately empty `--test=` filter on a **.NET** project
  and confirm it now **halts** (infra) instead of greening a `success` verify or
  spinning the passing-tests fixer on a `failure` verify.
- Repeat for each runner identified in Item 3 that defaults to exit-0-on-empty.
- Per the statemachine-test-loop hazard memory: audit gate fixtures before
  running any statemachine test exercising the new path; kill on memory climb.
