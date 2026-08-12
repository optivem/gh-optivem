# 2026-08-12 19:07:04 UTC — Fix the unquoted-shell `gh` stub bodies that break the commit stage on Linux

## TL;DR

**Why:** Commit-stage run [31629409687](https://github.com/optivem/gh-optivem/actions/runs/31629409687) fails in `internal/config` because two `gh` test stubs are written as raw unquoted shell — the `(` in `(HTTP 404)` is a POSIX-shell syntax error, so the stub dies with a parser error instead of emitting the message the test asserts on. The bug is invisible on Windows (where `writeStub` emits a `.bat` and `cmd.exe` echoes `(` literally), so it only shows up in CI.

**End result:** `TestRealCheckOwnerExists_NotFound` and `TestRealCheckOwnerExists_AuthFailureIsNotAVerdict` pass on both Windows and Linux, the commit stage is green, and `writeStub` fails loud on any future body that is not valid POSIX sh — catching the next occurrence on the dev box instead of 16 minutes into CI.

## Outcomes

- The `gh` stub bodies in `check_owner_exists_test.go` parse correctly under both `/bin/sh` and `cmd.exe`, so the two tests exercise the branches they were written for (definitive 404 verdict; 401 is not a verdict) on every platform.
- `gh-commit-stage.yml` runs green on `main` again — the *Unit tests* step no longer reports `FAIL github.com/optivem/gh-optivem/internal/config`.
- Any future `writeStub` body containing a shell syntax error fails immediately with `sh`'s own parser message, on Windows and Linux alike, rather than surfacing as a confusing downstream assertion failure.
- No production-code change: `realCheckOwnerExists` / `isGhNotFound` behaved correctly throughout — this is a test-authoring defect only.

## Context: the pinned root cause

- `internal/config/check_owner_exists_test.go:12` and `:30` pass raw unquoted shell as the fake `gh` body:
  ```go
  writeStub(t, dir, "gh", "echo gh: Not Found (HTTP 404) >&2\nexit 1")
  writeStub(t, dir, "gh", "echo gh: Requires authentication (HTTP 401) >&2\nexit 1")
  ```
- `writeStub` (`internal/config/testutil_stub_binary_test.go:23`) writes that body verbatim after a `#!/bin/sh` shebang on non-Windows. The unquoted `(` is a syntax error, so the stub exits 2 emitting `Syntax error: "(" unexpected` instead of the intended `gh:` line.
- `isGhNotFound` (`internal/config/config.go:1017`) then correctly sees no `(HTTP 404)` in stderr and correctly reports the probe as undecided — the assertions fail because the stub lied, not because the code is wrong.
- On Windows, `writeStub` emits a `.bat` (`testutil_stub_binary_test.go:20`) where `cmd.exe` echoes `(` literally, so all three tests PASS locally. Platform-divergent by construction.
- Introduced by commit `d254804b` ("Fail loud when the --owner probe cannot reach GitHub").
- Go-only repo, no parallel-language twin. All ~50 `writeStub` call sites were grepped: only these two bodies contain shell metacharacters; the rest are bare-word `echo` / `exit N`.
- Verified repro: running the body under `sh` gives `syntax error near unexpected token '('` (exit 2); the same tests pass under `go test` on Windows. The fix shape was verified on both platforms — under `sh` the quoted body prints `gh: Not Found (HTTP 404)`, and under `cmd.exe` it prints `"gh: Not Found (HTTP 404)"` with the literal quotes included; every assertion is a `strings.Contains` on `(HTTP 404)` / `HTTP 401`, so both pass.

## ▶ Next executable step (resume here)

Execute Step 1: in `internal/config/check_owner_exists_test.go`, double-quote the echoed message in both `writeStub` bodies (lines 12 and 30) so the body parses under `/bin/sh`, and add a one-line comment recording that a stub body must parse as *both* `/bin/sh` and `cmd.exe`. This is the change that turns CI green; Step 2 is the prevention guard and Step 3 is verification.

## Steps

- [ ] Step 1 — Quote the stub bodies (`internal/config/check_owner_exists_test.go`, Go).
  - Line 12: `writeStub(t, dir, "gh", "echo \"gh: Not Found (HTTP 404)\" >&2\nexit 1")`
  - Line 30: `writeStub(t, dir, "gh", "echo \"gh: Requires authentication (HTTP 401)\" >&2\nexit 1")`
  - Add a short comment (once, near the first call site) noting the body must parse as both `/bin/sh` and `cmd.exe`, and that Windows echoes the literal quotes — harmless, because the assertions are substring checks on `(HTTP 404)` / `HTTP 401`.

- [ ] Step 2 — Make `writeStub` reject a body that is not valid POSIX sh (`internal/config/testutil_stub_binary_test.go`, Go).
  - Resolve `sh` once at package-var scope: `var shPath, _ = exec.LookPath("sh")`. It **must** be a package-level var, not a lookup inside `writeStub` — `mkPathDir` calls `t.Setenv("PATH", dir)` with an empty temp dir, so any `LookPath` performed during a test would fail. Package-var init runs before any test.
  - Inside `writeStub`, when `shPath != ""`: write the body (with the `#!/bin/sh` shebang) to a temp file and run `exec.Command(shPath, "-n", tmp)` with stderr captured. On non-nil error, `t.Fatalf` with `sh`'s own stderr so the author sees the parser message directly.
  - When `shPath == ""`, skip the check silently. `sh` is present on the Windows dev box via Git Bash (`C:\Program Files\Git\usr\bin\sh.exe`) and always on the Linux runner, so the guard is effectively always on.
  - Document in the helper's doc comment that the guard validates the `/bin/sh` form only — it does **not** validate the `cmd.exe` `.bat` form.

- [ ] Step 3 — Verify locally before pushing.
  - `go test ./internal/config/ -run TestRealCheckOwnerExists -v -p 2` — all three tests pass on Windows.
  - `go test ./internal/config/ -p 2` — the whole package passes, confirming the new `writeStub` guard does not trip any of the other ~50 stub call sites.
  - Spot-check both corrected bodies under a real POSIX shell: write each to a file behind a `#!/bin/sh` shebang and confirm `sh` emits the intended `gh: ...` line on stderr with exit 1.
  - Never run unbounded `go test ./...` on Windows — always scope to the package or pass `-p 2`.

## Verification

- After the change is pushed, confirm the `gh-commit-stage.yml` run on `main` goes green — specifically that the *Unit tests* step no longer reports `FAIL github.com/optivem/gh-optivem/internal/config`. Do not poll `gh` faster than once every 2 minutes.
