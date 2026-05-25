# BPMN - TOP LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

Three top-level processes. Two are ticket-driven (`refine-ticket`, `implement-ticket`); one is ad-hoc with no ticket (`refactor` — per Q34).

## refine-ticket

INPUT: Ticket (any type) flagged as not-yet-ready.

1. `update-ticket` (MID) — target state: `IN REFINEMENT`
2. Call `refine-backlog` cycle
3. `update-ticket` (MID) — target state: `READY`

## implement-ticket

INPUT: Ticket in READY state. Required metadata: ticket type (`story` / `bug` / `task`), and — for `task` only — a subtype label (e.g., `cover-legacy`, `refactor-system`, `redesign-system`, `refactor-tests`, `onboard-external-system`).

1. `update-ticket` (MID) — target state: `IN PROGRESS`
2. **Look up CYCLE** by ticket type + subtype (mechanical 1:1, no judgment). See mapping table below.
3. Call chosen cycle.
4. `update-ticket` (MID) — target state: `IN ACCEPTANCE`

**Ticket-type+subtype → CYCLE mapping** (Q30 revised — mechanical lookup, single-cycle per ticket per Q30.b):

| Ticket type / subtype | CYCLE |
|---|---|
| `story` | `change-system-behavior` |
| `bug` | `change-system-behavior` |
| `task/cover-legacy` | `cover-system-behavior` |
| `task/redesign-system` | `redesign-system-structure` |
| `task/refactor-system` | `refactor-system-structure` |
| `task/refactor-tests` | `refactor-test-structure` |
| `task/onboard-external-system` | `onboard-external-system` |

**Subtype validation (required for `task`):** unknown subtypes — or `task` with no subtype — **hard-exit at the gateway**. The operator must re-classify the ticket (or refine it via `refine-ticket` first) before `implement-ticket` will run.

**Unrecognized ticket types** (e.g., `spike`): also hard-exit. Whether `spike` deserves its own mapping or its own TOP process is captured in the parent plan's *Exploration backlog* — not in scope for this plan.

## refactor

Ad-hoc refactor entry point — no ticket required (Q34). Captures the "I want to refactor without ticket overhead" case.

INPUT: none (operator-initiated).

1. Choose refactor type (loopable — after the chosen CYCLE returns, ask again):
    - `refactor-system-structure`  → call CYCLE (opportunistic mode — no checklist supplied)
    - `refactor-test-structure`    → call CYCLE (opportunistic mode)
    - `redesign-system-structure`  → call CYCLE (opportunistic mode)
    - **none** → END

No `update-ticket` bookends — there's no ticket. Coexists with the ticket-driven path (`task/refactor-system` → `implement-ticket` gateway → same CYCLE) and with opportunistic-inside-`change-system-behavior` step 3. Three surfaces, three ceremony levels — operator picks.

**Doesn't apply to `change` / `cover` / `onboard` cycles** — each requires upfront ticket metadata (ACs / scope / target system) that an ad-hoc entry can't supply.
