# 2026-05-20 11:45 UTC — Rename asset tree to `internal/assets/runtime/references/`

**Status:** READY (small dated plan; not yet picked up)

**Origin:** promoted from Items 2 + 3 of `plans/20260519-1537-post-meta-bpmn-topics.md` (DECIDED, ready for promotion). This plan captures only the rename mechanics; the *why* (architectural symmetry, three delivery mechanisms → three sibling trees under `runtime/`, `references/` accurately names the surviving content) lives in the source plan and is not duplicated here.

---

## Decision

Rename the surviving doctrine/reference assets so all three delivery mechanisms are siblings under `internal/assets/runtime/`:

- `internal/assets/global/docs/atdd/` → `internal/assets/runtime/references/atdd/`
- `internal/assets/code/` → `internal/assets/runtime/references/code/`

User-visible sync target shifts accordingly:

- `~/.gh-optivem/docs/` → `~/.gh-optivem/references/atdd/` (and a sibling `references/code/` if/when the code tree gets synced)

Names rejected (per source plan): `doctrine/` (no prose doctrine survives), `shared/` (already taken by `runtime/shared/` for argv-injected content).

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

### Go code

- `internal/assets/embed.go` — update embed roots to point at the new paths.
- `internal/assets/sync/sync.go`, `internal/assets/sync/materialize.go`, `internal/assets/sync/sync_test.go` — update source paths AND target paths.
- Any other Go file that reads `${docs_root}/...` literals — verify with `grep -rn 'global/docs\|assets/code' internal/`.

### Prompt files

- ~25 `Read ${docs_root}/atdd/architecture/...` and `Read ${docs_root}/atdd/code/language-equivalents/...` lines across ~10 prompt files under `internal/assets/runtime/prompts/atdd/`. Update path segments to match the new tree shape.
- Verify exact count + file list before editing: `grep -rn '\${docs_root}' internal/assets/runtime/prompts/atdd/`.

### User-facing surfaces

- Any doc that names the sync target path (`~/.gh-optivem/docs/`).
- Scaffolded-repo expectations + shop-template expectations of the sync target shape.

### Tests

- `internal/assets/sync/sync_test.go` — already listed above, but flagged separately because it asserts target-path shape and will need fixture updates.
- Any acceptance test that touches the synced layout under `~/.gh-optivem/`.

---

## Open decision in this plan

**`path-keys.md` placement.** Currently at `internal/assets/global/docs/atdd/process/path-keys.md`; consumed by Go code, not by prompts. Three options:

1. Follow the rename into `internal/assets/runtime/references/atdd/process/path-keys.md` — preserves the `process/` subdir under `references/` purely for this one file.
2. Promote to its own subtree (e.g., `internal/assets/runtime/references/atdd/path-keys.md`) — drops the `process/` subdir since it's the last surviving file there.
3. Move out of the references tree entirely (e.g., back into a `internal/` Go-only location) since it's not a prompt-readable reference at all.

Decide inside this plan, not as a follow-up. Recommendation: option 2 or 3 — option 1 perpetuates an empty-shell subdir.

---

## Known incidental cleanups (do, don't expand scope)

- After the move, `internal/assets/global/` becomes empty (or near-empty). Delete the empty directory; don't leave a stub. Per memory `feedback_drop_dont_relocate.md`, check if anything still references `global/` before deleting.
- Same for `internal/assets/code/` once its contents move.

---

## Done when

- All embed roots, sync paths, prompt `Read` lines, and test fixtures point at the new tree.
- `grep -rn 'global/docs\|assets/code/\|/\.gh-optivem/docs/' internal/ docs/` returns no stale references (or only intentional historical references, e.g., changelog entries).
- `go test ./...` (with `-p 2` per memory) passes.
- One acceptance run against the shop template confirms the new sync target shape works end-to-end.
- A short note added to the next changelog / release entry about the user-visible sync path change.

---

## Non-goals

- Inlining further doctrine into prompts (that work is Item 5's residual + the orphan-promotion plan; do not piggyback here).
- Restructuring the `references/atdd/architecture/` content itself — straight rename only, no content edits.
- Per-component fanout (Item 6 follow-up) — orthogonal.
