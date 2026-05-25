# BPMN - CYCLE LEVEL

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

> **Cross-file connectedness (Q-new-1/2/3, resolved 2026-05-25).** Every step below is an exact kebab reference to a HIGH orchestration or MID agent task defined elsewhere — not prose. HIGH names drop "red" (parameterized via `<Expected Test Result>`); MID uses `-driver-adapters` (was `-drivers`).

Per-ticket sub-processes selected by the classification gateway in `implement-ticket` (TOP). Acceptance Criteria and Checklists are NOT ticked at this level (Q7=A).

## refine-backlog

1. Read Backlog Items
2. Identify Gaps / Ambiguities
3. Refine Ticket Descriptions
4. `refine-acceptance-criteria` (MID)

## onboard-external-system

1. Identify External System
2. Document External System Contract
3. Set Up External System Access (credentials, endpoints, sandbox)
4. Verify External System Reachable

## change-system-behavior

1. `write-and-verify-tests` (HIGH) `<Expected Test Result: Failure>`
2. `implement-and-verify-system` (HIGH)

## cover-system-behavior

1. `write-and-verify-tests` (HIGH) `<Expected Test Result: Success>`

Note: COVER uses the same HIGH orchestration as CHANGE, parameterized by `<Expected Test Result>` (Q-new-1=A). Legacy-coverage authoring (Q16=B) is the success-branch of this HIGH; no separate "legacy" surface.

## redesign-system-structure

1. Implement Driver Adapters (Q-new-2=A: two MID-direct calls — no HIGH wrapper):
    1. `implement-system-driver-adapters` (MID)
    2. `implement-external-system-driver-adapters` (MID)
2. `implement-and-verify-system` (HIGH)

Note: MAY also call `onboard-external-system` as a sub-process when the redesign involves onboarding a new external system.

## refactor-system-structure

1. `implement-and-verify-system` (HIGH)

Note: `implement-and-verify-system` (HIGH) includes compile + verify, so `refactor-system-structure` inherits that discipline (Q8). The actual refactor work runs as MID `refactor-system` inside HIGH `implement-system` — see Q28.c.

## refactor-test-structure

1. `refactor-and-verify-tests` (HIGH)

Note: `refactor-and-verify-tests` (HIGH) runs the sequence `refactor-tests` → `compile-tests` → `verify-tests-pass` → `commit`, per Q8.
