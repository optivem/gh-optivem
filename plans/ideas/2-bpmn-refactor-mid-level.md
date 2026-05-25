# BPMN - MID LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## write-acceptance-tests

**Inputs:**
- expected-test-result

**Scopes:**
- acceptance-tests
- dsl-ports

**Outputs:**
- dsl-port-changed: bool

**Steps:**
1. `execute-agent` write-acceptance-tests

## write-contract-tests

**Inputs:**
- expected-test-result

**Scopes:**
- contract-tests
- dsl-ports

**Outputs:**
- dsl-port-changed: bool

**Steps:**
1. `execute-agent` write-contract-tests

## implement-dsl

**Inputs:**
- expected-test-result

**Scopes:**
- dsl
- system-driver-ports
- external-driver-ports

**Outputs:**
- system-driver-ports-changed: bool
- external-driver-ports-changed: bool

**Steps:**
1. `execute-agent` implement-dsl

## implement-system

**Inputs:** NONE

**Scopes:**
- system

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-system

## implement-system-driver-adapters

**Inputs:** NONE

**Scopes:**
- system-driver-adapters

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-system-driver-adapters

## implement-external-system-driver-adapters

**Inputs:** NONE

**Scopes:**
- external-system-driver-adapters

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-external-system-driver-adapters

## implement-external-system-stubs

**Inputs:** NONE

**Scopes:**
- external-system-stubs

**Outputs:** NONE

**Steps:**
1. `execute-agent` implement-external-system-stubs

## disable-tests

**Inputs:**
- tests

**Scopes:**
- acceptance-tests
- contract-tests

**Outputs:** NONE

**Steps:**
1. `execute-agent` disable-tests

## enable-tests

**Inputs:**
- tests

**Scopes:**
- acceptance-tests
- contract-tests

**Outputs:** NONE

**Steps:**
1. `execute-agent` enable-tests

## fix-unexpected-passing-tests

**Inputs:** NONE

**Scopes:**
- acceptance-tests
- contract-tests
- dsl
- system

**Outputs:** NONE

**Steps:**
1. `execute-agent` fix-unexpected-passing-tests

## fix-unexpected-failing-tests

**Inputs:** NONE

**Scopes:**
- acceptance-tests
- contract-tests
- dsl
- system

**Outputs:** NONE

**Steps:**
1. `execute-agent` fix-unexpected-failing-tests

## refactor-tests

**Inputs:** NONE

**Scopes:**
- acceptance-tests
- contract-tests

**Outputs:** NONE

**Steps:**
1. `execute-agent` refactor-tests

## refactor-system

**Inputs:** NONE

**Scopes:**
- system

**Outputs:** NONE

**Steps:**
1. `execute-agent` refactor-system

## refine-acceptance-criteria

**Inputs:**
- ticket

**Scopes:**
- ticket

**Outputs:** NONE

**Steps:**
1. `execute-agent` refine-acceptance-criteria

## update-ticket

**Inputs:**
- ticket
- target-state

**Scopes:**
- ticket

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
