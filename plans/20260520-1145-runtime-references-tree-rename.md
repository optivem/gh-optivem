# 2026-05-20 11:45 UTC — Rename asset tree to `internal/assets/runtime/references/`

> 🤖 **Picked up by agent (execute, batch-then-review)** — `Valentina_Desk` at `2026-05-20T10:55:30Z`

**Status:** READY (refined; not yet picked up for execution)

**Origin:** promoted from Items 2 + 3 of `plans/20260519-1537-post-meta-bpmn-topics.md` (DECIDED, ready for promotion). This plan captures only the rename mechanics; the *why* (architectural symmetry, three delivery mechanisms → three sibling trees under `runtime/`, `references/` accurately names the surviving content) lives in the source plan and is not duplicated here.

---

## Decision

Rename the surviving doctrine/reference assets so all three delivery mechanisms are siblings under `internal/assets/runtime/`:

- `internal/assets/global/docs/atdd/` → `internal/assets/runtime/references/atdd/`
- `internal/assets/code/` → `internal/assets/runtime/references/code/`

User-visible sync target shifts accordingly:

- `~/.gh-optivem/docs/` → `~/.gh-optivem/references/atdd/` (and a sibling `references/code/` if/when the code tree gets synced)

Names rejected (per source plan): `doctrine/` (no prose doctrine survives), `shared/` (already taken by `runtime/shared/` for argv-injected content).

**One exception to the rename:** `path-keys.md` does **not** move into `references/`; it leaves the embed/sync tree entirely. See the Resolved decision section below.

---

## Sequencing

**Blocked by:** `plans/20260520-0907-runtime-shared-scope-injection.md` — land that first so `scope.md` is firmly in `runtime/shared/` before this rename touches anything else.

**Does NOT block / not blocked by:**

- Items 7 + 8 promotion (`plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`) — those files leave the tree (inlined into prompts), not move within it.
- AT_GREEN collapse plan (Item 6 immediate, drafted in parallel by another agent) — orthogonal; touches `process-flow.yaml`, not the asset tree.
- chore-rename plan (`plans/20260520-1145-system-implementation-refactoring-rename.md`) — orthogonal; touches prompts under `runtime/prompts/atdd/`, not under `global/docs/`.

---

## Surfaces to touch

### Source-tree moves

- `internal/assets/global/docs/atdd/architecture/{driver-adapter,driver-port,dsl-core,dsl-port,system,test}.md` → `internal/assets/runtime/references/atdd/architecture/…`
- `internal/assets/code/language-equivalents/{java,csharp,typescript,README}.md` → `internal/assets/runtime/references/code/language-equivalents/…`
- `internal/assets/code/testkit-{architecture-rules,language-exceptions}.md` → `internal/assets/runtime/references/code/testkit-{architecture-rules,language-exceptions}.md`
- **Exception:** `internal/assets/global/docs/atdd/process/path-keys.md` → `internal/projectconfig/path-keys.md` (leaves the embed/sync tree; see Resolved decision section).

### Go code

