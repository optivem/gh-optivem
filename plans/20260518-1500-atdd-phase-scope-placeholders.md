# Plan: ATDD phase-scope placeholders consolidation

**Date:** 2026-05-18

**Context:** Phase scope is declared in three places today, in two different shapes:

1. `docs/atdd/process/change/behavior/1.1-at-red-test.md` §Scope — **prose**: `acceptance test files; DSL prototype stubs (interface + "TODO: DSL" throw)`.
2. `docs/atdd/process/change/behavior/1.2-at-red-dsl.md` §Scope — **prose**: `DSL Core impls; driver-port interface declarations`.
3. `docs/atdd/process/change/behavior/1.3-at-red-system-driver.md` §Scope — **`${...}` placeholders**: `` `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/` ``.

The BPMN orchestration plan ([20260518-1144](20260518-1144-atdd-bpmn-orchestration.md)) Snapshot A mirrors the same per-phase strings row-for-row (lines 37–43) and item 5 specifies that `check_phase_scope` resolves `${...}` via `Config.PlaceholderMap()`. Because RED-TEST and RED-DSL use prose with no placeholders, the runtime check has nothing to resolve and nothing to enforce on those phases. Two artefacts (the phase doc and the plan Snapshot) and one runtime action (`check_phase_scope`) all need to agree on what the scope is — and today they agree only by string-matching prose, which is unenforceable.

This plan adds three Family B keys (`at_test`, `dsl_port`, `dsl_core`) to the canonical key set + per-language defaults + the `placeholders.md` example block. That's the substrate. The full SSoT consolidation (single doctrinal source for per-phase assignment, agent-frontmatter projection, scope.md rule doc, fully-resolved-paths migration, validation) is owned by the [SSoT successor plan](20260518-1530-atdd-phase-scope-ssot.md) and depends on these substrate keys existing first.

**Scope after refinement (2026-05-18):** items 1–3 + item 7a (BPMN sibling-plans cross-reference). Items 4–9 dropped (see scope-trim Refined annotation below for full rationale).

