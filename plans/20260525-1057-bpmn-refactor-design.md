# BPMN four-level refactor — design plan

> **Working style: token-efficient.** Execute this plan in the cheapest form that still produces a quality result. If the user proposes a workflow that burns tokens unnecessarily (e.g., asking 8 questions individually via `AskUserQuestion` when 8 pre-drafted recommendations could be confirmed in one batch), **surface the cheaper alternative and let the user choose** — don't silently follow the costly path. The user has explicitly invited this pushback (memory: `feedback_flag_non_token_efficient`).

End-state-first: lock the design **before** drawing diagrams; the existing diagram generator (`gh optivem process show`) handles the drawing, so this plan never hand-draws Mermaid.

## Scope

### Intent: full replacement of the existing BPMN

*(Q17 resolved 2026-05-25: full replacement — option A.)*

The four-level structure (LOW / MID / HIGH / PEAK) is intended to **fully replace the existing BPMN**, not patch it. Concretely:

- The canonical source today is `internal/atdd/runtime/statemachine/process-flow.yaml` (read by the statemachine), rendered into `docs/process-diagram.md` plus the 21 SVGs under `docs/images/process-diagram-*.svg`.
- After this refactor lands, those artifacts are regenerated from — or removed in favour of — the new four-level structure. **Nothing from the old shape survives by accident.** Every retained behaviour is a deliberate decision recorded in this file.

### Cross-check vs existing BPMN

Because the replacement is full, every concern the existing BPMN models must be either **absorbed** into the new four-level structure (with location) or **dropped** (with reason). Inventory of the 21 existing diagrams:

**Maps cleanly into the four levels** — *all 15 absorption targets confirmed 2026-05-25:*

| # | Diagram | Absorption target |
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
| 13 | at-refactor-system | PEAK REFACTOR SYSTEM STRUCTURE (verify inherited from `WRITE SYSTEM (BIG)`; see Q8) |
| 15 | compile | MID Compile (calls EXECUTE COMMAND) |
| 16 | da-cycle | PEAK REDESIGN SYSTEM STRUCTURE (Implement Driver Adapters — renamed per Q15) |
| 17 | green-phase-cycle | HIGH WRITE SYSTEM BIG |
| 20 | red-phase-cycle | HIGH WRITE TESTS BIG |
| 21 | sut-cycle | HIGH WRITE SYSTEM BIG |

**Legacy cycles** — *resolved 2026-05-25 via Q16=B (legacy cycles collapse). Three of the four absorb into `COVER SYSTEM BEHAVIOR`; #4 is dropped (no separate legacy-test-run operation exists):*

| # | Diagram | Resolution | Rationale |
|---|---|---|---|
| 4 | run-legacy-cycle | **DROP** (no absorption) | No separate "run legacy tests" operation. The mid-level `Run Tests` task runs whatever's currently in the test suite; legacy-vs-latest distinction doesn't exist at run time per Q16=B + memory `feedback_legacy_tests_no_marker`. |
| 12 | legacy-acceptance-criteria-cycle | **PEAK `COVER SYSTEM BEHAVIOR`** (with `Expected Test Result: Success`) | Writing ACs for existing behavior = adding coverage = COVER. |
| 18 | legacy-at-cycle | **PEAK `COVER SYSTEM BEHAVIOR`** | Writing legacy ATs for existing behavior = COVER. |
| 19 | legacy-ct-cycle | **PEAK `COVER SYSTEM BEHAVIOR`** | Writing legacy CTs for existing behavior = COVER. |

**Not currently mapped** — *both included; resolutions recorded 2026-05-25:*

| # | Diagram | Resolution |
|---|---|---|
| 9 | external-system-onboarding | **PEAK new entry `ONBOARD EXTERNAL SYSTEM`** (Q-ext resolved to option b — standalone peak workflow; REDESIGN SYSTEM STRUCTURE can also call it as a sub-process). |
| 14 | backlog-refinement | **PEAK new entry `REFINE BACKLOG`** (Q9 resolved to A — sibling to the existing five peak entries). |

