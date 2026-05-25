# BPMN scope vocabulary doctrine + open questions

> **Parent plan:** `plans/20260525-1057-bpmn-refactor-design.md` (design archive).
> **Trigger:** review of `plans/ideas/2-bpmn-refactor-mid-level.md` (MID brainstorm) surfaced that the `**Scopes:**` blocks had been authored with invented kebab-prose names rather than the canonical Family B keys already configured in `internal/atdd/phase-scopes.yaml` and `internal/projectconfig/paths_defaults.go::CanonicalPathKeys()`. The MID file's Scopes were rewritten in place against the existing snake_case canonical keys (commit pending). This plan captures the open questions that surfaced while doing that and groups them under one execution surface so they can be resolved together before Phase C YAML encoding locks them in.

## Context

Two canonical scope-vocabulary surfaces already exist in the repo:

- **`internal/projectconfig/paths_defaults.go::CanonicalPathKeys()`** — Family B keys (snake_case): `driver_port`, `driver_adapter`, `external_system_driver_port`, `external_system_driver_adapter`, `at_test`, `dsl_port`, `dsl_core`, `ct_test`.
- **`internal/atdd/phase-scopes.yaml`** — SSoT mapping BPMN phase id → list of layer names (Family B keys above + `system_path` from Family A).

Memory `feedback_substitutable_paths_in_docs` already pins the rule: *"path-shaped scope uses Family B placeholders, never prose."* The MID file violated this; that violation is now fixed. This plan resolves the remaining questions raised during the review.

Q-numbering continues from the parent design plan's Q-series (last Q was Q34 in the parent + Q-new-5; this plan uses Q40+ to avoid collision).

---

## Open questions

### Q40 — Rename canonical scope keys from snake_case to kebab-case  *(DOCTRINE / NAMING)*

**Context.** Q29 in the parent plan locked kebab-case as the universal naming scheme ("kebab-case everywhere — YAML keys, doc headings, prompt filenames, in-prose references, anchor slugs, Go struct tags"). The canonical scope keys in `CanonicalPathKeys()` and `phase-scopes.yaml` are snake_case (`at_test`, `dsl_core`, …) — they predate Q29 and were not included in its sweep. The MID file's Scopes were just rewritten in snake_case to match the existing config; a follow-up rename would flip everything kebab.

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
| `Family A: system_path` | also kebab → `system-path`? Or leave Family A untouched? *(sub-question — `system_path` is structurally distinct from Family B)*. |
| Prompt frontmatter `scope:` keys (in `internal/assets/runtime/prompts/atdd/*.md`) | if prompts hard-code Family B keys, those rename too. |
| The five brainstorm files `plans/ideas/*.md` (LOW/MID/HIGH/CYCLE/TOP) | MID Scopes block rewrites snake → kebab. LOW has no Scopes blocks. HIGH/CYCLE/TOP have invocations but no Scopes. |
| `process-flow.yaml` (Phase C target) | not yet authored; will be written kebab from the start if Q40 = yes. |

**Options.**
- **(A) Rename all surfaces snake → kebab in one pass before Phase C YAML lock.** Single doctrine moment. Mechanical find/replace once the rename map is fixed. Q29 fully realized. Family A `system_path` → `system-path` for consistency.
- **(B) Rename everything except `system_path`.** Family B kebab, Family A stays snake. Justification: Family A is structurally distinct (single key, not a key set), so Q29's "everywhere" might not extend to it. Asymmetric but defensible.
- **(C) Defer the rename indefinitely.** MID/HIGH/CYCLE/TOP brainstorm + Phase C YAML use snake_case canonical. Q29 stays unfulfilled for Family B. Justification: rename is mechanical at any point, no urgency, churn cost not justified by clarity gain.

**Recommendation: A.** Q29's "everywhere" is explicit. Family A is the same scope vocabulary at a different level, so it goes too. Doing it once before Phase C YAML lock prevents writing the YAML twice. Find/replace is bounded. Per memory `feedback_teaching_repo_no_legacy`, the gh-optivem.yaml-in-real-projects ripple is "regenerate, don't migrate" — fits the teaching repo's no-legacy doctrine.

---

### Q41 — `ticket` scope vocabulary  *(DOCTRINE — non-path scope)*

**Context.** Two MID tasks — `refine-acceptance-criteria` and `update-ticket` — declare `**Scopes:** - ticket`. `ticket` is **not** a path key in `phase-scopes.yaml`; that file's vocabulary is exclusively path-based (Family B + Family A). The ticket is a GitHub issue (or Jira ticket per memory `feedback_naming_github_jira_first`), not a file in the repo. The existing `phase-scopes.yaml` already notes the exemption: *"every writing-agent phase id in process-flow.yaml is either listed here OR declares `scope: none` in its prompt frontmatter (the doctrinal exemption for agents that mutate only inter-phase artifacts or external systems — see runtime/shared/scope.md)."*

