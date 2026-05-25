# BPMN four-level refactor — design plan

> **Working style: token-efficient.** Execute this plan in the cheapest form that still produces a quality result. If the user proposes a workflow that burns tokens unnecessarily (e.g., asking 8 questions individually via `AskUserQuestion` when 8 pre-drafted recommendations could be confirmed in one batch), **surface the cheaper alternative and let the user choose** — don't silently follow the costly path. The user has explicitly invited this pushback (memory: `feedback_flag_non_token_efficient`).

End-state-first: lock the design **before** drawing diagrams; the existing diagram generator (`gh optivem process show`) handles the drawing, so this plan never hand-draws Mermaid.

## Scope

### Intent: full replacement of the existing BPMN

The four-level structure (LOW / MID / HIGH / PEAK) is intended to **fully replace the existing BPMN**, not patch it. Concretely:

- The canonical source today is `internal/atdd/runtime/statemachine/process-flow.yaml` (read by the statemachine), rendered into `docs/process-diagram.md` plus the 21 SVGs under `docs/images/process-diagram-*.svg`.
- After this refactor lands, those artifacts are regenerated from — or removed in favour of — the new four-level structure. **Nothing from the old shape survives by accident.** Every retained behaviour is a deliberate decision recorded in this file.

### Cross-check vs existing BPMN

Because the replacement is full, every concern the existing BPMN models must be either **absorbed** into the new four-level structure (with location) or **dropped** (with reason). Inventory of the 21 existing diagrams:

**Maps cleanly into the four levels (presumed during Item 6; verify each):**

| # | Diagram | Likely target |
|---|---|---|
| 1 | legend | doc artifact, regenerate |
| 2 | ticket-lifecycle | PEAK wrapper (Q7) |
| 3 | github-intake | PEAK wrapper (Q7) — tied to ticket lifecycle |
| 5 | run-cycle | MID Run Tests (Q5) |
| 6 | at-cycle | HIGH WRITE TESTS BIG ↔ WRITE SYSTEM BIG pairing |
| 7 | at-green-system | HIGH WRITE SYSTEM BIG |
| 8 | contract-test-sub-process | HIGH IMPLEMENT RED EXTERNAL SYSTEM DRIVER ADAPTERS — CONTRACT TESTS |
| 10 | structural-cycle-shared | HIGH shared IMPLEMENT TEST LAYER |
| 11 | commit-sub-process | MID Commit (calls EXECUTE COMMAND) |
| 13 | at-refactor-system | PEAK REFACTOR SYSTEM STRUCTURE (Q8 adds compile/verify) |
| 15 | compile | MID Compile (calls EXECUTE COMMAND) |
| 16 | da-cycle | PEAK REDESIGN SYSTEM STRUCTURE (Write Driver Adapters) |
| 17 | green-phase-cycle | HIGH WRITE SYSTEM BIG |
| 20 | red-phase-cycle | HIGH WRITE TESTS BIG |
| 21 | sut-cycle | HIGH WRITE SYSTEM BIG |

**Legacy cycles — collapse into normal flows** (per memory `feedback_legacy_tests_no_marker`: legacy tests are indistinguishable from AT/CT tests, so dedicated legacy cycles disappear). **All entries below are my best guess — user must verify in Item 1; initial guesses likely wrong on the change-vs-cover split:**

| # | Diagram | Likely target | Reasoning (verify) |
|---|---|---|---|
| 4 | run-legacy-cycle | shared sub-process called from both CHANGE and COVER | running existing legacy tests during either cycle |
| 12 | legacy-acceptance-criteria-cycle | **COVER** (was CHANGE — likely wrong) | writing new ACs against existing legacy behavior = adding coverage, not changing behavior |
| 18 | legacy-at-cycle | **COVER** (was CHANGE — likely wrong) | writing legacy ATs for existing behavior = adding coverage |
| 19 | legacy-ct-cycle | COVER | writing legacy CTs for existing behavior = adding coverage |

**Not currently mapped — needs decision:**

| # | Diagram | Gap |
|---|---|---|
| 9 | external-system-onboarding | No equivalent in the four brainstorm files. **See Q-ext** (conditional). |
| 14 | backlog-refinement | No equivalent in the four brainstorm files. User explicitly flagged in-scope. **See Q9.** |

