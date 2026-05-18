# Plan: Family B stem correction + CT vocabulary additions

> ✅ **REFINED** — walked 2026-05-18 with `/refine-plan`. Scope expanded materially during refinement: from "add CT vocab" → "correct predecessor's `at_test` stems deterministically against shop-`latest`, thread `sutNamespace` through `pathStems()`/`DefaultPaths()`, and add `ct_test` as sibling of corrected `at_test`". File renamed during refine: was `20260518-1742-ct-family-b-vocabulary.md`. Ready for `/execute-plan`.

**Date:** 2026-05-18

**Filed from:** [SSoT phase-scope plan (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md), item 1's `ct_test`-key-placement refinement. **Scope expanded during 2026-05-18 refine walk** — see Context.

**Context.** Originally filed as a CT-side vocabulary plan to add `ct_test` so [SSoT plan 20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)'s `phase-scopes.yaml` could reference it cleanly. During refinement, comparison against the shop template tree revealed that the **predecessor plan's `at_test` stems** (committed in `8322c38`: TS `src/test`, Java `src/test/java`, dotnet `Tests`) do not match where AT tests actually live in shop — they were guessed by analogy. Under the user's [[feedback_paths_deterministic_no_guessing]] rule, that guess-and-ship pattern is unacceptable. Adding `ct_test` correctly while leaving wrong `at_test` next to it would leave a cross-key inconsistency and ship deceptive defaults to new scaffolds.

This plan therefore grew to own both halves atomically:

1. **Correct** the predecessor's `at_test` stems against shop-`latest` reality (item 3b).
2. **Thread** `sutNamespace` through `pathStems()` / `DefaultPaths()` signatures (item 3a) — required because Java's stems include a `<sutNamespace>` segment.
3. **Add** `ct_test` to `canonicalPathKeys()` + `pathStems()` as a sibling of corrected `at_test` (items 2a, 3c).
4. **Update** the `DefaultPaths` docstring and `placeholders.md` example block to match (items 2b, 4).

All code changes land as a single atomic edit to `internal/projectconfig/paths_defaults.go` + one edit to `placeholders.md`. The predecessor's `at_test` is corrected, not removed; the predecessor's contract (hexagonal vocabulary, append-only key set, back-fill on migrate) is preserved.

Per [[feedback_new_plan_not_extend]]: the predecessor plan (20260518-1500) is **not** edited in place; this fresh plan carries the corrective work. A deferred follow-up (Hand-off) records the cross-link that should be added back to predecessor and SSoT plans to surface this corrective relationship.

**Sibling plans referenced:**
- [Predecessor — AT-side Family B vocabulary substrate (20260518-1500)](20260518-1500-atdd-phase-scope-placeholders.md) — defined `at_test`, `dsl_port`, `dsl_core` and committed initial (wrong) `at_test` stems. This plan **corrects** the stems and adds CT siblings. Cross-link from predecessor needed (deferred follow-up).
- [SSoT phase-scope plan (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md) — **consumer + overlap**. Consumes `ct_test` in `phase-scopes.yaml`'s CT rows. SSoT plan's item 3 (scaffolder fully-resolved paths) overlaps this plan's item 3a (sutNamespace signature threading); see item 3a's overlap note. SSoT plan needs amendment to consume this plan's signatures (deferred follow-up).

## Locked decisions

Inherited from predecessor:

- Hexagonal `port` / `adapter` / `core` vocabulary across all testkit roles. Plain `test` for test-file keys (matching `at_test`, `ct_test`).
- New keys append at the **end** of the `canonicalPathKeys()` slice — preserving the existing ordering-by-position invariant.
- The migrate path back-fills any absent canonical key automatically — no opt-in needed for existing projects.

Derived during this plan's 2026-05-18 refine walk:

- **Deterministic stem construction.** Per-language stems in `pathStems()` are pinned against the shop template's `latest/` form (the canonical, non-legacy layout). No guess-by-analogy. Adding new keys without a pinnable shop layout is blocked (use [[feedback_paths_deterministic_no_guessing]]).
- **`<sutNamespace>` only threads through Java stems.** TypeScript and dotnet stems are `sut_namespace`-free (their test folder trees aren't structured by namespace the way JVM packages are). The new `sutNamespace` parameter on `pathStems()`/`DefaultPaths()` is ignored by TS/dotnet branches.
- **`latest`/`Latest` is doctrine literal**, not project-customizable. The arch-version layer is part of the standard scaffold layout the tool writes. Projects with non-`latest`-only layouts (e.g. shop's `legacy/modNN`) extend via their own `paths:` block overrides; they don't drive default-stem doctrine.
- **AT and CT are siblings under a per-language parent**: `tests/latest/` (TS), `src/test/java/<sutNamespace>/latest/` (Java), `SystemTests/Latest/` (dotnet). Their stems are disjoint (no prefix overlap), so `check_phase_scope` (SSoT plan item 8) can use straight prefix matching once both are fully-resolved.
- **Dotnet contract test naming is asymmetric** vs other test types: `ExternalSystemContractTests` (not `ContractTests`). Encoded as a literal in `pathStems()`, not as a transformation rule.

## Items

### 1. Determine the full CT key set — RESOLVED

The schema in the SSoT plan's `phase-scopes.yaml` references:

- `ct_test` — contract test files (new; parallel to `at_test`)
- `external_driver_port`, `external_driver_adapter` — **already exist** in `canonicalPathKeys()` (the original four).
- `dsl_port`, `dsl_core` — **already in predecessor's item 1** (shared AT/CT).

**Resolved 2026-05-18 (refine walk):** only `ct_test` is added. No CT-specific DSL split, no harness/runner key.

> **Refined 2026-05-18:** CT-side key set is `ct_test` only. **Why:** the four CT prompts (`internal/assets/runtime/prompts/atdd/ct-*.md`) read the same `dsl-core.md` / `driver-port.md` architecture docs as the AT prompts — DSL is shared, not split, so `ct_dsl_port`/`ct_dsl_core` would be vocabulary inflation with no architecture behind them. `ct_runner` has no current code or doc referent. `external_driver_port` / `external_driver_adapter` already exist; `dsl_port` / `dsl_core` are added by the predecessor. The downstream `CT_GREEN_STUBS` scope question (Family B `external_driver_adapter` vs the Family A `external-systems/stubs/` tree) is an SSoT-plan concern, not a vocabulary concern.

### 2. Add `ct_test` to `canonicalPathKeys()` + rewrite the `DefaultPaths` docstring

#### 2a. Slice change

Append `ct_test` to `canonicalPathKeys()` (lines 48–58 of `internal/projectconfig/paths_defaults.go`):

```go
return []string{
    "driver_port",
    "driver_adapter",
    "external_driver_port",
    "external_driver_adapter",
    "at_test",
    "dsl_port",
    "dsl_core",
    "ct_test",    // NEW — contract test files
}
```

#### 2b. Docstring rewrite

Rewrite the `DefaultPaths` docstring (lines 5–24) to reflect both the new key set (2a) **and** the new per-language stems (items 3a/b/c). Three places to update:

1. **Key-list paragraph** (lines 6–9): bump count "seven" → "eight" and add `, ct_test` to the comma-separated list.

2. **Per-language tree block** (lines 17–23): rewrite to match items 3a/b/c's deterministic stems. Proposed final shape (executor: pin exact values against items 3a/b/c when editing):

   ```
   //   - typescript: <root>/src/testkit/{driver|external|dsl}/{port|adapter|core},
   //     <root>/tests/latest/{acceptance|contract}
   //   - java:       <root>/src/main/java/testkit/{driver|external|dsl}/{port|adapter|core},
   //     <root>/src/test/java/<sutNamespace>/latest/{acceptance|contract}
   //   - dotnet:     <root>/Testkit.{Driver|External|Dsl}.{Port|Adapter|Core},
   //     <root>/SystemTests/Latest/{AcceptanceTests|ExternalSystemContractTests}
   ```

3. **Signature note** (lines 5–14 prose): mention the new `sutNamespace` parameter on `DefaultPaths` and that Java stems interpolate it (TS/dotnet ignore it). State that `latest`/`Latest` is doctrine literal, not project-customizable.

> **Refined 2026-05-18:** Item 2 split into 2a (slice change) + 2b (docstring rewrite). **Why:** the original item 2 assumed the docstring update was a small append on top of unchanged at_test stems, but items 3a/b/c now change the at_test stems and add ct_test stems with `<sutNamespace>` threading. The docstring per-language tree block needs full rewriting, not appending. Splitting makes the surface area explicit for the executor and keeps 2a as a trivial one-line confirmable change. Earlier (pre-scope-expansion) note that the executor should pull stems from item 3 still applies — sub-item 2b explicitly references items 3a/b/c.

### 3. Thread `sut_namespace` + fix at_test stems + add ct_test stems — deterministic against shop-`latest`

The existing `at_test` stems committed by the predecessor (`src/test` for TS, `src/test/java` for Java, `Tests` for dotnet) do **not** match the shop template tree, where AT tests actually live at `tests/latest/acceptance` (TS), `src/test/java/<package>/latest/acceptance` (Java), and `SystemTests/Latest/AcceptanceTests` (dotnet). Predecessor's stems were guessed by analogy and predate the shop-`latest` doctrine being load-bearing.

This plan corrects them deterministically against shop-`latest` reality **and** adds `ct_test` stems as siblings, in a single atomic edit to `internal/projectconfig/paths_defaults.go`. Three sub-items, executed together.

#### 3a. Thread `sutNamespace` through `pathStems()` / `DefaultPaths()` signatures

Java stems include `<sut_namespace>` as a folder segment (e.g. default `src/test/java/shop/latest/acceptance`). TS and dotnet stems are `sut_namespace`-free (the namespace doesn't structure their test folder trees the way JVM packages do).

Signature changes in `internal/projectconfig/paths_defaults.go`:

- `func canonicalPathKeys() []string` — **unchanged**.
- `func pathStems(testLang string) ([]string, bool)` → `func pathStems(testLang, sutNamespace string) ([]string, bool)`. TS/dotnet ignore the parameter; Java interpolates it into the relevant stems.
- `func DefaultPaths(testLang, systemTestRoot string) map[string]string` → `func DefaultPaths(testLang, systemTestRoot, sutNamespace string) map[string]string`.

Caller updates: every call site of `DefaultPaths` must pass `sutNamespace`. The scaffolder is the main caller; it already knows sut_namespace at scaffold time (computed from `system.sut_namespace`, defaulting to the last path segment of `system.repo`).

**Overlap with SSoT plan item 3:** the SSoT plan's item 3 ("Scaffolder change — write fully-resolved paths at scaffold") also wires `sut_namespace` at scaffold time, but via a different route (retires `system.sut_namespace` as a `gh-optivem.yaml` field and bakes it into the values written to `paths:`). This plan's signature threading is the **substrate**: `pathStems()` produces per-language stems with `<sutNamespace>` already substituted. SSoT plan's scaffolder consumes the result. Cross-link both directions.

#### 3b. Fix at_test stems to shop-`latest` deterministic shape

Update `pathStems()` so `at_test` returns:

| Language | New at_test stem | Previous (wrong) stem |
|---|---|---|
| TypeScript | `tests/latest/acceptance` | `src/test` |
| Java | `src/test/java/<sutNamespace>/latest/acceptance` | `src/test/java` |
| .NET | `SystemTests/Latest/AcceptanceTests` | `Tests` |

`latest` / `Latest` is **doctrine literal** (always present, not project-customizable). Shop's customization of `<sutNamespace>` to a fully-qualified Java package prefix (`com/mycompany/myshop/systemtest`) is a project-level override of `system.sut_namespace`, **not** a doctrine variation. Default scaffold writes `src/test/java/shop/latest/acceptance`; shop's `paths:` block overrides with the fully-qualified value.

#### 3c. Add ct_test stems as siblings of at_test

Add `ct_test` to `pathStems()`:

| Language | ct_test stem |
|---|---|
| TypeScript | `tests/latest/contract` |
| Java | `src/test/java/<sutNamespace>/latest/contract` |
| .NET | `SystemTests/Latest/ExternalSystemContractTests` |

**Dotnet naming asymmetry note.** acceptance/e2e/smoke use `<TestType>Tests` (e.g. `AcceptanceTests`), but contract uses `ExternalSystemContractTests` (longer, distinguishes the external-system contract notion from in-process contract notions). This is pinned against the shop tree, not a stem-rule simple swap — encode it as a literal in `pathStems()`, not as a transformation.

> **Refined 2026-05-18:** Item 3 grew from "add ct_test stems by analogy" into a 3-sub-item correction of predecessor's at_test stems **plus** deterministic ct_test stems, all in one atomic file edit. **Why:** evidence from the shop template (`tests/latest/acceptance`, `src/test/java/<package>/latest/acceptance`, `SystemTests/Latest/AcceptanceTests`) shows predecessor's at_test stems were guessed by analogy and don't match where tests actually live. The user's [[feedback_paths_deterministic_no_guessing]] rule (stamped during this refine) makes guess-by-analogy unacceptable. Since fixing at_test + adding ct_test touches the same file/function atomically, scope grew rather than splitting into a separate plan. `<sutNamespace>` threading was forced by Java's package-path convention; TS and dotnet stay sut_namespace-free. `latest` is doctrine literal, not config. Overlaps SSoT plan item 3 (which also threads sut_namespace at scaffold time); cross-link both plans in Hand-off.

### 4. Update `placeholders.md` Family B example block

Edit `internal/assets/global/docs/atdd/process/placeholders.md` Family B example block (lines 27–36 today) to:

1. **Fix** the existing `at_test` example value (was wrong per item 3b's correction): `system-test/typescript/src/test` → `system-test/typescript/tests/latest/acceptance`.
2. **Add** `ct_test` as a new line in canonical order (after `dsl_core`): `ct_test: system-test/typescript/tests/latest/contract`.

Final block:

```yaml
paths:
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
  external_driver_port: system-test/typescript/src/testkit/external/port
  external_driver_adapter: system-test/typescript/src/testkit/external/adapter
  at_test: system-test/typescript/tests/latest/acceptance
  dsl_port: system-test/typescript/src/testkit/dsl/port
  dsl_core: system-test/typescript/src/testkit/dsl/core
  ct_test: system-test/typescript/tests/latest/contract
```

The example stays TypeScript-focused (matches the existing block); Java's `<sutNamespace>` interpolation behavior lives in `paths_defaults.go` doctrine (item 3a), not in this example. A larger rewrite of `placeholders.md` (substitution-model retirement under SSoT plan's δ) is owned by the [SSoT plan's item 9](20260518-1530-atdd-phase-scope-ssot.md) — out of scope here.

> **Refined 2026-05-18:** Rewrote item 4 to (a) drop the dead reference to item 3's (a)/(b)/(c) ladder, (b) make explicit that `at_test`'s example value also needs fixing (not just adding ct_test), and (c) pin the concrete final block inline so the executor doesn't have to re-derive. **Why:** the original wording assumed item 3 produced a verification ladder + that at_test stems were stable. Both assumptions are wrong post-item-3 expansion — item 3b changes the at_test stem deterministically, so the placeholders.md example must change in lockstep. Java/dotnet variants intentionally not added: example is illustrative; doctrine lives in `paths_defaults.go`.

## Out of scope

- **Per-phase scope assignment for CT phases** — owned by the [SSoT plan](20260518-1530-atdd-phase-scope-ssot.md)'s `phase-scopes.yaml`. This plan only owns the vocabulary substrate + deterministic stems.
- **CT-cycle phase doc rewrites** — the `docs/atdd/process/change/behavior/ct/` tree (less mature than AT) is its own consolidation. Not touched here.
- **BPMN CT process flow edits** — `process-flow.yaml`'s CT rows already exist; this plan doesn't reshape them.
- **`smoke_test` key** — separate deferred plan ([plans/deferred/20260518-1530-smoke-test-family-b-key.md](deferred/20260518-1530-smoke-test-family-b-key.md)). The shop tree confirms `smoke` is a sibling of `acceptance`/`contract` under `tests/latest/`, `src/test/java/<sutNamespace>/latest/`, and `SystemTests/Latest/`; the deferred plan should adopt the same deterministic stem shape this plan establishes.
- **`<TestType>Tests`-style stems for e2e/smoke** — out of scope here (this plan only touches `at_test` and `ct_test`). Future plans adding `e2e_test` / `smoke_test` should follow the same deterministic-against-shop-`latest` pattern.
- **SSoT plan δ (fully-resolved paths in `gh-optivem.yaml`)** — owned by [SSoT plan item 3](20260518-1530-atdd-phase-scope-ssot.md). This plan's threading of `sutNamespace` through `pathStems()` is the substrate SSoT consumes; the actual scaffolder rewrite (and retirement of `system.sut_namespace` as a config field) stays in SSoT.
- **Big rewrite of `placeholders.md`** under SSoT's substitution-model retirement — owned by [SSoT plan item 9](20260518-1530-atdd-phase-scope-ssot.md). This plan only touches the example block's values (item 4).
- **Editing predecessor plan (20260518-1500) or SSoT plan (20260518-1530) cross-references** — listed below as deferred follow-ups; not done in this plan's execution to avoid concurrent-agent collision risk on those files.

## Hand-off

**Execute order:** items 1 (already resolved) → 2a → 2b → 3a → 3b → 3c → 4. Items 2a/2b/3a/3b/3c all touch `internal/projectconfig/paths_defaults.go` and land as a single atomic edit; item 4 is a separate edit to `internal/assets/global/docs/atdd/process/placeholders.md`.

**Pre-requisites:**
- Predecessor AT-vocabulary plan ([20260518-1500](20260518-1500-atdd-phase-scope-placeholders.md)) items 1–3 must have landed (already true as of commits `8322c38`/`d7ec876`). This plan corrects predecessor's `at_test` stems and extends the same `canonicalPathKeys()` slice.

**Consumer + overlap:** the SSoT phase-scope plan ([20260518-1530](20260518-1530-atdd-phase-scope-ssot.md)):
- **Consumes** `ct_test` in its `phase-scopes.yaml` (SSoT plan item 1).
- **Overlaps** with this plan's item 3a (sutNamespace signature threading) — SSoT plan's item 3 has its own scaffolder fully-resolved-paths work that builds on top of this plan's signature changes.
- This plan **must land before SSoT**, not alongside or after, so SSoT's scaffolder item 3 can consume the new `DefaultPaths(testLang, systemTestRoot, sutNamespace)` signature.

**Callers of `DefaultPaths` to update** (signature grows by one parameter — `sutNamespace`):
- The scaffolder (location: `grep -r 'DefaultPaths(' internal/` at execute time to enumerate).
- Any tests that call `DefaultPaths` directly. The known test fixture file `internal/projectconfig/config_commands_test.go` (commit `d7ec876`) extends no-op paths fixtures; verify whether it constructs default paths via `DefaultPaths` and update accordingly.
- The migrate-path back-fill caller, if it invokes `DefaultPaths` to derive missing canonical-key values.

**Pre-execute checks:**
- Grep `plans/*.md` for concurrent agent pickup markers on `paths_defaults.go` / `placeholders.md` before adding this plan's execute marker, per [[feedback_check_concurrent_agents]].
- Inspect `git status` for uncommitted changes on those files from prior sessions.
- Re-verify shop-`latest` stems against `academy/shop/system-test/` tree before pinning — the tree may have evolved since this plan's 2026-05-18 refine.

**Test guardrails to add** (during execution):
- A `pathStems()` test asserting Java's stem interpolates `sutNamespace` correctly (default = `shop` → `src/test/java/shop/latest/acceptance`).
- A `DefaultPaths()` test asserting that for each `(testLang, sutNamespace)` pair, the returned map's `at_test` and `ct_test` values match the shop-`latest` tree shape (parametric test, one row per language).
- A regression assertion that all canonical keys are present in the returned map (catches future stem-shape drift).

**Post-execute:**
- Run the migrate path against shop's 12 `gh-optivem-*.yaml` files to back-fill `paths:` blocks deterministically with the corrected stems. Spot-check the resulting values against `academy/shop/system-test/` directories. (The migrate path is the canonical mechanism; do NOT manually hand-edit shop's yaml files.)
- Run `gh optivem config validate` on each shop yaml to confirm no validation errors.

**Deferred follow-ups (cross-link work on sibling plans — not executed here):**
- **Edit predecessor plan ([20260518-1500](20260518-1500-atdd-phase-scope-placeholders.md))** to add a "corrective successor" note pointing at this plan: "at_test stems committed by item 3 were guessed by analogy and have been corrected deterministically by [plans/20260518-1742-family-b-stems-and-ct-vocab.md] (refined 2026-05-18)".
- **Edit SSoT plan ([20260518-1530](20260518-1530-atdd-phase-scope-ssot.md))** item 3 to reference this plan's signature changes as the substrate; update the resolution-note for `ct_test` (which currently points at `20260518-1742-ct-family-b-vocabulary.md` — the pre-rename filename) and add the at_test correction to the "Hard dependencies" section.
- **Both edits SHOULD be run in a separate `/refine-plan` or direct-edit session**, after verifying the pickup-marker state on each target plan (per [[feedback_concurrent_agent_collision]]).