### Already-confirmed scope inclusions

- **Ticket intake and updates are in scope** (see Q7). Currently covered today by diagrams 2 (ticket-lifecycle) and 3 (github-intake); both must be absorbed.
- **Backlog refinement is in scope** (user flagged). Currently covered today by diagram 14; must be absorbed — see Q9.

## Inputs (brainstorm)

*(Q18 resolved 2026-05-25: files are exhaustive + final inputs — option A. Items 2–5 only apply Q-decisions on top of them.)*

The four files the user wrote, in `plans/ideas/`:

- `plans/ideas/1-bpmn-refactor-low-level.md` — **LOW:** APPROVE / EXECUTE AGENT / EXECUTE COMMAND primitives.
- `plans/ideas/2-bpmn-refactor-mid-level.md` — **MID:** parametrized agent tasks and command tasks that each `call` a low-level primitive (per Q4 recommendation).
- `plans/ideas/3-bpmn-refactor-high-level.md` — **HIGH:** orchestrations (WRITE TESTS BIG, WRITE RED ACCEPTANCE TESTS, IMPLEMENT RED DSL CORE, …, shared IMPLEMENT TEST LAYER / VERIFY TESTS PASS / VERIFY TESTS FAIL).
- `plans/ideas/4-bpmn-refactor-peak-level.md` — **PEAK:** workflow types (CHANGE SYSTEM BEHAVIOR, COVER SYSTEM BEHAVIOR, REDESIGN SYSTEM STRUCTURE, REFACTOR SYSTEM STRUCTURE, REFACTOR TEST STRUCTURE).

## Phases overview

- ~~**Phase A** (Item 1) — Batch-resolve the open questions in this file.~~ ✓ **Completed 2026-05-25.** All 23 questions (Q1–Q23 + Q-ext) resolved; cross-check inventory confirmed. See Decisions section.
- **Phase B** (Items 2–6) — Apply decisions to the four brainstorm docs + walk the cross-check inventory.
- **Phase C** (Items 7–9) — Update `process-flow.yaml`, regenerate `docs/process-diagram.md` via `gh optivem process show > docs/process-diagram.md`. **No hand-drawn Mermaid.** Schema/generator changes if needed.
- **Phase D** (Item 10) — Write a separate plan for downstream alignment (writing-agents, ATDD docs, retired SVGs). That new plan is then executed independently.

## Items

Each item is sized for one `/execute-plan` invocation. Re-running `/execute-plan plans/20260525-1057-bpmn-refactor-design.md` picks up the next unchecked item. Item numbering is stable — Item 1 was completed 2026-05-25 and removed per the `/execute-plan` rule (resolved items are deleted, not checked); remaining items keep their original numbers so cross-references in this file stay correct.

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

### Q17 — Full replacement vs additive vs partial replacement of the existing BPMN  *(DOCTRINE — pin before Item 1's cross-check confirmation; reshapes Scope > Intent)*

**Context.** The plan's Scope > Intent section asserts the four-level structure *fully replaces* the existing 21-diagram BPMN — "nothing from the old shape survives by accident." This framing drives the entire Cross-check inventory (every existing diagram must be absorbed or dropped) and the artifact removal in Phase C. The brainstorm files are titled "BPMN - LOW/MID/HIGH/PEAK" — they describe a new structure but don't explicitly say "replace everything." Origin: my inference in commit `c20fd4a`, not a user statement.

**Options.**
- **(A) Full replacement.** As currently drafted. Four-level structure fully supersedes the existing 21 diagrams. Every existing diagram is absorbed (mapped into the new structure) or dropped (with explicit rationale).
- **(B) Additive.** Four-level coexists with the existing BPMN. Some existing diagrams (e.g., legend, github-intake) stay as first-class artifacts alongside the new structure. Cross-check inventory becomes "which existing diagrams gain a four-level counterpart" rather than "absorb or drop."
- **(C) Partial replacement.** Explicit list of which existing diagrams are replaced and which are retained. Requires a per-diagram retention decision on top of the absorption-target decision.

