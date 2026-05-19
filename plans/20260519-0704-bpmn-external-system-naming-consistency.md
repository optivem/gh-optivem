# Plan: BPMN external-system naming consistency

**Date:** 2026-05-19

**Filed from:** [SSoT phase-scope plan (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md) execution session — surfaced during item 1 authoring when it became visible that CT phase ids and Family B key vocabulary use mixed "external" vs "external system" terminology.

**Context.** Today's BPMN + Family B vocabulary uses both short ("external") and long ("external system") forms inconsistently:

| Surface | Current name | Short or long? |
|---|---|---|
| Phase id (SIR variant) | `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE` | long |
| Phase id (CT-RED variant) | `CT_RED_EXTERNAL_DRIVER` | short |
| Phase id (CT-GREEN variant) | `CT_GREEN_STUBS` | absent ("external system" not in name) |
| Gateway binding | `external_system_driver_interface_changed` | long |
| Family B path keys | `external_driver_port`, `external_driver_adapter` | short |
| Agent names | `ct-red-external-driver`, `ct-green-stubs` | short |
| User-facing labels | "External System Driver" (comments), "EXTERNAL DRIVER" (phase_label) | mixed |

This plan standardises to **"external system" (long form)** across all surfaces. Rationale: "external driver" alone is ambiguous (driver to what?); "external system driver" is self-documenting (driver to an external SYSTEM). The long form is already in use for SIR and the gateway binding, so adopting it everywhere is alignment, not invention.

## Locked decisions

- **Long form everywhere.** Phase ids, Family B keys, agent names, prompt filenames, doc filenames, labels.
- **`ct-green-stubs` becomes `ct-green-external-system-stub`** (singular). Symmetric with `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE` (singular "REDESIGN"). The "stubs" plural-vs-singular question goes away.
- **Config migration is a rename pass**, not a back-fill. Existing scaffolded `gh-optivem.yaml` files have `external_driver_port` etc. The migrate command must rename them to `external_system_driver_port` etc., preserving comments and any user-customised values.

## Items

### 1. Phase id + agent name + prompt filename renames (Level A)

In `internal/atdd/runtime/statemachine/process-flow.yaml`:

- `CT_RED_EXTERNAL_DRIVER` → `CT_RED_EXTERNAL_SYSTEM_DRIVER` (node id, sequence flows, phase_label, change_type, agent name)
- `CT_GREEN_STUBS` → `CT_GREEN_EXTERNAL_SYSTEM_STUB` (node id, sequence flows, agent name)
- agent `ct-red-external-driver` → `ct-red-external-system-driver`
- agent `ct-green-stubs` → `ct-green-external-system-stub`

Cascades to:

- Rename `internal/assets/runtime/prompts/atdd/ct-red-external-driver.md` → `ct-red-external-system-driver.md`; update internal references.
- Rename `internal/assets/runtime/prompts/atdd/ct-green-stubs.md` → `ct-green-external-system-stub.md`; update internal references.
- Rename `docs/atdd/process/ct-red-external-driver.md` → `ct-red-external-system-driver.md`; update content references (any docs/agents/templates that link to it).
- Rename `docs/atdd/process/ct-green-stubs.md` → `ct-green-external-system-stub.md`; update content references.
- Rename `docs/atdd/process/change/behavior/ct/1.3-ct-red-external-driver.md` → `1.3-ct-red-external-system-driver.md`.
- Rename `docs/atdd/process/change/behavior/ct/2-ct-green-stubs.md` → `2-ct-green-external-system-stub.md`.
- Update `internal/atdd/runtime/statemachine/behavioral_cycle_test.go` and `transitions_test.go` phase id refs.
- Update `internal/atdd/phase-scopes.yaml` (created by SSoT plan item 1) — the two CT_* keys.
- Update `internal/atdd/phase_scopes_test.go` (created by SSoT plan item 11) — allowlist & references.

### 2. Family B path-key renames (Level B)

In `internal/projectconfig/paths_defaults.go`:

- `external_driver_port` → `external_system_driver_port`
- `external_driver_adapter` → `external_system_driver_adapter`
- Update `canonicalPathKeys()` slice
- Update `pathStems()` per-language stems (the path component itself is unchanged — only the YAML key changes; e.g. typescript stem stays `src/testkit/external/port` but the key it's stored under becomes `external_system_driver_port`)
- Update `DefaultPaths` docstring

Cascades to:

- `internal/projectconfig/paths_defaults_test.go` — test fixtures.
- `internal/projectconfig/config_commands_test.go` — fixtures + new rename-migration tests.
- `internal/projectconfig/config.go` — anywhere the keys are referenced explicitly (validator hints, error messages).
- `internal/assets/global/docs/atdd/process/placeholders.md` (or `path-keys.md` if SSoT plan item 9 has landed by then) — key vocabulary section.
- `internal/atdd/runtime/architecture/architecture.yaml` and `docs/atdd/architecture/*.md` — any node ids or refs.
- `internal/atdd/phase-scopes.yaml` (created by SSoT plan item 1) — CT layer refs.

### 3. Extend `gh optivem config migrate` with a key-rename pass

`runConfigMigrate` today does back-fills (add missing canonical keys). Extend it with a **rename pass** for the two old keys:

- For each pair `{old, new}` in the rename map:
  - If `paths.<old>` exists and `paths.<new>` does not → rename the YAML node (preserving the value, the comment block, and the position in the map).
  - If both exist → hard error: "ambiguous migration state, both old and new key present. Manual resolution required."
  - If only `<new>` exists → no-op.
- Mark `changed = true` so the file is rewritten.

**Test cases:**

- Pre-rename config (old keys present, new absent) → migrate renames in place.
- Post-rename config (new keys present, old absent) → no-op.
- Both present → hard error.
- Comment preservation across rename.
- Custom user-edited value preserved across rename.
- Idempotent: running migrate twice produces no diff on the second run.

**Validator update:** `internal/projectconfig/config.go` — `paths.<old>` becomes an unknown key under the new vocabulary. The error message should detect the specific old → new rename pair and direct the user to `gh optivem config migrate` (same pattern as the SSoT plan's item 5 pre-SSoT detection).

### 4. Existing-project rollout

Academy workspace projects (e.g. shop template, eshop-*) already have `external_driver_port` etc. in their committed `gh-optivem.yaml`. They will fail validation after this plan lands until migrated. Two options:

- **(a) In-tree migration as part of this plan.** Run `gh optivem config migrate` against every project in the workspace as a post-merge step; commit the rewritten yamls.
- **(b) Hands-off.** Let each project migrate on next `gh optivem` invocation; the validator's redirect to `config migrate` is sufficient.

Decide at refinement. Probably (a) for the curated academy projects (low count), (b) for any downstream user projects (gh-optivem doesn't own them).

## Out of scope

- Renaming `driver_port` / `driver_adapter` (the AT-side Family B keys). Their context is the SUT's own driver layer; "system" prefix would be redundant (system_path already exists).
- Renaming AT-cycle phase ids (`AT_RED_SYSTEM_DRIVER` etc.). The "SYSTEM" in those refers to the SUT, not the external system — different concept; the existing name is correct.
- Renaming the BPMN gateway binding `external_system_driver_interface_changed` — already long form.
- Renaming the SIR phase ids — already long form.

## Pre-requisites

- SSoT phase-scope plan ([20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)) items 1, 2, 11 must have landed (this plan rewrites entries in `phase-scopes.yaml` and the test).
- CT-vocabulary plan ([20260518-1742](20260518-1742-family-b-stems-and-ct-vocab.md)) should have landed so `ct_test` and the `DefaultPaths(testLang, systemTestRoot, sutNamespace)` signature are stable. Otherwise this rename will conflict with that plan's edits to the same `paths_defaults.go` file.

## Hand-off

**Pre-execute check:** grep `plans/*.md` for active pickup markers on `paths_defaults.go`, `config_commands.go`, `process-flow.yaml`, and the CT prompt files — coordinate with any concurrent agents before adding this plan's marker.

**Execute order suggestion (refine at /refine-plan):**

1. Item 2 — Family B key rename in `paths_defaults.go` + tests.
2. Item 3 — extend `config migrate` with the rename pass + tests.
3. Item 1 — phase id + agent + prompt + doc renames (bigger surface, faster once 2+3 settle the foundations).
4. Item 4 — academy workspace migration (decide at refinement).

**Post-execute:** rerun `gh optivem sync` in active scaffolded repos; verify the renamed paths_defaults pick up via the migrate path.
