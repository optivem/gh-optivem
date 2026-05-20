# 2026-05-19 15:37 UTC — Post-meta-bpmn discussion topics

**Status:** REFINED + DRAINED (2026-05-20) — every item is now marked DISCHARGED, RESOLVED (folded into a sibling), DECIDED (ready for a small dated plan), or PROMOTED (already has a dated plan). This file is no longer a work queue; it is an index of decisions and pointers.

**Outcome summary (per item):**

- **Item 1** (`## Scope` section duplicates frontmatter) — DISCHARGED. Item 5's inlining sweep absorbed both jobs (drop layer enumeration; keep behavioral framing as numbered steps inside GREEN prompts).
- **Item 2** (`global/` vs `runtime/` split misleading) — DECIDED. Promote to a small dated plan that renames to `internal/assets/runtime/references/atdd/` + `runtime/references/code/`, and updates the sync target to `~/.gh-optivem/references/atdd/`.
- **Item 3** (`doctrine/` vs `shared/` naming) — RESOLVED. Folded into Item 2's decision; canonical name `references/` chosen, both original candidates rejected.
- **Item 4** (doctrine/prompt structure mismatch) — LARGELY DISCHARGED. `task-` prefix removal folded into Item 9's rename plan; CI consistency walk noted as a residual but not promoted.
- **Item 5** (consolidation rule) — DECIDED + LARGELY DISCHARGED. The N=1/N≥2 rule was superseded by "inline everything, two mechanisms" (per-phase inlining + universal argv injection). Work is landed across commits `2df45d3`, `4b44722`, and plan `20260520-0907-runtime-shared-scope-injection.md`. Residual: `path-keys.md` classification.
- **Item 6** (component-fanout) — PARTIALLY DISCHARGED. Prompt-level merge done; structural collapse pending (small near-term plan: collapse `AT_GREEN_BACKEND`+`AT_GREEN_FRONTEND` → single `AT_GREEN`); per-component fanout deferred to a future plan.
- **Item 7** (`at-refactor.md` orphan) — PROMOTED to `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`. Decision: wire up, not delete.
- **Item 8** (`acceptance-criteria-refinement.md` + `analysis/` orphan) — PROMOTED to the same companion plan as Item 7.
- **Item 9** (naming drift, four names for chore) — DECIDED. Canonical name `system-implementation-refactoring`. Ready for a small dated rename plan. The `task-` prefix removal from Item 4 piggybacks on this plan.
- **Item 10** (disable-tests/enable-tests human-only docs) — DISCHARGED. Both are now real Haiku agent prompts (commit `b3b5952`); the "human-only docs" premise is obsolete.
- **Item 11** (AT_GREEN_BACKEND/FRONTEND hardcoded) — RESOLVED. Folded into Item 6's immediate collapse plan.
- **Item 12** (`<acceptance-api>` / `<acceptance-ui>` suite labels) — RESOLVED. Folded into Item 6's immediate collapse plan.

**Plans spawned by this refinement:**

- New small dated plan for Item 2 + Item 3 — asset-tree rename to `runtime/references/`.
- New small dated plan for Item 9 + Item 4 — `chore` → `system-implementation-refactoring` rename, plus `task-`-prefix removal on the two structure prompts.
- New small dated plan for Item 6's immediate collapse — `AT_GREEN_BACKEND`+`AT_GREEN_FRONTEND` → single `AT_GREEN`, suite reduction, `phase_doc:` drift cleanup.
- Existing plan picks up Items 7 + 8: `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md` (still in its own refinement walk).
- Deferred follow-up plan for per-component fanout (Item 6 stretch goal) — defer until concrete multi-component requirements arrive.

**Purpose (original):** scratchpad for topics, remarks, and follow-up items the user wants to raise AFTER the meta-bpmn coordination plan finishes. Each item here is a discussion seed, not a refined plan — items get promoted to their own dated plan file once discussed.

