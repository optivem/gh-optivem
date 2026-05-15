# Phase-Doc Placeholders

Phase docs reference filesystem paths that vary per project — the SUT's namespace folder, the system-test driver-port location, the system root. Rather than hardcoding values that go stale every time a team reorganises its tree, the docs use `${name}` placeholders. At sync time `gh-optivem` substitutes them against the project's `gh-optivem.yaml` and writes the concrete result to `./.gh-optivem/docs/`. Agents read the substituted output; the source-repo templates stay parameterised.

The same `${name}` syntax is used by agent prompts (`renderPrompt` in `clauderun`) — one substitution vocabulary, two consumers.

## Two placeholder families

Every placeholder is one of two families, sharing a single flat namespace.

### Family A — fixed-schema (from existing config fields)

Sourced from top-level fields in `gh-optivem.yaml`. The placeholder name is fixed; the value is the corresponding config field.

| Placeholder | Source |
|---|---|
| `${language}` | `system.lang` (or `system_test.lang` when `system.lang` is empty — multitier) |
| `${architecture}` | `system.architecture` |
| `${system_path}` | `system.path` |
| `${system_test_path}` | `system_test.path` |
| `${sut_namespace}` | `system.sut_namespace`, defaulting to the last path segment of `system.repo` |

### Family B — named locations (under `paths:`)

User-owned. Every key under the top-level `paths:` block becomes a `${key}` placeholder. The scaffolder writes a default block matching the project's layout:

```yaml
paths:
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
  external_driver_port: system-test/typescript/src/testkit/external/port
  external_driver_adapter: system-test/typescript/src/testkit/external/adapter
```

When a team reorganises its tree (e.g. renames `testkit` to `support`), they update `paths:` once and re-run any `gh optivem` command — phase docs re-materialize with the new layout. **No doc edits needed.**

Family B keys cannot shadow Family A names — `gh optivem config validate` rejects a `paths.language` key (or any other reserved name).

## What's NOT a placeholder

- `<channel>` stays in angle brackets. It's genuinely free-form (`api`, `ui`, `cli`) — the SUT may have several, and the reader picks. Phase docs use `${driver_adapter}/${sut_namespace}/<channel>/` to mean "for each channel the SUT exposes".
- The literal `shop/` in `glossary.md`'s explanatory paragraph. That paragraph TEACHES the convention — substituting it in prose would defeat the lesson. Substitution happens in PATHS, not in PROSE.

## How substitution happens

1. The user runs any `gh optivem` command inside a project.
2. The sync layer compares the project's current placeholder map (computed from `gh-optivem.yaml`) against `./.gh-optivem/.materialized.yaml` (the sidecar from the last materialization).
3. If identical, sync skips. If different (or sidecar absent), sync walks the embedded `docs/atdd/` tree, expands `${name}` references, prepends a per-file `substituted:` audit block, and writes to `./.gh-optivem/docs/`.
4. The sidecar is overwritten with the current placeholder map and binary version.
5. The user's home directory `~/.gh-optivem/docs/` remains UNSUBSTITUTED — it's shared across all the user's projects and must stay project-agnostic.

A `${name}` reference that has no corresponding key in either family is a sync-time error (`materialize: <file> references unfilled placeholders ${typo}; declared: ...`) — typos surface as a hard fail, not as broken paths shipped to agents.

## Editing phase docs

When editing a doc under `internal/assets/global/docs/atdd/process/`:

- Use `${name}` for any path component that varies per project. Refer to Family A first; if no fixed-schema placeholder fits, the path is a Family B candidate.
- Don't introduce new Family B keys without also updating the scaffolder's default `paths:` block (so newly-scaffolded projects don't trip the unfilled-placeholder error).
- Don't add ad-hoc `<angle-bracket>` markers as placeholder substitutes. If a value is parameterisable, it goes through the `${name}` mechanism.

Cross-references: [`glossary.md`](glossary.md) — `shop` doctrine that anchors `${sut_namespace}`. `internal/projectconfig/config.go` — `PlaceholderMap()` builds the flat map at runtime.
