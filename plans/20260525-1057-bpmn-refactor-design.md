# BPMN four-level refactor — design plan

> **Working style: token-efficient.** Execute this plan in the cheapest form that still produces a quality result. If the user proposes a workflow that burns tokens unnecessarily (e.g., asking 8 questions individually via `AskUserQuestion` when 8 pre-drafted recommendations could be confirmed in one batch), **surface the cheaper alternative and let the user choose** — don't silently follow the costly path. The user has explicitly invited this pushback.

End-state-first: lock the design **before** drawing diagrams; draw diagrams **before** writing a migration plan. Token-efficient — prose Q&A is cheap, Mermaid blocks are expensive, so drawing is a single late verification pass.

## Scope

### Intent: full replacement of the existing BPMN

The four-level structure (LOW / MID / HIGH / PEAK) is intended to **fully replace the existing BPMN**, not patch it. Concretely:

- The canonical source today is `internal/atdd/runtime/statemachine/process-flow.yaml` (read by the statemachine), rendered into `docs/process-diagram.md` plus the 21 SVGs under `docs/images/process-diagram-*.svg`.
- After this refactor lands, those artifacts are regenerated from — or removed in favour of — the new four-level structure. **Nothing from the old shape survives by accident.** Every retained behaviour is a deliberate decision recorded in this file.

### Cross-check vs existing BPMN (Phase A and B)

Because the replacement is full, every concern the existing BPMN models must be either **absorbed** into the new four-level structure (with location) or **dropped** (with reason). Inventory of the 21 existing diagrams:

**Maps cleanly into the four levels (presumed during Phase B; verify each):**

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
| 11 | commit-sub-process | MID Commit (inherits EXECUTE COMMAND) |
| 13 | at-refactor-system | PEAK REFACTOR SYSTEM STRUCTURE (Q8 adds compile/verify) |
| 15 | compile | MID Compile (inherits EXECUTE COMMAND) |
| 16 | da-cycle | PEAK REDESIGN SYSTEM STRUCTURE (Write Driver Adapters) |
| 17 | green-phase-cycle | HIGH WRITE SYSTEM BIG |
| 20 | red-phase-cycle | HIGH WRITE TESTS BIG |
| 21 | sut-cycle | HIGH WRITE SYSTEM BIG |

**Legacy cycles — collapse into normal flows:**

| # | Diagram | Likely target |
|---|---|---|
| 4 | run-legacy-cycle | absorbed into CHANGE / COVER |
| 12 | legacy-acceptance-criteria-cycle | absorbed into CHANGE |
| 18 | legacy-at-cycle | absorbed into CHANGE |
| 19 | legacy-ct-cycle | absorbed into COVER |

Per memory `feedback_legacy_tests_no_marker`: legacy tests are indistinguishable from AT/CT tests, so the dedicated legacy cycles disappear in the new structure.

**Not currently mapped — needs decision:**

| # | Diagram | Gap |
|---|---|---|
| 9 | external-system-onboarding | No equivalent in the four brainstorm files. Needs new peak entry, or absorption into REDESIGN. **See Q14.** |
| 14 | backlog-refinement | No equivalent in the four brainstorm files. User explicitly flagged this as in-scope; likely a new peak entry above the existing five. **See Q9.** |

**During Phase A,** when a question touches an area that already has an existing diagram, the answer must declare whether the existing behaviour is absorbed (and where) or dropped (with reason). Add a "Cross-check vs existing" sub-bullet under each affected Decision.

### Already-confirmed scope inclusions

- **Ticket intake and updates are in scope** (see Q7). Currently covered today by diagrams 2 (ticket-lifecycle) and 3 (github-intake); both must be absorbed.
- **Backlog refinement is in scope** (user flagged). Currently covered today by diagram 14; must be absorbed — see Q9.

## Inputs (brainstorm)

The four files the user wrote, in `plans/ideas/`:

