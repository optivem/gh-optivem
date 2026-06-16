# 2026-06-16 11:41 UTC — Named-test verify: presence check across partitioned suites (all languages)

## TL;DR

**Why:** `gh optivem test run --suite=acceptance --test=<name>` fans out to **four** partitioned sub-suites (`acceptance-api`, `acceptance-ui`, `acceptance-isolated-api`, `acceptance-isolated-ui`) and applies the `--test` name filter to **every** one. A named test exists in only the partition matching its isolation tag + channel, so the other sub-suites legitimately match zero. On Java, Gradle's `failOnNoMatchingTests=true` turns that per-suite empty slice into a hard exit-1 ("No tests found for given includes"), which the ATDD flow's verify-classifier labels `classInfra` → `TESTS_INFRA_HALT`. Rehearsal #76 died here: its new test `cannotCancelAnOrderAt2245OnDec31` is `@Isolated`, so the first sub-suite (`acceptance-api`, `-DexcludeTags=isolated`) found nothing and aborted the whole run before the isolated sub-suites ran.

**Regression origin:** shop commit `b944b48c` (2026-06-06) "fold isolated acceptance suites into the acceptance group" expanded the category from `[acceptance-api, acceptance-ui]` to all four. Before that, every acceptance test was present in both non-isolated suites, so a single `--test` always matched.

**End result:** A named verify passes iff **every requested test name actually executed at least once across the union of sub-suites** — isolated or not, API or UI, parameterized or not. A per-suite empty slice is no longer fatal; the union presence check is the single enforcement point. Works uniformly across Java / dotnet / TypeScript.

## Design — why per-name presence, not a count

Rejected alternatives:
- **Per-suite "no tests found" stays fatal** — the actual bug; can't keep it.
- **`totalExecuted > 0` across all suites** — fixes the single-name case but `test-names` is a *comma-separated list* (process-flow.yaml:90), so a multi-name verify where name A runs and name B is typo'd/gated-off still totals >0 and greens while B never ran. Real hole.
- **Exact `executed == written` count** — impossible: each test is a `@TestTemplate`/parameterized method that explodes to a variable number of invocations (one per channel × per `@DataSource` row). One method ≠ one count.

