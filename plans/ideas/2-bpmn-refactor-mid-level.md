# BPMN - MID LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## write-acceptance-tests

**Inputs:**
- acceptance-criteria
- expected-test-result

**Scopes:**
- at-test
- dsl-port
- dsl-core

**Outputs:**
- dsl-port-changed: bool

**Steps:**
1. `execute-agent` write-acceptance-tests

## write-contract-tests

**Inputs:**
- expected-test-result

**Scopes:**
- ct-test
- dsl-port
- dsl-core

**Outputs:**
- dsl-port-changed: bool

**Steps:**
1. `execute-agent` write-contract-tests

## implement-dsl

**Inputs:**
- expected-test-result

**Scopes:**
- dsl-core
- driver-port
- external-system-driver-port

**Outputs:**
- system-driver-ports-changed: bool
- external-driver-ports-changed: bool

**Steps:**
1. `execute-agent` implement-dsl

## implement-system

**Inputs:** NONE

**Scopes:**
- system-path

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-system

## implement-system-driver-adapters

**Inputs:** NONE

**Scopes:**
- driver-port
- driver-adapter

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-system-driver-adapters

## implement-external-system-driver-adapters

**Inputs:** NONE

**Scopes:**
- external-system-driver-port
- external-system-driver-adapter

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-external-system-driver-adapters

## implement-external-system-stubs

**Inputs:** NONE

**Scopes:**
- external-system-driver-adapter

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-external-system-stubs

## disable-tests

**Inputs:**
- tests

**Scopes:**
- at-test
- ct-test

**Outputs:** NONE

**Steps:**
1. `execute-agent` disable-tests

## enable-tests

**Inputs:**
- tests

**Scopes:**
- at-test
- ct-test

**Outputs:** NONE

**Steps:**
1. `execute-agent` enable-tests

## fix-unexpected-passing-tests

**Inputs:** NONE

**Scopes:**
- at-test
- ct-test
- dsl-port
- dsl-core
- driver-port
- driver-adapter
- external-system-driver-port
- external-system-driver-adapter
- system-path

**Outputs:** NONE

**Steps:**
1. `execute-agent` fix-unexpected-passing-tests

## fix-unexpected-failing-tests

**Inputs:** NONE

**Scopes:**
- at-test
- ct-test
- dsl-port
- dsl-core
- driver-port
- driver-adapter
- external-system-driver-port
- external-system-driver-adapter
- system-path

**Outputs:** NONE

**Steps:**
1. `execute-agent` fix-unexpected-failing-tests

## refactor-tests

**Inputs:** NONE

**Scopes:**
- at-test
- ct-test
- dsl-port
- dsl-core
- driver-port
- driver-adapter
- external-system-driver-port
- external-system-driver-adapter

**Outputs:** NONE

**Steps:**
1. `execute-agent` refactor-tests

## refactor-system

**Inputs:** NONE

**Scopes:**
- system-path

**Outputs:** NONE

**Steps:**
1. `execute-agent` refactor-system

## refine-acceptance-criteria

**Inputs:**
- ticket

**Scopes:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-agent` refine-acceptance-criteria

## update-ticket

**Inputs:**
- ticket
- target-state

**Scopes:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-agent` update-ticket

---

## compile

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem compile

## compile-system

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem compile-system

## compile-tests

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem compile-tests

## build-system

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem build-system

## start-system

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem start-system

## commit

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem commit

## run-tests

**Inputs:**
- filter — accepts: (1) test-type tag (`acceptance` / `contract` / `acceptance-api` / `acceptance-ui` / `contract-stub` / `contract-real`); (2) list of specific test names; (3) no filter (runs all tests)

**Outputs:** NONE

**Steps:**
1. `execute-command` gh optivem run-tests [filter]
