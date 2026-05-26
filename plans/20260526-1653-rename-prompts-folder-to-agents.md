# Rename `assets/runtime/prompts/atdd/` → `assets/runtime/agents/atdd/`

## Origin / intent

Workshop-prep terminology audit (2026-05-26). The 17 `.md` files under
`internal/assets/runtime/prompts/atdd/` are not raw prompts — each
declares a model + effort + scope in YAML frontmatter and is
dispatched by `gh optivem` via `clauderun` as a focused, single-purpose
agent. The codebase already calls them "agents" in many places (e.g.
`driver.go:21` "per-agent prompt", `embed.go` "every agent must
declare its model and effort", `embed_test.go` "every embedded
agent"). The folder name `prompts/` is the outlier — it pre-dates
the consolidation that turned each file into a self-contained agent
spec.

This plan renames the asset folder to match the vocabulary the rest
of the codebase already uses. **Scope is strictly the folder name
and its load paths** — no function renames, no filename renames, no
consumer-repo schema changes.

## Why `agents/` and not `briefs/` / `tasks/` / `instructions/`

Resolved during refinement (2026-05-26):

- `tasks/` — collides with BPMN, where `task` is the node in
  `process-flow.yaml`. Rejected.
- `instructions/` — collides with LLM "system instructions" / "system
  prompt" vocabulary. Rejected.
- `briefs/` — clean and grammatically pure (briefs are named by the
  job, so verb-shaped filenames fit), but introduces *new* vocabulary
  not yet in the codebase. The runtime, comments, tests, and Go
  identifiers already say "agent" everywhere. Rejected in favour of
  matching existing usage.
- `agents/` — matches existing vocabulary (`driver.go`, `embed.go`,
  test names, `agents.Names()`). Cosmetic friction with the
  English-grammar "agent = noun" convention; mitigation: workshop
  one-liner "each agent does exactly one job, so we name it by the
  job."

The BPMN → agent mapping stays explicit via `task-name:` params in
`process-flow.yaml`, so the folder name has no semantic role in
dispatch — it's just where the files live.

## Coexistence with the existing `agents/` directories

After the rename there are three `agents/` directories, each with a
distinct role:

```
internal/atdd/runtime/agents/         — Go package: dispatch registry, embed loader, tuning parser
internal/assets/runtime/agents/atdd/  — markdown agent definitions (this rename's destination)
.claude/agents/                       — Claude Code subagent definitions (Agent-tool target)
```

All three are clearly disambiguated by parent path. No filesystem
collision. The Go package's name is already accurate (it IS the
agent-dispatch package), so no rename there.

## In-flight plan coordination

Two plans committed earlier today touch overlapping surface:

- `plans/20260526-1448-agent-prompt-fixes.md` — edits CONTENT inside
  the 17 `.md` files under the soon-to-be-renamed folder.
- `plans/20260526-1620-strip-process-flow-banner-comments.md` —
  edits `process-flow.yaml`, including comments this plan also
  touches.

**Execution order:** land both of the above first, then run this
plan. This minimises rebase pain and avoids `git mv` interleaving
with content edits.

## Items

### Item 1 — Move the 17 files

`git mv internal/assets/runtime/prompts/atdd/*.md
internal/assets/runtime/agents/atdd/`

Use `git mv` (not Write + delete) so blame history follows the
files. The 17 files (per current tree):

```
disable-tests.md
enable-tests.md
fix-command-failed.md
fix-missing-output.md
fix-scope-diff.md
fix-unexpected-failing-tests.md
fix-unexpected-passing-tests.md
implement-dsl.md
implement-external-system-driver-adapters.md
implement-external-system-stubs.md
implement-system.md
implement-system-driver-adapters.md
refactor-system.md
refactor-tests.md
refine-acceptance-criteria.md
write-acceptance-tests.md
write-contract-tests.md
```

The `prompts/` parent directory becomes empty and `git mv` cleans
it up automatically.

### Item 2 — Update embedded-path constant

**File:** `internal/atdd/runtime/agents/embed.go`

Line 13:
```go
promptsDir     = "runtime/prompts/atdd"
```
becomes
```go
agentsDir      = "runtime/agents/atdd"
```

Replace every in-file reference to `promptsDir` (lines 75, 105, 136,
231, 237) with `agentsDir`.

### Item 3 — Update prose comments in `embed.go`

Replace the verbal references to the old path:

- Line 229 (`Names()` doc): "drop the prompt under
  internal/assets/runtime/prompts/atdd/, recompile" → "drop the agent
  definition under internal/assets/runtime/agents/atdd/, recompile".
- Line 237 panic message:
  `panic("agents: read embedded " + promptsDir + ": " + err.Error())`
  — the variable name change in Item 2 already updates this; verify
  the rendered error string is sensible (`agents: read embedded
  runtime/agents/atdd: …`).

Other comments in the file (lines 67–73, 88–93, 122–134, 178) refer
to "the prompt" / "the prompt frontmatter" / "the agent's prompt".
These remain accurate — the dispatched argv content IS a prompt
even though the file defines an agent. **Do not blanket-rewrite
these to "agent definition";** only update path references.

### Item 4 — Update `embed_test.go`

**File:** `internal/atdd/runtime/agents/embed_test.go`

- Line 145: `TestFixKindPromptsExist` → `TestFixKindAgentsExist`.
- Line 149 (comment inside that test): rewrite the path
  `internal/assets/runtime/prompts/atdd/` → `internal/assets/runtime/agents/atdd/`.
- Line 173 error message: "missing prompt for failure-kind %q" stays
  (the dispatched body IS a prompt at that point); the surrounding
  hint `agents.Names()` is already correct.