**Options.**
- **(A) Drop the `ticket` scope; both tasks declare `**Scopes:** NONE`.** Treats ticket-mutating agents as the "external systems" exemption already documented in `phase-scopes.yaml`. No new vocabulary needed. The scope rule the runtime enforces (diff vs. allowed paths) doesn't apply — ticket mutation happens via GitHub/Jira API, not file edits in the repo.
- **(B) Extend `phase-scopes.yaml` to admit a non-path vocabulary** (e.g., `external:ticket`, `external:slack`, …). Adds a second scope category for external-system mutations. Bigger doctrine change — requires runtime/validator updates to recognize the `external:*` prefix and skip the diff-vs-paths check for those entries.
- **(C) Leave `- ticket` as-is and resolve during Phase C YAML encoding.** Defers the doctrine call. Risk: Phase C encoder hits the same question and has to ping back here.

**Recommendation: A.** Cleanest. The existing exemption already covers this case verbatim ("external systems"). Adding a non-path vocabulary (option B) is a bigger surface change without a corresponding benefit — the runtime can't validate "did the agent only modify the ticket" without inspecting GitHub state, and a fake-named scope provides false specificity. The MID brainstorm's `- ticket` was an honest attempt to declare *something* in the Scopes slot; dropping it is faithful to the existing doctrine.

---

### Q42 — `implement-dsl` MID task collapses two phases — scope union vs split  *(MID)*

**Context.** Per Q28 prompt rename table in the parent plan, two existing prompts collapse into one MID task: `at-red-dsl.md` + `ct-red-dsl.md` → `implement-dsl.md`. The existing config keeps the phase scopes separate:
- `AT_RED_DSL: [dsl_core, driver_port]`
- `CT_RED_DSL: [dsl_core, external_system_driver_port]`

The MID task's Scopes block declares the **union**: `dsl_core, driver_port, external_system_driver_port`. When called from AT context, the task's scope envelope technically permits writes to `external_system_driver_port` (which it shouldn't touch). When called from CT context, it permits writes to `driver_port` (which it shouldn't touch). Loose-but-correct — the prompt itself reads the `tests:` parameter and only touches the relevant port; the scope envelope just doesn't constrain that.

**Options.**
- **(A) Keep collapsed task with union scope (current state).** Single MID task, scope envelope is the union of the two source phases. Accept loose scope at the envelope; rely on the prompt to stay narrow per-call.
- **(B) Split into `implement-dsl-acceptance` / `implement-dsl-contract`.** Two MID tasks, each with precise scope. Restores per-call scope precision. Cost: two MID tasks where Q28 deliberately picked one; reintroduces phase-coding in the task name (mild violation of `feedback_no_layer_coding_in_names`).
- **(C) Make scope a function of the input parameter.** The MID task's Scopes block becomes conditional on the `tests:` parameter (e.g., `tests: acceptance` → `[dsl_core, driver_port]`; `tests: contract` → `[dsl_core, external_system_driver_port]`). Requires Phase C YAML to support parameter-conditional scope expressions.

**Recommendation: A.** Q28's collapse was deliberate. Per-call scope precision is a "nice-to-have" — the actual mutation surface is governed by the prompt body, and the envelope only matters when the agent strays. The runtime's scope-diff check is a guardrail, not a fine-grained access control. Loose union scope is the minimum-mechanism choice that respects Q28. (C) is the structurally cleanest answer but requires Phase C YAML mechanism not currently scoped.

---

### Q43 — `refactor-tests` MID task — confirm scope envelope  *(MID — net-new task)*

**Context.** MID task `refactor-tests` (used by HIGH `refactor-and-verify-tests`, called from CYCLE `refactor-test-structure`) is net-new — `phase-scopes.yaml` has no equivalent phase. Its Scopes were just set to `at_test, ct_test` (symmetric with `disable-tests` / `enable-tests`). But test refactoring might legitimately touch DSL (extracting test helpers into the DSL surface) or testkit files (driver-port helpers).

**Options.**
- **(A) Narrow: `at_test, ct_test`.** Pure test-file refactoring only. Helper extraction to DSL/testkit happens as a separate task. Current state.
- **(B) Wider: `at_test, ct_test, dsl_port, dsl_core`.** Allow DSL surface evolution as part of test refactoring (common in practice — pulling test-helper logic up).
- **(C) Widest: `at_test, ct_test, dsl_port, dsl_core, driver_port, driver_adapter, external_system_driver_port, external_system_driver_adapter`.** All test-side layers, including testkit. Permits any structural refactoring within the test stack.

**Recommendation: B.** Test refactoring routinely pulls helpers into DSL — narrowing to `at_test, ct_test` only forbids the common case. But driver/external testkit refactoring is structurally different (different sub-cycle), so (C) over-includes.

---

### Q44 — `fix-*` scope envelope as pinned doctrine  *(MID — already decided, pin for permanence)*

**Context.** User confirmed during the review that `fix-unexpected-passing-tests` and `fix-unexpected-failing-tests` should declare the union of every writable layer ("EVERYTHING that is under tests, including system"). Scope is currently set to all 9 keys (`at_test`, `ct_test`, `dsl_port`, `dsl_core`, `driver_port`, `driver_adapter`, `external_system_driver_port`, `external_system_driver_adapter`, `system_path`).

