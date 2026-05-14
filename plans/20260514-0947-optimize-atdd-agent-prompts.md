# Plan: optimize ATDD agent prompts and centralize ATDD ownership in gh-optivem

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

### D1. gh-optivem owns the canonical ATDD assets, fully embedded

All ATDD assets live in the gh-optivem binary via `//go:embed`:

```
internal/atdd/assets/
  docs/atdd/architecture/*.md
  docs/atdd/process/*.md
  docs/atdd/code/*.md
  claude/agents/atdd/*.md     (Claude Code subagents — for interactive flow)
  claude/commands/atdd/*.md   (Claude Code slash commands — for interactive flow)
```

These are the only authoritative copies. Shop's `docs/atdd/`,
`.claude/agents/atdd/`, `.claude/commands/atdd/` are removed; shop becomes
a regular consumer.

The internal prompt files at `internal/atdd/runtime/agents/prompts/*.md`
remain embedded as today via the existing `//go:embed prompts/*.md`.
These are separate from the new assets bundle — they are content fed
into `claude -p` directly, never written to disk.

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

There is no `gh optivem atdd install` subcommand. Sync is automatic:

```
Every `gh optivem <command>` invocation, at startup:
  Read ~/.gh-optivem/.version stamp.
  If missing OR != this binary's version:
    Write embed.FS contents to ~/.gh-optivem/docs/atdd/,
                                ~/.claude/agents/atdd/,
                                ~/.claude/commands/atdd/.
    Update stamp file.
    Print one-line notice: "Synced ATDD assets to ~/.gh-optivem and ~/.claude (vX.Y.Z)."
  Else: no-op (single stat call).
Then proceed with the user's actual command.
```

Sync is blanket-on-every-command (not scoped to `implement` and friends)
to avoid the "new ATDD command forgot to call the sync" failure mode.
Cost when up-to-date is one file read.

**Escape hatch:** `GH_OPTIVEM_NO_AUTO_SYNC=1` (or `=true`) disables
auto-sync. When set, ATDD-consuming commands (`implement` etc.) fail
fast with `"ATDD assets out of date or missing. Run \`gh optivem atdd
sync\` or unset GH_OPTIVEM_NO_AUTO_SYNC."` rather than silently running
with stale assets. Non-ATDD commands proceed normally.

**Manual sync:** `gh optivem atdd sync` triggers the sync explicitly.
Used by users with the escape hatch set, or to force re-sync.

**Optional helpers (lower priority, can defer):**
- `gh optivem atdd status` — shows synced version vs binary version and
  which target paths are populated. Useful for debugging.
- `gh optivem atdd docs <slug>` / `gh optivem atdd docs --open` —
  convenience for humans who want to read docs without browsing
  `~/.gh-optivem/docs/atdd/` in their editor.

### D4. Minimize post-processing of prompt files

Per `feedback_minimize_prompt_post_processing.md`: avoid adding new
transforms between the on-disk prompt and what `claude -p` receives.
Each transform makes the source-of-truth ambiguous and hides what the
agent actually sees.

Transforms after this plan completes:

| # | Transform | Status |
|---|---|---|
| 1 | `agents.Prompt()` appends `shared/session-end.md` | **Keep** (grandfathered) |
| 2 | `filterConditionals` strips `<!-- if -->` blocks | **Delete** (no users after subtype-split) |
| 3 | `ExpandParams` substitutes `${name}` placeholders | **Keep** (load-bearing) |
| 4 | `OverrideText` append | **Keep** (only fires when caller provides one) |
| 5 | `materializePrompt` tempfile spill above argv limit | **Keep** (delivery path; no content change) |

Notably, a shared-header prepend (proposed earlier as S2) is **not**
introduced. The "ticket vars / one-shot dispatch / don't commit / don't
summarise" preamble stays inlined in each prompt file, accepting 9×
duplication.

### D5. Strip inlined references; prompts point at canonical paths

Each `### Reference: ...` block in the prompt body becomes a one-line
pointer using a `${atdd_docs_root}` placeholder substituted at render
time:

```
Apply Driver Port Rules — read `${atdd_docs_root}/architecture/driver-port.md`.
Apply DSL Core Rules — read `${atdd_docs_root}/architecture/dsl-core.md`.
Cycle conventions — read `${atdd_docs_root}/process/at-cycle-conventions.md`.
```

