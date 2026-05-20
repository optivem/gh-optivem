# Path-key vocabulary

This doc describes the canonical **path-key vocabulary** consumed by
`gh-optivem.yaml paths:` (per-project, user-owned values) and
`internal/atdd/phase-scopes.yaml` (per-phase scope assignment, doctrine
owned by gh-optivem). It is the single place to look up which keys
exist, what they mean, and what's reserved.

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

### Family B — named locations (under `paths:`)

User-owned values for a fixed, gh-optivem-owned key set. Every key
under `paths:` in `gh-optivem.yaml` is a canonical Family B key from
`CanonicalPathKeys()` in
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
The scaffolder writes a default block matching the project's layout;
the values are project-relative and may be hand-edited afterwards.

## Default values (TypeScript example)

The scaffolder writes a fully-resolved `paths:` block based on the
project's `system_test.path`, `system_test.lang`, and `sut_namespace`
(derived from `system.repo`'s last segment). For a TypeScript project
with `system_test.path: system-test/typescript` and
`sut_namespace: shop`, the default block is:

```yaml
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
  `paths.language: typescript` would otherwise quietly override the
  canonical `system.lang` value. Rejected names today: `language`,
  `architecture`, `system_path`, `system_test_path`, `sut_namespace`.
- **Non-canonical `paths.<name>` keys are rejected** (per plan
  20260518-1530 item 5). The validator enumerates `paths:` keys against
  `CanonicalPathKeys()` and hard-errors on any unknown name. Catches
  typos and stale keys.
- **`${...}` markers in `paths:` values are rejected** (per plan
  20260518-1530 item 5). Under SSoT, paths must be fully resolved at
  scaffold time; runtime substitution is retired.

See `internal/atdd/phase_scopes.go` (`CanonicalPathKeys` consumer +
`FamilyAPathKeysInScope`) and the `Validate()` method in
`internal/projectconfig/config.go` for the authoritative rule
implementations.

## Where the per-phase scope comes from

The vocabulary here is consumed by `internal/atdd/phase-scopes.yaml`,
which maps each BPMN phase id to the layer names that phase's agent
may modify. Layer names are joined with `gh-optivem.yaml paths:`
values at runtime — by the `check_phase_scope` action (BPMN
runtime) or the `gh optivem process scope` CLI query — to produce
the per-phase resolved-path set.

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