**Cross-reference:** this plan sits downstream of `20260519-0929-meta-bpmn-ssot-coordination.md` (discharged in commit `55dfb18`). That coordination plan covered the BPMN orchestration + SSoT + vocabulary cluster (plans `20260518-1144`, `20260518-1530`, `20260518-1742`, `20260518-2236`, `20260519-0704`). Nothing in here should preempt or duplicate items already tracked there.

---

## Framing

Items 1–12 below all surfaced from the same root question: **what is the contract between `internal/assets/global/docs/atdd/process/` (doctrine) and `internal/assets/runtime/prompts/atdd/` (prompts), and why is so much of it implicit?**

The two headline items are:

- **Item 5 — consolidation rule:** inline doctrine into prompt when N=1 reader; keep separate when N≥2.
- **Item 6 — component-based fanout:** replace hardcoded backend/frontend with parameterised per-component phases.

Items 1–4 and 7–12 are mostly symptoms / drift that the two headlines resolve or expose. Walk them one at a time; promote each to its own dated plan once discussed.

---

## Topics to discuss

### 1. `## Scope` section in phase docs duplicates frontmatter — DISCHARGED

**Status:** DISCHARGED — Item 5's inlining sweep absorbed both jobs in this item without needing the rename.

**What happened:**

- **Job 1 (drop layer enumeration):** executed. No inlined prompt under `internal/assets/runtime/prompts/atdd/` carries a `## Scope` prose section.
- **Job 2 (keep behavioral framing):** executed where it applies. GREEN prompts (`at-green-system.md` steps 2+3, `ct-green-external-system-stub.md` steps 2+3) carry the "tests/DSL/drivers are frozen during GREEN" + escalation framing as numbered steps inside the prompt body. RED prompts (`at-red-test.md`) intentionally do not — frozen-layer rules don't apply when the agent is writing tests.
- **Proposed rename `## Scope` → `## Frozen layers`:** never happened and didn't need to — the framing lives inline as prompt steps, not as a separate section.

**Residual (not a blocker):** behavioral framing is now distributed across N inlined prompts instead of pulled from one doctrine source. Drift between prompts (e.g. a GREEN phase forgetting the escalation step) becomes the new risk. A periodic consistency check across the GREEN prompts would catch it — not worth a dated plan today, but worth a check if a new GREEN-style phase gets added.

---

### 2. `global/` vs `runtime/` split is by delivery mechanism, naming is misleading — DECIDED, ready for promotion

**Status:** DECIDED — promote to a small dated plan that renames `internal/assets/global/docs/atdd/` + `internal/assets/code/` to `internal/assets/runtime/references/atdd/` (and `runtime/references/code/`).

**Updated landscape (post-Item 5 + plan `20260520-0907`):** three delivery mechanisms = three asset trees, only two of which are under `runtime/` today:

1. `internal/assets/runtime/prompts/atdd/*.md` — argv-dispatched per phase (13 files).
2. `internal/assets/runtime/shared/{preamble,scope,session-end}.md` — universally concatenated into every dispatched prompt at Go startup.
3. `internal/assets/global/docs/atdd/` + `internal/assets/code/` — synced to disk, selectively read by phase-specific prompts via `Read ${docs_root}/...`. Surviving content is reference material, not prose doctrine:
   - `global/docs/atdd/architecture/{driver-adapter,driver-port,dsl-core,dsl-port,system,test}.md` — per-phase architecture references.
   - `code/language-equivalents/{java,csharp,typescript,README}.md` — per-language syntax tables.
   - `code/testkit-{architecture-rules,language-exceptions}.md` — testkit references.
   - `global/docs/atdd/process/path-keys.md` — referenced by Go code (not prompts).
   - `global/docs/atdd/process/{analysis,change/behavior}/*.md` — orphans (Items 7/8) that get inlined into prompts and never end up in the renamed tree.

**Decision:** rename to `runtime/references/atdd/` (and `runtime/references/code/`). Reasoning:

