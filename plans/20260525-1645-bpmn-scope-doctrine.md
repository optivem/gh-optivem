# BPMN scope vocabulary doctrine + open questions

> **Parent plan:** `plans/20260525-1057-bpmn-refactor-design.md` (design archive).
> **Trigger:** review of `plans/ideas/2-bpmn-refactor-mid-level.md` (MID brainstorm) surfaced that the `**Scopes:**` blocks had been authored with invented kebab-prose names rather than the canonical Family B keys already configured in `internal/atdd/phase-scopes.yaml` and `internal/projectconfig/paths_defaults.go::CanonicalPathKeys()`. This plan captured the open questions raised while fixing that, grouping them under one execution surface so they can be resolved together before Phase C YAML encoding locks them in.

## Context

Two canonical scope-vocabulary surfaces already exist in the repo:

- **`internal/projectconfig/paths_defaults.go::CanonicalPathKeys()`** — Family B keys (snake_case): `driver_port`, `driver_adapter`, `external_system_driver_port`, `external_system_driver_adapter`, `at_test`, `dsl_port`, `dsl_core`, `ct_test`.
- **`internal/atdd/phase-scopes.yaml`** — SSoT mapping BPMN phase id → list of layer names (Family B keys above + `system_path` from Family A).

Memory `feedback_substitutable_paths_in_docs` already pins the rule: *"path-shaped scope uses Family B placeholders, never prose."*

Q-numbering continues from the parent design plan's Q-series. Q40–Q46 resolved during the 2026-05-25 `/refine-plan` walk and `/execute-plan` session (see git history); Q41 + Q43 + Q45 C landed as commits, Q42 / Q44 / Q46 were no-op confirmations. The remaining items are below.

---

## Open questions

### Q40 — Rename canonical scope keys from snake_case to kebab-case  *(DOCTRINE / NAMING)*

**✅ Decided: A** — Rename all surfaces snake→kebab in one pass before Phase C YAML lock. Family A `system_path` → `system-path` included. Per Q29 "kebab-case everywhere" doctrine; per `feedback_teaching_repo_no_legacy`, no migrate-relocation pass needed (teachers regenerate gh-optivem.yaml).

**Context.** Q29 in the parent plan locked kebab-case as the universal naming scheme ("kebab-case everywhere — YAML keys, doc headings, prompt filenames, in-prose references, anchor slugs, Go struct tags"). The canonical scope keys in `CanonicalPathKeys()` and `phase-scopes.yaml` are snake_case (`at_test`, `dsl_core`, …) — they predate Q29 and were not included in its sweep.

**Surfaces affected by the rename.**

| Surface | Change |
|---|---|
| `internal/projectconfig/paths_defaults.go::CanonicalPathKeys()` | rename 8 keys: `at_test` → `at-test`, `dsl_core` → `dsl-core`, etc. |
| `internal/projectconfig/paths_defaults.go::pathStems()` | iteration order tied to key order — no string change, but key-iterating tests flip. |
| `internal/projectconfig/paths_defaults.go::ExternalDriverKeyRenames` | existing old→new map (`external_driver_port` → `external_system_driver_port`); may absorb the snake→kebab pass OR live alongside as a second map. |
| `internal/atdd/phase-scopes.yaml` | rewrite all phase entries (22 phases × 2-3 keys). |
| `internal/projectconfig/config.go` | validator Rule 22a — error messages + canonical-key check. |
| `internal/projectconfig/path-keys.md` | doctrine doc rewrite. |
| `internal/projectconfig/config_test.go`, `paths_defaults_test.go`, `internal/atdd/phase_scopes_test.go` | fixture strings update. |
| `gh-optivem.yaml` in every real project's `paths:` block | keys all rename — per memory `feedback_teaching_repo_no_legacy`, teachers regenerate configs; no migrate-relocation pass needed. |
| `Family A: system_path` | also kebab → `system-path` (decided yes per option A; Q29 applies). |
| Prompt frontmatter `scope:` keys (in `internal/assets/runtime/prompts/atdd/*.md`) | if prompts hard-code Family B keys, those rename too. |
| The five brainstorm files `plans/ideas/*.md` (LOW/MID/HIGH/CYCLE/TOP) | MID Scopes block rewrites snake → kebab. LOW has no Scopes blocks. HIGH/CYCLE/TOP have invocations but no Scopes. |
| `process-flow.yaml` (Phase C target) | not yet authored; will be written kebab from the start. |

**Options.**
- **(A) Rename all surfaces snake → kebab in one pass before Phase C YAML lock.** Single doctrine moment. Mechanical find/replace once the rename map is fixed. Q29 fully realized. Family A `system_path` → `system-path` for consistency.
- **(B) Rename everything except `system_path`.** Family B kebab, Family A stays snake. Justification: Family A is structurally distinct (single key, not a key set), so Q29's "everywhere" might not extend to it. Asymmetric but defensible.
- **(C) Defer the rename indefinitely.** MID/HIGH/CYCLE/TOP brainstorm + Phase C YAML use snake_case canonical. Q29 stays unfulfilled for Family B. Justification: rename is mechanical at any point, no urgency, churn cost not justified by clarity gain.

