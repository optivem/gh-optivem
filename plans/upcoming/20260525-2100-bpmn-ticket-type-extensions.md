# BPMN — ticket-type extensions (exploration backlog)

**Date:** 2026-05-25
**Trigger:** do **not** execute on a schedule. Pick up only when a real `spike` workflow emerges, or when a real multi-cycle ticket need arises in practice. Until then, this file is a future-work marker, not an action item.

## Origin

Carried forward from `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` (since deleted) — the "Exploration backlog" section listed two open questions that the five-level BPMN refactor deliberately did **not** answer. They survived the refactor as deferred topics; this file is where they live now.

## Open questions

### 1. `spike` ticket type

The TOP `implement-ticket` gateway table currently has no row for `spike`. Possible shapes if/when needed:

- Add a new CYCLE `investigate-spike` (or similar) and wire `spike` → that CYCLE.
- Promote `spike` to its own TOP process — different bookends (no Mark IN ACCEPTANCE; spike output is a findings artifact, not shipped code).
- Treat `spike` as a flavor of `refactor` (the existing no-ticket-overhead TOP) and skip the gateway entirely.

**Decision deferred until a real `spike` workflow exists.** Don't pre-design.

### 2. Multi-cycle ticket model

The five-level refactor settled on **single-cycle tickets only** (Q30.b = A in `plans/20260525-1057-bpmn-refactor-design.md`): multi-cycle work must split during refinement into separate tickets. Revisit only if a real workflow demands a single ticket spanning multiple CYCLEs that can't reasonably be split.

If it ever needs revisiting, options to consider:

- Sequenced CYCLE list at the TOP level (`implement-ticket` runs CYCLE-A, then CYCLE-B, …).
- Parent ticket / child ticket model where the parent's gateway dispatches to multiple child tickets each running one CYCLE.

**Decision deferred.** Current answer (split during refinement) holds.

## Out of scope

- Pre-designing either workflow without a real driving need.
- Editing the existing five-level structure to "make room" for these — both extensions are additive at the TOP gateway when their time comes.
