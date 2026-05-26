# Archive BPMN refactor design plan + establish `plans/archived/` convention

## Goal

Move `plans/20260525-1057-bpmn-refactor-design.md` (the self-described "pure design archive") out of the active `plans/` root into a new `plans/archived/` subdirectory, and establish `plans/archived/` as a documented convention. Strip both stale design-archive citations from `internal/atdd/runtime/statemachine/process-flow.yaml` so the yaml file stops citing plans entirely. Repoint every other cross-reference to the new path so no link goes stale.

## Why now

- The design plan self-describes as a "pure design archive" — execution moved to a separate plan that has since been retired (commit `f1f1f7b`, `20260525-1517-bpmn-refactor-yaml-and-diagrams.md`).
- The yaml header at `process-flow.yaml:5-6` already cites a non-existent execution plan (`1517-yaml-and-diagrams`), proving that yaml-citing-plans is a maintenance trap: a plan can be retired without the yaml header being updated.
- Sibling completed plans (`20260526-0832-...`, `20260526-1310-...`) sit in `plans/` root with no convention distinguishing "in-flight" from "frozen reference". This plan defines that convention.

## Out of scope

- **Sweeping other completed plans into `archived/`.** Scope is one file (the design plan) plus the convention text. Future archiving is case-by-case, decided when the next plan author judges a file frozen.
- **Editing the design plan's body content.** Only internal self-references that name its own path get mechanically updated. Q&A debate, Decisions, cross-check inventory — untouched.
- **Reviving the retired `1517-yaml-and-diagrams` reference.** Line 6 of the yaml header cites a plan that no longer exists; that citation goes away with line 5's removal.

## Convention: what `plans/archived/` is

(This section is the policy text — to be referenced from `CLAUDE.md` later if/when a second plan ever lands in `archived/`. For now it lives here, in the plan that establishes the directory.)

**`plans/archived/` holds plans that are frozen reference material — kept on disk because they record *why* something was decided, but no longer drive execution.**

A plan qualifies for `plans/archived/` when **all** of the following are true:

1. The plan's execution items are either done, retired, or split into separate (already-landed) follow-up plans.
2. The plan's contents are useful as reference (e.g., design Q&A, cross-check inventory, decision rationale) — not just procedural checklists whose value evaporated with execution.
3. No in-flight plan is still actively editing the file or relying on its scope being mutable.

If a plan no longer meets criterion (2) — purely a procedural checklist with no reference value — **delete it instead of archiving**.

**Editing policy for archived plans:** archived plans are frozen. Permissible edits are limited to:

- Mechanical path-repointing when *another* file moves and breaks an internal cross-reference inside the archived plan.
- Frontmatter / one-line markers (e.g., "archived YYYY-MM-DD") added at archive time.

Body content (Q&A, decisions, prose) is not edited after archiving. If reality diverges from the archived decision, write a new plan that supersedes it — do not retro-edit history.

**Cross-referencing archived plans:** other plan files MAY cite `plans/archived/...` paths when load-bearing context lives there (e.g., upcoming plans citing settled design decisions). **Code / runtime YAML files MUST NOT cite plan paths**, archived or otherwise — code stands on its own; the plan record is for humans reading the plan tree, not for the binary to consume.
