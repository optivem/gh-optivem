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

**✅ Decided: A** — Rename all surfaces snake→kebab in one pass before Phase C YAML lock. Family A `system_path` → `system-path` included. Per Q29 "kebab-case everywhere" doctrine; per `feedback_teaching_repo_no_legacy`, no migrate-relocation pass needed (teachers regenerate gh-optivem.yaml).

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

**✅ Decided: A** — Drop `- ticket` on both MID tasks (`refine-acceptance-criteria`, `update-ticket`); declare `**Scopes:** NONE`. Uses existing external-systems exemption in `phase-scopes.yaml`. No new vocabulary. Per `feedback_drop_dont_relocate` — upstream mechanism already covers it.

**Context.** Two MID tasks — `refine-acceptance-criteria` and `update-ticket` — declare `**Scopes:** - ticket`. `ticket` is **not** a path key in `phase-scopes.yaml`; that file's vocabulary is exclusively path-based (Family B + Family A). The ticket is a GitHub issue (or Jira ticket per memory `feedback_naming_github_jira_first`), not a file in the repo. The existing `phase-scopes.yaml` already notes the exemption: *"every writing-agent phase id in process-flow.yaml is either listed here OR declares `scope: none` in its prompt frontmatter (the doctrinal exemption for agents that mutate only inter-phase artifacts or external systems — see runtime/shared/scope.md)."*

**Options.**
- **(A) Drop the `ticket` scope; both tasks declare `**Scopes:** NONE`.** Treats ticket-mutating agents as the "external systems" exemption already documented in `phase-scopes.yaml`. No new vocabulary needed. The scope rule the runtime enforces (diff vs. allowed paths) doesn't apply — ticket mutation happens via GitHub/Jira API, not file edits in the repo.
- **(B) Extend `phase-scopes.yaml` to admit a non-path vocabulary** (e.g., `external:ticket`, `external:slack`, …). Adds a second scope category for external-system mutations. Bigger doctrine change — requires runtime/validator updates to recognize the `external:*` prefix and skip the diff-vs-paths check for those entries.
- **(C) Leave `- ticket` as-is and resolve during Phase C YAML encoding.** Defers the doctrine call. Risk: Phase C encoder hits the same question and has to ping back here.

**Recommendation: A.** Cleanest. The existing exemption already covers this case verbatim ("external systems"). Adding a non-path vocabulary (option B) is a bigger surface change without a corresponding benefit — the runtime can't validate "did the agent only modify the ticket" without inspecting GitHub state, and a fake-named scope provides false specificity. The MID brainstorm's `- ticket` was an honest attempt to declare *something* in the Scopes slot; dropping it is faithful to the existing doctrine.

---

### Q42 — `implement-dsl` MID task collapses two phases — scope union vs split  *(MID)*

**✅ Decided: A** — Keep collapsed `implement-dsl` task with union scope `[dsl_core, driver_port, external_system_driver_port]`. Envelope loose; prompt body reads `tests:` parameter and narrows actual mutation per-call. Respects Q28's deliberate collapse; avoids layer-coding in task names (option B) and avoids Phase C YAML mechanism expansion (option C).

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

**✅ Decided: C** — Widest: `at_test, ct_test, dsl_port, dsl_core, driver_port, driver_adapter, external_system_driver_port, external_system_driver_adapter` (all 8 test-side keys). `refactor-tests` owns the entire test stack, since MID has only two refactor tasks (`refactor-tests` + `refactor-system`); driver-adapter refactoring has no other home. Pairs cleanly with Q44: `refactor-tests` = test stack (8 layers), `fix-*` = test stack + `system_path` (9 layers). **Doctrine pin: this union is intentional. Do not narrow.** A future reviewer might see "8-layer envelope on a refactor task" and propose option A or B; the union owns the whole test stack by design.

**Context.** MID task `refactor-tests` (used by HIGH `refactor-and-verify-tests`, called from CYCLE `refactor-test-structure`) is net-new — `phase-scopes.yaml` has no equivalent phase. Its Scopes were just set to `at_test, ct_test` (symmetric with `disable-tests` / `enable-tests`). But test refactoring might legitimately touch DSL (extracting test helpers into the DSL surface) or testkit files (driver-port helpers).

**Options.**
- **(A) Narrow: `at_test, ct_test`.** Pure test-file refactoring only. Helper extraction to DSL/testkit happens as a separate task. Current state.
- **(B) Wider: `at_test, ct_test, dsl_port, dsl_core`.** Allow DSL surface evolution as part of test refactoring (common in practice — pulling test-helper logic up).
- **(C) Widest: `at_test, ct_test, dsl_port, dsl_core, driver_port, driver_adapter, external_system_driver_port, external_system_driver_adapter`.** All test-side layers, including testkit. Permits any structural refactoring within the test stack.

