# BPMN refactor — encode YAML + render diagrams

Phase C + Phase D of the BPMN five-level refactor (TOP / CYCLE / HIGH / MID / LOW). Encodes the new structure into `internal/atdd/runtime/statemachine/process-flow.yaml`, regenerates `docs/process-diagram.md` via `gh optivem process show`, and writes the downstream-alignment plan as a Phase D handoff.

> **Working style: token-efficient.** Execute in the cheapest form that still produces a quality result. If a workflow burns tokens unnecessarily, surface the cheaper alternative and let the user choose. (Memory: `feedback_flag_non_token_efficient`.)

## Inputs (read these first)

This plan does NOT re-host the design rationale — it executes a design that has already been settled.

- **Design archive:** `plans/20260525-1057-bpmn-refactor-design.md` — full Q&A history (Q1–Q34 + Q-new-1/2/3 + Q-ext), cross-check inventory of the 21 existing diagrams, prompt-rename table. Use this when you need the *why* behind any decision below.
- **Brainstorm sources (authoritative inputs):** `plans/ideas/1-5-bpmn-refactor-*-level.md` — LOW / MID / HIGH / CYCLE / TOP. These are the design surface this plan encodes into YAML. They must be Q-tag-stripped first (parent plan's Item 12) — verify before starting Item 1.

## Scope

- Full replacement of the existing BPMN (Q17 = A). All 21 existing process diagrams either **absorbed** into the new five-level structure or **dropped** with explicit rationale (cross-check inventory in the archive).
- The existing `gh optivem process show` rendering pipeline stays (Q23 = A). No new renderer, no per-level Mermaid split (Q14 = A).
- `docs/process-diagram.md` regenerated from the new YAML at each item that touches `process-flow.yaml`. **No hand-drawn Mermaid.**
- Phase D writes a separate downstream-alignment plan (Q21 = A); this plan does NOT execute the downstream work.

## Decisions ledger (compact)

Single-line summaries. For options considered + rationale, read the archive plan's `## Decisions` section.

### Structure (Q17, Q18, Q26)
- Five-level: **TOP / CYCLE / HIGH / MID / LOW.** Brainstorms exhaustive + final.

### Naming (Q15, Q27, Q28.a, Q29)
- Kebab-case lowercase **everywhere** — YAML keys, doc headings, prompt filenames, in-prose refs, anchor slugs, Go struct tags.
- Verb-based identifiers; "Write" for tests, "Implement" for code.
- HIGH composites use full-scope names: `implement-and-verify-system`, `refactor-and-verify-tests`, `write-and-verify-tests`.
- **`agent-name:` field DROPPED from `process-flow.yaml`.** Runtime derives `prompt_path(task_name) = task_name + ".md"`; errors at startup if prompt file missing.

### LOW primitives
- 4 primitives: `approve`, `execute-agent`, `execute-command`, **`fix`** (new — single attempt, PRE-approval only, no own validation; caller re-validates).
- `approve` NO-branch: exit-only (caller owns NO). Add `TODO` comment in YAML for possible revisit (parameterized NO-action OR split into two primitives).
- `execute-command` is PRE-only (asymmetric with agent's PRE+POST — intentional, machine-checkable success).
- Terminology: `calls` (BPMN call-activity).

### MID tasks
- `run-tests` — single task with polymorphic filter (test-type tag | test-name list | no filter). Encoding shape (single-string-with-prefix vs structured discriminated union) decided during YAML encoding.
- `write-contract-tests` added (symmetric with `write-acceptance-tests`).
- Vocabulary unified: `-driver-adapters` (not `-drivers`).
- Tasks added by Item 11's connectedness pass (already in brainstorms): `implement-system`, `implement-external-system-stubs`, `refactor-tests`, `refactor-system`, `refine-acceptance-criteria`, `update-ticket`, `build-system`, `start-system`.

### HIGH orchestrations (Q31 = D)
- Parameterized core `write-and-verify-tests` (`<Expected Test Result>` threaded through step 1 + `implement-test-layer`).
- Two thin wrappers: `write-and-verify-tests-fail` (Failure) + `write-and-verify-tests-pass` (Success).
- CYCLEs call **wrappers**, not the core — call sites are parameter-free and self-documenting.
- No `(BIG)` suffix; no `red`/`green` in HIGH names — parameterized via `implement-test-layer` (Q-new-1).
- `refactor-and-verify-tests` is a distinct HIGH (refactor-tests → compile-tests → verify-tests-pass → commit). `refactor-system-structure` reuses `implement-and-verify-system`.

### CYCLE composition
Seven cycles:

- `change-system-behavior` — step 3 is loopable opportunistic refactor menu (calls existing CYCLEs in *no-checklist / opportunistic mode*; Q33).
- `cover-system-behavior` — legacy-coverage cycle, expected=Success (Q16 = B, parameterized expectation instead of separate legacy cycles).
- `redesign-system-structure` — step 1 splits into `1a implement-system-driver-adapters` + `1b implement-external-system-driver-adapters` (MID-direct, no umbrella; Q-new-2).
- `refactor-system-structure` — step 1 calls HIGH `implement-and-verify-system`.
- `refactor-test-structure` — step 1 calls HIGH `refactor-and-verify-tests`.
- `refine-backlog` — standalone sibling (Q9 = A).
- `onboard-external-system` — standalone (Q-ext = b); `redesign-system-structure` may call it as a sub-process.

### TOP processes
Three:

- `refine-ticket` — backlog grooming; steps include `update-ticket` calls for state transitions (IN REFINEMENT / READY).
- `implement-ticket` — Mark IN PROGRESS → mechanical-lookup gateway by ticket-type + optional `task` subtype → call chosen CYCLE → Mark IN ACCEPTANCE. Unknown subtypes **hard-exit at gateway**. Single-cycle tickets only (multi-cycle work splits during refinement; Q30.b = A).
- `refactor` — ad-hoc, no ticket overhead (Q34). Loopable refactor-type chooser; no Mark-Ticket bookends.

### Gateway lookup table (TOP `implement-ticket`)
Already lives in `plans/ideas/5-bpmn-refactor-top-level.md`. Encode as the YAML gateway:

| Ticket type / subtype | CYCLE |
|---|---|
| `story` | `change-system-behavior` |
| `bug` | `change-system-behavior` |
| `task/cover-legacy` | `cover-system-behavior` |
| `task/redesign-system` | `redesign-system-structure` |
| `task/refactor-system` | `refactor-system-structure` |
| `task/refactor-tests` | `refactor-test-structure` |
| `task/onboard-external-system` | `onboard-external-system` |

### Contract blocks (Q13 = A)
- Live in `process-flow.yaml` as `user_task` metadata: `scopes:` + `outputs:`.
- Both consumers read the same YAML — (1) agent invocation uses them for prompt context + permitted file scope; (2) post-execute BPMN verify reads `outputs:` to validate "required output variables present?" and `scopes:` to validate "scope constraints satisfied? (diff)".
- Schema/generator extension (Item 2) likely needed to add these fields.

### Cross-check resolution highlights
- Diagram #13 (`at-refactor-system`) → `change-system-behavior` step 3 opportunistic loop (Q32). `at-refactor-system.md` prompt: DROP.
- Diagram #4 (`run-legacy-cycle`) → DROP (no separate legacy test-run operation; `run-tests` runs whatever's in the suite).
- Diagrams #12, #18, #19 → CYCLE `cover-system-behavior` (Q16 = B parameterized expectation).
- All 15 standard absorption rows walked and confirmed (Item 6, parent plan).
- Full inventory + rationale per row: archive plan's "Cross-check vs existing BPMN" section.

### Deferred to Item 9 (during YAML encoding)
- **Q5.a — `run-tests` filter encoding shape.** Single-string-with-prefix vs structured discriminated union. Pick during encoding.
- **Q31.a — `cover-system-behavior` AT vs CT internal handling.** Three options (single CYCLE with nesting | invoked twice per ticket | two subtypes `task/cover-legacy-acceptance` + `task/cover-legacy-contract`). Pick during encoding.

### Exploration backlog (not blocking)
- `spike` ticket type — currently no entry in the gateway table. Future: cycle? Or own TOP process?
- Multi-cycle ticket model — currently rejected (Q30.b = A); revisit if real workflow demands it.

---

## Items

Each item is sized for one `/execute-plan` invocation. Re-running `/execute-plan plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` picks up the next unchecked item. Resolved items are deleted, not checked (per `/execute-plan` rule).

### Prerequisite

Verify **`plans/20260525-1531-bpmn-ideas-contract-authoring.md`** has fully landed before starting Item 1 — the brainstorms in `plans/ideas/*.md` are this plan's authoritative input, and that prerequisite plan authors the per-task `Inputs:` / `Scopes:` / `Outputs:` / `Steps:` contracts (absorbing the parent design plan's Item 12 Q-tag strip in the process). Without it, Phase C is not a mechanical YAML-encoding pass — every task's contract would have to be invented on the fly. (Parent design archive for *why* behind each decision: `plans/20260525-1057-bpmn-refactor-design.md`.)

1. - [ ] **Item 1 — Phase C.1: Prototype `refactor-system-structure` in YAML.** Encode the simplest cycle (`refactor-system-structure`) in `internal/atdd/runtime/statemachine/process-flow.yaml`. Save the current `docs/process-diagram.md` as a backup first (`cp docs/process-diagram.md docs/process-diagram.md.pre-refactor`). Run `gh optivem process show > docs/process-diagram.md`. Inspect the regenerated output for the new cycle. Compare against the refined `plans/ideas/4-bpmn-refactor-cycle-level.md`. Commit (YAML + regenerated md).
    **Done when:** regenerated diagram for `refactor-system-structure` matches the refined brainstorm; backup file in place for Item 3's diff.

2. - [ ] **Item 2 — Phase C.2: Schema/generator changes if needed.** Based on Item 1 findings, extend the YAML schema and generator if needed. Most likely candidate: adding `scopes:` / `outputs:` metadata to `user_task` for Q13 contract blocks. Also: handle `agent-name:` field removal per Q28.a — runtime derives prompt path from task name; error at startup if file missing. If no changes needed, mark this item done with a one-line "no schema/generator changes required" note in this file and skip the commit. Otherwise commit (generator + schema + tests).
    **Done when:** schema + generator support the metadata Item 3's YAML encoding will need.

3. - [ ] **Item 3 — Phase C.3: Migrate rest of YAML.** Encode the full structure into `process-flow.yaml`:
    - TOP: `refine-ticket`, `implement-ticket` (with mechanical-lookup gateway from the table above), `refactor` (ad-hoc).
    - All 7 CYCLEs (`change-system-behavior`, `cover-system-behavior`, `redesign-system-structure`, `refactor-system-structure`, `refactor-test-structure`, `refine-backlog`, `onboard-external-system`).
    - All HIGH orchestrations (parameterized core + wrappers + composites).
    - All MID `call_activity` definitions (including the 8 added by Item 11 connectedness pass).
    - LOW primitives (4: approve, execute-agent, execute-command, fix).

    Resolve deferred questions during encoding:
    - **Q5.a** — pick `run-tests` filter encoding shape.
    - **Q31.a** — pick `cover-system-behavior` AT vs CT handling.

    Regenerate `docs/process-diagram.md`. Diff against `.pre-refactor` backup to confirm every retained behaviour appears. Resolve any gap (either by adding to YAML or by writing an explicit drop-rationale comment, cross-referencing the archive's cross-check inventory). Remove the `.pre-refactor` backup once verified. Commit.
    **Done when:** TOP + all cycles + all HIGH + all MID + all LOW encoded; regenerated diagram covers everything in the archive's cross-check inventory; no intended-to-survive behaviour is missing; Q5.a + Q31.a recorded in this file.

4. - [ ] **Item 4 — Phase D handoff: Write the downstream-alignment plan.** Create `plans/<YYYYMMDD-HHMM>-bpmn-refactor-downstream.md` covering:
    - Writing-agent updates per Q1 (FIX as separate primitive), Q4 (terminology), Q5 (run-tests filter parameter shape).
    - **Prompt file renames + deletions per the locked Q28 table in the archive plan**, including:
      - `agent-name:` field removal from `process-flow.yaml` (Q28.a) — runtime contract change.
      - `fix-verify.md` split into `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md` (Q28.b).
      - Q28.c resolutions: `refactor-system.md` canonical (from `task-system-implementation-refactoring.md`); `at-refactor-system.md` DELETE (Q32); `task-system-interface-redesign.md` + `task-external-system-interface-redesign.md` split per recommended resolution.
      - Legacy `legacy-*.md` prompts DELETE per Q16 = B collapse.
    - ATDD docs updates (`docs/atdd/process/*.md`, `docs/atdd/architecture/*.md`) for the new five-level vocabulary.
    - Retired SVG cleanup under `docs/images/process-diagram-*.svg`.

    Use the same `## Items` checklist shape so it's `/execute-plan`-able. Do **not** execute that plan here — the user invokes `/execute-plan` on it separately. Commit.
    **Done when:** the downstream plan file exists with its own Items checklist; this plan's Items section is fully resolved.

---

## Re-running `/execute-plan`

Invoke `/execute-plan plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` repeatedly. Each invocation reads this file, finds the next unchecked Item, executes it, deletes the resolved item from this file, and commits. Default cadence is `/clear` between items — cached-prefix replay grows with every read/edit, so the natural seam is a `/clear`.

## Standing constraints (from user memory)

- **Token-efficient by default** — flag any user-proposed workflow that burns tokens unnecessarily (`feedback_flag_non_token_efficient`).
- **Session-handoff cadence: auto-commit, then surface `/clear` + `/execute-plan`.** End-of-item: auto-commit with a surgical message via raw `git` (no `/commit` per `feedback_use_commit_skill`); then surface the literal next-session commands in a Next-steps block (`feedback_offer_clear_then_execute_plan`, `feedback_execute_plan_always_next_steps`).
- **Concurrent-agent collision risk** — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
- **Legacy test artifacts** are indistinguishable from AT/CT artifacts on disk — no folder, no annotation, no filename suffix (`feedback_legacy_tests_no_marker`). Authoring cycles collapse per Q16 = B in the archive.
