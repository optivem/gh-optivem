# BPMN - HIGH LEVEL

===========================

## WRITE TESTS

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**INPUT: Expected Test Result**

**OUTPUT: Tests**

**Port-change wiring** (Q6):

| Producer (mid-level task) | Output variable | Consumer (branch below) |
|---|---|---|
| Write Acceptance Tests | `dsl-port-changed: bool` | Step 2 — "DSL Port Changed?" |
| Implement DSL | `external-driver-ports-changed: bool` | Step 2.1.1 — "External System Driver Ports Changed?" |
| Implement DSL | `system-driver-ports-changed: bool` | Step 2.1.2 — "System Driver Ports Changed?" |

Note: the External/System driver port-change checks are nested *under* "YES: Implement RED DSL Core" because those port-change outputs only exist after DSL is implemented.

Note: `<Expected Test Result>` (from INPUT above) is threaded to all nested `Implement RED *` calls — no need to repeat per leaf (Q6.a).

1. Write RED Acceptance Tests
2. DSL Port Changed? (reads `dsl-port-changed` from step 1)
    1. YES: Implement RED DSL Core
        1. External System Driver Ports Changed? (reads `external-driver-ports-changed` from Implement DSL)
            1. YES: Implement RED External System Driver Adapters
        2. System Driver Ports Changed? (reads `system-driver-ports-changed` from Implement DSL)
            1. YES: Implement RED System Driver Adapters

## WRITE RED ACCEPTANCE TESTS

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**OUTPUT: Tests**

1. Write Acceptance Tests (AGENT)
2. Compile Tests
3. Verify Tests Fail
4. Disable Tests (AGENT)
5. Commit

## IMPLEMENT RED DSL CORE

1. Implement Test Layer
    1. Agent Action: Implement DSL Core

## IMPLEMENT RED SYSTEM DRIVER ADAPTERS

1. Implement Test Layer
    1. Agent Action: Implement System Driver Adapters

## IMPLEMENT RED EXTERNAL SYSTEM DRIVER ADAPTERS - CONTRACT TESTS

1. Write RED Contract Test
    1. Note: supposed to think about the External System Driver Ports
    2. Output: list of tests
2. DSL Port Changed?
    1. Implement RED DSL
        1. Note: supposed to use the External System Driver Ports
3. Implement External System Driver Adapters
4. Verify Tests Pass <Contract Tests - Real>
5. Verify Tests Fail <Contract Tests - Stub>
6. Implement External System Stubs
7. Verify Tests Pass <Contract Tests - Stub>

===========================

## IMPLEMENT SYSTEM

1. Implement System
2. Build System
3. Start System
4. Verify Tests Pass <Tests>
5. Commit

===========================

## REFACTOR TESTS

1. Refactor Tests
2. Compile Tests
3. Verify Tests Pass
4. Commit

===========================

## 《 SHARED 》IMPLEMENT TEST LAYER

1. Execute <Agent Action>
2. Enable Tests <Tests>
3. Compile Tests
4. Based on result we expect: <Expected Test Result>
    1. If expect success:
        1. Verify Tests Pass
    2. If expect failure:
        1. Verify Tests Fail <Tests>
        2. Disable Tests <Tests>
5. Commit

## 《 SHARED 》VERIFY TESTS PASS

1. Run Tests
2. Success?
    1. YES: END
    2. NO: Fix Unexpected Failing Tests

## 《 SHARED 》VERIFY TESTS FAIL

1. Run Tests
2. Success?
    1. YES: Fix Unexpected Passing Tests
    2. NO: END


===========================

Run Tests filter: see Q5/MID brainstorm — single `Run Tests` task with polymorphic filter accepting a test-type tag, a list of test names, or none.
