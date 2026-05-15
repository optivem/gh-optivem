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

Items 1, 2, 3 have landed in commits on this branch — see Audit findings
below for the embedded-docs blocker surfaced during Item 1 testing.

## Scope

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

**Known blocker from Items 1–3** (see Audit findings below) — Item 4
verification will hit the embedded-docs `${...}` meta-references the
first time `MaterializeProject` walks the full tree. Expect points 1–6
above to fail until the audit follow-up plan lands; that's the
intended outcome of Item 4 (capture failure mode for a follow-up).

## Audit findings — surfaced during Items 1–3

During Item 1's wiring tests, `MaterializeProject` against the full
embedded `docs/atdd/` tree errors out with:

> `materialize: docs/atdd/process/cycles.md references unfilled
> placeholders ${system_lang}, ${test_lang}; declared: architecture,
> driver_adapter, driver_port, …`

Two distinct classes of issues:

1. **Undeclared "real" placeholders.** `cycles.md` line 263 references
   `${system_lang}` and `${test_lang}` as substitution targets, but
   `projectconfig.PlaceholderMap()` only emits `${language}` (with the
   monolith→multitier fallback). Either:
   - Add `system_lang` (sourced from `system.lang`) and `test_lang`
     (sourced from `system_test.lang`) to `PlaceholderMap`, OR
   - Edit cycles.md to use `${language}` and drop the system/test
     distinction in the prose (the schema doesn't carry the
     distinction at substitution time today).

2. **Teaching meta-references.** `placeholders.md` uses `${name}`,
   `${key}`, `${typo}` in code spans to TEACH the placeholder syntax
   (e.g. *"the docs use `${name}` placeholders"*). The current
   substitution layer can't distinguish teaching prose from real
   placeholders. Options:
   - Add an escape syntax (`$${name}` → literal `${name}`) to
     `internal/expand/expand.go` and edit the teaching docs.
   - Skip substitution inside markdown code spans (requires a basic
     markdown-aware walker in `materialize.go`).
   - Exclude `placeholders.md` (and any other doc that's pure teaching
     material) from the substitution pass via an opt-out frontmatter
     directive.

Both classes block running `MaterializeProject` against the embedded
tree end-to-end. A follow-up plan should pick a design and land the
fix; Item 4 then runs the rehearsal verification.

**Item 1's tests bypass this gap** by pre-writing a value-equal sidecar
so `MaterializeProject`'s staleness check short-circuits before the
walk. That tests the wiring (Dispatch → MaterializeProject →
projectDocsRoot → renderPrompt) but does not exercise the walker
against the real embedded corpus. Production callers WILL hit the gap
on first dispatch in a fresh project — hence the need for the
follow-up before Item 4 can pass.

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
- Per-language defaults helper landed this session:
  `internal/projectconfig/paths_defaults.go`.
- Glossary doctrine for `paths:` keys:
  `internal/assets/global/docs/atdd/process/glossary.md`.
- Placeholder doctrine + audit-finding source:
  `internal/assets/global/docs/atdd/process/placeholders.md` and
  `cycles.md:263`.
