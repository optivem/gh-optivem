# BPMN naming-doctrine + TOP-level reframe — design plan

## End state (what this plan delivers)

After Items 1–5 execute, the BPMN brainstorm and runtime end up as:

**5-level process model.** Layer labels (organizational categories, not part of any identifier):

| Layer | What it is | Example identifier |
|---|---|---|
| **TOP** | The single top-level process invoked per ticket | `implement-ticket` |
| **CYCLE** | Per-ticket sub-processes selected by classification gateway | `change-system-behavior`, `cover-system-behavior`, `redesign-system-structure`, `refactor-system-structure`, `refactor-test-structure`, `refine-backlog`, `onboard-external-system` |
| **HIGH** | Orchestrations of MID tasks, named for their full composite scope | `implement-and-verify-system`, `refactor-and-verify-tests`, `write-and-verify-red-tests` *(name TBD Item 2)* |
| **MID** | Discrete agent-executable tasks; one MID task → one prompt file | `write-acceptance-tests`, `implement-dsl`, `implement-system`, `fix-unexpected-passing-tests`, `fix-unexpected-failing-tests` |
| **LOW** | Atomic sub-steps inside a MID task | (per existing `plans/ideas/1-*.md`) |

**Brainstorm file layout.**
- `plans/ideas/1-bpmn-refactor-low-level.md` — unchanged structure; kebab-case applied.
- `plans/ideas/2-bpmn-refactor-mid-level.md` — unchanged structure; kebab-case applied.
- `plans/ideas/3-bpmn-refactor-high-level.md` — Q27 collision-renames applied; kebab-case applied.
- `plans/ideas/4-bpmn-refactor-cycle-level.md` — renamed from `4-bpmn-refactor-peak-level.md`; `(WRAPPER) TICKET LIFECYCLE` removed (migrated to TOP).
- `plans/ideas/5-bpmn-refactor-top-level.md` — **new**; contains `implement-ticket` body (Mark IN PROGRESS → classification gateway → call chosen CYCLE → Mark IN ACCEPTANCE).

**Single naming rule across every layer.** Every process-model identifier (TOP / CYCLE / HIGH / MID / LOW) is **kebab-case lowercase** in every appearance — YAML keys (`process-flow.yaml`), doc headings, prompt filenames, in-prose references, anchor slugs, Go struct tags. No two-layer split, no ALL CAPS, no snake_case, no camelCase. Layer labels (TOP / CYCLE / HIGH / MID / LOW) are NOT part of identifier names — no `_workflow` / `_subprocess` / `high.*` / `_cycle` suffixes or namespaces.

**Runtime contract change (Q28.a).** The `agent-name:` field in `process-flow.yaml` is DROPPED entirely. Runtime derives prompt path deterministically: `prompt_path(task_name) = task_name + ".md"`. Convention over configuration; runtime errors at startup if the file is missing. Eliminates double-data between task name and prompt reference.

**Prompt file renames (Q28 table; full spec in Decisions section).**
- Verb-based exact-match: every MID task `X` has a prompt at `X.md`.
- `fix-verify.md` → SPLIT into `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md`.
- All `legacy-*` prompts DELETED (collapse into verb-based prompts per Q16=B).
- Three `task-*` prompts: `task-` prefix dropped, `-redesign` cycle suffix eliminated, `refactor-system.md` collision resolved by reading prompt content during Item 3.

---

> **How to use this plan.** Open in a fresh `/clear`-ed chat. Use `/refine-plan plans/20260525-1130-bpmn-naming-doctrine.md` to walk the questions and lock decisions (the four questions Q26–Q29 below). Then `/clear` again and `/execute-plan` to apply the locked decisions to the brainstorm files + record in Decisions. Phase C YAML migration in the parent plan (`plans/20260525-1057-bpmn-refactor-design.md`) depends on this plan landing first.

## Context

This plan was spun off from `plans/20260525-1057-bpmn-refactor-design.md` (Phase B.3 / B.4 — HIGH and PEAK brainstorm fix-ups) during the 2026-05-25 session. The parent session resolved 11 medium-tentative fix-ups and committed Items 2–5 (LOW / MID / HIGH / PEAK brainstorms refined). During that work, three substantial doctrine questions surfaced that warranted their own session:

- **Q26 — IMPLEMENT TICKET top-level reframe** (surfaced by user near end of session: "but none of these is top level process… because top level process is for example the process IMPLEMENT TICKET, right, and it happens then to choose which cycles to trigger?")
- **Q27 — Naming collision resolution at HIGH** (surfaced when user asked "wait but if we drop [`(BIG)`], will there be naming collisions?")
- **Q28 — Verb-based exact-match prompt naming + `agent-name:` field rename** (the rename work part of Q24=C from parent plan, scoped concretely here)
- **Q29 — Casing / display convention** (surfaced when user asked "should we have lowercase or capitals, underscores or spaces… maybe doc names should look like IDs")

Decisions made here cascade into:
1. Possibly further edits to the brainstorm files (`plans/ideas/3-bpmn-refactor-high-level.md`, `plans/ideas/4-bpmn-refactor-peak-level.md`) — currently committed with `(BIG)` dropped, which exposes the collision Q27 addresses.
2. Phase C YAML migration in the parent plan (Items 7–9) — depends on Q26/Q27/Q29 being locked because YAML keys + structure follow from them.
3. Item 10's Phase D downstream-alignment plan — Q28 specifies the prompt renames and `agent-name:` field rename, which Item 10 executes.

## Current state of the brainstorm (post-parent-session)

- LOW (`plans/ideas/1-*.md`) — committed; no naming-doctrine impact.
- MID (`plans/ideas/2-*.md`) — committed; has `Write Contract Tests` added (Q25), Q24 breadcrumb noted (verb-based prompt renaming deferred to Phase D).
- HIGH (`plans/ideas/3-*.md`) — committed with `(BIG)` dropped. **Naming collision exists:** orchestration `IMPLEMENT SYSTEM` ↔ step `Implement System` (same snake_case key); same for `REFACTOR TESTS` ↔ `Refactor Tests`. Q27 resolves this.
- PEAK (`plans/ideas/4-*.md`) — committed; has `(WRAPPER) TICKET LIFECYCLE` section that may be reframed as the top-level `IMPLEMENT TICKET` process per Q26.

