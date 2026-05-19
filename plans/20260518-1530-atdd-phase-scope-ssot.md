# Plan: ATDD phase-scope single source of truth (SSoT)

> ✅ **Partial execute 2026-05-18 / 2026-05-19** — items 1, 2 landed in commit `a171da4` (parallel agent: `phase-scopes.yaml`, `scope.md`, `CanonicalPathKeys` export). Item 11 landed in commit `feab2b1` (`internal/atdd/phase_scopes_test.go` + CT_RED_TEST defer in `phase-scopes.yaml`). Items 3 + 10 landed this session: `DefaultPaths(testLang, systemTestPath, sutNamespace)` signature change (`paths_defaults.go`), scaffolder rewires (`optivem_yaml.go:BuildOptivemYAML` + `buildSystem`) producing fully-resolved `System.Path` and `paths:` testkit values, `SutNamespace` field dropped from `projectconfig.System` (accessor still derives from `System.Repo`), and `scope: {}` placeholder added to 7 runtime prompts (allowlist-shape for the 2 multitier GREEN prompts). Items 4–9 remain. Note: `docs/atdd/process/shared/scope.md` was relocated to `internal/assets/global/docs/atdd/process/shared/scope.md` by commit `2ae72bd` (docs-into-assets reorganization). Also filed: [BPMN external-system naming consistency plan (20260519-0704)](20260519-0704-bpmn-external-system-naming-consistency.md) capturing CT_RED_EXTERNAL_DRIVER / CT_GREEN_STUBS phase-id renames + Family B `external_driver_*` key renames + the matching `config migrate` rename pass.

> ✅ **Refined 2026-05-18** — walked item-by-item with the user; every original OPEN QUESTION resolved (η, μ, λ, ζ, γ extension, ct_test, agent-to-phase mapping, placeholders.md scope). Locked decision ε revised in lockstep (manual-only → deterministic via existing `gh optivem config migrate`). Ready for `/execute-plan` once the listed hard dependencies land.

**Date:** 2026-05-18