This Q exists only to **pin the doctrine** so future reviewers don't propose narrowing. A naive review would see "9-layer scope = too broad, narrow it." The decision is deliberate: a fix may need to touch any layer the change cycle touched, and the cycle as a whole spans every layer.

**Options.**
- **(A) Pin: fix-* scope = union of all writable layers. Cannot narrow.** Document in this plan + as a comment in `phase-scopes.yaml` when the fix phases land there.
- **(B) Narrow per-context.** Fix scope is the scope of the cycle that invoked it. Requires fix to know its caller, which the current LOW `fix` primitive doesn't support (`fix` takes a `failure` payload, not caller-cycle context).
- **(C) No pinning needed — current state stands without doctrine annotation.** Risk: future reviewer narrows it without realizing the union was intentional.

**Recommendation: A.** User already chose the union scope. Pinning the doctrine prevents drift. (B) requires LOW primitive changes that Q1 in the parent plan ruled against ("single attempt, no recursion").

---

### Q45 — HIGH file Scopes audit needed?  *(HIGH)*

**Context.** HIGH orchestrations (`write-and-verify-tests`, `implement-and-verify-dsl`, `implement-and-verify-system`, `refactor-and-verify-tests`, `implement-test-layer`, …) don't declare `**Scopes:**` blocks in `plans/ideas/3-bpmn-refactor-high-level.md` — they're orchestrations, not agent tasks. They call MID tasks (which carry the scope) and command tasks (which have no scope).

Question: does the HIGH file need any audit equivalent to the MID one, or does scope flow purely through MID-task invocations?

**Options.**
- **(A) No HIGH-file audit needed.** Orchestrations have no scope; scope lives only on agent (MID) tasks. The runtime computes the effective scope envelope from the invoked MID task's declaration, not from the orchestration that wrapped it.
- **(B) HIGH orchestrations declare an aggregated scope** equal to the union of called MID tasks' scopes. Redundant (computable from the call graph) but possibly useful for documentation / static analysis.
- **(C) Audit HIGH file for other contract drift** (Inputs/Outputs/Steps naming consistency vs MID), even though Scopes don't apply. E.g., naming consistency: HIGH has `implement-and-verify-external-system-driver-adapter-contract-tests` (singular `adapter`) while MID is `implement-external-system-driver-adapters` (plural). Worth pinning consistency now before Phase C.

**Recommendation: A + C.** No scope declarations on HIGH; but do a separate pass for naming-consistency / Inputs / Outputs drift between MID and HIGH (and CYCLE and TOP). Track that as a separate item in the next plan iteration, not in this scope-doctrine plan.

---

### Q46 — CYCLE and TOP files — equivalent Scopes audit?  *(CYCLE / TOP)*

**Context.** Same shape as Q45 but for CYCLE (`plans/ideas/4-bpmn-refactor-cycle-level.md`) and TOP (`plans/ideas/5-bpmn-refactor-top-level.md`). These are orchestrations, like HIGH — no agent tasks at this level (per Q-new-4 template, MID is the lowest agent-task layer).

**Options.**
- **(A) No CYCLE/TOP Scopes audit needed.** Same reasoning as Q45-A.
- **(B) Audit CYCLE/TOP for any accidental Scopes blocks that were authored** despite the template saying agent-tasks-only. Cleanup pass.

**Recommendation: B.** Quick verification pass; if no Scopes blocks exist in CYCLE/TOP, this resolves to a no-op confirmation.

---

## Resolution order

Suggested order if Q40–Q46 are batch-resolved:

1. **Q41** (ticket scope) — small, isolated, unblocks final MID edits.
2. **Q42–Q44** (MID per-task confirmations) — small, isolated; current state is the recommended option for each.
3. **Q45–Q46** (HIGH/CYCLE/TOP audit) — small, isolated; resolves to no-op or short follow-up.
4. **Q40** (snake → kebab rename) — largest. Do last so MID/HIGH/CYCLE/TOP are Scope-stable before the global rename.

## Items

After Q40–Q46 are resolved, this plan grows execution items proportional to the chosen options (e.g., Q40=A → ~6 file-edit items + a fixture-test pass; Q41=A → 2 MID edits to drop `- ticket`). Item authoring is deferred until question batch resolves.

## Cross-references

- Parent design plan: `plans/20260525-1057-bpmn-refactor-design.md`
- Phase C execution plan: `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`
- Memory `feedback_substitutable_paths_in_docs` — path scopes use Family B placeholders, never prose.
- Memory `feedback_no_layer_coding_in_names` — Q42 option B mildly violates this.
- Memory `feedback_naming_github_jira_first` — Q41 reasoning: tickets are GitHub/Jira, external-system territory.
- Memory `feedback_teaching_repo_no_legacy` — Q40 doesn't need a migrate-relocation pass; teachers regenerate.
