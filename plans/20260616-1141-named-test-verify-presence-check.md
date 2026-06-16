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

**Step 1 (runner core) is DONE and committed (gh-optivem `5bdf0dc`).** Resume at **Step 2** — the shop consumer wiring, which is what actually switches the check on (until a suite declares `testCountPath`, the runner change is inert). Steps 2–4 are per-language and independent of each other; Step 5 is the end-to-end #76 rehearsal. The consumer repo is `../shop` (sibling of gh-optivem; `REHEARSAL_REPO=shop`). Per Decision 3, apply the `testCountPath` additions to BOTH the `acceptance-*` and `contract-*` suites in each language's `tests.yaml`.

## Steps

- [x] **Step 1 — gh-optivem runner: presence check (the core).** ✅ Landed: `testnames.go` (`executedTestNames` for JUnit/TRX/Playwright, dedup to method names), `tests.go` (`runOneSuite` returns names; `RunTests` unions + `R ⊆ E` check; `nameExecuted` substring-tolerant match), `verify_classify.go` row + test, `config.go` doc. `go build ./...` clean; `go test ./internal/build/runner/ ./internal/atdd/process/actions/` green (incl. partition-pass, absent-everywhere, multi-name partial-miss cases).
  - New `internal/build/runner/testnames.go`: `executedTestNames(path string) (map[string]bool, error)` mirroring `countExecutedTests`'s extension/dir dispatch, returning the set of **bare method names**. JUnit: collect `<testcase name>`, cut at first ` [`. TRX: collect `<UnitTestResult testName>`, cut at first `(`. Playwright JSON: walk `suites[].specs[].title` (and nested suites). Missing file → empty set (same rule as the counter). Malformed-but-present → error.
  - `internal/build/runner/tests.go`: when `opts.Test` is non-empty, accumulate the union of `executedTestNames(suite.TestCountPath)` across suites; after the loop assert every requested name ∈ union, else return an error of the exact form `requested test(s) never executed: <names>` (parseable by the classifier). Keep the existing `totalExecuted == 0` guard for the unfiltered path. Reuse `TestCountPath` as the report source (no new config key; widen its doc comment per Decision 4).
  - `internal/atdd/process/actions/verify_classify.go`: add one infraPatterns row matching `requested test(s) never executed` → presence-specific label (Decision 1). Add the matching `verify_classify_test.go` case.
  - `internal/build/runner/testnames_test.go`: per-format fixtures (reuse `testcount_test.go` shapes + add `<testcase>`/`<UnitTestResult>`/`specs` bodies), dir-glob (Java), de-dup of channel/param invocations to one method name, missing-file→empty, malformed→error. Extend `tests_test.go` (if a `RunTests`-level harness exists) with: isolated test present only in 2 of 4 suites → passes; one of two requested names absent → fails naming it.
  - Scope `go test ./internal/build/runner/...` (no unbounded `./...` on Windows).

- [ ] **Step 2 — shop Java consumer.**
  - `system-test/java/build.gradle`: in the `test { filter { … } }` block add `setFailOnNoMatchingTests(false)` so a per-suite empty slice exits 0 instead of 1. (Filters still apply; this only stops the empty-match hard error. Task-wide, so it covers acceptance AND contract.)
  - `system-test/java/tests.yaml`: add `testCountPath: build\test-results\test` (the JUnit XML dir) to the four `acceptance-*` suites **and** the `contract-*` suites (Decision 3). Without it the new check has no name source and a zero-run would silently green.

- [ ] **Step 3 — shop dotnet consumer.**
  - Verify TRX `testName` extraction matches real output (`ShouldPlaceOrder(channel: UI)` → `ShouldPlaceOrder`; the runner's `bareMethodTRX` strips args + FQN). `acceptance-*` already declare `testCountPath`; confirm `contract-*` do too (add if missing, Decision 3). Resolve open Decision 2: check `dotnet test`'s exit code on a zero-match `--filter` — if non-zero, add a `--pass-with-no-tests`-equivalent so per-partition empties aren't fatal (Java/TS get this via their own flags). Confirm by reproducing a named isolated verify.

- [ ] **Step 4 — shop TypeScript consumer.**
  - `playwright.config.ts`: add a `['json', { outputFile: '…/results.json' }]` reporter alongside the existing two (`channel-list-reporter`, `html`).
  - `system-test/typescript/tests.yaml`: add `testCountPath:` pointing at that JSON file to the four `acceptance-*` suites **and** the `contract-*` suites; append `--pass-with-no-tests` to their commands so a zero-match `--grep` exits 0 instead of erroring.
  - Confirm Playwright JSON `specs[].title` is the bare test name (channel is a project, not in the title); the runner's `namesPlaywrightJSON` walks nested `suites[].specs[].title`.

- [ ] **Step 5 — end-to-end verify.**
  - Re-run the #76 rehearsal (monolith Java config) and confirm the named isolated test is found, runs red as expected, and the flow proceeds past `VERIFY_TESTS_FAIL_ACCEPTANCE` instead of `TESTS_INFRA_HALT`. Spot-check a dotnet and a TS named verify if a quick path exists.
  - Confirm the WIP-gate env var (`GH_OPTIVEM_RUN_WIP_TESTS`) genuinely lifts in the isolated sub-suite now that the run reaches it (it never got exercised before — the run died on the non-isolated suite first).

## Decisions (resolved 2026-06-16)

1. **Zero-match failure → infra halt, new label.** A requested name that executed in zero partitions yields no verdict (not red/green), so it belongs on the existing `TESTS_INFRA_HALT` — same stop-for-human behavior, no new node/diagram/flow churn. Implement as **one new `verify_classify.go` infraPatterns row** matching the runner's precise error (`requested test(s) never executed: …`) with a presence-specific label naming the real post-fix causes (wrong name, WIP-gated-off everywhere, wrong partition). Rejected: reusing the stale "did they compile?" label (misdirects), and a distinct halt type (plumbing for a distinction the flow never acts on).
2. **dotnet zero-match exit code → verify in Step 3.** Codebase is self-contradictory (`config.go` says exits 0; `verify_classify.go` has a dotnet "No test matches" infra row). Pin the real behavior before touching the dotnet consumer; it decides whether dotnet needs a `--pass-with-no-tests` equivalent.
3. **Scope → acceptance + contract.** The runner core is suite-agnostic and covers contract for free; contract has the identical `*Stub*`/`*Real*` partition bug. Only incremental cost is `testCountPath` on the contract suites (Java's `build.gradle` opt-out is task-wide, so it already covers contract). Quick-confirm the contract partition miss before wiring. Rejected acceptance-only: defers a byte-identical bug to the next contract rehearsal.
4. **Report path → reuse `TestCountPath`.** Count and names come from the same artifact; a separate key is pure drift risk. Keep the field, widen its doc comment to "machine-readable report driving both count and presence." Renaming the field is a breaking config churn across consumer yamls for cosmetics — not worth it.