## Open questions

### Q26 — IMPLEMENT TICKET top-level reframe

**Context.** The original brainstorm structure assumed PEAK was the top level. User insight (2026-05-25) reframed this: the actual top-level process is `IMPLEMENT TICKET` — invoked once per ticket, decides which cycle(s) to trigger based on ticket type, and wraps the lifecycle (Mark IN PROGRESS → cycle(s) → Mark IN ACCEPTANCE). The things called "PEAK entries" are therefore **cycles** (sub-processes selected per-ticket), not top-level processes.

This reframe also explains why the `TICKET LIFECYCLE` wrapper felt awkward as a separate `(WRAPPER)` section in PEAK — it's not a wrapper, it's the body of the actual top-level `IMPLEMENT TICKET` process.

**Options.**

- **(A) 5 levels with rename: TOP / CYCLE / HIGH / MID / LOW.** Add a new TOP level (`IMPLEMENT TICKET`), rename PEAK → CYCLE everywhere (file `4-bpmn-refactor-peak-level.md` → `4-bpmn-refactor-cycle-level.md`; in-doc references; Decisions section). Cleanest end state; honestly names what each level is. Most file churn.
- **(B) 5 levels without rename: TOP / PEAK / HIGH / MID / LOW.** Add TOP level for `IMPLEMENT TICKET`; keep PEAK terminology for cycles even though it's a slight misnomer (PEAK no longer = top). Less churn than (A); preserves existing file names + Decisions vocabulary.
- **(C) Stay 4 levels: keep PEAK as top.** Treat `IMPLEMENT TICKET` as the top entry of PEAK's wrapper section, fold cycle-selection logic into it. Doesn't acknowledge the structural insight; risks the same confusion resurfacing later. Lowest churn.
- **(D) Other** — e.g., merge PEAK + TOP into one level called PROCESS where `IMPLEMENT TICKET` and the cycles all live together. User specifies.

**Tightly coupled to.** Q27 (collision question changes if HIGH becomes mid-tier vs top-tier of its own visible layer). Q29 (casing decision applies uniformly across all levels — adding a TOP level adds another layer of names to convention-check).

**Recommendation.** **(A)** if you accept the structural insight is correct and want the doc to match it; (B) is acceptable if churn is a concern. (C) is not recommended — the insight will resurface during Phase C YAML migration and cost more later.

---

### Q27 — Naming collision resolution at HIGH

**Context.** Phase B.3 dropped the `(BIG)` suffix from the 3 entry-point HIGH orchestrations (`WRITE TESTS`, `IMPLEMENT SYSTEM`, `REFACTOR TESTS`). This creates collisions when these are encoded in `process-flow.yaml`:

- HIGH `IMPLEMENT SYSTEM` orchestration ↔ MID `Implement System` task → both become snake_case key `implement_system`.
- HIGH `REFACTOR TESTS` orchestration ↔ MID `Refactor Tests` task → both become snake_case key `refactor_tests`.
- HIGH `WRITE TESTS` orchestration ↔ no MID `Write Tests` task → no collision (MID has `Write Acceptance Tests`, `Write Contract Tests`).

**Existing YAML convention** in `internal/atdd/runtime/statemachine/process-flow.yaml` already uses suffix-disambiguation: `at_cycle`, `ct_subprocess`, `red_phase_cycle`, `legacy_at_cycle`, `external_system_onboarding`. So suffixes on orchestrations are already established.

**Options.**