**Predecessor:** [Phase-scope placeholders substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — adds the Family B keys (`at_test`, `dsl_port`, `dsl_core`) this plan consumes. This plan MUST land after the predecessor's items 1–3 are merged.

**Context.** The predecessor plan adds Family B *substrate* keys but stops short of consolidating where per-phase scope ASSIGNMENT lives. Today that assignment is duplicated across three surfaces: phase doc §Scope sections, BPMN Snapshot A's per-phase table, and the `check_phase_scope` runtime in BPMN. This plan introduces a single doctrinal source of truth and rewires all three consumers to read from it. It also resolves a sister duplication: today's `gh-optivem.yaml paths:` values are *base paths* that get composed with the Family A `${sut_namespace}` placeholder at use; this plan makes them *fully resolved* (sut_namespace baked in) and retires runtime substitution.

**Sibling plans referenced:**
- [Predecessor — AT-side Phase-scope placeholders substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — adds the AT-side Family B keys (`at_test`, `dsl_port`, `dsl_core`) this plan's `phase-scopes.yaml` references.
- [CT-side Family B vocabulary (20260518-1742)](20260518-1742-family-b-stems-and-ct-vocab.md) — adds the CT-side Family B keys (`ct_test`, and any siblings) this plan's `phase-scopes.yaml` references. Filed during this plan's refinement (2026-05-18) per `[[feedback_new_plan_not_extend]]`.
- [BPMN orchestration plan (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md) — consumer. Snapshot A retires (item 8 of this plan); `check_phase_scope` (BPMN item 5) gets rewired (item 8 of this plan). Header sibling-plans entry added (the predecessor's item 7a covers this).
- [BPMN plan §Conventions snapshot (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md#%C2%A7conventions-snapshot-inlined-to-survive-the-upcoming-split) — the canonical Snapshot A this plan retires.
- [Deferred — Multitier GREEN scope (plans/deferred/)](deferred/20260518-1530-multitier-green-scope.md) — picks up `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` scope (left on item 11's allowlist by this plan).
- [Deferred — `smoke_test` Family B key (plans/deferred/)](deferred/20260518-1530-smoke-test-family-b-key.md) — adds the `smoke_test` canonical key surfaced during this plan's refinement.
- [Deferred — Structure-cycle SSoT alignment (plans/deferred/)](deferred/20260518-1530-structure-cycle-ssot-alignment.md) — picks up `${...}/${sut_namespace}/` rewrites in `docs/atdd/process/change/structure/*.md` once structure-cycle BPMN shape settles and the user's mid-authoring is committed.

## Locked architecture decisions

Recorded here so the items below are unambiguous. All decided during the refinement of the predecessor plan, 2026-05-18.

- **β1 — Doctrinal key names.** Family B layer names (`driver_port`, `dsl_port`, etc.) are canonical ATDD vocabulary. Same in every gh-optivem project. Users own VALUES (physical paths), not NAMES.
- **Architecture/phase separation.** Layer names live with the architecture (defined by the canonical key set in `internal/projectconfig/paths_defaults.go canonicalPathKeys()`). Phases consume a SUBSET of layers per phase. Two independent doctrinal structures, joined at runtime.
- **δ — Fully resolved paths.** `gh-optivem.yaml paths:` values are concrete fully-resolved paths (e.g. `system-test/typescript/src/testkit/driver/port/shop`). No `${name}` substitution at runtime. `system.sut_namespace` is dropped from config — it's a scaffold-time-only input used to generate the resolved values, then not persisted. `${sut_namespace}` Family A placeholder retires.
- **γ — Family A path-shaped keys participate in scope identically to Family B.** Because `system.path` is now fully resolved, GREEN's scope can reference `system_path` like any other layer. No "Family A vs Family B" special-casing in scope.
- **ε — Migration is deterministic, via the existing `gh optivem config migrate` command** *(revised during this plan's refinement, 2026-05-18)*. The predecessor's original ε said "human-only, no auto-write" — over-conservative once we noticed there's already an idempotent, comment-preserving `config migrate` surface (`config_commands.go:247`) that does pure-deterministic back-fills for `project.provider` and `repos:`. SSoT joins via the same `DefaultPaths(testLang, systemTestPath, sutNamespace)` pinned in item 3 are deterministic too. So: pre-SSoT configs fail validation with a hard error pointing users at `gh optivem config migrate`, which performs the join + drops `system.sut_namespace` in one pass. The original "no AI in the loop" concern stands — `config migrate` is text-deterministic, not LLM-mediated.
- **All-error validation.** Unknown / obsolete fields are hard errors, not warnings. Consistent with existing `gh optivem config validate` strictness.

## Target architecture

**Tool source (gh-optivem):**
- `internal/projectconfig/paths_defaults.go` — canonical layer name list (extended by predecessor's item 1) + per-language default *base* path stems used at scaffold time.
- `internal/atdd/phase-scopes.yaml` *(new)* — phase → list of layer names. The doctrinal source of truth for per-phase scope.

**Scaffolded project:**
- `gh-optivem.yaml paths:` — layer name → fully-resolved physical path. **The only place users edit paths.**
- `internal/assets/runtime/prompts/atdd/*.md` frontmatter `scope:` *(new field)* — projected `{layer_name: resolved_path}` map (in the user's scaffolded project, post-sync — the asset templates ship with `scope: {}`). Marked "do not hand-edit — run `gh optivem sync` to refresh". Regenerated from `gh-optivem.yaml paths:` ⋈ `phase-scopes.yaml` ⋈ `process-flow.yaml` (`user_task.agent:` for the phase→file mapping). **Purpose: documentation/IDE inspection, not runtime enforcement.**
- `docs/atdd/process/shared/scope.md` *(new)* — the **rule** ("only modify paths listed in your `scope:`; if you need more, alert the user"). Single place. Phase docs link here.
- `docs/atdd/process/**.md` — phase docs. Reference layer NAMES by bare name (e.g. "RED-TEST writes to `driver_port`, `dsl_core`"). No `${...}` syntax, no path values.

**Runtime:**
- `check_phase_scope` (item 8) is the load-bearing enforcement. It reads `internal/atdd/phase-scopes.yaml` (embedded) for the per-phase layer list + `gh-optivem.yaml paths:` (in the user's project) for resolution + `git diff --name-only` for actual changes. Matches resolved paths as **directory-aware** prefixes against the diff (item 3 λ resolution).
- The runtime-prompt `scope:` frontmatter is not separately injected into the model context (runtime prompts are passed to Claude as text). It exists for humans inspecting the prompt file and IDE tooling — it does not enforce. Enforcement lives in `check_phase_scope`.

## Items

### 4. Scaffolder/sync change — project `scope:` into runtime-prompt frontmatter

Edit the scaffolder/sync layer so that on scaffold AND on `gh optivem sync` it projects per-phase scope into the **runtime prompt files** under `internal/assets/runtime/prompts/atdd/*.md` (these are the actual per-phase agent surfaces consumed by clauderun — verified during refinement; `.claude/agents/atdd/` today contains only meta agents and is not the consumer).

**Phase → file mapping (already exists, no new schema).** `internal/atdd/runtime/statemachine/process-flow.yaml` maps phase node ids to agent names via the `agent:` field on `user_task` nodes (e.g. `AT_RED_TEST` → `agent: at-red-test` → file `internal/assets/runtime/prompts/atdd/at-red-test.md`). The projection step reuses this; it does NOT add a new `phase:` frontmatter field or rely on filename-vs-phase-id case conversion.

What the projection does:

1. Reads `internal/atdd/phase-scopes.yaml` (embedded in the binary).
2. Reads the project's `gh-optivem.yaml paths:`.
3. Reads `internal/atdd/runtime/statemachine/process-flow.yaml` (embedded) to walk every `user_task` node and resolve its `agent:` → prompt filename.
4. For each `(phase_id, agent_name)` pair: look up the phase's layer list in `phase-scopes.yaml`, join with `paths:` to produce `{layer_name: resolved_path}`, and write this as the `scope:` field in `internal/assets/runtime/prompts/atdd/<agent_name>.md`'s frontmatter (in the user's project, *after* the scaffold/sync copies the prompts into place).
5. Adds a `# do not hand-edit — run \`gh optivem sync\` to refresh` comment alongside.
6. Phases on item 11's allowlist (`AT_GREEN_BACKEND`, `AT_GREEN_FRONTEND`) have their projected file's `scope:` written as `{}` plus a comment citing the deferred multitier plan — so the file shape is still consistent.

Example output in `internal/assets/runtime/prompts/atdd/at-red-system-driver.md` (post-sync):

```yaml
---
model: sonnet
effort: medium
scope:
  driver_port:    system-test/typescript/src/testkit/driver/port/shop
  driver_adapter: system-test/typescript/src/testkit/driver/adapter/shop
# do not hand-edit — projected from gh-optivem.yaml ⋈ phase-scopes.yaml ⋈ process-flow.yaml; run `gh optivem sync` to refresh
---
```

**Purpose of the projected `scope:` field.** Documentation/IDE/grep — not load-bearing for runtime enforcement. Runtime scope enforcement is `check_phase_scope` (item 8), which independently reads `phase-scopes.yaml` + `gh-optivem.yaml`. The projection makes the per-phase scope visible to humans inspecting the prompt file (and to any IDE tooling that parses frontmatter) without forcing them to cross-reference two yaml files mentally.

> **Refined 2026-05-18:** (1) Consumer surface resolved — runtime prompts (`internal/assets/runtime/prompts/atdd/*.md`), not new `.claude/agents/atdd/` subagents. **Why:** the runtime prompts are the existing per-phase agent surface; creating ~10 new Claude Code subagent files just to host a projected `scope:` field would be significant scope creep with no current consumer for the `Agent`-tool dispatch path. (2) Agent-to-phase mapping OPEN QUESTION dropped — BPMN's `user_task.agent:` field already provides the mapping; no filename-convention or `phase:`-frontmatter decision needed. **Why:** refinement surfaced that the mapping schema already exists in `process-flow.yaml`; reusing it keeps the BPMN surface as the single source of truth for "which agent runs in phase X". (3) Purpose of the projected `scope:` field clarified as documentation, not runtime enforcement. **Why:** the target-architecture preamble's "Agents read their own frontmatter `scope:`. Self-contained." claim was load-bearing only when the consumer was Claude Code subagents with frontmatter-aware dispatch; with runtime prompts (which are passed to Claude as text), the frontmatter isn't separately injected. The plan still delivers the feature — enforcement just lives in `check_phase_scope` (item 8), which was always its proper home anyway.

### 5. Validation rules in `gh optivem config validate`

Extend the validator (likely `internal/projectconfig/config.go` or sibling) with new error cases:

- **Pre-SSoT detection (migration error):** `system.sut_namespace` field present → hard error directing the user to `gh optivem config migrate`:
  ```
  ERROR: gh-optivem.yaml uses the pre-SSoT ATDD scope model
  (system.sut_namespace is set; paths: values are not fully resolved).

  Run:  gh optivem config migrate

  The migrate command will:
    - join sut_namespace into each paths:<key> value, deterministically;
    - join sut_namespace into system.path;
    - delete the system.sut_namespace field.

  Existing comments and key ordering are preserved. The command is
  idempotent and safe to re-run. Review the diff before committing,
  especially if your project flattened or added channel sub-dirs
  beyond the canonical layout.
  ```
  No inline suggested values — the user runs `config migrate` and reviews the resulting diff. Item 6 (the SSoT join step in `runConfigMigrate`) is the executor; this validator error is purely the trigger that tells users *why* and *how*.

- **`${...}` markers in `paths:` values** → hard error. "Under SSoT, paths must be fully resolved; substitution is scaffold-time-only."
- **Unknown fields anywhere in `gh-optivem.yaml`** → hard error. Applies uniformly at every nesting level (top-level, `system.*`, `system.backend.*`, `system.frontend.*`, `system_test.*`, `paths.*`, `external_systems.*`, `sonar.*`, etc.) so a typo like `system.namepsace` is caught with the same hard-fail as a typo at the root. Implementation: set `yaml.KnownFields(true)` on the top-level decoder. (`paths.*` strictness is satisfied by the next bullet; the recursive `KnownFields(true)` covers everything else.)
- **`paths.<name>` where `<name>` is not in `canonicalPathKeys()`** → hard error. Catches typos and stale keys. Note: under `KnownFields(true)`, `paths:` is a `map[string]string` so the strict-fields option doesn't auto-catch this — the validator must enumerate `paths:` keys against `canonicalPathKeys()` explicitly.

(Validation of `phase-scopes.yaml` itself — phase-name foreign-key against `process-flow.yaml`, layer-name validity against `canonicalPathKeys()` + `system_path` — is **out of scope for runtime validation** because `phase-scopes.yaml` is embedded in the binary and cannot drift between build and runtime. It lives entirely in item 11's build-time test surface. See Refined annotation below.)

> **Refined 2026-05-18:** (1) Dropped the two bullets that re-validated `phase-scopes.yaml` at runtime. **Why:** `phase-scopes.yaml` is embedded in the binary; it cannot drift between build and runtime. Item 11's build-time tests are the right surface. Per `[[feedback_drop_dont_relocate]]` — when an upstream mechanism already covers it, drop entirely. (2) Unknown-fields rule tightened to apply at **every** nesting level via `yaml.KnownFields(true)`. **Why:** uniform rule is easier to reason about than a "strict here, lax there" hybrid; catches the common nested-typo class (`system.namepsace` etc.) with the same hard-fail as a root typo. (3) Pre-SSoT error message reframed to **call** `DefaultPaths(testLang, systemTestPath, sutNamespace)` (item 3's new signature) instead of re-implementing the join. **Why:** single source of truth for the path-join rule. The validator's suggested values and a fresh scaffold's output are guaranteed identical.

### 6. Extend `gh optivem config migrate` with the SSoT join step

Edit `runConfigMigrate` in `config_commands.go:308+` to add a third deterministic back-fill (alongside the existing `project.provider` and `repos:` back-fills):

**SSoT back-fill — applies iff `system.sut_namespace` is present:**

1. Read `system.sut_namespace` value (call it `ns`).
2. For each key `k` in `paths:`, rewrite the value to `<current value> + "/" + ns` (or use the `DefaultPaths(testLang, systemTestPath, ns)` output where the current value matches the pre-SSoT default — preserves user-customized values, joins on top of customizations).
3. Rewrite `system.path` to `<current value> + "/" + ns`.
4. Delete the `system.sut_namespace` field.
5. Mark `changed = true` so the file is rewritten.

**Contract guarantees (inherited from existing migrate):**
- Idempotent — once `system.sut_namespace` is absent, the SSoT back-fill is a no-op on subsequent runs.
- Comment-preserving — uses the existing `yaml.Node` round-trip path (see `config_commands.go:319-322`).
- Independent of other back-fills — runs in the same pass as `project.provider` / `repos:` for configs predating multiple bumps, or alone for configs that already migrated those fields.

**Update the `Long` and `Short` doc strings** at `config_commands.go:266-282` to add the SSoT bullet:

```
• Migrates to the SSoT path model: joins system.sut_namespace into each
  paths:<key> value and into system.path, then deletes system.sut_namespace.
  Applied when system.sut_namespace is present in the file.
```

**Test cases to add (sibling to existing `config_commands_test.go`):**
- Pre-SSoT config (sut_namespace set, paths: unresolved) → migrate produces fully-resolved paths and drops sut_namespace.
- Post-SSoT config (sut_namespace absent) → no-op.
- Customized paths: value (user added a channel sub-dir) → migrate appends sut_namespace on top of customization, doesn't clobber.
- Pre-SSoT + missing `project.provider` → both back-fills run in one pass.
- Comment preservation — assert that a leading `# my custom comment` on `paths:` survives the rewrite.

> **Refined 2026-05-18:** Item replaced — was "Author migration doc at `docs/atdd/process/migration-ssot.md`"; now "Extend `gh optivem config migrate` with the SSoT join step". **Why:** during refinement the user surfaced that an idempotent, comment-preserving `gh optivem config migrate` command already exists (`config_commands.go:247`, back-filling `project.provider` and `repos:`). The canonical-layout case (probably 90%+ of pre-SSoT configs) auto-migrates deterministically via the same `DefaultPaths` join from item 3 — no AI in the loop, no need for a separate doc. The validator error in item 5 now points users at `config migrate`, which acts as the executor. The original migration doc would have duplicated content already in `config migrate --help` and the validator error message; deleted per `[[feedback_drop_dont_relocate]]`. Locked decision ε revised in lockstep (see preamble).

### 7. Phase doc rewrites — §Scope sections point at `scope.md` + runtime-prompt frontmatter

Sweep `docs/atdd/process/change/behavior/**/*.md` for `^## Scope` and rewrite each §Scope section. **The `## Scope` heading is retained** so a `grep` for it still finds every per-phase scope statement; only the *contents* change.

**Files in scope (9, enumerated at refinement 2026-05-18 to avoid re-sweeping at execute time):**

AT-cycle behavior docs (5):
- `1.1-at-red-test.md`
- `1.2-at-red-dsl.md`
- `1.3-at-red-system-driver.md` *(loses its `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/` placeholders)*
- `2-at-green-system.md`
- `3-at-refactor.md`

CT-cycle behavior docs (4):
- `ct/1.1-ct-red-test.md`
- `ct/1.2-ct-red-dsl.md`
- `ct/1.3-ct-red-external-driver.md`
- `ct/2-ct-green-stubs.md`

**Replacement shape** for each §Scope section:

```markdown
## Scope

This phase touches the `<layer_a>`, `<layer_b>`, `<layer_c>` layers (bare
names; resolved physical paths live in `gh-optivem.yaml paths:` and in
the runtime prompt's projected `scope:` frontmatter).

See [docs/atdd/process/shared/scope.md](../../shared/scope.md) for the
scope rule.
```

The bare layer names match `phase-scopes.yaml`'s entry for the phase. The relative link path adjusts per-file (`../../shared/scope.md` from `change/behavior/`, `../../../shared/scope.md` from `change/behavior/ct/`).

**Files NOT in scope** (with reasons):

- `1-sir-write.md`, `2-chore-write.md` — structure-cycle docs. Structure phases are BPMN `call_activity` invocations of a generic `structural_cycle` process (`process-flow.yaml:1100-1141`), not `user_task` nodes; they have no entry in `phase-scopes.yaml` and no projected runtime-prompt `scope:` frontmatter. Plus both files are currently uncommitted (user is mid-authoring per `git status`). Deferred to `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md` (filed during this refinement) — picks up when structure cycle's BPMN shape is settled.
- `internal/assets/global/docs/atdd/process/cycles.md` and the other asset-template phase docs — per the predecessor plan's out-of-scope ruling: "the source-of-truth for phase docs is the `docs/atdd/process/change/behavior/` tree per recent commits". Asset-template consolidation is a separate sibling.

**Convention** (codified in `docs/atdd/process/shared/scope.md` from item 2):
- Phase docs reference layer **names** only (no `${...}` placeholders, no path values).
- The authoritative per-phase scope is the runtime-prompt frontmatter `scope:` (per item 4).
- Enforcement at WRITE time is `check_phase_scope` (per item 8).

> **Refined 2026-05-18:** (1) Sweep enumerated to 9 files instead of "sweep all". **Why:** the 11 candidate files surfaced two policy questions (structure-cycle docs in/out, CT-cycle docs in/out) that needed answers, not silent decisions. Listing them at refinement time forces those decisions to be visible. (2) Structure-cycle docs deferred. **Why:** their phases are BPMN call_activity invocations, not user_tasks; SSoT machinery doesn't apply yet; the docs are uncommitted and being authored. Filed as `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md`. (3) CT-cycle docs included. **Why:** CT phases are user_tasks; same SSoT model applies; CT-vocabulary plan (20260518-1742) is already a hard prerequisite of this plan so by item 7's execute time the vocabulary exists. (4) `## Scope` heading retained, contents replaced. **Why:** grep-discoverability — readers searching for "## Scope" still find every per-phase scope statement. (5) Replacement shape includes the bare-layer-name sentence + the scope.md link; concrete relative-path examples given.

### 8. Retire BPMN Snapshot A; rewire `check_phase_scope`; drop param-threading

Edit `plans/20260518-1144-atdd-bpmn-orchestration.md`. Three changes:

**(a) §Conventions snapshot — Snapshot A.** Delete the per-phase allowed-paths table entirely (μ resolved 2026-05-18). Replace with a 1-line citation:

> *Per-phase allowed-paths assignment is defined in `internal/atdd/phase-scopes.yaml` (per the SSoT phase-scope plan, [20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)). This plan no longer owns that assignment. `check_phase_scope` (item 5 below) reads it directly from the embedded yaml at runtime.*

Git history preserves the prior shape; no need to leave a frozen reference in the plan file.

**(b) Item 5 — `check_phase_scope` action.** Rewrite the action's wiring story:

- Reads `internal/atdd/phase-scopes.yaml` (embedded in the binary) for the per-phase layer list, keyed by the current BPMN node's `id:` (e.g. `AT_RED_TEST`).
- Reads `gh-optivem.yaml paths:` for resolution (literal values, no `${...}` substitution — substitution retired per locked δ).
- Joins to produce `{layer_name: resolved_path}` for the current phase, identically to item 4's sync-time projection.
- Runs `git diff --name-only <pre-phase-ref> HEAD` (and `git status --porcelain` for unstaged modifications).
- Matches each diff path as **directory-aware** prefix against the resolved scope list (per the contract handed off from item 3, λ resolved 2026-05-18). A diff path matches an allowed path P iff `diffPath == P || strings.HasPrefix(diffPath, P+"/")`. Raw `strings.HasPrefix(diffPath, P)` is wrong — it would let `.../shop` match `.../shop2/...`.
- On out-of-scope diff: errors with a message pointing to `docs/atdd/process/shared/scope.md` (per item 2).
- For phases on item 11's allowlist (`AT_GREEN_BACKEND`, `AT_GREEN_FRONTEND`): no-op (return success without checking diff), per the deferred multitier plan. Log a single line citing the deferred plan so the absence of enforcement is visible.

**Old wiring to remove from item 5's prose:**
- `Config.PlaceholderMap()` substitution path (no longer used — paths are literal).
- "values come from Snapshot A" framing (Snapshot A is gone).
- The `allowed_paths` `params:` threading from `process-flow.yaml` to the action — `check_phase_scope` now reads `phase-scopes.yaml` directly by node id; per-node `allowed_paths` params are no longer needed.

**(c) §Node params tables (BPMN plan lines 218–236).** Delete (or replace with cross-reference). These tables thread `allowed_paths: "<Snapshot A row: ...>"` strings into per-node `params:` blocks in `process-flow.yaml`. Under SSoT, the action looks up its phase's scope from `phase-scopes.yaml` keyed by node id — **no per-node `allowed_paths` param is needed in `process-flow.yaml`**. The whole threading mechanism is obsolete. Replace the tables with: *"Per-node `allowed_paths` params are no longer needed; `check_phase_scope` reads its phase's scope from `internal/atdd/phase-scopes.yaml` directly by node id. See [SSoT plan (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md) item 8 for the rewired action."*

> **Refined 2026-05-18:** (1) μ resolved — Snapshot A deleted entirely with 1-line citation; no frozen reference. **Why:** SSoT means one source; "frozen reference + caveat" is two sources that promise to stay in sync — exactly the failure mode SSoT exists to prevent. Git history preserves the prior shape. (2) Added explicit (c) bullet — the per-node `allowed_paths` param-threading in BPMN plan lines 218–236 is also obsolete under SSoT (the action reads `phase-scopes.yaml` directly by node id, no params needed). **Why:** without calling this out explicitly, execute-plan would update Snapshot A + item 5 prose but leave the param-threading tables intact, producing a half-migrated BPMN plan. (3) Added explicit "for allowlist phases, no-op + log" behavior to the action. **Why:** item 1's AT-GREEN handling decision left `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND` unscoped pending the deferred multitier plan; the runtime needs explicit per-phase behavior, not a "missing key" panic.

### 9. Retire `placeholders.md`; replace with `path-keys.md`

Material rewrite + rename (fate resolved 2026-05-18). The current `internal/assets/global/docs/atdd/process/placeholders.md` is post-SSoT obsolete on its premise: the entire substitution-at-sync-time mechanism it documents (lines 47–56) is retired, and its "use `${name}` for any path component that varies" editing guidance (lines 57–65) is the *opposite* of item 7's "phase docs reference bare layer names" rule. Surgical edits would leave a doc whose central story is false.

**Action: rename and rewrite.**

1. **Rename file:** `internal/assets/global/docs/atdd/process/placeholders.md` → `internal/assets/global/docs/atdd/process/path-keys.md`.
2. **Rewrite contents** around the new framing: this doc describes the **canonical path-key vocabulary** consumed by `gh-optivem.yaml paths:` and `internal/atdd/phase-scopes.yaml`. No substitution mechanics. Suggested section structure:
   - **Two key families** (Family A = top-level `gh-optivem.yaml` fields; Family B = user-extensible keys under `paths:`). Same two-family terminology survives.
   - **Family A path-shaped keys** (today: `system_path`, `system_test_path`). Note that `system_test_path` is the parent of every Family B testkit key and is **not** a valid `phase-scopes.yaml` layer (per item 1's γ resolution).
   - **Family B canonical keys** (`canonicalPathKeys()`'s seven/eight entries). Note that values are fully-resolved physical paths set at scaffold time (per item 3) — no runtime substitution.
   - **User-owns-values, gh-optivem-owns-names** rule, with the Family-B-shadow guard (`paths.<reserved>` rejected by validator per item 5).
   - **Pointer** to `docs/atdd/process/shared/scope.md` (item 2) for the scope rule that consumes these keys; pointer to `gh-optivem.yaml` schema docstrings for authoritative field descriptions.
3. **Explicitly document the removed mechanism** in a one-paragraph "Historical note" at the bottom: pre-SSoT, `${name}` placeholders were resolved at sync time against a `PlaceholderMap`; that mechanism is retired. Pre-SSoT projects migrate via `gh optivem config migrate` (per item 6).
4. **Remove all `${...}` syntax references** in the new doc body — the new doc never demonstrates substitution because substitution is dead. The Historical note is the only place `${name}` appears.

**References to update** (blast radius is tiny — verified at refinement):

- `internal/projectconfig/paths_defaults.go` — replace `placeholders.md` mentions with `path-keys.md`.
- `internal/projectconfig/paths_defaults_test.go` — same.
- No phase doc links to `placeholders.md` today (verified). No additional grep needed.
- Scaffolded project copies refresh on next `gh optivem sync` automatically — existing scaffolded projects get the new file by name, no manual user step.

> **Refined 2026-05-18:** Item rewritten from "edit placeholders.md to note substitution retired + remove sut_namespace" to "rename to path-keys.md + material rewrite". **Why:** the doc's central story (sync-time `${name}` substitution into phase docs) is **gone** post-SSoT; the file's framing was load-bearing only when substitution was alive. Surgical edits would leave the doc actively misleading. Renaming keeps the doc valuable (canonical-key vocabulary IS worth documenting in one place) while making the filename match the content. Blast radius confirmed tiny — only 2 Go files reference the current name, no phase docs link to it.

## Out of scope

- **Family B substrate vocabulary additions** — owned by the [predecessor plan (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) items 1–3.
- **Architecture-doc vocabulary alignment** — `dsl-port.md` / `dsl-core.md` / etc. already use the hexagonal vocabulary this plan reads from. No edits needed.
- **Renaming existing Family B keys** — none; the locked hexagonal naming holds.
- **Modeling concrete testkit layouts in the `shop` template repo or in `gh-optivem` example trees** — flagged as a separate follow-up during predecessor refinement. Park as its own plan after this one lands.
- **Generalising the scope mechanism to non-ATDD workflows** — out of scope; this is ATDD-specific.

## Open questions to resolve during /refine-plan

- ~~**(η) File locations.**~~ — resolved 2026-05-18: standalone `internal/atdd/phase-scopes.yaml`. `docs/atdd/process/shared/` directory was confirmed to exist (verified during refinement).
- ~~**(μ) Snapshot A retention.**~~ — resolved 2026-05-18: delete entirely with 1-line citation; git history preserves the prior shape.
- ~~**(ζ) Schema validation specifics.**~~ — resolved 2026-05-18: tests live at `internal/atdd/phase_scopes_test.go` (co-located with the yaml); assertion shape pinned in item 11 (5 asserts including the allowlist mechanism).
- ~~**(λ) Path-match semantics in `check_phase_scope`.**~~ — resolved 2026-05-18: no trailing slashes in `paths:` values; matcher does directory-aware prefix matching (item 8 implements per the contract in item 3).
- **(θ) Multi-SUT projects.** If a project has multiple SUTs (unusual today but possible), how do they map? Single `sut_namespace`-per-project remains the model unless a real use case surfaces.
- ~~**Agent-to-phase mapping.**~~ — resolved 2026-05-18: consumer surface is `internal/assets/runtime/prompts/atdd/*.md` (runtime prompts), and the mapping already exists via BPMN's `user_task.agent:` field in `process-flow.yaml`. No new mapping schema needed.
- ~~**`ct_test` key.**~~ — resolved 2026-05-18: fresh dedicated plan at `plans/20260518-1742-family-b-stems-and-ct-vocab.md`.
- ~~**placeholders.md scope.**~~ — resolved 2026-05-18: material rewrite + rename to `path-keys.md` (item 9). Substitution premise is post-SSoT obsolete; surgical edits would leave the doc misleading.

## Hand-off

**Hard dependencies:**
- Predecessor plan ([20260518-1500](20260518-1500-atdd-phase-scope-placeholders.md)) items 1–3 must land before this plan executes. The Family B keys `at_test`, `dsl_port`, `dsl_core` must exist in `canonicalPathKeys()` for `phase-scopes.yaml` (item 1) to reference them safely.
- CT-vocabulary plan ([20260518-1742](20260518-1742-family-b-stems-and-ct-vocab.md)) must land before or alongside this plan. The Family B key `ct_test` (referenced by `CT_RED_TEST` in `phase-scopes.yaml`) is defined there.

**Execute order (refined 2026-05-18; updated 2026-05-19 post-partial-execute):**

1. ~~Items 1, 2~~ — ✅ landed 2026-05-18.
2. ~~Item 11~~ — ✅ landed 2026-05-18 with broadened FK rule (every node dispatching a concrete writing agent — `user_task` or `call_activity` — must be in `phase-scopes.yaml` or on `phasesDeferredByPlan`). Allowlist active for `AT_GREEN_BACKEND`, `AT_GREEN_FRONTEND` (multitier plan), `SYSTEM_INTERFACE_REDESIGN_CYCLE`, `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`, `CHORE_CYCLE` (structure-cycle plan).
3. ~~Items 3, 10~~ — ✅ landed 2026-05-19. `DefaultPaths(testLang, systemTestPath, sutNamespace)` signature live; scaffolder bakes sutNamespace into `System.Path` (monolith) and `paths:` testkit values; `System.SutNamespace` field dropped; `c.SutNamespace()` accessor now derives purely from `System.Repo`. Migrate (`config_commands.go:inferPathDefaults`) passes `""` for sutNamespace pending item 6's SSoT join step. Runtime prompts carry `scope: {}` placeholder (7 standard + 2 allowlist citing the deferred multitier plan).
4. Item 4 (sync projection into runtime-prompt frontmatter) — wires existing scaffolds to refresh `scope:` on next `gh optivem sync`. Mapping comes from BPMN `user_task.agent:` — no new schema.
5. Items 5 + 6 (validator + extended `config migrate`) — together: item 6 implements the SSoT join in `runConfigMigrate`, item 5's hard error points users at it. Both depend on item 3's new `DefaultPaths` signature.
6. Item 8 (BPMN plan edits + `check_phase_scope` runtime rewire) — depends on `phase-scopes.yaml` being live; deletes Snapshot A; drops obsolete param-threading (BPMN plan lines 218–236); allowlist phases get no-op + log.
7. Item 7 (phase doc §Scope sweep — 9 files, 5 AT + 4 CT). Structure-cycle docs deferred per the new sibling plan.
8. Item 9 (rename `placeholders.md` → `path-keys.md` + material rewrite). Last — doc consistency catches up to the now-true runtime story.

**Coordinate with rename plan ([20260519-0704](20260519-0704-bpmn-external-system-naming-consistency.md)):** the rename plan also touches `paths_defaults.go`, `process-flow.yaml`, CT prompt files, and the `phase-scopes.yaml` CT rows. Whichever lands first, the other must be re-walked for conflicts.

**Pre-execute check:** grep `plans/*.md` and `plans/deferred/*.md` for any concurrent agent pickup markers on the files touched here — the BPMN plan, `paths_defaults.go`, `config_commands.go`, `config.go`, `optivem_yaml.go`, `path-keys.md` (currently `placeholders.md`), runtime-prompt templates under `internal/assets/runtime/prompts/atdd/`, and the new `internal/atdd/phase-scopes.yaml` location — before adding this plan's marker, per `[[feedback_check_concurrent_agents]]`. As of refinement, the CT-vocabulary plan ([20260518-1742](20260518-1742-family-b-stems-and-ct-vocab.md)) is being refined by a parallel agent; coordinate before this plan's execute starts.

**Post-execute:** rerun `gh optivem sync` in any active scaffolded repo to refresh the runtime-prompt `scope:` frontmatter from the new `phase-scopes.yaml`. Existing scaffolded projects also pick up the renamed `path-keys.md` (replaces `placeholders.md`) automatically via sync.
