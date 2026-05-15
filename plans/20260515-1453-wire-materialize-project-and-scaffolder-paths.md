# Plan: Wire MaterializeProject into Dispatch + scaffolder `paths:` defaults

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-15T15:23:32Z`

## Background

The phase-doc substitution feature is half-shipped. Earlier work (closed
out in commit `7d2c8d2`, design context preserved at `git show
76f9b1b:plans/20260515-1037-phase-doc-path-substitution-from-config.md`)
landed:

- `internal/assets/sync/materialize.go` — `MaterializeProject` substitutes
  embedded ATDD docs against a placeholder map and writes them to
  `./.gh-optivem/docs/` inside a project tree, with a sidecar at
  `./.gh-optivem/.materialized.yaml` for idempotent skip.
- `projectconfig.Config.Paths` — a top-level `paths:` map on the config
  schema (`internal/projectconfig/config.go:186`).
- `projectconfig.Config.PlaceholderMap()` — builds the flat
  `${name}` → value map mixing fixed-schema fields (Family A:
  `language`, `sut_namespace`, `system_path`, …) with `paths:` entries
  (Family B: `driver_port`, `driver_adapter`, …).
- Doc edits — the embedded `internal/assets/global/docs/atdd/process/*.md`
  files now use `${name}` placeholders.

What is missing for the feature to actually work end-to-end:

1. **No caller of `MaterializeProject`.** A `grep` across the repo finds
   zero production callers — `clauderun.Dispatch` (the agent-launch path)
   still renders prompts via `renderPrompt`, which resolves `${docs_root}`
   against the user-global `assetsync.DocsRoot()`. Nothing triggers
   project-local materialization.

2. **No `paths:` defaults shipped by the scaffolder.** A freshly
   scaffolded project has no `paths:` block, so `PlaceholderMap()`
   returns a map without Family B keys, and any doc referencing
   `${driver_port}` fails substitution (`substituteDoc` errors on
   unfilled placeholders).

3. **No migration for pre-existing configs.** `runConfigMigrate`
   (`internal/cmd/.../config_commands.go:307`) handles other back-fills
   (e.g. repos for multi-repo monoliths, GitHub provider, markdown
   provider) but does not add `paths:`.

4. **No end-to-end verification yet.** The original plan's Step 4 was
   deferred because items 1–3 above had to land first.

This plan covers all four.

## Scope

### Item 1 — Wire `MaterializeProject` into `clauderun.Dispatch`

**Files**: `internal/atdd/runtime/clauderun/clauderun.go`, plus its tests.

**Spec** (carried forward from the design plan):

1. `Options` (line 48) gains a `ProjectConfig *projectconfig.Config`
   field so the placeholder map is at hand without re-loading. Discuss
   during execution whether to also keep `RepoPath` separate (already
   exists?) or derive it from `ProjectConfig` — read the struct first
   and minimize churn.
2. `Dispatch` (line 292), before invoking `renderPrompt`, calls
   `assetsync.MaterializeProject(opts.RepoPath, version, opts.ProjectConfig.PlaceholderMap())`
   when `opts.RepoPath` and `opts.ProjectConfig` are both non-empty.
   Surfaces the returned `projectDocsRoot` to `renderPrompt`.
3. `renderPrompt` (line 401) substitutes `${docs_root}` with the
   project-local `projectDocsRoot` when present, falling back to the
   user-global `assetsync.DocsRoot()` for callers that don't supply a
   `ProjectConfig` (CLI utilities, scaffold flows).

**Acceptance signal**:

- Unit test: a `Dispatch` invocation with a populated `Options.ProjectConfig`
  and a temp-dir `RepoPath` materializes docs into `<RepoPath>/.gh-optivem/docs/`
  and the rendered prompt references that path.
- Unit test: a `Dispatch` invocation without `ProjectConfig` falls back
  to the user-global docs path (regression guard for non-ATDD callers).

### Item 2 — Per-language scaffolder default `paths:` block

**Files**: `internal/configinit/configinit.go` (and any sibling file that
owns the YAML template assembly — read first), plus tests.

**Spec**:

- For each `(language, architecture)` pair the scaffolder supports
  (`typescript-monolith`, `java-monolith`, `dotnet-monolith`, plus any
  multitier variants present), produce a default `paths:` map matching
  `internal/assets/global/docs/atdd/process/glossary.md` doctrine. Read
  the glossary first so the keys align — the plan-closeout commit's
  parent (`git log -- internal/assets/global/docs/atdd/process/glossary.md`)
  may have updated vocabulary.
- Initial keys per the design plan: `driver_port`, `driver_adapter`,
  `external_driver_port`, `external_driver_adapter`, `system_test_root`,
  `sut_root`. Confirm against `glossary.md`.
- Per-language path stems (suggested defaults to confirm against the
  glossary):
  - TypeScript flat-src: `system-test/typescript/src/testkit/driver/port`
    etc., `system/monolith/typescript/src/<sut_namespace>` for the SUT.
  - Java Maven: `system-test/java/src/main/java/.../testkit/driver/port`
    etc.
  - .NET solution: `system-test/dotnet/<solution>/Testkit.Driver.Port/`
    etc.
- The defaults are written into `gh-optivem.yaml` at scaffold time. They
  should not be regenerated on every command — once on initial scaffold,
  the user owns subsequent edits.

**Acceptance signal**:

- Unit test per language: scaffolding emits a `gh-optivem.yaml` whose
  `paths:` block is non-empty and contains the documented keys.
- Smoke check (manual or fixture-driven): the defaults make
  `MaterializeProject` succeed against a `gh-optivem.yaml` shipped from
  the scaffolder with no further user edits.

### Item 3 — Migration helper back-fills `paths:`

**Files**: `internal/cmd/.../config_commands.go` (around
`runConfigMigrate`, line 307), plus tests in `config_commands_test.go`.

**Spec**:

- Extend `runConfigMigrate` to detect a missing or empty `paths:` block
  and back-fill it with the per-language defaults from Item 2 (factor
  the defaults into a single shared helper so the scaffolder and the
  migrator agree by construction).
- Idempotent: re-running `gh optivem config migrate` after a back-fill
  is a no-op (`changed == false`).
- Existing partial `paths:` blocks: leave user keys alone, fill in only
  missing canonical keys. Document the merge rule in a code comment.

**Acceptance signal**:

- `TestRunConfigMigrate_BackfillsPathsForMonolith` (parallel to the
  existing `TestRunConfigMigrate_BackfillsReposForMultiRepoMonolith`)
  asserts the block is added with correct per-language defaults.
- `TestRunConfigMigrate_IsIdempotent` (existing) continues to pass.
- New: idempotence specifically for `paths:` — a second migrate run
  after a back-fill returns `changed == false`.

### Item 4 — End-to-end verification against a fresh rehearsal

**Files**: none in this repo. Produces an audit note.

**Spec**:

- Scaffold a fresh TypeScript monolith rehearsal under
  `C:/GitHub/optivem/academy/` via the normal `gh optivem` scaffold flow.
  The originally-named rehearsal at `rehearsal-20260515-095931` no
  longer exists; pick a new timestamped path.
- Run any `gh optivem` command that exercises `Dispatch` (e.g. an
  ATDD-running command — the AT-RED-DSL phase is the smallest exerciser).
  Confirm:
  1. `./.gh-optivem/docs/atdd/process/at-red-system-driver.md` exists
     inside the rehearsal.
  2. Placeholders are substituted: `${language}` → `typescript`,
     `${sut_namespace}` → `<repo-tail>`, `${driver_port}` → the
     concrete path from the rehearsal's `paths:` block.
  3. The frontmatter `substituted:` audit block is present and lists
     only keys that actually appeared in that file's body.
  4. `./.gh-optivem/.materialized.yaml` sidecar exists and matches the
     placeholder map.
  5. `touch gh-optivem.yaml` with no semantic change then re-run —
     materialization is skipped (sidecar comparison is value-based, not
     mtime-based).
  6. Re-run AT-RED-DSL and confirm the agent reads concrete paths.
- Migration sub-check: take a pre-existing `gh-optivem.yaml` that lacks
  `paths:` (or hand-author one), run `gh optivem config migrate`,
  confirm `paths:` is back-filled, then run the materialize flow and
  confirm it succeeds.
- Negative check: `paths.driver_port` resolving to a non-existent
  directory does NOT block (substitution layer doesn't FS-check
  `paths.*` values; `validatePath` already rejects malformed values at
  config load).

**Acceptance signal**: a short audit note appended to this plan (or to
a follow-up commit message) confirming each numbered point above passes
or, where it fails, capturing the failure mode for a follow-up plan.

## Execution ordering

Items must land in this order:

1. **Item 2 first.** Without per-language `paths:` defaults, Item 1's
   tests can't run end-to-end (docs referencing `${driver_port}` will
   fail substitution).
2. **Item 1 second.** Now there's a caller for `MaterializeProject`.
3. **Item 3 third.** Pre-existing configs get the back-fill.
4. **Item 4 last.** Verification ties it together against a fresh
   rehearsal and a migrated config.

Items 2 and 3 can technically be parallelized (different files, shared
defaults helper), but the shared helper is small enough that doing them
sequentially in one focused session is cheaper than coordinating two
subagents.

## Non-goals

(Carried forward from the design plan.)

- Replacing the doctrinal `shop/` literal in `glossary.md`'s explanatory
  paragraph. That paragraph TEACHES the convention.
- Substituting placeholders in agent body files
  (`internal/assets/runtime/prompts/atdd/*.md`) — already covered by
  `renderPrompt`'s `ExpandParams`.
- Auto-discovering `paths:` entries by scanning the file tree.
- Refactoring `glossary.md` to remove the canonical location names —
  the `paths:` keys must MATCH the glossary, not the other way around.
- Multi-language / multi-tree projects (multiple system-test trees with
  per-language driver ports).

## Cross-references

- Closed-out predecessor plan: `git show 76f9b1b:plans/20260515-1037-phase-doc-path-substitution-from-config.md`
  — full design context (background, principles, placeholder vocab,
  design decisions).
- Closeout commit: `7d2c8d2` (deleted the predecessor plan after rolling
  remaining work into this one).
- Substitution mechanism: `internal/assets/sync/materialize.go`.
- Placeholder map source: `internal/projectconfig/config.go` —
  `PlaceholderMap()` at line 377; `Paths map` at line 186.
- Existing migration pattern to mirror: `runConfigMigrate` at
  `internal/.../config_commands.go:307`, tests around
  `TestRunConfigMigrate_BackfillsReposForMultiRepoMonolith` in
  `config_commands_test.go:776`.
- Agent prompt pipeline: `clauderun.Dispatch` (line 292),
  `clauderun.renderPrompt` (line 401), `clauderun.Options` (line 48) in
  `internal/atdd/runtime/clauderun/clauderun.go`.
- Glossary doctrine for `paths:` keys:
  `internal/assets/global/docs/atdd/process/glossary.md`.