**Sibling plans referenced:**
- [SSoT phase-scope architecture (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md) — **successor**. Owns the per-phase scope assignment doctrine (`internal/atdd/phase-scopes.yaml`), the shared rule doc (`docs/atdd/process/shared/scope.md`), the scaffolder projection to agent frontmatter, the resolved-paths migration (no runtime substitution), validation rules, and the phase-doc §Scope rewrites that items 4–6 originally proposed. Depends on this plan's items 1–3 (Family B substrate vocabulary) landing first.
- [BPMN orchestration plan (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md) — consumer. Item 7a of **this** plan adds a sibling-plans cross-reference to it. Snapshot A's per-phase table is retired by the SSoT successor (not by this plan).

**Naming decision (refined — 2026-05-18, user):** Family B keys use hexagonal `port` / `adapter` / `core` vocabulary across all testkit roles, matching the existing four keys (`driver_port` / `driver_adapter` / `external_driver_port` / `external_driver_adapter`) and the architecture docs (`dsl-port.md`, `dsl-core.md`). Singular test-file keys use plain naming. The canonical keys are:

- `at_test` *(acceptance test files — singular, not hexagonal)*
- `dsl_port`, `dsl_core` *(`dsl_port` replaces the prior proposal `dsl_interface`)*
- `driver_port`, `driver_adapter` *(existing, no change)*
- `external_driver_port`, `external_driver_adapter` *(existing, no change)*

CT-cycle keys (`ct_test` and any external-driver-specific CT roles) remain **deferred to a sibling plan** per item 8 — they live in less-mature `docs/atdd/process/change/behavior/ct/` phase docs.

**Reasons (for hexagonal):** (1) zero existing-key churn — the four existing Family B keys stay as-is; (2) works clean in Java, C#, TypeScript with no reserved-word collisions (Java's `interface` keyword forced a divergence between config key name and on-disk folder name in the alternative); (3) matches the architecture docs' vocabulary (`dsl-port.md`, `dsl-core.md`), so the same word denotes the same thing in both surfaces; (4) avoids a breaking config-rename migration step.

**Trade-off accepted:** DSL is `port`+`core` while drivers are `port`+`adapter` — semantically meaningful asymmetry (DSL is domain logic that composes drivers; drivers are external bridges) but readers unfamiliar with hexagonal vocabulary need a one-line orientation in the doctrine.

**Alternatives considered and rejected (2026-05-18, user):**
- **Plain `interface`/`implementation` (with rename of existing four keys to `system_driver_interface` etc.)** — initially chosen, then reversed: `interface` is a Java reserved word, so the config-key vocabulary and the on-disk Java folder name would diverge on one of three target languages. That defeats the consolidation's purpose.
- **Plain `api`/`impl` (Java-safe variant)** — symmetric, no reserved-word issue, but `impl` is jargony, `api` is overloaded, and the rename of existing keys still costs a migration step for zero readability gain over hexagonal.
- **Original mixed proposal (`at_test`, `dsl_interface`, `dsl_core`)** — inconsistent within DSL itself (`interface` plain + `core` hexagonal).

> **Refined 2026-05-18:** Naming decision rewritten end-to-end. **Why:** the user surfaced the complete eight-role listing (acceptance_test / contract_test / dsl / system_driver / external_system_driver) and walked through interface/implementation vs port/core/adapter vs api/impl. Java's `interface` reserved word ruled out the symmetric-plain option. (B) — hexagonal everywhere — won on zero-churn + cross-language cleanliness + architecture-doc coherence. The `dsl_interface` → `dsl_port` rename is the only key-name change vs. the original plan; everything else stays.
>
> **Refined 2026-05-18 (scope trim):** Items 4–9 removed from this plan. **Why:** during refinement the user surfaced that per-phase scope assignment is duplicated across phase doc §Scope sections (items 4–6), BPMN Snapshot A (item 7b), and the BPMN `check_phase_scope` runtime — and that the right consolidation is a single doctrinal source of truth (`internal/atdd/phase-scopes.yaml` in tool source), projected to each agent's frontmatter by the scaffolder, with a shared `docs/atdd/process/shared/scope.md` defining the *rule*. Three further locked decisions during refinement: **(δ)** `gh-optivem.yaml paths:` values are fully-resolved concrete paths (sut_namespace baked in, no runtime substitution; `system.sut_namespace` is dropped from config); **(γ)** Family A path-shaped keys participate in scope identically to Family B; **(ε)** migration from pre-SSoT configs is a hard error with deterministic suggested values, human edits manually (no AI in the loop); plus a validation policy decision: unknown / obsolete fields are hard errors, not warnings. Under that architecture, items 4–6 (phase-doc §Scope bullet rewrites with `${name}/${sut_namespace}/`) and item 7b (Snapshot A rewrite) are wrong-shaped — phase docs reference layer NAMES only, no substitution, no per-phase tables in plan files. Items 1–3 (Family B substrate vocabulary additions) remain — they're the prerequisite the successor SSoT plan depends on. Item 7a (BPMN sibling-plans cross-reference) remains. Items 8 (CT keys) and 9 (GREEN placeholder) are absorbed by the successor — both become entries in `phase-scopes.yaml`. See sibling-plans entry below for the successor.

## Items

### 1. Add three Family B keys to the canonical list

Edit `internal/projectconfig/paths_defaults.go:canonicalPathKeys()` (line 44) to extend the canonical Family B key set from four to seven entries:

```go
return []string{
    "driver_port",
    "driver_adapter",
    "external_driver_port",
    "external_driver_adapter",
    "at_test",    // NEW — acceptance test files
    "dsl_port",   // NEW — DSL port (fluent interface)
    "dsl_core",   // NEW — DSL Core implementation
}
```

The canonical-key contract (per the existing docstring): order is fixed so `DefaultPaths`, the migrate back-fill, and tests over either iterate in the same order. New keys go at the **end** of the slice — appending preserves the ordering invariant for any code that indexes by position.

The migrate path (which back-fills only canonical keys that are absent) will pick up the new keys on the next `gh optivem` run against an existing project — no opt-in needed. User overrides are preserved per the existing back-fill rule.

**Also update the `DefaultPaths` docstring at lines 5–24** — the prose "The four keys match the doctrine ..." and "canonical four set by the user" must change to "The seven keys ..." / "canonical seven set by the user", and the comma-separated key list must include the three new keys. The per-language path-stem bullets in the docstring (lines 17–19) are extended in item 2, not here.

> **Refined 2026-05-18:** Renamed `dsl_interface` → `dsl_port` in the new-key list per the (B) hexagonal naming decision (see preamble). Added explicit docstring-update note (lines 5–24). **Why:** docstring drift caused the original item to mention "four → seven" only in the surrounding prose, not in the change itself; making it a sub-step prevents the docstring being left at "four" after execution.

### 2. Pin per-language path defaults

Edit `internal/projectconfig/paths_defaults.go:pathStems()` (line 57) to extend each language branch from four stems to seven, in the same canonical order. The values must mirror the post-scaffold tree (the `system-test/{lang}/` subdir is flattened by `copySystemTests`, per the existing docstring).

**Open sub-question — actual scaffold layout.** The repo currently has no system-test scaffold templates under `internal/assets/` (verified — no `dsl/`, `testkit/`, or per-language test trees there), so the canonical post-scaffold layout for DSL interface / DSL Core / acceptance test files is **not** pinned by an existing template. Three options, decided per language as part of executing this item:

(a) **Pin against the existing `shop` template repo** if it carries a settled layout. If a `shop` repo exists with `src/testkit/dsl/port`, `src/testkit/dsl/core`, `src/test/...` (or analogue), use that as the source of truth.

(b) **Pin against the architecture docs** if they nominate concrete paths. `dsl-port.md`, `dsl-core.md`, `test.md` describe **shape** (fluent interface, naming) but not **location** at the time of writing — execute-time check whether that has changed.

(c) **Defer to a follow-up plan** with an explicit `TODO: layout` marker in the default map, if (a) and (b) both come up empty. Better to ship the placeholder vocabulary with stubs than to invent layout doctrine here.

Tentative values pending the per-language check:

| Language | `at_test` | `dsl_port` | `dsl_core` |
|---|---|---|---|
| TypeScript | `src/test` | `src/testkit/dsl/port` | `src/testkit/dsl/core` |
| Java | `src/test/java` | `src/main/java/testkit/dsl/port` | `src/main/java/testkit/dsl/core` |
| .NET | `Tests` | `Testkit.Dsl.Port` | `Testkit.Dsl.Core` |

These are derived by analogy from the existing `driver_port` / `driver_adapter` stems (lines 60–82) and are **proposals**, not pinned doctrine. Execute-time verification per (a)/(b)/(c) above is mandatory.

> **Refined 2026-05-18:** Column header `dsl_interface` → `dsl_port` per the (B) hexagonal naming decision (see preamble). Directory names unchanged (they already used `dsl/port`). **Why:** mechanical propagation of the vocabulary rename — the original plan's `dsl_interface` config key had already mapped to a `dsl/port` folder, so the rename actually *removes* a key-vs-folder mismatch.
>
> **Refined 2026-05-18:** Open sub-question (a/b/c) kept as execute-time verification, not pre-decided. **Why:** user prefers verifying the `shop` template's actual layout at execute time rather than blocking refinement on it. Execute-plan must run (a) → (b) → (c) in order per-language and surface any (c) fallbacks before completing the item.
>
> **Follow-up idea (2026-05-18, not in scope here):** model concrete reference layouts in both the `shop` template repo and in `gh-optivem`'s own asset/example trees, so future scaffolds have a working canonical layout to pin against rather than relying on execute-time analogy from `driver_port`/`driver_adapter`. Park as a separate plan once this consolidation lands — surface to the user before opening it.

### 3. Update `placeholders.md` Family B example block + reserved-name guard

Edit `internal/assets/global/docs/atdd/process/placeholders.md` (line 27) — the example `paths:` block under "Family B — named locations" — to include the three new keys, in canonical order:

```yaml
paths:
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
  external_driver_port: system-test/typescript/src/testkit/external/port
  external_driver_adapter: system-test/typescript/src/testkit/external/adapter
  at_test: system-test/typescript/src/test
  dsl_port: system-test/typescript/src/testkit/dsl/port
  dsl_core: system-test/typescript/src/testkit/dsl/core
```

**These TypeScript values are pinned canonical.** The example block in `placeholders.md` is the authoritative TypeScript reference layout. Item 2's `pathStems()` TypeScript branch must mirror these exactly; Java and .NET still go through item 2's (a)/(b)/(c) execute-time verification.

No changes needed to the doctrinal text — the "user-owned ... every key under the top-level `paths:` block becomes a `${key}` placeholder" rule covers the new keys automatically. The "Family B keys cannot shadow Family A names" sentence likewise needs no edit (none of the three new keys collide with `language` / `architecture` / `system_path` / `system_test_path` / `sut_namespace`).

> **Refined 2026-05-18:** (1) `dsl_interface` → `dsl_port` per the (B) hexagonal naming decision. (2) Pinned the three new TypeScript stems as canonical reference, removed the "(Final stems pending item 2's per-language check.)" caveat. **Why:** user wants concrete values in the plan rather than tentative-pending-verification. The placeholders.md example block is the natural place to pin a reference layout (it shows one language by convention); item 2 still verifies Java + .NET against shop / arch docs / TODO fallback.

### ~~4. Bullet-ify `1.1-at-red-test.md` §Scope~~ — REMOVED

> **Refined 2026-05-18 (scope trim):** Removed. **Why:** under the SSoT architecture, phase docs reference layer NAMES only (no `${name}/${sut_namespace}/` substitution); the rule lives in `docs/atdd/process/shared/scope.md`; per-agent resolved paths live in agent frontmatter. The bullet-rewrite this item proposed is wrong-shaped under that architecture. Absorbed by the [SSoT successor plan](20260518-1530-atdd-phase-scope-ssot.md).

### ~~5. Bullet-ify `1.2-at-red-dsl.md` §Scope~~ — REMOVED

> **Refined 2026-05-18 (scope trim):** Removed for the same reason as item 4. Absorbed by the [SSoT successor plan](20260518-1530-atdd-phase-scope-ssot.md).

### ~~6. Verify `1.3-at-red-system-driver.md` §Scope~~ — REMOVED

> **Refined 2026-05-18 (scope trim):** Removed for the same reason as item 4. Note: 1.3's §Scope was the *only* phase doc already using `${...}/${sut_namespace}/` placeholders; under the SSoT architecture it loses them too (bare key names only). Absorbed by the [SSoT successor plan](20260518-1530-atdd-phase-scope-ssot.md).

### 7. Add cross-reference to BPMN plan

Edit `plans/20260518-1144-atdd-bpmn-orchestration.md` header — **Sibling plans referenced** (around line 8–11) — adding an entry pointing back to this plan:

```diff
 **Sibling plans referenced:**
 - [Part 1 — AT-cycle architecture & §Conventions](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) — ...
 - [Part 2 — `atdd-at-cycle.md` per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — ...
 - [Legacy coverage cycle](20260518-1116-legacy-coverage-cycle.md) — ...
+- [Phase-scope placeholders substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — defines the `at_test`, `dsl_port`, `dsl_core` Family B keys. Substrate prerequisite for the SSoT successor plan (which retires Snapshot A and rewires `check_phase_scope`).
```

> **Refined 2026-05-18 (scope trim):** Item collapsed from "Two edits" to "One edit". **Why:** part (b) — Snapshot A table rewrite to use `${...}/${sut_namespace}/` bullets — is wrong-shaped under the SSoT architecture. The SSoT successor plan retires Snapshot A entirely (canonical source moves to `internal/atdd/phase-scopes.yaml`; the BPMN plan cites it instead of duplicating it). Only part (a) — the sibling-plans cross-reference — survives here, and its description is updated to point at the substrate role rather than the now-retired Snapshot A consumption story.

### ~~8. (Open question) CT-cycle parallel keys~~ — REMOVED

> **Refined 2026-05-18 (scope trim):** Removed. **Why:** under the SSoT architecture, CT-cycle phases become entries in `internal/atdd/phase-scopes.yaml` like any other phase, referencing whatever layer keys they need. The dedicated "CT-cycle plan" framing is unnecessary — CT is just more rows. Absorbed by the [SSoT successor plan](20260518-1530-atdd-phase-scope-ssot.md).

### ~~9. (Open question) GREEN phase placeholder~~ — REMOVED

> **Refined 2026-05-18 (scope trim):** Removed. **Why:** GREEN's scope (`system_path` after the fully-resolved-paths migration locked in **(γ)**) is just another entry in `phase-scopes.yaml`. No special handling needed once Family A / Family B both produce resolved paths. Absorbed by the [SSoT successor plan](20260518-1530-atdd-phase-scope-ssot.md).

## Out of scope

- **SSoT phase-scope architecture** — all of: `internal/atdd/phase-scopes.yaml`, `docs/atdd/process/shared/scope.md`, scaffolder projection to agent frontmatter, fully-resolved-paths migration, sut_namespace removal from config, Snapshot A retirement, `check_phase_scope` rewiring, validation rules for obsolete/unknown fields. Owned by the [SSoT successor plan (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md), which depends on this plan's items 1–3.
- **Phase-doc §Scope rewrites** — phase docs (1.1, 1.2, 1.3, and beyond) gain bare-key-name references and drop their `${name}/${sut_namespace}/` syntax under the successor plan. This plan does NOT touch them.
- **Renaming the existing four Family B keys** — `driver_port` / `driver_adapter` / `external_driver_port` / `external_driver_adapter` stay as-is per the locked hexagonal naming decision.
- **CT-cycle keys, GREEN phase placeholder** — absorbed by the successor as `phase-scopes.yaml` entries (no per-phase plan items needed).
- **Editing the asset-template phase docs** under `internal/assets/global/docs/atdd/process/at-red-*.md` — the source-of-truth for phase docs is the `docs/atdd/process/change/behavior/` tree per recent commits (`6cc16c6`, `82bb983`, `fe3bd49`). If the assets need parallel edits, that's a separate consolidation.
- **Substitution at sync time for `docs/atdd/process/`** — per current `placeholders.md`, substitution walks the `internal/assets/global/docs/atdd/` tree. The successor retires runtime substitution entirely (paths become fully-resolved at scaffold time only); this plan doesn't preempt that.
- **BPMN plan item rewrites** — only the header sibling-plans entry is touched; items 5 / 8 / 9 of the BPMN plan are not re-opened here.

## Hand-off

Execute order: items **1, 2, 3** as one atomic change (they all touch the canonical-key contract); then item **7** (single-line cross-reference edit to the BPMN plan).

- Item 2's per-language defaults are an **execute-time verification** task — the proposed values in the table above are starting points, not pinned answers. TypeScript is pinned by item 3's `placeholders.md` example. For Java and .NET, run (a) → (b) → (c) in order; if all come up empty for a language, leave that language's three new stems as a `TODO: layout` marker and surface it back to the user before completing the item.
- After this plan lands, the SSoT successor ([20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)) is unblocked and can proceed.

**Pre-execute check:** grep `plans/*.md` for any concurrent agent pickup markers on `paths_defaults.go` / `placeholders.md` / `plans/20260518-1144-atdd-bpmn-orchestration.md` before adding this plan's marker, per `[[feedback_check_concurrent_agents]]`.
