# Path-key vocabulary

This doc describes the canonical **path-key vocabulary** consumed by
`gh-optivem.yaml system-test.paths:` (per-project, user-owned values)
and the inline per-phase scope on writing-agent MID nodes in
`internal/atdd/runtime/statemachine/process-flow.yaml` (per-phase scope
assignment, doctrine owned by gh-optivem). It is the single place to
look up which keys exist, what they mean, and what's reserved.

Layer names are part of canonical ATDD vocabulary: the same key set in
every gh-optivem project. **Users own VALUES** (physical paths in their
repo); **gh-optivem owns NAMES** (the key set itself).

All identifiers use **kebab-case** throughout (Q29 / Q40 + plan
20260525-2311 Q5): YAML keys, doc headings, prompt placeholders, anchor
slugs, Family A names. The non-path-shaped Family A keys (`language`,
`architecture`, `sut-namespace`, `system-test-path`) follow the same
kebab convention as Family B and as the top-level `gh-optivem.yaml`
keys.

## Two key families

Every path key is one of two families, sharing a single flat namespace.

### Family A — fixed-schema (top-level `gh-optivem.yaml` fields)

Sourced from top-level fields in `gh-optivem.yaml`. The key name is
fixed; the value is the corresponding config field.

| Key | Source |
|---|---|
| `language` | `system.lang` (or `system-test.lang` when `system.lang` is empty — multitier) |
| `architecture` | `system.architecture` |
| `system-path` | `system.path` (fully resolved, sut-namespace baked in per SSoT) |
| `system-db-migration-path` | `system.db-migration-path` (default `system/db/migrations`; shared across every SUT) |
| `system-test-path` | `system-test.path` |

**Path-shaped Family A keys eligible for per-phase scope:**
`system-path` and `system-db-migration-path`. `system-test-path` is
**not** scope-eligible — it is the parent of every Family B testkit key
and admitting it would let any phase escape the layer partition (see
`FamilyAPathKeysInScope` in `internal/atdd/phase_scopes.go`).

