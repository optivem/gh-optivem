# BPMN - TOP LEVEL

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

Single top-level process invoked once per ticket. Marks ticket state at the start and end of work and classifies which cycle to call based on ticket type (Q26=A).

## refine-ticket

INPUT: Ticket (any type) flagged as not-yet-ready.

1. Mark Ticket IN REFINEMENT
2. Call `refine-backlog` cycle
3. Mark Ticket READY

## implement-ticket

INPUT: Ticket in READY state (with metadata: type, acceptance criteria, etc.)

1. Mark Ticket IN PROGRESS
2. Decide Cycle based on ticket type:
    1. Feature → `change-system-behavior`
    2. Coverage gap → `cover-system-behavior`
    3. Driver-adapter ticket → `redesign-system-structure`
    4. System refactor → `refactor-system-structure`
    5. Test refactor → `refactor-test-structure`
    6. New external system → `onboard-external-system`
3. Call chosen cycle
4. Mark Ticket IN ACCEPTANCE

Note: cycle-selection sub-questions remain open and are resolved during the parent plan's Item 6 cross-check walk:
- Is cycle-selection automatic (gateway on ticket field) or manual (human picks)?
- Are there preconditions to entering a cycle (e.g., "ACs must be in approved state" before `change-system-behavior`)?
