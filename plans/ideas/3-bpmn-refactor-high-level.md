# BPMN - HIGH LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

> **Q-new-1 doctrine (resolved 2026-05-25).** HIGH orchestrations are parameterized on `<Expected Test Result>`; the prior "red"-prefixed names were misleading because every orchestration ultimately routes through the shared `implement-test-layer` which already takes Expected Test Result as input. "red" has been dropped from every HIGH name. `change-system-behavior` invokes these with `<Expected: Failure>`; `cover-system-behavior` with `<Expected: Success>`.

===========================

## write-and-verify-tests-fail

Thin wrapper — gives CYCLE callers a parameter-free, self-documenting name. Called by `change-system-behavior`.

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**OUTPUT: Tests**

1. `write-and-verify-tests` `<Expected Test Result: Failure>`

## write-and-verify-tests-pass

Thin wrapper — gives CYCLE callers a parameter-free, self-documenting name. Called by `cover-system-behavior`.

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**OUTPUT: Tests**

1. `write-and-verify-tests` `<Expected Test Result: Success>`

===========================

## write-and-verify-tests

Parameterized core — single source of truth for the test-writing orchestration. Operators don't call this directly; they invoke one of the two thin wrappers above (`write-and-verify-tests-fail` / `write-and-verify-tests-pass`). Inner orchestrations (`implement-and-verify-dsl`, etc.) also call this core internally and inherit the parameter.

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**INPUT: Expected Test Result**

**OUTPUT: Tests**

**Port-change wiring** (Q6):

| Producer (mid-level task) | Output variable | Consumer (branch below) |
|---|---|---|
| `write-acceptance-tests` | `dsl-port-changed: bool` | Step 2 — "DSL Port Changed?" |
| `implement-dsl` | `external-driver-ports-changed: bool` | Step 2.1.1 — "External System Driver Ports Changed?" |
| `implement-dsl` | `system-driver-ports-changed: bool` | Step 2.1.2 — "System Driver Ports Changed?" |

Note: the External/System driver port-change checks are nested *under* "YES: `implement-and-verify-dsl`" because those port-change outputs only exist after DSL is implemented.

Note: `<Expected Test Result>` (from INPUT above) is threaded to all nested orchestration calls — no need to repeat per leaf (Q6.a).

1. `write-and-verify-acceptance-tests`
2. DSL Port Changed? (reads `dsl-port-changed` from step 1)
    1. YES: `implement-and-verify-dsl`
        1. External System Driver Ports Changed? (reads `external-driver-ports-changed` from `implement-dsl`)
            1. YES: `implement-and-verify-external-system-driver-adapters`
        2. System Driver Ports Changed? (reads `system-driver-ports-changed` from `implement-dsl`)
            1. YES: `implement-and-verify-system-driver-adapters`

## write-and-verify-acceptance-tests

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**INPUT: Expected Test Result**

**OUTPUT: Tests**

1. `write-acceptance-tests` (AGENT)
2. `compile-tests`
3. Based on `<Expected Test Result>`:
    1. If expect success: `verify-tests-pass`
    2. If expect failure:
        1. `verify-tests-fail`
        2. `disable-tests` (AGENT)
4. `commit`

## implement-and-verify-dsl

1. `implement-test-layer`
    1. Agent Action: `implement-dsl`

## implement-and-verify-system-driver-adapters

1. `implement-test-layer`
    1. Agent Action: `implement-system-driver-adapters`

## implement-and-verify-external-system-driver-adapter-contract-tests

1. `write-contract-tests` (AGENT)
    1. Note: supposed to think about the External System Driver Ports
    2. Output: list of tests
2. DSL Port Changed?
    1. YES: `implement-and-verify-dsl`
        1. Note: supposed to use the External System Driver Ports
3. `implement-external-system-driver-adapters` (AGENT)
4. `verify-tests-pass` <Contract Tests - Real>
5. `verify-tests-fail` <Contract Tests - Stub>
6. `implement-external-system-stubs` (AGENT)
7. `verify-tests-pass` <Contract Tests - Stub>

===========================

## implement-and-verify-system

1. `implement-system`
2. `build-system`
3. `start-system`
4. `verify-tests-pass` <Tests>
5. `commit`

===========================

## refactor-and-verify-tests

1. `refactor-tests`
2. `compile-tests`
3. `verify-tests-pass`
4. `commit`

===========================

## 《 SHARED 》implement-test-layer

1. Execute <Agent Action>
2. `enable-tests` <Tests>
3. `compile-tests`
4. Based on result we expect: <Expected Test Result>
    1. If expect success:
        1. `verify-tests-pass`
    2. If expect failure:
        1. `verify-tests-fail` <Tests>
        2. `disable-tests` <Tests>
5. `commit`

## 《 SHARED 》verify-tests-pass

1. `run-tests`
2. Success?
    1. YES: END
    2. NO: `fix-unexpected-failing-tests`

## 《 SHARED 》verify-tests-fail

1. `run-tests`
2. Success?
    1. YES: `fix-unexpected-passing-tests`
    2. NO: END


===========================

Run Tests filter: see Q5/MID brainstorm — single `run-tests` task with polymorphic filter accepting a test-type tag, a list of test names, or none.