`system-db-migration-path` is the shared canonical migration set
(Flyway-ordered SQL files under `system/db/migrations` by default)
consumed by every SUT (3 languages × 2 architectures) via a Flyway
sidecar. It sits as a sibling of `system/monolith/` and
`system/multitier/`, not as a child of either — schema migrations are
architecture- and language-agnostic. Writing-agent MIDs that legitimately
need to add a migration (e.g. `implement-system` when the AT asserts
persisted state) carry it in their `write:` set; refactor-class phases
read it but cannot write to it (column renames are a schema change, which
is a behavior-change verb's job).

### Family B — named locations (under `system-test.paths:`)

User-owned values for a fixed, gh-optivem-owned key set. Every key
under `system-test.paths:` in `gh-optivem.yaml` is a canonical Family B
key from `CanonicalPathKeys()` in
`internal/projectconfig/paths_defaults.go`. The current set:

| Key | Layer it names |
|---|---|
| `driver-port` | system-test driver port (interface) |
| `driver-adapter` | system-test driver adapter (implementation) |
| `external-system-driver-port` | external-system driver port (interface) |
| `external-system-driver-adapter` | external-system driver adapter (implementation) |
| `at-test` | acceptance test files |
| `dsl-port` | DSL port (interface) |
| `dsl-core` | DSL core (implementation) |
| `ct-test` | contract test files |

Values are **fully-resolved physical paths** set at scaffold time
(per plan 20260518-1530 item 3). No runtime `${...}` substitution.

### Ownership: scaffold-authoritative at `init`, operator-owned afterwards

`gh optivem init` writes the `system-test.paths:` block as the
**authoritative initial value matching the directory tree the same
scaffolder just created**. The scaffolder owns both sides of the join
(YAML + tree), so the eight Family B values are correct by construction
at that moment — they are not "defaults the operator never asked for".

After `init`, the block is operator-owned at every other layer:

- **Validate-time**: `Validate()` in `internal/projectconfig/config.go`
  (Rule 22a) rejects a missing or non-canonical `system-test.paths:`
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
`system-test:`, based on the project's `system-test.path`,
`system-test.lang`, and `sut-namespace` (derived from `system.repo`'s
last segment). For a TypeScript project with
`system-test.path: system-test/typescript` and `sut-namespace: shop`,
the emitted YAML is:

```yaml
system-test:
  path: system-test/typescript
  repo: optivem/shop
  lang: typescript
  sonar-project: optivem_shop-system-test
  paths:
    driver-port: system-test/typescript/src/testkit/driver/port/shop
    driver-adapter: system-test/typescript/src/testkit/driver/adapter/shop
    external-system-driver-port: system-test/typescript/src/testkit/external/port/shop
    external-system-driver-adapter: system-test/typescript/src/testkit/external/adapter/shop
    at-test: system-test/typescript/tests/latest/acceptance
    dsl-port: system-test/typescript/src/testkit/dsl/port/shop
    dsl-core: system-test/typescript/src/testkit/dsl/core/shop
    ct-test: system-test/typescript/tests/latest/contract
```

Java and dotnet defaults differ in stem shape (Java structures tests
by package: `src/test/java/<sut-namespace>/latest/acceptance`; dotnet
uses literal subdirs: `SystemTests/Latest/AcceptanceTests`). See
`pathStems()` in `internal/projectconfig/paths_defaults.go` for the
authoritative per-language stems pinned against the shop template's
`latest/` form. `latest` / `Latest` is doctrine literal — always
present, not project-customizable.

## Validation rules

- **Family B keys that shadow Family A names are rejected.** A typo'd
  `system-test.paths.language: typescript` would otherwise quietly
  override the canonical `system.lang` value. Rejected names today:
  `language`, `architecture`, `system-path`, `system-db-migration-path`,
  `system-test-path`, `sut-namespace`.
- **Non-canonical `system-test.paths.<name>` keys are rejected** (per
  plan 20260518-1530 item 5). The validator enumerates
  `system-test.paths:` keys against `CanonicalPathKeys()` and
  hard-errors on any unknown name. Catches typos and stale keys.
- **`${...}` markers in `system-test.paths:` values are rejected** (per
  plan 20260518-1530 item 5). Under SSoT, paths must be fully resolved
  at scaffold time; runtime substitution is retired.
- **`paths:` on non-system-test tiers is rejected.** The block is
  meaningful only on `system-test`, mirroring how `TierSpec.Config` is
  also system-test-only. A typo'd `system.backend.paths:` would parse
  as a no-op without this rule.

See `internal/atdd/phase_scopes.go` (`CanonicalPathKeys` consumer +
`FamilyAPathKeysInScope`) and the `Validate()` method in
`internal/projectconfig/config.go` for the authoritative rule
implementations.

## Where the per-phase scope comes from

The vocabulary here is consumed by the inline `read:` and `write:` lists
on each writing-agent MID's `EXECUTE_AGENT` call-activity node in
`internal/atdd/runtime/statemachine/process-flow.yaml`, which name the
layers that MID's agent may read and modify. Layer names are joined
with `gh-optivem.yaml system-test.paths:` values at runtime — by the
`check_phase_scope` / `validate-outputs-and-scopes` actions (BPMN
runtime) or the `gh optivem process scope` CLI query — to produce the
per-phase resolved-path set. The accessor is `Engine.Scope(processName)`
on `internal/atdd/runtime/statemachine`.

For the scope rule itself ("only modify paths listed in the phase's
scope; otherwise stop and alert the user"), see
[`shared/scope.md`](shared/scope.md).

## Historical note: pre-SSoT `${...}` substitution

Pre-SSoT, phase docs and agent prompts referenced `${...}`
placeholders (e.g. a Family B path key joined with the
sut-namespace key) that the sync-time tool resolved against a
`PlaceholderMap`. That mechanism is
retired (per plan 20260518-1530's locked decision δ). Phase docs now
reference layer **names** only — no `${...}` syntax — and `paths:`
values are fully resolved at scaffold time. Pre-SSoT projects migrate
via `gh optivem config migrate`, which joins `sut-namespace` into each
`paths:` value, joins it into `system.path`, and deletes
`system.sut-namespace` from the file in one deterministic pass.

## Historical note: top-level `paths:` block

Before plan 20260520-1900, the Family B block sat at the top level of
`gh-optivem.yaml` (`paths:` directly under the document root). The
keys all describe the system-test tier's layered layout, so the block
was relocated under `system-test:` alongside `path`, `repo`, `lang`,
`config`, and `sonar-project` — every other field that describes the
system-test tier. The key set, validation rules, and `PlaceholderMap`
output (still flat — `driver-port`, `at-test`, etc. emit at the top of
the placeholder namespace) are unchanged; only the YAML location moved.
The migrate command continues to operate on the post-move location:
the external-driver key-rename and SSoT-join passes both walk
`system-test.paths:` directly.
