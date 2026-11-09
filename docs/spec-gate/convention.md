# Spec-coverage convention (v1)

This document defines how acceptance criteria are written, how tests link to them, and how a gate derives a lifecycle state per criterion from test evidence. It is independent of any test architecture or tool: any stack that can emit a test tag/property into JUnit XML can follow it.

## 1. Concepts

- **Use case**: a markdown file describing a main scenario plus extensions (alternative and exception flows).
- **Criterion**: one path through a use case, with a stable ID. Each criterion needs at least one acceptance test, so criteria coverage is path coverage.
- **Spec coverage**: `verified criteria / total criteria`, always shown with its definition (for example "21 of 27 acceptance criteria verified"). The gate cannot know whether the list of paths is complete; that is spec quality, a review concern.

## 2. ID format

`UC-<n>.<m>` where `<n>` is the use case number and `<m>` the criterion number within it (`UC-001.3`).

- The prefix before the dot must equal the use case `id` (`UC-001`).
- IDs are unique across the whole spec directory.
- IDs are never reused once published; retire a criterion instead of renumbering.

## 3. Markdown layout

Default spec directory: `docs/use-cases/` (configurable, see section 8). Two layouts, which can be mixed:

- **Flat:** one `.md` file per use case directly in the spec directory. `README.md` is skipped (use it as an index).
- **Folder per use case:** `<specDir>/UC-NNN-<name>/acceptance-criteria.md` is the spec file. Other files in the folder (for example `use-case-narrative.md`, `business-rules.md`) are for people and are not parsed. A `UC-NNN-*` folder whose front-matter `id` differs gets a warning.

Criteria are written as bullets or as tagged Gherkin scenarios.

```markdown
---
id: UC-001
title: Place Order
---

# UC-001 Place Order

## Main scenario
1. Customer enters SKU, quantity, country.
2. System validates input, creates order, confirms.

- UC-001.1: A valid order is placed and gets status PLACED.
- UC-001.2: Base price = unit price x quantity.

## Extensions
### 5a. ERP unavailable
- UC-001.14: An "ERP unavailable" message is shown.
- UC-001.15 [flag: erp-fallback]: Orders fall back to cached prices.
```

Rules:

- Front matter is required and must contain `id` and `title`. Missing or malformed front matter is a validation error.
- Optional front matter `status: implemented | planned` (default `implemented`). A planned use case is specified but not built yet: it is validated and listed as "planned, N criteria, not gated", but left out of the totals, coverage and threshold.
- A criterion is a list line: `- <ID>: <text>`, under the scenario or extension it belongs to. The heading it sits under is its grouping context.
- An optional `[flag: <name>]` follows the ID (see section 6).
- Everything else in the file is free prose and is ignored.

Gherkin form, inside a fenced ```` ```gherkin ```` block:

```gherkin
Feature: Place Order

  @UC-001.1
  Scenario: Should place order with valid input
    When the customer places an order
    Then the order is placed

  @UC-001.8 @narrative-4a
  Scenario: Should reject order with zero quantity
    When the customer places an order with quantity 0
    Then the order is rejected
