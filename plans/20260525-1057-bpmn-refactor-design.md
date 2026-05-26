# BPMN five-level refactor — design plan

> **This plan now holds the design record only.** Phase C + Phase D execution has moved to `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` (encode YAML + regenerate diagrams + Phase D handoff). This file is a pure design archive: Q&A debate (Q1–Q34 + Q-new + Q-ext), Decisions section, and cross-check inventory of the 21 existing diagrams. The execution plan references this archive for *why* behind any decision.

> **Working style: token-efficient.** Execute this plan in the cheapest form that still produces a quality result. If the user proposes a workflow that burns tokens unnecessarily (e.g., asking 8 questions individually via `AskUserQuestion` when 8 pre-drafted recommendations could be confirmed in one batch), **surface the cheaper alternative and let the user choose** — don't silently follow the costly path. The user has explicitly invited this pushback (memory: `feedback_flag_non_token_efficient`).

End-state-first: lock the design **before** drawing diagrams; the existing diagram generator (`gh optivem process show`) handles the drawing, so this plan never hand-draws Mermaid.

## Scope

### Intent: full replacement of the existing BPMN

*(Q17 resolved 2026-05-25: full replacement — option A.)*

The five-level structure (TOP / CYCLE / HIGH / MID / LOW — per Q26=A) is intended to **fully replace the existing BPMN**, not patch it. Concretely:

- The canonical source today is `internal/atdd/runtime/statemachine/process-flow.yaml` (read by the statemachine), rendered into `docs/process-diagram.md` plus the 21 SVGs under `docs/images/process-diagram-*.svg`.
- After this refactor lands, those artifacts are regenerated from — or removed in favour of — the new five-level structure. **Nothing from the old shape survives by accident.** Every retained behaviour is a deliberate decision recorded in this file.

### Cross-check vs existing BPMN

Because the replacement is full, every concern the existing BPMN models must be either **absorbed** into the new five-level structure (with location) or **dropped** (with reason). Inventory of the 21 existing diagrams:

**Maps cleanly into the five levels** — *all 15 absorption targets walked + confirmed 2026-05-25 (Item 6). Stale row wording updated to post-Q27 / post-Q32 names:*

