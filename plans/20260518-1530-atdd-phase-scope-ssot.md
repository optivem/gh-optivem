# Plan: ATDD phase-scope single source of truth (SSoT)

> ⚠️ **NOT YET REFINED** — drafted from the refinement discussion of [predecessor plan 20260518-1500](20260518-1500-atdd-phase-scope-placeholders.md). Run `/refine-plan` on this file before `/execute-plan`. Several items below carry **OPEN QUESTION** markers that need user decisions during refinement.

**Date:** 2026-05-18

**Predecessor:** [Phase-scope placeholders substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — adds the Family B keys (`at_test`, `dsl_port`, `dsl_core`) this plan consumes. This plan MUST land after the predecessor's items 1–3 are merged.

**Context.** The predecessor plan adds Family B *substrate* keys but stops short of consolidating where per-phase scope ASSIGNMENT lives. Today that assignment is duplicated across three surfaces: phase doc §Scope sections, BPMN Snapshot A's per-phase table, and the `check_phase_scope` runtime in BPMN. This plan introduces a single doctrinal source of truth and rewires all three consumers to read from it. It also resolves a sister duplication: today's `gh-optivem.yaml paths:` values are *base paths* that get composed with the Family A `${sut_namespace}` placeholder at use; this plan makes them *fully resolved* (sut_namespace baked in) and retires runtime substitution.

**Sibling plans referenced:**
- [Predecessor — Phase-scope placeholders substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — adds the Family B keys this plan's `phase-scopes.yaml` references.
- [BPMN orchestration plan (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md) — consumer. Snapshot A retires (item 8 of this plan); `check_phase_scope` (BPMN item 5) gets rewired (item 8 of this plan). Header sibling-plans entry added (the predecessor's item 7a covers this).
- [BPMN plan §Conventions snapshot (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md#%C2%A7conventions-snapshot-inlined-to-survive-the-upcoming-split) — the canonical Snapshot A this plan retires.

## Locked architecture decisions

Recorded here so the items below are unambiguous. All decided during the refinement of the predecessor plan, 2026-05-18.

- **β1 — Doctrinal key names.** Family B layer names (`driver_port`, `dsl_port`, etc.) are canonical ATDD vocabulary. Same in every gh-optivem project. Users own VALUES (physical paths), not NAMES.
- **Architecture/phase separation.** Layer names live with the architecture (defined by the canonical key set in `internal/projectconfig/paths_defaults.go canonicalPathKeys()`). Phases consume a SUBSET of layers per phase. Two independent doctrinal structures, joined at runtime.
- **δ — Fully resolved paths.** `gh-optivem.yaml paths:` values are concrete fully-resolved paths (e.g. `system-test/typescript/src/testkit/driver/port/shop`). No `${name}` substitution at runtime. `system.sut_namespace` is dropped from config — it's a scaffold-time-only input used to generate the resolved values, then not persisted. `${sut_namespace}` Family A placeholder retires.
- **γ — Family A path-shaped keys participate in scope identically to Family B.** Because `system.path` is now fully resolved, GREEN's scope can reference `system_path` like any other layer. No "Family A vs Family B" special-casing in scope.
- **ε — Migration is human-only.** Pre-SSoT configs fail validation with a hard error + deterministic suggested values printed in the error message. The user inspects against their actual layout and edits `gh-optivem.yaml` themselves. No auto-write. No AI in the loop.
- **All-error validation.** Unknown / obsolete fields are hard errors, not warnings. Consistent with existing `gh optivem config validate` strictness.

## Target architecture

**Tool source (gh-optivem):**
- `internal/projectconfig/paths_defaults.go` — canonical layer name list (extended by predecessor's item 1) + per-language default *base* path stems used at scaffold time.
- `internal/atdd/phase-scopes.yaml` *(new)* — phase → list of layer names. The doctrinal source of truth for per-phase scope.

**Scaffolded project:**
- `gh-optivem.yaml paths:` — layer name → fully-resolved physical path. **The only place users edit paths.**
- `.claude/agents/atdd/*.md` frontmatter `scope:` *(new field)* — projected `{layer_name: resolved_path}` map. Marked "do not hand-edit — run `gh optivem sync` to refresh". Regenerated from `gh-optivem.yaml paths:` ⋈ `phase-scopes.yaml` for the agent's phase.
- `docs/atdd/process/shared/scope.md` *(new)* — the **rule** ("only modify paths listed in your `scope:`; if you need more, alert the user"). Single place. Phase docs link here.
- `docs/atdd/process/**.md` — phase docs. Reference layer NAMES by bare name (e.g. "RED-TEST writes to `driver_port`, `dsl_core`"). No `${...}` syntax, no path values.

**Runtime:**
- Agents read their own frontmatter `scope:`. Self-contained.
- `check_phase_scope` reads `internal/atdd/phase-scopes.yaml` (embedded) for the per-phase layer list + `gh-optivem.yaml paths:` (in the user's project) for resolution + `git diff --name-only` for actual changes. Matches resolved paths as prefixes against the diff.

## Items

### 1. Create `internal/atdd/phase-scopes.yaml`

Add a new yaml file at `internal/atdd/phase-scopes.yaml` *(OPEN QUESTION — η — confirm location during refinement; alternatives include `internal/atdd/doctrine/phase-scopes.yaml`, folding into `internal/atdd/runtime/statemachine/process-flow.yaml`)*.

Schema:

```yaml
phases:
  AT-RED-TEST:          [at_test, dsl_port, dsl_core]
  AT-RED-DSL:           [dsl_core, driver_port]
  AT-RED-SYSTEM-DRIVER: [driver_port, driver_adapter]
  AT-GREEN:             [system_path]
  CT-RED-TEST:          [ct_test, dsl_port, dsl_core]            # OPEN QUESTION — exact CT layer set
  CT-RED-DSL:           [dsl_core, external_driver_port]         # OPEN QUESTION
  CT-RED-EXTERNAL-DRIVER: [external_driver_port, external_driver_adapter]
  CT-GREEN-STUBS:       [external_driver_adapter]                # OPEN QUESTION
```

- Phase names match the BPMN process-flow's phase identifiers.
- Layer names match keys in `canonicalPathKeys()` from `internal/projectconfig/paths_defaults.go` (Family B) OR Family A path-shaped keys (`system_path`).

**OPEN QUESTION — γ extension:** confirm `system_path` is the only Family A key that's path-shaped enough to appear here, vs. also `system_test_path`. Walk through actual phase needs during refinement.

**OPEN QUESTION — ct_test key:** the predecessor plan punted CT keys to "a sibling plan". Under the SSoT architecture, CT is just more rows here — but `ct_test` is a new Family B key (parallel to `at_test`). Decide during refinement: add `ct_test` to the predecessor's item 1 list (small predecessor amendment), or add it here as a Family B vocabulary extension.

### 2. Create `docs/atdd/process/shared/scope.md`

Author a new shared doc at `docs/atdd/process/shared/scope.md` *(OPEN QUESTION — η — confirm `shared/` exists or needs to be created; check `docs/atdd/process/` tree before writing)*.

Content (sketch — refine for tone during /refine-plan):

```markdown
# Scope rule

Every ATDD phase has a **scope**: the set of paths the agent for that
phase is allowed to modify. Your agent's `scope:` frontmatter lists
those paths, fully resolved.

**Rule:** in a given phase, only modify files under paths listed in
your `scope:`. If the task appears to require touching paths outside
scope, **stop and alert the user** rather than expanding scope
silently — scope creep is usually a sign that the phase boundary is
wrong, the test scope is wrong, or a refactor is needed first.

(The per-phase layer assignment that produced your `scope:` is
defined doctrinally in `internal/atdd/phase-scopes.yaml` within
gh-optivem; the resolved paths come from `gh-optivem.yaml paths:` in
this project.)
```

Phase docs and the `check_phase_scope` runtime point users here when scope is breached.

### 3. Scaffolder change — write fully-resolved paths at scaffold

Edit the scaffolder (location TBD — likely under `internal/projectconfig/` or `internal/scaffold/`) so that at scaffold time it:

1. Reads the scaffold-time inputs (language, architecture, system.repo for sut_namespace default, etc.).
2. Generates `gh-optivem.yaml` with `paths:` values **fully resolved**, sut_namespace already joined in. Example for a TS shop project:
   ```yaml
   paths:
     driver_port:    system-test/typescript/src/testkit/driver/port/shop
     driver_adapter: system-test/typescript/src/testkit/driver/adapter/shop
     # ...
   ```
3. Writes `system.path` fully resolved (`src/main/java/shop`, not `src/main/java`).
4. **Does NOT write `system.sut_namespace`** — the field is retired.

**OPEN QUESTION — λ — path-trailing slashes:** decide whether resolved paths end in `/` for prefix-match safety (`driver_port: system-test/.../port/shop/` so that an accidentally-named sibling `shop2` doesn't match) or stay unslashed (consistent with current scheme). Affects `check_phase_scope` matching semantics.

### 4. Scaffolder/sync change — project `scope:` into agent frontmatter

Edit the scaffolder/sync layer so that on scaffold AND on `gh optivem sync` it:

1. Reads `internal/atdd/phase-scopes.yaml` (embedded in the binary).
2. Reads the project's `gh-optivem.yaml paths:`.
3. For each agent file under `.claude/agents/atdd/`, maps the agent → its phase (via filename convention or a `phase:` frontmatter field — **OPEN QUESTION**, decide during refinement), looks up the phase's layer list, joins with `paths:` to produce `{layer_name: resolved_path}`, and writes this as the `scope:` field in the agent's frontmatter.
4. Adds a `# do not hand-edit — run \`gh optivem sync\` to refresh` comment alongside.

Example output in `.claude/agents/atdd/at-red-system-driver.md`:

```yaml
---
name: at-red-system-driver
description: ...
scope:
  driver_port:    system-test/typescript/src/testkit/driver/port/shop
  driver_adapter: system-test/typescript/src/testkit/driver/adapter/shop
# do not hand-edit — projected from gh-optivem.yaml ⋈ phase-scopes.yaml; run `gh optivem sync` to refresh
---
```

**OPEN QUESTION — agent-to-phase mapping:** today, is the mapping (a) implicit from filename (`at-red-system-driver.md` → `AT-RED-SYSTEM-DRIVER`)? (b) explicit via a `phase:` frontmatter field? (c) other? Inspect existing agent files during refinement.

### 5. Validation rules in `gh optivem config validate`

Extend the validator (likely `internal/projectconfig/config.go` or sibling) with new error cases:

- **Pre-SSoT detection (migration error):** `system.sut_namespace` field present → hard error with migration message and deterministic suggested values:
  ```
  ERROR: gh-optivem.yaml uses the pre-SSoT ATDD scope model.

  Under the SSoT model, `paths:` values must be fully-resolved concrete
  paths (include the SUT namespace and any custom sub-dirs your project
  uses). `system.sut_namespace` is no longer a config field.

  Inspect your actual layout, then edit gh-optivem.yaml. Suggested
  values (VERIFY AGAINST YOUR TREE BEFORE SAVING):

    paths:
      driver_port: system-test/typescript/src/testkit/driver/port/shop
      ... (per current paths: keys, with `/$sut_namespace` appended)
    system:
      path: src/main/java/shop   # was: src/main/java
      # drop the sut_namespace field

  See docs/atdd/process/migration-ssot.md for details.
  ```
  Suggested values are deterministic string-joins of current `paths:` value + `system.sut_namespace`. The tool does not write them. User edits manually.

- **`${...}` markers in `paths:` values** → hard error. "Under SSoT, paths must be fully resolved; substitution is scaffold-time-only."
- **Unknown top-level fields in `gh-optivem.yaml`** → hard error. Catches typos and stale schemas.
- **`paths.<name>` where `<name>` is not in `canonicalPathKeys()`** → hard error. Catches typos and stale keys.
- **Phase name in `phase-scopes.yaml` not matching any BPMN process-flow phase** → build-time error (tested in CI). **OPEN QUESTION — ζ — exact test location**.
- **Layer name in `phase-scopes.yaml` not in `canonicalPathKeys()` AND not a Family A path-shaped key** → build-time error.

### 6. Migration doc — `docs/atdd/process/migration-ssot.md`

Author a migration doc that the validator error in item 5 points to. Content:

- Why the migration (one paragraph): paths now fully resolved, sut_namespace retired as a placeholder.
- Concrete before/after example for `gh-optivem.yaml`.
- Note on customized layouts: if you flattened or added channel sub-dirs, your suggested values from the validator are starting points only; verify against your actual tree.
- Note: post-migration, agent frontmatter `scope:` is auto-projected by `gh optivem sync`. Do not hand-edit.
- Note: no rollback path — pre-SSoT shape is unsupported going forward.

### 7. Phase doc rewrites — drop §Scope, link to scope.md

Edit each existing phase doc under `docs/atdd/process/change/behavior/`:

- `1.1-at-red-test.md` — replace §Scope prose with a paragraph referencing the agent's `scope:` field and `scope.md`. Bare layer names where useful for prose ("This phase touches `at_test`, `dsl_port`, `dsl_core`").
- `1.2-at-red-dsl.md` — same.
- `1.3-at-red-system-driver.md` — same (loses its `${name}/${sut_namespace}/` placeholders).
- Plus any other phase doc with a §Scope section (sweep `docs/atdd/process/**/*.md` for `## Scope`).

**Convention** (codified somewhere in process docs):
- §Scope sections are *removed* from phase docs.
- Phase docs MAY mention layer names in prose for clarity, but the *authoritative* per-phase scope is the agent's `scope:` frontmatter (sourced from `phase-scopes.yaml`).

### 8. Retire BPMN Snapshot A; rewire `check_phase_scope`

Edit `plans/20260518-1144-atdd-bpmn-orchestration.md`:

- **§Conventions snapshot — Snapshot A.** Delete the per-phase allowed-paths table. Replace with: "Per-phase allowed-paths assignment is defined in `internal/atdd/phase-scopes.yaml` and consumed by `check_phase_scope` (item 5 below). This plan does not own that assignment; see [SSoT plan (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md)."
- **Item 5 — `check_phase_scope`.** Rewrite the action's wiring story:
  - Reads `internal/atdd/phase-scopes.yaml` (embedded) for the per-phase layer list.
  - Reads `gh-optivem.yaml paths:` for resolution (literal values, no `${...}` substitution).
  - Matches `git diff --name-only` output as path-prefix against the resolved scope list.
  - On out-of-scope diff: errors with a message pointing to `scope.md`.

**OPEN QUESTION — μ — Snapshot A retention:** the SSoT plan's recommendation is to delete the table entirely (cite `phase-scopes.yaml` instead). Alternative: keep a frozen "Snapshot as of {date}" for plan-reading convenience, with a "do not consult for runtime truth — see `phase-scopes.yaml`" caveat. Decide during /refine-plan.

### 9. `placeholders.md` doctrine update

Edit `internal/assets/global/docs/atdd/process/placeholders.md`:

- Note that runtime substitution is retired. `${name}` placeholders in source-tree templates are expanded **at scaffold time** to produce `gh-optivem.yaml paths:` values. After scaffold, paths are literal.
- Remove `${sut_namespace}` from Family A — the placeholder no longer exists at runtime.
- **OPEN QUESTION — placeholders.md fate:** does the entire doc need rewriting (substitution model is materially different) or just edits? The "two consumers" line (phase docs + agent prompts) shifts to "one consumer at scaffold time" — material change. Walk the doc during /refine-plan.

### 10. Asset-template agent frontmatter — `scope:` placeholder marker

For each agent template under `internal/assets/.../agents/atdd/`, add a `scope:` placeholder to the frontmatter that the sync layer (item 4) populates. Example template:

```yaml
---
name: at-red-system-driver
description: ...
scope: {}   # populated by `gh optivem sync` from gh-optivem.yaml ⋈ phase-scopes.yaml
---
```

The template ships with empty `scope: {}`; sync overwrites on first run.

### 11. Tests + schema validation for `phase-scopes.yaml`

Add Go tests that, at build time, assert:

- Every phase name in `phase-scopes.yaml` matches a real BPMN process-flow phase identifier (cross-reference against `process-flow.yaml`).
- Every layer name referenced is in `canonicalPathKeys()` OR is a known Family A path-shaped key (`system_path`).
- No duplicate layer references within a phase's list.
- All values are non-empty arrays.

These catch drift between the architecture, the BPMN process flow, and the scope doctrine at CI time.

## Out of scope

- **Family B substrate vocabulary additions** — owned by the [predecessor plan (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) items 1–3.
- **Architecture-doc vocabulary alignment** — `dsl-port.md` / `dsl-core.md` / etc. already use the hexagonal vocabulary this plan reads from. No edits needed.
- **Renaming existing Family B keys** — none; the locked hexagonal naming holds.
- **Modeling concrete testkit layouts in the `shop` template repo or in `gh-optivem` example trees** — flagged as a separate follow-up during predecessor refinement. Park as its own plan after this one lands.
- **Generalising the scope mechanism to non-ATDD workflows** — out of scope; this is ATDD-specific.

## Open questions to resolve during /refine-plan

- **(η) File locations.** `internal/atdd/phase-scopes.yaml` vs `internal/atdd/doctrine/phase-scopes.yaml` vs folding into `process-flow.yaml`. Also `docs/atdd/process/shared/scope.md` — does `shared/` exist?
- **(μ) Snapshot A retention.** Delete entirely vs. keep as frozen reference with caveat.
- **(ζ) Schema validation specifics.** Where do the tests live; exact assertion shape.
- **(λ) Path-match semantics in `check_phase_scope`.** Prefix vs glob; trailing-slash policy.
- **(θ) Multi-SUT projects.** If a project has multiple SUTs (unusual today but possible), how do they map? Single `sut_namespace`-per-project remains the model unless a real use case surfaces.
- **Agent-to-phase mapping.** Inspect existing `.claude/agents/atdd/*.md` to see whether mapping is by filename, frontmatter `phase:`, or other.
- **`ct_test` key.** Add via small amendment to predecessor item 1, or fold here as a Family B extension.
- **placeholders.md scope.** Material rewrite vs. surgical edits.

## Hand-off

**Hard dependency:** predecessor plan ([20260518-1500](20260518-1500-atdd-phase-scope-placeholders.md)) items 1–3 must land before this plan executes. The Family B keys `at_test`, `dsl_port`, `dsl_core` must exist in `canonicalPathKeys()` for `phase-scopes.yaml` (item 1) to reference them safely.

**Execute order (proposed — refine):**

1. Items 1, 2 (yaml + scope.md) — pure new files, no runtime impact yet.
2. Item 11 (tests for `phase-scopes.yaml`) — guards item 1's correctness before downstream items consume it.
3. Items 3, 10 (scaffolder + agent template `scope:` marker) — together; new scaffolds get fully-resolved paths + populated frontmatter.
4. Item 4 (sync projection) — wires existing scaffolds to refresh frontmatter on next sync.
5. Item 5 (validator) — once items 3–4 work, the migration error is meaningful (suggesting fully-resolved values that match what new scaffolds get).
6. Item 6 (migration doc) — referenced by item 5's error.
7. Item 8 (BPMN plan + `check_phase_scope` runtime) — depends on `phase-scopes.yaml` and resolved-path paths being live.
8. Items 7, 9 (phase docs + placeholders.md doctrine) — last; doc consistency catches up to the now-true runtime story.

**Pre-execute check:** grep `plans/*.md` for any concurrent agent pickup markers on the files touched here (the BPMN plan, `paths_defaults.go`, `placeholders.md`, agent templates, the new yaml location) before adding this plan's marker, per `[[feedback_check_concurrent_agents]]`.

**Post-execute:** rerun `gh optivem sync` in any active scaffolded repo to refresh agent frontmatter from the new `phase-scopes.yaml`.
