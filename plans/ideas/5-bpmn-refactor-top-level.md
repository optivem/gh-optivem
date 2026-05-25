# BPMN - TOP LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## refine-ticket

**Inputs:**
- ticket — any type; flagged as not-yet-ready.

**Outputs:**
- ticket-state: `READY`

**Steps:**
1. `update-ticket` (ticket: `<input>`, target-state: `IN REFINEMENT`)
2. `refine-backlog` (ticket: `<input>`)
3. `update-ticket` (ticket: `<input>`, target-state: `READY`)

## implement-ticket

**Inputs:**
- ticket — `READY` state. Required metadata: ticket type (`story` / `bug` / `task`), and — for `task` only — a subtype label (e.g., `cover-legacy`, `refactor-system`, `redesign-system`, `refactor-tests`, `onboard-external-system`).

**Outputs:**
- ticket-state: `IN ACCEPTANCE`

**Steps:**
1. `update-ticket` (ticket: `<input>`, target-state: `IN PROGRESS`)
2. Look up CYCLE by ticket type + subtype (mechanical 1:1 lookup):

    | Ticket type / subtype | CYCLE |
    |---|---|
    | `story` | `change-system-behavior` |
    | `bug` | `change-system-behavior` |
    | `task/cover-legacy` | `cover-system-behavior` |
    | `task/redesign-system` | `redesign-system-structure` |
    | `task/refactor-system` | `refactor-system-structure` |
    | `task/refactor-tests` | `refactor-test-structure` |
    | `task/onboard-external-system` | `onboard-external-system` |

3. Call chosen cycle with inputs extracted from the ticket:
    - `change-system-behavior` / `cover-system-behavior` → `acceptance-criteria` from ticket
    - `redesign-system-structure` / `refactor-system-structure` / `refactor-test-structure` → checklist from ticket (optional)
    - `onboard-external-system` → `external-system-description` from ticket
4. `update-ticket` (ticket: `<input>`, target-state: `IN ACCEPTANCE`)

**Subtype validation (required for `task`):** unknown subtypes — or `task` with no subtype — hard-exit at the gateway. The operator must re-classify the ticket (or refine it via `refine-ticket` first) before `implement-ticket` will run.

**Unrecognized ticket types** (e.g., `spike`): also hard-exit.

## refactor

Ad-hoc refactor entry point — no ticket required.

**Inputs:** NONE

**Outputs:** NONE

**Steps:**
1. Choose refactor type (loopable — after the chosen CYCLE returns, ask again):
    - `refactor-system-structure` → opportunistic mode (no checklist)
    - `refactor-test-structure` → opportunistic mode (no checklist)
    - `redesign-system-structure` → opportunistic mode (no checklist)
    - **none** → END