- `internal/assets/embed.go` — update the `//go:embed` directive AND the package comment block (which currently describes `global/   — synced to ~/.gh-optivem/docs/`).
- `internal/assets/sync/sync.go`, `internal/assets/sync/materialize.go`, `internal/assets/sync/sync_test.go` — update source paths AND target paths. (`sync_test.go` has bare `global/` and `.gh-optivem/docs/` references that the narrower grep below won't catch — read the file directly.)
- `internal/projectconfig/config.go`, `internal/projectconfig/paths_defaults.go`, `internal/projectconfig/paths_defaults_test.go` — reference `internal/assets/global/docs/atdd/process/path-keys.md` in comments and one user-facing error message. Update in lockstep with item 8's decision on where `path-keys.md` lands.
- `main.go`, `asset_commands.go`, `implement_commands.go` — code comments referencing `~/.gh-optivem/docs/`. (`asset_commands.go` also has a user-visible `Long:` help string — listed separately under "User-facing surfaces".)
- `internal/atdd/runtime/clauderun/clauderun.go`, `internal/atdd/runtime/clauderun/clauderun_test.go` — code comments AND a real path assertion in the test (`filepath.Join(".gh-optivem", "docs")`); the assertion will fail until updated.
- Any other Go file that reads `${docs_root}/...` literals — verify with `grep -rn 'global/docs\|global/\|assets/code\|\.gh-optivem/docs\|path-keys\.md' internal/ main.go *_commands.go` (the broadened pattern catches the bare-`global/` and user-visible-path cases the narrower pattern missed; expanded scope picks up top-level Go files).

### Prompt files

- ~25 `Read ${docs_root}/atdd/architecture/...` and `Read ${docs_root}/atdd/code/language-equivalents/...` lines across ~10 prompt files under `internal/assets/runtime/prompts/atdd/`. Update path segments to match the new tree shape.
- Verify exact count + file list before editing: `grep -rn '\${docs_root}' internal/assets/runtime/prompts/atdd/`.

### User-facing surfaces

- `README.md` — two explicit `~/.gh-optivem/docs/` mentions (the auto-sync sentence and the `gh optivem asset sync` example).
- `asset_commands.go` — the `Long:` help string (`Walk the binary's embedded global/ tree and write methodology docs to ~/.gh-optivem/docs/.`) shown via `gh optivem asset sync --help`.
- Scaffolded-repo / shop-template expectations: verified empty (`grep` across `../shop`, `../courses`, `../hub` returned zero references to `.gh-optivem/docs`). No external-repo changes needed.

---

## Resolved decision: `path-keys.md` placement

**Decision: Option 3 — move out of the embed/sync tree entirely.**

New location: `internal/projectconfig/path-keys.md` (sits next to the Go code that references it in comments + error messages).

**Rationale (verified against current code, not the plan):**

- Source location today: `internal/assets/global/docs/atdd/process/path-keys.md`. Embedded via `//go:embed global` (`internal/assets/embed.go:18`) and synced to `~/.gh-optivem/docs/atdd/process/path-keys.md` via the `~/.gh-optivem/docs/atdd` owned subdir (`internal/assets/sync/sync.go:69`).
- **No runtime code consumes it.** All 5 references in this repo are comments or strings — no `os.ReadFile`, no embed-FS read, no prompt-side `Read ${docs_root}/...` line.
- **No agent/prompt consumes it.** Verified across `internal/assets/runtime/prompts/`, `internal/assets/runtime/shared/`, and `.claude/agents/`.
- **The error message at `config.go:780` already points to the repo path**, not the synced user path. Users hitting the error today already can't navigate to a local copy without cloning the repo — the sync is incidental.

So `path-keys.md` is a developer-facing reference doc accidentally riding the sync pipeline. Moving it out of the embed tree (option 3) aligns with how it's actually used.

**Concrete consequences for this plan:**

- Source-tree move: `internal/assets/global/docs/atdd/process/path-keys.md` → `internal/projectconfig/path-keys.md`. (Not under `runtime/references/`.)
- `embed.go`: no special-case handling — `path-keys.md` simply isn't in the embed tree anymore. Both `global/` and `assets/code/` disappear from the `//go:embed` directive.
- `sync.go` / `materialize.go`: no path-keys-specific changes (it was never explicitly handled — it rode the `docs/atdd/` wipe-and-replace).
- `config.go:780` error message: update the path string from `internal/assets/global/docs/atdd/process/path-keys.md` to `internal/projectconfig/path-keys.md`.
- `config.go`, `paths_defaults.go`, `paths_defaults_test.go` comments: update repo-path references the same way.
- `process/` subdir under `global/docs/atdd/` becomes empty when `path-keys.md` leaves AND the Items 7+8 promotion plan lands (which removes the orphan prompts now under `process/analysis/` and `process/change/behavior/` — `refine-acc.md` plus `update-ticket.md`, `at-refactor.md`; note `refine-acc.md` is the renamed-and-reframed successor to the original `acceptance-criteria-refinement.md` per commit `5b79342`). The empty `process/` subdir disappears with the rest of `global/`.

**Non-goals:** rewriting `path-keys.md` content; adding a GitHub-URL form of the error message; making `path-keys.md` user-discoverable via some other channel. If users start hitting the error in practice and the repo-path string isn't enough, that's a separate follow-up.

---

## Known incidental cleanups (do, don't expand scope)

- After this plan's moves, `internal/assets/code/` becomes empty. Delete the empty directory; don't leave a stub. Per memory `feedback_drop_dont_relocate.md`, check if anything still references `assets/code/` before deleting.
- `internal/assets/global/` deletion is **gated on the Items 7+8 promotion plan also landing** (`plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`). Until that plan lands, `global/docs/atdd/process/` still holds the orphan prompts (`process/analysis/refine-acc.md`, `process/analysis/update-ticket.md`, `process/change/behavior/at-refactor.md`). If this rename lands first, leave `global/` in place; the next plan to land will empty it and can delete it then. If both have already landed when this executes, delete the empty `global/` here. Per memory `feedback_drop_dont_relocate.md`, check for any remaining references before deleting.

---

## Done when

- All affected surfaces point at the new tree: embed roots (`embed.go`), sync paths (`sync.go`, `materialize.go`), prompt `Read` lines (~24 across 10 files), test fixtures (`sync_test.go`, `clauderun_test.go`), user-facing strings (`README.md` + `asset_commands.go` `Long:` help), and the `projectconfig/` references to `path-keys.md` (now at its new Go-only location per item 8).
- `grep -rn 'global/docs\|global/\|assets/code\|\.gh-optivem/docs\|path-keys\.md' internal/ docs/ main.go *_commands.go README.md` returns no stale references (or only intentional historical references, e.g., release-notes entries).
- `go test ./...` (with `-p 2` per memory) passes.
- One acceptance run against the shop template confirms the new sync target shape works end-to-end.
- A short note added to the next **GitHub release notes** (no `CHANGELOG.md` exists; the release-note body on the next tag is the channel) about the user-visible sync path change (`~/.gh-optivem/docs/` → `~/.gh-optivem/references/atdd/`).

---

## Non-goals

- Inlining further doctrine into prompts (that work is the source plan's Item 5 residual + the orphan-promotion plan `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`; do not piggyback here).
- Restructuring the `references/atdd/architecture/` content itself — straight rename only, no content edits.
- Per-component fanout (source plan's Item 6 follow-up) — orthogonal.
