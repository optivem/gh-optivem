# Plan: Substitute path placeholders in phase docs from `gh-optivem.yaml`

## Background

The ATDD phase docs under `internal/assets/global/docs/atdd/process/` ship to
scaffolded repos via `assets/sync` (today, a verbatim copy to
`~/.gh-optivem/docs/`). They reference filesystem paths that vary per project:

- The language (`typescript` / `java` / `dotnet`) appears in
  `system-test/<lang>/...`, `system/monolith/<lang>/...`, etc.
- The SUT-internal driver namespace (currently `shop/` per `glossary.md`
  doctrine) may be scaffolded as a different name (the rehearsal at
  `C:/GitHub/optivem/academy/rehearsal-20260515-095931` uses `myShop/`).

Today the docs paper over this with literal placeholders like
`system-test/<lang>/.../testkit/driver/...` (added in the 2026-05-15 doc-path
fix for `at-red-system-driver.md` and `ct-red-external-driver.md`). That
literal makes sense **in this source repo** because the docs are templates,
but in a scaffolded repo the reader sees `<lang>` verbatim and has to mentally
substitute it — which is a regression from the `gh-optivem.yaml` config that
*already knows* `system.lang = typescript`.

The agent-prompt rendering pipeline already solves the same problem the right
way: `clauderun.renderPrompt` calls `statemachine.ExpandParams` against a
fixed-schema map keyed by `language`, `architecture`, `subtype`,
`docs_root`, etc., sourced from `gh-optivem.yaml`. Phase docs should
participate in the same substitution.

This plan introduces config-driven path substitution into the phase-doc
pipeline so the docs land in scaffolded repos with concrete paths, while
the source-repo template uses `${name}` placeholders identical to the agent-
prompt convention.

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

3. **Source-repo docs use `${name}`, never `<name>`.** The 2026-05-15 fix
   added `<lang>` literals — those are stop-gap and will become `${language}`
   under this plan. `<angle-brackets>` survive only for genuinely free-form
   placeholders (e.g. `<channel>` = `api` / `ui` / `cli`, where the SUT may
   have multiple), where the reader IS expected to pick.

## Placeholder vocabulary (initial set)

Sourced from `projectconfig.Config` (parsed from `gh-optivem.yaml`):

| Placeholder | Config field | Example |
|---|---|---|
| `${language}` | `system.lang` (or `system_test.lang` when the agent is test-side) | `typescript`, `java` |
| `${system_path}` | `system.path` | `system/monolith/typescript` |
| `${system_test_path}` | `system_test.path` | `system-test/typescript` |
| `${sut_namespace}` | new field (default `shop`, overridable) | `shop`, `myShop` |
| `${architecture}` | `system.architecture` | `monolith`, `multitier` |

The `${sut_namespace}` field is new: per the rehearsal observation, the
scaffolder sometimes produces a name other than `shop/` (TS uses `myShop`,
Java may use `com.mycompany.myshop`). Today this is implicit — the plan adds
an explicit `system.sut_namespace` (or similar) to `gh-optivem.yaml` so docs
can render the correct folder name. Default is `shop` for backward
compatibility with the glossary.

## Affected docs (initial scope)

Any phase doc that mentions a path. Concrete starting list (Grep
`internal/assets/global/docs/atdd/process/*.md`):

- `at-red-system-driver.md` — current `<lang>` and `shop/` literals → `${language}`, `${sut_namespace}`
- `ct-red-external-driver.md` — current `<lang>` literals
- `at-green-system.md` — `shop/api/.../CustomerController.java`,
  `shop/ui/.../register-customer.page.tsx` examples (system code, not test)
- `cycles.md` — table cell `System Drivers only (`shop/`)`
- `system-interface-redesign.md` — `driver-port/.../shop/<channel>` paths
- `glossary.md` — the `shop/` doctrine paragraph itself needs a sentence
  noting that the literal folder name comes from `${sut_namespace}` in
  `gh-optivem.yaml`

## Implementation steps

### Step 1 — Add `sut_namespace` to project config