**Recommendation: discuss before drafting.** Both (B) and (A) are defensible; (C) is the most flexible but adds per-diagram decision overhead. **No pre-drafted recommendation; user decides.**

---

### Q18 — Are the four brainstorm files exhaustive + final, or working drafts?  *(DOCTRINE — pin before Item 1's batch; reshapes Items 2–5)*

**Context.** Items 2–5 "refine the LOW/MID/HIGH/PEAK brainstorm" by applying the resolved Q-decisions to `plans/ideas/1-4-*.md`. This assumes those four files are the complete + correct source material for the new structure. If they're working drafts you'd revise independently, or if you have more brainstorm material not yet captured, Items 2–5's scope changes.

**Options.**
- **(A) Files are exhaustive + final inputs.** Items 2–5 only apply Q-decisions on top of them; no new content added beyond what the decisions imply. Plan as currently drafted.
- **(B) Files are working drafts; user adds/revises content during Items 2–5.** Items 2–5 expand to include "user adds missing content" before applying Q-decisions. Adds a user-input step inside each B-item.
- **(C) Some files are final, some are drafts.** User specifies per-file.

**Recommendation: discuss before drafting.** **No pre-drafted recommendation; user decides.**

---

### Q19 — Confirm "scope inclusions" attribution  *(SCOPE)*

**Context.** Plan's Scope section says:
- *"Ticket intake and updates are in scope (see Q7)"* — attributed to user via Q7.
- *"Backlog refinement is in scope (user flagged)"* — attributed to user.

These are stated as already-confirmed. Verifying the attribution now avoids building Items on a wrong premise.

**Options.**
- **(A) Both inclusions are confirmed.** No change.
- **(B) Only one was actually flagged by you; the other is my inference.** User specifies which is which; for the inferred one, decide whether to keep as scope or drop.
- **(C) Neither was flagged; both are my inference.** User decides per-inclusion whether to keep, drop, or convert to its own scope question.

**Recommendation: A.** Both inclusions came up in the prior plan-shaping conversation (commit `c20fd4a` includes both). Confirming with you is a sanity check, not a real ambiguity. If you remember saying one but not both, edit.

---

### Q20 — Item decomposition (10 items, this order)  *(PROCESS)*

**Context.** The Phase A→B→C→D shape and the 10-item granularity originated in commit `c20fd4a` ("reshape as /execute-plan checklist"). Worth confirming this matches your intended cadence before launching execution.

**Options.**
- **(A) Keep the 10-item shape.** Each item ~one `/execute-plan` invocation. Items 2–5 per-level (LOW/MID/HIGH/PEAK) to keep cross-check focus.
- **(B) Merge Items 2–5 into one Phase-B item.** Fewer commits, broader chunks. Trade-off: harder to fan out as parallel subagents.
- **(C) Split Item 9 (migrate-rest-of-YAML) by level.** Three sub-items: peak migration / high migration / mid migration.
- **(D) Other restructure** — user specifies.

**Recommendation: A.** Matches `/execute-plan` cadence; per-level B-items enable parallel subagent fan-out (memory: independent file edits → parallel subagents). Item 9 stays as one because the YAML migration needs to be diffed against the pre-refactor baseline as a single unit.

---

### Q21 — Phase D produces a separate downstream plan vs inlined items  *(PROCESS)*

**Context.** Item 10 writes a separate plan for downstream work (writing-agent updates, ATDD docs, retired SVG cleanup) instead of folding it into this plan.

**Options.**
- **(A) Separate plan, as drafted.** Cleaner `/execute-plan` boundary; downstream work decoupled from BPMN structure work.
- **(B) Inline downstream work as Items 11–N in this plan.** Single plan covers both; tighter coupling.

**Recommendation: A.** Memory `feedback_new_plan_not_extend` says broaden scope via a fresh plan, not by extending. Downstream alignment touches three surfaces (writing-agents, ATDD docs, SVGs) — separate plan warranted.

---

