# Plan: Config-driven path substitution in phase docs

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-15T13:29:14Z`

## Background

The ATDD phase docs under `internal/assets/global/docs/atdd/process/` ship to
scaffolded repos via `assets/sync` (today, a verbatim copy to
`~/.gh-optivem/docs/`). They reference filesystem paths that vary per project
across two axes:

1. **Config-knowable** values — the language (`typescript`/`java`/`dotnet`),
   the system path (`system/monolith/typescript`), the SUT-internal driver
   namespace (`shop/` per `glossary.md` doctrine; rehearsal at
   `C:/GitHub/optivem/academy/rehearsal-20260515-095931` uses `myShop/`).
   These already live (or can live) as fixed-schema fields in
   `gh-optivem.yaml`.

2. **Named locations within the project tree** — path tails like
   `.../testkit/driver/port/`, `.../testkit/driver/adapter/`. Every ATDD
   project has a driver port and a driver adapter (the *name* is stable),
   but where the user puts them is not. Commit `c88f205` ("atdd-process-docs:
   correct Driver paths (`testkit/driver/{port,adapter}`)") is direct
   evidence that even THIS repo has changed its mind about the layout.
   Plausible layouts seen in the wild:
   - `system-test/typescript/src/testkit/driver/port/`
   - `system-test/typescript/src/support/drivers/in/`
   - `system-test/typescript/src/framework/driver/port/`

Today the docs paper over both axes with literal placeholders like
`system-test/<lang>/.../testkit/driver/...` (added in the 2026-05-15 doc-path
fix for `at-red-system-driver.md` and `ct-red-external-driver.md`). That
literal makes sense **in this source repo** because the docs are templates,
but in a scaffolded repo the reader sees `<lang>` verbatim and has to mentally
substitute it — a regression from the `gh-optivem.yaml` config that *already
knows* `system.lang = typescript`.

The agent-prompt rendering pipeline already solves the same problem the right
way: `clauderun.renderPrompt` calls `statemachine.ExpandParams` against a
fixed-schema map keyed by `language`, `architecture`, `subtype`, `docs_root`,
etc., sourced from `gh-optivem.yaml`. Phase docs should participate in the
same substitution, with two placeholder sources: fixed-schema config fields
(axis 1) and a top-level `paths:` block (axis 2) the user owns.

> **Consolidation note.** This plan was consolidated from
> `20260515-1037-phase-doc-path-substitution-from-config.md` (fixed-schema
> substitution) and `20260515-1100-phase-doc-driver-concept-references.md`
> (named-locations block) on 2026-05-15. Both plans shared the same
> substitution mechanism and affected file set; merging avoids touching
> every doc twice.

## End-result example

To make the shape concrete: here's what the inputs and outputs look like
end-to-end for a TypeScript monolith scaffold (matching the rehearsal at
`C:/GitHub/optivem/academy/rehearsal-20260515-095931`).

### Input — `gh-optivem.yaml` in the scaffolded repo

```yaml
system:
  repo: optivem/myShop           # ${sut_namespace} auto-derives to "myShop"
  lang: typescript
  architecture: monolith
  path: system/monolith/typescript
  # sut_namespace: myShop         # optional override; defaults to last segment of system.repo
system_test:
  lang: typescript
  path: system-test/typescript

paths:                            # NEW required block; scaffolder ships defaults
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
  external_driver_port: system-test/typescript/src/testkit/external/port
  external_driver_adapter: system-test/typescript/src/testkit/external/adapter
```

### Source-repo template — `internal/assets/global/docs/atdd/process/at-red-system-driver.md`

Lives in this repo, uses `${name}` placeholders identical to agent-prompt
convention:

```markdown
The system-test driver port lives at:
  ${driver_port}/${sut_namespace}/

The SUT-side driver implementation lives at:
  ${system_path}/.../${sut_namespace}/<channel>/

For ${language} projects, prefer the per-channel adapter pattern.
```

Note `<channel>` stays in angle-brackets — it's free-form (`api`/`ui`/`cli`)
and the reader picks. Only `${...}` get substituted.

### Output — `~/.gh-optivem/docs/atdd/process/at-red-system-driver.md` after `gh optivem sync`

What an agent in the scaffolded repo Reads:

```markdown
---
substituted:
  language: typescript
  sut_namespace: myShop
  system_path: system/monolith/typescript
  driver_port: system-test/typescript/src/testkit/driver/port
---

