# 2026-05-19 15:37 UTC — Post-meta-bpmn discussion topics

**Status:** PARKED — do not pick up until `20260519-0929-meta-bpmn-ssot-coordination.md` is fully landed.

**Purpose:** scratchpad for topics, remarks, and follow-up items the user wants to raise AFTER the meta-bpmn coordination plan finishes. Each item here is a discussion seed, not a refined plan — items get promoted to their own dated plan file once discussed.

**Cross-reference:** this plan sits downstream of `20260519-0929-meta-bpmn-ssot-coordination.md`. That coordination plan covers the BPMN orchestration + SSoT + vocabulary cluster (plans `20260518-1144`, `20260518-1530`, `20260518-1742`, `20260518-2236`, `20260519-0704`). Nothing in here should preempt or duplicate items already tracked there.

---

## Framing

Items 1–12 below all surfaced from the same root question: **what is the contract between `internal/assets/global/docs/atdd/process/` (doctrine) and `internal/assets/runtime/prompts/atdd/` (prompts), and why is so much of it implicit?**

The two headline items are:

- **Item 5 — consolidation rule:** inline doctrine into prompt when N=1 reader; keep separate when N≥2.
- **Item 6 — component-based fanout:** replace hardcoded backend/frontend with parameterised per-component phases.

Items 1–4 and 7–12 are mostly symptoms / drift that the two headlines resolve or expose. Walk them one at a time; promote each to its own dated plan once discussed.

---

## Topics to discuss

### 1. `## Scope` section in phase docs duplicates frontmatter

**Status:** DECIDED — drop the layer enumeration, keep the behavioral framing.

**Context:** phase docs like `at-green-system.md` and `at-red-test.md` have a `## Scope` prose section that re-states which layers the phase touches. Scope already lives in (a) prompt frontmatter `scope:`, (b) `phase-scopes.yaml`, (c) `gh optivem process scope <PHASE>`. The prose section is a fourth surface that drifts.

**Decision:** the section has two jobs:

- Job 1 — restate allowed paths (pure duplication of `phase-scopes.yaml`). **Drop.**
- Job 2 — behavioral framing ("Tests/DSL/drivers are frozen during GREEN; needing to touch a frozen layer signals an earlier RED phase was wrong; stop and ask"). **Keep**, because this is not derivable from frontmatter and changes how the agent reasons about scope violations.

Rename `## Scope` → `## Frozen layers` (or similar) and let each phase's section say something genuinely useful instead of restating boilerplate.

**Possible follow-up:** promote to a small plan that rewrites the `## Scope` section across all `change/behavior/*.md` and `change/structure/*.md` phase docs.

---

### 2. `global/` vs `runtime/` split is by delivery mechanism, naming is misleading

**Context:** the asset tree has two roots:

- `internal/assets/runtime/prompts/atdd/*.md` — passed to `claude -p` over argv. Never hits disk.
- `internal/assets/global/docs/atdd/*.md` — synced to `~/.gh-optivem/docs/atdd/` because the prompt does `Read ${docs_root}/...`, which needs a real filesystem path.

Both are agent-consumed. The split is **delivery mechanism** (argv vs disk-sync), not audience. The names `global/`, `docs/`, and the user-visible sync path `~/.gh-optivem/docs/` all evoke "human-facing documentation," which is misleading.

**Remark:** this item partly collapses once Item 5's consolidation rule lands — what remains in the doctrine tree is just the genuinely-shared docs (`scope.md`, `conventions.md`, `path-keys.md`, `at-green-system.md` if it stays 1:N). The remaining name only has to cover "cross-cutting shared rules."

**Naming proposals for the shared-only remnant:**

- (a) `runtime/doctrine/atdd/` — paired with `runtime/prompts/atdd/`. "Doctrine" is already in `scope.md` vocabulary. **Author's pick.**
- (b) `runtime/shared/atdd/` — literal, neutral.
- (c) `runtime/phase-docs/atdd/` — matches `phase_doc:` field in `process-flow.yaml`, but slightly inaccurate once shared cross-cutting docs land alongside.

Sync target: drop `docs` from `~/.gh-optivem/docs/` too — e.g. `~/.gh-optivem/doctrine/atdd/`.

**Possible follow-up:** sequence after Item 5; rename touches `sync.go`, `embed.go`, every `Read ${docs_root}/...` line, scaffolded repo paths, and the shop template.

---

### 3. Naming proposal: `runtime/doctrine/` vs `runtime/shared/`

**Context:** subsumed by Item 2. Tracked separately only because the naming choice is the actionable bit; the rest of Item 2 is rationale.

**Remark:** pick a name before the rename plan is written so the executor isn't bikeshedding mid-flight.

**Possible follow-up:** decision lives inside Item 2's promoted plan.

---

### 4. Doctrine/prompt naming + structure mismatch

**Context:** comparing `global/docs/atdd/process/` and `runtime/prompts/atdd/`, several mismatches exist:

