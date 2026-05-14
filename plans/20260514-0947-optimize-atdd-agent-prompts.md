# Plan: optimize ATDD agent prompts and centralize ATDD ownership in gh-optivem

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-14T09:45:52Z`

## Context

`gh optivem implement` dispatches one of nine embedded agent prompts under
`internal/atdd/runtime/agents/prompts/*.md` per phase via `claude -p` (see
`internal/atdd/runtime/clauderun/clauderun.go:984`). The user reports the
Claude steps feel slow even on simple work and suspects they burn more
tokens than necessary. Static analysis of the embedded prompts confirms the
suspicion: the prompt bodies are large and heavily duplicated.

A second motivation is structural: the user wants **gh-optivem to be the
canonical owner of the ATDD process**, fully controlled from a single
location, with updates propagating automatically when the gh-optivem
binary upgrades — without any per-consumer-repo install ceremony.

### Size baseline (today)

```
atdd-backend.md          71 lines    4.7 KB
atdd-chore.md           223 lines   15.4 KB
atdd-driver.md          302 lines   21.5 KB
atdd-dsl.md             324 lines   23.2 KB
atdd-fix-verify.md       64 lines    5.4 KB
atdd-frontend.md         73 lines    4.6 KB
atdd-stubs.md            91 lines    5.1 KB
atdd-task.md            178 lines   13.2 KB
atdd-test.md            313 lines   21.0 KB
shared/session-end.md    25 lines    1.8 KB
─────────────────────────────────────────────
total                 1664 lines  115.9 KB
```

Each `claude -p` invocation gets the **full prompt as system input every
time** (`runAutonomous` at `clauderun.go:1009-1043` passes the rendered
string via argv). Three of the nine prompts are over 20 KB.

### Problem inventory

**P0. Three-way duplication of the ATDD source-of-truth.**
ATDD process docs and Claude assets exist in three places today:

| Where | What | Role |
|---|---|---|
| `shop/docs/atdd/` and `shop/.claude/agents/atdd/` | Source of truth | Hand-maintained in a course repo |
| `gh-optivem/internal/atdd/runtime/agents/prompts/*.md` | Embedded prompts | Copy-pasted with shop's docs inlined, used by `claude -p` |
| Every scaffolded repo (page-turner, etc.) | Installed copy | Output of `gh optivem init` |

This duplication is the structural root cause of the prompt bloat — fix
the duplication, the prompts can confidently reference canonical paths
without inlining content.

**P1. Inlined reference docs are the bulk of the payload (highest impact).**
Every prompt body ends with a `## References` section that pastes the
contents of multiple `docs/atdd/process/*.md` and
`docs/atdd/architecture/*.md` files verbatim. Examples:

- `atdd-dsl.md` inlines six full reference files
  (`shared-phase-progression.md`, `at-cycle-conventions.md`,
  `ct-cycle-conventions.md`, `at-red-dsl.md`, `ct-red-dsl.md`,
  `dsl-core.md`, `driver-port.md`, `language-equivalents.md`).
  About **85 %** of the file is references.
- `atdd-driver.md` inlines a similar bundle. About **85 %** is references.
- `atdd-test.md` (313 lines) follows the same pattern.

`atdd-fix-verify.md:178` and `atdd-task.md:178` already acknowledge the
alternative ("read with the Read tool if a step asks you to consult
them"). The agent has Read; there is no need to ship the docs in the
prompt — provided the docs are reachable at a known path at dispatch
time.

**P2. Heavy cross-prompt duplication.**
The boilerplate header (ticket vars, "one-shot dispatch", "don't commit /
don't summarise") is near-identical across **all nine prompts**.
`driver-port.md`, `language-equivalents.md`, and cycle-conventions files
are each inlined in 2–4 prompt files. Editing the canonical copy requires
hand-syncing every embedded copy.

**P3. AT phase + CT phase fused into a single prompt.**
`atdd-dsl.md`, `atdd-driver.md`, and `atdd-test.md` each describe both
their AT-cycle and CT-cycle variants in the same file. On any dispatch,
half the prompt content (the other cycle's content) is irrelevant.

**P4. Verbose, repetitive prose inside the prompt-specific sections.**
Anti-pattern lists frequently restate guard rails already stated in
"Conventions" and "WRITE" steps above. Tightening prose without removing
meaning would shrink each prompt by 10–20 %.

## Locked-in design

The plan went through several iterations during 2026-05-14 discussion;
the design below is the final shape.

### D1. gh-optivem owns all canonical assets, in one embedded tree

All embedded assets (runtime prompts, methodology docs, Claude Code
subagents, slash commands) live under a single `internal/assets/`
package with one `//go:embed` declaration. Subdivision is by **consumer**
(how the asset is delivered), with methodology as the leaf level so
future TDD / DDD / HA content extends the tree without inventing new
top-level directories.

```
internal/assets/
  runtime/                          ← fed to `claude -p` via argv (never on disk)
    prompts/atdd/*.md               ← per-phase prompts (atdd-dsl-at.md, etc.)
    shared/preamble.md              ← prepended bookend (D4)
    shared/session-end.md           ← appended bookend
  global/                           ← synced to user's home (D2 + D3)
    docs/atdd/{architecture,process,code}/*.md
    claude/agents/atdd/*.md         ← Claude Code interactive subagents
    claude/commands/atdd/*.md       ← Claude Code slash commands
  embed.go                          ← single //go:embed assets/...
```

The existing runtime prompts at `internal/atdd/runtime/agents/prompts/*.md`
**move** to `internal/assets/runtime/prompts/atdd/`. The
`internal/atdd/runtime/agents/` package is refactored to a thin wrapper:
its `Prompt()` function reads runtime prompts from
`internal/assets` rather than from its own local `embed.FS`. Public API
of `agents.Prompt()` is unchanged, so `clauderun` consumers are
unaffected.

These are the only authoritative copies. Shop's `docs/atdd/`,
`.claude/agents/atdd/`, `.claude/commands/atdd/` are removed; shop
becomes a regular consumer.

### D2. No copy to consumer repos; sync to per-user global paths

Consumer repos contain **zero ATDD assets on disk**. Sync targets:

```
~/.gh-optivem/docs/atdd/        ← docs (humans browse; agent reads here)
~/.claude/agents/atdd/           ← subagents (Claude Code finds for /atdd-*)
~/.claude/commands/atdd/         ← slash commands (same)
~/.gh-optivem/.version           ← stamp file (binary version of last sync)
```

The `atdd/` subdirectories are entirely owned by gh-optivem — re-syncs
overwrite their contents wholesale. Anything outside (`~/.claude/agents/myteam/`,
`docs/myteam-notes/` in any repo) is untouched forever.

### D3. Auto-sync on first invocation after binary upgrade

There is no `gh optivem asset install` subcommand. Sync is automatic:

```
Every `gh optivem <command>` invocation, at startup:
  Read ~/.gh-optivem/.version stamp.
  If missing OR != this binary's version:
    Walk embed.FS `global/` subtree, write to:
      ~/.gh-optivem/docs/        ← docs/atdd/... (future: docs/tdd/...)
      ~/.claude/agents/atdd/      ← Claude Code subagents
      ~/.claude/commands/atdd/    ← Claude Code slash commands
    Update stamp file.
    Print one-line notice: "Synced gh-optivem assets to ~/.gh-optivem and ~/.claude (vX.Y.Z)."
  Else: no-op (single stat call).
Then proceed with the user's actual command.
```

Sync is blanket-on-every-command (not scoped to `implement` and friends)
to avoid the "new ATDD command forgot to call the sync" failure mode.
Cost when up-to-date is one file read. The `atdd/` (and future `tdd/`,
`ddd/`, `ha/`) subdirectories under `~/.gh-optivem/docs/` and
`~/.claude/{agents,commands}/` are entirely owned by gh-optivem —
re-syncs overwrite their contents wholesale. Anything outside
(`~/.claude/agents/myteam/`, `docs/myteam-notes/` in any repo) is
untouched forever.

**Escape hatch:** `GH_OPTIVEM_NO_AUTO_SYNC=1` (or `=true`) disables
auto-sync. When set, ATDD-consuming commands (`implement` etc.) fail
fast with `"gh-optivem assets out of date or missing. Run \`gh optivem
asset sync\` or unset GH_OPTIVEM_NO_AUTO_SYNC."` rather than silently
running with stale assets. Non-ATDD commands proceed normally.

**Manual sync:** `gh optivem asset sync` triggers the sync explicitly.
Used by users with the escape hatch set, or to force re-sync. Naming
follows the `gh` CLI convention of singular noun namespaces (`gh repo`,
`gh pr`, `gh workspace`).

**Optional helpers (lower priority, can defer):**
- `gh optivem asset status` — shows synced version vs binary version
  and which target paths are populated. Useful for debugging.
- `gh optivem docs <slug>` / `gh optivem docs --open` — convenience for
  humans who want to read docs without browsing `~/.gh-optivem/docs/`
  in their editor.

### D4. Bounded post-processing: shared bookends, nothing else

The on-disk prompt file plus a tightly-bounded set of named substitutions
must equal what the agent receives. The only sanctioned transforms are
named-placeholder expansion and **two shared bookends** (preamble at the
top, session-end at the bottom) that frame every prompt symmetrically.
Conditionals, smart includes, templating layers, and any other
post-processing are out.

The 8-line preamble currently duplicated near-verbatim across all nine
prompt files (`ticket vars / one-shot dispatch / don't commit / don't
summarise`) is extracted to `shared/preamble.md` and **prepended** by
`agents.Prompt()`, mirroring how `shared/session-end.md` is appended.

Transforms after this plan completes:

| # | Transform | Status |
|---|---|---|
| 0 | `agents.Prompt()` **prepends** `shared/preamble.md` | **Add** (mirror of session-end append) |
| 1 | `agents.Prompt()` appends `shared/session-end.md` | **Keep** |
| 2 | `filterConditionals` strips `<!-- if -->` blocks | **Delete** (no users after subtype-split) |
| 3 | `ExpandParams` substitutes `${name}` placeholders | **Keep** (load-bearing) |
| 4 | `OverrideText` append | **Keep** (only fires when caller provides one) |
| 5 | `materializePrompt` tempfile spill above argv limit | **Keep** (delivery path; no content change) |

Net: 5 transforms today → 5 after. `filterConditionals` swaps out;
preamble prepend swaps in.

Token impact per dispatch is roughly zero — today the preamble is shipped
inline in each prompt; after the change it is shipped via the prepend.
The win is maintainability: the preamble is edited once in
`shared/preamble.md` instead of nine hand-synced copies.

### D5. Strip inlined references; prompts point at canonical paths

Each `### Reference: ...` block in the prompt body becomes a one-line
pointer using a `${docs_root}` placeholder substituted at render time:

```
Apply Driver Port Rules — read `${docs_root}/atdd/architecture/driver-port.md`.
Apply DSL Core Rules — read `${docs_root}/atdd/architecture/dsl-core.md`.
Cycle conventions — read `${docs_root}/atdd/process/at-cycle-conventions.md`.
```

`${docs_root}` is added to the fixed-schema placeholders in
`renderPrompt` (`clauderun.go:432-448`), populated with the absolute
path `~/.gh-optivem/docs/` at render time. The methodology (`atdd/`,
future `tdd/`, `ddd/`, `ha/`) is part of the pointer that the prompt
file writes — a single placeholder serves every methodology. The
agent's `Read` tool resolves the absolute path against the filesystem;
no working-directory dependency.

### D6. Keep "safety" rules inline; move only reference material

Stays inline in each prompt:
- The 8-line preamble (ticket vars, one-shot dispatch, don't commit,
  don't summarise, don't push).
- A 1–2 sentence "what this phase produces" anchor.
- Cross-cutting "never X" guardrails specific to the prompt.

Moves to `${atdd_docs_root}/...` Read-on-demand pointers:
- Full `docs/atdd/architecture/*.md` content.
- Full `docs/atdd/process/*.md` content.
- `docs/atdd/code/language-equivalents.md` tables.
- Cycle-conventions and DSL-rules docs.

Budget: total inline guardrails per prompt under ~500 bytes. If a rule
grows past that, it has probably crept into reference territory and
should move out.

Rationale: rules whose violation is high-cost (lost work, surprise
commits) must be in the context window from turn 0 — agents skip Reads
when they think they already know. Reference material can be Read-on-
demand because skipping is recoverable.

### D7. Split AT-cycle and CT-cycle prompts into separate files (S3a)

`atdd-dsl.md`, `atdd-driver.md`, `atdd-test.md` each split into a
per-cycle pair:

```
atdd-dsl-at.md      atdd-dsl-ct.md
atdd-driver-at.md   atdd-driver-ct.md
atdd-test-at.md     atdd-test-ct.md
```

Dispatcher picks the right file based on phase. Each file reads
top-to-bottom with no conditionals.

Token efficiency: identical to conditional gating (S3b). The
maintainability win is what justifies S3a: readers don't have to
mentally execute `<!-- if:cycle=... -->` blocks to see what each cycle
actually receives.

### D8. Split atdd-task.md by subtype; delete filterConditionals

`atdd-task.md` line 134 uses `<!-- if:subtype=external-system-interface-redesign -->`
for a ~20-line block. Two subtypes ever reach this prompt:
`system-interface-redesign` and `external-system-interface-redesign`.

Split into:
```
atdd-task-system-interface-redesign.md
atdd-task-external-system-interface-redesign.md
```

Dispatcher picks based on subtype. Once split, `<!-- if -->` markers
appear nowhere in the prompts package and `filterConditionals` +
`conditionalRE` (`clauderun.go:358-373`) + the corresponding tests can
be **deleted**. One fewer post-processing transform.

### D9. Tighten prose

Two-pass copyedit on the surviving prompt bodies. Remove restatements
of guard rails already stated earlier in the same prompt. No new
mechanics. Done after the structural moves above so the editing target
is the final shape.

Expected ~15 % size drop on each surviving prompt.

### D10. Per-language equivalents files

`docs/atdd/code/language-equivalents.md` today is one combined doc
covering every supported language. Each dispatch that consults it makes
the agent ingest equivalents for languages the project does not use.

Split into per-language files synced under
`~/.gh-optivem/docs/atdd/code/language-equivalents/`:

```
language-equivalents/
  go.md
  java.md
  python.md
  typescript.md
  csharp.md
  ...
  README.md           ← links to each language file (replaces the
                        "all-in-one-view" the combined doc gave)
```

Add `${language}` to the fixed-schema placeholder map, populated from
the project's stack at dispatch time (the same source `gh optivem` uses
elsewhere). Prompt pointers parameterize:

```
Apply language equivalents — read `${docs_root}/atdd/code/language-equivalents/${language}.md`.
```

Polyglot projects are handled per-phase: backend phases dispatch with
`${language}=go`, frontend phases with `${language}=typescript`. Each
phase consults one language. A phase that genuinely needs both writes
two pointer lines.

Same family as D7 / D8 — axis-based file splits that ship only the
slice this dispatch needs. Language is independent of cycle (D7) and
subtype (D8), so this is its own decision rather than a sub-case.
Placed after D9 numerically because it was added late in the design
walk-through; logically it belongs alongside D7 and D8.

If `${language}` is unset, render fails fast — load-bearing placeholder.
A silent default would mask config bugs.

## Critical files

**D1 + D2 — single embedded asset tree, ownership move:**
- `internal/assets/` — new package root for all embedded assets.
- `internal/assets/embed.go` — single `//go:embed assets/...` covering
  both `runtime/` and `global/` subtrees. Follows the
  `internal/atdd/runtime/agents/embed.go:10-16` pattern.
- `internal/assets/runtime/prompts/atdd/*.md` — moved from
  `internal/atdd/runtime/agents/prompts/*.md`.
- `internal/assets/runtime/shared/preamble.md` — new (extracts the
  duplicated preamble per D4).
- `internal/assets/runtime/shared/session-end.md` — moved from
  `internal/atdd/runtime/agents/shared/session-end.md`.
- `internal/assets/global/docs/atdd/{architecture,process,code}/*.md` —
  new (moved from `shop/docs/atdd/`).
- `internal/assets/global/claude/agents/atdd/*.md` — new (moved from
  `shop/.claude/agents/atdd/`, including `meta/` subtree).
- `internal/assets/global/claude/commands/atdd/*.md` — new (moved from
  `shop/.claude/commands/atdd/`).
- `internal/atdd/runtime/agents/` — refactored to a thin wrapper that
  reads from `internal/assets`. `agents.Prompt()` keeps its public API
  (same callsites in `clauderun`), now prepends `runtime/shared/preamble.md`
  and appends `runtime/shared/session-end.md`.
- `internal/atdd/install.go` — rewrite or remove. The current
  shop-walk install (`install.go:88-114`) is replaced by the sync
  mechanism below; if the file's other responsibilities (system code
  copy, workflows) survive, they stay; the ATDD-asset half is deleted.
- `internal/atdd/install_test.go` — drop ATDD-asset fixtures.
- shop repo (separate PR or coordinated commit): delete
  `shop/docs/atdd/`, `shop/.claude/agents/atdd/`,
  `shop/.claude/commands/atdd/`. Leave a `shop/docs/atdd/README.md`
  stub pointing at gh-optivem.

**D3 — auto-sync mechanism:**
- `internal/assets/sync/sync.go` — new package. Exports `EnsureSynced()`
  which reads the stamp, compares, walks the `global/` subtree of the
  embedded FS, writes to `~/.gh-optivem/docs/` and `~/.claude/`,
  updates the stamp. Atomic write via temp+rename. File lock for
  concurrent invocations.
- `internal/assets/sync/sync_test.go` — coverage for stamp matching,
  cross-version sync, escape-hatch behavior, atomic-write under
  concurrent invocation.
- `cmd/gh-optivem/main.go` (or wherever the root command lives) —
  invoke `sync.EnsureSynced()` at startup unless
  `GH_OPTIVEM_NO_AUTO_SYNC` is truthy.
- `cmd/gh-optivem/asset_sync.go` — new subcommand
  `gh optivem asset sync` for explicit sync.
- ATDD-consuming commands (notably `gh optivem implement`) — when the
  escape hatch is set, check the stamp and fail fast with the
  documented error if stale.

**D5 + D10 — placeholder substitution:**
- `internal/atdd/runtime/clauderun/clauderun.go:432-448` — add two
  placeholders to the fixed-schema map:
  - `docs_root` — absolute path of `~/.gh-optivem/docs/`.
  - `language` — current dispatch's target language (e.g. `go`,
    `typescript`); load-bearing, render fails if unset.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — extend the
  `TestRenderPrompt_*` cases with `${docs_root}` and `${language}`
  substitution assertions, plus the "unset `${language}` fails fast"
  case.

**D7 + D8 + D10 — prompt file splits:**
- `internal/assets/runtime/prompts/atdd/atdd-dsl-at.md` — new
  (AT-cycle half of `atdd-dsl.md`).
- `internal/assets/runtime/prompts/atdd/atdd-dsl-ct.md` — new
  (CT-cycle half of `atdd-dsl.md`).
- `internal/assets/runtime/prompts/atdd/atdd-driver-at.md`,
  `atdd-driver-ct.md` — same pattern.
- `internal/assets/runtime/prompts/atdd/atdd-test-at.md`,
  `atdd-test-ct.md` — same pattern.
- `internal/assets/runtime/prompts/atdd/atdd-task-system-interface-redesign.md` —
  new (atdd-task.md minus the gated block).
- `internal/assets/runtime/prompts/atdd/atdd-task-external-system-interface-redesign.md` —
  new (atdd-task.md with gated block inlined).
- Delete: `atdd-dsl.md`, `atdd-driver.md`, `atdd-test.md`,
  `atdd-task.md`.
- `internal/assets/global/docs/atdd/code/language-equivalents/{go,java,python,typescript,csharp,...}.md` —
  per-language split of the prior combined `language-equivalents.md`.
- `internal/assets/global/docs/atdd/code/language-equivalents/README.md` —
  index linking to each language file.
- Dispatcher — `internal/atdd/runtime/clauderun/clauderun.go` or the
  agent resolution layer — extend the agent-name lookup to map
  `(phase, cycle)` and `(task, subtype)` to the right file; pass
  `${language}` through to render.

**D5 + D6 — prompt body slimming:**
- `internal/assets/runtime/prompts/atdd/atdd-dsl-{at,ct}.md` — strip
  inlined references, keep guardrails inline.
- `internal/assets/runtime/prompts/atdd/atdd-driver-{at,ct}.md` — same.
- `internal/assets/runtime/prompts/atdd/atdd-test-{at,ct}.md` — same.
- `internal/assets/runtime/prompts/atdd/atdd-task-*.md` — strip inlined
  `system.md`, `driver-port.md`, `driver-adapter.md` references.
- `internal/assets/runtime/prompts/atdd/atdd-chore.md` — strip inlined
  `task-and-chore-cycles.md`.
- `internal/assets/runtime/prompts/atdd/atdd-backend.md`,
  `atdd-frontend.md`, `atdd-stubs.md`, `atdd-fix-verify.md` — strip
  any remaining inlines.

**filterConditionals removal:**
- `internal/atdd/runtime/clauderun/clauderun.go:347-373` — delete
  `conditionalRE` and `filterConditionals`.
- `internal/atdd/runtime/clauderun/clauderun.go:449-453` — remove the
  `body = filterConditionals(body, params)` call from `renderPrompt`.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — delete the
  conditional-gating test cases.

## Reuse references

- `internal/atdd/runtime/agents/embed.go:10-16` — precedent for the
  `//go:embed` + `embed.FS` pattern that the new
  `internal/atdd/assets/embed.go` follows.
- `internal/atdd/runtime/agents/embed.go:32-39` — `agents.Prompt`
  pattern for appending `shared/session-end.md`; stays as-is.
- `internal/atdd/runtime/clauderun/clauderun.go:432-448` — fixed-schema
  placeholder map; `${docs_root}` and `${language}` join this set.
- `internal/atdd/runtime/clauderun/clauderun.go:264-269` —
  `findUnfilledPlaceholders` already guards against typos; the new
  placeholders don't break the guarantee.
- `atdd-fix-verify.md:178` / `atdd-task.md:178` — precedent for
  "reference exists at a known path; read with the Read tool" —
  generalized by D5.

## Steps

### Step 4 — D7 + D8 + D10 (file splits)

Split `atdd-dsl`, `atdd-driver`, `atdd-test` into per-cycle files.
Split `atdd-task` into per-subtype files. Split
`docs/atdd/code/language-equivalents.md` into per-language files
under `language-equivalents/<language>.md` (plus a README index).
Update the dispatcher's agent-name lookup to map phase/cycle/subtype
to filename; pass `${language}` through to render.

**Validation:**
- Each new prompt file reads top-to-bottom without conditionals.
- AT-cycle dispatch picks the `-at.md` file; CT-cycle picks `-ct.md`.
- Task dispatch picks the right subtype file.
- Rendered language-equivalents pointer resolves to the correct
  per-language file via `${language}`.
- `TestRenderPrompt_*` updated to cover each variant.

### Step 5 — filterConditionals deletion

Delete `conditionalRE`, `filterConditionals`, the
`body = filterConditionals(...)` call in `renderPrompt`, and the
conditional-gating tests.

**Validation:**
- Build succeeds.
- No `<!-- if -->` markers remain in any prompt file
  (`rg 'if:[a-z_]+=' internal/assets/runtime/prompts/` returns empty).

### Step 6 — D6 + D9 (strip inlined references + prose copyedit)

Per-prompt copyedit:
- Replace each `### Reference: ...` block with a one-line
  `${docs_root}/atdd/...` pointer (use `${language}` where the
  reference is language-equivalents).
- Keep guardrails inline; budget ~500 bytes total per prompt.
- Two-pass copyedit on surviving prose; remove duplicated guard rails.

Land as one PR per prompt (or one big PR — operator choice) so a
regression in one prompt doesn't taint the others.

**Validation:**
- `wc -c` on the three heavy prompts drops to ~6 KB each.
- A smoke `gh optivem implement` run on a known ticket completes
  without the agent reporting "missing context."
- `findUnfilledPlaceholders` still returns empty.

### Step 7 — measure

Capture per-dispatch token usage before / after from the
`writeExitBanner` output (`clauderun.go:844`) on the same ticket.
The banner already prints `… token in / … token out, $…` when the
`claude -p --output-format json` envelope decodes.

**Validation:** input tokens drop by the predicted ~40–70 % on the
three heavy prompts; output tokens unchanged or slightly lower; wall
time drops proportionally.

## Out of scope

- Changing the Claude CLI invocation flags (e.g. enabling a hypothetical
  `--prompt-cache`). Worth a follow-up plan if/when the upstream CLI
  exposes one.
- An MCP-server alternative to the temp-extract / global-sync model
  (would eliminate the on-disk docs requirement entirely but is a
  significant separate project — flagged during planning as Option 3,
  not chosen).
- Restructuring the agent set itself (merging `atdd-backend` +
  `atdd-frontend` into `atdd-system`, etc.). Agent topology is a
  different conversation.
- Re-shaping the `${allowed_roots}` / `${checklist}` substitution blocks
  produced by the driver. They are already compact and per-call.
- The non-ATDD portions of `gh optivem init` (system code copy,
  workflow installation, externals). D1's scope is the ATDD asset
  subset only.
- `gh optivem docs <slug>` / `--open` helpers and
  `gh optivem asset status`. Nice-to-have, can land in a follow-up.
