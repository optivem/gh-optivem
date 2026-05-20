# Nest top-level `paths:` under `system_test:`

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-20T16:47:33Z`

**Cross-references**:

- Sibling in-flight plan: [`20260520-1834-strip-init-paths-default-scaffolding.md`](20260520-1834-strip-init-paths-default-scaffolding.md)
  also edits `internal/steps/optivem_yaml.go:195` and the same test surface
  but answers a different question ("does `init` seed `paths:` at all?").
  This plan answers "where does `paths:` *live* in the schema?". Both can
  land in either order; see [Sequencing](#sequencing) below.
- Doctrine precedent: the external-driver rename plan
  [`20260519-0704-...`](deferred/20260519-0704-rename-external-driver-keys.md) — same
  shape of change (rename + migrate-command rewrite + legacy-detection
  validation rule). The `ExternalDriverKeyRenames` map and Rule 22b in
  `internal/projectconfig/config.go` are the template to mirror.
- Predecessor doctrine: the SSoT plan
  [`20260518-1530-...`](deferred/20260518-1530-ssot-path-model.md) — established
  that `paths:` values are fully resolved at scaffold time, with no
  runtime `${...}` substitution. Nothing in this plan disturbs that
  rule; only the *location* of the block changes.

## Context

`internal/projectconfig/config.go:187`:

```go
Paths map[string]string `yaml:"paths,omitempty"`
```

The eight canonical Family B keys
(`driver_port`, `driver_adapter`, `external_system_driver_port`,
`external_system_driver_adapter`, `at_test`, `dsl_port`, `dsl_core`,
`ct_test`) are all *system-test-tier* paths — they all live under
`system_test.path` in the resolved value. But they sit in a top-level
`paths:` block:

```yaml
system_test:
  path: system-test/dotnet
  repo: optivem/shop
  lang: dotnet
  config: system-test/tests.yaml

paths:                                # ← top-level, but every value
  driver_port:    system-test/dotnet/Testkit.Driver.Port/shop     #   below this line is
  driver_adapter: system-test/dotnet/Testkit.Driver.Adapter/shop  #   under system_test.path
  …
```

The asymmetry is real: every entry is `<system_test.path>/<stem>/<sut_namespace>`,
but the block sits where a future `system.paths:` (for backend
internals) or `frontend.paths:` (for frontend E2E layouts) would
collide naming-wise.

## The reframe

Top-level `paths:` was a reasonable name when the only consumer was
phase-doc substitution and the values were conceptually "named
locations the operator owns". The reframe is:

> The eight keys describe the **system_test tier's layered layout**.
> They belong on the system_test tier, alongside `path`, `repo`,
> `lang`, `config`, `sonar_project` — every other field that describes
> the system_test tier.

After the move:

```yaml
system_test:
  path: system-test/dotnet
  repo: optivem/shop
  lang: dotnet
  config: system-test/tests.yaml
  paths:
    driver_port:    system-test/dotnet/Testkit.Driver.Port/shop
    driver_adapter: system-test/dotnet/Testkit.Driver.Adapter/shop
    external_system_driver_port:    system-test/dotnet/Testkit.External.Port/shop
    external_system_driver_adapter: system-test/dotnet/Testkit.External.Adapter/shop
    at_test:        system-test/dotnet/SystemTests/Latest/AcceptanceTests
    dsl_port:       system-test/dotnet/Testkit.Dsl.Port/shop
    dsl_core:       system-test/dotnet/Testkit.Dsl.Core/shop
    ct_test:        system-test/dotnet/SystemTests/Latest/ExternalSystemContractTests