Chosen: **set membership `R ⊆ E`.** `R` = requested names (the `--test` set / writer's `test-names`). `E` = the set of **method names that executed at least once**, unioned across all sub-suites, recovered from each report's per-testcase entries and de-duplicated back to the bare method name. Collapsing invocations to method names is what makes the check agnostic to isolation, channel, and parameterization simultaneously. The check runs **only when `--test` is non-empty**; unfiltered whole-suite runs keep today's `totalExecuted == 0` guard.

### Per-report name shapes (verified against real reports)

| Lang | Report | Per-test entry | Bare method = |
|---|---|---|---|
| Java | JUnit XML dir `build/test-results/test/*.xml` | `<testcase name="method [Channel: UI]" classname="…">` | `name` up to first ` [` |
| dotnet | TRX `TestResults/testResults.trx` | `<UnitTestResult testName="ShouldPlaceOrder(channel: UI)">` | `testName` up to first `(` |
| TS | Playwright JSON (**not yet configured**) | `suites[].specs[].title` (+ nested) | `title` (channel handled by project, not in title) |

Stripping rule is per-format (different separators), so the extractor dispatches on extension exactly like `countExecutedTests` does.

## Per-language reality (they are NOT in the same state)

- **Java** — hard-broken. `failOnNoMatchingTests=true` aborts per-suite; **no** `testCountPath` declared, so even the existing count guard is inert. Needs: build.gradle opt-out + `testCountPath` + the new name check.
- **dotnet** — *not* hard-broken. `dotnet test` exits 0 on a zero-match `--filter`, and the acceptance suites already declare `testCountPath` (TRX). It limps through today on the weak guard. Needs: name extraction from TRX so it gets the strong `R ⊆ E` check too. Likely **no** consumer build/config change.
- **TypeScript** — likely broken like Java: Playwright exits non-zero ("Error: No tests found") on a zero-match `--grep`, and there is **no machine-readable report** (`playwright.config.ts` reporters = `channel-list-reporter` + `html` only) and **no** `testCountPath`. Needs: a JSON reporter wired, `testCountPath` pointing at it, `--pass-with-no-tests` on the suite commands, plus name extraction.

## ▶ Next executable step (resume here)

Start with **Step 1** (gh-optivem runner: name extraction + `R ⊆ E` guard + unit tests) — it is the language-shared heart and unblocks all three consumers. It is self-contained and testable with fixtures (no consumer repo needed). Steps 2–4 are the per-language consumer (shop) wiring; Step 5 verifies end-to-end on the #76 rehearsal.

## Steps

- [ ] **Step 1 — gh-optivem runner: presence check (the core).**
  - New `internal/build/runner/testnames.go`: `executedTestNames(path string) (map[string]bool, error)` mirroring `countExecutedTests`'s extension/dir dispatch, returning the set of **bare method names**. JUnit: collect `<testcase name>`, cut at first ` [`. TRX: collect `<UnitTestResult testName>`, cut at first `(`. Playwright JSON: walk `suites[].specs[].title` (and nested suites). Missing file → empty set (same rule as the counter). Malformed-but-present → error.
  - `internal/build/runner/tests.go`: when `opts.Test` is non-empty, accumulate the union of `executedTestNames(suite.TestCountPath)` across suites; after the loop assert every requested name ∈ union, else return an error naming the **missing** tests (this is the new infra-vs-red boundary — a genuinely-absent named test is a wiring/typo failure, not a red). Keep the existing `totalExecuted == 0` guard for the unfiltered path. Reuse `TestCountPath` as the report source (no new config key).
  - `internal/build/runner/testnames_test.go`: per-format fixtures (reuse `testcount_test.go` shapes + add `<testcase>`/`<UnitTestResult>`/`specs` bodies), dir-glob (Java), de-dup of channel/param invocations to one method name, missing-file→empty, malformed→error. Extend `tests_test.go` (if a `RunTests`-level harness exists) with: isolated test present only in 2 of 4 suites → passes; one of two requested names absent → fails naming it.
  - Scope `go test ./internal/build/runner/...` (no unbounded `./...` on Windows).

- [ ] **Step 2 — shop Java consumer.**
  - `system-test/java/build.gradle`: in the `test { filter { … } }` block add `setFailOnNoMatchingTests(false)` so a per-suite empty slice exits 0 instead of 1. (Filters still apply; this only stops the empty-match hard error.)
  - `system-test/java/tests.yaml`: add `testCountPath: build\test-results\test` to the four `acceptance-*` suites (the XML dir). Without it the new check has no name source and a zero-run would silently green.

- [ ] **Step 3 — shop dotnet consumer.**
  - Verify TRX `testName` extraction matches real output (`ShouldPlaceOrder(channel: UI)` → `ShouldPlaceOrder`). `testCountPath` already present. Expect **no** build/config edit — confirm by reproducing a named isolated verify.

- [ ] **Step 4 — shop TypeScript consumer.**
  - `playwright.config.ts`: add a `['json', { outputFile: '…/results.json' }]` reporter alongside the existing two.
  - `system-test/typescript/tests.yaml`: add `testCountPath:` pointing at that JSON file to the four `acceptance-*` suites; append `--pass-with-no-tests` to their commands so a zero-match `--grep` exits 0.
  - Confirm Playwright JSON `specs[].title` is the bare test name (channel is a project, not in the title).

- [ ] **Step 5 — end-to-end verify.**
  - Re-run the #76 rehearsal (monolith Java config) and confirm the named isolated test is found, runs red as expected, and the flow proceeds past `VERIFY_TESTS_FAIL_ACCEPTANCE` instead of `TESTS_INFRA_HALT`. Spot-check a dotnet and a TS named verify if a quick path exists.
  - Confirm the WIP-gate env var (`GH_OPTIVEM_RUN_WIP_TESTS`) genuinely lifts in the isolated sub-suite now that the run reaches it (it never got exercised before — the run died on the non-isolated suite first).

## Open questions

1. **Missing-name failure: infra-halt or red?** A requested name that executed nowhere is a wiring/typo/gated-off problem, not a legitimate red. Plan routes it to a hard error (≈ infra). Confirm that's the desired flow branch vs. surfacing it as a distinct "named test never ran" halt label in `verify_classify.go`.
2. **dotnet `--filter` zero-match exit code.** Plan assumes `dotnet test` exits 0 on zero match (so no consumer change). If a given runner/SDK version exits non-zero, dotnet needs the same `--pass-with-no-tests`-equivalent treatment as TS. Verify in Step 3.
3. **Scope beyond acceptance.** Contract suites are also partitioned (`-DexternalSystemMode`, isolated). Apply the same `testCountPath` + presence check there, or keep this change scoped to `acceptance` and follow up for `contract`?
4. **Single report path reused for count and names.** Reusing `TestCountPath` keeps config lean but couples the two; acceptable, or introduce a distinct `testNamesPath`? (Recommend reuse — same file, both derivable.)
