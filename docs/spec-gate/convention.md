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

Default spec directory: `docs/use-cases/` (configurable, see section 8). One file per use case.

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
- A criterion is a list line: `- <ID>: <text>`, under the scenario or extension it belongs to. The heading it sits under is its grouping context.
- An optional `[flag: <name>]` follows the ID (see section 6).
- Everything else in the file is free prose and is ignored.
- No Gherkin in v1.

## 4. Linking tests to criteria

The criterion ID is carried by a test **tag** (JUnit tag, surfaced as a property in the XML report). The gate reads only the JUnit XML; it does not scan source.

- Put the tag on the **top-layer test only** (the test a reader would call the acceptance test). Do not tag helper layers.
- One test may carry several IDs; one ID may be carried by several tests (many-to-many). Many IDs on one test is reported as a smell candidate (later version).
- The tag `wip` marks a test as in progress.

### Tag syntax per stack

| Stack | Syntax | Notes |
|---|---|---|
| Java (JUnit 5) | `@Tag("UC-001.3")`, `@Tag("wip")` | Repeatable; several tags per method. **Stock Gradle does not write tags into the XML** (it writes an empty `<properties/>`), so a `TestExecutionListener` plus a `doLast` step on the `test` task must inject them as `<property>` entries; see below. |
| .NET (xUnit/NUnit) | `[Trait("Criterion", "UC-001.3")]` / `[Category("UC-001.3")]` | Configure the logger so traits/categories are written as `<property>` entries. |
| TypeScript (Jest/Playwright/Vitest) | Annotation or title tag, e.g. `{ tag: ["@UC-001.3"] }` | Use a JUnit reporter that emits tags as `<property name="tag" value="UC-001.3">`; a leading `@` is stripped. |
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

Precedence when conditions overlap: Waived (if unexpired) > In progress (`wip`) > Failing > Verified > Planned. A skipped result does not count as passing; a criterion with only skipped results is not Verified.

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

- duplicate criterion IDs;
- a criterion ID whose prefix does not match its use case `id`;
- malformed or missing front matter.

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
| `baseline` | string | `<specDir>/.spec-baseline.json` | Adoption baseline path |

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
    "failing": 0, "planned": 1, "waived": 0,
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
