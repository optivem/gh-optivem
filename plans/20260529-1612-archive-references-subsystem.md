# Archive the references subsystem and purge it from Go

**Created:** 2026-05-29 16:12 (local, UTC+2)
**Cross-references:**
- `plans/20260529-1611-fix-wip-gate-snippet-additive.md` — independent sibling (the urgent production
  fix); recommended to land that first. This plan has no dependency on it beyond a shared origin.

## Why this plan exists

While investigating a rehearsal failure, an audit found the `references` asset tree is **dead from the
consumer side**:

- `${references-root}` is referenced by **zero** markdown assets — no agent prompt, no YAML config
  points at `references/`, `language-equivalents`, the `atdd/architecture/*.md` docs, or
  `code/testkit-*.md`.
- The `internal/assets/sync` package still writes those docs to `~/.gh-optivem/references/` on every
  invocation (`EnsureSynced` at `main.go:102`) and to `<repo>/.gh-optivem/references/` on every Dispatch
  (`MaterializeProject` in `clauderun.go`), but nothing reads the output.
- `//go:embed runtime` is the only embed, and `embeddedReferencesRoot = "runtime/references"` is the
  sync package's only payload. `ForceSync` has no caller. The `EscapeHatchHint` string mentions
  `gh optivem asset sync`, a command that does not exist in the codebase.

So archiving the docs cleanly retires the entire sync/materialize subsystem.

## Decision (resolved with the user 2026-05-29)

**Move** `runtime/references/` to a top-level `archive/` dir (preserved in-tree, no longer
embedded/shipped/synced), retire the now-purposeless sync subsystem, and **delete all references prose
from Go comments**.

## Items

1. **Move `internal/assets/runtime/references/` → top-level `archive/references/`** (outside
   `internal/assets/`, so `//go:embed runtime` no longer ships it). Use `git mv` to preserve history.
   Add `archive/README.md` noting the tree is retired/unused, kept for reference only.

2. **Delete the `internal/assets/sync` package** (`sync.go`, `materialize.go`, and their tests) — its
   only payload was `runtime/references`. Removes `EnsureSynced`, `ForceSync` (already dead),
   `ReferencesRoot`, `MaterializeProject`, `ProjectReferencesRoot`, `Stale`, the
   `GH_OPTIVEM_NO_AUTO_SYNC` escape hatch, and the stamp/sidecar machinery.

3. **Remove the `EnsureSynced` call at `main.go:102`** (plus its `Result`/notice handling and the
   `assetsync` import).

4. **Remove the references plumbing from `internal/atdd/runtime/clauderun/clauderun.go`:**
   - the `MaterializeProject` call + `projectReferencesRoot` threading in `Dispatch` (~585–605);
   - the `${references-root}` substitution in `renderPromptWithReferencesRoot` (~764–783) and the
     `referencesRoot` fallback;
   - collapse `renderPromptWithReferencesRoot`/`renderPrompt` to a single renderer if
     `projectReferencesRoot` has no caller left.

5. **Purge references prose from Go comments** in `internal/assets/embed.go` (the `runtime/references/`
   bullet), `internal/atdd/runtime/driver/driver.go:555`, and any residual mentions surfaced by
   `grep -rn "references-root\|references/\|MaterializeProject\|EnsureSynced" --include=*.go`.

6. **Confirm no stale command/help references** to `asset sync` or `GH_OPTIVEM_NO_AUTO_SYNC` remain
   after the sync package is deleted (the `EscapeHatchHint` string goes with it).

## Note on `renderGateMarkerExample` / language-equivalents

The to-be-archived `references/code/language-equivalents/<lang>.md` files currently hold a *duplicate*
copy of the WIP-gate syntax. The sibling plan (`...-fix-wip-gate-snippet-additive.md`) moves the live
copy into `shared/wip-gate-<lang>.md`. Once that plan lands, archiving these docs removes the duplicate
SSoT rather than the live source — confirm the sibling plan has moved the snippet before relying on
this. If this plan lands first, the Go literal in `renderGateMarkerExample` is still the live source and
is untouched here.

## Verification

- `go build ./...` and `go vet ./...` clean (catches orphaned imports / dead params).
- Scoped `go test` on `clauderun`, `agents`, `driver`, and `main` packages.
- `grep -rn "references-root\|runtime/references\|MaterializeProject\|EnsureSynced\|GH_OPTIVEM_NO_AUTO_SYNC"`
  over `internal/ cmd/ main.go --include=*.go` returns nothing.
- A full ATDD dispatch still renders prompts with no unfilled placeholders (no prompt referenced
  `${references-root}`, so this should be a no-op — confirm).