**Recommendation: B.** Test refactoring routinely pulls helpers into DSL — narrowing to `at_test, ct_test` only forbids the common case. But driver/external testkit refactoring is structurally different (different sub-cycle), so (C) over-includes.

---

### Q44 — `fix-*` scope envelope as pinned doctrine  *(MID — already decided, pin for permanence)*

**✅ Decided: A** — Pin doctrine: `fix-*` scope = 9-layer union of all writable layers (`at_test, ct_test, dsl_port, dsl_core, driver_port, driver_adapter, external_system_driver_port, external_system_driver_adapter, system_path`). **Cannot narrow.** Document here + add comment in `phase-scopes.yaml` when fix phases land there. (B) ruled out by Q1 in parent plan (LOW `fix` is single-attempt, no recursion, no caller context). Pairs with Q43: `refactor-tests` = 8-layer test stack, `fix-*` = 9-layer (adds `system_path`).

**Forward reference.** See **Q47** (invocation-context scope inheritance) for a proposal that could re-open this decision — but only if Q1's single-attempt lock is also reconsidered. Q44 = A is stable in any scenario where Q1 is also stable, so pinning A now costs nothing if Q47 later changes the model.

**Context.** User confirmed during the review that `fix-unexpected-passing-tests` and `fix-unexpected-failing-tests` should declare the union of every writable layer ("EVERYTHING that is under tests, including system"). Scope is currently set to all 9 keys (`at_test`, `ct_test`, `dsl_port`, `dsl_core`, `driver_port`, `driver_adapter`, `external_system_driver_port`, `external_system_driver_adapter`, `system_path`).

This Q exists only to **pin the doctrine** so future reviewers don't propose narrowing. A naive review would see "9-layer scope = too broad, narrow it." The decision is deliberate: a fix may need to touch any layer the change cycle touched, and the cycle as a whole spans every layer.

**Options.**
- **(A) Pin: fix-* scope = union of all writable layers. Cannot narrow.** Document in this plan + as a comment in `phase-scopes.yaml` when the fix phases land there.
- **(B) Narrow per-context.** Fix scope is the scope of the cycle that invoked it. Requires fix to know its caller, which the current LOW `fix` primitive doesn't support (`fix` takes a `failure` payload, not caller-cycle context).
- **(C) No pinning needed — current state stands without doctrine annotation.** Risk: future reviewer narrows it without realizing the union was intentional.

**Recommendation: A.** User already chose the union scope. Pinning the doctrine prevents drift. (B) requires LOW primitive changes that Q1 in the parent plan ruled against ("single attempt, no recursion").

---

### Q45 — HIGH file Scopes audit needed?  *(HIGH)*

**✅ Decided: A + C** — No HIGH Scopes audit (orchestrations have no scope; scope lives only on MID tasks). But add a follow-up item to audit MID/HIGH/CYCLE/TOP naming consistency (e.g., singular `adapter` vs plural `adapters`, Inputs/Outputs/Steps drift) — tracked as a **separate plan**, not in this scope-doctrine plan. **Action item:** when this plan is committed, author a fresh follow-up plan (`plans/YYYYMMDD-HHMM-bpmn-name-consistency-audit.md`) per `feedback_new_plan_not_extend`.

**Context.** HIGH orchestrations (`write-and-verify-tests`, `implement-and-verify-dsl`, `implement-and-verify-system`, `refactor-and-verify-tests`, `implement-test-layer`, …) don't declare `**Scopes:**` blocks in `plans/ideas/3-bpmn-refactor-high-level.md` — they're orchestrations, not agent tasks. They call MID tasks (which carry the scope) and command tasks (which have no scope).

Question: does the HIGH file need any audit equivalent to the MID one, or does scope flow purely through MID-task invocations?

**Options.**
- **(A) No HIGH-file audit needed.** Orchestrations have no scope; scope lives only on agent (MID) tasks. The runtime computes the effective scope envelope from the invoked MID task's declaration, not from the orchestration that wrapped it.
- **(B) HIGH orchestrations declare an aggregated scope** equal to the union of called MID tasks' scopes. Redundant (computable from the call graph) but possibly useful for documentation / static analysis.
- **(C) Audit HIGH file for other contract drift** (Inputs/Outputs/Steps naming consistency vs MID), even though Scopes don't apply. E.g., naming consistency: HIGH has `implement-and-verify-external-system-driver-adapter-contract-tests` (singular `adapter`) while MID is `implement-external-system-driver-adapters` (plural). Worth pinning consistency now before Phase C.