- Other tests (`TestPrompt_StripsFrontmatter`, etc.) keep the name
  `Prompt` because the function under test still returns the prompt
  body — see Item 6 for the deliberate non-rename.

### Item 5 — Fix the stale comment in `registry.go`

**File:** `internal/atdd/runtime/agents/registry.go`

Lines 3–4 currently reference a path that has never existed:
```
// `.claude/agents/atdd/<name>.md` agent to dispatch
```

Rewrite to the post-move location:
```
// `internal/assets/runtime/agents/atdd/<name>.md` agent to dispatch
```

This is a drift fix that happens to align with the rename.

### Item 6 — Deliberately NOT renamed

The following stay as-is because they accurately describe what they
do:

- **`Prompt(name string) (string, error)`** (embed.go:74) — returns
  the prompt-body string that gets handed to `claude -p`. The
  *function* returns a prompt; the *file* defines an agent. Keep
  the function name.
- **Go package `agents`** (`internal/atdd/runtime/agents/`) — already
  correct. The package is the agent-dispatch layer.

If the workshop later needs a "BriefFor(name)" or similar alias for
readability, that's a follow-up — not part of this plan.

### Item 7 — Update top-level `assets/embed.go` package doc

**File:** `internal/assets/embed.go`

Lines 5–6:
```
//   - runtime/prompts/    — fed to `claude -p` via argv, never written to
//     disk in consumer repos. Per-phase prompts under runtime/prompts/atdd/.
```
becomes:
```
//   - runtime/agents/     — fed to `claude -p` via argv, never written to
//     disk in consumer repos. Per-phase agent definitions under runtime/agents/atdd/.
```

### Item 8 — Update `driver.go` comments

**File:** `internal/atdd/runtime/driver/driver.go`

- Line 21 (inside the package-level comment block): "embedded per-agent
  prompt (from internal/atdd/runtime/agents/prompts/)" — this is
  stale (path never existed under that prefix; correct old path was
  `internal/assets/runtime/prompts/atdd/`). Rewrite to:
  "embedded per-agent definition (from
  internal/assets/runtime/agents/atdd/)".
- Line 672 (above `registerAgentDispatchers`): "drop a prompt under
  internal/atdd/runtime/agents/prompts/, recompile" — same stale
  path. Rewrite to: "drop an agent definition under
  internal/assets/runtime/agents/atdd/, recompile".

### Item 9 — Update `process-flow.yaml` comments

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Three hits of "prompt" that refer to the renamed files:

- **Line 100**: `→ fix-missing-output.md       (validateOutputsAndScopes — prompt TBD, see plans/upcoming/)`
  → change to `… — agent TBD, see plans/upcoming/`.
- **Line 101**: same shape with `fix-scope-diff.md` — same rewrite.
- **Line 1938**: `# on RUN_AGENT. Prompt path is derived from task-name per Q28.a`
  → `# on RUN_AGENT. Agent definition path is derived from task-name per Q28.a`.

These are the only three. The YAML body itself (node fields, params)
contains no path references that need updating.

### Item 10 — Update in-prompt path references

**Files:**
- `internal/assets/runtime/agents/atdd/fix-scope-diff.md` (post-move location)
- `internal/assets/runtime/agents/atdd/fix-missing-output.md` (post-move location)

Both files reference the old folder path in their body text (lines 19
and 49 in each, per pre-move grep). Replace all four hits:

`internal/assets/runtime/prompts/atdd/<failing-task-name>.md`
→
`internal/assets/runtime/agents/atdd/<failing-task-name>.md`

Plain string substitution — the surrounding sentences ("Read the
prompt to confirm what the agent was supposed to touch", "Open
.../<name>.md") remain accurate.

### Item 11 — Build + test

```
go build ./...
scripts/test.sh    # or: go test -p 2 ./internal/atdd/... ./internal/assets/...
```

Per repo guidance: never `go test ./...` unbounded on Windows.

Expected pass surface:
- `internal/atdd/runtime/agents/...` (embed_test.go + registry tests)
- `internal/atdd/runtime/driver/...` (dispatch wiring)
- `internal/atdd/runtime/statemachine/...` (process-flow load)
- Any consumer of `assets.FS` that reads the embed tree.

### Item 12 — Manual verification

```
gh optivem process show write-acceptance-tests
gh optivem process scope write-acceptance-tests
```

Both should resolve without errors. Output should include references
to the new path (or no path at all — these commands surface scope,
not file location).

## Out of scope (explicitly)

Listed here so the rename doesn't accidentally grow:

- **Consumer-repo schema.** The `gh-optivem.yaml` field `task_prompts:`
  and the documented `config/prompts/<name>.md` convention
  (CONTRIBUTING.md:392, projectconfig/config_test.go:357 et al.)
  are *operator-facing schema* and a separate decision. If renamed,
  it's a breaking change for any existing consumer config.
- **Filename rename to noun forms.** Renaming
  `write-acceptance-tests.md` → `test-writer.md` etc. would align
  with `.claude/agents/*.md` noun convention but is a larger
  architectural conversation (some agents would naturally collapse;
  others wouldn't).
- **`Prompt(name)` function rename.** See Item 6.
- **Historical plans, archived plans, reports.** Files under
  `plans/upcoming/`, `plans/deferred/`, `plans/archived/`,
  `reports/` reference the old path as a state-in-time snapshot.
  Leave them; the rename is not retroactive.

## Rollback

Single `git revert` on the rename commit restores the old folder
and all referenced paths. Asset embedding via `//go:embed runtime`
picks up the restored tree on rebuild — no separate registration to
unwind.

## Pickup marker

(Add `**Pickup:** YYYY-MM-DD HH:MM <agent>` here when execution
begins, per repo plan-execution convention.)