- **(A) Industry-pragmatic — `_workflow` suffix on entry-point HIGH orchestrations.** YAML: `implement_system_workflow:`, `refactor_tests_workflow:`, `write_tests_workflow:`. Doc: choose per-Q29 (either show `(WORKFLOW)` in doc heading, or keep doc heading clean and document the doc→YAML mapping convention). Matches existing `_cycle`/`_subprocess` pattern. **Industry-common across Camunda, Airflow, Argo, Temporal, AWS Step Functions.**
- **(A-alt) Industry-pragmatic — `_cycle` or `_subprocess` suffix.** Match existing runtime convention exactly. E.g., `implement_system_cycle:`. More vocabulary-consistent with current YAML but locks brainstorm to runtime naming.
- **(B) Silver-canonical — rename to specificity.** Rename the colliding HIGH orchestrations to describe their full end-to-end scope, eliminating collision without a suffix: `IMPLEMENT SYSTEM` → `IMPLEMENT AND VERIFY SYSTEM`; `REFACTOR TESTS` → `REFACTOR AND VERIFY TESTS`. Inner steps stay `Implement System` / `Refactor Tests`. Most BPMN-canonical (Bruce Silver's *Method and Style*); avoids suffix vocabulary.
- **(C) Mixed-form (non-canonical) — switch entry-point HIGH to noun-phrase.** `IMPLEMENT SYSTEM` → `SYSTEM IMPLEMENTATION`; `REFACTOR TESTS` → `TEST REFACTORING`. Avoids collision because noun-phrase keys differ from verb-noun keys. Breaks Silver's convention (he keeps HIGH-as-sub-process verb+noun); awkward for longer orchestration names.
- **(D) YAML namespacing.** Nest YAML under `high.*` / `mid.*` / `cycle.*` keys instead of flat `processes:`. Requires schema change to `process-flow.yaml`. Out of scope for brainstorm-level decisions but worth flagging as a defensible alternative.

**Tightly coupled to.** Q26 (if TOP/CYCLE added, the collision-prone layers are clearly HIGH-vs-MID — easier to reason about). Q29 (doc display convention determines whether `(WORKFLOW)` appears in headings).

**Recommendation.** **(A) `_workflow` suffix in YAML, doc display per Q29.** Reasoning: matches existing runtime `_cycle`/`_subprocess` convention; industry-standard; lint-enforceable; survives renames. Silver's (B) is cleaner in principle but produces awkward names for longer orchestrations and doesn't scale to YAML where you scan a flat list of 50+ keys.

---

### Q28 — Verb-based exact-match prompt naming + `agent-name:` field rename

**Context.** Parent plan Q24 decided the doctrine: verb-based, exact-match to MID task names; legacy prompts collapse mechanically per Q16=B; rename work deferred to Phase D's downstream-alignment plan. **This child plan locks the concrete renames** so Phase D has a precise spec to execute.

Today's prompts (in `internal/assets/runtime/prompts/atdd/`):

| File today | Maps to MID task | Proposed verb-based rename |
|---|---|---|
| `at-red-test.md` | Write Acceptance Tests | `write-acceptance-tests.md` |
| `ct-red-test.md` | Write Contract Tests (added in Q25) | `write-contract-tests.md` |
| `at-red-dsl.md` + `ct-red-dsl.md` | Implement DSL (parameterized) | `implement-dsl.md` (one file) |
| `at-red-system-driver.md` | Implement System Drivers | `implement-system-drivers.md` |
| `ct-red-external-system-driver.md` | Implement External System Drivers | `implement-external-system-drivers.md` |
| `at-green-system.md` | (HIGH: WRITE SYSTEM step 1 = Implement System per Q15) | `implement-system.md` |
| `ct-green-external-system-stub.md` | Implement External System Stubs | `implement-external-system-stubs.md` |
| `at-refactor-system.md` | (PEAK: REFACTOR SYSTEM STRUCTURE) | `refactor-system.md` |
| `disable-tests.md` | Disable Tests | `disable-tests.md` (no rename) |
| `enable-tests.md` | Enable Tests | `enable-tests.md` (no rename) |
| `fix-verify.md` | Fix Unexpected Passing/Failing Tests | `fix-verify.md` (or split into two — see Q28.b) |
| `refine-acc.md` | (PEAK: REFINE BACKLOG step 4) | `refine-acceptance-criteria.md` |
| `update-ticket.md` | (PEAK: TICKET LIFECYCLE Mark IN PROGRESS/ACCEPTANCE) | `update-ticket.md` (no rename) |
| `task-system-interface-redesign.md` | (HIGH: IMPLEMENT RED SYSTEM DRIVER ADAPTERS variant) | TBD per Q28.c |
| `task-external-system-interface-redesign.md` | TBD | TBD per Q28.c |
| `task-system-implementation-refactoring.md` | (PEAK: REFACTOR SYSTEM STRUCTURE) | TBD per Q28.c |
| `legacy-at-test.md` | (collapse → `write-acceptance-tests.md` per Q16=B) | DELETE |
| `legacy-at-dsl.md` | (collapse → `implement-dsl.md`) | DELETE |
| `legacy-at-system-driver.md` | (collapse → `implement-system-drivers.md`) | DELETE |
| `legacy-ct-test.md` | (collapse → `write-contract-tests.md`) | DELETE |
| `legacy-ct-dsl.md` | (collapse → `implement-dsl.md`) | DELETE |
| `legacy-ct-external-system-driver.md` | (collapse → `implement-external-system-drivers.md`) | DELETE |
| `legacy-ct-external-system-stub.md` | (collapse → `implement-external-system-stubs.md`) | DELETE |

**Sub-questions.**

**Q28.a — `agent-name:` YAML field rename.** Field currently named `agent-name:` (under `params:` in `user_task` nodes that call `red_phase_cycle` etc.). Options:
- (i) `task-name:` — matches the "agent task executor" framing
- (ii) `executor:` — matches "executor for the named task"
- (iii) `prompt:` — most literal (it's a prompt file reference)
- (iv) keep `agent-name:` — no breaking change

**Q28.b — Should `fix-verify.md` split into two prompts?** Today one file handles both Fix Unexpected Passing Tests and Fix Unexpected Failing Tests. Per verb-exact-match doctrine they should be separate prompts:
- (i) Split into `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md`
- (ii) Keep as one file `fix-verify.md` — exception to the doctrine because they share most logic
- (iii) Rename to `fix-test-result-mismatch.md` — single file covering both, more accurate name

**Q28.c — `task-*` prompts.** Three prompts have a `task-` prefix. What do they map to?
- `task-system-interface-redesign.md` — likely the HIGH `IMPLEMENT RED SYSTEM DRIVER ADAPTERS` orchestration, used during REDESIGN SYSTEM STRUCTURE cycle. Rename to `implement-system-driver-adapters-redesign.md`?
- `task-external-system-interface-redesign.md` — likely HIGH `IMPLEMENT RED EXTERNAL SYSTEM DRIVER ADAPTERS - CONTRACT TESTS`. Rename to `implement-external-system-driver-adapters-redesign.md`?
- `task-system-implementation-refactoring.md` — likely PEAK `REFACTOR SYSTEM STRUCTURE`. Rename to `refactor-system.md` (but that already exists from `at-refactor-system.md` rename) — possible collision. Resolution needed.

**Tightly coupled to.** Q26 (TOP/CYCLE reframe doesn't change prompt names — prompts are MID-task-level). Q27 (no coupling — orchestration names differ from task names). Q29 (prompts are kebab-case files; brainstorm doc uses ALL CAPS — different layers).

**Recommendation.** Lock the verb-based rename table (above) as the canonical spec. **Q28.a: (i) `task-name:`** (cleanest semantic match). **Q28.b: (i) split** (consistent with verb-exact-match doctrine, no exceptions). **Q28.c: (case-by-case)** — verify each `task-*` prompt's actual content during refinement walk; choose specific names then.

---

### Q29 — Casing / display convention (doc layer vs YAML layer)

**Context.** The brainstorm doc currently uses ALL CAPS + spaces for orchestrations (`## WRITE TESTS`), Title Case + spaces for step names (`1. Write Acceptance Tests`). YAML uses snake_case + underscores. User raised: should the doc names match YAML (snake_case) for direct correspondence?

**Industry two-layer rule** (Camunda, Airflow, AWS Step Functions, Argo, Temporal):
- **Display / Doc / Diagram label** = Title Case + spaces (or ALL CAPS for emphasis). E.g., "Implement System Workflow."
- **Technical ID / YAML key** = snake_case + underscores + suffix. E.g., `implement_system_workflow`.
- **Cross-reference in prose** = display name + backticked ID. E.g., "Implement System Workflow (`implement_system_workflow`)."

Nobody puts snake_case identifiers in diagram headings or doc section titles. The IDs are for the runtime; the display is for humans.

**Options.**

- **(A) Keep current — ALL CAPS + spaces in doc.** `## IMPLEMENT SYSTEM`. Personal style; visually distinctive (orchestrations stand out from Title Case steps); non-canonical but works. No churn.
- **(B) Switch doc to Title Case + spaces (BPMN-canonical).** `## Implement System`. Matches Camunda Modeler / Signavio / Silver's *Method and Style*. Loses visual hierarchy unless step names are distinguished by other means (numbering already does this; could keep).
- **(C) Switch doc to snake_case + underscores (matches YAML 1:1).** `## implement_system`. Tight YAML coupling; reads like code; non-standard for design docs; long names become unreadable.
- **(D) Hybrid:** ALL CAPS at orchestration level (current), add YAML key in parens or backticks under each heading. `## IMPLEMENT SYSTEM (YAML: \`implement_system_workflow\`)`. Explicit cross-reference; some redundancy.

**Tightly coupled to.** Q27 (display-of-suffix in doc depends on this — option D Q27 with option A Q29 = clean doc without `(WORKFLOW)` shown; option D Q27 with option B Q29 = different look). Q26 (TOP-level naming convention applies uniformly).

**Recommendation.** **(A) keep ALL CAPS + spaces**, add a top-of-file note (one line) describing the doc ↔ YAML mapping convention. Reasoning: zero churn; the current style works for scan-ability and matches existing doc convention; the convention note removes ambiguity for future readers.

---

## Items (to be executed AFTER Q26–Q29 are locked)

These items are written to be executable via `/execute-plan` once decisions land in the Decisions section. Items 1–4 are sized for one `/execute-plan` invocation each.

1. - [ ] **Apply Q26=A reframe to brainstorm files (TOP / CYCLE / HIGH / MID / LOW).** Rename `plans/ideas/4-bpmn-refactor-peak-level.md` → `plans/ideas/4-bpmn-refactor-cycle-level.md`. Create new `plans/ideas/5-bpmn-refactor-top-level.md` containing the `implement-ticket` body (Mark IN PROGRESS → Decide Cycle(s) → Call chosen Cycle → Mark IN ACCEPTANCE; see "TOP-level reframe — implications" in the Discussion archive for the sketch). Fold `(WRAPPER) TICKET LIFECYCLE` from the former PEAK file into the new TOP file. Sweep `PEAK` → `CYCLE` in parent plan Decisions section, this child plan, and all brainstorm cross-references. Apply Q29 kebab-case to all process-model identifiers in the affected files. Commit.
   **Done when:** brainstorm files reflect TOP / CYCLE / HIGH / MID / LOW; `implement-ticket` has its own section; all process-model identifiers are kebab-case.

2. - [ ] **Apply Q27=B rename-to-specificity to HIGH brainstorm.** Rename the colliding HIGH orchestrations: `implement-system` → `implement-and-verify-system`; `refactor-tests` → `refactor-and-verify-tests`. Verify `write-tests` against the HIGH brainstorm content; rename for consistency if it includes a verify step (likely `write-and-verify-red-tests` or similar). Apply Q29 kebab-case throughout. Commit.
   **Done when:** HIGH collisions resolved; all HIGH orchestration identifiers are kebab-case and describe their full composite scope.

3. - [ ] **Apply Q28 prompt rename spec.** The actual prompt-file renames happen during Item 10 of the parent plan (Phase D downstream-alignment). This item writes the locked rename spec (the consolidated Q28 prompt rename table in the Decisions section of this plan) into either (a) the parent plan's Decisions section, or (b) a new sub-section in Item 10's Phase D plan template. Resolves Q28.c by reading the three `task-*` prompt files and the existing `at-refactor-system.md` to (i) drop `task-` prefix, (ii) eliminate `-redesign` cycle suffix, (iii) resolve `refactor-system.md` collision. Commit.
   **Done when:** Phase D has a concrete, line-by-line rename spec; Q28.c task-* names resolved; collision resolved.

4. - [ ] **Apply Q29=kebab-case-everywhere convention.** Rewrite all brainstorm-doc section headings to kebab-case lowercase. Update all in-prose references to process-model names to kebab-case. Add a top-of-file convention note to each brainstorm file: "All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names." Commit.
   **Done when:** kebab-case is uniform across LOW / MID / HIGH / CYCLE / TOP brainstorm files; convention note present at top of each.

5. - [ ] **Hand off to parent plan.** Once Q26–Q29 are locked and Items 1–4 of this child plan are committed, the parent plan's Item 6 (cross-check walk) and Items 7–9 (Phase C YAML migration) can proceed. Delete this child plan (`plans/20260525-1130-bpmn-naming-doctrine.md`) — its Decisions live in the parent plan's Decisions section by the time you reach this item. Commit the deletion.
   **Done when:** this file is deleted; parent plan's Decisions reflect Q26/Q27/Q28/Q29 outcomes; user is ready to `/execute-plan` parent plan Item 6.

---

## Decisions

*(Empty — fill during refinement walk.)*

### Q26 — IMPLEMENT TICKET top-level reframe

**Decision: (A) 5 levels with rename — TOP / CYCLE / HIGH / MID / LOW.**

User-flow framing (locks the choice): *"User initiates `IMPLEMENT TICKET`, then based on ticket classification we choose one of the cycles."* This sentence IS the (A) structure: TOP (`IMPLEMENT TICKET`) → classification gateway → CYCLE. (B) would force phrasing like "choose one of the peaks (sitting under TOP)" — i.e., "peak that isn't peak." (C) would force "`IMPLEMENT TICKET` lives inside PEAK alongside the things it selects" — collapsing two levels that the user's own flow distinguishes.

Rationale: each layer's name matches what it actually is. PEAK is a misnomer in (B) (implies top, but sits under TOP); (C) keeps PEAK as a mixed bag (top-level process + cycles coexist at the same level). (A) front-loads the rename so Phase C YAML migration has a 1:1 mapping (5 brainstorm files → 5 logical layers in `process-flow.yaml`), with no translation table.

Concrete changes (executed in Item 1):
- Rename `plans/ideas/4-bpmn-refactor-peak-level.md` → `plans/ideas/4-bpmn-refactor-cycle-level.md`.
- Create new `plans/ideas/5-bpmn-refactor-top-level.md` containing the `IMPLEMENT TICKET` body (Mark IN PROGRESS → Decide Cycle(s) → Call chosen Cycle → Mark IN ACCEPTANCE; see "TOP-level reframe — implications" in the Discussion archive for the sketch).
- Remove `(WRAPPER) TICKET LIFECYCLE` from the (former) PEAK file; it migrates into the new TOP file.
- Sweep `PEAK` → `CYCLE` in parent plan Decisions section, this child plan, and any brainstorm cross-references.

Cycle-selection sub-questions (auto-vs-manual gateway, chaining, preconditions) are noted in the Discussion archive and remain open — they do not block Q26 itself and will be resolved during Item 1's refinement of the new TOP file.

### Q27 — Naming collision resolution at HIGH

**Decision: (B) Silver-canonical — rename HIGH orchestrations to describe their full composite scope.**

Rationale (industry + textbook converge):
- **BPMN textbook (Bruce Silver, *Method and Style*).** A sub-process name should describe what the *whole composite* does, distinct from any single contained task. The collision `IMPLEMENT SYSTEM` (HIGH) ↔ `Implement System` (MID) is a *symptom of a naming bug in the parent*, not a YAML-mechanics issue to paper over with a suffix.
- **Industry (Camunda, AWS Step Functions, Argo, Temporal, Airflow).** Mature codebases use proper composite names AND optionally a suffix on technical IDs. The "just suffix it" reading of (A) is not industry-canonical; the proper-name half (B) is mandatory, the suffix is optional.
- **User's "no HIGH/MID/LOW in naming" rule** (see Standing constraints): closes the gap by ruling out the optional suffix layer (type-tag suffixes like `_workflow` / `_subprocess` encode layer-shape into the name). Drops industry practice down to pure (B). Also rules out (D) YAML namespacing (literal layer in keys) and (C) noun-phrase mixed-form (encodes top-level-shape by grammar switch).

Concrete renames (locked spec; verified against the HIGH brainstorm during Item 2 execution). Names are shown in their canonical kebab-case identifier form per Q29:
- `implement-system` → `implement-and-verify-system` (covers writing green system code + verifying AT passes).
- `refactor-tests` → `refactor-and-verify-tests` (covers refactoring + verifying tests still pass).
- `write-tests` → check during Item 2 execution; rename for consistency if the HIGH orchestration includes verification (likely `write-and-verify-red-tests` or similar).

Excluded options:
- **(A) `_workflow` / `_subprocess` suffix** — would lock in the naming bug AND encode layer-shape, violating the no-layer-coding rule.
- **(C) noun-phrase mixed-form** — type-codes "top-level-shaped" by switching grammar.
- **(D) YAML namespacing** — literal `high.*` / `cycle.*` keys violate the no-layer-coding rule.

### Q28 — Verb-based exact-match prompt naming + `agent-name:` field rename

**Q28.a decision: (v) DROP the field entirely — convention over configuration.**

New runtime contract: `prompt_path(task_name) = kebab-case(lowercase(task_name)) + ".md"`. The verb-exact-match doctrine (Q24 parent + Q28 prompt rename table) makes the indirection redundant. Self-enforcing — runtime errors at startup if the convention-derived path doesn't exist.

Rationale:
- Eliminates a per-node config field across `process-flow.yaml`; less to maintain, less drift, less misnaming.
- No layer-coding in YAML field name (`agent-` / `executor-` / `task-` were all runtime-role vocabulary, violating the no-layer-coding rule from Standing constraints).
- Convention IS the lint rule — discoverability cost is one line of doc explaining the transform.
- Side benefit for Q28.c: forces `task-*` prompts to be renamed to their MID task name; runtime can't find them otherwise. No separate "rename to match" enforcement step needed.

Excluded options:
- **(i) `task-name:` / (ii) `executor:` / (iii) `prompt:` / (iv) keep `agent-name:`** — all preserve a redundant indirection that the verb-exact-match doctrine renders obsolete. (ii) and (iv) additionally encode runtime-role/layer vocabulary in YAML, violating the no-layer-coding rule.

**Q28.b decision: (i) Split into two prompts.**

Two distinct MID tasks → two distinct prompts:
- `Fix Unexpected Passing Tests` → `fix-unexpected-passing-tests.md`
- `Fix Unexpected Failing Tests` → `fix-unexpected-failing-tests.md`

Rationale:
- **Token efficiency at invocation** (user's primary concern). Each prompt loads only the instructions relevant to its case; (iii) single-composite would load both halves per invocation and risk the agent mis-applying the irrelevant half.
- **Doctrine consistency.** Verb-exact-match means distinct work = distinct task = distinct prompt; (iii) hides an internal branch.
- **Gateway routing is mechanical.** Runtime knows test polarity from the test result; routing is free, no in-prompt investigation needed.
- **Teaching clarity.** Two focused tasks visible in the brainstorm rather than a single composite with hidden branching.

Implementation caveat for Phase D: if the diagnostic logic for the two cases turns out to be ~90% identical when authored, consider factoring shared steps into a referenced file. That's an implementation detail at prompt-authoring time, not a structural reason to recombine.

Excluded:
- **(ii) Keep `fix-verify.md`** — eliminated by Q28.a=DROP (convention-derived path can't be `fix-verify.md` from any verb-based task name).
- **(iii) Single composite `fix-test-result-mismatch.md`** — loads both cases per invocation; hides an internal branch the runtime could route on; violates verb-exact-match doctrine.

**Q28.c decision: Lock principles now; defer concrete names to Item 3 execution.**

Principles (locked):
1. **Drop the `task-` prefix** from all three prompts. It's layer-coding (synonym for "agent task executor target"); doubly redundant under Q28.a=DROP (runtime derives prompt path from MID task name, no per-prompt routing config).
2. **No `-redesign` (or similar) cycle-context suffix.** Cycle is determined upstream by the gateway in `IMPLEMENT TICKET` (TOP) → cycle selection; the MID task name must describe what the task *does*, not which cycle it runs inside. Cycle-suffix is layer-coding (Standing constraint violation).
3. **Resolve the `refactor-system.md` collision by reading actual prompt content** during Item 3. The candidates:
   - `task-system-implementation-refactoring.md` (current name, currently in use)
   - `at-refactor-system.md` (Q28 main table proposes renaming to `refactor-system.md`)
   If both prompts describe the same MID task → merge into single `refactor-system.md`. If different scope → name each for its actual scope (likely something like `refactor-system-implementation` vs `refactor-system-structure` if they cover internals vs adapter shape respectively).

Concrete naming work deferred to Item 3 (Phase D rename spec authoring), which has access to read the actual prompt contents and brainstorm MID-task list together. This walk locks the principles so Item 3 has unambiguous guidance.

**Q28 prompt rename table (locked).** Consolidated from the spec at lines 79–101, with Q28.a/b/c applied and PEAK → CYCLE updated per Q26=A. Under Q28.a=DROP, the "Required filename" column is the source of truth — runtime derives it deterministically from the MID task name and errors at startup if the file is missing.

| File today | Maps to MID task | Required filename |
|---|---|---|
| `at-red-test.md` | Write Acceptance Tests | `write-acceptance-tests.md` |
| `ct-red-test.md` | Write Contract Tests (added in Q25) | `write-contract-tests.md` |
| `at-red-dsl.md` + `ct-red-dsl.md` | Implement DSL (parameterized) | `implement-dsl.md` (one file, two callers) |
| `at-red-system-driver.md` | Implement System Drivers | `implement-system-drivers.md` |
| `ct-red-external-system-driver.md` | Implement External System Drivers | `implement-external-system-drivers.md` |
| `at-green-system.md` | Implement System (HIGH: write-system step 1, per Q15) | `implement-system.md` |
| `ct-green-external-system-stub.md` | Implement External System Stubs | `implement-external-system-stubs.md` |
| `at-refactor-system.md` | Refactor System (CYCLE: refactor-system-structure) | `refactor-system.md` *(verify against Q28.c collision)* |
| `disable-tests.md` | Disable Tests | `disable-tests.md` (no rename) |
| `enable-tests.md` | Enable Tests | `enable-tests.md` (no rename) |
| `fix-verify.md` | Fix Unexpected Passing Tests **AND** Fix Unexpected Failing Tests | **SPLIT** (Q28.b): `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md` |
| `refine-acc.md` | Refine Acceptance Criteria (CYCLE: refine-backlog step 4) | `refine-acceptance-criteria.md` |
| `update-ticket.md` | Update Ticket (TOP: implement-ticket — Mark IN PROGRESS / Mark IN ACCEPTANCE, per Q26=A) | `update-ticket.md` (no rename) |
| `task-system-interface-redesign.md` | *TBD per Q28.c* (drop `task-` prefix; no `-redesign` cycle suffix) | *Item 3 resolves by reading prompt content* |
| `task-external-system-interface-redesign.md` | *TBD per Q28.c* | *Item 3 resolves* |
| `task-system-implementation-refactoring.md` | *TBD per Q28.c — collision with `refactor-system.md` to resolve* | *Item 3 resolves* |
| `legacy-at-test.md` | (collapse → `write-acceptance-tests.md` per Q16=B) | **DELETE** |
| `legacy-at-dsl.md` | (collapse → `implement-dsl.md`) | **DELETE** |
| `legacy-at-system-driver.md` | (collapse → `implement-system-drivers.md`) | **DELETE** |
| `legacy-ct-test.md` | (collapse → `write-contract-tests.md`) | **DELETE** |
| `legacy-ct-dsl.md` | (collapse → `implement-dsl.md`) | **DELETE** |
| `legacy-ct-external-system-driver.md` | (collapse → `implement-external-system-drivers.md`) | **DELETE** |
| `legacy-ct-external-system-stub.md` | (collapse → `implement-external-system-stubs.md`) | **DELETE** |

### Q29 — Casing / display convention

**Decision: (C-kebab-unified) Kebab-case everywhere — YAML, doc headings, prompts, in-prose references.** Single uniform convention; no two-layer rule, no translation gap. Refines original (C) snake_case → kebab-case based on the 2026-05-25 walk; further refined to kebab-in-YAML when user pushed back on the unnecessary two-layer split.

**Single rule (locked):** Every process-model identifier — top-level processes, cycles, orchestrations, tasks, sub-tasks — uses **kebab-case lowercase** in every layer it appears.

| Layer | Casing | Example |
|---|---|---|
| YAML keys (`process-flow.yaml`) | kebab-case | `implement-and-verify-system:` |
| Doc headings (brainstorm files) | kebab-case | `## implement-and-verify-system` |
| Prompt filenames (MID tasks only) | kebab-case | `write-acceptance-tests.md` |
| In-prose references | kebab-case | "the `implement-and-verify-system` orchestration calls `write-acceptance-tests`" |
| Anchor slugs (GFM auto-generated) | kebab-case | `#implement-and-verify-system` (matches the heading verbatim) |

**Scope (kebab-case applies to):**
- YAML keys in `process-flow.yaml` for every process-model element (top-level processes, cycles, orchestrations, tasks, sub-tasks).
- All brainstorm-doc section headings naming a process-model element.
- All prompt filenames in `internal/assets/runtime/prompts/atdd/` (Q28.a's convention `prompt_path(task_name) = task_name + ".md"` — task name is already kebab-case, so the formula reduces to identity).
- In-prose references to process-model names within the brainstorm docs (e.g., "the `implement-and-verify-system` orchestration calls `write-acceptance-tests`").
- Any Go struct tags or string constants that name process-model elements (`yaml:"implement-and-verify-system"`).

**Out of scope (kebab-case does NOT apply to):**
- Standard English prose, paragraph text, explanatory writing.
- Layer labels (TOP / CYCLE / HIGH / MID / LOW) — these are organizational categories for the brainstorm doc, not process-model element names.
- Proper nouns (Bruce Silver, Camunda, Airflow, etc.) and technical acronyms (BPMN, YAML, TDD, etc.).
- Existing snake_case YAML keys in `process-flow.yaml` (`at_cycle`, `ct_subprocess`, `red_phase_cycle`, `legacy_at_cycle`, `external_system_onboarding`) — these are migrated to kebab-case during Phase C's YAML rewrite, where they are being replaced anyway.

**Rationale:**
- **One rule beats two rules.** "Kebab-case everywhere" is one sentence of doctrine. A snake_case-YAML + kebab-case-display split would be two rules plus a translation gap.
- **Most modern in greenfield 2026.** Kebab-case is the cloud-native default across Argo Workflows, Tekton, Kubernetes top-level keys, GitHub Actions, npm, AWS resource naming, modern web framework filenames, Helm charts.
- **kebab-case is fully valid YAML.** Go YAML parsers (`yaml.v3`, `sigs.k8s.io/yaml`) handle kebab keys via struct tags (`yaml:"implement-and-verify-system"`) with zero ceremony.
- **Migration cost already paid.** Parent plan Phase C rewrites `process-flow.yaml` entirely. The existing ~5–10 snake_case keys are being replaced as part of the refactor; picking kebab-case for the new keys adds zero incremental work.
- **Reads better than snake_case for human-facing artifacts.** Hyphens visually behave like word separators; underscores read as code.
- **GFM anchor alignment.** GitHub's markdown renderer kebab-cases headings into anchor slugs anyway; writing headings in kebab-case directly means `## implement-and-verify-system` → anchor `#implement-and-verify-system` with zero translation.

**Excluded options:**
- **(A) ALL CAPS + spaces** — visually distinctive but not an ID convention anywhere; doesn't survive into anchors/slugs cleanly; requires per-doc mental translation to YAML.
- **(B) Title Case + spaces** — BPMN textbook canonical but ambiguous in prose ("Implement And Verify System" — is "And" name or grammar?); loses 1:1 mapping to filenames/YAML.
- **(C-snake) `## implement_and_verify_system`** — reads as Python code in a design doc; loses the modern-display feel; doesn't match the kebab-case prompt filenames.
- **(D) Hybrid ALL CAPS + backticked YAML** — commits to two display conventions when one suffices.
- **Two-layer rule (snake YAML + kebab display)** — initially proposed in this walk; rejected by user as unnecessary complexity. One convention is cleaner.

**Cascade implications (recorded for Item 4 execution):**
- All brainstorm doc section headings rewritten to kebab-case lowercase.
- All Q27 HIGH renames in their kebab-case identifier form: `implement-and-verify-system`, `refactor-and-verify-tests`, (consistency-check) `write-and-verify-red-tests` or similar.
- All Q26 TOP/CYCLE names: `implement-ticket` (TOP); `change-system-behavior` / `cover-system-behavior` / `redesign-system-structure` / `refactor-system-structure` / `refactor-test-structure` / `refine-backlog` / `onboard-external-system` (CYCLE — names verified during Item 1 execution).
- All MID tasks: `write-acceptance-tests`, `implement-dsl`, `implement-system`, `fix-unexpected-passing-tests`, `fix-unexpected-failing-tests`, etc. — exactly matching the Q28 prompt filenames.
- `process-flow.yaml` keys all kebab-case; Go struct tags / string constants updated to match.

---

## Discussion archive (from 2026-05-25 session)

### Industry survey — suffix conventions for workflow IDs

| Tool / domain | Convention | Example |
|---|---|---|
| **Camunda 7/8** (enterprise) | `*Process` / `*Subprocess` suffix in process IDs | `loanApprovalProcess`, `creditCheckSubprocess` |
| **jBPM** | `*Process` suffix universal | `OrderFulfillmentProcess` |
| **Flowable / Bonita** | Suffix on IDs, file names `*-process.bpmn` | `customer-onboarding-process.bpmn` |
| **AWS Step Functions** | `*-state-machine` / `*-workflow` in resource names | `order-fulfillment-state-machine` |
| **GitHub Actions** | Workflow filenames suffix-heavy | `build-and-test.yml`, `release-workflow.yml` |
| **Argo Workflows / Tekton** | `*-workflow`, `*-pipeline` suffix | `ci-build-workflow.yaml` |
| **Airflow DAGs** | `*_dag` suffix | `customer_etl_dag` |
| **Signavio / SAP BPM** | Consistent suffix (`Process`, `Workflow`, `Routine`) | enforced via tool conventions |

### Bruce Silver's *Method and Style* (BPMN textbook) — naming rules

| Element type | Canonical form | Example |
|---|---|---|
| Atomic activity / task | Verb + Object | "Approve Order", "Implement DSL", "Run Tests" |
| Sub-process (composite, but appears as an activity in parent flow) | Verb + Object | "Onboard Customer", "Implement RED DSL Core" |
| Top-level process (whole workflow) | **Noun phrase** | "Customer Onboarding", "Order Fulfillment" |

Silver only switches to noun-phrase at the top-level. Sub-processes that are called as activities stay verb+noun. The reasoning: at activity level you're describing an *action* → verb; at top-level process you're describing the *subject matter / lifecycle* → noun.

If we accept Q26's reframe (IMPLEMENT TICKET as top-level), Silver's rule applies cleanly:
- TOP `IMPLEMENT TICKET` → could be `TICKET IMPLEMENTATION` (noun-phrase) per Silver, but verb+noun also common.
- CYCLE / HIGH / MID → verb + Object (already are).
- LOW → verb (already are).

### Two-layer rule (display vs ID) — universal pattern

In BPMN XML:
```xml
<process id="orderFulfillmentProcess" name="Order Fulfillment">
  <subProcess id="paymentSubprocess" name="Process Payment">
    <task id="chargeCardTask" name="Charge Card"/>
  </subProcess>
</process>
```

- `id` has the suffix (technical, unique, lint-enforceable).
- `name` doesn't (human-readable label).

Same pattern in Airflow UI (DAG ID `customer_etl_dag` displays as "Customer ETL"), AWS Step Functions Console (resource `OrderFulfillmentStateMachine` displays as "Order Fulfillment"), Argo / Tekton (suffixed YAML, clean UI labels).

**Where icons exist (diagrams):** no suffix needed — icon conveys type.
**Where icons don't exist (YAML, code, IDs):** suffix wins — substitutes for the missing icon.

Our brainstorm doc is closer to "diagram" (has visual cues like `##` heading levels, `《 SHARED 》` prefixes); our YAML is closer to "BPMN XML" (flat-namespace technical IDs).

### Specific collisions identified in current brainstorm

Post-Phase-B.3 (after `(BIG)` dropped), the following collisions exist when encoded in `process-flow.yaml`:

| HIGH orchestration | MID task | YAML key collision |
|---|---|---|
| `WRITE TESTS` | (no MID task by that name) | ❌ none |
| `IMPLEMENT SYSTEM` | `Implement System` (implied — not currently in MID) | ⚠️ if MID adds the umbrella task |
| `REFACTOR TESTS` | `Refactor Tests` (implied — not currently in MID) | ⚠️ if MID adds the umbrella task |
| `WRITE RED ACCEPTANCE TESTS` | `Write Acceptance Tests` (MID) | ❌ no — different names |
| `IMPLEMENT RED DSL CORE` | `Implement DSL` (MID) | ❌ no — different names |

So 2 of 3 entry-point HIGH orchestrations are at risk depending on whether MID grows umbrella tasks matching the HIGH name.

### TOP-level reframe — implications

If Q26 = A (5 levels with rename):

**File structure:**
- `plans/ideas/1-bpmn-refactor-low-level.md` (unchanged)
- `plans/ideas/2-bpmn-refactor-mid-level.md` (unchanged)
- `plans/ideas/3-bpmn-refactor-high-level.md` (unchanged content; possibly cross-ref updates)
- `plans/ideas/4-bpmn-refactor-cycle-level.md` (renamed from `4-*-peak-level.md`; remove `(WRAPPER) TICKET LIFECYCLE`)
- `plans/ideas/5-bpmn-refactor-top-level.md` (new; contains `IMPLEMENT TICKET` body)

**`IMPLEMENT TICKET` body sketch:**
```
## IMPLEMENT TICKET

INPUT: Ticket (with metadata: type, acceptance criteria, etc.)

1. Mark Ticket IN PROGRESS
2. Decide Cycle(s) based on ticket type:
   - Feature → CHANGE SYSTEM BEHAVIOR
   - Coverage gap → COVER SYSTEM BEHAVIOR
   - Driver-adapter ticket → REDESIGN SYSTEM STRUCTURE
   - System refactor → REFACTOR SYSTEM STRUCTURE
   - Test refactor → REFACTOR TEST STRUCTURE
   - Backlog work → REFINE BACKLOG
   - New external system → ONBOARD EXTERNAL SYSTEM
3. Call chosen Cycle
4. Mark Ticket IN ACCEPTANCE
```

**Cycle-selection sub-questions** (not blocking for Q26 itself; can be resolved during refinement):
- Does `IMPLEMENT TICKET` always pick exactly one cycle, or can it chain (e.g., REFINE BACKLOG then CHANGE SYSTEM BEHAVIOR)?
- Is cycle-selection automatic (gateway on ticket field) or manual (human picks)?
- Are there preconditions to entering a cycle (e.g., "ACs must be in approved state" before CHANGE SYSTEM BEHAVIOR)?

### Token-efficiency note from parent session

This child plan exists because the parent session's chat was burning significant tokens drilling into naming-doctrine details (per memory `feedback_flag_non_token_efficient`, the user invited pushback). Splitting into a fresh-chat child plan was the user's call (parent session ~80K tokens deep when split). Per memory `feedback_offer_clear_then_execute_plan`: the natural seam for fresh chat is `/clear` then this plan.

---

## Standing constraints (inherited from parent plan)

- **No HIGH / MID / LOW in naming** (locked 2026-05-25 during Q27 walk). Layer labels (TOP / CYCLE / HIGH / MID / LOW) are organizational categories for the brainstorm doc — they MUST NOT appear in process names, task names, YAML keys, suffix tags, or grammar-shape encodings. This rules out `_workflow` / `_subprocess` type-suffixes, `high.*` / `cycle.*` YAML namespacing, and noun-phrase grammar switches used to signal "top-level-shaped." Names must describe **scope** (what the thing does), not **layer** (what tier it sits at). Applies across Q27, Q28, Q29, and all downstream Phase C / Phase D work.
- **Token-efficient by default** — flag any user-proposed workflow that burns tokens unnecessarily and offer a cheaper alternative (`feedback_flag_non_token_efficient`).
- **Session-handoff cadence: auto-commit, then surface `/clear` + `/execute-plan`** at end of each item.
- For agent-authored surgical commits with specific message + file list, use raw `git`, not `/commit` (`feedback_use_commit_skill`).
- Concurrent-agent collision risk — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
