# BPMN - CYCLE LEVEL

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

Per-ticket sub-processes selected by the classification gateway in `implement-ticket` (TOP). Acceptance Criteria and Checklists are NOT ticked at this level (Q7=A).

## refine-backlog

1. Read Backlog Items
2. Identify Gaps / Ambiguities
3. Refine Ticket Descriptions
4. Refine Acceptance Criteria

## onboard-external-system

1. Identify External System
2. Document External System Contract
3. Set Up External System Access (credentials, endpoints, sandbox)
4. Verify External System Reachable

## change-system-behavior

1. Write Tests <Expected Test Result: Success>
2. Implement System

## cover-system-behavior

1. Write Tests <Expected Test Result: Success>

## redesign-system-structure

1. Implement Driver Adapters (includes System Driver Adapters & External System Driver Adapters)
2. Implement System

Note: MAY also call `onboard-external-system` as a sub-process when the redesign involves onboarding a new external system.

## refactor-system-structure

1. Implement System

Note: "Implement System" calls the high-level `implement-system` orchestration, which includes compile + verify, so `refactor-system-structure` inherits compile/verify from that orchestration (Q8).

## refactor-test-structure

1. Refactor Tests

Note: "Refactor Tests" calls the high-level `refactor-tests` orchestration (Refactor Tests → Compile Tests → Verify Tests Pass → Commit), per Q8.