- **Directory shape:** doctrine is nested (`analysis/`, `change/behavior/`, `change/structure/`, `shared/`); prompts are flat. Hierarchy is dropped on the prompt side.
- **1:N fanout:** `at-green-system.md` → `at-green-system-backend.md` + `at-green-system-frontend.md`. The `-backend`/`-frontend` suffix convention isn't documented anywhere.
- **Doctrine without prompt:** `at-refactor.md`, `system-implementation-change.md`, `analysis/acceptance-criteria-refinement.md`. Are these human-only, planned-but-orphaned, or drift? (See also items 7, 8.)
- **Prompt without doctrine:** `chore.md`, `fix-verify.md`. Utility prompts that don't fit the phase-doc model — but undocumented.
- **Naming inconsistency:** behavior phases are bare (`at-red-test.md` ↔ `at-red-test.md`); structure phases get `task-` prefix on the prompt (`external-system-interface-redesign.md` → `task-external-system-interface-redesign.md`). Either both should have it or neither.

**Remark:** root cause is no enforced 1:1 mapping or naming convention between the two trees. A small test walking both trees and asserting the relationship (allowing for documented fanout + utility exceptions) would surface drift mechanically.

**Possible follow-up:** much of this dissolves once Items 5 (consolidation) and 6 (component-fanout) land. What remains: a structural-walk test for the surviving doctrine ↔ prompt pairs.

---

### 5. Consolidation rule: inline doctrine when N=1, keep separate when N≥2 — HEADLINE

**Context:** the "thin agent + reference doc" pattern has a cost (two files, scope duplication, naming drift, indirection) and a benefit (single source of truth across N readers). When N=1, you pay the cost without earning the benefit.

**Decision (preliminary, user-validated direction):** the rule writes itself — keep separate when N ≥ 2 readers; inline when N = 1.

**Applied to current state:**

| Doc | Readers | Action |
|---|---|---|
| `shared/scope.md` | many | keep separate |
| `shared/conventions.md` | many | keep separate |
| `shared/path-keys.md` | many | keep separate |
| `change/behavior/at-green-system.md` | backend + frontend (2) | keep separate (but see Item 6 — collapses to 1 reader under component-fanout) |
| `change/behavior/at-red-test.md` | 1 prompt | inline |
| `change/behavior/at-red-dsl.md` | 1 prompt | inline |
| `change/behavior/at-red-system-driver.md` | 1 prompt | inline |
| All `ct-*` behavior docs | 1 prompt each | inline |
| `change/behavior/disable-tests.md`, `enable-tests.md` | 0 (verify) | see Item 10 |
| `change/structure/*` (except `system-implementation-change.md`) | 1 prompt each | inline |
| `change/behavior/at-refactor.md` | 0 | see Item 7 |
| `change/structure/system-implementation-change.md` | 0 | see Item 9 |
| `analysis/acceptance-criteria-refinement.md` | 0 | see Item 8 |

**Resulting shape (drops `global/` entirely):**

```
runtime/
  prompts/atdd/    # self-contained per-phase prompts (frontmatter + content)
  doctrine/atdd/   # scope.md, conventions.md, path-keys.md (+ at-green-system.md unless Item 6 absorbs it)
```

**Why this is the headline:** Items 1, 2, 3, 4 are all symptoms of treating thin-vs-fat as a project-wide convention instead of a per-doc judgment call. Resolve this and most of them dissolve.

**Possible follow-up:** promoted plan that (a) walks each phase doc and inlines into its prompt, (b) leaves the genuinely-shared docs in the renamed `doctrine/` tree, (c) updates the structural-walk test (Item 4) to assert the new contract.

---

### 6. Component-based fanout instead of hardcoded backend/frontend — HEADLINE

**Context:** `at-green-system-backend.md` + `at-green-system-frontend.md` bakes in a binary split that doesn't generalise (microservices, mobile + web + admin, multi-service systems all break it).

**Proposal:** replace with parameterised per-component phase.

- `at-green-component.md` — per-component implementer prompt. Takes a `component:` parameter, resolves scope to that component's paths, does the work for that slice.
- `at-green-system.md` — either the single-component case (when the project has no fanout) OR a meta-orchestrator that fans out N `at-green-component` invocations. The orchestrator (statemachine) likely owns the fanout, not the prompt.

**Per-phase applicability (quick pass — needs proper audit):**

- `at-green-system` — yes, system code lives in each component
- `at-red-test` — usually one shared DSL/test suite → no
- `at-red-dsl` — single DSL layer → no
- `at-red-system-driver` — maybe (per-component if each has its own driver)
- `at-refactor` — maybe needs both shapes (local vs structural)
- `ct-*` (component tests) — almost certainly per-component (the name says so)

**Downstream cost (non-trivial):**

- `gh-optivem.yaml` schema: `system.path` (singular) → `system.components: [{name, path, language?}, ...]`
- `phase-scopes.yaml` resolution: per-component scope baking
- The whole SSoT plan (`20260518-1530`) which currently bakes a single `sut_namespace`
- `process-flow.yaml`: replace `AT_GREEN_BACKEND` + `AT_GREEN_FRONTEND` static `call_activity` nodes (lines 415-437) with a parameterised loop over `system.components` (see Item 11)
- `suite:` labels currently `<acceptance-api>` / `<acceptance-ui>` (lines 422 + 434) — need per-component naming (see Item 12)
- Shop template (currently backend+frontend) — migration story for existing scaffolded repos