### Q22 — Q6's table shape (port-change wiring)  *(HIGH / PROCESS)*

**Context.** Q6 uses a three-column table (Producer / Output variable / Consumer). Other shapes exist.

**Options.**
- **(A) Three-column producer-output-consumer table.** Current draft.
- **(B) Truth table.** Per port, what conditions trigger which branch.
- **(C) Sequence-style.** Ordered list of "task X sets variable Y, branch Z reads it."

**Recommendation: A.** Producer-output-consumer is BPMN-natural and maps cleanly to YAML metadata in Q13.

---

### Q23 — Rendering pipeline (`gh optivem process show` stays?)  *(PROCESS / Phase C)*

**Context.** Phase C assumes the existing YAML + `gh optivem process show` rendering pipeline can express the four-level structure. If the new structure needs a richer output (per-level sub-diagrams, embedded contract blocks, etc.) that exceeds what the current generator emits, Item 8 expands.

**Options.**
- **(A) Existing pipeline stays.** Generator extensions in Item 8 stay scoped to YAML schema additions (e.g., `scopes:` / `outputs:` per Q13).
- **(B) New pipeline or rendering tool.** Item 8 grows to include pipeline replacement.
- **(C) Existing pipeline + parallel renderer.** E.g., per-level Mermaid sub-diagrams alongside the current single output.

**Recommendation: A.** Cheapest; existing pipeline already produces the BPMN diagrams. Defer (B)/(C) until a concrete gap surfaces in Item 7's prototype.

---

### Q16 — Do legacy authoring cycles disappear, or stay as distinct cycles producing indistinguishable artifacts?  *(DOCTRINE — pin before Item 1's cross-check confirmation)*

**Context.** Memory `feedback_legacy_tests_no_marker` says legacy *test artifacts* (files on disk) must be indistinguishable from change-cycle artifacts — no folder, no annotation, no filename suffix. The memory does **not** say the *authoring cycles* themselves disappear. In fact it explicitly preserves "the legacy cycle's own verify gate (`VERIFY_LEGACY_AT` / `VERIFY_LEGACY_CT`) ... applies at authoring time, inside the legacy cycle." This plan's earlier wording extrapolated artifacts → cycles, which is a doctrine call the user has not made. The user flagged this for discussion (2026-05-25).

