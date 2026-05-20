# Path-key vocabulary

This doc describes the canonical **path-key vocabulary** consumed by
`gh-optivem.yaml system_test.paths:` (per-project, user-owned values)
and `internal/atdd/phase-scopes.yaml` (per-phase scope assignment,
doctrine owned by gh-optivem). It is the single place to look up which
keys exist, what they mean, and what's reserved.

Layer names are part of canonical ATDD vocabulary: the same key set in
every gh-optivem project. **Users own VALUES** (physical paths in their
repo); **gh-optivem owns NAMES** (the key set itself).

## Two key families

Every path key is one of two families, sharing a single flat namespace.

### Family A — fixed-schema (top-level `gh-optivem.yaml` fields)

Sourced from top-level fields in `gh-optivem.yaml`. The key name is
fixed; the value is the corresponding config field.

| Key | Source |
|---|---|
| `language` | `system.lang` (or `system_test.lang` when `system.lang` is empty — multitier) |
| `architecture` | `system.architecture` |
| `system_path` | `system.path` (fully resolved, sut_namespace baked in per SSoT) |
| `system_test_path` | `system_test.path` |

**Path-shaped Family A keys eligible for `phase-scopes.yaml` scope:**
`system_path`. `system_test_path` is **not** scope-eligible — it is the
parent of every Family B testkit key and admitting it would let any
phase escape the layer partition (see `FamilyAPathKeysInScope` in
`internal/atdd/phase_scopes.go`).

### Family B — named locations (under `system_test.paths:`)

User-owned values for a fixed, gh-optivem-owned key set. Every key
under `system_test.paths:` in `gh-optivem.yaml` is a canonical Family B
key from `CanonicalPathKeys()` in
`internal/projectconfig/paths_defaults.go`. The current set:

| Key | Layer it names |
|---|---|
| `driver_port` | system-test driver port (interface) |
| `driver_adapter` | system-test driver adapter (implementation) |
| `external_system_driver_port` | external-system driver port (interface) |
| `external_system_driver_adapter` | external-system driver adapter (implementation) |
| `at_test` | acceptance test files |
| `dsl_port` | DSL port (interface) |
| `dsl_core` | DSL core (implementation) |
| `ct_test` | contract test files |

Values are **fully-resolved physical paths** set at scaffold time
(per plan 20260518-1530 item 3). No runtime `${...}` substitution.

### Ownership: scaffold-authoritative at `init`, operator-owned afterwards

`gh optivem init` writes the `system_test.paths:` block as the
**authoritative initial value matching the directory tree the same
scaffolder just created**. The scaffolder owns both sides of the join
(YAML + tree), so the eight Family B values are correct by construction
at that moment — they are not "defaults the operator never asked for".

After `init`, the block is operator-owned at every other layer:

- **Validate-time**: `Validate()` in `internal/projectconfig/config.go`
  (Rule 22a) rejects a missing or non-canonical `system_test.paths:`
  block. The binary does not synthesise defaults to keep validation
  passing.
- **Migrate-time**: `gh optivem config migrate` does not back-fill the
  `paths:` block. Pre-SSoT configs without a `paths:` block must have
  one added by the operator before the next `gh optivem` command.

The asymmetry is intentional: only `init` can legitimately write a
`paths:` block from a derived layout, because only `init` is also
creating the tree that block describes. Future work that adds a
"default paths" writer elsewhere in the binary — at validate-time, at
migrate-time, or as a runtime fallback — is regressing the doctrine
and should be rejected. The single derivation lives in
`projectconfig.DefaultPaths` and is called from exactly one place:
`internal/steps/optivem_yaml.go::BuildOptivemYAML`.

## Default values (TypeScript example)

The scaffolder writes a fully-resolved `paths:` block nested under
`system_test:`, based on the project's `system_test.path`,
`system_test.lang`, and `sut_namespace` (derived from `system.repo`'s
last segment). For a TypeScript project with
`system_test.path: system-test/typescript` and `sut_namespace: shop`,
the emitted YAML is:

```yaml
system_test:
  path: system-test/typescript
  repo: optivem/shop
  lang: typescript
  sonar_project: optivem_shop-system-test
  paths:
    driver_port: system-test/typescript/src/testkit/driver/port/shop
    driver_adapter: system-test/typescript/src/testkit/driver/adapter/shop
    external_system_driver_port: system-test/typescript/src/testkit/external/port/shop
    external_system_driver_adapter: system-test/typescript/src/testkit/external/adapter/shop
    at_test: system-test/typescript/tests/latest/acceptance
    dsl_port: system-test/typescript/src/testkit/dsl/port/shop
    dsl_core: system-test/typescript/src/testkit/dsl/core/shop
    ct_test: system-test/typescript/tests/latest/contract
```

Java and dotnet defaults differ in stem shape (Java structures tests
by package: `src/test/java/<sut_namespace>/latest/acceptance`; dotnet
uses literal subdirs: `SystemTests/Latest/AcceptanceTests`). See
`pathStems()` in `internal/projectconfig/paths_defaults.go` for the
authoritative per-language stems pinned against the shop template's
`latest/` form. `latest` / `Latest` is doctrine literal — always
present, not project-customizable.

## Validation rules

- **Family B keys that shadow Family A names are rejected.** A typo'd
  `system_test.paths.language: typescript` would otherwise quietly
  override the canonical `system.lang` value. Rejected names today:
  `language`, `architecture`, `system_path`, `system_test_path`,
  `sut_namespace`.
- **Non-canonical `system_test.paths.<name>` keys are rejected** (per
  plan 20260518-1530 item 5). The validator enumerates
  `system_test.paths:` keys against `CanonicalPathKeys()` and
  hard-errors on any unknown name. Catches typos and stale keys.
- **`${...}` markers in `system_test.paths:` values are rejected** (per
  plan 20260518-1530 item 5). Under SSoT, paths must be fully resolved
  at scaffold time; runtime substitution is retired.
- **`paths:` on non-system_test tiers is rejected.** The block is
  meaningful only on `system_test`, mirroring how `TierSpec.Config` is
  also system_test-only. A typo'd `system.backend.paths:` would parse
  as a no-op without this rule.

See `internal/atdd/phase_scopes.go` (`CanonicalPathKeys` consumer +
`FamilyAPathKeysInScope`) and the `Validate()` method in
`internal/projectconfig/config.go` for the authoritative rule
implementations.

## Where the per-phase scope comes from

The vocabulary here is consumed by `internal/atdd/phase-scopes.yaml`,
which maps each BPMN phase id to the layer names that phase's agent
may modify. Layer names are joined with `gh-optivem.yaml
system_test.paths:` values at runtime — by the `check_phase_scope`
action (BPMN runtime) or the `gh optivem process scope` CLI query —
to produce the per-phase resolved-path set.

For the scope rule itself ("only modify paths listed in the phase's
scope; otherwise stop and alert the user"), see
[`shared/scope.md`](shared/scope.md).

## Historical note: pre-SSoT `${...}` substitution

Pre-SSoT, phase docs and agent prompts referenced `${...}`
placeholders (e.g. a Family B path key joined with the
sut_namespace key) that the sync-time tool resolved against a
`PlaceholderMap`. That mechanism is
retired (per plan 20260518-1530's locked decision δ). Phase docs now
reference layer **names** only — no `${...}` syntax — and `paths:`
values are fully resolved at scaffold time. Pre-SSoT projects migrate
via `gh optivem config migrate`, which joins `sut_namespace` into each
`paths:` value, joins it into `system.path`, and deletes
`system.sut_namespace` from the file in one deterministic pass.

## Historical note: top-level `paths:` block

Before plan 20260520-1900, the Family B block sat at the top level of
`gh-optivem.yaml` (`paths:` directly under the document root). The
keys all describe the system_test tier's layered layout, so the block
was relocated under `system_test:` alongside `path`, `repo`, `lang`,
`config`, and `sonar_project` — every other field that describes the
system_test tier. The key set, validation rules, and `PlaceholderMap`
output (still flat — `driver_port`, `at_test`, etc. emit at the top of
the placeholder namespace) are unchanged; only the YAML location moved.
The migrate command continues to operate on the post-move location:
the external-driver key-rename and SSoT-join passes both walk
`system_test.paths:` directly.
