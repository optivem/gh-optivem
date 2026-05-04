# Plan: speed up `go test ./internal/atdd/...` on Windows

**Goal**: cut full-tree runtime from 5+ min and stop the 100% RAM pin. Target: under 60 s, peak RAM under 4 GB.

**Root-cause hypothesis**: 13 sibling packages under `internal/atdd/` → Go links 13 separate static test binaries, by default in parallel up to `NumCPU`. On Windows the linker is the slow phase, and Windows Defender real-time-scans every fresh `.exe` in `%LocalAppData%\Temp\go-build*\` and `%LocalAppData%\go-build\`. Those two effects compound.

The fixes are mostly environment / wrapper changes — the test code itself is fine (hermetic, no real `exec`, no sleeps, no goroutines, no network). Each step is cheap and reversible; do them in order and bail as soon as the runtime is acceptable.

## Step 1 — measure baseline (no changes)

```powershell
go clean -testcache
Measure-Command { go test ./internal/atdd/... } | Select-Object TotalSeconds
```

Record the number and rough peak RAM from Task Manager. We'll compare against this after each step.

## Step 2 — throttle parallel link jobs (zero-risk, in-repo)

```powershell
go test -p 2 ./internal/atdd/...
```

Default `-p` is `NumCPU`. On a 16-core box that's 16 simultaneous link processes. `-p 2` usually halves peak RAM and often runs *faster* because the box stops swapping. If this alone gets you under target, stop here and just document it.

## Step 3 — Windows Defender exclusions (biggest single win, user-machine config)

Settings → Virus & threat protection → Exclusions → add:

- **Folders**: `%LocalAppData%\go-build`, `%USERPROFILE%\go`, the repo root.
- **Processes**: `go.exe`, `link.exe`, `compile.exe`, `gofmt.exe`.

Remeasure. This typically removes 30–60% of wall time for Go-on-Windows builds.

## Step 4 — verify caches are on a fast, non-synced disk

```powershell
go env GOCACHE GOMODCACHE
```

If either points under `OneDrive`, `Dropbox`, or a network drive, move them:

```powershell
setx GOCACHE C:\go-cache
setx GOMODCACHE C:\go-mod
```

Sync clients re-read every cache file Go writes — pathological for build-heavy workloads.

## Step 5 — codify the fast path (in-repo doc change)

Update `CONTRIBUTING.md` § Tests with:

- Default local invocation: `go test -p 2 ./...`.
- One-liner pointing to the Defender exclusion list (steps above).
- "Run only the package you're touching" (`go test ./internal/atdd/runtime/clauderun`) as the dev-loop default; `./...` is for pre-push / CI.

Optionally add `scripts/test.sh` as `go test -p ${GO_TEST_P:-2} "$@"` so `bash scripts/test.sh ./internal/atdd/...` is the documented entry point. Keep CI workflows untouched — Linux runners don't have the linker/Defender problem and benefit from full parallelism.

## Step 6 — only if Steps 2–4 don't get you to target

Audit `internal/atdd/runtime/`'s 13-package layout. Several are tiny binding/adapter packages (`actions`, `gates`, `verify`, `release`, `classify`, `intake`, `board`) that exist mostly to keep imports acyclic. Merging the trivial ones into `runtime` directly reduces the per-binary link cost. This is invasive and changes the public-ish package surface, so only do it if the cheap fixes prove insufficient.

## Out of scope

- Changing CI parallelism (Linux is fine).
- Rewriting tests — they're already hermetic; the cost is in build/link, not execution.
- `t.Parallel()` — only 2 files use it; not the bottleneck.
