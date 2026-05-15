# Plan: Config-driven path substitution in phase docs

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

### Output — `./.gh-optivem/docs/atdd/process/at-red-system-driver.md` after project-local materialize

(Decision 2026-05-15: substituted docs go to a project-local `.gh-optivem/docs/`
directory inside the project tree, not into the user-global `~/.gh-optivem/docs/`.
The user-global location is shared across every project on the machine and must
not carry per-project substitutions.)

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
   already-substituted text once into `./.gh-optivem/docs/` and the agents
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

## Remaining sub-items deferred from this session

- ⏳ Wire `MaterializeProject` into `clauderun.Dispatch`: today
  `assetsync.MaterializeProject` exists and works, but `clauderun.renderPrompt`
  still resolves `${docs_root}` via `assetsync.DocsRoot()` (the user-global
  path). Until this lands, no `gh optivem` command path actually triggers
  project-local materialization. Wire-through:
  1. `Dispatch` calls `MaterializeProject(opts.RepoPath, version,
     cfg.PlaceholderMap())` before `renderPrompt`.
  2. `renderPrompt` substitutes `${docs_root}` with `ProjectDocsRoot(opts.RepoPath)`
     when `opts.RepoPath` is non-empty.
  3. `Options` gains an explicit `ProjectConfig *projectconfig.Config`
     field (or `Dispatch` accepts it as a separate argument) so the
     placeholder map is at hand without re-loading.
- ⏳ Migration helper: extend `runConfigMigrate` to back-fill the default
  `paths:` block when missing. Today's scaffolder seeds it, but pre-existing
  configs need the migrate path before Step 4 verification can be exercised
  against them. Scaffolder default-block authoring depends on this.
- ⏳ Scaffolder default `paths:` block. Decide per-language defaults
  (TypeScript flat-src, Java Maven layout, .NET solution layout) and
  thread them through `internal/configinit/configinit.go` so newly
  scaffolded projects ship with a non-empty `paths:` block matching the
  glossary doctrine for their layout.
- ⏳ Step 4 — End-to-end verification against a rehearsal. Deferred:
  blocked on the three items above (no caller wired in, no migration
  helper, no scaffolder defaults). The originally-named rehearsal at
  `C:/GitHub/optivem/academy/rehearsal-20260515-095931` no longer exists;
  the next agent should scaffold a fresh TS rehearsal once the wire-up
  lands. Verification list once unblocked:
  - Run `gh optivem` against a TS rehearsal (any command that triggers
    project-local sync — e.g. an ATDD-running command).
  - Open `./.gh-optivem/docs/atdd/process/at-red-system-driver.md` inside
    the rehearsal and confirm `${language}` → `typescript`,
    `${sut_namespace}` → `myShop`, `${driver_port}` → the concrete path
    declared in the rehearsal's `paths:` block. (Note: location changed
    from the original `~/.gh-optivem/docs/...` — user-global location
    MUST NOT be substituted because it's shared across projects.)
  - Confirm the per-file `substituted:` audit block lists only keys that
    appeared in that file's body.
  - Confirm `./.gh-optivem/.materialized.yaml` sidecar exists and matches
    the current placeholder map; touch `gh-optivem.yaml` with no semantic
    change and re-run — verify materialization is skipped.
  - Re-run the AT-RED-TEST cycle and confirm the agent's reads of the doc
    return concrete paths.
  - Confirm a `paths.*` key that resolves to a non-existent directory
    does NOT block (the plan's warning rule is deferred — currently the
    substitution layer doesn't FS-check `paths.*` values, since
    validatePath already rejects malformed values at config load).

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