```

- Each distinct `@UC-NNN.M` tag on a `Scenario:` / `Scenario Outline:` / `Example:` is one criterion. Other tags (such as `@narrative-4a`) are ignored.
- Several scenarios may share one tag; they form one criterion whose text is their titles joined with `; ` (so editing any of them marks the criterion Changed).
- The grouping context is the enclosing `Rule:` title, else the markdown heading above the block.
- A scenario with two `@UC-…` tags is a validation error (one scenario, one criterion). A scenario with none is a warning: it is not a criterion.
- Scenarios outside a `gherkin` fence are ignored.

## 4. Linking tests to criteria

The criterion ID is carried by a test **tag** (JUnit tag, surfaced as a property in the XML report). The gate reads only the JUnit XML; it does not scan source.

- Put the tag on the **top-layer test only** (the test a reader would call the acceptance test). Do not tag helper layers.
- One test may carry several IDs; one ID may be carried by several tests (many-to-many). Many IDs on one test is reported as a smell candidate (later version).
- The tag `wip` marks a test as in progress.

### Tag syntax per stack

| Stack | Syntax | Notes |
|---|---|---|
| Java (JUnit 5) | `@Tag("UC-001.3")`, `@Tag("wip")` | Repeatable; several tags per method. **Stock Gradle does not write tags into the XML** (it writes an empty `<properties/>`), so a `TestExecutionListener` plus a `doLast` step on the `test` task must inject them as `<property>` entries; see below. |
| .NET (xUnit) | `[Trait("tag", "UC-001.3")]`, one per ID | `JunitXml.TestLogger` (`dotnet test --logger "junit;LogFilePath=..."`) writes each trait as `<property name="tag" value="UC-001.3"/>`. The property name must be lowercase `tag` or `criterion`. |
| TypeScript (Playwright) | Annotation `{ type: "tag", description: "UC-001.3" }` (shop wraps it as `criteria("UC-001.3")`) | Playwright's built-in `junit` reporter writes annotations as `<property name="tag" value="UC-001.3"/>`; a leading `@` is stripped. |
| Any other | Emit a `<property>` (name `tag` or `criterion`, value the ID) or a test-case-level tag in JUnit XML | The only contract is that the ID appears as a property value on the `<testcase>`. |

### Java and Gradle: getting tags into the XML

Verified on a real run of shop's Place Order tests: Gradle's JUnit XML writer drops `@Tag` values. The working setup (see shop `system-test/java`):

1. A `TestExecutionListener` (registered through `META-INF/services/org.junit.platform.launcher.TestExecutionListener`, with `junit-platform-launcher` on the test compile classpath) appends `class<TAB>method<TAB>tags` lines for each planned method to a per-JVM file under a directory passed as a system property. Gradle runs one launcher execution per class, so the listener must append, not overwrite. `@TestTemplate` invocations are created at run time, so record the template container (it has a `MethodSource` and the method's tags), not only `isTest()` identifiers.
2. A `doLast` on the `test` task reads those files and inserts `<properties><property name="tag" value="UC-001.n"/></properties>` into each matching `<testcase>` (the method name is the `name` attribute up to the first space or `(`).

### Result names the gate normalises

A templated test appears as `method [Channel: API]`; a plain test as `method()`; both collapse to `method`. A placeholder result named like `method() [Skipped: UI channel was not specified]` means the test does not apply to that channel; the gate ignores it rather than counting a skipped result.

Recognised IDs are property values matching `^UC-\d+\.\d+$`. The value `wip` marks in progress. Other values are ignored.

### Parameterised and templated tests

Results named `method [Channel: UI]` (a templated or parameterised method run once per channel) are normalised by stripping a trailing ` [...]` suffix. Tags, not method names, are the primary link, so one tagged method with several channel results counts as **one test with several results**; all results must pass for the test to pass.

## 5. Lifecycle

| State | Meaning | Condition |
|---|---|---|
| Planned | Specified, no test yet | No linked test |
| In progress | Work started | A linked test is tagged `wip` |
| Verified | Proven | At least one linked test, all linked results pass, none tagged `wip` |
| Failing | Broken | A linked result is red (failure or error) and not tagged `wip` |
| Waived | Deliberate, time-boxed exception | Waiver entry present and not expired |
| Changed | Text edited since it was verified | The linked tests would make it Verified, but the criterion text no longer matches the fingerprint recorded when it was last verified (section 9, Spec state file) |

Precedence when conditions overlap: Waived (if unexpired) > In progress (`wip`) > Failing > Changed > Verified > Planned. A Changed criterion counts as not verified, so a passing old test cannot hide a changed requirement. A skipped result does not count as passing; a criterion with only skipped results is not Verified.

**Use case state** is derived from its criteria: Planned (no criterion has a test), In progress (some verified, or a linked test tagged `wip`), Verified (all criteria verified).

A Waived criterion counts as **not verified** in the coverage percentage, but is not reported as a gap until its waiver expires; an expired waiver reverts the criterion to its test-derived state.

"Needs Clarification" is not part of v1.

## 6. Waivers and flags

### Waiver (expiring exception)

Declared in `spec.yaml`:

```yaml
waivers:
  - id: UC-001.14
    reason: ERP sandbox unavailable until vendor delivers credentials
    approver: jane.doe
    until: 2026-12-31
```

All four fields are required. `until` is an ISO date; the waiver is valid through that date inclusive.

### Flag (deliberate dark work)

A criterion may carry `[flag: <name>]` in the markdown. In strict mode it is excluded from the gate unless the flag is on (passed with `--flag <name>` or listed under `flags:` in `spec.yaml`). A flag has no expiry or approver; a waiver is an expiring exception, a flag is unfinished work hidden behind a feature flag.

## 7. Orphans and validation

`validate` fails (error) on:

- duplicate criterion IDs (across files, or a bullet and a scenario with the same ID);
- a criterion ID whose prefix does not match its use case `id`;
- malformed or missing front matter, or a `status` other than `implemented`/`planned`;
- a Gherkin scenario tagged with more than one criterion ID;
- an implemented use case with no criteria (so a file in an unreadable format can't pass silently).

It warns (without failing) on a Gherkin scenario with no criterion tag and on a `UC-NNN-*` folder whose front-matter `id` differs.

The report also lists:

- **unknown IDs**: a test references an ID that is not in any spec;
- **orphan tests**: tagged tests whose IDs are all unknown, or results that cannot be linked to a spec.

## 8. `spec.yaml`

Located at the repository root.

| Key | Type | Default | Meaning |
|---|---|---|---|
| `specDir` | string | `docs/use-cases` | Directory of use-case markdown files |
| `results` | string (glob) or list | none (required) | JUnit XML result files, for example `**/build/test-results/**/*.xml` |
| `threshold` | number 0-100 | `100` | Minimum spec coverage in strict mode |
| `waivers` | list | `[]` | Waiver entries (section 6) |
| `exclude` | list of globs | `[]` | Result files to ignore, for example `**/legacy/**` |
| `flags` | list of strings | `[]` | Flags considered on |
| `baseline` | string | `<specDir>/.spec-baseline.json` | Spec state file path (adoption baseline and criterion fingerprints) |

## 9. Gating

The mode is chosen by the **pipeline stage**, never by branch. `--mode lenient|strict`.

| Stage | Mode | Blocks | Reports only |
|---|---|---|---|
| Commit stage (every push) | lenient | validate errors, Failing | Planned, In progress, Waived |
| Acceptance stage | lenient | validate errors, Failing | Planned, In progress, Waived |
| Release / promotion | strict | validate errors, Failing, any in-scope criterion not Verified, coverage below `--threshold` | Waived (until expiry) |

In strict mode, in-scope means: not excluded by an off flag and not waived (unexpired).

### Adoption baseline

`adopt` writes the baseline file listing currently unverified criteria. In strict mode, baselined criteria are non-blocking; a criterion that was Verified at baseline and regresses, or any new unverified criterion, blocks.

### Spec state file and criterion fingerprints

The baseline file is the machine-managed spec state file. Besides the `unverified` list it holds `fingerprints`, a map from criterion id to a hash of the criterion text (whitespace-normalised SHA-256, truncated to 16 hex characters):

```json
{ "schemaVersion": 1, "unverified": ["UC-001.3"], "fingerprints": { "UC-001.1": "7f3a9c01d2b4e856" } }
```

- `adopt` writes a fingerprint for every Verified criterion.
- `record` adds the fingerprint of each Verified criterion that has none yet (run it after a story's acceptance tests are green), drops fingerprints of criteria that no longer exist, and never overwrites a differing fingerprint. `--criteria <ids>` limits it to the named criteria (a named one that is not Verified is reported and skipped); `--create` creates a missing state file with an empty `unverified` list instead of failing, so no existing gap is baselined. `gh optivem implement` runs `record --create --criteria <the story's merged criteria>` for every story once its acceptance tests are green on all channels, and commits the state file.
- When a Verified criterion's text no longer matches its fingerprint, `status` and `check` report it as **Changed** (unverified); strict mode blocks it. After the criterion's test has been re-checked against the new wording, `record --refresh <id>` accepts the new text.
- A criterion with no recorded fingerprint is never Changed.

This is how a story that edits a criterion (merged into the use cases before ATDD builds it) shows up as unverified until its test is written or re-checked.

### Exit codes

`0` pass, `1` gate failed, `2` usage or configuration error.

## 10. JSON report, schema v1 (outline)

`--json` output is the stable contract that every other integration consumes. Breaking changes bump `schemaVersion`.

```json
{
  "schemaVersion": 1,
  "mode": "lenient",
  "threshold": 100,
  "summary": {
    "total": 14, "verified": 13, "inProgress": 0,
    "failing": 0, "planned": 1, "waived": 0, "changed": 0,
    "coveragePercent": 92.9
  },
  "useCases": [
    {
      "id": "UC-001", "title": "Place Order", "state": "In progress",
      "criteria": [
        {
          "id": "UC-001.1", "text": "...", "group": "Main scenario",
          "state": "Verified", "flag": null,
          "tests": [
            { "name": "shouldPlaceOrder", "results": [ { "name": "shouldPlaceOrder [Channel: UI]", "status": "passed" } ], "wip": false }
          ],
          "waiver": null,
          "baselined": false
        }
      ]
    }
  ],
  "orphans": [ { "name": "...", "reason": "no known criterion ID" } ],
  "unknownIds": [ { "id": "UC-009.1", "test": "..." } ],
  "validation": { "errors": [ { "code": "duplicate-id", "message": "..." } ] },
  "gate": { "passed": true, "reasons": [] }
}
```

Result `status` is one of `passed`, `failed`, `error`, `skipped`. State strings are exactly: `Planned`, `In progress`, `Verified`, `Failing`, `Waived`.

## 11. Outputs

Terminal report; `--json`; GitHub job summary (`GITHUB_STEP_SUMMARY`); sticky pull-request comment markdown starting with the marker `<!-- gh-optivem-spec -->` (printed, not posted); a self-contained HTML report readable by a product owner. No commit-status posting in v1.

## 12. Generating readable criteria

`gh optivem spec generate` turns the tagged acceptance tests back into readable Gherkin. A product owner or customer can then read the criteria after a story's own criteria text has been deleted. The command reads the test sources (`.java`, `.cs`, `*.spec.ts` / `*.test.ts`), not the JUnit XML, because only the sources have both the tag and the steps. It prints one `Feature` per use case. The output is generated, so do not commit it.

```
gh optivem spec generate                                  # tests from system-test.paths.at-test in gh-optivem.yaml
gh optivem spec generate --tests <dir> --use-case UC-001 --out criteria.feature
```

Mapping rules:

- `.given()` / `.when()` / `.then()` open a `Given` / `When` / `Then` line. `.and()` opens an `And` line.
- Identifiers are split into lowercase words. Acronyms keep their case.
- A Given or When line is the subject followed by its comma-joined modifiers. A one-word Given subject gets "a" or "an".
- Every assertion after `.then()` is its own `And` line. An assertion on an entity is prefixed by that entity.
- Strings are rendered in double quotes. Numbers lose type suffixes. `null` becomes `none`. Enums keep their last segment.
- Parameterised tests become `Scenario Outline` with `Examples`. A provider class or method is named in a comment.
- Each scenario carries one `@UC-NNN.M` tag, so a test with several IDs appears under each. Wip tests also carry `@wip`. Untagged tests go in a trailing `Feature: Untagged acceptance tests` block.
- Channels and isolation are not rendered.

Before:

```java
@Tag("UC-001.2")
@TestTemplate
void shouldCalculateBasePriceAsProductOfUnitPriceAndQuantity() {
    scenario
            .given().product().withUnitPrice("20.00")
            .when().placeOrder().withQuantity(5)
            .then().shouldSucceed()
            .and().order().hasBasePrice("100.00");
}
```

After:

```gherkin
Feature: UC-001 Place Order

  @UC-001.2
  Scenario: Should calculate base price as product of unit price and quantity
    Given a product with unit price "20.00"
    When place order with quantity 5
    Then should succeed
    And order has base price "100.00"
```

It is all or nothing. A test the grammar cannot read fails the run with `file:line test-name: reason`, and so does a criterion whose use case is not in `specDir`. On such a failure nothing is printed or written. Exit codes: 0 ok, 1 unparseable tests or unknown IDs, 2 usage or configuration error.
