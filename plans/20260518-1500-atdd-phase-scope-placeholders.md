# Plan: ATDD phase-scope placeholders consolidation

**Date:** 2026-05-18

**Context:** Phase scope is declared in three places today, in two different shapes:

1. `docs/atdd/process/change/behavior/1.1-at-red-test.md` §Scope — **prose**: `acceptance test files; DSL prototype stubs (interface + "TODO: DSL" throw)`.
2. `docs/atdd/process/change/behavior/1.2-at-red-dsl.md` §Scope — **prose**: `DSL Core impls; driver-port interface declarations`.
3. `docs/atdd/process/change/behavior/1.3-at-red-system-driver.md` §Scope — **`${...}` placeholders**: `` `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/` ``.

The BPMN orchestration plan ([20260518-1144](20260518-1144-atdd-bpmn-orchestration.md)) Snapshot A mirrors the same per-phase strings row-for-row (lines 37–43) and item 5 specifies that `check_phase_scope` resolves `${...}` via `Config.PlaceholderMap()`. Because RED-TEST and RED-DSL use prose with no placeholders, the runtime check has nothing to resolve and nothing to enforce on those phases. Two artefacts (the phase doc and the plan Snapshot) and one runtime action (`check_phase_scope`) all need to agree on what the scope is — and today they agree only by string-matching prose, which is unenforceable.

This plan consolidates the per-phase scope vocabulary onto the existing `${name}` Family B placeholder mechanism so the same map in `gh-optivem.yaml` drives both human-readable phase docs and the BPMN scope-check action. No new substitution mechanism is introduced — just three new keys under the existing `paths:` block.