- `plans/ideas/1-bpmn-refactor-low-level.md` — **LOW:** APPROVE / EXECUTE AGENT / EXECUTE COMMAND primitives.
- `plans/ideas/2-bpmn-refactor-mid-level.md` — **MID:** parametrized agent tasks and command tasks that each `inherit` a low-level primitive.
- `plans/ideas/3-bpmn-refactor-high-level.md` — **HIGH:** orchestrations (WRITE TESTS BIG, WRITE RED ACCEPTANCE TESTS, IMPLEMENT RED DSL CORE, …, shared IMPLEMENT TEST LAYER / VERIFY TESTS PASS / VERIFY TESTS FAIL).
- `plans/ideas/4-bpmn-refactor-peak-level.md` — **PEAK:** workflow types (CHANGE SYSTEM BEHAVIOR, COVER SYSTEM BEHAVIOR, REDESIGN SYSTEM STRUCTURE, REFACTOR SYSTEM STRUCTURE, REFACTOR TEST STRUCTURE).

## Process (sequencing)

### Phase A — Resolve open questions  *(this file)*

Present all open questions with their pre-drafted recommendations **in one batched message**. Ask the user to either accept all recommendations or list the ones they'd like to change/discuss. Update the **Decisions** section in one pass. Estimated cost ~3K tokens. No diagrams yet.

Per memory `feedback_flag_non_token_efficient`, do not propose a one-question-at-a-time walk for this kind of structured question set with pre-draftable recommendations — it's ~8× more expensive for marginal benefit.

### Phase B — Refine the four brainstorm docs

Edit `plans/ideas/1-bpmn-refactor-low-level.md` … `plans/ideas/4-bpmn-refactor-peak-level.md` in place so they match the Phase A decisions. During this phase, **also walk the cross-check inventory** above and verify each presumed-mappable diagram actually fits. If a diagram doesn't map, surface a follow-up question rather than guessing. Still no diagrams in this phase.

### Phase C — Update YAML + regenerate diagrams

**Do not hand-draw Mermaid.** `docs/process-diagram.md` and the SVGs under `docs/images/process-diagram-*.svg` are **generated** from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. The top of `docs/process-diagram.md` says explicitly *"Do not edit by hand — edit the YAML and regenerate via `gh optivem process show > docs/process-diagram.md`"*.

Phase C uses this mechanism rather than duplicating it:

- **C.1 — Prototype.** Encode one peak entry in YAML (recommended: REFACTOR SYSTEM STRUCTURE — simplest). Regenerate. Inspect output. Verify it matches the refined brainstorm docs.
- **C.2 — Generator / schema changes if needed.** The existing schema (`service_task`, `user_task`, `gateway`, `call_activity` with `params:`, `sequence_flows` with `when:`) covers most of the new structure. Only extend if Phase A decisions require it — e.g., adding `scopes:` / `outputs:` metadata to `user_task` for contract blocks (see Q13).
- **C.3 — Migrate rest of YAML.** Encode the remaining peak entries + high orchestrations + mid `call_activity` definitions. Regenerate. Diff against the previous `docs/process-diagram.md` to confirm nothing intended-to-survive is missing (use the cross-check inventory).

Phase C's outputs: updated `process-flow.yaml` and regenerated `docs/process-diagram.md`. **No hand-drawn `5-bpmn-refactor-end-state.md` file.**

### Phase D — Downstream alignment  *(separate file, future)*

Once Phase C is locked (YAML + regenerated diagrams reflect the new structure), write a real plan `plans/<timestamp>-bpmn-refactor-downstream.md` that handles artifacts not produced by `gh optivem process show`:

- **Writing-agents** — names, scopes, output schemas may need updates to match Q1 (FIX as new agent), Q4 (terminology), Q5 (Run Tests parameterization).
- **ATDD docs** — `docs/atdd/process/*.md`, `docs/atdd/architecture/*.md` and any other file that references the old structure or hand-drawn SVGs.
- **Retired SVGs** — any `docs/images/process-diagram-*.svg` from the legacy set that no longer corresponds to a process in the regenerated `docs/process-diagram.md`.

