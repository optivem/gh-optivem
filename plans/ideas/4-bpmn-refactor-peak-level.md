# BPMN - PEAK LEVEL

Acceptance Criteria and Checklists are NOT ticked at this level (Q7=A).

## (WRAPPER) TICKET LIFECYCLE

Common entry/exit wrapper around every peak entry below. Marks ticket state at the start and end of work on a ticket.

1. Mark Ticket IN PROGRESS
2. <call the chosen peak entry>
3. Mark Ticket IN ACCEPTANCE

## REFINE BACKLOG

1. Read Backlog Items
2. Identify Gaps / Ambiguities
3. Refine Ticket Descriptions
4. Refine Acceptance Criteria

## ONBOARD EXTERNAL SYSTEM

1. Identify External System
2. Document External System Contract
3. Set Up External System Access (credentials, endpoints, sandbox)
4. Verify External System Reachable

## CHANGE SYSTEM BEHAVIOR (ATDD)

1. Write Tests <Expected Test Result: Success>
2. Implement System

## COVER SYSTEM BEHAVIOR

1. Write Tests <Expected Test Result: Success>

## REDESIGN SYSTEM STRUCTURE

1. Implement Driver Adapters (includes System Driver Adapters & External System Driver Adapters)
2. Implement System

Note: MAY also call `ONBOARD EXTERNAL SYSTEM` as a sub-process when the redesign involves onboarding a new external system.

## REFACTOR SYSTEM STRUCTURE

1. Implement System

Note: "Implement System" calls the high-level `IMPLEMENT SYSTEM` orchestration, which includes compile + verify, so REFACTOR SYSTEM STRUCTURE inherits compile/verify from that orchestration (Q8).

## REFACTOR TEST STRUCTURE

1. Refactor Tests

Note: "Refactor Tests" calls the high-level `REFACTOR TESTS` orchestration (Refactor Tests → Compile Tests → Verify Tests Pass → Commit), per Q8.
