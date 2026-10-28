# gh-optivem spec: shop trial run (Place Order, Java monolith)

Run of the Step 7 CLI (`gh optivem spec`) against a copy of the shop Java system tests. Inputs: `UC-001-place-order.md` (14 criteria) and the tag assignment in `mapping.md`.

## Disclosure: JUnit XML is synthetic

The XML was **generated, not produced by a Gradle run**. The shop system tests need the running system (Docker, Keycloak and the other services), so no real test run was attempted. A script generated one `<testsuite>` per tagged class with one `<testcase>` per channel result, named `method [Channel: UI]` / `[Channel: API]` (derived from each method's `@Channel` annotation; `alsoForFirstRow` counted as API plus UI), all passing, with one `<property name="tag" value="UC-001.n"/>` per tag. 5 classes, 65 results. Known unverified risk: stock Gradle JUnit XML does not write JUnit 5 tags as `<property>` entries, so a real run needs a reporter or listener that emits them (the convention doc already states the contract). That is the main thing a real run must still prove.

## Setup (all in a temp copy; `shop` untouched, nothing pushed)

- Copied only `shop/system-test/java/src` (not the full repo) to a short temp path (long paths broke Python on Windows).
- Added `@Tag("UC-001.n")` (plus the `org.junit.jupiter.api.Tag` import) above `@TestTemplate` on the top-layer Place Order tests in `latest/acceptance` (4 classes) and `latest/e2e` (1 class), per `mapping.md`. Criterion 3 and 4 share `orderTotalShouldApplyCouponDiscountAndTax` (two tags); criterion 14 has no test.
- `docs/use-cases/UC-001-place-order.md` copied from the trial folder.
- `spec.yaml`:

```yaml
specDir: docs/use-cases
results: "**/build/test-results/**/*.xml"
exclude:
  - "**/legacy/**"
threshold: 90
```

- A fake `legacy/` result file (same tests, one red) was added to prove the exclude glob.
- Binary: `go build -o gh-optivem-spec.exe .`

## Results

| Command | Outcome | Exit |
|---|---|---|
| `spec check --mode lenient` | 13 of 14 verified (92.9%), criterion 14 Planned, gate PASSED | 0 |
| `spec check --mode strict` (threshold 90) | same counts, gate FAILED: "UC-001.14 is Planned (strict requires Verified)" | 1 |
| `spec check --mode strict --json --html report.html` | valid schema v1 JSON; summary 13/0/0/1; no orphans, unknown IDs or validation errors; HTML written (11 KB) | 1 |
| `spec check --pr-comment` | sticky comment starting `<!-- gh-optivem-spec -->`, printed | 0 |
| `spec validate` | `OK: 1 use case(s) valid` | 0 |
| `spec status` | terminal report, same as lenient check | 0 |
| `spec adopt` | wrote `docs/use-cases/.spec-baseline.json` with `UC-001.14`; strict check then passes with `note: baselined` | 0 |
| Bad `--mode`, missing `--config` | clear `ERROR:` message | 2 |
| `spec.yaml` without `exclude` | the red legacy duplicate turns criterion 1 Failing: 12 of 14, gate FAILED | 1 |

Expected states met: 13 of 14 criteria Verified, criterion 14 Planned. JSON per-criterion test counts show the many-to-many link and templated normalisation working: UC-001.1 has 4 tests with 8 results, UC-001.3 has 6 tests with 10 results (shared test also on UC-001.4), UC-001.14 has none. No `@TestTemplate` suffix problem (the reqcov failure) occurred: `method [Channel: UI]` and `[Channel: API]` collapse to one test.

## Friction and findings

- No bugs found in `internal/spec` or `spec_commands.go`; no code changes made.
- Real-run risk: tags as XML properties (see disclosure). Document the Gradle setup for this (reporter or `TestExecutionListener`) in Step 9.
- Any unparsable result file aborts the whole run with exit 2 (fail loud, good), but the message names only the file and XML line; a hint to check the `exclude` globs would help.
- `adopt` prints a Windows-style path (`docs\use-cases\...`); cosmetic.
- Dropping the `exclude` glob shows why it matters: shop's `legacy/` duplicates would pollute results.
- Shop spec quality (unchanged from `comparison.md`): the narrative lists 3 extensions while the tests cover about 9 more behaviours, so criteria had to be authored from the tests; criterion 14 is a real gap.
- Effort: tagging 38 tag lines across 5 classes was mechanical (scripted); a human would use the same mapping table.