```

Knock-on benefit: when (if) a future tier needs its own named-location
block, it gets `system.paths:` or `system.backend.paths:` — a clean
parallel structure rather than overloading the top-level slot.

## What stays the same

- **Family B as a concept stays.** `path-keys.md`'s two-family model
  is unchanged; only the *location* of the Family B block in the
  schema changes. Keys, vocabulary, validation rules around shadowing
  / canonical-set / no-substitution all keep their meaning.
- **`CanonicalPathKeys()` and `DefaultPaths()` stay.** They still return
  the same eight keys / same per-language stems. Only the assignment
  site (`pc.Paths = …` → `pc.SystemTest.Paths = …`) moves.
- **`PlaceholderMap()` output stays.** Still flat — `driver_port`,
  `at_test`, etc. emit at the top of the placeholder namespace. Only the
  internal read changes (`c.Paths` → `c.SystemTest.Paths`).
- **SSoT (fully-resolved, no `${...}`) stays.** The no-substitution
  rule and the migrate-time `sut_namespace` join logic are unchanged
  in meaning; they just operate on the nested node.

## Files touched

### Schema move (the core change)

- **`internal/projectconfig/config.go`**
  - Remove the top-level `Paths map[string]string \`yaml:"paths,omitempty"\`` field
    from `Config` (line 187). No `LegacyPaths` alias — this is a teaching
    repo; stale on-disk configs lose the top-level block on next Load
    (YAML decoder drops unknown keys) and the existing canonical-set
    validation (Rule 22) surfaces the missing nested keys.
  - Add `Paths map[string]string \`yaml:"paths,omitempty"\`` to `TierSpec`
    (line 270). Document that on backend/frontend tiers the field is
    rejected (mirroring how `TierSpec.Config` is already restricted to
    system_test — see Rule 0b at line 466).
  - Update `PlaceholderMap` (line 383): read from `c.SystemTest.Paths`
    instead of `c.Paths`. Behaviour preserved.
  - Update Rules 22 / 22a / 22b (lines 744–813): read from
    `c.SystemTest.Paths`; update error message paths from `paths.<key>`
    to `system_test.paths.<key>`.
  - Add a new rule under tier-side validation: reject non-empty
    `c.System.Backend.Paths` / `c.System.Frontend.Paths` — `paths:` is
    system_test-only today (mirrors Rule 0b's per-tier `Config`
    restriction).

### Migrate command — read-site update only

No relocation pass: per teaching-repo doctrine, stale top-level `paths:`
blocks are not migrated. The existing migrate passes that already touch
`paths:` need their read sites updated to the new location.

- **`config_commands.go`** — `runConfigMigrate` (the testable core
  around line 327):
  - Update the existing `renameExternalDriverKeys` and SSoT-join passes
    to operate on `system_test.paths:` instead of top-level `paths:`.
    They never see a pre-nesting config; the YAML they walk after this
    change always has the nested location.
  - Update the `Long:` help text (lines 272–295): the two bullets that
    mention `paths:` say `system_test.paths:`.

- **`config_commands_test.go`** — update existing rename + SSoT-join
  table entries so the YAML input uses `system_test.paths:` (not
  top-level `paths:`). No new relocation-specific entries.

### Scaffolder

