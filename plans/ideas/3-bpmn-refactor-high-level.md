# BPMN - HIGH LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## write-and-verify-acceptance-tests-fail

**Inputs:**
- acceptance-criteria

**Outputs:**
- tests

**Steps:**
1. `write-and-verify-acceptance-tests` (acceptance-criteria: `<input>`, expected-test-result: failure)

## write-and-verify-acceptance-tests-pass

**Inputs:**
- acceptance-criteria

**Outputs:**
- tests

**Steps:**
1. `write-and-verify-acceptance-tests` (acceptance-criteria: `<input>`, expected-test-result: success)

## write-and-verify-acceptance-tests

**Inputs:**
- acceptance-criteria
- expected-test-result

**Outputs:**
- tests

**Steps:**
1. `write-and-verify-acceptance-tests-code` (acceptance-criteria: `<input>`, expected-test-result: `<input>`)
2. DSL port changed? (reads `dsl-port-changed` from step 1)
    1. YES: `implement-and-verify-dsl` (expected-test-result: `<input>`, tests: acceptance)
        1. External system driver ports changed? (reads `external-driver-ports-changed` from step 2.1)
            1. YES: `implement-and-verify-external-system-driver-adapters` (expected-test-result: `<input>`, tests: acceptance)
        2. System driver ports changed? (reads `system-driver-ports-changed` from step 2.1)
            1. YES: `implement-and-verify-system-driver-adapters` (expected-test-result: `<input>`, tests: acceptance)

## write-and-verify-acceptance-tests-code

**Inputs:**
- acceptance-criteria
- expected-test-result

**Outputs:**
- tests
- dsl-port-changed: bool

**Steps:**
1. `write-acceptance-tests` (acceptance-criteria: `<input>`, expected-test-result: `<input>`)
2. `compile-tests`
3. Based on expected-test-result:
    1. If success: `verify-tests-pass` (filter: acceptance)
    2. If failure:
        1. `verify-tests-fail` (filter: acceptance)
        2. `disable-tests` (tests: acceptance)
4. `commit`

## implement-and-verify-dsl

**Inputs:**
- expected-test-result
- tests

**Outputs:**
- system-driver-ports-changed: bool
- external-driver-ports-changed: bool

**Steps:**
1. `implement-test-layer` (agent-action: implement-dsl, expected-test-result: `<input>`, tests: `<input>`)

## implement-and-verify-system-driver-adapters

**Inputs:**
- expected-test-result
- tests

**Outputs:** NONE

**Steps:**
1. `implement-test-layer` (agent-action: implement-system-driver-adapters, expected-test-result: `<input>`, tests: `<input>`)

## implement-and-verify-external-system-driver-adapters

**Inputs:**
- expected-test-result
- tests

**Outputs:** NONE

**Steps:**
1. `implement-test-layer` (agent-action: implement-external-system-driver-adapters, expected-test-result: `<input>`, tests: `<input>`)

## implement-and-verify-external-system-driver-adapters-contract-tests

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `write-contract-tests` (expected-test-result: success)
2. DSL port changed? (reads `dsl-port-changed` from step 1)
    1. YES: `implement-and-verify-dsl` (expected-test-result: success, tests: contract)
3. `implement-external-system-driver-adapters`
4. `verify-tests-pass` (filter: contract-real)
5. `verify-tests-fail` (filter: contract-stub)
6. `implement-external-system-stubs`
7. `verify-tests-pass` (filter: contract-stub)

## implement-and-verify-system

**Inputs:**
- agent-action — the MID system-mutating agent task to execute (`implement-system` or `refactor-system`)

**Outputs:** NONE

**Steps:**
1. Call `agent-action` (the MID agent task named by the input)
2. `build-system`
3. `start-system`
4. `verify-tests-pass`
5. `commit`

## refactor-and-verify-tests

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `refactor-tests`
2. `compile-tests`
3. `verify-tests-pass`
4. `commit`

## 《 SHARED 》implement-test-layer

**Inputs:**
- agent-action — the MID agent task to execute
- expected-test-result
- tests — filter (passed to `enable-tests`, `verify-tests-fail`, `disable-tests`)

**Outputs:**
- Agent output values (pass-through from `agent-action`)

**Steps:**
1. Call `agent-action` (the MID agent task named by the input)
2. `enable-tests` (tests: `<input>`)
3. `compile-tests`
4. Based on expected-test-result:
    1. If success: `verify-tests-pass` (filter: `<input tests>`)
    2. If failure:
        1. `verify-tests-fail` (filter: `<input tests>`)
        2. `disable-tests` (tests: `<input>`)
5. `commit`

## 《 SHARED 》verify-tests-pass

**Inputs:**
- filter (optional — passed to `run-tests`; runs all tests if absent)

**Outputs:** NONE

**Steps:**
1. `run-tests` (filter: `<input>`)
2. Success?
    1. YES: END
    2. NO: `fix-unexpected-failing-tests`

## 《 SHARED 》verify-tests-fail

**Inputs:**
- filter (optional — passed to `run-tests`; runs all tests if absent)

**Outputs:** NONE

**Steps:**
1. `run-tests` (filter: `<input>`)
2. Success?
    1. YES: `fix-unexpected-passing-tests`
    2. NO: END
