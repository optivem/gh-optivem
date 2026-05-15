# Plan: Reference driver-port / driver-adapter as concepts, not hardcoded paths

## Background

The companion plan `20260515-1037-phase-doc-path-substitution-from-config.md`
adds config-driven substitution for placeholders like `${language}` and
`${sut_namespace}`. That covers the half of every doc path that is
config-knowable — values that already live in `gh-optivem.yaml`.

But the OTHER half of the path is the tail — `.../testkit/driver/port/`,
`.../testkit/driver/adapter/` — and those segments are NOT config-knowable
today. The human is free to lay them out however they want. Plausible
layouts seen in the wild and in past iterations of this repo:

- `system-test/typescript/src/testkit/driver/port/`
- `system-test/typescript/src/support/drivers/in/`
- `system-test/typescript/src/framework/driver/port/`
- etc.

The recent commit `c88f205` ("atdd-process-docs: correct Driver paths
(`testkit/driver/{port,adapter}`)") is direct evidence that even THIS repo
has changed its mind about the layout. Any doc that hardcodes the tail will
go stale the next time the user reorganises.

What IS stable across every ATDD project is the **concept**: every project
has a driver port and a driver adapter. What's not stable is where the user
puts them. The phase docs should refer to those concepts by name and
resolve the actual path through a small user-owned manifest.

## End-result example

### Input — additions to `gh-optivem.yaml`

A new `concepts:` block the user owns and edits whenever they reorganise:

```yaml
concepts:
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
  external_driver_port: system-test/typescript/src/testkit/external/port
  external_driver_adapter: system-test/typescript/src/testkit/external/adapter
```

The scaffolder writes a default block matching `glossary.md` conventions
when a project is created; the user overrides as their layout evolves.

### Source-repo template

```markdown
The system-test driver port lives at:
  ${driver_port}/

When you add a new channel, create:
  ${driver_adapter}/<channel>/<channel>-driver.ts
```

### Output after `gh optivem sync`

```markdown
---
substituted:
  driver_port: system-test/typescript/src/testkit/driver/port
  driver_adapter: system-test/typescript/src/testkit/driver/adapter
---

The system-test driver port lives at:
  system-test/typescript/src/testkit/driver/port/

When you add a new channel, create:
  system-test/typescript/src/testkit/driver/adapter/<channel>/<channel>-driver.ts
```

When the user later renames `testkit` to `support`, they update the manifest
once, re-sync, and every phase doc that referred to `${driver_port}` reflects
the new layout. No doc edits needed.

## Boundary with the companion plan

| Concern | Owned by |
|---|---|
| `${language}`, `${system_path}`, `${sut_namespace}`, etc. — values lifted from existing `gh-optivem.yaml` fields | Companion plan (`20260515-1037`) |
| `${driver_port}`, `${driver_adapter}`, etc. — concept names whose paths the user freely structures and may rename over time | This plan |

Both plans share the same substitution mechanism: sync-time, inline body
substitution, with a frontmatter `substituted:` audit block. Only the
placeholder vocabulary and its source differ. This plan's vocabulary is
declared by the user under `concepts:`; the companion plan's vocabulary is
fixed-schema, lifted from existing top-level config fields.

## Initial concept list

Concepts we already know exist in every ATDD project (anchor against
`internal/assets/global/docs/atdd/process/glossary.md`):

- `driver_port` — system-test driver-port interface package
- `driver_adapter` — system-test driver-adapter implementations (per-channel)
- `external_driver_port` — CT external driver-port
- `external_driver_adapter` — CT external driver-adapter
- `system_test_root` — top of system-test tree, for "create a file under here"
  instructions where the doc should not pre-commit to a sub-folder
- `sut_root` — top of SUT tree
- (extend as the glossary grows)

Defaults shipped in `gh-optivem.yaml` should match `glossary.md` so a
brand-new scaffold works without the user touching `concepts:` at all.
Experienced users override only what they reorganise.

## Implementation steps

### Step 1 — Add `concepts:` block to project config

- New top-level `concepts` map in `gh-optivem.yaml` schema. Each key is a
  concept name; each value is a path relative to the repo root.
- Wire it through `projectconfig.Config` so the placeholder map built by the
  companion plan picks up `concepts.*` keys (flattened to `${name}`).
- Provide a default `concepts:` block the scaffolder writes for each
  architecture template.

### Step 2 — Convert affected docs to concept references

Audit phase docs (Grep `internal/assets/global/docs/atdd/process/*.md`) for
hardcoded `testkit/driver/port`, `testkit/driver/adapter`, `testkit/external/`,
etc. fragments. Replace with `${driver_port}`, `${driver_adapter}`, etc.

The companion plan's Step 3 already converts `<lang>` and `shop/` literals;
this plan's Step 2 picks up the next layer down.

### Step 3 — Validate concepts at sync time

`findUnfilledPlaceholders` (per the companion plan, lifted to a shared
package) already catches typos. Extend the error message so `${driver_prt}`
(typo) reports "unknown concept; declared concepts are: driver_port,
driver_adapter, ...".

### Step 4 — Document the convention

In the docs section the companion plan creates: add a sub-section explaining
that concepts named under `${...}` resolve from the user's `concepts:` map,
and that users should edit `concepts:` (not the docs) when they reorganise.

## Open questions

- **Q1.** Syntax: `${driver_port}` (flat — same as companion plan) vs.
  `${concepts.driver_port}` (dotted — namespace-clear)? Flat is consistent
  with `${language}` style; dotted prevents collisions with future top-level
  config fields. Lean toward flat unless a real collision shows up.

- **Q2.** Discovery vs. declaration. Could the tool auto-discover driver-port
  by scanning for files matching a marker (e.g. interface name `DriverPort`)?
  Cheaper for the user but fragile (renamed interface = broken docs).
  Lean toward declaration — explicit and the user already owns their layout.

- **Q3.** Should the scaffolder REQUIRE `concepts:` to be present, or treat
  it as optional with built-in defaults per architecture? Optional with
  defaults is friendlier, but means a doc that references a concept the
  user hasn't declared resolves to the default — which may not exist in
  their tree. Probably: ship defaults, but `gh optivem sync` warns when a
  concept resolves to a path that doesn't exist on disk.

- **Q4.** How does this interact with multiple system-test trees (e.g. a
  multi-language project where each language has its own driver port)?
  Out of scope for v1; revisit when we have the use case.

## Non-goals

- Replacing the companion plan's `${language}` / `${sut_namespace}`
  substitution. This plan ADDS a vocabulary layer; the companion plan
  remains the substrate.

- Auto-discovering concept paths by scanning the file tree (see Q2).

- Refactoring `glossary.md` to remove concept names — concept names in this
  plan should match the glossary, not the other way around.

- Substitution in agent body files. Same boundary as the companion plan:
  agent prompts go through `renderPrompt`'s `ExpandParams` at dispatch.

## Cross-references

- Companion plan:
  `plans/20260515-1037-phase-doc-path-substitution-from-config.md` —
  config-driven substitution for the `${language}` / `${sut_namespace}` half
  of the problem. Read that plan first; this one assumes its mechanism.

- Commit `c88f205` (2026-05-15) "atdd-process-docs: correct Driver paths
  (`testkit/driver/{port,adapter}`)" — concrete evidence that hardcoded
  driver paths churn even within this repo, motivating concept-level
  indirection.

- `internal/assets/global/docs/atdd/process/glossary.md` — canonical
  concept vocabulary; this plan's manifest keys must align.