- **`internal/steps/optivem_yaml.go:195`**
  - Change `pc.Paths = projectconfig.DefaultPaths(…)` to
    `pc.SystemTest.Paths = projectconfig.DefaultPaths(…)`. Trivial one-line
    swap.
  - If the strip-init plan lands first this line is already gone — the
    plan is then a no-op at this file. See [Sequencing](#sequencing).

### Runtime read sites

- **`internal/atdd/runtime/actions/bindings.go:1052`** — change
  `cfg.Paths[layer]` to `cfg.SystemTest.Paths[layer]`. Single line; the
  surrounding doctrine comment at lines 1033–1036 needs the same
  qualifier swap.

- **`process_commands.go:278`** — same: `cfg.Paths[layer]` →
  `cfg.SystemTest.Paths[layer]`.

### Tests (read-site updates)

- **`internal/projectconfig/config_test.go`** — every test that builds
  a `Config` literal with `Paths: map[…]` needs to move the literal
  under `SystemTest: TierSpec{Paths: map[…]}`. Same for YAML fixtures
  that use top-level `paths:`. Add a new test for the per-tier-rejection
  rule (backend/frontend tiers cannot carry `paths:`).
- **`internal/projectconfig/paths_defaults_test.go`** — no schema
  change needed (it tests `DefaultPaths` in isolation), but any test
  that round-trips through `Marshal` / `Load` needs the literal moved.
- **`process_commands_test.go`** — `cfg.Paths[…]` reads at lines 95–97
  become `cfg.SystemTest.Paths[…]`.
- **`internal/atdd/runtime/clauderun/clauderun_test.go`** — `Paths:`
  on `Config` literals (line 357 and similar) move under `SystemTest:`.
- **`internal/atdd/runtime/actions/bindings_test.go`** — same.
- **`internal/atdd/runtime/driver/driver_test.go`** — same.
- **`internal/steps/optivem_yaml_test.go`** — `got.Paths` checks
  (lines 236, 240, 244–245) become `got.SystemTest.Paths`. The
  `pm := pc.PlaceholderMap()` test at line 274 is unaffected — the
  flat name namespace is preserved.

### Docs

- **`internal/projectconfig/path-keys.md`** — the canonical-key vocab
  doc. Update Family B section header from "under `paths:`" to "under
  `system_test.paths:`". Update the TypeScript default-block example
  (lines 66–76) to show the nested location. Add a one-paragraph note
  in the historical section explaining the move and pointing at this
  plan + the migrate command.

- **`CLAUDE.md`** (this repo) — already has a "No GitHub Pages" doctrine
  block. Consider adding a one-liner under a new "Schema" section
  warning future agents not to reintroduce top-level `paths:` — same
  shape as the GitHub Pages warning. Optional; the validate-time legacy
  alias is the real guard.

## Knock-on

- **shop repo's 12 `gh-optivem-*.yaml` configs.** Per the sibling
  strip-init plan, these currently have no `paths:` block at all; once
  they grow one they should write it nested under `system_test:`. No
  data-fix or migration needed.

- **The four scaffolder YAML emissions per init invocation.** Once
  `Paths` is a field of `TierSpec`, the emitted YAML will write
  `paths:` two levels deep instead of at top level. The yaml.v3
  marshaller handles this without further intervention — no
  custom marshalling needed.

- **`phase-scopes.yaml` and `FamilyAPathKeysInScope`.** Phase-scope
  entries reference layer **names** (e.g. `driver_port`, `at_test`)
  not paths-block locations. No change. The doc comment at
  `phase_scopes.go:24` mentions "projectconfig.CanonicalPathKeys()" by
  reference, which still returns the same names — no edit needed.

## Sequencing

This plan and [`20260520-1834-strip-init-paths-default-scaffolding.md`](20260520-1834-strip-init-paths-default-scaffolding.md)
both touch `optivem_yaml.go:195`, the migrate command, and the same
test surface. They are otherwise orthogonal — *whether* to seed
`paths:` and *where* `paths:` lives are independent decisions.

**Recommended order**: land **this plan first** (mechanical schema
move with full legacy-alias + migrate coverage), then the strip-init
plan (a one-line removal in the new schema). Reverse is also fine; if
strip-init lands first, the relocation work in this plan shrinks by
one line (`optivem_yaml.go:195` is gone) and one test case.

If both land in parallel (one commit each), expect a small
trivially-resolvable conflict at `optivem_yaml.go:195` and at the
`Paths` assertions in `optivem_yaml_test.go`.

## Open questions

1. ~~**Legacy alias detection — error or auto-relocate at load time?**~~
   **Decided (2026-05-20):** Drop legacy-alias machinery entirely. No
   `LegacyPaths` field, no Rule 0d, no migrate-relocation pass.
   gh-optivem is a teaching repo; teachers regenerate configs by
   re-running `gh optivem init`, so there's no operator-base whose
   hand-edited configs must be preserved across schema changes. Stale
   on-disk top-level `paths:` blocks are silently dropped by the YAML
   decoder on next Load and surface as the normal "missing canonical
   keys" Rule 22 validation error. See
   `feedback_teaching_repo_no_legacy.md` in memory.

2. ~~**Per-tier `paths:` restriction — Validate-rejection or schema-level
   omission?**~~ **Decided (2026-05-20):** Match the Rule-0b precedent.
   Keep `Paths` on `TierSpec`; add a Validate rule rejecting non-empty
   `c.System.Backend.Paths` / `c.System.Frontend.Paths`. Type-level
   enforcement (a new `SystemTestSpec` struct) is the more "make invalid
   states unrepresentable" path, but it would diverge from how
   `TierSpec.Config` is already handled and expand this plan's scope. If
   the codebase later wants type-level enforcement for system-test-only
   fields, the natural refactor lifts `Config` *and* `Paths` (and any
   future siblings) onto a `SystemTestSpec` together — separate plan.

3. **CLAUDE.md doctrine line — yes or no?** The validate-time legacy
   alias is the real guard against regression. A CLAUDE.md line is a
   belt-and-braces hint for agents proposing schema changes. Slight
   preference for adding it (cheap; the GitHub Pages doctrine
   precedent shows the shape works) but not blocking.

4. **Sequencing with the strip-init plan.** Recommend this-plan-first
   for clean staging; the user may prefer the reverse if they want to
   simplify this plan's scope by stripping the scaffolder seed before
   the relocation. Decide before pickup.