**Recommendation: A + C.** No scope declarations on HIGH; but do a separate pass for naming-consistency / Inputs / Outputs drift between MID and HIGH (and CYCLE and TOP). Track that as a separate item in the next plan iteration, not in this scope-doctrine plan.

---

### Q46 — CYCLE and TOP files — equivalent Scopes audit?  *(CYCLE / TOP)*

**✅ Decided: B → no-op confirmed.** Ran `grep '**Scopes:**'` against both files during refinement (2026-05-25). **No matches.** CYCLE and TOP files contain no accidental Scopes blocks. No further action.

**Context.** Same shape as Q45 but for CYCLE (`plans/ideas/4-bpmn-refactor-cycle-level.md`) and TOP (`plans/ideas/5-bpmn-refactor-top-level.md`). These are orchestrations, like HIGH — no agent tasks at this level (per Q-new-4 template, MID is the lowest agent-task layer).

**Options.**
- **(A) No CYCLE/TOP Scopes audit needed.** Same reasoning as Q45-A.
- **(B) Audit CYCLE/TOP for any accidental Scopes blocks that were authored** despite the template saying agent-tasks-only. Cleanup pass.

**Recommendation: B.** Quick verification pass; if no Scopes blocks exist in CYCLE/TOP, this resolves to a no-op confirmation.

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

Q40–Q46 resolved during refinement walk (2026-05-25). Q47 added and deferred. Execution order for the resulting items (when authored in a follow-up `/refine-plan` pass or directly in `/execute-plan`):

1. **Q41** (drop `- ticket`) — 2 MID edits in `plans/ideas/2-bpmn-refactor-mid-level.md`. Smallest, isolated.
2. **Q42** (keep `implement-dsl` union) — no edit; current state stands. Doctrine pin only.
3. **Q43** (`refactor-tests` widest scope, pinned) — verify current MID Scopes block matches C; add doctrine pin comment.
4. **Q44** (`fix-*` 9-layer union, pinned) — verify current MID Scopes block matches A; add doctrine pin comment.
5. **Q45 C follow-up** — author a fresh plan (`plans/YYYYMMDD-HHMM-bpmn-name-consistency-audit.md`) covering MID/HIGH/CYCLE/TOP naming-consistency and Inputs/Outputs/Steps drift.
6. **Q40** (snake → kebab rename) — largest. Do last so MID/HIGH/CYCLE/TOP Scopes blocks are stable before the global rename. Surfaces: `paths_defaults.go::CanonicalPathKeys()`, `phase-scopes.yaml`, `config.go` Rule 22a, `path-keys.md`, test fixtures, prompt frontmatter scope: keys, brainstorm Scopes blocks, Family A `system_path` → `system-path`.

## Items

Item authoring deferred to a follow-up `/refine-plan` walk or direct `/execute-plan` session. Estimated item shapes:

- **Q41:** 2 edits to `plans/ideas/2-bpmn-refactor-mid-level.md` (drop `- ticket`, set `Scopes: NONE` on `refine-acceptance-criteria` and `update-ticket`).
- **Q42–Q44:** doctrine-pin verifications; possible inline comment additions to MID file and (later) `phase-scopes.yaml`.
- **Q45 C:** spawn separate plan; no edits in this surface.
- **Q40:** ~6 file-edit items + a fixture-test pass; bounded find/replace once the rename map is locked.

## Cross-references

- Parent design plan: `plans/20260525-1057-bpmn-refactor-design.md`
- Phase C execution plan: `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md`
- Memory `feedback_substitutable_paths_in_docs` — path scopes use Family B placeholders, never prose.
- Memory `feedback_no_layer_coding_in_names` — Q42 option B mildly violates this.
- Memory `feedback_naming_github_jira_first` — Q41 reasoning: tickets are GitHub/Jira, external-system territory.
- Memory `feedback_teaching_repo_no_legacy` — Q40 doesn't need a migrate-relocation pass; teachers regenerate.
- Memory `feedback_drop_dont_relocate` — Q41 reasoning: existing external-systems exemption already covers ticket-mutating agents.
- Memory `feedback_new_plan_not_extend` — Q45 follow-up: name-consistency audit goes in a fresh plan, not appended here.
- Memory `feedback_no_deferred_mechanism` — Q47 reasoning: scope is pinned per phase, not computed per invocation pattern.