**Remark:** this is an architectural shift, not a rename. Belongs strictly **after** meta-bpmn lands and SSoT shape stabilises — trying to layer it in now risks invalidating decisions still being made in `20260518-1530`.

**Possible follow-up:** dedicated plan, sequenced after meta-bpmn + Item 5. Groups naturally with Items 11, 12 (its scope-expansion consequences).

---

### 7. `at-refactor.md` is fully orphaned

**Context:** zero references in any `.go`, `.yaml`, or other `.md`. Only mention is in `plans/20260518-2236-migrate-process-docs-hierarchy.md` calling it "new addition." No state-machine binding, no prompt, no Go code reads it.

**Remark:** either dead, or planned-but-orphaned. Decide which before Item 5's consolidation pass touches it (otherwise the consolidation walker has nowhere to put it).

**Possible follow-up:** quick investigation — was it staged for a future plan, or left over from a removed flow? Delete or wire up.

---

### 8. `acceptance-criteria-refinement.md` + entire `analysis/` orphan

**Context:** same situation as Item 7 — zero references anywhere. The whole `analysis/` subdir under `global/docs/atdd/process/` is read by nothing.

**Remark:** likely meta-agent or human-only doc that never got wired up. Same decision as Item 7.

**Possible follow-up:** investigate, then delete or wire up. If the `analysis/` subdir is intended for meta-agent prompts (e.g. acceptance-criteria refinement before AT_RED phases), that's a separate scope-expansion item.

---

### 9. `chore` vs `system-implementation-change` — four names for the same thing

**Context:** for one underlying concept the codebase uses four different names:

- The prompt is named `chore.md`
- The doctrine doc is `system-implementation-change.md` (in `change/structure/`)
- The state machine `change_type:` label is `CHORE` (`process-flow.yaml:1136`)
- The user-facing label (`github_setup.go:80`) and gate prompts call it `subtype:system-implementation-change`

The user types `subtype:system-implementation-change`, sees `CHORE` in commit messages, the prompt is `chore.md`, the doctrine is `system-implementation-change.md`.

**Remark:** pick one canonical name and propagate. This is naming drift, not architecture — should be cheap to fix once the canonical pick is made.

**Possible follow-up:** small dated plan to unify. Likely lands after Item 5 (which decides whether `system-implementation-change.md` survives at all — if it inlines into `chore.md` under N=1, the naming question reduces to "what do we call the resulting single prompt").

---

### 10. `disable-tests.md` / `enable-tests.md` are human-only docs

**Context:** the state machine uses `service_task` actions (`disable_change_driven` at line 879, `enable_change_driven` at line 412) — Go-coded actions, no agent invocation, no prompt. Both doctrine docs have no prompt and are not Read by anything else. Pure human reference for "what the disable/enable service tasks do."

**Remark:** directly contradicts the "docs are agent-only, not for humans" framing established earlier in this conversation. Two options:

- (a) Accept they're exceptions — explicit "human reference" carve-out in whatever tree they live in.
- (b) Move them — they don't belong in a runtime-asset tree at all; move to `docs/atdd/` (the actual human-facing tree) or delete if the service task code is self-documenting.

**Possible follow-up:** decision lives inside Item 5's plan, or as a tiny separate move once Item 5 picks the new tree shape.

---

### 11. `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` hardcoded in `process-flow.yaml`

**Context:** the backend/frontend split isn't just in prompt filenames — `process-flow.yaml:415-437` declares two sequential `call_activity` nodes. So Item 6's "switch to per-component" touches the state machine structurally, not just prompts.

**Remark:** roughly, turn the two static nodes into one parameterised loop over `system.components`. Requires the state-machine engine to support loop/fanout constructs, which may or may not exist today.

**Possible follow-up:** scope-expansion of Item 6's plan. Investigate state-machine fanout support before committing to the component model.

---

### 12. `suite: "<acceptance-api>"` / `"<acceptance-ui>"` labels also bake backend/frontend

**Context:** `process-flow.yaml` lines 422 + 434 use `suite: "<acceptance-api>"` for backend, `suite: "<acceptance-ui>"` for frontend. The angle-bracket placeholders evoke "project-resolved," but the names themselves assume the backend/frontend split.

**Remark:** per-component model needs per-component suite names. Symptom of Item 6, not an independent problem.

**Possible follow-up:** folded into Item 6's plan.

---

## Walking-order note

Discussion order should track the framing — resolve the headlines first, the symptoms collapse naturally:

1. **Item 5** (consolidation rule) — settles 1, 2, 3, 4 as side-effects.
2. **Item 6** (component-fanout) — settles 11, 12 as side-effects.
3. **Items 7, 8, 9, 10** — orphan/drift cleanup that needs the headlines' tree shape first.

Item 1 has already been decided in conversation (see its block above).