**Options.**
- **(A) Cycles persist; artifacts are indistinguishable.** The four-level structure includes explicit legacy entry points — e.g., a peak entry `COVER LEGACY BEHAVIOR`, or a `legacy: true` flag on `COVER SYSTEM BEHAVIOR` that selects a different mid-level orchestration (one that uses the "inverted-RED expected-to-pass" verify gate). The legacy *cycle* is preserved as a first-class authoring shape; only the produced test files are uniform. This is what the memory literally says.
- **(B) Cycles collapse; legacy reduces to a parameter on the normal flows.** What was a "legacy AT cycle" becomes `COVER SYSTEM BEHAVIOR` with `expected-test-result: success`. No first-class legacy concept at peak or high level — the "expected to pass" property is just the existing per-cycle parameter, with values `success` (legacy/cover) or `failure` (change/red). This is what the plan's Scope > Legacy table currently assumes.
- **(C) Something else** — e.g., legacy is a wholly separate process tree, or only some legacy cycles collapse (e.g., diagram 19 collapses, diagrams 4/12/18 don't).

**Cross-check.** Existing diagrams 4 (run-legacy-cycle), 12 (legacy-acceptance-criteria-cycle), 18 (legacy-at-cycle), 19 (legacy-ct-cycle). If (A) wins, the Scope > Legacy table is rewritten — those diagrams stay as distinct cycles, just minus any test-artifact marker — and Items 4 (HIGH) / 5 (PEAK) gain new entries. If (B) wins, the table's COVER absorption targets stand, subject to per-row confirmation.

**Recommendation: discuss before drafting.** Both options are defensible. (A) is faithful to the memory but adds peak/high surface area. (B) is the leaner shape but reads more into the memory than it says. The choice is a doctrine call about how visible the legacy concept should be in the process model — not an obvious mechanical win. **No pre-drafted recommendation; user decides.**

---

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

## Decisions

*(All resolved 2026-05-25 in Phase A / Item 1. Subsequent items reference these.)*

### DOCTRINE
- **Q16 — Legacy cycle fate (collapse vs persist):** ✓ **B: legacy cycles collapse.** No dedicated legacy peak entry. **`COVER SYSTEM BEHAVIOR` (with `Expected Test Result: Success`) IS the legacy-coverage peak entry** — the `expected-test-result` parameter is what distinguishes legacy/cover (success) from change/red (failure). Test artifacts indistinguishable per memory `feedback_legacy_tests_no_marker`. There is **no separate "run legacy tests" operation** — the mid-level `Run Tests` task runs whatever's currently in the test suite, including tests authored via the legacy/cover path.
- **Q17 — Full replacement vs additive vs partial:** ✓ **A: full replacement.** Four-level structure fully supersedes the existing 21 diagrams; all existing diagrams will be deleted in Phase C (Item 9) once the new YAML is in place.
- **Q18 — Brainstorm files exhaustive vs working drafts:** ✓ **A: exhaustive + final.** Items 2–5 only apply Q-decisions to `plans/ideas/1-4-*.md`; no new content added beyond what decisions imply.

### SCOPE
- **Q19 — Scope-inclusion attribution (ticket intake + backlog refinement):** ✓ **A: both confirmed.**

### PROCESS / NAMING
- **Q10 — Per-phase acceptance criteria:** ✓ **A: inline criteria per item** (already implemented in Items section).
- **Q11 — Commit cadence (git, for plan execution):** ✓ **A: one git commit per item.**
- **Q12 — Q6 prompt shape:** ✓ **A: propose-then-confirm.**
- **Q13 — Contract block format and location:** ✓ **A: contract blocks live in `process-flow.yaml` as `user_task` metadata** (`scopes:`, `outputs:`). Both consumers read from the same YAML: (1) the agent invocation uses `scopes:`/`outputs:` for prompt context + permitted file scope; (2) the post-execute BPMN verify step (currently EXECUTE AGENT step 3) reads the same `outputs:` to validate "required output variables present?" and `scopes:` to validate "scope constraints satisfied? (diff)". Single source of truth, no drift.
- **Q14 — `docs/process-diagram.md` structure:** ✓ **A: one file.**
- **Q15 — "Write" vs "Implement" naming:** ✓ **A: Write for tests, Implement for code.** Peak entries get renamed: `Write System` → `Implement System`; `Write Driver Adapters` → `Implement Driver Adapters`.
- **Q20 — Item decomposition (10-item shape):** ✓ **A: keep 10 items.**
- **Q21 — Phase D as separate downstream plan:** ✓ **A: separate plan.**
- **Q22 — Q6's table shape (producer/output/consumer):** ✓ **A: three-column table.**
- **Q23 — Rendering pipeline (`gh optivem process show` stays):** ✓ **A: existing pipeline stays.**

### LOW
- **Q1 — Fix-loop recursion bounds:** ✓ **A: FIX as separate primitive** (4th low-level primitive, single attempt, terminates regardless of outcome).
- **Q2 — EXECUTE COMMAND post-approve symmetry:** ✓ **C: keep asymmetric** (commands have machine-checkable success; agents produce content needing human review).
- **Q3 — APPROVE NO-branch:** ✓ **A: APPROVE stays exit-only; caller owns NO branch.** **Action for Phase C:** when encoding APPROVE in `process-flow.yaml`, add a `TODO:` comment mentioning options B (parameterized NO-action) and C (two distinct primitives APPROVE-OR-EXIT, APPROVE-OR-RETRY) for possible revisit.
- **Q4 — Terminology:** ✓ **A: "calls"** (matches BPMN call-activity semantics).

### MID
- **Q5 — Run Tests granularity:** ✓ **A (modified): single `Run Tests` task with polymorphic filter parameter.** Filter accepts: (1) a test-type tag — `acceptance` / `contract` / `acceptance-api` / `acceptance-ui` / `contract-stub` / `contract-real`; OR (2) a list of specific test names (used by CHANGE SYSTEM BEHAVIOR when ACs dictate exact tests); OR (3) no filter — runs all tests.

### HIGH
- **Q6 — Port-change output→branch wiring:** ✓ **filled table.** Both `?` rows = **Implement DSL** (implementing the DSL may cause changes to driver ports — both system-driver and external-driver). Final table:

  | Producer (mid-level task) | Output variable | Consumer (high-level branch) |
  |---|---|---|
  | Write Acceptance Tests | `dsl-port-changed: bool` | WRITE TESTS BIG step 2 |
  | Implement DSL | `system-driver-ports-changed: bool` | WRITE TESTS BIG step 2.1.2 |
  | Implement DSL | `external-driver-ports-changed: bool` | WRITE TESTS BIG step 2.1.1 |

### PEAK
- **Q7 — Ticket lifecycle placement:** ✓ **A: peak-level wrapper** (marks IN PROGRESS → calls the peak entry → marks In Acceptance). AC/Checklists **not** ticked at this level.
- **Q8 — REFACTOR flows missing compile/verify:** ✓ **(i): add new `REFACTOR TESTS (BIG)` high-level orchestration** parallel to `WRITE SYSTEM (BIG)`. Steps: Refactor Tests → Compile Tests → Verify Tests Pass → Commit. Peak `REFACTOR TEST STRUCTURE` stays one-line ("Refactor Tests") and calls this orchestration. **`REFACTOR SYSTEM STRUCTURE` needs no change** — its "Write System" already calls `WRITE SYSTEM (BIG)` which includes compile + verify. Rationale: compile+verify discipline lives at high level, never at peak; mirrors `WRITE SYSTEM (BIG)` pattern.
- **Q9 — Backlog refinement integration:** ✓ **A: new peak entry REFINE BACKLOG** (sibling to the existing five).

### Follow-ups (resolved)
- **Q-ext — External system onboarding integration:** ✓ **(b): new peak entry `ONBOARD EXTERNAL SYSTEM`.** Standalone peak workflow. Rationale: flexibility for onboard-only tickets (e.g., adding a logging provider with no structural redesign) AND for redesign-that-includes-onboarding (REDESIGN SYSTEM STRUCTURE can call ONBOARD EXTERNAL SYSTEM as a sub-process). Coupling onboarding into REDESIGN would block modeling onboard-only tickets cleanly.

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
- **Session-handoff cadence: auto-commit, then surface `/clear` + `/execute-plan`.** Workflow for end-of-item handoff:
    1. **Auto-commit first** — do not ask the user for permission to commit at end-of-item; pre-approval is given here, in this standing constraint. Commit the item's changes (plan-file deletion + code/doc edits) with a surgical message via raw `git`. Without this commit, the next session's fresh agent sees uncommitted changes in `git status` with no context — high risk of work loss or accidental overwrite.
    2. **Then surface the literal next-session commands** (`/clear`, then `/execute-plan plans/20260525-1057-bpmn-refactor-design.md`) in a Next-steps block at the end of the response so the user knows the precise next step.
  Default cadence is `/clear` between items, not inline continuation — cached-prefix replay grows with every read/edit, so the natural seam is a `/clear`. (`feedback_offer_clear_then_execute_plan`, `feedback_execute_plan_always_next_steps`.)
- For agent-authored surgical commits with specific message + file list, use raw `git`, not `/commit` (`feedback_use_commit_skill`).
- Concurrent-agent collision risk — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
- Legacy **test artifacts** (files on disk) are indistinguishable from AT/CT artifacts — no folder, no annotation, no filename suffix (`feedback_legacy_tests_no_marker`). Whether the legacy **authoring cycles** themselves collapse into the normal flows or persist as distinct cycles is a separate doctrine call — **see Q16**, not a standing constraint.