(Note: the original plan had a one-slice prototype as Phase D's first item — that role moved into Phase C.1, so the prototype-before-bulk safety net is implicit in C.)

---

## Open questions (Phase A backlog)

Each question must be resolved before Phase B begins. Walk in order. One `AskUserQuestion` per turn.

**Sequencing note.** The PROCESS questions (Q10–Q13) govern *how* Phase A itself is executed (acceptance criteria, commit cadence, prompt shape, contract format). Walk them **first**, before the design questions (Q1–Q9). Q12 is the one exception — it only matters when you reach Q6, so it can be deferred to that point.

### Q1 — Fix-loop recursion bounds  *(LOW)*

**Context.** In `1-bpmn-refactor-low-level.md`, EXECUTE AGENT step 4 says *"Valid? NO: Fix Agent Run (AGENT)"*. Fix is itself an agent run, so a fix that also fails validation would loop forever. The same shape repeats in EXECUTE COMMAND step 3. The file's top FUTURE IDEA note already hints: *"For FIX, currently one task. Maybe 2, to separate planning vs execution, so that human approves…"*.

**Options.**
- **(A) FIX is a separate primitive.** Its own contract, single attempt, terminates regardless of outcome (human approves via the surrounding gate). Adds a 4th low-level primitive. Cleanest separation, no recursion possible.
- **(B) Bounded retry with human escalation.** Fix runs as EXECUTE AGENT up to N attempts; on all-fail, prompt human (continue / abort / manual-fix). Keeps 3 primitives, adds retry-count config and a tri-state human prompt.
- **(C) Single retry, then HARD EXIT.** Fix runs once; if it still fails, exit. Simplest but brittle.

**Recommendation: A.** Different semantics → different primitive (EXECUTE AGENT does work, FIX repairs failed work). Eliminates recursion entirely — no retry counter, no max-N, no "what if fix fails to fix" edge. Matches the FUTURE IDEA's plan/execute split, which is incompatible with FIX = EXECUTE AGENT with a different prompt. This is a teaching repo, so clarity > 3-primitive minimalism.

### Q2 — EXECUTE COMMAND post-approve symmetry  *(LOW)*

**Context.** EXECUTE AGENT has both `Approve (PRE)` and `Approve (POST)`. EXECUTE COMMAND only has `Approve (PRE)`. Asymmetric — needs justification or alignment.

**Options.**
- **(A) Add `Approve (POST)` to EXECUTE COMMAND.** Symmetric, defensive.
- **(B) Remove `Approve (POST)` from EXECUTE AGENT.** Symmetric, lean. Validation step already exists in EXECUTE AGENT — is the post-approve redundant?
- **(C) Keep asymmetric.** Justify: agent output is reviewed for scope/correctness; command success-or-fail is binary so post-approve adds no signal.

**Recommendation:** Probably **C** but worth confirming. Commands have machine-checkable success (exit code); agents produce content that needs human-eye review even after machine validation passes scope/output checks.

### Q3 — APPROVE NO-branch (exit vs retry)  *(LOW)*

**Context.** APPROVE currently only models hard-exit on NO. But the post-approve inside EXECUTE AGENT, on a NO, probably means *"re-run the agent,"* not *"kill the whole flow."* APPROVE's contract is too narrow for both callers.

**Options.**
- **(A) APPROVE stays single-shape (exit-only); callers handle the NO branch themselves** by wrapping retry logic around it. Simplest primitive.
- **(B) APPROVE gains a parameterized NO-action** (`exit` | `retry-caller`). One primitive, two modes.
- **(C) Two distinct primitives: APPROVE-OR-EXIT and APPROVE-OR-RETRY.** Cleanest semantics, adds a primitive.

**Recommendation:** **A** (callers own the branch). APPROVE remains a pure human-gate primitive; control flow on NO belongs to the caller. Aligns with the principle that primitives should be single-purpose.

### Q4 — Terminology: "inherits" vs "calls"/"instantiates"  *(LOW)*

**Context.** `2-bpmn-refactor-mid-level.md` says agent tasks *"inherit the low level EXECUTE AGENT task"*. BPMN doesn't have inheritance — it has **call activities** and **subprocesses**. The mental model is parametrized invocation, not OO inheritance.

**Options.**
- **(A) "calls"** — matches BPMN call-activity semantics. Standard BPMN vocabulary.
- **(B) "instantiates"** — emphasizes parametrization.
- **(C) "invokes"** — neutral and common in process modelling.
- **(D) Keep "inherits"** — justify with a definition note.

**Recommendation: A.** "Calls" is the canonical BPMN term and unambiguous to anyone with process-modelling background.

### Q5 — Run Tests granularity  *(MID)*

**Context.** `2-bpmn-refactor-mid-level.md` says: *"Run Tests (it can run all tests or we pass some filter to be selective... not sure if one task or multiple)"*. `3-bpmn-refactor-high-level.md` is literally cut off mid-sentence on this same point. The ambiguity will propagate up.

**Options.**
- **(A) Single Run Tests task with required filter parameter.** Filter can be "all" or a selector.
- **(B) Two tasks: Run All Tests, Run Filtered Tests.** Explicit at the cost of a near-duplicate.
- **(C) Three tasks split by test type:** Run Acceptance, Run Contract, Run Unit. Matches the existing process where each test type has distinct semantics.

**Recommendation:** Probably **A** for the mid-level (single parametrized task), with the **filter expressed in terms of test type** (acceptance / contract / unit / all). Splitting into separate mid-level tasks (C) duplicates the EXECUTE COMMAND wrapping; passing the type as a parameter is the natural extension of the "inherits EXECUTE COMMAND" pattern. Worth confirming.

**Cross-check.** Existing diagram 5 (run-cycle) is the absorption target. Verify the parameterized model can express what run-cycle currently does.

### Q6 — Port-change output→branch wiring  *(HIGH)*

**Context.** `3-bpmn-refactor-high-level.md` branches on *"DSL Port Changed?"* and *"External System Driver Ports Changed?"*. These depend on outputs produced by prior mid-level tasks. `2-bpmn-refactor-mid-level.md` hints at one: *"outputs: dsl-port-changed: true/false"* on Write Acceptance Tests. The output→branch wiring needs to be made explicit: which mid-level task produces which output variable, and which high-level branch consumes it.

**Decision form.** Not a multi-option question — needs a table.

| Producer (mid-level task) | Output variable | Consumer (high-level branch) |
|---|---|---|
| Write Acceptance Tests | `dsl-port-changed: bool` | WRITE TESTS BIG step 2 |
| ? | `system-driver-ports-changed: bool` | WRITE TESTS BIG step 2.1.2 |
| ? | `external-driver-ports-changed: bool` | WRITE TESTS BIG step 2.1.1 |

User to fill in the remaining producers (likely Implement DSL or a dedicated detection step). This question may be parked until after Q5 — the test-running shape can affect where these outputs are observed. **See also Q12 — Q6's prompt shape is itself a Phase A process decision.**

### Q7 — Ticket lifecycle placement  *(PEAK)*

**Context.** `4-bpmn-refactor-peak-level.md` opens with: *"Maybe common for all, as entry point before they start is marking the ticket IN PROGRESS, and when they finish to mark as In Acceptance."* Cross-cutting concern, currently unstructured. Also notes: *"I don't know if Acceptance Criteria and Checklists should be ticked or not, or I ignore them."*

**Scope confirmation (from user).** Ticket intake and updates **are in scope** — the question is *how* to structure them, not *whether* to include them.

**Options.**
- **(A) Wrapper at peak level** — a meta-process that marks IN PROGRESS → calls the peak entry → marks In Acceptance. Each peak entry stays focused on its own work.
- **(B) New low-level primitive UPDATE TICKET STATUS** invoked from each peak entry as steps 1 and N.
- **(C) Per-peak-entry steps** — replicate the marking inside each peak entry.

**Sub-question.** Are Acceptance Criteria / Checklists ticked off as part of the flow, or ignored at this level?

**Cross-check.** Diagrams 2 (ticket-lifecycle) and 3 (github-intake) are the absorption targets. Whichever option wins must subsume both. Github-intake in particular may pull in some pre-work (e.g., ticket parsing) that doesn't fit a pure mark-status wrapper — verify during Phase B.

**Recommendation:** **A** (peak-level wrapper). Cross-cutting concerns belong outside the work they wrap; both B and C scatter the lifecycle handling. On the sub-question, default to **not ticking AC/Checklists** at this level — those are tracking artifacts orthogonal to the BPMN flow — unless the user disagrees.

### Q8 — REFACTOR flows missing compile/verify steps  *(PEAK)*

**Context.** In `4-bpmn-refactor-peak-level.md`, REFACTOR SYSTEM STRUCTURE has only *"Write System"*. REFACTOR TEST STRUCTURE has only *"Refactor Tests"*. Neither includes compile or verify-tests-pass. Real refactors need both — the test suite must still pass after the refactor.

**Options.**
- **(A) Add explicit Compile + Verify-Tests-Pass to both refactor peaks.** Steps surface at peak level.
- **(B) Add a single Verify step that subsumes compile + verify-tests-pass.** Hides the detail but matches user mental model ("did the refactor break anything?").
- **(C) Leave it implicit — argue Write System / Refactor Tests at the high level already contain compile/verify.** Risky: peak readers won't see it.

**Recommendation: A.** Refactor without verify-pass is meaningless; making it explicit at peak teaches the discipline. Same applies to the non-refactor peaks (CHANGE / COVER / REDESIGN) for consistency — but those already terminate in tests passing via their underlying Write System (BIG), so the peak-level shape is already correct there.

### Q9 — Backlog refinement integration  *(PEAK, new — from cross-check)*

**Context.** The existing BPMN models backlog refinement (`docs/images/process-diagram-14-backlog-refinement.svg`). The user has confirmed refinement is in-scope for the refactor, but none of the four brainstorm files mention it. We need a peak entry (or wrapper) that absorbs it.

**Options.**
- **(A) New peak entry: REFINE BACKLOG.** Sibling to CHANGE / COVER / REDESIGN / REFACTOR-SYSTEM / REFACTOR-TEST. Self-contained workflow: read ticket → break down → write Gherkin → return refined ticket.
- **(B) Pre-peak meta-wrapper** — refinement is a separate entry point that runs *before* any of the existing five peak entries (e.g., before CHANGE SYSTEM BEHAVIOR). Models the "refine then implement" sequencing explicitly.
- **(C) Fold into Q7's ticket-lifecycle wrapper** — refinement is part of the intake side and belongs in the wrapper, not as a peak entry of its own.

**Cross-check.** Existing diagram 14 is the absorption target. Whatever option wins must subsume what that diagram models. Read the SVG and `process-flow.yaml` for the refinement nodes before answering.

**Recommendation:** **A** (new peak entry). Refinement is a distinct activity with its own inputs/outputs — it produces a refined ticket, not a system change. Treating it as a peak sibling makes it teachable as its own thing rather than smuggling it inside intake or change-behaviour.

### Q10 — Per-phase acceptance criteria  *(PROCESS)*

**Context.** The Process section defines Phases A/B/C/D but doesn't say when each is "done". Without explicit criteria, the new session has to judge.

**Options.**
- **(A) Inline acceptance criteria per phase** (e.g., Phase A done when every Decision has status ≠ pending; Phase B done when every brainstorm doc reflects every Decision and has zero contradictions vs Decisions; Phase C done when every PEAK and HIGH entry has a Mermaid diagram + contract block with no unresolved branch labels or undeclared outputs).
- **(B) Single checklist** appended to this file, checked off as you go.
- **(C) Leave implicit** — rely on agent judgment.

**Recommendation: A.** Inline criteria sit next to the phase they govern, so they're impossible to miss.

### Q11 — Commit cadence  *(PROCESS)*

**Context.** The plan is silent on when to commit during Phases A/B/C. Risk: either too granular (8 commits in Phase A) or too coarse (one big commit at the end loses checkpointing).

**Options.**
- **(A) One commit per phase boundary** — end of Phase A (decisions locked), end of Phase B (docs refined), end of Phase C (diagrams drawn). Three commits.
- **(B) One commit per resolved question in Phase A**, then per-phase for B and C. ~13 commits in Phase A alone.
- **(C) Single commit at the end of A+B+C combined.** Risky — loses the ability to revert one phase.

**Recommendation: A.** Phase boundaries are the natural rhythm and produce readable history without commit churn.

### Q12 — Q6 (port-change wiring) prompt shape  *(PROCESS)*

**Context.** Q6 needs a table filled in, not a multi-option choice. `AskUserQuestion`'s structured 2–4-option shape doesn't fit. The new session needs a strategy.

**Options.**
- **(A) Agent drafts a complete table** from a best read of the brainstorm files, then asks user via one `AskUserQuestion` with options like "looks right" / "needs edits" / "skip — handle in Phase B". Propose-then-confirm.
- **(B) Free-form conversation** — agent asks each row separately (Producer? Output? Consumer?) and fills the table iteratively. Several turns.
- **(C) Skip the table, resolve via narrative description** in the Decisions section.

**Recommendation: A.** Propose-then-confirm has lower cognitive load than blank-table iteration; the brainstorm files already constrain the answer space.

### Q14 — `docs/process-diagram.md` structure: one file or multiple  *(PROCESS / Phase C)*

**Context.** Today `docs/process-diagram.md` is one scrolling file containing all process sections (as generated). The new four-level structure could plausibly split into multiple files (e.g., one per level: `low.md`, `mid.md`, `high.md`, `peak.md`) for clearer navigation.

**Options.**
- **(A) Keep one file.** Zero migration, matches current generator behaviour, cross-process navigation via in-page anchors. Best for top-to-bottom reading (teaching audience).
- **(B) Split by level** — `process-diagram-low.md`, `…-mid.md`, `…-high.md`, `…-peak.md`. Mirrors the new structure. Requires generator change to emit multiple files + an index. Adds navigation friction (Ctrl-F across files needs a multi-file editor).
- **(C) Split by process** (one file per named process in YAML). Finest granularity, ugliest navigation, most files.
- **(D) Mixed:** index file (`process-diagram.md`) + per-level files. Best UX, most generator complexity.

**Recommendation: A** (keep one file). Lowest cost, no generator change, matches current. Defer split to Phase D *only* if the regenerated file is genuinely unwieldy. Splitting is a presentation concern, easy to revisit later — don't pay the cost preemptively.

### Q13 — Contract block format and location  *(PROCESS)*

**Context.** "Contract blocks" describe each node's `agent-name` / `scopes` / `outputs` etc. — see `2-bpmn-refactor-mid-level.md` for the existing style. With Phase C now driven by `process-flow.yaml` regeneration rather than hand-drawn diagrams, the question splits in two: (i) what's the format, and (ii) where does it live?

**Options.**
- **(A) Format: match mid-level brainstorm style. Location: in `process-flow.yaml` as node-level metadata** (extend the `user_task` schema with `scopes:`, `outputs:`). Generator emits them next to each diagram. Single source of truth — no drift. Requires a small schema/generator extension (Phase C.2).
- **(B) Format: match mid-level brainstorm style. Location: in the refined brainstorm docs only**, separate from YAML. YAML stays minimal. Risk: drift between docs and YAML over time.
- **(C) Richer schema** (`inputs`, `outputs`, `scopes`, `side-effects`, `error-modes`, `human-gates`) co-located in YAML. Most rigorous, biggest generator change.

**Recommendation: A.** Co-locating contract metadata in YAML eliminates drift and gives the generator a single source for both the diagram and the contract documentation. The schema extension is small and within scope for Phase C.2.

---

## Cross-check follow-up (only if confirmed needed)

- **Q14 — External system onboarding integration.** Existing diagram 9 (`process-diagram-9-external-system-onboarding-sub-process.svg`) is not currently mapped to anything in the four brainstorm files. Likely absorbs into REDESIGN SYSTEM STRUCTURE (since onboarding produces new driver adapters), but worth a discrete decision. Not added to the main backlog because the user did not explicitly flag it — surface as a question only if Phase B reveals a real gap.

---

## Decisions

*(filled in as each question is resolved during Phase A)*

### PROCESS *(walk first)*
- **Q10 — Per-phase acceptance criteria:** *(pending — recommended A: inline criteria per phase)*
- **Q11 — Commit cadence:** *(pending — recommended A: one commit per phase boundary)*
- **Q12 — Q6 prompt shape:** *(pending — recommended A: propose-then-confirm)*
- **Q13 — Contract block format and location:** *(pending — recommended A: in YAML as `user_task` metadata)*
- **Q14 — `docs/process-diagram.md` structure:** *(pending — recommended A: keep one file)*

### LOW
- **Q1 — Fix-loop recursion bounds:** *(pending — recommended A: FIX as separate primitive)*
- **Q2 — EXECUTE COMMAND post-approve symmetry:** *(pending — recommended C: keep asymmetric, justified)*
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
- **Q-ext — External system onboarding integration:** *(only if Phase B reveals a real gap; previously labelled Q14 — renumbered when the diagram-structure question took that slot)*

---

## How to resume in a new session

1. Read this file.
2. Read the four brainstorm inputs under `plans/ideas/`.
3. Skim the existing BPMN: `internal/atdd/runtime/statemachine/process-flow.yaml` (the top documents the YAML schema) and `docs/process-diagram.md` (the top documents the generator command). Open any `docs/images/process-diagram-*.svg` referenced in the cross-check table that you want fresh context on.
4. Execute **Phase A** — walk **Q10 → Q11 → Q13** first (PROCESS questions; Q12 is deferred to when Q6 is reached), then **Q1 → Q9** in order. One `AskUserQuestion` per turn. Update each entry's status in the Decisions section as you go.
5. When all are resolved, run **Phase B** (refine the brainstorm docs in place; also walk the cross-check inventory and verify each presumed-mappable diagram fits).
6. Then **Phase C** — update `process-flow.yaml`, run `gh optivem process show > docs/process-diagram.md`, inspect output. Do NOT hand-draw Mermaid. Generator/schema changes go in C.2 if needed (e.g. adding contract metadata per Q13).
7. **Phase D** is a separate plan covering downstream alignment: writing-agents, ATDD docs, retired SVG cleanup.

Standing constraints (from user memory):
- **Token-efficient by default** — flag any user-proposed workflow that burns tokens unnecessarily and offer a cheaper alternative (`feedback_flag_non_token_efficient`).
- For agent-authored surgical commits with specific message + file list, use raw `git`, not `/commit` (`feedback_use_commit_skill`).
- Concurrent-agent collision risk — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
- Legacy tests/diagrams collapse into AT/CT, not preserved as separate flows (`feedback_legacy_tests_no_marker`).
