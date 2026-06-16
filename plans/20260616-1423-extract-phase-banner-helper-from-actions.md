# 2026-06-16 14:23:00 UTC — Split the `actions` binding package by concern (deferred follow-up)

The original scope of this plan — extracting the `WritePhaseBoundary` phase-banner
helper out of `actions` into `outlog` and repointing the driver — is **done** (see
git history). Only the deferred follow-up below remains.

## Deferred

- [ ] **Split `bindings.go` (~1690 lines) by concern.** Full logic-vs-helper audit of the `actions` package: classify each symbol as **bound logic** (registered `NodeFn`), **computation helper** (`ResolveLayerPaths`, `pathInScope`, `shellEscape`, `dirtyTreePaths`, fingerprint fns), or **infra** (`Deps`, real runners, `RegisterAll`). Promote the existing comment-banner sections into concern files — sketch: `tracker.go`, `command.go`, `scope.go`, `worktree.go` (shared fingerprint machinery), `outputs.go`, `external.go`, plus `deps.go` / `runners.go` / `register.go` for the spine; `fix_progress.go` and `verify_classify.go` already exist. Pure file motion within one package — no signature/behaviour change; `go build` + the existing `actions` tests are the full safety net. ⏳ Deferred: independent of the banner move; do as its own change.