**Sibling plans referenced:**
- [BPMN orchestration plan (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md) — consumer. Snapshot A rows for RED-TEST / RED-DSL change from prose to `${...}` bullets after this plan lands; item 5's `check_phase_scope` action then has resolvable placeholders for all RED phases. Two small touches in the BPMN plan (header sibling-plans entry + Snapshot A rewrite) are owned by **this** plan's item 7, not by re-opening the BPMN plan.
- [BPMN plan §Conventions snapshot (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md#%C2%A7conventions-snapshot-inlined-to-survive-the-upcoming-split) — the canonical Snapshot A this plan reshapes.

**Naming decision (already taken — 2026-05-18, user):** new Family B keys use short style with no `_path` suffix, matching the existing `driver_port` / `driver_adapter` style: **`at_test`**, **`dsl_interface`**, **`dsl_core`**. Alternatives (`acceptance_test`, `_path`-suffixed) considered and rejected.

## Items

### 1. Add three Family B keys to the canonical list

Edit `internal/projectconfig/paths_defaults.go:canonicalPathKeys()` (line 44) to extend the canonical Family B key set from four to seven entries:

```go
return []string{
    "driver_port",
    "driver_adapter",
    "external_driver_port",
    "external_driver_adapter",
    "at_test",         // NEW — acceptance test files
    "dsl_interface",   // NEW — DSL interface declarations
    "dsl_core",        // NEW — DSL Core implementation files
}
```

The canonical-key contract (per the existing docstring): order is fixed so `DefaultPaths`, the migrate back-fill, and tests over either iterate in the same order. New keys go at the **end** of the slice — appending preserves the ordering invariant for any code that indexes by position.

The migrate path (which back-fills only canonical keys that are absent) will pick up the new keys on the next `gh optivem` run against an existing project — no opt-in needed. User overrides are preserved per the existing back-fill rule.

### 2. Pin per-language path defaults

Edit `internal/projectconfig/paths_defaults.go:pathStems()` (line 57) to extend each language branch from four stems to seven, in the same canonical order. The values must mirror the post-scaffold tree (the `system-test/{lang}/` subdir is flattened by `copySystemTests`, per the existing docstring).

**Open sub-question — actual scaffold layout.** The repo currently has no system-test scaffold templates under `internal/assets/` (verified — no `dsl/`, `testkit/`, or per-language test trees there), so the canonical post-scaffold layout for DSL interface / DSL Core / acceptance test files is **not** pinned by an existing template. Three options, decided per language as part of executing this item:

(a) **Pin against the existing `shop` template repo** if it carries a settled layout. If a `shop` repo exists with `src/testkit/dsl/port`, `src/testkit/dsl/core`, `src/test/...` (or analogue), use that as the source of truth.

(b) **Pin against the architecture docs** if they nominate concrete paths. `dsl-port.md`, `dsl-core.md`, `test.md` describe **shape** (fluent interface, naming) but not **location** at the time of writing — execute-time check whether that has changed.

(c) **Defer to a follow-up plan** with an explicit `TODO: layout` marker in the default map, if (a) and (b) both come up empty. Better to ship the placeholder vocabulary with stubs than to invent layout doctrine here.

Tentative values pending the per-language check:

| Language | `at_test` | `dsl_interface` | `dsl_core` |
|---|---|---|---|
| TypeScript | `src/test` | `src/testkit/dsl/port` | `src/testkit/dsl/core` |
| Java | `src/test/java` | `src/main/java/testkit/dsl/port` | `src/main/java/testkit/dsl/core` |
| .NET | `Tests` | `Testkit.Dsl.Port` | `Testkit.Dsl.Core` |

These are derived by analogy from the existing `driver_port` / `driver_adapter` stems (lines 60–82) and are **proposals**, not pinned doctrine. Execute-time verification per (a)/(b)/(c) above is mandatory.

### 3. Update `placeholders.md` Family B example block + reserved-name guard

Edit `internal/assets/global/docs/atdd/process/placeholders.md` (line 27) — the example `paths:` block under "Family B — named locations" — to include the three new keys, in canonical order:

```yaml
paths:
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
  external_driver_port: system-test/typescript/src/testkit/external/port
  external_driver_adapter: system-test/typescript/src/testkit/external/adapter
  at_test: system-test/typescript/src/test
  dsl_interface: system-test/typescript/src/testkit/dsl/port
  dsl_core: system-test/typescript/src/testkit/dsl/core
```

(Final stems pending item 2's per-language check.)

No changes needed to the doctrinal text — the "user-owned ... every key under the top-level `paths:` block becomes a `${key}` placeholder" rule covers the new keys automatically. The "Family B keys cannot shadow Family A names" sentence likewise needs no edit (none of the three new keys collide with `language` / `architecture` / `system_path` / `system_test_path` / `sut_namespace`).

### 4. Bullet-ify `1.1-at-red-test.md` §Scope

Edit `docs/atdd/process/change/behavior/1.1-at-red-test.md` §Scope (line 7) — replace the semicolon-separated prose with a `${...}` bullet list. Concrete change:

```diff
 ## Scope

-acceptance test files; DSL prototype stubs (interface + `"TODO: DSL"` throw)
+- `${at_test}/${sut_namespace}/` — acceptance test files
+- `${dsl_interface}/${sut_namespace}/` — DSL interface declarations (new methods)
+- `${dsl_core}/${sut_namespace}/` — `"TODO: DSL"` prototype impls
```

Same `${...}/${sut_namespace}/` shape as 1.3 today. The three bullets cover the same surface as the original prose — acceptance test files + interface additions + prototype impls — but each becomes a distinct, substitutable path.

### 5. Bullet-ify `1.2-at-red-dsl.md` §Scope

Edit `docs/atdd/process/change/behavior/1.2-at-red-dsl.md` §Scope (line 7) — same shape as item 4:

```diff
 ## Scope

-DSL Core impls; driver-port interface declarations
+- `${dsl_core}/${sut_namespace}/` — DSL Core impls (replacing `"TODO: DSL"` prototypes)
+- `${driver_port}/${sut_namespace}/` — Driver port interface declarations (new methods)
```

### 6. Verify `1.3-at-red-system-driver.md` §Scope (no-op expected)

Read `docs/atdd/process/change/behavior/1.3-at-red-system-driver.md` §Scope (line 7) — already conforms (uses `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/`). The semicolon→bullet rewrite still applies for consistency with items 4 and 5:

```diff
 ## Scope

-`${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/`
+- `${driver_port}/${sut_namespace}/`
+- `${driver_adapter}/${sut_namespace}/`
```

### 7. Update BPMN plan Snapshot A + add cross-reference

Two edits in `plans/20260518-1144-atdd-bpmn-orchestration.md`:

**(a) Header — Sibling plans referenced** (around line 8–11): add an entry pointing back to this plan as a typed dependency for items 5 and 8:

```diff
 **Sibling plans referenced:**
 - [Part 1 — AT-cycle architecture & §Conventions](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) — ...
 - [Part 2 — `atdd-at-cycle.md` per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — ...
 - [Legacy coverage cycle](20260518-1116-legacy-coverage-cycle.md) — ...
+- [Phase-scope placeholders consolidation (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — defines `${at_test}`, `${dsl_interface}`, `${dsl_core}` Family B keys consumed by Snapshot A (RED-TEST, RED-DSL) and by item 5's `check_phase_scope` action. Land that plan before item 5 / 8 / 9 Part A execute.
```

**(b) §Conventions snapshot — Snapshot A — table** (lines 37–43): rewrite RED-TEST and RED-DSL rows from prose to `${...}` bullet lists:

```diff
 | Phase | Allowed paths |
 |---|---|
-| RED-TEST | acceptance test files; DSL prototype stubs (interface + `"TODO: DSL"` throw) |
-| RED-DSL | DSL Core impls; driver-port interface declarations |
+| RED-TEST | `${at_test}/${sut_namespace}/`, `${dsl_interface}/${sut_namespace}/`, `${dsl_core}/${sut_namespace}/` |
+| RED-DSL | `${dsl_core}/${sut_namespace}/`, `${driver_port}/${sut_namespace}/` |
 | RED-SYSTEM-DRIVER | `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/` |
 | GREEN | production system code only; tests/DSL/drivers are frozen |
 | CT-RED-TEST / CT-RED-DSL / CT-RED-EXTERNAL-DRIVER / CT-GREEN-STUBS | `external/**` only |
```

Snapshot A bullet style stays inline (comma-separated) to fit the table cell — the table form already separates phases by row, so per-path bullets inside a cell would over-indent. The phase docs (items 4–6) use proper bullets because they have full sections to themselves. (This is the one place the bullets-over-semicolons rule yields to layout.)

**Out of scope here:** GREEN and CT rows of Snapshot A — see items 8 and 9 (Open questions) below.

### 8. (Open question) CT-cycle parallel keys

The CT row of Snapshot A reads `external/**` — a wildcard, not a placeholder. The CT phases (CT-RED-TEST / CT-RED-DSL / CT-RED-EXTERNAL-DRIVER / CT-GREEN-STUBS) have the same prose-vs-placeholder problem this plan fixes for the AT cycle.

Two sub-options:

(a) **Add parallel Family B keys** — `external_test`, `external_dsl_interface`, `external_dsl_core` — mirroring `at_test` / `dsl_interface` / `dsl_core`. This makes CT phase docs (when they exist) bullet-list `${external_test}/${sut_namespace}/` etc., and lets `check_phase_scope` enforce CT scope the same way it enforces AT scope.

(b) **Defer to a CT-specific follow-up plan.** This plan ships AT-cycle consolidation only; CT consolidation lives in a sibling plan with its own per-language path-default analysis (CT may not flatten the same way the AT testkit does).

**Recommendation:** defer (option b). Reasons:

- CT phase docs in `docs/atdd/process/change/behavior/ct/` are not yet at the maturity of 1.1–1.3 (they're earlier in the doc tree's evolution) — adding placeholders to a moving target risks rework.
- The wildcard `external/**` in Snapshot A is **good enough for now** because `check_phase_scope` (per BPMN plan item 5) reads it as a literal path-prefix match against `git diff --name-only` output. The runtime doesn't need `${external_*}` to enforce CT scope — it just needs paths starting with `external/`.
- Splitting the work keeps blast radius small and lets the AT-cycle consolidation land cleanly without a CT discovery exercise on the critical path.

If the user prefers (a), item 8 expands into three new canonical keys + three default-path checks (parallel to items 1–3) — same shape, doubles the LOC.

### 9. (Open question) GREEN phase placeholder

Snapshot A's GREEN row reads `production system code only; tests/DSL/drivers are frozen` — also prose, also unenforceable.

GREEN's allowed scope is roughly `${system_path}/${sut_namespace}/` (Family A `system_path` already exists). One bullet:

```
GREEN | `${system_path}/${sut_namespace}/`
```

**Recommendation:** include in this plan as a final small Snapshot A edit (item 7 (b) addendum). Reasons:

- No new keys needed — both `${system_path}` and `${sut_namespace}` are Family A and already in `Config.PlaceholderMap()`.
- Closes the AT-cycle scope-row consolidation completely: RED-TEST / RED-DSL / RED-SYSTEM-DRIVER / GREEN all become `${...}`-driven; only CT (item 8) remains as prose pending its own plan.

If the user prefers to defer GREEN as well, drop the addendum and leave the GREEN row as prose.

## Out of scope

- **CT-cycle Family B keys** — see item 8; deferred to a sibling plan by default.
- **Renaming the existing four Family B keys** — `driver_port` / `driver_adapter` / `external_driver_port` / `external_driver_adapter` already match the short-no-suffix style; no rename.
- **Editing the asset-template phase docs** under `internal/assets/global/docs/atdd/process/at-red-*.md` — the source-of-truth for phase docs is the `docs/atdd/process/change/behavior/` tree per recent commits (`6cc16c6`, `82bb983`, `fe3bd49`). If the assets need parallel edits, that's a separate consolidation.
- **Substitution at sync time for `docs/atdd/process/`** — per `placeholders.md`, substitution walks the `internal/assets/global/docs/atdd/` tree. Whether `docs/atdd/process/` is *also* subject to substitution is a separate question; this plan assumes readers interpret `${...}` mentally in the source-spec tree (same as 1.3 today).
- **BPMN plan item rewrites** — only Snapshot A rows and the header sibling-plans entry change; items 5 / 8 / 9 are not re-opened.

## Hand-off

Execute order: items **1, 2, 3** (config + scaffolder + doctrine) first as one atomic change (they all touch the contract); then items **4, 5, 6** (phase docs) in parallel; then item **7** (BPMN plan Snapshot A + cross-reference).

- Item 2's per-language defaults are an **execute-time verification** task — the proposed values in the table above are starting points, not pinned answers. If the verification comes up empty for any language, leave that language's three new stems as a `TODO: layout` marker and surface it back to the user before completing the item.
- Items 8 and 9 are open questions — confirm scope (defer item 8 / include item 9 addendum, both per recommendations above) before drafting any item-9-specific changes.

**Pre-execute check:** grep `plans/*.md` for any concurrent agent pickup markers on `paths_defaults.go` / `placeholders.md` / the three phase docs before adding this plan's marker, per `[[feedback_check_concurrent_agents]]`.