| # | Diagram | Absorption target |
|---|---|---|
| 1 | legend | doc artifact, regenerate |
| 2 | ticket-lifecycle | TOP `implement-ticket` (Q26=A reframes Q7's peak-wrapper to the body of TOP) |
| 3 | github-intake | TOP `implement-ticket` — tied to ticket lifecycle |
| 5 | run-cycle | MID `run-tests` (Q5) |
| 6 | at-cycle | HIGH `write-and-verify-acceptance-tests-fail` ↔ `implement-and-verify-system` pairing |
| 7 | at-green-system | HIGH `implement-and-verify-system` |
| 8 | contract-test-sub-process | HIGH `implement-red-external-system-driver-adapters-contract-tests` |
| 10 | structural-cycle-shared | HIGH shared `implement-test-layer` |
| 11 | commit-sub-process | MID `commit` (calls `execute-command`) |
| 13 | at-refactor-system | **CYCLE `change-system-behavior` step 3** (loopable opportunistic refactor menu — calls CYCLE `refactor-system-structure` in opportunistic mode; see Q32 + Q33 resolutions) |
| 15 | compile | MID `compile` (calls `execute-command`) |
| 16 | da-cycle | CYCLE `redesign-system-structure` (Implement Driver Adapters — renamed per Q15) |
| 17 | green-phase-cycle | HIGH `implement-and-verify-system` |
| 20 | red-phase-cycle | HIGH `write-and-verify-acceptance-tests-fail` |
| 21 | sut-cycle | HIGH `implement-and-verify-system` |

**Legacy cycles** — *resolved 2026-05-25 via Q16=B (legacy cycles collapse). Three of the four absorb into `COVER SYSTEM BEHAVIOR`; #4 is dropped (no separate legacy-test-run operation exists):*

| # | Diagram | Resolution | Rationale |
|---|---|---|---|
| 4 | run-legacy-cycle | **DROP** (no absorption) | No separate "run legacy tests" operation. The mid-level `Run Tests` task runs whatever's currently in the test suite; legacy-vs-latest distinction doesn't exist at run time per Q16=B + memory `feedback_legacy_tests_no_marker`. |
| 12 | legacy-acceptance-criteria-cycle | **CYCLE `cover-system-behavior`** → HIGH `write-and-verify-acceptance-tests-pass` (Q31 Option D wrapper over parameterized core). | Writing ACs for existing behavior = adding coverage = COVER. |
| 18 | legacy-at-cycle | **CYCLE `cover-system-behavior`** → HIGH `write-and-verify-acceptance-tests-pass`. | Writing legacy ATs for existing behavior = COVER. |
| 19 | legacy-ct-cycle | **CYCLE `cover-system-behavior`** → HIGH `write-and-verify-acceptance-tests-pass` (with internal CT handling per Q31.a deferred). | Writing legacy CTs for existing behavior = COVER. |

**Not currently mapped** — *both included; resolutions recorded 2026-05-25:*

| # | Diagram | Resolution |
|---|---|---|
| 9 | external-system-onboarding | **CYCLE `onboard-external-system`** (Q-ext resolved to option b — standalone cycle; `redesign-system-structure` can also call it as a sub-process). |
| 14 | backlog-refinement | **CYCLE `refine-backlog`** (Q9 resolved to A — sibling to the existing five cycles). |

### Already-confirmed scope inclusions

- **Ticket intake and updates are in scope** (see Q7). Currently covered today by diagrams 2 (ticket-lifecycle) and 3 (github-intake); both must be absorbed.
- **Backlog refinement is in scope** (user flagged). Currently covered today by diagram 14; must be absorbed — see Q9.

## Inputs (brainstorm)

Brainstorm content has been fully absorbed into this design plan and into `internal/atdd/runtime/statemachine/process-flow.yaml`; the original `plans/ideas/` working files were retired 2026-05-26.

## Phases overview

- ~~**Phase A** (Item 1) — Batch-resolve the open questions in this file.~~ ✓ **Completed 2026-05-25.** All 23 questions (Q1–Q23 + Q-ext) resolved. See Decisions section.
- ~~**Phase B** (Items 2–6, 11, 12) — Apply decisions to the five brainstorm docs + walk the cross-check inventory + cross-file connectedness pass + Q-tag strip.~~ ✓ **Completed 2026-05-25** (Items 2–6 via commits up to `0ad5548`; Item 11 via commit `fac98ea`; Item 12 (Q-tag strip) absorbed and executed under `plans/20260525-1531-bpmn-ideas-contract-authoring.md`).
- **Phase C** (was Items 7–9) — Encode the new structure in `process-flow.yaml`, regenerate `docs/process-diagram.md`. **Moved** to `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`.
- **Phase D** (was Item 10) — Write the downstream-alignment plan (writing-agents, ATDD docs, retired SVGs). **Moved** to `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`.

## Items

No execution items remain. Items 1–6, 11, and 12 are resolved (deleted per the `/execute-plan` rule; Item 12's scope was absorbed into and executed under `plans/20260525-1531-bpmn-ideas-contract-authoring.md`). Items 7–10 were extracted to `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`. This file is now a pure design archive — `/execute-plan` should be invoked on the YAML-and-diagrams plan from here on.

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

**Context.** Items 2–5 "refine the LOW/MID/HIGH/PEAK brainstorm" by applying the resolved Q-decisions to the brainstorm files. This assumes those four files are the complete + correct source material for the new structure. If they're working drafts you'd revise independently, or if you have more brainstorm material not yet captured, Items 2–5's scope changes.

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
- **Q18 — Brainstorm files exhaustive vs working drafts:** ✓ **A: exhaustive + final.** Items 2–5 only apply Q-decisions to the brainstorm files; no new content added beyond what decisions imply.

### SCOPE
- **Q19 — Scope-inclusion attribution (ticket intake + backlog refinement):** ✓ **A: both confirmed.**

### PROCESS / NAMING
- **Q10 — Per-phase acceptance criteria:** ✓ **A: inline criteria per item** (already implemented in Items section).
- **Q11 — Commit cadence (git, for plan execution):** ✓ **A: one git commit per item.**
- **Q12 — Q6 prompt shape:** ✓ **A: propose-then-confirm.**
- **Q13 — Contract block format and location:** ✓ **A: contract blocks live in `process-flow.yaml` as `user_task` metadata** (`scopes:`, `outputs:`). Both consumers read from the same YAML: (1) the agent invocation uses `scopes:`/`outputs:` for prompt context + permitted file scope; (2) the post-execute BPMN verify step (currently EXECUTE AGENT step 3) reads the same `outputs:` to validate "required output variables present?" and `scopes:` to validate "scope constraints satisfied? (diff)". Single source of truth, no drift.
- **Q13.a — Per-task contract blocks in brainstorm (Phase B.2 follow-up):** ✓ **A: one illustrative block + YAML-truth note.** Only `Write Acceptance Tests` shown inline as illustration; YAML is single source of truth for all tasks. Avoids two-place drift. Per-task inline sketches rejected for the same reason. **Superseded 2026-05-25 by Q-new-4** — every task gets its full `Inputs:` / `Scopes:` / `Outputs:` / `Steps:` contract authored in the brainstorm files (the cheap design surface) before YAML encoding. Two-place drift is avoided by the brainstorm files being transient (deleted post-Phase-D), not by leaving them under-specified.
- **Q14 — `docs/process-diagram.md` structure:** ✓ **A: one file.**
- **Q15 — "Write" vs "Implement" naming:** ✓ **A: Write for tests, Implement for code.** Cycle entries get renamed: `Write System` → `Implement System`; `Write Driver Adapters` → `Implement Driver Adapters`.
- **Q15-HIGH extension (Phase B.3 follow-up):** ✓ **Yes — apply Q15 to HIGH orchestration names too.** `WRITE SYSTEM (BIG)` → `IMPLEMENT SYSTEM (BIG)`; step 1 `Write System` → `Implement System`. Tests stay as `Write`. Keeps vocabulary uniform across HIGH + CYCLE.
- **Q20 — Item decomposition (10-item shape):** ✓ **A: keep 10 items.**
- **Q21 — Phase D as separate downstream plan:** ✓ **A: separate plan.**
- **Q22 — Q6's table shape (producer/output/consumer):** ✓ **A: three-column table.**
- **Q23 — Rendering pipeline (`gh optivem process show` stays):** ✓ **A: existing pipeline stays.**
- **Q24 — Agent-naming doctrine (Phase B.2 follow-up):** ✓ **Verb-based, exact-match to MID task names.** Existing prompts under `internal/assets/runtime/prompts/atdd/` (noun-based cycle-phase identifiers like `at-red-test`, `at-red-dsl`, `ct-red-test`) get renamed to match MID task verbs (`write-acceptance-tests`, `implement-dsl`, `write-contract-tests`, etc.). The `agent-name:` YAML field likely renames to `task-name:` or `executor:` (decided in Phase D). Legacy `legacy-*` prompts collapse mechanically per Q16=B since task names are agnostic about expected test result. **Rename work deferred to Phase D's downstream-alignment plan (Item 10).** MID brainstorm carries a one-line breadcrumb.

### LOW
- **Q1 — Fix-loop recursion bounds:** ✓ **A: FIX as separate primitive** (4th low-level primitive, single attempt, terminates regardless of outcome).
- **Q1.a — FIX INPUT shape (Phase B.1 follow-up):** ✓ **A: single `Failure Context` payload.** Callers (EXECUTE AGENT, EXECUTE COMMAND) populate primitive-appropriate data — validation errors, missing/invalid outputs, scope-diff violations (agent) or command name + input params + stderr/exit code (command). Encoding deferred to Phase C YAML.
- **Q1.b — FIX approvals (Phase B.1 follow-up):** ✓ **A: PRE only.** POST is structurally meaningless — FIX is bounded to a single attempt and the caller's own validation re-runs after FIX returns; no FIX-POST loopback to gate. PRE is non-negotiable because FIX performs destructive ops (file edits, command runs).
- **Q1.c — FIX validates own output (Phase B.1 follow-up):** ✓ **A: no.** Caller owns re-validation (EXECUTE AGENT step 4 re-runs Validate Output & Scope after FIX returns). DRY — duplicating validation in FIX would require the same `outputs:`/`scopes:` to live in two YAML places.
- **Q2 — EXECUTE COMMAND post-approve symmetry:** ✓ **C: keep asymmetric** (commands have machine-checkable success; agents produce content needing human review).
- **Q3 — APPROVE NO-branch:** ✓ **A: APPROVE stays exit-only; caller owns NO branch.** **Action for Phase C:** when encoding APPROVE in `process-flow.yaml`, add a `TODO:` comment mentioning options B (parameterized NO-action) and C (two distinct primitives APPROVE-OR-EXIT, APPROVE-OR-RETRY) for possible revisit.
- **Q4 — Terminology:** ✓ **A: "calls"** (matches BPMN call-activity semantics).
- **Q-late-1 (2026-05-25, walk-feedback) — `approve` exit-symbol naming.** ✓ YES branch stays `END` (primitive returns to caller — same semantic as `fix` step 3's `END`); NO branch `HARD EXIT` → `END ERROR (because approval was not obtained)`. `END` = primitive returns to caller; `END ERROR` = primitive ends with error result, default caller behaviour is to re-propagate (which transitively kills the process). Asymmetric naming on purpose — Q3=A preserves `approve`'s reusability from inside `execute-agent` PRE/POST and `fix` PRE; if YES also meant "process exits successfully" the primitive could only be a terminal step.
- **Q-late-2 (2026-05-25, walk-feedback) — input-name / placeholder refinements in brainstorm.** ✓ Applied directly to the LOW brainstorm:
  - `approve.Inputs.prompt` → `question` (disambiguates from `execute-agent`'s prompt template — same word, different concept).
  - `fix.Inputs.failure-context` → `failure` (terser; "context" was redundant).
  - `execute-agent.Steps.3` sub-steps thread `[outputs]` / `[scopes]` bracket placeholders rather than narrating "the required output variables" abstractly (consistency with step 2's `[task-name]` / `[params]` style).
- **Q-late-3 (2026-05-25, walk-feedback) — `fix` calls `execute-agent`, with `fix-on-failure: false` flag.** ✓ Restructured. Reasoning:
  - **Reusability** — one agent-running primitive (`execute-agent`); `fix` is a thin wrapper that supplies the fix-agent's `task-name` and disables the recovery cascade.
  - **Mechanical loop prevention** — the "single attempt, no recursion" rule was a comment-only constraint in the original brainstorm; with `fix-on-failure: false` it becomes a YAML-enforced flag the runtime can't ignore.
  - **Changes:** `execute-agent` gains input `fix-on-failure` (default `true`); step 4 NO is conditional on `[fix-on-failure]` and passes `scopes` / `outputs` through to `fix`. `fix` gains inputs `task-name`, `scopes`, `outputs` (last two pass-through from failing task; omitted when caller is `execute-command`). `fix` step 2 calls `execute-agent` with `fix-on-failure=false`.
  - **Originally-open routing question — resolved 2026-05-25 by Q-late-5 below** (β-convention naming derivation); `task-name` input subsequently dropped from `fix`.
- **Q-late-4 (2026-05-25, walk-feedback) — `execute-agent.Inputs.prompt` → `params`.** ✓ Renamed. Per Q28.a the prompt FILE is derived from `task-name` (`task_name + ".md"`); the input `prompt` was redundant if read as "file reference". Reinterpreted as call-site template-substitution values yields `params` — the dynamic values that fill placeholders in the static prompt file at `task-name + ".md"`. Step 2 rewritten as `Run agent for task [task-name] [params]`. Name overlaps with `execute-command.Inputs.params` (CLI args) — different concepts, same word, scoped to each task; not worth a further rename.
- **Q-late-5 (2026-05-25, walk-feedback) — One generic `fix` agent vs multiple specialized `fix-*` agents; routing mechanism.** ✓ **(2) Multiple specialized + (β-convention) routing.**
  - **Multiple specialized `fix-*` agents** — one per failure kind: `fix-missing-output`, `fix-scope-diff`, `fix-command-failed`, `fix-unexpected-passing-tests` (Q28.b), `fix-unexpected-failing-tests` (Q28.b), and whatever else surfaces. Aligns with Q28.b doctrine ("verb-exact-match means distinct work = distinct prompt"); token-efficient at invocation (each prompt is small + focused); remediation logic per kind is genuinely distinct (revert out-of-scope edits vs augment re-run prompt vs diagnose stderr vs figure out why prod code is too lenient).
  - **(β-convention) routing via naming convention** — failure payload carries a `kind` field; the fix-* task-name is derived as `"fix-" + failure.kind`. No caller input needed; no separate routing table. Aligns with Q28.a doctrine (derive prompt paths from names, no explicit fields). Same logic that says "prompt path = `task-name + '.md'`" extends to "fix prompt path = `'fix-' + failure.kind + '.md'`."
  - **Supersedes Q-late-3's `task-name` input on `fix`** — input dropped (no longer needed since it's derived). `fix.Inputs` is now `failure`, `scopes`, `outputs`. Brainstorm simplified accordingly: `execute-agent` step 4 NO and `execute-command` step 3 NO call fix without specifying task-name; `fix` step 2 derives the inner `execute-agent` task-name as `"fix-" + [failure].kind`.
  - **Inventory deferred to Phase D** (exploration backlog): enumerate the closed set of failure-kinds and author/rename the matching `fix-<kind>.md` prompts. Not blocking the YAML encoding (Phase C) because the routing mechanism is convention, not table — Phase C encodes the convention; Phase D fills the inventory.

### MID
- **Q5 — Run Tests granularity:** ✓ **A (modified): single `Run Tests` task with polymorphic filter parameter.** Filter accepts: (1) a test-type tag — `acceptance` / `contract` / `acceptance-api` / `acceptance-ui` / `contract-stub` / `contract-real`; OR (2) a list of specific test names (used by CHANGE SYSTEM BEHAVIOR when ACs dictate exact tests); OR (3) no filter — runs all tests.
- **Q5.a — Run Tests filter encoding (Phase B.2 follow-up):** ✓ **A: abstract** in brainstorm (three forms documented, no commitment to encoding). Single-string-with-prefix vs structured discriminated union deferred to Phase C YAML.
- **Q25 — `Write Contract Tests` mid-level task (Phase B.2 follow-up):** ✓ **A: added to MID.** Symmetric with `Write Acceptance Tests`. Currently maps to `ct-red-test.md`; per Q24 renames to `write-contract-tests` in Phase D.

### HIGH
- **Q6 — Port-change output→branch wiring:** ✓ **filled table.** Both `?` rows = **Implement DSL** (implementing the DSL may cause changes to driver ports — both system-driver and external-driver). Final table:

  | Producer (mid-level task) | Output variable | Consumer (high-level branch) |
  |---|---|---|
  | Write Acceptance Tests | `dsl-port-changed: bool` | WRITE TESTS step 2 |
  | Implement DSL | `system-driver-ports-changed: bool` | WRITE TESTS step 2.1.2 |
  | Implement DSL | `external-driver-ports-changed: bool` | WRITE TESTS step 2.1.1 |
- **Q6.a — `<Expected Test Result>` threading (Phase B.3 follow-up):** ✓ **hoist to outer signature, drop from leaves.** It's already declared as INPUT at the top of WRITE TESTS; the prior leaf-only placement was incomplete propagation. Cleanest. **STILL VALID under Option D (Item 6, Q31)** — the parameterized core `write-and-verify-acceptance-tests` threads `<Expected Test Result>` from outer to leaves; the two thin wrappers `write-and-verify-acceptance-tests-fail` / `write-and-verify-acceptance-tests-pass` pin the parameter at the call site so CYCLEs invoke parameter-free names.
- **Q8.a — Add REFACTOR SYSTEM orchestration symmetric to REFACTOR TESTS? (Phase B.3 follow-up):** ✓ **A: no.** Q8 covers compile/verify inheritance for REFACTOR SYSTEM STRUCTURE via IMPLEMENT SYSTEM orchestration. A separate REFACTOR SYSTEM orchestration would duplicate.
- **Q8.b — REFACTOR TESTS INPUT/OUTPUT contracts (Phase B.3 follow-up):** ✓ **A: none.** Matches IMPLEMENT SYSTEM lean style; tests as INPUT/OUTPUT would be redundant for a refactor (input == output by definition).
- **Q31 — Test-writing orchestration: thin wrappers over parameterized core (Option D):** ✓ **resolved 2026-05-25 (Item 6).** Reconciles the prior `write-and-verify-red-tests` inconsistency AND Q-new-1's parameterization decision (commit `fac98ea`). Final shape:
  1. **Parameterized core `write-and-verify-acceptance-tests`** — kept exactly as fac98ea wrote it (single orchestration, `<Expected Test Result>` parameter threaded through step 1 + `implement-test-layer`). Single source of truth for the logic.
  2. **Two thin wrappers** added at HIGH:
     - `write-and-verify-acceptance-tests-fail` → calls `write-and-verify-acceptance-tests <Expected: Failure>`
     - `write-and-verify-acceptance-tests-pass` → calls `write-and-verify-acceptance-tests <Expected: Success>`
  3. **CYCLEs invoke the wrappers, not the core.** `change-system-behavior` step 1 calls `write-and-verify-acceptance-tests-fail`; `cover-system-behavior` step 1 calls `write-and-verify-acceptance-tests-pass`. CYCLE-level invocations are clean and self-documenting — no `<Expected Test Result>` parameter visible at the call site.
  
  Rationale: combines the best of "have both" (operator-facing names are explicit; mirror existing SHARED `verify-tests-pass` / `verify-tests-fail` naming) with the best of "parameterize" (single source of truth, no duplication). Inner orchestrations (`implement-and-verify-dsl`, etc.) stay parameterized via the existing core — they aren't called by CYCLEs, so wrappers there would add no value.
- **Q31.a — `cover-system-behavior` AT vs CT internal handling (deferred to Item 9):** how does `cover-system-behavior` distinguish AT-coverage from CT-coverage tickets? Options: (i) single CYCLE with internal nesting (mirrors fail-side's AT-with-CT-nested shape), (ii) single CYCLE invoked twice per ticket, (iii) two subtypes (`task/cover-legacy-acceptance`, `task/cover-legacy-contract`). Resolve when encoding `cover-system-behavior` in `process-flow.yaml`.
- **Q31.b — Side bug from fac98ea fixed:** `change-system-behavior` step 1 originally read `Write Tests <Expected Test Result: Success>` — Success was wrong (the change cycle writes failing tests). fac98ea already corrected this to `<Expected: Failure>`. With Option D wrappers, the CYCLE invocations no longer carry the parameter at all — the wrapper name (`write-and-verify-acceptance-tests-fail` / `write-and-verify-acceptance-tests-pass`) carries the expectation.
- **Q32 — Diagram #13 (at-refactor-system) absorption:** ✓ **resolved 2026-05-25 (Item 6).** The cross-check table previously mapped diagram #13 to CYCLE `refactor-system-structure`. That was wrong: diagram #13 depicts OPPORTUNISTIC post-GREEN refactor (per the `at-refactor-system.md` prompt content), not the ticket-driven cycle. New absorption target: **`change-system-behavior` step 3** (the loopable refactor menu from Q33). **Mechanism (a)** — step 3 calls existing CYCLEs in *opportunistic mode* (no checklist supplied; CYCLE degrades to "look at just-landed patch"). Reuses one CYCLE definition for both ticket-driven and opportunistic callers. The `at-refactor-system.md` prompt itself is **DROP** — folded into the CYCLE's opportunistic mode. **Q28.c "Phase D decides drop/retain for `at-refactor-system.md`" resolves to DROP.**

### TOP
- **Q30 — Ticket-type+subtype → CYCLE mapping:** ✓ **REVISED 2026-05-25 (Item 6).** Earlier framing said "ticket-type → change-type is not 1:1, classification is a judgment step." Superseded: ticket-type + optional `task` subtype label maps **1:1 to CYCLE via mechanical lookup**. No judgment at the gateway. Full table:

  | Ticket type / subtype | CYCLE |
  |---|---|
  | `story` | `change-system-behavior` |
  | `bug` | `change-system-behavior` |
  | `task/cover-legacy` | `cover-system-behavior` |
  | `task/redesign-system` | `redesign-system-structure` |
  | `task/refactor-system` | `refactor-system-structure` |
  | `task/refactor-tests` | `refactor-test-structure` |
  | `task/onboard-external-system` | `onboard-external-system` |

  Subtype validation is required for `task`: unknown subtypes **hard-exit at the gateway** (the operator must re-classify or refine the ticket).

- **Q30.a — Classification mechanism:** ✓ **A: explicit field.** The "field" is ticket-type + optional `task` subtype label (e.g., `story`, `task/refactor-system`). Gateway reads it mechanically — no human judgment at gateway time. Refinement (TOP `refine-ticket`) is where this metadata gets set during backlog grooming. Resolves the trade-off in favour of upfront discipline over judgment-at-gateway latency.

- **Q30.b — Multi-cycle tickets:** ✓ **A: single-cycle tickets only.** Multi-cycle work splits into separate tickets during refinement (e.g., a feature that requires a prior refactor = one `task/refactor-system` ticket + one `story`). Gateway stays purely mechanical: one classification → one cycle. Multi-cycle complexity belongs at backlog-refinement, not at the gateway.

- **Q33 — Refactor step in `change-system-behavior` (red-green-REFACTOR triad):** ✓ **resolved 2026-05-25 (Item 6).** Add a step 3 to `change-system-behavior`:
  ```
  3. Refactor (loopable):
     - refactor-system-structure  → call CYCLE (opportunistic mode, no checklist)
     - refactor-test-structure    → call CYCLE (opportunistic mode, no checklist)
     - redesign-system-structure  → call CYCLE (opportunistic mode, no checklist)
     - none                       → exit loop, continue to commit
  ```
  Classical TDD red-green-REFACTOR triad. Loopable matches reality (one refactor often suggests another). **Mechanism (a)** — step 3 calls existing CYCLEs as sub-processes in *opportunistic mode* (CYCLE accepts no-checklist invocation; degrades to "look at just-landed patch" semantics). Reuses one CYCLE definition for two callers (gateway + step 3). **Only `change-system-behavior` gets this step** — `cover` / `redesign-*` / `refactor-*` / `onboard` don't have a GREEN moment that triggers refactor. Not a Q30.b violation: refactor sub-step is INSIDE the change cycle's definition, not a separate gateway dispatch (ticket has one classification, cycle has internal structure).

- **Q34 — Ad-hoc refactor TOP process:** ✓ **resolved 2026-05-25 (Item 6).** Add a third TOP process named `refactor` (no `start-` prefix, no `ad-hoc-` prefix — concise; the scope is "I want to refactor without ticket overhead"). Body:
  ```
  refactor:
    1. Choose refactor type (loopable):
       - refactor-system-structure  → call CYCLE (opportunistic mode)
       - refactor-test-structure    → call CYCLE (opportunistic mode)
       - redesign-system-structure  → call CYCLE (opportunistic mode)
       - none                       → END
  ```
  No "mark ticket" bookends (no ticket exists). Coexists with **two other refactor surfaces**: (1) ticket-driven via `task/refactor-system` → `implement-ticket` gateway → CYCLE; (2) opportunistic-inside-change via Q33 step 3. Three surfaces, three ceremony levels. **Doesn't apply to `change` / `cover` / `onboard`** — each needs upfront ticket metadata (ACs / scope / target system).

### CYCLE
- **Q7 — Ticket lifecycle placement:** ✓ **A: cycle-level wrapper** (marks IN PROGRESS → calls the chosen cycle → marks In Acceptance). AC/Checklists **not** ticked at this level. **Superseded 2026-05-25 by Q26=A** — the wrapper is now the body of the top-level `implement-ticket` process; the cycles below it are pure per-ticket sub-processes.
- **Q7.a — TICKET LIFECYCLE invocation model (Phase B.4 follow-up):** ✓ **A: standalone wrapper section** with `<call the chosen cycle>` placeholder. Drawn once, applies to all. Not duplicated per cycle, not implicit framework concern. **Superseded 2026-05-25 by Q26=A** — the wrapper is now the body of the top-level `implement-ticket` process.
- **Q8 — REFACTOR flows missing compile/verify:** ✓ **(i): add new `REFACTOR TESTS` high-level orchestration** parallel to `IMPLEMENT SYSTEM`. Steps: Refactor Tests → Compile Tests → Verify Tests Pass → Commit. Cycle `refactor-test-structure` stays one-line ("Refactor Tests") and calls this orchestration. **`refactor-system-structure` needs no change** — its "Implement System" already calls `IMPLEMENT SYSTEM` orchestration which includes compile + verify. Rationale: compile+verify discipline lives at high level, never at cycle level.
- **Q9 — Backlog refinement integration:** ✓ **A: new cycle `refine-backlog`** (sibling to the existing five).
- **Q9.a — `refine-backlog` internal steps (Phase B.4 follow-up):** ✓ **A: accept tentative draft** (Read Backlog Items / Identify Gaps / Refine Ticket Descriptions / Refine Acceptance Criteria). Item 6 cross-check walk verifies against existing diagram 14.
- **Q-ext.a — `onboard-external-system` internal steps (Phase B.4 follow-up):** ✓ **A: accept tentative draft** (Identify / Document Contract / Set Up Access / Verify Reachable). Item 6 verifies against existing diagram 9.
- **Q-late-6 (2026-05-25, walk-feedback) — Drop named `Outputs:` from CYCLE and TOP definitions; set to `NONE`.** ✓ Applied to the CYCLE brainstorm (7 cycles) and the TOP brainstorm (`refine-ticket`, `implement-ticket`; `refactor` was already NONE). Reasoning: CYCLE per-ticket sub-processes return control to TOP's `implement-ticket` gateway, which doesn't consume a named return — it just runs `update-ticket` next. TOP processes have no caller at all. The previous Outputs (`modified-system-and-tests`, `new-tests`, `restructured-system`, `refactored-system`, `refactored-tests`, `refined-ticket`, `documented-external-system`/`reachable-external-system` at CYCLE; `ticket-state: READY` / `ticket-state: IN ACCEPTANCE` at TOP) were descriptive prose, not consumed contracts. Inline form `**Outputs:** NONE` per Q-new-4 editorial rule. **HIGH stays unchanged** — sibling HIGH steps inside a CYCLE consume each other's outputs (e.g., `change-system-behavior` step 2's verify side reads the test list authored by step 1; the Q6 `dsl-port-changed` / driver-port-changed booleans wire across HIGH siblings). HIGH `Outputs:` blocks are real dataflow and stay. **Open follow-up (not blocking):** inter-HIGH wiring at the CYCLE call site is currently implicit — step 2's call signature `(agent-action: implement-system)` does not reference the test list produced by step 1. The Q-new-4 convention ("intra-flow plumbing annotated inline on producer/consumer steps") covers it in principle; explicit `(reads <var> from step N)` annotations may be needed during the CYCLE walk.

### Follow-ups (resolved)
- **Q-ext — External system onboarding integration:** ✓ **(b): new cycle `onboard-external-system`.** Standalone cycle. Rationale: flexibility for onboard-only tickets (e.g., adding a logging provider with no structural redesign) AND for redesign-that-includes-onboarding (`redesign-system-structure` can call `onboard-external-system` as a sub-process). Coupling onboarding into REDESIGN would block modeling onboard-only tickets cleanly.

### Cross-file connectedness (resolved 2026-05-25 — Item 11 applies these)

- **Q-new-1 — "RED" in `write-and-verify-red-tests` for the COVER cycle:** ✓ **A: drop "red"; single parameterized HIGH.** Rename `write-and-verify-red-tests` → `write-and-verify-tests`. Both `change-system-behavior` (Expected: Failure) and `cover-system-behavior` (Expected: Success) call the same HIGH with different `<Expected Test Result>`. Aligns with Q5/Q16=B doctrine that expectation is a parameter, not a structural fork. Cascades to inner HIGH orchestrations: `write-red-acceptance-tests`, `implement-red-dsl-core`, `implement-red-system-driver-adapters`, `implement-red-external-system-driver-adapters`, `implement-red-external-system-driver-adapters-contract-tests` all drop "red" — they all parameterize via the shared `implement-test-layer` (which already takes `<Expected Test Result>`), so "red" was always misleading. **Supersedes the "red" portion of Q27** — the silver-canonical rename keeps the `-and-verify-` spine, drops "red".
- **Q-new-2 — `redesign-system-structure` step 1 "Implement Driver Adapters":** ✓ **A: two CYCLE sub-steps to existing MID tasks.** Step 1 splits into `1a implement-system-driver-adapters` + `1b implement-external-system-driver-adapters` (both MID-direct calls, modulo Q-new-3 rename). No new MID umbrella, no new HIGH wrapper. Rationale: the only consumer is REDESIGN; a one-off umbrella adds surface without reuse (per Q28.c "not recommended" note).
- **Q-new-3 — "drivers" vs "driver-adapters" vocabulary:** ✓ **A: rename MID to `-driver-adapters`.** `implement-system-drivers` → `implement-system-driver-adapters`; `implement-external-system-drivers` → `implement-external-system-driver-adapters`. Matches HIGH+CYCLE+hexagonal-architecture vocabulary. **Supersedes the corresponding rows in the Q28 prompt rename table** (`at-red-system-driver.md` → `implement-system-driver-adapters.md`; `ct-red-external-system-driver.md` → `implement-external-system-driver-adapters.md`; same for the collapse rows).
- **Q-new-6 — Acceptance-test HIGH naming: make scope visible in name (refines Q31 Option D):** ✓ **A: push specificity all the way down the 3-layer hierarchy.** The Q31 Option D shape (two thin wrappers over parameterized core, plus an inner test-code primitive) creates a duplication-feel because three HIGH tasks share a near-identical name stem (`write-and-verify-...`) while each layer carries a different scope. The asymmetry is **structurally earned** — acceptance is the only HIGH side that combines cycle-entry parameter hiding (outer) with a cascading DSL/adapter decision tree (middle); DSL/drivers stay flat siblings sharing `implement-test-layer`. But the naming hides the depth. Rename map:

  | Layer | Before (current) | After (Q-new-6) | What it does |
  |---|---|---|---|
  | Outer (cycle entry) | `write-and-verify-tests-fail` / `write-and-verify-tests-pass` | `write-and-verify-acceptance-tests-fail` / `write-and-verify-acceptance-tests-pass` | Pins `<Expected Test Result>`; calls middle. |
  | Middle (canonical full flow) | `write-and-verify-tests` | `write-and-verify-acceptance-tests` | Writes test code + cascading DSL/adapter impl if ports changed. |
  | Inner (test-code primitive) | `write-and-verify-acceptance-tests` | `write-and-verify-acceptance-test-code` | Pure: write test files, compile, verify, commit. No production-side impl. |

  Rationale:
  1. **Outer ↔ Middle share the root.** Suffix encodes the only thing the outer adds (the pinned parameter). Outer is genuinely a thin wrapper — the name promises what the code does.
  2. **Plural `tests` vs singular `test-code` carries semantic weight.** "Acceptance tests" (plural) = the full testing work including supporting DSL/adapters. "Acceptance test-code" (singular) = just the test source files themselves. A reader can guess the scope difference from the name alone.
  3. **Removes the misleading overlap.** Today's `write-and-verify-acceptance-tests` (inner) reads as if it's the canonical full flow; it is actually the no-impl subset. Demoting it to `-test-code` fixes that.
  4. **Doesn't propagate to DSL/drivers.** They have no 3-layer stack — single MID-level tasks (`implement-and-verify-dsl`, `implement-and-verify-system-driver-adapters`, `implement-and-verify-external-system-driver-adapters`) each call the SHARED `implement-test-layer`. No name-stem duplication to fix.

  **Alternatives considered and rejected:**
  - **Keep current names** (cheapest, asymmetric): preserves the duplication-feel; future readers will keep raising it.
  - **Collapse middle into outer**: duplicates the cascading decision tree between `-fail` and `-pass` wrappers.
  - **Drop the outer wrappers** (let CYCLE pass `<Expected Test Result>` directly): reverses Q31 Option D — reopens a settled doctrine question.

  **Touch-points (executed by `plans/20260525-1659-bpmn-acceptance-test-rename.md`):**
  - HIGH brainstorm — rename 3 task headings + internal call-site references.
  - CYCLE brainstorm — rename 2 invocation call sites (`change-system-behavior` step 1, `cover-system-behavior` step 1).
  - `plans/20260525-1057-bpmn-refactor-design.md` (this file) — update Q31 / Q31.b / Q6.a wording + cross-check inventory rows for diagrams #6, #20.
  - `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` — update "HIGH orchestrations (Q31 = D)" bullet wording.

  **Supersedes** the naming-only portion of Q31 Option D. The structural shape from Q31 (thin wrappers + parameterized core + inner primitive) is **unchanged** — only the names change.

### Brainstorm-file representation (resolved 2026-05-25 — applied by deleted plan `plans/20260525-1531-bpmn-ideas-contract-authoring.md`)

- **Q-new-4 — Uniform task template (Inputs/Scopes/Outputs/Steps):** ✓ **A: every task in the brainstorm files uses the same template** — LOW primitives, MID tasks, HIGH orchestrations, CYCLE per-ticket flows, TOP processes. **Supersedes Q13.a** ("one illustrative block + YAML-truth note") — every task now gets its full contract authored upfront in the cheap markdown surface so Phase C YAML encoding is mechanical translation. Template body:

  ```
  ## <task-name>

  **Inputs:**
  - ...

  **Scopes:** (agent tasks only — permitted file scope)
  - ...

  **Outputs:**
  - ...

  **Steps:**
  1. ...
  ```

  When a section is empty, collapse it to a single inline line instead of a one-item `- NONE` bullet:

  ```
  **Inputs:** NONE
  ```

  Rules:
  - Three core sections always shown (Inputs, Outputs, Steps); collapse to inline `**X:** NONE` when empty.
  - Agent tasks add a fourth `**Scopes:**` section listing permitted file scope. Non-agent tasks (commands, MID/HIGH/CYCLE/TOP orchestrations) omit it.
  - `Outputs:` lists operator-visible outputs only — the task's contract with its caller.
  - Intra-flow plumbing (e.g., `dsl-port-changed` consumed by a sibling step) is annotated inline on the producer/consumer steps (`(reads dsl-port-changed from step 1)`), not surfaced at the task level.

- **Q-new-5 — Editorial rules for brainstorm files:** ✓ **A: strip decision-rationale, historical, and reverse-cross-reference annotations.** Brainstorm files describe the resulting process; rationale lives here in the design plan. Strip from all five brainstorm files:
  1. **Decision-rationale parentheticals** — `(per Q13=A)`, `(Q-new-3)`, `(per Q28.a)`, `(resolved 2026-05-25)`, etc.
  2. **Historical refs** — `(was implement-system-drivers — renamed per Q-new-3)`, `(was at-red-test)`, etc.
  3. **Reverse cross-references** — `(called by HIGH implement-and-verify-system step 2)`, `Called by cover-system-behavior.`, etc. IDs are searchable; reverse lookups don't belong inline.
  4. **Top-of-file doctrine banners** — Q29 naming-convention banner, Q-new-1 doctrine resolved banner, Cross-file connectedness banner. Same category as #1.
  5. **Standalone `Note (Q...):` paragraphs** — same category as #1.
  6. **`TODO (Phase C revisit): ... (Q...)` lines** — same category as #1.

  Keep: the "Design content only" admonition banner at the top of each file (editor's instruction, not rationale).

### Naming doctrine (resolved 2026-05-25 in child plan `plans/20260525-1130-bpmn-naming-doctrine.md`)

The child plan locked four naming-doctrine decisions; full text + Discussion archive lives in that plan. Summary:

- **Q26 = (A) 5 levels with rename — TOP / CYCLE / HIGH / MID / LOW.** New TOP level holds the single `implement-ticket` process (Mark IN PROGRESS → classification gateway → call chosen CYCLE → Mark IN ACCEPTANCE). PEAK terminology dropped. Resolves Q7/Q7.a's TICKET LIFECYCLE wrapper — it IS the body of `implement-ticket`.
- **Q27 = (B) Silver-canonical rename to specificity at HIGH.** Colliding HIGH orchestrations renamed to describe their full composite scope: `implement-system` → `implement-and-verify-system`; `refactor-tests` → `refactor-and-verify-tests`; `write-tests` → `write-and-verify-red-tests`. No `_workflow`/`_subprocess` suffix — names describe scope, not layer (Standing constraint: no layer-coding).
- **Q28.a = (v) DROP the `agent-name:` field entirely.** Runtime contract change: `agent-name:` removed from `process-flow.yaml`. Runtime derives prompt path deterministically: `prompt_path(task_name) = task_name + ".md"` (task name already kebab-case per Q29, so the formula is identity). Errors at startup if file missing. Convention over configuration; eliminates double-data; eliminates layer-coding in YAML field names.
- **Q28.b = (i) SPLIT `fix-verify.md`** into `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md`. Token-efficient at invocation; doctrine consistency (verb-exact-match means distinct work = distinct prompt).
- **Q28.c (task-* principles).** (1) Drop `task-` prefix everywhere. (2) No `-redesign` cycle-context suffix — cycle is determined upstream by the gateway in `implement-ticket`. (3) `refactor-system.md` collision resolved by reading content (see resolution below).
- **Q29 = (C-kebab-unified) kebab-case everywhere.** Every process-model identifier uses kebab-case lowercase in YAML keys, doc headings, prompt filenames, in-prose references, anchor slugs, and Go struct tags. One rule, no two-layer split.

#### Q28 prompt rename table (locked — executed in Item 10's downstream-alignment plan)

Under Q28.a=DROP, the "Required filename" column is the source of truth — runtime derives it from the MID task name and errors at startup if the file is missing.

| File today | Maps to MID task | Required filename |
|---|---|---|
| `at-red-test.md` | `write-acceptance-tests` | `write-acceptance-tests.md` |
| `ct-red-test.md` | `write-contract-tests` (added Q25) | `write-contract-tests.md` |
| `at-red-dsl.md` + `ct-red-dsl.md` | `implement-dsl` (parameterized) | `implement-dsl.md` (one file, two callers) |
| `at-red-system-driver.md` | `implement-system-driver-adapters` (Q-new-3) | `implement-system-driver-adapters.md` |
| `ct-red-external-system-driver.md` | `implement-external-system-driver-adapters` (Q-new-3) | `implement-external-system-driver-adapters.md` |
| `at-green-system.md` | `implement-system` (HIGH `write-system` step 1, per Q15) | `implement-system.md` |
| `ct-green-external-system-stub.md` | `implement-external-system-stubs` | `implement-external-system-stubs.md` |
| `disable-tests.md` | `disable-tests` | `disable-tests.md` (no rename) |
| `enable-tests.md` | `enable-tests` | `enable-tests.md` (no rename) |
| `fix-verify.md` | `fix-unexpected-passing-tests` AND `fix-unexpected-failing-tests` | **SPLIT** (Q28.b): `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md` |
| `refine-acc.md` | `refine-acceptance-criteria` (CYCLE `refine-backlog` step 4) | `refine-acceptance-criteria.md` |
| `update-ticket.md` | `update-ticket` (TOP `implement-ticket` — Mark IN PROGRESS / Mark IN ACCEPTANCE) | `update-ticket.md` (no rename) |
| `at-refactor-system.md` | *opportunistic post-green refactor — folded into CYCLEs' opportunistic mode per Q32 resolution* | **DELETE** (Q32 resolves Q28.c (a) — folded into existing CYCLE invocations from `change-system-behavior` step 3 / TOP `refactor`) |
| `task-system-interface-redesign.md` | *Q28.c resolution below* | *Phase D resolves* |
| `task-external-system-interface-redesign.md` | *Q28.c resolution below* | *Phase D resolves* |
| `task-system-implementation-refactoring.md` | `refactor-system` (CYCLE `refactor-system-structure`) | `refactor-system.md` |
| `legacy-at-test.md` | (collapse → `write-acceptance-tests.md` per Q16=B) | **DELETE** |
| `legacy-at-dsl.md` | (collapse → `implement-dsl.md`) | **DELETE** |
| `legacy-at-system-driver.md` | (collapse → `implement-system-driver-adapters.md`) | **DELETE** |
| `legacy-ct-test.md` | (collapse → `write-contract-tests.md`) | **DELETE** |
| `legacy-ct-dsl.md` | (collapse → `implement-dsl.md`) | **DELETE** |
| `legacy-ct-external-system-driver.md` | (collapse → `implement-external-system-driver-adapters.md`) | **DELETE** |
| `legacy-ct-external-system-stub.md` | (collapse → `implement-external-system-stubs.md`) | **DELETE** |

#### Q28.c resolution (from reading prompt content)

**`task-system-implementation-refactoring.md` ↔ `at-refactor-system.md` collision** — **different scope, but Q32 collapses them into one CYCLE.**
- `task-system-implementation-refactoring.md` — ticket-driven internal refactor with checklist (REFACTOR SYSTEM STRUCTURE cycle). **Becomes canonical `refactor-system.md`.**
- `at-refactor-system.md` — opportunistic post-GREEN refactor that ran at end of the ATDD cycle. **RESOLVED 2026-05-25 via Q32 mechanism (a): DROP.** The CYCLE `refactor-system-structure` is invoked from two callers (ticket-driven via `implement-ticket` gateway, opportunistic via `change-system-behavior` step 3 / TOP `refactor`). In opportunistic mode the CYCLE accepts no-checklist invocation and degrades to "look at the just-landed patch / what's on the operator's mind" semantics. One CYCLE definition serves both ceremony levels; the opportunistic-only prompt is no longer needed.

**`task-system-interface-redesign.md` + `task-external-system-interface-redesign.md`** — **brainstorm-vs-prompt mismatch flagged for Phase D.**
- The CYCLE `redesign-system-structure` splits the work into two MID steps (`implement-driver-adapters`, `implement-system`), but the prompts handle the system change AND its driver-adapter absorption together (because the adapter must absorb the change to keep tests passing — splitting loses the coupling).
- **Recommended resolution:** keep the brainstorm split atomic, replicate the absorption discipline via the cycle's orchestration spec (call adapter-implementation immediately after system-surface-change in the same task or with shared state). The system-surface part folds into `implement-system.md`; the driver-adapter absorption part folds into `implement-system-drivers.md` (system side) and `implement-external-system-drivers.md` (external side). The currently-composite prompts are split during Phase D.
- **Alternative resolution:** collapse the brainstorm's two CYCLE steps into one composite MID task `reshape-system-surface-with-adapters` (or similar — scope-describing, not cycle-coded). The prompt stays composite. **Not recommended** — produces a one-off MID task only used by `redesign-system-structure`; the atomic split is reusable.
- Phase D picks (a) or alternative based on Item 6 cross-check.

---

## Re-running `/execute-plan`

No execution items remain on this plan. Phase C + Phase D continue on **`plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`** (Items 7–10 re-numbered 1–4 there, with a compact Decisions ledger). This file is now a design archive — Q&A debate, Decisions section, cross-check inventory — referenced from the execution plan for *why* behind any decision. No further `/execute-plan` invocations on this file.

## Exploration backlog

Open ideas surfaced during this plan but explicitly deferred — not blocking any current Item. Each is a future-exploration prompt; promote to an Item only when ready to land.

- **Spike ticket type.** The Q30 mapping table has no entry for `spike` — a ticket with that type would hard-exit at the `implement-ticket` gateway. Explore: is `spike` a recognized ticket type at all? Does it map to a cycle (e.g., a learning/research cycle that writes no production code)? Or does it sit outside `implement-ticket` entirely, perhaps as its own TOP process? *(Captured 2026-05-25 in Item 6 walk.)*
- **Cover ticket subtype split (Q31.a).** Should `task/cover-legacy` split into `task/cover-legacy-acceptance` and `task/cover-legacy-contract` to make the test-layer choice explicit? Currently one subtype, with `cover-system-behavior` expected to handle both AT and CT internally — but the internal mechanism is unresolved (deferred to Item 9). *(Captured 2026-05-25 in Item 6 walk.)*
- **`fix-*` agent inventory (Q-late-5).** Enumerate the closed set of failure-kinds (`missing-output`, `scope-diff`, `command-failed`, `unexpected-passing-tests`, `unexpected-failing-tests`, …) and author/rename the matching `fix-<kind>.md` prompts. (β-convention) routing in `fix` step 2 (`task-name = "fix-" + failure.kind`) presumes these prompts exist; Phase D should fill the inventory or specify behaviour for missing-prompt at runtime. Not blocking Phase C YAML encoding. *(Captured 2026-05-25 in walk-feedback.)*

## Standing constraints (from user memory)

- **Token-efficient by default** — flag any user-proposed workflow that burns tokens unnecessarily and offer a cheaper alternative (`feedback_flag_non_token_efficient`).
- **Session-handoff cadence: auto-commit, then surface `/clear` + `/execute-plan`.** Workflow for end-of-item handoff:
    1. **Auto-commit first** — do not ask the user for permission to commit at end-of-item; pre-approval is given here, in this standing constraint. Commit the item's changes (plan-file deletion + code/doc edits) with a surgical message via raw `git`. Without this commit, the next session's fresh agent sees uncommitted changes in `git status` with no context — high risk of work loss or accidental overwrite.
    2. **Then surface the literal next-session commands** (`/clear`, then `/execute-plan plans/20260525-1057-bpmn-refactor-design.md`) in a Next-steps block at the end of the response so the user knows the precise next step.
  Default cadence is `/clear` between items, not inline continuation — cached-prefix replay grows with every read/edit, so the natural seam is a `/clear`. (`feedback_offer_clear_then_execute_plan`, `feedback_execute_plan_always_next_steps`.)
- For agent-authored surgical commits with specific message + file list, use raw `git`, not `/commit` (`feedback_use_commit_skill`).
- Concurrent-agent collision risk — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
- Legacy **test artifacts** (files on disk) are indistinguishable from AT/CT artifacts — no folder, no annotation, no filename suffix (`feedback_legacy_tests_no_marker`). Whether the legacy **authoring cycles** themselves collapse into the normal flows or persist as distinct cycles is a separate doctrine call — **see Q16**, not a standing constraint.