**Recommendation: A.** Q29's "everywhere" is explicit. Family A is the same scope vocabulary at a different level, so it goes too. Doing it once before Phase C YAML lock prevents writing the YAML twice. Find/replace is bounded. Per memory `feedback_teaching_repo_no_legacy`, the gh-optivem.yaml-in-real-projects ripple is "regenerate, don't migrate" — fits the teaching repo's no-legacy doctrine.

---

### Q47 — Invocation-context scope inheritance for `fix-*` (and other MID tasks?)  *(DOCTRINE — mechanism, deferred)*

**⏳ Deferred** — Captured during refinement of Q44. Resolving requires a parallel reconsideration of Q1 in the parent plan (LOW `fix` single-attempt, no recursion). Until Q1 is re-opened, Q44 = A holds.

**Context.** Q44 pinned `fix-*` MID-task scope as the 9-layer union of all writable layers. During refinement, an alternative model was proposed: **`fix-*` scope is a function of how it was invoked**, not a hardcoded union. Specifically:

- When `fix-*` is invoked via **`execute-agent`** (structured BPMN flow, called from a parent agent context): scope = inherit the parent's scope.
- When `fix-*` is invoked via **`execute-command`** (ad-hoc, no parent agent context): scope = wide / all layers (current Q44 default).

**Why this is appealing.**
- Least-privilege envelope on the structured-flow path (most common case).
- Recognizes `execute-command` invocations have no narrower context to inherit, so wide is correct there.
- Avoids the "future reviewer narrows the union" risk Q44 is trying to prevent — no hardcoded union to narrow.

**Why it doesn't dominate Q44 = A as-is.**
1. **Cross-layer root-cause problem.** When fix is invoked from CT-RED-DSL-CORE (scope `[dsl_core, external_system_driver_port]`) but the actual bug is in `system_path`, inherited-narrow scope locks fix out of the layer it needs. The structured-flow path — the *common* case — becomes broken by construction.
2. **Q1 is load-bearing.** Q1 locked single-attempt, no recursion. Under inherited-narrow scope, fix has no escalation path. Cross-layer bugs would just fail. Narrow inheritance is only viable if Q1 also admits escalation.
3. **Mechanism cost.** Phase C YAML needs to express both scope-as-function-of-invocation AND dispatch on invocation primitive type (`execute-agent` vs `execute-command`). Per `feedback_no_deferred_mechanism`, scope is pinned per phase, not computed per invocation pattern.
4. **`execute-command` rare.** Designing for the escape-hatch invocation pattern provides no win on the primary (structured-flow) path.

**Required prerequisite if reopened.** A Q1 revisit: should LOW `fix` admit escalation (e.g., "tried with narrow scope, failed, re-invoke with wider scope")? If Q1 stays "single attempt, no recursion," Q47's narrow-inheritance proposal is structurally incoherent and Q44 = A is the only consistent choice.

**Resolution.** Deferred until Q1 is re-examined. If/when Q1 admits escalation, Q47 becomes resolvable and may supersede Q44.

---

## Resolution order

Only Q40 remains actionable; Q47 is deferred indefinitely.

1. **Q40** (snake → kebab rename) — bounded find/replace once the rename map is locked. Best done in a fresh `/clear`-ed session due to surface count (~6 file edits + test fixtures + prompt frontmatter + brainstorm Scopes blocks). Sequence after `plans/20260525-1659-bpmn-acceptance-test-rename.md` (Q-new-6) and `plans/20260525-1710-bpmn-name-consistency-audit.md` (Q45 C) — both leave Q40's surfaces unchanged but may rewrite adjacent text; doing Q40 last avoids re-baselining.

## Items

- **Q40:** ~6 file-edit items + a fixture-test pass; bounded find/replace once the rename map is locked. Author the per-file items via `/refine-plan` at the start of the fresh session.

## Cross-references

- Parent design plan: `plans/20260525-1057-bpmn-refactor-design.md`
- Phase C execution plan: `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`
- Sibling rename plan: `plans/20260525-1659-bpmn-acceptance-test-rename.md` (Q-new-6 HIGH renames)
- Spawned by Q45 C: `plans/20260525-1710-bpmn-name-consistency-audit.md`
- Memory `feedback_substitutable_paths_in_docs` — path scopes use Family B placeholders, never prose.
- Memory `feedback_teaching_repo_no_legacy` — Q40 doesn't need a migrate-relocation pass; teachers regenerate.
- Memory `feedback_no_deferred_mechanism` — Q47 reasoning: scope is pinned per phase, not computed per invocation pattern.