- New field `system.sut_namespace` in `gh-optivem.yaml` schema (default
  `shop` if absent — keeps every existing config working unchanged).
- Wire it through `projectconfig.Config` and into the placeholder map.

### Step 2 — Extend sync to substitute

- Refactor `internal/assets/sync/sync.go` so the docs-copy step optionally
  takes a placeholder map (built from `projectconfig.Config`).
- For each `.md` file under `internal/assets/global/docs/atdd/`, run
  `statemachine.ExpandParams` (the existing helper) before writing to the
  destination.
- Verify `findUnfilledPlaceholders` (lifted out of `clauderun.go` to a
  shared package, or duplicated for now) catches typo'd placeholders at
  sync time, the same way it catches them at agent dispatch time.

### Step 3 — Convert doc literals to `${name}`

Each affected doc replaces:
- `<lang>` → `${language}`
- `shop/` (where it's the SUT driver namespace) → `${sut_namespace}/`
- Other path fragments as the vocabulary grows.

Do NOT replace `<channel>` — that one is genuinely free-form and the
reader picks from `api` / `ui` / `cli` / etc.

### Step 4 — Test against rehearsal

- Run `gh optivem sync` (or whichever command publishes docs) against the
  TS rehearsal at `C:/GitHub/optivem/academy/rehearsal-20260515-095931`.
- Open `~/.gh-optivem/docs/atdd/process/at-red-system-driver.md` and
  confirm `${language}` has expanded to `typescript` and `${sut_namespace}`
  to `myShop` (or whatever the rehearsal's value is).
- Re-run the AT-RED-TEST cycle and confirm the agent's reads of the doc
  return concrete paths.

### Step 5 — Document the convention

- Add a section to `docs/atdd/architecture/` (or wherever doc/template
  conventions live) capturing: "phase docs use `${name}` placeholders
  identical to agent prompts; sync expands them; here is the vocabulary."
- Cross-reference from `glossary.md` so the `shop/` doctrine paragraph
  stays correct.

## Open questions

- **Q1.** Should `${sut_namespace}` be auto-derived from `system.repo` (e.g.
  `optivem/shop` → `shop`) when not explicitly set? Default-from-repo
  reduces config bloat but couples two concerns.

- **Q2.** Is sync-time substitution the right layer, or should `assets/sync`
  stay a pure file-copy and the substitution live in a new
  `assets/render` step? Today the agent prompts substitute at *dispatch*
  time, not sync time — a docs render would be the asymmetric one. Trade-off:
  sync-time is simpler and works for the common case (docs are static once
  the project is scaffolded); dispatch-time is more flexible if a doc
  placeholder ever needs per-run context (unlikely for paths, but worth
  noting).

- **Q3.** Where does `${sut_namespace}` come from for repos that have no
  driver code yet (e.g. brand-new scaffolds before AT-RED-DSL has ever run)?
  Likely the scaffolder writes the namespace before any cycle runs, so this
  is moot — but verify before assuming.

## Non-goals

- Replacing the doctrinal `shop/` literal in `glossary.md`'s explanatory
  paragraph itself. That paragraph TEACHES the convention and uses `shop`
  as the canonical example, the way a Python tutorial uses `mymodule`.
  Substitute it in PATHS, not in PROSE.

- Substituting placeholders in agent body files (`internal/assets/runtime/
  prompts/atdd/*.md`). Those already go through `renderPrompt`'s
  `ExpandParams`, and the agent dispatch is the right place for that pass.

## Cross-references

- 2026-05-15 conversation: doc-path fix that introduced the `<lang>`
  literal in `at-red-system-driver.md` and `ct-red-external-driver.md`.
  Those literals are the immediate motivation for this plan.
- Agent-prompt substitution convention: `clauderun.renderPrompt`
  (`internal/atdd/runtime/clauderun/clauderun.go`), specifically the
  `params` map built around line 418.
- Existing `ExpandParams` helper:
  `internal/atdd/runtime/statemachine/` (referenced in `clauderun.go`).
