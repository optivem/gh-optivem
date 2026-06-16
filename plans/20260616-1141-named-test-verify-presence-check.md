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

**Steps 1–4 are DONE and committed** (gh-optivem runner `5bdf0dc`; shop consumer edits committed). All that remains is **Step 5 — runtime verification**, which is **deferred because it needs a runtime environment** (docker + language toolchains + the SUT) not available in a headless editor session. The next move is operator-run, not an edit: kick off the #76 Java rehearsal (`bash gh-optivem/scripts/atdd-rehearsal.sh 76 --config <java-yaml> --auto --headless`) and confirm it clears `TESTS_INFRA_HALT`; then spot-check a dotnet and a TS named verify (details + the dotnet Decision-2 exit-code check are in Step 5). There is no further code/config edit to make unless a runtime check fails.

## Steps

- [ ] **Step 5 — end-to-end verify (⏳ deferred: needs a runtime env — docker + language toolchains + the SUT; does not run in a headless editor session).** Steps 1–4 are landed: gh-optivem runner (`5bdf0dc`); shop consumer edits (Java `build.gradle` + `tests.yaml`, TS `playwright.config.ts` + `tests.yaml`; dotnet needed no edit — all suites already declare `testCountPath`). Remaining is runtime confirmation:
  - **Java (primary):** re-run the #76 rehearsal on a Java monolith config — `bash gh-optivem/scripts/atdd-rehearsal.sh 76 --config <java-yaml> --auto --headless`. Confirm the named isolated test `cannotCancelAnOrderAt2245OnDec31` is found, runs **red**, and the flow proceeds past `VERIFY_TESTS_FAIL_ACCEPTANCE` instead of `TESTS_INFRA_HALT`. Also confirm `GH_OPTIVEM_RUN_WIP_TESTS` actually lifts in the isolated sub-suite now the run reaches it (never exercised before — the run died on the non-isolated suite first).
  - **dotnet (Decision 2):** confirm `dotnet test --filter` exits **0** on a zero-match selection (`config.go` assumes this; `verify_classify.go`'s "No test matches" row implies otherwise). If it exits non-zero, the per-partition empty aborts the run before the presence check — dotnet then needs its own no-match-tolerant setting (vstest has no `--pass-with-no-tests`; likely a `.runsettings` / `/p:` flag). Config is otherwise complete.
  - **TypeScript:** run a named TS acceptance verify and confirm (a) Playwright writes `playwright-report/results.json` with `specs[].title` = the bare test name, (b) `--pass-with-no-tests` makes non-matching partitions exit 0, and (c) the appended `--grep '<test>'` coexists with the existing `--grep` / `--grep-invert` so the named test runs in ≥1 partition. Wired but entirely unverified at runtime.

## Decisions (resolved 2026-06-16)

1. **Zero-match failure → infra halt, new label.** A requested name that executed in zero partitions yields no verdict (not red/green), so it belongs on the existing `TESTS_INFRA_HALT` — same stop-for-human behavior, no new node/diagram/flow churn. Implement as **one new `verify_classify.go` infraPatterns row** matching the runner's precise error (`requested test(s) never executed: …`) with a presence-specific label naming the real post-fix causes (wrong name, WIP-gated-off everywhere, wrong partition). Rejected: reusing the stale "did they compile?" label (misdirects), and a distinct halt type (plumbing for a distinction the flow never acts on).
2. **dotnet zero-match exit code → verify in Step 3.** Codebase is self-contradictory (`config.go` says exits 0; `verify_classify.go` has a dotnet "No test matches" infra row). Pin the real behavior before touching the dotnet consumer; it decides whether dotnet needs a `--pass-with-no-tests` equivalent.
3. **Scope → acceptance + contract.** The runner core is suite-agnostic and covers contract for free; contract has the identical `*Stub*`/`*Real*` partition bug. Only incremental cost is `testCountPath` on the contract suites (Java's `build.gradle` opt-out is task-wide, so it already covers contract). Quick-confirm the contract partition miss before wiring. Rejected acceptance-only: defers a byte-identical bug to the next contract rehearsal.
4. **Report path → reuse `TestCountPath`.** Count and names come from the same artifact; a separate key is pure drift risk. Keep the field, widen its doc comment to "machine-readable report driving both count and presence." Renaming the field is a breaking config churn across consumer yamls for cosmetics — not worth it.
