# 2026-08-13 07:12:00 UTC — Fix TestVerifyEnvironment_DockerDaemonTimeout failing on Linux CI

## TL;DR

**Why:** `TestVerifyEnvironment_DockerDaemonTimeout` fails on the Linux GitHub Actions runner (run `31675813045`, job `94370023718`) with `verify_environment_tools_test.go:624: expected error when docker info times out, got nil`. The non-Windows docker stub body is `"sleep 2\nexit 0"` (`internal/config/verify_environment_tools_test.go:613`), but `mkPathDir` restricts `PATH` to only the stub's temp dir for the test's duration, so the stub script's own bare `sleep` call can't be resolved by `/bin/sh`. With no `set -e`, the failed `sleep` is silently skipped and the script falls straight through to `exit 0` in milliseconds — well inside the test's shrunk 200ms `dockerProbeTimeout` — so `probeDockerDaemon` sees a clean, fast success instead of a timeout and returns `nil`. The Windows branch of the same test already solved this exact PATH-shadowing problem for `ping` by using an absolute path (`"%SystemRoot%\System32\ping.exe"`); that fix was never mirrored on the Linux/macOS branch's `sleep` call. This is deterministic, not a flake — the test was added in commit `eefc0b25` and has never actually passed on Linux CI.
**End result:** The non-Windows stub body calls `sleep` via an absolute path so it genuinely blocks past the shrunk timeout regardless of the test's restricted `PATH`, `TestVerifyEnvironment_DockerDaemonTimeout` passes on Linux CI, and the full `go test ./...` suite is green.

## Outcomes

- `TestVerifyEnvironment_DockerDaemonTimeout` reliably exercises the real timeout/kill path on Linux and macOS CI runners, not just Windows.
- The fix mirrors the existing, already-working pattern used by the Windows branch of the same test, rather than introducing a new approach.
- No other test in the package uses a bare (unresolvable-under-restricted-PATH) external command inside a stub body — confirmed via search, this was the only occurrence of `sleep` in stub bodies.

## ▶ Next executable step (resume here)

All local work is done (line fixed, full `go test ./...` green on Windows). Step 2 needs a Linux CI run to confirm — commit and push, then watch the next Actions run for `TestVerifyEnvironment_DockerDaemonTimeout` on the Linux job. If it passes, delete this plan file; if it still fails, reopen investigation.

## Steps

- [ ] Step 2: Run `go test ./internal/config/... -run TestVerifyEnvironment_DockerDaemonTimeout -v` in CI (Linux) — this fix targets a Linux/macOS-only code path and cannot be meaningfully verified on the Windows dev box, where the test already takes the `ping.exe` branch regardless of this change. ⏳ Deferred: requires a Linux CI run; local Windows box only exercises the `ping.exe` branch.

## Open questions

- None — the root cause is pinned to `file:line`, the fix mirrors an established, working pattern in the same test, and the scope is a single line in a single file.
