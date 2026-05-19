# 2026-05-19 09:29 UTC — Plan coordination meta-plan: BPMN + SSoT + vocabulary

**Coordination bounds requested by user:** first = `20260518-1144-atdd-bpmn-orchestration.md`, last = `20260519-0704-bpmn-external-system-naming-consistency.md`.

**Plans analysed:** 5 in-scope, 2 referenced-only (`20260519-0911-author-esir-write-phase-doc.md`, `20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md` — both out of scope but materially relevant to `20260518-2236`'s status; see Needs-decision §1). `20260518-1500-atdd-phase-scope-placeholders.md` is referenced as a hard dependency by 1530 and 1742 but the file no longer exists in `plans/` — items 1–3 are recorded as landed in commits `8322c38`/`d7ec876`, so it is treated as fully landed for the purposes of this analysis.

## Per-plan status snapshot

| Plan | Status | Items done / total | Touched files (primary) | Notes |
|---|---|---|---|---|
| `20260518-1144-atdd-bpmn-orchestration.md` | ✅ REFINED | 0 / 8 (10 originally; items 1 and 10 dropped; item 9B deferred) | `process-flow.yaml`, `runtime/gates/*`, `runtime/actions/*`, runtime prompt files (per-phase agent prompts) | Cross-cutting §Conventions `<channel>` edit landed in commit `82bb983`. Item 7 has a hard dep on the legacy-coverage-cycle plan (out of scope) for the legacy-marker convention. |
| `20260518-1530-atdd-phase-scope-ssot.md` | ✅ Partial execute | 3 / 11 (items 1, 2, 11 landed in `a171da4`; item 11 broadened post-landing) | `internal/atdd/phase-scopes.yaml` (new), `phase_scopes_test.go`, `scope.md`, `paths_defaults.go` (signature), `optivem_yaml.go`, `config.go`, `config_commands.go`, runtime prompts (frontmatter `scope:`), 9 phase docs under `change/behavior/`, BPMN plan file (Snapshot A removal), rename `placeholders.md` → `path-keys.md` | Item 8 *edits* plan 1144's file. CanonicalPathKeys exported during item 11 landing. |
| `20260518-1742-family-b-stems-and-ct-vocab.md` | ✅ REFINED | 0 / 4 (item 1 resolved at refine) | `internal/projectconfig/paths_defaults.go`, `internal/assets/global/docs/atdd/process/placeholders.md` | Bidirectional with SSoT: SSoT needs `ct_test` vocab; this plan needs SSoT item 3's `DefaultPaths(testLang, systemTestPath, sutNamespace)` signature. Plan itself flags "atomic single-session" as pragmatic alternative. |
| `20260518-2236-migrate-process-docs-hierarchy.md` | ⚠️ NOT YET REFINED | 0 / 11 | `docs/atdd/process/**` (whole tree), `internal/assets/global/docs/atdd/process/**`, `process-flow.yaml` (11 `phase_doc:` lines), runtime prompt files (`Read` lines), `clauderun_test.go`, `dispatch_spy_test.go`, `driver_test.go`, `embedded_smoke_test.go`, `sync_test.go`, `paths_defaults.go:7` (comment), `clauderun.go:54` (comment), `.claude/agents/atdd/meta/*` | Open Q1 gates execution on SSoT reconciliation. **Superseded by out-of-scope plan `20260519-0922` per 0922's own annotation** — see Needs-decision §1. |
| `20260519-0704-bpmn-external-system-naming-consistency.md` | Refined (Locked decisions section, no explicit ✅ marker) | 0 / 4 | `process-flow.yaml` (phase id renames), `paths_defaults.go` (Family B key renames), `config_commands.go` (rename pass), `config.go` (validator), `paths_defaults_test.go`, `config_commands_test.go`, runtime prompt files (renames), 4 phase-doc files (renames under `change/behavior/ct/` + `change/structure/`), `placeholders.md`/`path-keys.md`, `architecture.yaml`, `phase-scopes.yaml`, `phase_scopes_test.go` | Hard dep: SSoT items 1, 2, 11 (landed). Should follow CT-vocab (1742). Plan explicitly notes: "Whichever lands first, the other must be re-walked for conflicts." |

### Referenced-only plans (read for graph completeness, not in coordination scope)

| Plan | Why referenced |
|---|---|
| `20260518-1500-atdd-phase-scope-placeholders.md` | Hard predecessor for 1530 and 1742; file no longer exists in `plans/`; items 1–3 landed in commits `8322c38`/`d7ec876`. Treat as fully landed. |
| `20260519-0911-author-esir-write-phase-doc.md` | Out of scope; relevant because it indirectly bears on 2236's supersession (see Needs-decision §1). |
| `20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md` | Out of scope; **explicitly supersedes 2236** in two ways per its own annotation. See Needs-decision §1. |

## Dependency graph (post-consolidation, unit-level — see Execution units below)

```
[1500 LANDED] ──► 1742 ◄══ atomic ══► 1530 ──► 0704
                            │
                            ├──► 1144 (after 1530 lands)
                            │
                            └──► 2236 (gated on user decision — see Needs-decision §1)
```

Raw plan-level edges (pre-consolidation):

```
1500 (LANDED) ──► 1742 ──► 1530 (signature dep, both directions — cycle)
1500 (LANDED) ──► 1530 (predecessor items 1–3)
1530 ──► 1144 (1530 item 8 rewires 1144's check_phase_scope; 1530 item 8 also edits 1144's plan file)
1530 ──► 0704 (0704 hard dep: SSoT items 1, 2, 11)
1742 ──► 0704 (0704: "CT-vocabulary should have landed so ct_test and DefaultPaths signature are stable")
1530 ──► 2236 (2236 Open Q1 gates on SSoT plan reconciliation)
1144 ──► 2236 (both touch process-flow.yaml)
```

## Conflicts

### 1. `internal/projectconfig/paths_defaults.go` — hard conflict (1530 + 1742 + 0704)

- **1530 item 3** changes `DefaultPaths` signature to `DefaultPaths(testLang, systemTestPath, sutNamespace string)` and rewrites the per-key path emission to bake `sutNamespace` in.
- **1742 items 2a, 2b, 3a, 3b** rewrite `canonicalPathKeys()` (appends `ct_test`), rewrites `pathStems()` (corrects `at_test` stems against shop-`latest` and adds `ct_test`), and rewrites the `DefaultPaths` docstring.
- **0704 item 2** renames `external_driver_port` → `external_system_driver_port` and `external_driver_adapter` → `external_system_driver_adapter` in `canonicalPathKeys()`, `pathStems()`, and the `DefaultPaths` docstring.
- **Why hard:** all three plans rewrite the same three call surfaces (`canonicalPathKeys()`, `pathStems()`, `DefaultPaths` + docstring) in the same atomic file. Any execute order forces the later two plans to mechanically rebase against the prior plan's text.
- **Recommended resolution:** atomic single-session for `1530 + 1742` (see Consolidation §1); then serial commit for `0704` (Consolidation §2) — `0704` is mechanical key-rename and validates cleanly as its own commit. The cascading test-file edits (`paths_defaults_test.go`) follow the same ordering.

### 2. `internal/projectconfig/config_commands.go` (`runConfigMigrate`) — hard conflict (1530 + 0704)

- **1530 item 6** extends `runConfigMigrate` with a **SSoT join pass** (joins `sut_namespace` into each `paths:<key>` value + into `system.path`; deletes `system.sut_namespace`).
- **0704 item 3** extends `runConfigMigrate` with a **rename pass** (`external_driver_*` → `external_system_driver_*`).
- **Why hard:** both add a new back-fill pass inside the same function. Order matters mechanically (the SSoT join must run before the rename, or vice versa — the rename is on already-joined keys so the SSoT join should run first).
- **Recommended resolution:** serial — `1530` item 6 lands first (as part of the 1530+1742 atomic unit), `0704` item 3 adds its rename pass on top. The `Long`/`Short` docstring at `config_commands.go:266–282` accumulates one bullet per plan.

### 3. `internal/projectconfig/config.go` — hard conflict (1530 + 0704)

- **1530 item 5** extends the validator with: pre-SSoT detection (hard error pointing at `gh optivem config migrate`), `${...}` markers in `paths:` values (hard error), unknown fields at every nesting level (`yaml.KnownFields(true)`), and `paths.<name>` not in `canonicalPathKeys()` (hard error).
- **1530 item 3** drops the `SutNamespace string` field from the `System` struct (line 252).
- **0704 item 3 validator update** detects old key name (`paths.external_driver_*`) and redirects to `config migrate` with the same pattern as 1530's pre-SSoT detection.
- **Why hard:** both plans add validator branches that must coexist; 0704's pattern is parallel to 1530's, but they're additions to the same validator surface.
- **Recommended resolution:** same as Conflict §2 — 1530 lands first, 0704 layers its key-rename validator branch on top.

### 4. `internal/atdd/runtime/statemachine/process-flow.yaml` — hard conflict (1144 + 2236 + 0704)

- **1144 items 2, 4, 5, 8, 9** add new nodes (`GATE_DSL_FLAGS_PRESENT`, `STOP_FLAG_UNSET`, `STOP_SCOPE_VIOLATION`, `STOP_LEGACY_FAILED`, scope-exception gateways), threads `allowed_paths` params into ~8 `call_activity` invocations.
- **2236 item 4** rewrites the 11 `phase_doc:` lines (lines 324, 340, 365, 419, 431, 483, 500, 519, 538, 1099 + later) to new hierarchical paths.
- **0704 item 1** renames phase ids (`CT_RED_EXTERNAL_DRIVER` → `CT_RED_EXTERNAL_SYSTEM_DRIVER`; `CT_GREEN_STUBS` → `CT_GREEN_EXTERNAL_SYSTEM_STUB`) — affects node ids, sequence_flows, agent names, change_type, phase_label.
- **Note re. 1530 item 8:** that item *removes* the per-node `allowed_paths` params from this file too — so 1144 item 8/9 (which threads them in) may be obsoleted by 1530 item 8 if 1530 item 8 lands first. This is captured under Needs-decision §2.
- **Why hard:** three plans rewrite the same file in three independent dimensions (structure additions, doc-path rewrites, name renames). Even though each touches different lines, the merge surface is non-trivial and any concurrent edit risks merge conflicts on adjacent lines.
- **Recommended resolution:** strict serial — 1530 (item 8 in the 1530+1742 atomic unit) first, then 0704 (renames are mechanical and easy to validate), then 1144 (largest structural addition; its `allowed_paths`-threading items 8/9 must be re-scoped against 1530 item 8 first — see Needs-decision §2). 2236 deferred per Needs-decision §1.

### 5. `internal/assets/runtime/prompts/atdd/*.md` — hard conflict (1144 + 1530 + 0704 + 2236)

- **1144 item 6** adds a `scope_exception`-signal section to per-phase agent prompt content.
- **1530 items 4, 10** add `scope: {}` frontmatter placeholder (item 10) and project resolved-path values into it on sync (item 4).
- **0704 item 1** renames two prompt files (`ct-red-external-driver.md` → `ct-red-external-system-driver.md`; `ct-green-stubs.md` → `ct-green-external-system-stub.md`) and updates their content references.
- **2236 item 5** rewrites the `Read ${docs_root}/atdd/process/<old>.md` lines in each prompt file to the new hierarchical paths.
- **Why hard:** the same prompt files are touched for four independent reasons (content addition, frontmatter addition, filename rename, body link rewrite). Filename renames in particular are not commutative with the other edits.
- **Recommended resolution:** strict serial in the order — 1530 (item 10 frontmatter placeholder lands with the 1530+1742 atomic unit) → 0704 (renames first so subsequent edits target the new filenames) → 1144 (content addition) → 2236 (deferred per Needs-decision §1).

### 6. `internal/atdd/phase-scopes.yaml` + `internal/atdd/phase_scopes_test.go` — coordination conflict (1530 ↔ 0704)

- **1530 item 1** (landed) created `phase-scopes.yaml` keyed by current BPMN phase ids (including `CT_RED_EXTERNAL_DRIVER`, `CT_GREEN_STUBS`).
- **1530 item 11** (landed) added `phase_scopes_test.go` with an allowlist including those phase ids.
- **0704 item 1** renames those phase ids → `CT_RED_EXTERNAL_SYSTEM_DRIVER`, `CT_GREEN_EXTERNAL_SYSTEM_STUB` — must update both files in lockstep.
- **Why coordination, not hard:** the edits are mechanical key renames in already-landed files; whichever side runs first dictates a small mechanical update on the other.
- **Recommended resolution:** 0704 owns both file updates as part of its item 1.

### 7. `internal/assets/global/docs/atdd/process/placeholders.md` (or `path-keys.md`) — hard conflict (1530 + 1742 + 0704)

- **1742 item 4** edits the Family B example block (fixes `at_test` value; adds `ct_test`).
- **1530 item 9** renames `placeholders.md` → `path-keys.md` and materially rewrites the contents.
- **0704 item 2 cascade** updates key-vocabulary references for the rename.
- **Why hard:** rename + rewrite + content edits on the same file in three plans.
- **Recommended resolution:** the 1530+1742 atomic unit handles 1530 item 9's rename + rewrite + 1742 item 4's content fix in one session. 0704 item 2 cascade then lands against the renamed file (`path-keys.md`).

### 8. Phase docs under `docs/atdd/process/change/behavior/**` + `change/structure/**` — hard conflict (1530 + 0704 + 2236)

- **1530 item 7** rewrites the `## Scope` section of 9 phase docs (5 AT + 4 CT) — heading retained, contents replaced.
- **0704 item 1** renames `change/behavior/ct/1.3-ct-red-external-driver.md` → `1.3-ct-red-external-system-driver.md` and `change/behavior/ct/2-ct-green-stubs.md` → `2-ct-green-external-system-stub.md`, plus content references.
- **2236 item 3** *cut-pastes the entire tree* from `docs/atdd/process/` into `internal/assets/global/docs/atdd/process/` (per Open Q1; tree shape itself disputed against actual new files).
- **Why hard:** structural relocation (2236), file renames (0704), and content edits (1530) on the same files. 2236 in particular reshapes the parent directory.
- **Recommended resolution:** 1530 item 7 edits first (within the 1530+1742 atomic unit's scope), then 0704 file renames; 2236 deferred per Needs-decision §1.

### 9. Plan-file cross-edits — coordination conflict (informational only — read-only on plans per coordinator rules)

- **1530 item 8** **edits** plan `20260518-1144`'s file: deletes Snapshot A, rewrites item 5 prose, deletes the §Node params tables (lines 218–236).
- **1742's deferred follow-ups** describe edits to plan `20260518-1500` (which is landed; no plan file to edit anyway) and plan `20260518-1530`'s file (example values + filename refs).
- **2236 item 10** sweeps plans (`20260518-1530`, `20260518-1742`, etc.) for OLD-path references.
- **Why surfaced:** this coordinator is read-only on plan files; the cross-edits are part of plan-execution items and not in coordinator scope. They are flagged so the executor knows that the plan-file state itself will be in flux during execution; subsequent agents reading the modified plan files should re-extract their dependency view.
- **Recommended resolution:** no action by coordinator. Executors of each item that includes plan-file edits should commit those edits as part of the same atomic unit.

## Consolidation findings (decided)

### 1. `20260518-1530` + `20260518-1742` — atomic single-session (decided)

- **Why entangled:** bidirectional dependency (cycle). 1530 needs `ct_test` in `canonicalPathKeys()` for its `phase-scopes.yaml` (item 1) — landed against the pre-`ct_test` slice with `ct_test` referenced as a forward dep. 1742 needs SSoT item 3's `DefaultPaths(testLang, systemTestPath, sutNamespace)` signature. Both plans touch `paths_defaults.go` and `placeholders.md`/`path-keys.md` with mechanically intertwined edits. Both plan bodies explicitly suggest "atomic single-session by one executor" as the pragmatic alternative to the inter-plan staging dance.
- **Resolution (recommended): atomic single-session execution.** One executor lands the in-flight items of both plans in a single agent session, producing one or a tightly-grouped sequence of commits. **Why:** both plans were filed under `[[feedback_new_plan_not_extend]]`; merging now would violate that rule. Atomic session resolves the entanglement in working memory without rewriting either plan. The plan bodies endorse this resolution.
- **Alternatives considered:**
  - **Merge into a fresh plan** — rejected because both plans are refined and `/execute-plan`-ready; merging would re-open the refinement walk for no operational gain.
  - **Strict serial in the order 1530-items-1-2-11-3 → 1742-all → 1530-items-4-10** (per 1530's own Hand-off Execute order step 3 and 1742's "co-ordinated pair" pre-requisite) — workable but every step forces a fresh agent context, a re-read of the prior plan's landing state, and more granular commits. Reserve as fallback if a single executor's context budget is exhausted mid-session.

### 2. `20260518-1530` + `20260518-1742` (atomic unit) → `20260519-0704` — serial-after, not consolidated (decided)

- **Why entangled:** 0704 touches `paths_defaults.go`, `config_commands.go`, `config.go`, `phase-scopes.yaml`, `phase_scopes_test.go`, `placeholders.md`/`path-keys.md`, and `process-flow.yaml` — all in the post-SSoT-and-CT-vocab surface. 0704's plan body explicitly states: "Whichever lands first, the other must be re-walked for conflicts." 0704's hard pre-requisite is SSoT items 1, 2, 11 (landed); its softer dep is CT-vocab (1742) so the `ct_test` and `DefaultPaths` signature are stable when its rename pass runs.
- **Resolution (recommended): serial — 0704 lands after the 1530+1742 atomic unit, as a separate session and commit.** **Why:** 0704 is a mechanical key-rename + phase-id-rename pass with its own focused test suite. Keeping it as a separate session preserves reviewability of the rename diff (which would otherwise be lost in the larger SSoT-and-vocab edit), and 0704's tests (idempotent migrate, comment preservation, both-keys-present hard-error) validate cleanly in isolation.
- **Alternative considered:** **3-plan atomic session (1530 + 1742 + 0704).** Rejected because the rename's review value is higher when isolated; the 1530+1742 atomic unit already has substantial context-budget pressure (multiple file rewrites + 9 phase docs + 9 prompts + `phase-scopes.yaml` design); folding 0704 in pushes a single session above what's reliably executable. The serial overhead is one extra `git pull` and a 2-minute re-read.

### 3. `20260518-1144` — runs strictly after the 1530+1742 unit (decided)

- **Why entangled:** 1144 item 5 (`check_phase_scope` action) is **rewired** by 1530 item 8: the action no longer reads `params:` for `allowed_paths`; instead it reads `internal/atdd/phase-scopes.yaml` directly by node id. 1144 items 8 and 9 (which thread `allowed_paths` params into `call_activity` invocations) are **obsoleted** by 1530 item 8's removal of those params — see Needs-decision §2. Additionally, 1530 item 8 *edits 1144's plan file* (deletes Snapshot A, rewrites item 5 prose, deletes the §Node params tables); 1144 should not be executed until those plan-file edits are in place so the executor reads the post-edit shape.
- **Resolution (recommended): strict serial — 1144 lands in a later wave, after the 1530+1742 atomic unit and after 0704.** **Why:** preserves the post-SSoT shape that 1530 item 8 establishes; defers the largest structural addition to `process-flow.yaml` until the renames and SSoT machinery are stable; gives the executor a clean post-1530-edits plan file to read.
- **Alternative considered:** **execute 1144 in parallel with 0704 (both run after the 1530+1742 unit).** Rejected because both touch `process-flow.yaml` substantially (0704 renames node ids; 1144 adds new nodes), making concurrent execution a merge-conflict generator on adjacent lines.

### 4. `20260518-2236` — deferred from this coordination set (decided, see Needs-decision §1 for residual user choice)

- **Why entangled:** plan is NOT YET REFINED; Open Q1 explicitly gates execution on SSoT plan 1530 reconciliation (which is also in scope here); its cut-paste item 3 has been overtaken by reality per the out-of-scope plan 0922's annotation.
- **Resolution (decided by coordinator, conservative): exclude 2236 from execution waves below.** **Why:** 2236 cannot be executed as written — Open Q1 is unresolved, plan is unrefined, and at least two of its items are partially obsoleted by a subsequent plan (0922). Including it in any wave would mislead the user. The user-facing question (defer permanently, or refine + reconcile vs 0922) is captured in Needs-decision §1.
- **Alternative considered:** **leave 2236 in Wave 2 conditional on the user resolving Open Q1.** Rejected because that turns the meta-plan into "depending on what you decide, run X or Y" — which is exactly the open-question structure the agent rules say to resolve up front.

## Execution units (post-consolidation)

The wave plan below operates on these units, not on raw plan files.

| Unit | Plans | Type | Touched files (primary) |
|---|---|---|---|
| U1 | `20260518-1530` (items 3–10) + `20260518-1742` (items 2a–4) | atomic-single-session | `paths_defaults.go`, `optivem_yaml.go`, `config.go`, `config_commands.go`, `phase-scopes.yaml` (extends), `process-flow.yaml` (item 8 cleanup only), 9 phase docs in `change/behavior/`, 9 runtime prompts (frontmatter), `placeholders.md` → `path-keys.md` (rename + rewrite + Family B example) |
| U2 | `20260519-0704` (items 1–3, item 4 per refinement) | standalone, serial-after-U1 | `paths_defaults.go` (key rename), `config_commands.go` (rename pass), `config.go` (validator), `process-flow.yaml` (phase id renames), 2 runtime prompt renames, 4 phase-doc renames, `architecture.yaml`, `phase-scopes.yaml` + `phase_scopes_test.go` (key updates) |
| U3 | `20260518-1144` (items 2–9) | standalone, serial-after-U2 | `process-flow.yaml` (new nodes, gateways, sequence_flows), `runtime/gates/*` (new bindings), `runtime/actions/*` (new actions), runtime prompts (scope_exception content addition) |
| U4-DEFERRED | `20260518-2236` | needs-decision (see §1) | not scheduled |

## Needs-decision (genuine ambiguity only)

### 1. Is `20260518-2236` in scope at all, given that `20260519-0922` supersedes it?

- **Background:** the out-of-scope plan `20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md` (refined-in-progress per its pickup marker) states verbatim: *"Superseded in two ways by this plan: (a) the actual new files have no numeric prefixes; (b) the move has already happened and the old files are in `_ARCHIVED_PENDING_DELETE/` rather than deleted, so the 'delete old files' step is already done up to a final rm. This plan replaces the reference-rewrite portion (its Items 4–7) with the correct destination filenames."*
- **Implications:** 2236's File-mapping table (numbered-prefix filenames like `1.1-at-red-test.md`) does not match the actual filesystem (filenames have no numbered prefixes). 2236's item 3 cut-paste is partially already done in a different shape. 2236's items 4–7 (reference rewrites) are being re-owned by 0922 with corrected destinations. 2236's Open Q1 (SSoT reconciliation) is also still open.
- **Question for user:** pick one —
  - **(a) Defer 2236 permanently** in favour of 0922 (which is unrefined but at least addresses post-archive reality). **Conservative recommended default** — 2236 cannot be executed as written.
  - **(b) Refine 2236 first against current filesystem reality**, then re-coordinate. This duplicates 0922's authoring; not recommended unless there is intent to retire 0922.
  - **(c) Treat 2236 as the canonical plan and edit 0922 instead.** Inverse of (a); only sensible if 0922's pickup is being abandoned.
- **Why this is in Needs-decision (not Consolidation):** the choice changes whether 2236 appears in any future wave at all, not just where it sits in the order. The coordinator cannot pick (a)/(b)/(c) because the decision crosses out of the in-scope plan set into 0922's intent (out-of-scope).

### 2. Are 1144's items 8 and 9 (allowed_paths param threading) obsoleted by 1530 item 8?

- **Background:** 1530 item 8 (c) explicitly says: *"Per-node `allowed_paths` params are no longer needed; `check_phase_scope` reads its phase's scope from `internal/atdd/phase-scopes.yaml` directly by node id."* This deletes the param-threading mechanism in `process-flow.yaml` (lines 218–236 per 1530 item 8 (c)). But 1144 items 8 and 9 *add* those exact `allowed_paths` params to every `call_activity` invocation.
- **Implication for U3 (1144):** if U1 (1530 item 8) lands first, U3's items 8 and 9 become no-ops (the params are obsolete before they're added). U3's item 5 (`check_phase_scope` implementation) is already rewritten by 1530 item 8's plan-file edits to read `phase-scopes.yaml` instead — so the executor of U3 will read a 1144 plan file whose item 5 already reflects the post-SSoT shape, and whose items 8 and 9 *should also* be marked obsolete.
- **Conservative interpretation:** when U1 lands, the U1 executor should additionally update 1144's plan file (items 8 and 9) to mark them obsolete and pin the reason — but this is a plan-file edit outside U1's stated scope. The alternative is for U3's executor to read 1144 items 8 and 9 as written, notice they are obsolete against the post-U1 state, and skip them.
- **Question for user:** pick one —
  - **(a) U1's executor extends 1530 item 8's plan-file edits to also strike 1144 items 8 and 9** (already striking item 5 prose + Snapshot A + §Node params tables in 1144). Cleanest plan-file state; small scope creep beyond the literal item 8 description. **Recommended.**
  - **(b) Leave 1144 items 8 and 9 standing**; U3's executor decides at execute time. Risks U3 executing the obsolete items mechanically against a post-SSoT `process-flow.yaml`.
- **Why this is in Needs-decision (not Consolidation):** answering (a)/(b) changes U1's executor's surface area (which files they edit), and that needs a user nod before execute.

## Execution waves

### Wave 1 — can start now

**Batch A (one agent session, atomic):**

- **Unit U1** — `20260518-1530` items 3–10 + `20260518-1742` items 2a–4. Single executor, single session.
  - Files owned: `internal/projectconfig/paths_defaults.go`, `internal/projectconfig/optivem_yaml.go`, `internal/projectconfig/config.go`, `internal/projectconfig/config_commands.go`, `internal/atdd/phase-scopes.yaml` (extends already-landed file), 9 phase docs under `docs/atdd/process/change/behavior/` (and `ct/`), 9 runtime prompts under `internal/assets/runtime/prompts/atdd/` (frontmatter `scope:` placeholder), `internal/assets/global/docs/atdd/process/placeholders.md` (rename to `path-keys.md` + rewrite + Family B example fix), `internal/atdd/runtime/statemachine/process-flow.yaml` (item 8 cleanup only — Snapshot A removal + §Node params tables removal in the BPMN *plan file*; runtime YAML itself sees no node additions in this unit).
  - Estimated session count: 1.
  - Pre-execute commands (per 1530 Hand-off + `[[feedback_check_concurrent_agents]]`):
    - `git -C C:/GitHub/optivem/academy/gh-optivem status` — confirm clean working tree on the files above.
    - `Grep` `plans/*.md` and `plans/deferred/*.md` for active pickup markers on `paths_defaults.go`, `config_commands.go`, `config.go`, `optivem_yaml.go`, `placeholders.md`/`path-keys.md`, runtime prompts, and `internal/atdd/phase-scopes.yaml`. Coordinate before adding U1's marker.
    - Verify `Needs-decision §2` answered (so the U1 executor knows whether to strike 1144 items 8 and 9 in the plan file).
  - Post-execute: rerun `gh optivem sync` in any active scaffolded repo to refresh the runtime-prompt `scope:` frontmatter.

(Wave 1 has no parallel-safe second batch — 0704 and 1144 both touch files U1 rewrites; they must serialise into Wave 2 and Wave 3.)

### Wave 2 — after Wave 1 lands

**Batch A (one agent session):**

- **Unit U2** — `20260519-0704` items 1–3, plus item 4 decision per refinement.
  - Files owned: `internal/projectconfig/paths_defaults.go` (Family B key rename), `internal/projectconfig/paths_defaults_test.go`, `internal/projectconfig/config_commands.go` (rename pass added to `runConfigMigrate`), `internal/projectconfig/config_commands_test.go`, `internal/projectconfig/config.go` (validator hint), `internal/atdd/runtime/statemachine/process-flow.yaml` (phase id renames + agent name + phase_label + change_type), 2 runtime prompt file renames (`ct-red-external-driver.md` → `ct-red-external-system-driver.md`; `ct-green-stubs.md` → `ct-green-external-system-stub.md`), 4 phase-doc file renames (under `change/behavior/ct/` + `change/structure/`), `internal/atdd/runtime/architecture/architecture.yaml`, `internal/atdd/phase-scopes.yaml` (CT_* key updates), `internal/atdd/phase_scopes_test.go` (allowlist updates), `internal/assets/global/docs/atdd/process/path-keys.md` (key-vocabulary cascade).
  - Estimated session count: 1.
  - Pre-execute commands:
    - `git status` clean on the above files.
    - `Grep` `plans/*.md` for active pickup markers on `paths_defaults.go`, `process-flow.yaml`, `config_commands.go`, CT prompt files.
    - Confirm U1 landed (the `DefaultPaths(testLang, systemTestPath, sutNamespace)` signature is live; `path-keys.md` exists).
  - Post-execute: `gh optivem config migrate` against the 12 shop `gh-optivem-*.yaml` files (per 0704 item 4 option (a) if chosen at refinement) to back-fill the key rename; then `gh optivem config validate` on each to confirm no validation errors.

### Wave 3 — after Wave 2 lands

**Batch A (one agent session):**

- **Unit U3** — `20260518-1144` items 2, 3, 4, 5, 6, 7. (Items 8 and 9 conditionally skipped per Needs-decision §2 answer.)
  - Files owned: `internal/atdd/runtime/statemachine/process-flow.yaml` (new gateway/STOP nodes; new sub-process bindings in `red_phase_cycle` + `green_phase_cycle`; new sequence_flows for `GATE_DSL_FLAGS_PRESENT`, `STOP_FLAG_UNSET`, `STOP_SCOPE_VIOLATION`, `STOP_LEGACY_FAILED`), `internal/atdd/runtime/gates/*` (new `dsl_flags_present`, `phase_scope_clean`, `scope_exception_requested`, `failing_legacy_present` bindings), `internal/atdd/runtime/actions/*` (new `check_phase_scope`, `detect_failing_legacy` actions; updated `disable_change_driven` + `enable_change_driven` actions for §Conventions disable-reason format), `internal/assets/runtime/prompts/atdd/*.md` (scope_exception instruction sections added to per-phase agent prompts; renamed CT files from Wave 2 still get the addition).
  - Estimated session count: 1 (item 7 has a residual hard-dep on the legacy-coverage plan's marker convention — that plan is **out of scope** for this coordination; if the marker convention has not landed by Wave 3 start, item 7 is the only item that blocks — items 2–6 and 8/9 are independent). Refer to legacy-coverage-cycle pickup marker.
  - Pre-execute commands:
    - `git status` clean.
    - `Grep` `plans/*.md` for active pickup markers on `process-flow.yaml`, gates, actions, prompts.
    - Confirm Waves 1 and 2 landed (phase ids are post-rename; `check_phase_scope` reads `phase-scopes.yaml` by node id; prompt frontmatter has `scope:` populated).
    - Verify legacy-coverage-cycle marker convention status (item 7 only).

### Wave 4 — conditional, after Needs-decision §1 resolved

- **Unit U4 (deferred)** — `20260518-2236`. Currently not scheduled. If the user resolves Needs-decision §1 with option (b) (refine 2236 against current filesystem reality), 2236 enters a future wave that depends on Waves 1 + 2 having landed (because the SSoT and rename pieces are 2236's Open Q1 reconciliation surface). If the user picks option (a), 2236 is dropped from this coordination set entirely.

## Pre-execute checks (apply before any wave starts)

- `Grep` `plans/*.md` and `plans/deferred/*.md` for active pickup markers on the files touched by the upcoming wave (use the file lists in each unit above).
- `git -C C:/GitHub/optivem/academy/gh-optivem status` — confirm no uncommitted changes on those files.
- Confirm the open question in the `Needs-decision` section above has been answered (Needs-decision §1 before Wave 4 is contemplated; Needs-decision §2 before Wave 1 starts so the U1 executor's scope is clear).
- Confirm no concurrent agent has acquired a pickup marker on the same plan file between this meta-plan being written and execution starting — concurrent-agent collisions absorb work silently per `[[feedback_concurrent_agent_collision]]`.

## Out of scope of this meta-plan

- Plan content correctness — not audited; use `process-audit` or each plan's own refine cycle.
- Architecture/code alignment — not audited; use `architecture-sync`.
- Actual execution — use `/execute-plan` against each unit's lead plan (with the consolidation/serialisation context above) when each wave starts.
- Plans outside the in-scope coordination slice (the seven plans in `plans/` not in the resolved scope) — read only when cited as a hard dependency. `20260519-0911` and `20260519-0922` were read because they materially bear on `20260518-2236`'s supersession status (Needs-decision §1); other out-of-scope plans were not read.