The system-test driver port lives at:
  system-test/typescript/src/testkit/driver/port/myShop/

The SUT-side driver implementation lives at:
  system/monolith/typescript/.../myShop/<channel>/

For typescript projects, prefer the per-channel adapter pattern.
```

The agent reads concrete paths and references `myShop/` directly — no
mental substitution, no risk of emitting `${language}` into a tool call.
The frontmatter `substituted:` block is a ~20-token audit trail; the body
is the source of truth.

When the user later renames `testkit` to `support`, they update `paths:`
once, re-sync, and every phase doc that referred to `${driver_port}`
reflects the new layout. No doc edits needed.

## Principles

1. **One placeholder vocabulary across prompts and docs.** Agent prompts
   already use `${language}`, `${docs_root}`, `${architecture}`, `${subtype}`.
   Phase docs use the same syntax and the same source — `projectconfig.Config`
   keyed off `gh-optivem.yaml` — so a learner who understands the prompt
   substitution understands the doc substitution.

2. **Substitution happens at sync time, not at read time.** Agents Read the
   docs many times per run; per-Read substitution is wasted work. Sync the
   already-substituted text once into `~/.gh-optivem/docs/` and the agents
   Read concrete paths thereafter.

   Corollary: substitution is **inline in the body**, not declared in
   frontmatter and resolved on read. A frontmatter-only scheme (e.g.
   `language: typescript` declared once, body keeps `${language}`) does not
   shrink the body — it just adds an overhead block on top — and forces every
   Read to spend reasoning tokens on lookup, with the bonus failure mode of
   the agent emitting `${language}` verbatim into tool calls. Inline
   substitution pays the cost once at sync time and is read-cheap thereafter.

   A *tiny* frontmatter `substituted:` audit block IS worth adding (see
   Step 2) — but as a debuggability aid, not as the substitution mechanism.

3. **Source-repo docs use `${name}`, never `<name>`.** The 2026-05-15 fix
   added `<lang>` literals — those are stop-gap and will become `${language}`
   under this plan. `<angle-brackets>` survive only for genuinely free-form
   placeholders (e.g. `<channel>` = `api`/`ui`/`cli`, where the SUT may
   have multiple), where the reader IS expected to pick.

4. **Two placeholder sources, one mechanism.** Fixed-schema names
   (`${language}`, `${sut_namespace}`) come from existing or new top-level
   fields in `gh-optivem.yaml`. Named-location names (`${driver_port}`,
   `${driver_adapter}`) come from a top-level `paths:` map the scaffolder
   materialises and the user owns. Both flow through the same placeholder
   map and the same substitution call.

## Placeholder vocabulary

Sourced from `projectconfig.Config` (parsed from `gh-optivem.yaml`). Two
families, one flat namespace:

### Family A — fixed-schema (lifted from existing/new top-level fields)

| Placeholder | Config field | Example |
|---|---|---|
| `${language}` | `system.lang` (or `system_test.lang` when the agent is test-side) | `typescript`, `java` |
| `${system_path}` | `system.path` | `system/monolith/typescript` |
| `${system_test_path}` | `system_test.path` | `system-test/typescript` |
| `${sut_namespace}` | auto-derived from last segment of `system.repo`; overridable via `system.sut_namespace` | `shop`, `myShop` |
| `${architecture}` | `system.architecture` | `monolith`, `multitier` |

`${sut_namespace}` defaults to the last path segment of `system.repo`
(`optivem/myShop` → `myShop`). If a project wants a different value than
the repo name (rare — Java may use `com.mycompany.myshop` for the package
even though the repo is `myShop`), it can set `system.sut_namespace`
explicitly. Today the value is implicit; this plan makes it explicit and
substitutable.

### Family B — named locations (under top-level `paths:`)

Required top-level `paths:` block in `gh-optivem.yaml`. Each key is a
named location; each value is a path relative to the repo root. The
scaffolder writes per-architecture defaults into every new project so the
block is never empty.

Initial keys, anchored against `glossary.md`:

- `driver_port` — system-test driver-port interface package
- `driver_adapter` — system-test driver-adapter implementations (per-channel)
- `external_driver_port` — CT external driver-port
- `external_driver_adapter` — CT external driver-adapter
- `system_test_root` — top of system-test tree, for "create a file under here"
  instructions where the doc should not pre-commit to a sub-folder
- `sut_root` — top of SUT tree
- (extend as the glossary grows)

Scaffolder-materialised defaults match `glossary.md` doctrine. Users edit
`paths:` (not the docs) when they reorganise their tree.

## Affected docs (initial scope)

Any phase doc that mentions a path. Concrete starting list (Grep
`internal/assets/global/docs/atdd/process/*.md`):

- `at-red-system-driver.md` — `<lang>` and `shop/` literals + `testkit/driver/...`
- `ct-red-external-driver.md` — `<lang>` literals + `testkit/external/...`
- `at-green-system.md` — `shop/api/.../CustomerController.java`,
  `shop/ui/.../register-customer.page.tsx` examples (system code, not test)
- `cycles.md` — table cell `System Drivers only (`shop/`)`
- `system-interface-redesign.md` — `driver-port/.../shop/<channel>` paths
- `glossary.md` — the `shop/` doctrine paragraph needs a sentence noting
  that the literal folder name comes from `${sut_namespace}` and that
  driver layout fragments come from named-location placeholders under
  `paths:` (both from `gh-optivem.yaml`)

## Design decisions

Decided 2026-05-15 (the three open questions that shaped the schema):

- **D1 (sut_namespace default).** Auto-derive from the last path segment of
  `system.repo` (e.g. `optivem/myShop` → `myShop`). User overrides by
  setting `system.sut_namespace` explicitly when the repo name and the
  desired namespace diverge.

- **D2 (placeholder syntax).** Flat `${driver_port}` — consistent with
  `${language}`, `${sut_namespace}`, etc., so learners only ever see one
  syntax. Sync-time error if a `paths.*` key shadows a Family A key.

- **D3 (paths: block).** Required in schema. Scaffolder materialises a
  default block matching `glossary.md` doctrine into every new
  `gh-optivem.yaml`. An existing config without the block is invalid —
  migration handled in Step 1.

- **D4 (block name).** Top-level `paths:`, not `concepts:` / `layers:` /
  `landmarks:` — most direct (values literally are paths), zero metaphor.

Carried over from source plans (already settled before merge):

- **Sync-time vs. read-time substitution** → sync-time, inline body
  substitution, with frontmatter `substituted:` audit block. Reads are hot
  (every cycle re-reads the relevant phase doc multiple times); a
  dispatch-time or read-time scheme would re-pay substitution cost on every
  Read and risks the agent emitting `${language}` literally.

- **Discovery vs. declaration of paths** → declaration. Auto-discovering by
  scanning for files matching a marker is fragile (renamed interface =
  broken docs); the user already owns their layout.

- **Multi-tree / multi-language projects** → out of scope for v1.

- **`${sut_namespace}` for pre-AT-RED-DSL repos** → moot; scaffolder writes
  the value before any cycle runs. Verify during Step 4.

## Implementation steps

### Step 1 — Schema additions to project config

- New optional field `system.sut_namespace` in `gh-optivem.yaml` schema. When
  absent, derive from the last path segment of `system.repo` (per D1).
- New **required** top-level `paths:` map. Each key is a named location; each
  value is a path relative to the repo root. The scaffolder writes a default
  block (matching `glossary.md` doctrine) into every new project so the
  block is never empty in practice (per D3).
- Wire both through `projectconfig.Config` so the placeholder map flattens to
  a single flat `name → value` lookup (per D2: Family A keys + Family B keys
  in one namespace).
- Sync-time error if a `paths.*` key shadows a Family A key.
- Migration helper: `gh optivem config migrate` reads an existing
  `gh-optivem.yaml` missing `paths:` and writes the default block in place
  (so we don't break any pre-existing scaffold).

### Step 2 — Extend sync to substitute

- Refactor `internal/assets/sync/sync.go` so the docs-copy step takes the
  flat placeholder map built from `projectconfig.Config`.
- For each `.md` file under `internal/assets/global/docs/atdd/`, run
  `statemachine.ExpandParams` (the existing helper, lifted to a shared
  package if necessary) **inline against the body text** before writing to
  the destination. Substitution is in the body itself, not a frontmatter
  declaration the agent has to resolve at read time.
- Prepend a small audit-trail frontmatter block to each substituted file:

  ```yaml
  ---
  substituted:
    language: typescript
    sut_namespace: myShop
    driver_port: system-test/typescript/src/testkit/driver/port
  ---
  ```

  Only the keys that actually appeared in the body are listed (don't dump
  the whole placeholder map). ~20 tokens for debuggability. Body remains the
  source of truth.
- Lift `findUnfilledPlaceholders` out of `clauderun.go` into a shared package
  and use it at sync time. Error message reports the declared vocabulary
  (`unknown placeholder ${driver_prt}; declared: driver_port, driver_adapter,
  language, ...`).
- Warning (not error) when a `paths.*` entry resolves to a directory that
  doesn't exist on disk in the scaffolded repo.

### Step 3 — Convert doc literals to `${name}` (single pass per doc)

For each affected doc, replace in one edit:
- `<lang>` → `${language}`
- `shop/` (where it's the SUT driver namespace) → `${sut_namespace}/`
- `testkit/driver/port` → `${driver_port}` (or appropriate fragment)
- `testkit/driver/adapter` → `${driver_adapter}`
- `testkit/external/port` → `${external_driver_port}`
- `testkit/external/adapter` → `${external_driver_adapter}`

Do NOT replace `<channel>` — that one is genuinely free-form (`api`/`ui`/
`cli`).

Special handling for `glossary.md`: keep the doctrinal `shop/` literal in
the explanatory paragraph (it TEACHES the convention; substituting it in
prose would be confusing). Add a sentence noting that the literal folder
name in code paths comes from `${sut_namespace}` and driver layout
fragments come from named-location placeholders under `paths:`.
Substitute in PATHS, not in PROSE.

### Step 4 — Test against rehearsal

- Run `gh optivem sync` against the TS rehearsal at
  `C:/GitHub/optivem/academy/rehearsal-20260515-095931`.
- Open `~/.gh-optivem/docs/atdd/process/at-red-system-driver.md` and confirm
  `${language}` → `typescript`, `${sut_namespace}` → `myShop`,
  `${driver_port}` → the concrete path declared in the rehearsal's config.
- Re-run the AT-RED-TEST cycle and confirm the agent's reads of the doc
  return concrete paths.
- Confirm the warning fires (not errors) when a `paths.*` entry resolves
  to a directory that doesn't exist on disk yet.

### Step 5 — Document the convention

- Add a section to `docs/atdd/architecture/` (or wherever doc/template
  conventions live) capturing:
  - Phase docs use `${name}` placeholders identical to agent prompts.
  - Sync expands them inline; reads return concrete paths.
  - Two placeholder families, one flat namespace: fixed-schema
    (Family A — `${language}`, etc., lifted from top-level config fields)
    and named locations (Family B — `${driver_port}` etc., under `paths:`).
  - Users edit `paths:` (not the docs) when they reorganise their tree.
- Cross-reference from `glossary.md` so the `shop/` doctrine paragraph and
  the new placeholder-vocabulary note stay in sync.

## Non-goals

- Replacing the doctrinal `shop/` literal in `glossary.md`'s explanatory
  paragraph itself. That paragraph TEACHES the convention and uses `shop`
  as the canonical example, the way a Python tutorial uses `mymodule`.
  Substitute it in PATHS, not in PROSE.

- Substituting placeholders in agent body files (`internal/assets/runtime/
  prompts/atdd/*.md`). Those already go through `renderPrompt`'s
  `ExpandParams`, and the agent dispatch is the right place for that pass.

- Auto-discovering `paths:` entries by scanning the file tree.

- Refactoring `glossary.md` to remove the canonical location names — the
  `paths:` keys in this plan must MATCH the glossary, not the other way
  around.

- Multi-language / multi-tree projects (multiple system-test trees with
  per-language driver ports). Revisit when a use case arises.

## Cross-references

- 2026-05-15 conversation: doc-path fix that introduced the `<lang>`
  literal in `at-red-system-driver.md` and `ct-red-external-driver.md`.
  Those literals are the immediate motivation for Family A.
- Commit `c88f205` (2026-05-15) "atdd-process-docs: correct Driver paths
  (`testkit/driver/{port,adapter}`)" — concrete evidence that hardcoded
  driver paths churn even within this repo, motivating Family B
  (named-location indirection via `paths:`).
- Agent-prompt substitution convention: `clauderun.renderPrompt`
  (`internal/atdd/runtime/clauderun/clauderun.go`), specifically the
  `params` map built around line 418.
- Existing `ExpandParams` helper:
  `internal/atdd/runtime/statemachine/` (referenced in `clauderun.go`).
- `internal/assets/global/docs/atdd/process/glossary.md` — canonical
  location-name vocabulary; Family B keys (`paths:`) must align.