- **Architectural symmetry.** Three delivery mechanisms map to three sibling trees under `runtime/`. Today's split is historical accident, not intent.
- **`references/` fits the surviving content.** After Item 5's inlining, what's left is technical reference material (architecture descriptions, language tables) — read on demand by specific prompts. `doctrine/` (the original Item 2 author pick) no longer fits; the prose doctrine it referred to is gone.
- **The user-visible sync path is misleading.** `~/.gh-optivem/docs/` evokes human-readable documentation, but the synced content is agent fuel. Rename to `~/.gh-optivem/references/atdd/` makes the purpose honest for students opening the directory.

**Surfaces touched by the dated plan:**

- Source tree: move `internal/assets/global/docs/atdd/architecture/` → `internal/assets/runtime/references/atdd/architecture/`; move `internal/assets/code/` → `internal/assets/runtime/references/code/`.
- `internal/assets/embed.go` — update embed roots.
- `internal/assets/sync/sync.go`, `sync/materialize.go`, `sync/sync_test.go` — update source paths and target paths.
- ~25 `Read ${docs_root}/atdd/architecture/...` and `Read ${docs_root}/atdd/code/language-equivalents/...` lines across 10 prompt files.
- Sync target path: `~/.gh-optivem/docs/` → `~/.gh-optivem/references/atdd/` (or equivalent). Update any docs that mention the old path.
- Scaffolded-repo and shop-template expectations of the sync target shape.
- `path-keys.md` — decide in the dated plan whether it follows the rename (lives under `runtime/references/atdd/process/`?), gets a more honest home, or stays as-is.

**Sequencing:**

- Items 7 and 8 do not block this plan — their inlining is orthogonal (they leave the tree, not move within it).
- Plan `20260520-0907-runtime-shared-scope-injection.md` should land first so `scope.md` is firmly in `runtime/shared/` before this rename touches anything else.

**Possible follow-up:** one small dated plan covering the source-tree move + sync target rename + `Read` line updates.

---

### 3. Naming proposal: `runtime/doctrine/` vs `runtime/shared/` — RESOLVED (folded into Item 2)

**Status:** RESOLVED — folded into Item 2's decision. Canonical name is `runtime/references/atdd/` (with sibling `runtime/references/code/` for the language tables). Neither `doctrine/` nor `shared/` was picked:

- `doctrine/` no longer fits because the prose doctrine it referred to is gone (inlined into prompts per Item 5).
- `shared/` is already taken by `internal/assets/runtime/shared/` (universal argv-injected preamble + scope + session-end), which is a different delivery mechanism. Reusing the name would muddle the two trees.

`references/` accurately names the surviving content (architecture descriptions, language tables) by how it's consumed (read on demand by specific phases), and avoids collision.

---

### 4. Doctrine/prompt naming + structure mismatch — LARGELY DISCHARGED

**Status:** LARGELY DISCHARGED. The framing premise is gone: there is barely any doctrine tree left to mismatch against (`process/` is down to `path-keys.md` + the two orphans). Sub-bullets resolved:

- **Directory-shape mismatch (nested vs flat):** dissolved by Item 5's inlining sweep — no nested doctrine to compare.
- **`-backend` / `-frontend` 1:N fanout:** dissolved by Item 6's prompt-level merge.
- **Doctrine without prompt:** `at-refactor.md` and `acceptance-criteria-refinement.md` → tracked in Items 7/8 (promoted to companion plan). `system-implementation-change.md` is gone from the tree; naming-drift question handled by Item 9.
- **Prompt without doctrine:** under Item 5's "inline everything, no doctrine tree" model, this is no longer a mismatch — every prompt is self-contained by design.

