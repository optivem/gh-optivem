# BPMN - HIGH LEVEL

===========================

## WRITE TESTS (BIG)

**INPUT: Acceptance Criteria** (scenario based Gherkin)

**INPUT: Expected Test Result**

**OUTPUT: Tests**

1. Write RED Acceptance Tests
2. DSL Port Changed?
    1. YES: Implement RED DSL Core
        1. External System Driver Ports Changed?
            1. YES: Implement RED External System Driver Adapters
        2. System Driver Ports Changed?
            1. YES: Implement RED System Driver Adapters <Expected Test Result>

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

## IMPLMEMENT RED SYSTEM DRIVER ADAPTERS

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

## WRITE SYSTEM (BIG)

1. Write System
2. Verify Tests Pass <Tests>
3. Commit

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