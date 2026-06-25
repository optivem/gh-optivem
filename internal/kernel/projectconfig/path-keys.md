# Path-key vocabulary

This doc describes the canonical **path-key vocabulary** consumed by
`gh-optivem.yaml system-test.paths:` (per-project, user-owned values)
and the inline per-phase scope on writing-agent MID nodes in
`internal/atdd/process/process-flow.yaml` (per-phase scope
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
| `system-path` | `system.path` (the verbatim system code root; no sut-namespace segment baked in) |
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
| `system-driver-port` | system-test driver port (interface) |
| `system-driver-adapter` | system-test driver adapter (implementation) |
| `external-system-driver-port` | external-system driver port (interface) |
| `external-system-driver-adapter` | external-system driver adapter (implementation) |
| `at-test` | acceptance test files |
| `dsl-port` | DSL port (interface) |
| `dsl-core` | DSL core (implementation) |
| `ct-test` | contract test files |
| `system-driver-adapter-shared` | shared System Driver adapter foundation (`driver/adapter/shared/**`) — the test-transport wrappers (Playwright/HTTP clients, base `Client`, shared DTOs, serialization, config) consumed by every channel adapter, the API driver, and external adapters |
| `common` | shared testkit common primitives (`testkit/common/**` — `.NET`: `Common/**`) — the cross-cutting functional types (`Result`, `Converter`, `Closer`, `ResultAssert`) consumed across the whole testkit by test writers, the DSL, and every driver adapter; any testkit-writing agent may extend them in place |
| `domain-value-types` | the system's **domain value types** (`testkit/domainvaluetypes/**` — `.NET`: `DomainValueTypes/**`) — the business vocabulary the system models and tests assert on (today the `OrderStatus` enum; future `Money` / `Sku` / `Quantity` value objects). A sibling of `common` that **travels with it**: universally readable and writable wherever `common` is, so any agent can extend domain vocabulary (e.g. add a new `OrderStatus` value its test references) without a scope halt. **Not** the harness/infra enums `ChannelMode` / `ExternalSystemMode` — those answer *how the test is run*, not *what the system models*, and stay in the already-writable `dsl-port`. **Not** the C# CLR "value type" (`struct`) or JVM value-class language features — those are a runtime memory concept, unrelated to this scope layer. |

Values are **fully-resolved physical paths** set at scaffold time
(per plan 20260518-1530 item 3). No runtime `${...}` substitution.

### Per-channel System Driver adapter members

The System Driver test adapters split physically by **channel folder** —
`driver/adapter/api`, `driver/adapter/ui`, … — each owned by a different
channel team, with the shared/common test-transport foundation living under
`driver/adapter/shared`. The whole-layer `system-driver-adapter` key above
names the adapter root (where broad writers — the DSL stub-writer,
`implement-system`, `update-system` — scope); the `shared` foundation is its
own first-class key `system-driver-adapter-shared` (below). The per-channel
split is carried by a **sibling** block under `system-test:`:

```yaml
system-test:
  channels: [api, ui]
  paths:
    system-driver-adapter: system-test/typescript/src/testkit/driver/adapter
    # …other Family B keys…
  system-driver-adapter-channels:        # sibling of paths:, not a key inside it
    api: system-test/typescript/src/testkit/driver/adapter/api
    ui:  system-test/typescript/src/testkit/driver/adapter/ui
```

Each member is a `channel → fully-resolved adapter path` entry, read
**verbatim** (no runtime path construction). They are the narrow write-scope
and resume footprint for the per-channel `--target driver-adapter --channel
<ch>` slice (plan 20260530-1725) and the per-team ownership boundary.

**Why a sibling, not a key inside `paths:`.** `paths:` is a homogeneous
name→path map; a `channel → path` sub-map cannot live inside it without a
custom (un)marshaler. The members therefore live **outside** the flat
`paths:` map / `CanonicalPathKeys()`. Every consumer that resolves them is
taught about them explicitly — none picks them up for free:

- `PlaceholderMap` emits each as a dotted Family B key
  `system-driver-adapter-channels.<ch>`, so a layer reference can name one.
- `Validate` (Rule 24) ties the members 1:1 to `channels:` (below).
- Preflight (`collectTiers` in `internal/atdd/runtime/preflight`) `os.Stat`s
  each member like every other `paths:` entry, under the field name
  `system-test.system-driver-adapter-channels.<ch>`.
- `driverAdapterFootprint` (`internal/atdd/runtime/driver/scoped.go`) reads
  the member directly as the channel's resume footprint.

**Why explicit, not derived.** Per-language folder *casing* makes a single
`<root>/<channel>` join non-derivable: TS/Java use a lowercase subfolder
(`.../adapter/api`), .NET PascalCases it (`.../Adapter/Api`), and a
team may restructure. Only scaffold-time resolution can carry that shape —
the same resolve-fully-at-`init`, read-verbatim doctrine the other path keys
follow.

### Shared System Driver adapter foundation

`driver/adapter/shared` is the **test-transport foundation** — the Playwright
and HTTP client wrappers, the base `Client` interface, shared DTOs,
serialization, and config — consumed by every channel adapter (UI page
objects, the API driver) *and* by external adapters (an external system with
no API can only be driven via Playwright; HTTP wrappers are shared across api
+ every external adapter). It is placed by architectural layer, not by the
current consumer set.

It is its own first-class, **explicitly stored** Family B key
`system-driver-adapter-shared` — it joins `paths:` like every other layer key,
so a developer can rename or relocate the shared folder (the same
*users-own-values, explicit-only, no-runtime-fallback* doctrine, and the same
reasoning as the `system-driver-adapter-channels` members). Its value is the
whole `driver/adapter/shared` subtree (NOT just `…/shared/client` — the base
`Client`, shared DTOs, serialization and config live directly under `shared/`).

Adapter implementers may **extend** this foundation directly: the four adapter
call-activities (`implement`/`update`-`system-driver-adapters` and their
external variants) carry `system-driver-adapter-shared` in both `read:` and
`write:`. Unlike `system-driver-adapter`, this key is **never narrowed by
channel** — `narrowAdapterScopeByChannel` rewrites only the exact
`system-driver-adapter` entry, so every channel dispatch and the external
dispatch can read and extend the foundation without a scope halt.
`driver/adapter/ui` and `driver/adapter/shared` are disjoint subtrees, so
per-channel ownership and resume footprints (keyed on the channel member) are
unaffected.

> **`external` nests under the driver layer.** The external-system driver
> keys resolve to `driver/port/external` and `driver/adapter/external`
> (`.NET`: `Driver.Port/External`, `Driver.Adapter/External`) — nested under
> the driver layer, not a sibling `external/*` dir — matching the shop
> template (reconciled per plan 20260526-1430). The channel members track
> only the System Driver adapter root and do not split externals by channel.

### Ownership: scaffold-authoritative at `init`, operator-owned afterwards

`gh optivem init` writes the `system-test.paths:` block as the
**authoritative initial value matching the directory tree the same
scaffolder just created**. The scaffolder owns both sides of the join
(YAML + tree), so the eleven Family B values are correct by construction
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

The `system-driver-adapter-channels:` block follows the identical
doctrine: `init` writes one member per `channels:` entry (via
`projectconfig.DefaultSystemDriverAdapterChannels`, called from the same
`BuildOptivemYAML`); `migrate` does **not** back-fill; the operator owns
edits afterwards.

## Default values (TypeScript example)

The scaffolder writes a fully-resolved `paths:` block nested under
`system-test:`, based on the project's `system-test.path` and
`system-test.lang`. The values reproduce the shop template's checked-in
`paths:` block exactly (reconciled per plan 20260526-1430): TypeScript and
dotnet carry no namespace segment; Java carries its source package
(`com/<owner>/<system>`) as a middle segment. For a TypeScript project with
`system-test.path: system-test/typescript`, the emitted YAML is:

```yaml
system-test:
  path: system-test/typescript
  repo: optivem/shop
  lang: typescript
  sonar-project: optivem_shop-system-test
  paths:
    system-driver-port: system-test/typescript/src/testkit/driver/port
    system-driver-adapter: system-test/typescript/src/testkit/driver/adapter
    external-system-driver-port: system-test/typescript/src/testkit/driver/port/external
    external-system-driver-adapter: system-test/typescript/src/testkit/driver/adapter/external
    at-test: system-test/typescript/tests/latest/acceptance
    dsl-port: system-test/typescript/src/testkit/dsl/port
    dsl-core: system-test/typescript/src/testkit/dsl/core
    ct-test: system-test/typescript/tests/latest/contract
    system-driver-adapter-shared: system-test/typescript/src/testkit/driver/adapter/shared
    common: system-test/typescript/src/testkit/common
    domain-value-types: system-test/typescript/src/testkit/domainvaluetypes
  channels: [api, ui]
  system-driver-adapter-channels:
    api: system-test/typescript/src/testkit/driver/adapter/api
    ui: system-test/typescript/src/testkit/driver/adapter/ui
```

Java and dotnet defaults differ in stem shape. Java nests everything under its
source package and structures tests by package
(`src/main/java/com/<owner>/<system>/testkit/…`,
`src/test/java/com/<owner>/<system>/systemtest/latest/acceptance`); dotnet uses
literal project subdirs (`Driver.Adapter`, `SystemTests/Latest/AcceptanceTests`).
The Java package is resolved at `init` from the same owner + system-name the
scaffold's namespace-rename passes use, so the `paths:` block matches the
just-renamed on-disk tree. See `pathStems()` in
`internal/projectconfig/paths_defaults.go` for the authoritative per-language
stems pinned against the shop template's `latest/` form. `latest` / `Latest` is
doctrine literal — always present, not project-customizable.

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
- **`system-driver-adapter-channels:` members are tied 1:1 to
  `channels:`** (Rule 24). Every member must name a declared channel (a
  casing slip like `Api` gets a did-you-mean); every member value is
  fully-resolved and repo-relative (same as a `paths:` entry); and once
  `system.architecture` is set, every declared channel must have a member
  (derive is rejected, so an unbacked channel has no resolvable adapter
  path). The block is also system-test-only — rejected on
  backend/frontend, same as `paths:`.

See `internal/atdd/phase_scopes.go` (`CanonicalPathKeys` consumer +
`FamilyAPathKeysInScope`) and the `Validate()` method in
`internal/projectconfig/config.go` for the authoritative rule
implementations.

## Where the per-phase scope comes from

The vocabulary here is consumed by the inline `read:` and `write:` lists
on each writing-agent MID's `EXECUTE_AGENT` call-activity node in
`internal/atdd/process/process-flow.yaml`, which name the
layers that MID's agent may read and modify. Layer names are joined
with `gh-optivem.yaml system-test.paths:` values at runtime — by the
`check_phase_scope` / `validate-outputs-and-scopes` actions (BPMN
runtime) or the `gh optivem process scope` CLI query — to produce the
per-phase resolved-path set. The accessor is `Engine.Scope(processName)`
on `internal/engine/statemachine`.

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
values are fully resolved at scaffold time. `system.path`, however, is
the verbatim system code root: no sut-namespace segment is baked into
it. `gh optivem config migrate` does **not** reintroduce one — it only
back-fills `system.db-migration-path` when absent (see
`config_commands.go`); it never joins `sut-namespace` into `system.path`
or any `paths:` value.

## Historical note: top-level `paths:` block

Before plan 20260520-1900, the Family B block sat at the top level of
`gh-optivem.yaml` (`paths:` directly under the document root). The
keys all describe the system-test tier's layered layout, so the block
was relocated under `system-test:` alongside `path`, `repo`, `lang`,
`config`, and `sonar-project` — every other field that describes the
system-test tier. The key set, validation rules, and `PlaceholderMap`
output (still flat — `system-driver-port`, `at-test`, etc. emit at the top of
the placeholder namespace) are unchanged; only the YAML location moved.
The migrate command continues to operate on the post-move location:
the external-driver key-rename and SSoT-join passes both walk
`system-test.paths:` directly.