**Residual 1 — `task-` prefix on two prompts:** folded into Item 9's rename plan. The only prompts carrying the `task-` prefix are `task-system-interface-redesign.md` and `task-external-system-interface-redesign.md`. All 12 other prompts (`at-*`, `ct-*`, `chore`, `fix-verify`, `disable-tests`, `enable-tests`) are bare. The Item 9 rename plan is already touching prompt filenames + agent names + `process-flow.yaml` agent references; adding the `task-` drops costs ~30 seconds of additional edits. Rename targets:

- `task-system-interface-redesign.md` → `system-interface-redesign.md`
- `task-external-system-interface-redesign.md` → `external-system-interface-redesign.md`
- Update `agent: task-system-interface-redesign` → `agent: system-interface-redesign` at `process-flow.yaml:1199` and similarly at line 1208.

**Residual 2 — CI consistency walk (noted, not promoted):** a small test that walks `process-flow.yaml`, asserts every `agent:` value resolves to an existing prompt file under `internal/assets/runtime/prompts/atdd/`, and every `phase_doc:` value resolves to an existing file. Would mechanically catch the dangling `phase_doc:` references flagged in Items 6, 9, and here (multiple lines in `process-flow.yaml` point to files that no longer exist post-Item-5). Worth doing eventually; not blocking anything; not worth a dedicated plan today. Pick up when someone hits another dangling-reference bug.

---

### 5. Inline everything; no separate doctrine tree — HEADLINE — DECIDED + LARGELY DISCHARGED

**Status:** DECIDED — the N=1/N≥2 framing was superseded by a wholesale "inline everything" decision. Most of the work is already landed; what's left is residual cleanup tracked in other items.

**Decision (actual):** the doctrine tree `internal/assets/global/docs/atdd/process/` dissolves entirely. There is **no separate doctrine tree** in the target state — every byte of doctrine flows through prompts. Two injection mechanisms cover the cases the old N=1/N≥2 rule tried to split:

- **Per-phase inlining** — phase-specific content (was N=1) is inlined directly into the per-phase prompt file under `internal/assets/runtime/prompts/atdd/`. The prompt is self-contained; no `Read ${docs_root}/...` round-trip.
- **Universal argv injection** — cross-cutting content (was N≥2) moves under `internal/assets/runtime/shared/` and is concatenated into every dispatched prompt at Go startup via the same mechanism `preamble.md` / `session-end.md` already use. One source of truth, but no runtime Read.

Net result: both injection paths produce prompts whose body already contains the doctrine the agent needs. The doctrine tree is gone; the user-facing `~/.gh-optivem/docs/` sync path shrinks accordingly.

**Discharge status:**