### Already-confirmed scope inclusions

- **Ticket intake and updates are in scope** (see Q7). Currently covered today by diagrams 2 (ticket-lifecycle) and 3 (github-intake); both must be absorbed.
- **Backlog refinement is in scope** (user flagged). Currently covered today by diagram 14; must be absorbed — see Q9.

## Inputs (brainstorm)

The four files the user wrote, in `plans/ideas/`:

- `plans/ideas/1-bpmn-refactor-low-level.md` — **LOW:** APPROVE / EXECUTE AGENT / EXECUTE COMMAND primitives.
- `plans/ideas/2-bpmn-refactor-mid-level.md` — **MID:** parametrized agent tasks and command tasks that each `call` a low-level primitive (per Q4 recommendation).
- `plans/ideas/3-bpmn-refactor-high-level.md` — **HIGH:** orchestrations (WRITE TESTS BIG, WRITE RED ACCEPTANCE TESTS, IMPLEMENT RED DSL CORE, …, shared IMPLEMENT TEST LAYER / VERIFY TESTS PASS / VERIFY TESTS FAIL).
- `plans/ideas/4-bpmn-refactor-peak-level.md` — **PEAK:** workflow types (CHANGE SYSTEM BEHAVIOR, COVER SYSTEM BEHAVIOR, REDESIGN SYSTEM STRUCTURE, REFACTOR SYSTEM STRUCTURE, REFACTOR TEST STRUCTURE).

## Phases overview

- **Phase A** (Item 1) — Batch-resolve the 14 open questions in this file.
- **Phase B** (Items 2–6) — Apply decisions to the four brainstorm docs + walk the cross-check inventory.
- **Phase C** (Items 7–9) — Update `process-flow.yaml`, regenerate `docs/process-diagram.md` via `gh optivem process show > docs/process-diagram.md`. **No hand-drawn Mermaid.** Schema/generator changes if needed.
- **Phase D** (Item 10) — Write a separate plan for downstream alignment (writing-agents, ATDD docs, retired SVGs). That new plan is then executed independently.

## Items

Each item is sized for one `/execute-plan` invocation. Re-running `/execute-plan plans/20260525-1057-bpmn-refactor-design.md` picks up the next unchecked item.

