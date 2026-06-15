# Relocate `internal/assets/` under `internal/atdd/`

**Date:** 2026-06-15 (local)
**Status:** Proposed — design plan, ready for `/refine-plan` or `/execute-plan`.

---

## Problem (why this plan exists)

ATDD is one *process*, and everything about it should live under one roof. Today it
is split across two top-level `internal/` packages whose names collide confusingly:

- `internal/atdd/` — the **Go code** for the process: `process/` (BPMN definition,
  gates, verify, actions) and `runtime/` (the engine: driver, intake, preflight, …).
- `internal/assets/` — the **embedded data** for the process: a `//go:embed runtime`
  tree of `.md` prompt bodies (`runtime/agents/atdd/*.md`) and shared chunks
  (`runtime/shared/*.md`).

The word **"runtime"** means two different things in the two paths —
`atdd/runtime` is "the Go runtime engine"; `assets/runtime` is "assets used at ATDD
runtime". That collision is the wart.

Crucially, the embedded asset tree is **100% ATDD** (no `global/`, no non-ATDD
assets — those older trees referenced in `reports/`, `archive/`, and some backlog
plans no longer exist on disk). So `internal/assets` is "ATDD prompt assets wearing
a generic, central-asset-root name," sitting far from the code it is coupled to and
changes together with.

**Decision (locked, 2026-06-15 Q&A):** ATDD is the only orchestrated process. So we
do **not** introduce a `internal/processes/` namespace (that would be a single-child
wrapper). We simply move the assets under `internal/atdd/`, giving:

```
internal/atdd/
  process/    ← BPMN definition (process-flow.yaml, gates, verify, actions)   [unchanged]
  runtime/    ← Go engine (driver, intake, preflight, release, …)            [unchanged]
  assets/     ← embedded .md prompts + shared chunks   ← MOVED from internal/assets/
```

After the move, the collision reads clearly as **data vs code**: `atdd/assets`
vs `atdd/runtime`.

## Key fact that keeps this cheap

`assets.FS.ReadFile(...)` calls pass paths **relative to the embed root**, e.g.
`"runtime/agents/atdd/<name>.md"`, `"runtime/shared/wip-gate-<lang>.md"`. The
`runtime/` tree moves *together with* `embed.go`, so the `//go:embed runtime`
directive and every one of those path strings stay **byte-for-byte identical**.

The only functional change is the Go **import path**:
`github.com/optivem/gh-optivem/internal/assets`
→ `github.com/optivem/gh-optivem/internal/atdd/assets`.

The package name stays `assets`, so call sites (`assets.FS`) are untouched.

## Scope decisions to confirm during refine

- **Keep the inner `runtime/` folder inside `assets/`?** After the move the path is
  `internal/atdd/assets/runtime/{agents/atdd,shared}`. Since the old sibling tree
  (`global/`) is gone, that `runtime/` layer is now also a single-child wrapper, and
  `assets/runtime/agents/atdd` is deeply nested. **Flattening** it (to e.g.
  `internal/atdd/assets/{agents,shared}` with `//go:embed agents shared`) would
  change every `assets.FS.ReadFile` path string and the embed directive.
  **Recommendation: do NOT flatten in this plan** — keep the move mechanical and
  zero-risk; track flattening as a separate optional follow-up. (Flagged here so the
  decision is explicit, not forgotten.)

---

## Steps

### 1. Move the directory (preserve history)
- `git mv internal/assets internal/atdd/assets`
- Moves `embed.go` + the entire `runtime/` tree. No file *content* changes yet.

### 2. Update the Go import path (functional — 4 files)
Replace `gh-optivem/internal/assets` → `gh-optivem/internal/atdd/assets` in:
- `internal/atdd/phase_scopes_test.go`
- `internal/atdd/runtime/agents/embed.go`
- `internal/atdd/process/clauderun/clauderun.go`
- `internal/atdd/process/clauderun/clauderun_test.go`

(Confirm with a fresh `grep -rl '"github.com/optivem/gh-optivem/internal/assets"'`
before editing, in case new importers appeared.)

### 3. Update the package doc comment in `embed.go`
- `internal/atdd/assets/embed.go` header still says "the single embedded asset tree
  for gh-optivem." Reword to reflect it is the ATDD process's embedded asset tree and
  its new home. Keep the `runtime/agents/` and `runtime/shared/` description accurate.

### 4. Fix Go doc-comment path references (non-functional, for accuracy)
Update literal `internal/assets/runtime/...` → `internal/atdd/assets/runtime/...` in
comments:
- `internal/atdd/runtime/driver/driver.go` (≈ lines 14, 813)
- `internal/atdd/runtime/agents/embed.go` (≈ line 279)
- `internal/atdd/runtime/agents/embed_test.go` (≈ line 149)
- `internal/atdd/runtime/driver/swappable_agentset_test.go` (≈ lines 8, 32)
- `internal/atdd/process/clauderun/clauderun.go` (≈ lines 1068, 1105)
- `internal/atdd/process/clauderun/clauderun_test.go` (≈ line 2094)
- `internal/atdd/process/gates/bindings.go` (≈ line 229)
- `internal/engine/statemachine/types.go` (≈ line 116)
- `internal/atdd/phase_scopes_test.go` (≈ line 259)

(Sweep with `grep -rn 'internal/assets' internal --include=*.go` and fix every hit.)

### 5. Fix the runtime prompt bodies that cite their own path
These `.md` files (now living at the new location) reference the old path inside
their text and must be updated so the dispatched prompt points agents to the right
place:
- `internal/atdd/assets/runtime/agents/atdd/scope-diff-fixer.md`
- `internal/atdd/assets/runtime/agents/atdd/missing-output-fixer.md`

(Sweep `grep -rn 'internal/assets' internal/atdd/assets` for any others.)

### 6. Update the meta audit agents that hardcode the path
The ATDD meta agents glob/cite the prompt tree by literal path and will break or
mislead otherwise:
- `.claude/agents/atdd/meta/bpmn-logic-audit.md`
- `.claude/agents/atdd/meta/runtime-prompts-audit.md`
- `.claude/agents/atdd/meta/process-audit.md` (verify whether it points here)

Note: these live in the `claude` settings source repo and are distributed via
`/sync-claude`. Update them in their source of truth, then sync.

### 7. Leave historical artifacts alone
Do **not** rewrite path references inside `archive/`, `reports/`, or completed/backlog
`plans/*.md` — they are point-in-time records (and some already reference
even-older trees like `internal/assets/global/` that no longer exist). Touching them
rewrites history for no benefit.

### 8. Verify
- `go build ./...`
- `go test ./internal/atdd/...` (covers the embed, driver, clauderun, phase-scope
  tests that exercise `assets.FS`).
- `gofmt` only the Go files actually edited (never a whole dir).
- Sanity: `gh optivem` ATDD entrypoint still resolves agent prompts (the embed
  walk-test in `clauderun_test.go` already asserts every shared/agent asset reads
  back).

---

## Risk / rollback
- **Risk: low.** One `git mv` + an import-path rename; embed directive and all
  embed-relative path strings are unchanged, so the binary ships the identical asset
  bytes.
- **Rollback:** `git mv internal/atdd/assets internal/assets` and revert the import
  edits — or just `git revert` the commit.

## Out of scope (optional follow-ups)
- Flattening the inner `assets/runtime/` wrapper (see scope decision above).
- Any rename of `internal/atdd/process/` (only relevant under a `processes/`
  namespace, which we explicitly rejected).