- Per-phase inlining sweep: largely landed (commits `2df45d3`, `4b44722` — placeholder plumbing for inlined prompts; subsequent commits sweep individual phase docs).
- `at-green-system-backend.md` + `at-green-system-frontend.md`: already merged into a single `at-green-system` phase (clears Item 6's dependency on this row).
- Universal argv injection for `scope.md` + `conventions.md`: in flight as `plans/20260520-0907-runtime-shared-scope-injection.md`.

**Residuals (still open after this item):**

- `path-keys.md` — last file under `process/` that isn't an orphan. Decide whether it follows `scope.md` into `runtime/shared/` (universal injection) or gets inlined per-phase, or stays as-is until its consumers are known.
- `at-refactor.md` — orphan, tracked in Item 7.
- `analysis/acceptance-criteria-refinement.md` — orphan, tracked in Item 8.
- `system-implementation-change.md` was an orphan and is gone from the tree; the naming-drift question survives in Item 9.
- `disable-tests.md` / `enable-tests.md` — gone from the tree (Item 10 was about whether they should have existed there at all; mostly moot now).

**Why this is the headline:** Items 1, 2, 3, 4 were all symptoms of treating thin-vs-fat as a project-wide convention. Under "inline everything," they collapse: Item 2's `runtime/doctrine/` rename never happens because no doctrine tree survives; Item 3's naming choice is moot; Item 4's structural-walk test reduces to "no doctrine docs should exist under `process/` except residuals."

**Possible follow-up:** no new plan from Item 5 itself — the work is discharged across `2df45d3`, `4b44722`, and `20260520-0907-runtime-shared-scope-injection.md`. The `path-keys.md` residual may warrant a small dated plan once its consumers are confirmed.

---

### 6. Collapse backend/frontend now; per-component fanout as follow-up — HEADLINE

**Status:** PARTIALLY DISCHARGED at the prompt level; structural collapse pending; per-component fanout deferred to a future plan.

**What's already done (prompt level):**

- `at-green-system-backend.md` and `at-green-system-frontend.md` are merged into a single `internal/assets/runtime/prompts/atdd/at-green-system.md`.
- The merged prompt has `scope: {}` (empty) and a TODO frontmatter comment marking the future `at-green-system` + `at-green-component` split as deferred.
- The body is generic (no backend/frontend mention).

**What's still live (state-machine level):**

- `process-flow.yaml:391-393` — top-level `AT_GREEN_SYSTEM` is a `call_activity` wrapping the `at_green_system` sub-process.
- `at_green_system` sub-process (lines 424-483) still has two sequential `call_activity` nodes — `AT_GREEN_BACKEND` and `AT_GREEN_FRONTEND` — both invoking the (already-parameterised) shared `green_phase_cycle` with different params:
  - backend: `suite: "<acceptance-api>"`, `phase_id: AT_GREEN_BACKEND`, `phase_label: "AT - GREEN - SYSTEM (backend)"`
  - frontend: `suite: "<acceptance-ui>"`, `phase_id: AT_GREEN_FRONTEND`, `phase_label: "AT - GREEN - SYSTEM (frontend)"`
- Both nodes run unconditionally — there is no architecture-conditional branching. Monolith projects still execute `AT_GREEN_FRONTEND` even when there is nothing to implement there.
- Both nodes reference `phase_doc: docs/atdd/process/change/behavior/at-green-system.md`, which no longer exists in the asset tree — drift from Item 5's inlining sweep.

**Immediate change (do now — small dated plan):**

Collapse the duality at the state-machine level. Replace the AT_GREEN_BACKEND + AT_GREEN_FRONTEND duality with a single node:

```
ENABLE_TESTS → AT_GREEN → COMMIT → TICK → MOVE_TICKET_IN_ACCEPTANCE → END
```

- One `call_activity` into `green_phase_cycle` with `agent: at-green-system`, no `<acceptance-api>` / `<acceptance-ui>` split (suite reduces to a single project-wide acceptance suite, or the suite param drops out — to be decided in the dated plan).
- `phase_doc:` drift gets cleaned up in the same edit — either drop the field or repoint it once the post-Item-5 prompt sourcing is locked.
- Always-runs-both bug disappears: the collapsed node runs once per AT cycle, regardless of architecture.
- `green_phase_cycle` parameterisation stays as-is — it is already the correct shape for either today's single-call use or future fanout.

**Follow-up plan (defer — separate dated plan):**

Per-component fanout — the original Item 6 architectural shift. Out of scope for the immediate collapse; promote when concrete multi-component requirements arrive.

- `gh-optivem.yaml` schema: `system.path` (singular) → `system.components: [{name, path, language?}, …]`.
- `process-flow.yaml`: turn the single `AT_GREEN` node into a parameterised loop over `system.components`. Requires the state-machine engine to support loop/fanout constructs (verify support before committing).
- `phase-scopes.yaml`: per-component scope baking. Folds in the deferred plan `plans/deferred/20260518-1530-multitier-green-scope.md` (which is narrowly about scope-key naming for the existing backend/frontend duality and only makes sense once the per-component shape lands).
- `suite:` labels: per-component naming.
- Shop template migration story.
- Per-phase applicability audit (rough cut): `at-green-system` yes; `at-red-test` no (shared DSL/test suite); `at-red-dsl` no; `at-red-system-driver` maybe; `at-refactor` maybe; `ct-*` almost certainly per-component.

**Consequences for sibling items in this scratchpad:**

- Item 11 (`AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` hardcoded) — discharged by the immediate collapse, not by parameterisation.
- Item 12 (`suite: "<acceptance-api>"` / `"<acceptance-ui>"` labels) — discharged by the immediate collapse (suite reduces to one).
- `plans/deferred/20260518-1530-multitier-green-scope.md` — its premise (multitier projects have AT_GREEN_BACKEND + AT_GREEN_FRONTEND as separate nodes) goes away after the collapse. The scope-key naming question only re-emerges inside the per-component fanout follow-up, at which point this deferred plan folds into that follow-up.

**Possible follow-up:** TWO dated plans — (a) the immediate collapse (cheap, can land now), (b) the per-component fanout (defer, sequence after concrete multi-component requirements + SSoT shape stabilisation).

---

### 7. `at-refactor.md` is fully orphaned — PROMOTED

**Status:** PROMOTED to `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md` — decision is "wire up," not "delete." Specifics (canonical framing, scope rules, prompt reader, state-machine wiring, inlining into the prompt) are tracked there.

**Original context:** zero references in any `.go`, `.yaml`, or other `.md`; the file was a DRAFT stub left over without agent wiring.

---

### 8. `acceptance-criteria-refinement.md` + entire `analysis/` orphan — PROMOTED

**Status:** PROMOTED to `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md` (same companion plan as Item 7). Decision is "wire up," not "delete."

**Original context:** zero references anywhere; the file is the only content under `process/analysis/`, so the subdir's fate follows this file's fate. Once wired up, the surviving location is the prompt tree under `internal/assets/runtime/prompts/atdd/` (per Item 5's inlining model) — the `analysis/` subdir under `process/` dissolves.

---

### 9. `chore` vs `system-implementation-change` — naming drift — DECIDED, ready for promotion

**Status:** DECIDED — canonical name is `system-implementation-refactoring`. Ready to promote to a small dated rename plan.

**Why this name (not `system-implementation-change`):** the prompt body explicitly states "no boundary or behavioral impact" — that's refactoring, not just generic change. Picking `-refactoring` also creates a clean symmetry with the sibling `change/structure/` subtypes:

- `system-interface-redesign` — interface change, system-side
- `external-system-interface-redesign` — interface change, external-side
- `system-implementation-refactoring` — implementation change, system-side

Each name now says **what** changed (interface vs implementation) and **what kind** of change (redesign vs refactoring). Generic `change` was uninformative.

**Surfaces to rename in the dated plan:**

- `internal/assets/runtime/prompts/atdd/chore.md` → `system-implementation-refactoring.md`
- Agent name inside `process-flow.yaml` (line 1235) and any other binding: `chore` → `system-implementation-refactoring`
- Prompt body wording: "Chore Agent" → "Implementation Refactoring Agent"; "CHORE - WRITE" phase label → "SYSTEM - IMPLEMENTATION - REFACTORING - WRITE" (or chosen short form)
- `change_type:` commit-prefix value at `process-flow.yaml:1234`: `CHORE` → `SYSTEM-IMPLEMENTATION-REFACTORING` (or chosen short form for commit messages)
- Routing condition at `process-flow.yaml:306`: `change_type == system-implementation-change` → `change_type == system-implementation-refactoring`
- Comment at `process-flow.yaml:273`: update mapping line
- `internal/steps/github_setup.go:80`: `subtype:system-implementation-change` → `subtype:system-implementation-refactoring`; description "Structural change to system internals (no test-stack artifact)" → "Refactoring of system internals (no boundary or behavioral change)"
- Documentation surfaces: `docs/process-diagram.md`, `docs/images/process-diagram-5-run-cycle.svg`, deferred plan `plans/deferred/20260518-2236-migrate-process-docs-hierarchy.md`, and any other references found by `grep -ri system-implementation-change`.

**Bonus drift cleaned up incidentally:**

- `process-flow.yaml:1236` references `phase_doc: docs/atdd/process/change/structure/system-implementation-change.md` — that file is gone (Item 5 inlining). The rename plan drops or repoints this field.
- `change_type:` is overloaded inside `process-flow.yaml` — at line 306 it carries the routing name, at line 1234 it carries the commit-prefix label. The rename plan should pick **one** convention (e.g., use the long kebab everywhere and let the commit step shorten if needed) rather than perpetuate the overload.

**Migration notes:**

- Existing GitHub issues labelled `subtype:system-implementation-change` need a label migration (or both labels recognised during a transition window). To be decided inside the dated plan.

**Possible follow-up:** small dated plan to do the rename. Sequence whenever; no architectural dependencies.

---

### 10. `disable-tests.md` / `enable-tests.md` are human-only docs — DISCHARGED

**Status:** DISCHARGED — the premise is obsolete. Both files are now real agent prompts, not human-only docs.

**What happened:** commit `b3b5952` ("atdd/disable-enable-tests: switch from deterministic Go to Haiku agents") converted both phases from `service_task` Go actions to `user_task` agent dispatches. Current state:

- `internal/assets/runtime/prompts/atdd/disable-tests.md` and `enable-tests.md` exist as Haiku agent prompts (model: haiku, effort: low, scope: {} since the work is mechanical per-language).
- Dispatched as agent nodes in `process-flow.yaml`: `agent: enable-tests` at line 429, `agent: disable-tests` at line 922.
- The old doctrine stubs under `global/docs/atdd/process/change/behavior/` were swept by Item 5's inlining.

Item 10's framing (carve-out vs move vs delete for "human-only docs") never had to be answered — a third option (make them agent-driven) landed instead.

**Related (not Item 10):** `plans/deferred/20260520-0002-deterministic-disable-enable-fallback.md` parks the idea of a deterministic-Go alternative as a future fallback if the Haiku approach proves flaky. Orthogonal — does not reopen Item 10.

---

### 11. `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` hardcoded in `process-flow.yaml` — RESOLVED (folded into Item 6)

**Status:** RESOLVED — folded into Item 6's two-plan split.

- **Immediate collapse plan** (Item 6) discharges Item 11: replaces the two sequential `call_activity` nodes (`AT_GREEN_BACKEND` + `AT_GREEN_FRONTEND` at `process-flow.yaml:432-456`) with a single `AT_GREEN` node calling `green_phase_cycle` once. No loop/fanout engine work required.
- **Deferred per-component fanout plan** (Item 6 follow-up) is where the "parameterised loop over `system.components`" question lives, if/when concrete multi-component requirements arrive.

No separate promotion needed for Item 11.

---

### 12. `suite: "<acceptance-api>"` / `"<acceptance-ui>"` labels also bake backend/frontend — RESOLVED (folded into Item 6)

**Status:** RESOLVED — folded into Item 6's two-plan split.

- **Immediate collapse plan** (Item 6) discharges Item 12: collapsing `AT_GREEN_BACKEND` + `AT_GREEN_FRONTEND` into a single `AT_GREEN` reduces the `suite:` param to one project-wide acceptance suite (or drops the param entirely — to be decided in the collapse plan based on what `run_targeted_tests` needs).
- **Deferred per-component fanout plan** (Item 6 follow-up) handles per-component suite naming if the component model lands.

No separate promotion needed for Item 12.

---

## Walking-order note

Discussion order should track the framing — resolve the headlines first, the symptoms collapse naturally:

1. **Item 5** (consolidation rule) — settles 1, 2, 3, 4 as side-effects.
2. **Item 6** (component-fanout) — settles 11, 12 as side-effects.
3. **Items 7, 8, 9, 10** — orphan/drift cleanup that needs the headlines' tree shape first.

Item 1 has already been decided in conversation (see its block above).