1. - [ ] **Phase A — Resolve open questions + verify cross-check inventory (batch-confirm).** Read this plan, the four brainstorm inputs (`plans/ideas/1-4-*.md`), and skim the existing BPMN (`internal/atdd/runtime/statemachine/process-flow.yaml`, `docs/process-diagram.md`). In a single batched message to the user, present:
    1. All **15 open questions** (Q1–Q14 + Q15) with their pre-drafted recommendations. User accepts all or lists edits.
    2. The **cross-check inventory** from the Scope section (all three tables: maps-cleanly / legacy / not-mapped). For each row, ask the user to confirm the absorption target or correct it. **Pay special attention to the legacy mappings (#4, #12, #18, #19) — the absorption targets there are my best guess and likely contain errors.**
    Update the **Decisions** section with the final answers (replace each `(pending — recommended X)` with the resolved answer). Update the Scope cross-check tables with any user corrections. Commit.
    **Done when:** every Decisions entry has status ≠ pending; every cross-check row is user-confirmed; commit on `main`.

2. - [ ] **Phase B.1 — Refine LOW brainstorm.** Apply Q1 (FIX primitive), Q2 (post-approve symmetry), Q3 (APPROVE NO-branch), Q4 (terminology) to `plans/ideas/1-bpmn-refactor-low-level.md`. Cross-check against any LOW-affecting existing diagrams. Commit.
    **Done when:** the file reflects every LOW decision; no contradictions vs Decisions.

3. - [ ] **Phase B.2 — Refine MID brainstorm.** Apply Q5 (Run Tests granularity), Q4 (rename "inherits" → "calls"), Q13 (contract format if it changes how MID describes tasks) to `plans/ideas/2-bpmn-refactor-mid-level.md`. Cross-check diagrams 5 (run-cycle), 11 (commit), 15 (compile). Commit.

4. - [ ] **Phase B.3 — Refine HIGH brainstorm.** Apply Q6 (port-change wiring table) and any other HIGH-affecting decisions to `plans/ideas/3-bpmn-refactor-high-level.md`. Fix the file's mid-sentence cut-off at the end. Cross-check diagrams 6, 7, 8, 10, 17, 20, 21. Commit.

5. - [ ] **Phase B.4 — Refine PEAK brainstorm.** Apply Q7 (ticket wrapper), Q8 (refactor compile/verify), Q9 (new REFINE BACKLOG peak entry), **Q15 (rename "Write System" / "Write Driver Adapters" → "Implement …" if convention A is chosen)** to `plans/ideas/4-bpmn-refactor-peak-level.md`. Cross-check diagrams 2, 3, 13, 14, 16. Commit.

6. - [ ] **Phase B.5 — Cross-check inventory walk.** Walk the cross-check tables in this plan's Scope section row by row. For each "Maps cleanly" diagram, confirm it actually fits the refined brainstorm. For each "Legacy" diagram, confirm the collapse target. Decide on **Q-ext** (external-system-onboarding) — promote to a real question or drop. Append any new follow-ups to the Decisions section. Commit.
    **Done when:** every existing diagram has a confirmed absorption target or explicit drop rationale.

7. - [ ] **Phase C.1 — Prototype REFACTOR SYSTEM STRUCTURE in YAML.** Encode the simplest peak entry (REFACTOR SYSTEM STRUCTURE) in `internal/atdd/runtime/statemachine/process-flow.yaml`. Save a copy of the current `docs/process-diagram.md` first (e.g., `cp docs/process-diagram.md docs/process-diagram.md.pre-refactor`). Run `gh optivem process show > docs/process-diagram.md`. Inspect the regenerated output for the new peak entry. Compare against the refined `plans/ideas/4-bpmn-refactor-peak-level.md`. Commit (YAML + regenerated md).
    **Done when:** regenerated diagram for REFACTOR SYSTEM STRUCTURE matches the refined brainstorm.

8. - [ ] **Phase C.2 — Schema/generator changes if needed.** Based on Item 7 findings, extend the YAML schema and generator if needed. Most likely candidate: adding `scopes:` / `outputs:` metadata to `user_task` for contract blocks per Q13 (if Q13 resolved to option A). If no changes needed, mark this item done with a one-line "no schema/generator changes required" note in this file and skip the commit. Otherwise commit (generator + schema + tests).

9. - [ ] **Phase C.3 — Migrate rest of YAML.** Encode all remaining peak entries + high orchestrations + mid `call_activity` definitions into `process-flow.yaml`. Regenerate `docs/process-diagram.md`. Diff against the pre-refactor copy from Item 7 to confirm every retained behaviour appears. Resolve any gap (either by adding to YAML or by writing an explicit drop-rationale comment). Remove the `.pre-refactor` backup once verified. Commit.
    **Done when:** all peak entries are encoded; regenerated diagram covers everything in the cross-check inventory; no intended-to-survive behaviour is missing.

10. - [ ] **Phase D handoff — Write the downstream-alignment plan.** Create `plans/<YYYYMMDD-HHMM>-bpmn-refactor-downstream.md` covering: writing-agent updates (per Q1, Q4, Q5 decisions), ATDD docs updates (`docs/atdd/process/*.md`, `docs/atdd/architecture/*.md`), retired SVG cleanup. Use the same `## Items` checklist shape as this plan so it's `/execute-plan`-able. Do **not** execute that plan here — the user invokes `/execute-plan` on it separately. Commit.
    **Done when:** the downstream plan file exists with its own Items checklist; this plan's Items section is fully checked.

---

## Open questions (Phase A reference content for Item 1)

Pre-drafted with recommendations. Item 1 batches all of these to the user; user confirms or edits.

### Q1 — Fix-loop recursion bounds  *(LOW)*

**Context.** In `1-bpmn-refactor-low-level.md`, EXECUTE AGENT step 4 says *"Valid? NO: Fix Agent Run (AGENT)"*. Fix is itself an agent run, so a fix that also fails validation would loop forever. The same shape repeats in EXECUTE COMMAND step 3. The file's top FUTURE IDEA note already hints: *"For FIX, currently one task. Maybe 2, to separate planning vs execution, so that human approves…"*.

**Options.**
- **(A) FIX is a separate primitive.** Its own contract, single attempt, terminates regardless of outcome. Adds a 4th low-level primitive. No recursion possible.
- **(B) Bounded retry with human escalation.** Fix runs as EXECUTE AGENT up to N attempts; on all-fail, prompt human. Keeps 3 primitives, adds retry-count config.
- **(C) Single retry, then HARD EXIT.** Simplest but brittle.

**Recommendation: A.** Different semantics → different primitive. Matches the FUTURE IDEA's plan/execute split. Teaching repo, so clarity > 3-primitive minimalism.

### Q2 — EXECUTE COMMAND post-approve symmetry  *(LOW)*

**Context.** EXECUTE AGENT has both `Approve (PRE)` and `Approve (POST)`. EXECUTE COMMAND only has `Approve (PRE)`. Asymmetric.

**Options.**
- **(A) Add `Approve (POST)` to EXECUTE COMMAND.** Symmetric, defensive.
- **(B) Remove `Approve (POST)` from EXECUTE AGENT.** Symmetric, lean.
- **(C) Keep asymmetric.** Justify: agent output needs human-eye review; command success is binary.

**Recommendation: C.** Commands have machine-checkable success; agents produce content needing human review.

### Q3 — APPROVE NO-branch (exit vs retry)  *(LOW)*

**Context.** APPROVE currently only models hard-exit on NO. But a NO on the post-approve inside EXECUTE AGENT probably means *"re-run the agent,"* not *"kill the flow."*

**Options.**
- **(A) APPROVE stays exit-only; callers handle NO-branch themselves.**
- **(B) APPROVE gains a parameterized NO-action** (`exit` | `retry-caller`).
- **(C) Two distinct primitives** (APPROVE-OR-EXIT, APPROVE-OR-RETRY).

**Recommendation: A.** APPROVE remains pure; control flow on NO belongs to caller.

### Q4 — Terminology: "inherits" vs "calls"/"instantiates"  *(LOW)*

**Context.** Mid-level says agent tasks *"inherit the low level EXECUTE AGENT task"*. BPMN has call activities, not inheritance.

**Options.** (A) "calls", (B) "instantiates", (C) "invokes", (D) keep "inherits".

**Recommendation: A.** "Calls" matches BPMN call-activity semantics.

### Q5 — Run Tests granularity  *(MID)*

**Context.** Mid-level says: *"Run Tests (it can run all tests or we pass some filter to be selective... not sure if one task or multiple)"*. High-level is cut off mid-sentence on this.

**Options.**
- **(A) Single Run Tests task with required filter parameter** (`acceptance` / `contract` / `unit` / `all`).
- **(B) Two tasks: Run All Tests, Run Filtered Tests.**
- **(C) Three tasks split by test type.**

**Recommendation: A.** Natural extension of the "calls EXECUTE COMMAND with params" pattern.

**Cross-check.** Existing diagram 5 (run-cycle) is the absorption target.

### Q6 — Port-change output→branch wiring  *(HIGH)*

**Context.** High-level branches on *"DSL Port Changed?"* and *"External System Driver Ports Changed?"*. These depend on outputs produced by prior mid-level tasks. The wiring needs to be explicit.

**Decision form.** A table — see Q12 for prompt shape.

| Producer (mid-level task) | Output variable | Consumer (high-level branch) |
|---|---|---|
| Write Acceptance Tests | `dsl-port-changed: bool` | WRITE TESTS BIG step 2 |
| ? | `system-driver-ports-changed: bool` | WRITE TESTS BIG step 2.1.2 |
| ? | `external-driver-ports-changed: bool` | WRITE TESTS BIG step 2.1.1 |

User to fill in the remaining producers during Item 1.

### Q7 — Ticket lifecycle placement  *(PEAK)*

**Context.** Peak file mentions *"marking the ticket IN PROGRESS … then In Acceptance"* but doesn't structure it. Cross-cutting.

**Scope confirmation (from user).** In scope — see Scope section above.

**Options.**
- **(A) Wrapper at peak level** — marks IN PROGRESS → calls the peak entry → marks In Acceptance.
- **(B) New low-level primitive UPDATE TICKET STATUS** invoked from each peak entry.
- **(C) Per-peak-entry steps.**

**Sub-question.** Are Acceptance Criteria / Checklists ticked off, or ignored at this level?

**Cross-check.** Diagrams 2 (ticket-lifecycle) and 3 (github-intake) are absorption targets.

**Recommendation: A.** Peak-level wrapper for cross-cutting concerns; default **not ticking AC/Checklists** at this level.

### Q8 — REFACTOR flows missing compile/verify steps  *(PEAK)*

**Context.** REFACTOR SYSTEM STRUCTURE has only *"Write System"*; REFACTOR TEST STRUCTURE has only *"Refactor Tests"*. Neither verifies tests still pass.

**Options.**
- **(A) Add explicit Compile + Verify-Tests-Pass to both refactor peaks.**
- **(B) Single Verify step** (subsumes compile + verify).
- **(C) Leave implicit.**

**Recommendation: A.** Refactor without verify is meaningless; teach the discipline at peak.

### Q9 — Backlog refinement integration  *(PEAK)*

**Context.** Existing diagram 14 (backlog-refinement) is in scope per user. Not covered in any brainstorm file. Needs absorption.

**Options.**
- **(A) New peak entry: REFINE BACKLOG.** Sibling to the existing five.
- **(B) Pre-peak meta-wrapper** — refinement runs before any other peak entry.
- **(C) Fold into Q7's ticket-lifecycle wrapper.**

**Cross-check.** Existing diagram 14 is the absorption target. Read the SVG / YAML for refinement nodes before answering.

**Recommendation: A.** Refinement is a distinct activity (produces a refined ticket, not a system change) — peak sibling.

### Q10 — Per-phase acceptance criteria  *(PROCESS)*

**Context.** Phases need "done" criteria. Without them, the executor judges by feel.

**Options.** (A) Inline criteria per item (already done in this version of the plan), (B) Single checklist, (C) Implicit.

**Recommendation: A** — and already implemented in this file's Items section.

### Q11 — Commit cadence  *(PROCESS)*

**Context.** When to commit during Phases A/B/C.

**Options.** (A) Per item (this plan's default — each Item ends with "Commit"), (B) Per phase boundary, (C) Single commit at end.

**Recommendation: A** (per item, matching `/execute-plan` natural cadence). Squash later if desired.

### Q12 — Q6 (port-change wiring) prompt shape  *(PROCESS)*

**Context.** Q6 needs a table filled in, not a multi-option pick.

**Options.**
- **(A) Agent drafts the full table, asks user to confirm/edit.** Propose-then-confirm.
- **(B) Iterate row by row.**
- **(C) Narrative description in Decisions.**

**Recommendation: A.** Lower cognitive load.

### Q13 — Contract block format and location  *(PROCESS)*

**Context.** Contract blocks describe each node's `agent-name` / `scopes` / `outputs`. With Phase C regenerating from YAML, the question is where these live.

**Options.**
- **(A) In `process-flow.yaml` as `user_task` metadata** (`scopes:`, `outputs:`). Single source of truth. Requires schema/generator extension in Item 8.
- **(B) In the refined brainstorm docs only.** YAML stays minimal. Drift risk.
- **(C) Richer schema co-located in YAML.** Heaviest.

**Recommendation: A.** Eliminates drift; schema extension is small.

### Q14 — `docs/process-diagram.md` structure: one file or multiple  *(PROCESS / Phase C)*

**Context.** Today one scrolling file. Could split per level for clearer navigation.

**Options.** (A) One file (current), (B) Split by level, (C) Split by process, (D) Mixed (index + per-level).

**Recommendation: A.** Lowest cost; no generator change. Easy to revisit in Phase D if file becomes unwieldy.

### Q15 — Naming convention: "Write" vs "Implement"  *(NAMING — pin early)*

**Context.** The brainstorm files mix the two verbs inconsistently:
- **Mid-level:** "Write Acceptance Tests" (tests) + "Implement DSL" / "Implement System Drivers" / "Implement External System Drivers" (code). Write for tests, Implement for code.
- **High-level:** "Write RED Acceptance Tests" (tests) + "IMPLEMENT RED DSL CORE" / "IMPLEMENT RED SYSTEM DRIVER ADAPTERS" (code). Same convention.
- **Peak-level:** "Write System" / "Write Driver Adapters" (code — uses *Write* for code, inconsistent with mid/high).

Pinning the convention now avoids rework in Items 2–5 (every brainstorm doc gets edited; naming churn would compound).

**Options.**
- **(A) "Write" for tests, "Implement" for production code.** Matches mid/high convention; natural English in TDD/ATDD vocabulary. Peak needs renaming: "Write System" → "Implement System", "Write Driver Adapters" → "Implement Driver Adapters".
- **(B) "Write" throughout.** Forces high/mid to rename ("Implement DSL" → "Write DSL Core", etc.).
- **(C) "Implement" throughout.** Forces awkward phrasing ("Implement Acceptance Tests").
- **(D) Other verb pair.** E.g., "Author" / "Build". Possible but adds learning cost.

**Recommendation: A.** "Write tests, Implement code" is canonical TDD vocabulary; peak is the outlier and gets renamed (small surface, fixed in Item 5).

---

## Cross-check follow-up (only if confirmed needed)

- **Q-ext — External system onboarding integration.** Existing diagram 9 (`process-diagram-9-external-system-onboarding-sub-process.svg`) is not currently mapped to anything in the four brainstorm files. Likely absorbs into REDESIGN SYSTEM STRUCTURE (since onboarding produces new driver adapters), but worth a discrete decision. Promoted to a real question only if Item 6's cross-check walk surfaces a real gap.

---

## Decisions

*(Item 1 fills this in; subsequent items reference it.)*

### PROCESS / NAMING
- **Q10 — Per-phase acceptance criteria:** *(pending — recommended A: inline criteria per item, already implemented)*
- **Q11 — Commit cadence:** *(pending — recommended A: per item)*
- **Q12 — Q6 prompt shape:** *(pending — recommended A: propose-then-confirm)*
- **Q13 — Contract block format and location:** *(pending — recommended A: in YAML as `user_task` metadata)*
- **Q14 — `docs/process-diagram.md` structure:** *(pending — recommended A: keep one file)*
- **Q15 — "Write" vs "Implement" naming:** *(pending — recommended A: Write for tests, Implement for code; rename peak entries accordingly)*

### LOW
- **Q1 — Fix-loop recursion bounds:** *(pending — recommended A: FIX as separate primitive)*
- **Q2 — EXECUTE COMMAND post-approve symmetry:** *(pending — recommended C: keep asymmetric)*
- **Q3 — APPROVE NO-branch:** *(pending — recommended A: caller owns NO branch)*
- **Q4 — Terminology:** *(pending — recommended A: "calls")*

### MID
- **Q5 — Run Tests granularity:** *(pending — recommended A: single task with type filter)*

### HIGH
- **Q6 — Port-change output→branch wiring:** *(pending — needs table; see Q12 for prompt shape)*

### PEAK
- **Q7 — Ticket lifecycle placement:** *(pending — recommended A: peak-level wrapper; AC/Checklists not ticked)*
- **Q8 — REFACTOR flows missing compile/verify:** *(pending — recommended A: add explicit Compile + Verify-Tests-Pass)*
- **Q9 — Backlog refinement integration:** *(pending — recommended A: new peak entry REFINE BACKLOG)*

### Follow-ups (conditional)
- **Q-ext — External system onboarding integration:** *(only if Item 6 reveals a real gap)*

---

## Re-running `/execute-plan`

Invoke `/execute-plan plans/20260525-1057-bpmn-refactor-design.md` repeatedly. Each invocation:

1. Reads this file and finds the next unchecked Item.
2. Executes it (asking user for input when needed — Item 1 in particular requires user confirmation of decisions).
3. Marks the Item checkbox `[x]` when done.
4. Commits.
5. Stops (per-item gating is the default).

Items are independent enough that you can invoke `/execute-plan` once per item, or chain several. Items 6, 8, and the cross-check sweeps inside B-items may surface new questions — record them in the Decisions section as new follow-ups (no need to add new Items unless the work is non-trivial).

## Standing constraints (from user memory)

- **Token-efficient by default** — flag any user-proposed workflow that burns tokens unnecessarily and offer a cheaper alternative (`feedback_flag_non_token_efficient`).
- For agent-authored surgical commits with specific message + file list, use raw `git`, not `/commit` (`feedback_use_commit_skill`).
- Concurrent-agent collision risk — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
- Legacy tests/diagrams collapse into AT/CT, not preserved as separate flows (`feedback_legacy_tests_no_marker`).
