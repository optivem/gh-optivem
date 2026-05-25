# BPMN ideas contract authoring

> **Sequenced before `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`.** Authors per-task `Inputs:` / `Outputs:` / `Steps:` contracts directly in the five `plans/ideas/*.md` brainstorm files so Phase C+D becomes a mechanical YAML-encoding pass with no on-the-fly contract invention. Cross-references the design plan `plans/20260525-1057-bpmn-refactor-design.md` for *why* behind each decision; this plan only authors *what*.

> **Absorbs Item 12 of the design plan** (Q-tag strip). Item 6 of this plan deletes Item 12 from the design plan upon completion.

## Why now (rework analysis)

The design plan's Q13.a authored an illustrative contract block for `write-acceptance-tests` only, with the assumption that the YAML would be authored per-task during Phase C. That leaves every other task's `scopes:` / `outputs:` undefined.

The ideas files are a **cheap design surface** — markdown, no schema, no diagram regen, no downstream tooling. Iterating contracts here costs one file edit. Iterating contracts in YAML costs schema validation + diagram regen + potential statemachine-test fallout. Even though the ideas files get deleted post-Phase-D, front-loading the design work into the cheap surface minimizes total rework. UI-wireframe analogy: wireframes get thrown away once the UI ships, but doing them *before* writing code minimizes code rework.

## Doctrine

### Uniform task template

Every task in the brainstorm files — LOW primitives, MID tasks, HIGH orchestrations, CYCLE per-ticket flows, TOP processes — uses this template:

```
## <task-name>

**Inputs:**
- ...
- NONE  (if no inputs)

**Outputs:**
- ...
- NONE  (if no outputs)

**Steps:**
1. ...
```

Rules:
- All three sections always shown, even if empty (write `- NONE`).
- `Outputs:` lists operator-visible outputs only — the task's contract with its caller.
- Intra-flow plumbing (e.g., `dsl-port-changed` consumed by a sibling step) is annotated inline on the producer/consumer steps (`(reads dsl-port-changed from step 1)`), not surfaced at the task level.

### Editorial rules

Strip from all five files:
1. **Decision-rationale parentheticals** — `(per Q13=A)`, `(Q-new-3)`, `(per Q28.a)`, `(resolved 2026-05-25)`, etc.
2. **Historical refs** — `(was implement-system-drivers — renamed per Q-new-3)`, `(was at-red-test)`, etc.
3. **Reverse cross-references** — `(called by HIGH implement-and-verify-system step 2)`, `Called by cover-system-behavior.`, etc. IDs are searchable; reverse lookups don't belong inline.
4. **Top-of-file doctrine banners** — Q29 naming-convention banner, Q-new-1 doctrine resolved banner, Cross-file connectedness banner. Same category as #1.
5. **Standalone `Note (Q...):` paragraphs** — same category as #1.
6. **`TODO (Phase C revisit): ... (Q...)` lines** — same category as #1.

Keep:
- The "Design content only" admonition banner at the top of each file (editor's instruction, not rationale).

### Ambiguity handling

If a task's `Inputs:` or `Outputs:` aren't determined by the design plan's Decisions section or by an obvious reading of the brainstorm intent:
- **Surface as a new Q-item in the design plan** (`Q-new-N`), with a recommendation.
- **Defer the task block until the Q resolves** rather than guessing.
- Resolving Q-items is part of this plan's scope — don't push them downstream to Phase C.

## Items

Per-file items run in dependency order (LOW → MID → HIGH → CYCLE → TOP); cross-link check at the end. Each item is one `/execute-plan` invocation.

1. - [ ] **1-low — editorial + author contracts.** Tasks: `approve`, `execute-agent`, `execute-command`, `fix`. Notes: `execute-agent` Outputs = "Agent output values (as declared by caller's Output input)"; the others' Outputs = `NONE`. Strip the `TODO (Phase C revisit)` line on `approve` and the `Intentional asymmetry vs execute-agent (Q2=C)` comment on `execute-command`. Commit.

2. - [ ] **2-mid — editorial + author contracts.** Every agent task and every command task gets its own Inputs/Outputs/Steps block (no "show one, name the rest" pattern). Conventions:
    - Agent task Steps: `1. execute-agent <task-name>`.
    - Agent task Inputs: lists the task's `scopes` (permitted file scope).
    - Agent task Outputs: lists declared output variables (e.g., `dsl-port-changed: bool`).
    - Command task Steps: `1. execute-command <command> <params>`.
    - Command task Inputs: command params.
    - Command task Outputs: `NONE` (success is exit-code based).

    Use the design plan's Q6 table for the known port-change outputs (`write-acceptance-tests` → `dsl-port-changed`; `implement-dsl` → `system-driver-ports-changed` + `external-driver-ports-changed`). For tasks where scopes/outputs aren't already decided: propose contract → confirm with user → write. If ambiguous beyond propose-then-confirm, surface as new Q-item in design plan and defer per Doctrine § Ambiguity handling. Commit.

3. - [ ] **3-high — editorial + author contracts.** Most tasks already have `INPUT:` / `OUTPUT:` lines — convert to the template. Annotate intra-flow plumbing inline. Tasks: `write-and-verify-tests-fail`, `write-and-verify-tests-pass`, `write-and-verify-tests`, `write-and-verify-acceptance-tests`, `implement-and-verify-dsl`, `implement-and-verify-system-driver-adapters`, `implement-and-verify-external-system-driver-adapter-contract-tests`, `implement-and-verify-system`, `refactor-and-verify-tests`, plus three SHARED tasks (`implement-test-layer`, `verify-tests-pass`, `verify-tests-fail`). Commit.

4. - [ ] **4-cycle — editorial + author contracts.** Each CYCLE's Inputs = ticket / ACs / nothing; Outputs = operator-visible artifact ("Modified system + tests" or similar). Cycles: `refine-backlog`, `onboard-external-system`, `change-system-behavior`, `cover-system-behavior`, `redesign-system-structure`, `refactor-system-structure`, `refactor-test-structure`. Commit.

5. - [ ] **5-top — editorial + author contracts.** Processes: `refine-ticket`, `implement-ticket`, `refactor`. Inputs = Ticket (with required metadata listed inline) or `NONE` for `refactor`. Outputs operator-visible (e.g., ticket state transition for the ticket-driven processes; `NONE` for `refactor`). Commit.

6. - [ ] **Cross-link check + design-plan handoff.** (a) Walk every `<reference-to-other-task>` across the five files; confirm the referenced task exists and its declared Outputs satisfy the referencing step's needs (catches contract drift between Items 1–5). (b) Delete Item 12 from `plans/20260525-1057-bpmn-refactor-design.md` (its scope absorbed here); add a one-line breadcrumb in the design plan's Phases overview noting the move. (c) Update the cross-ref in `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` to note this plan as a prerequisite. Commit.

## Re-running `/execute-plan`

After Item 6 commits, this plan is done. Continue Phase C+D on `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` — Phase C is now a mechanical YAML encoding pass.

## Standing constraints

(Same as parent design plan — see `plans/20260525-1057-bpmn-refactor-design.md` § Standing constraints. Most relevant here: token-efficient, surgical commits via raw `git`, auto-commit at end-of-item, surface `/clear` + `/execute-plan` between items.)