`${atdd_docs_root}` is added to the fixed-schema placeholders in
`renderPrompt` (`clauderun.go:432-448`), populated with the absolute
path `~/.gh-optivem/docs/atdd/` at render time. The agent's `Read` tool
resolves the absolute path against the filesystem; no working-directory
dependency.

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

## Critical files

**D1 + D2 — ownership move and embed expansion:**
- `internal/atdd/assets/docs/atdd/{process,architecture,code}/*.md` —
  new (moved from `shop/docs/atdd/`).
- `internal/atdd/assets/claude/agents/atdd/*.md` — new (moved from
  `shop/.claude/agents/atdd/`, including `meta/` subtree).
- `internal/atdd/assets/claude/commands/atdd/*.md` — new (moved from
  `shop/.claude/commands/atdd/`).
- `internal/atdd/assets/embed.go` — new; declares `//go:embed assets/...`
  following the `internal/atdd/runtime/agents/embed.go:10-16` precedent.
- `internal/atdd/install.go` — rewrite or remove. The current
  shop-walk install (`install.go:88-114`) is replaced by the sync
  mechanism below; if the file's other responsibilities (system code
  copy, workflows) survive, they stay; the ATDD-asset half is deleted
  from here.
- `internal/atdd/install_test.go` — drop ATDD-asset fixtures.
- shop repo (separate PR or coordinated commit): delete
  `shop/docs/atdd/`, `shop/.claude/agents/atdd/`,
  `shop/.claude/commands/atdd/`. Leave a `shop/docs/atdd/README.md`
  stub pointing at gh-optivem.

**D3 — auto-sync mechanism:**
- `internal/atdd/sync/sync.go` — new package. Exports `EnsureSynced()`
  which reads the stamp, compares, writes assets if needed, updates
  the stamp. Atomic write via temp+rename. File lock for concurrent
  invocations.
- `internal/atdd/sync/sync_test.go` — coverage for stamp matching,
  cross-version sync, escape-hatch behavior, atomic-write under
  concurrent invocation.
- `cmd/gh-optivem/main.go` (or wherever the root command lives) —
  invoke `sync.EnsureSynced()` at startup unless
  `GH_OPTIVEM_NO_AUTO_SYNC` is truthy.
- `cmd/gh-optivem/atdd_sync.go` — new subcommand
  `gh optivem atdd sync` for explicit sync.
- ATDD-consuming commands (notably `gh optivem implement`) — when the
  escape hatch is set, check the stamp and fail fast with the
  documented error if stale.

**D5 — placeholder substitution for doc paths:**
- `internal/atdd/runtime/clauderun/clauderun.go:432-448` — add
  `atdd_docs_root` to the fixed-schema placeholders, populated with
  the absolute path of `~/.gh-optivem/docs/atdd/`.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — extend the
  `TestRenderPrompt_*` cases with `${atdd_docs_root}` substitution
  assertions.

**D7 + D8 — prompt file splits:**
- `internal/atdd/runtime/agents/prompts/atdd-dsl-at.md` — new
  (AT-cycle half of `atdd-dsl.md`).
- `internal/atdd/runtime/agents/prompts/atdd-dsl-ct.md` — new
  (CT-cycle half of `atdd-dsl.md`).
- `internal/atdd/runtime/agents/prompts/atdd-driver-at.md`,
  `atdd-driver-ct.md` — same pattern.
- `internal/atdd/runtime/agents/prompts/atdd-test-at.md`,
  `atdd-test-ct.md` — same pattern.
- `internal/atdd/runtime/agents/prompts/atdd-task-system-interface-redesign.md` —
  new (atdd-task.md minus the gated block).
- `internal/atdd/runtime/agents/prompts/atdd-task-external-system-interface-redesign.md` —
  new (atdd-task.md with gated block inlined).
- Delete: `atdd-dsl.md`, `atdd-driver.md`, `atdd-test.md`,
  `atdd-task.md`.
- Dispatcher — `internal/atdd/runtime/clauderun/clauderun.go` or
  the agent resolution layer — extend the agent-name lookup to map
  `(phase, cycle)` and `(task, subtype)` to the right file.

**D5 + D6 — prompt body slimming:**
- `internal/atdd/runtime/agents/prompts/atdd-dsl-{at,ct}.md` — strip
  inlined references, keep guardrails inline.
