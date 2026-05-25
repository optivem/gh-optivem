# BPMN - TOP LEVEL

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

Single top-level process invoked once per ticket. Marks ticket state at the start and end of work and classifies which cycle to call based on the ticket's **change type** (Q26=A; Q30).

**Change-type → CYCLE mapping** (Q30 — see parent plan):

| Change type | CYCLE |
|---|---|
| Feature | `change-system-behavior` |
| Coverage gap | `cover-system-behavior` |
| Driver-adapter redesign | `redesign-system-structure` |
| System refactor | `refactor-system-structure` |
| Test refactor | `refactor-test-structure` |
| New external system | `onboard-external-system` |

## refine-ticket

INPUT: Ticket (any type) flagged as not-yet-ready.

1. Mark Ticket IN REFINEMENT
2. Call `refine-backlog` cycle
3. Mark Ticket READY

## implement-ticket

INPUT: Ticket in READY state (with metadata: ticket type, change type, acceptance criteria, etc.)

1. Mark Ticket IN PROGRESS
2. **Classify ticket into change type** (judgment step — Q30.a in parent plan decides whether this reads an explicit field or is inferred).
3. **Look up CYCLE for change type** (mechanical 1:1 — see table above).
4. Call chosen cycle.
5. Mark Ticket IN ACCEPTANCE

Note: cycle-selection sub-questions remain open and are resolved in parent plan (Item 6 cross-check walk): Q30.a (classification mechanism), Q30.b (multi-cycle tickets), plus preconditions to entering a cycle (e.g., "ACs must be in approved state" before `change-system-behavior`).