- `internal/atdd/runtime/agents/prompts/atdd-driver-{at,ct}.md` — same.
- `internal/atdd/runtime/agents/prompts/atdd-test-{at,ct}.md` — same.
- `internal/atdd/runtime/agents/prompts/atdd-task-*.md` — strip inlined
  `system.md`, `driver-port.md`, `driver-adapter.md` references.
- `internal/atdd/runtime/agents/prompts/atdd-chore.md` — strip inlined
  `task-and-chore-cycles.md`.
- `internal/atdd/runtime/agents/prompts/atdd-backend.md`,
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
  placeholder map; `${atdd_docs_root}` joins this set.
- `internal/atdd/runtime/clauderun/clauderun.go:264-269` —
  `findUnfilledPlaceholders` already guards against typos; the new
  `${atdd_docs_root}` doesn't break the guarantee.
- `atdd-fix-verify.md:178` / `atdd-task.md:178` — precedent for
  "reference exists at a known path; read with the Read tool" —
  generalized by D5.

## Steps

### Step 1 — D1 + D2 (ownership move + embed expansion)

Land as one bundle (the move is non-atomic if split — every consumer
would see a window where docs are gone). Sub-steps:

1. Create `internal/atdd/assets/` with the three asset trees moved
   from shop.
2. Add `internal/atdd/assets/embed.go` with `//go:embed assets/...`.
3. Update `internal/atdd/install.go` (or remove the ATDD-asset half).
4. Update `internal/atdd/install_test.go` fixtures.
5. Delete the original copies in shop; leave a pointer-README.

**Validation:**
- Build succeeds; embed.FS contains every asset.
- shop becomes equivalent to any other consumer after the change.

### Step 2 — D3 (auto-sync mechanism)

1. Create `internal/atdd/sync/sync.go` with `EnsureSynced()` and
   atomic-write + lock support.
2. Wire `sync.EnsureSynced()` into `cmd/gh-optivem/main.go` startup,
   gated by `GH_OPTIVEM_NO_AUTO_SYNC`.
3. Add `gh optivem atdd sync` subcommand.
4. Add the staleness-error in ATDD-consuming commands for the
   escape-hatch path.

**Validation:**
- First invocation after install writes the three target trees and
  the stamp file.
- Subsequent invocation is a no-op (stamp matches).
- Simulated version bump triggers re-sync.
- Concurrent invocation under file-lock test: no torn writes.
- `GH_OPTIVEM_NO_AUTO_SYNC=1` skips sync; `implement` then fails
  with the documented error when stale.

### Step 3 — D5 (`${atdd_docs_root}` placeholder)

Add the placeholder to `renderPrompt`'s fixed-schema map, populate
with `~/.gh-optivem/docs/atdd/`. Add `TestRenderPrompt_*` case.

**Validation:**
- Rendered prompts contain absolute paths.
- `findUnfilledPlaceholders` returns empty.

### Step 4 — D7 + D8 (file splits)

Split `atdd-dsl`, `atdd-driver`, `atdd-test` into per-cycle files.
Split `atdd-task` into per-subtype files. Update the dispatcher's
agent-name lookup to map phase/cycle/subtype to filename.

**Validation:**
- Each new prompt file reads top-to-bottom without conditionals.
- AT-cycle dispatch picks the `-at.md` file; CT-cycle picks `-ct.md`.
- Task dispatch picks the right subtype file.
- `TestRenderPrompt_*` updated to cover each variant.

### Step 5 — filterConditionals deletion

Delete `conditionalRE`, `filterConditionals`, the
`body = filterConditionals(...)` call in `renderPrompt`, and the
conditional-gating tests.

**Validation:**
- Build succeeds.
- No `<!-- if -->` markers remain in any prompt file
  (`rg 'if:[a-z_]+=' internal/atdd/runtime/agents/prompts/` returns
  empty).

### Step 6 — D6 + D9 (strip inlined references + prose copyedit)

Per-prompt copyedit:
- Replace each `### Reference: ...` block with a one-line
  `${atdd_docs_root}/...` pointer.
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
- `gh optivem atdd docs <slug>` / `--open` helpers and
  `gh optivem atdd status`. Nice-to-have, can land in a follow-up.
