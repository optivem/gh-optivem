# 2026-09-25 10:38:54 UTC — Retry gh's "unexpected end of JSON input" in Go, and stop the Go/bash retry lists drifting apart

## TL;DR

**Why:** gh-acceptance-stage run 35614000906 failed one matrix job (monolith, monorepo, java, typescript) because a single slow GitHub API call, `gh api repos/<owner>/<repo>/environments/production -X PUT`, came back with an empty body. gh reported that as `unexpected end of JSON input`, and the Go retry classifier (`internal/kernel/shell/retry.go:21-34`) does not treat that phrase as temporary, so `MustRunWithRetry` (called from `GitHub.CreateEnvironment`, `internal/kernel/shell/github.go:601`) stopped the program on the first try. The bash copy of the list (`.github/scripts/retry.sh:60-68`) already retries this phrase. The Go copy fell behind it.
**End result:** the Go transient and hard-fail lists match `retry.sh`, so the same failure is retried up to 4 times. A new test fails whenever `retry.sh` gains a pattern that Go doesn't cover, unless that difference is on a commented allowlist.

## Outcomes

- A `gh api` call that returns an empty or cut-off body (`unexpected end of JSON input`) is retried with the usual 5s → 15s → 45s backoff instead of failing the scaffold.
- The Go `retryTransient` / `retryHardFail` regexes cover every pattern in the vendored `retry.sh` lists, apart from a short allowlist of intentional differences, each with a comment explaining it.
- `TestClassifyError` pins the exact failing output from run 35614000906, plus one case for each newly added phrase.
- A parity test in `retry_test.go` reads `.github/scripts/retry.sh` and fails loudly on future drift.

## ▶ Next executable step (resume here)

Step 1: in `internal/kernel/shell/retry.go`, list every alternative in `_RETRY_RETRYABLE` and in the hard-fail list of `.github/scripts/retry.sh` next to the Go `retryTransient` / `retryHardFail` alternatives. Add each one Go is missing (at minimum `unexpected end of JSON input`, `Request failed with status code 5\d\d`, `Bootstrapper: An error occurred`, `Error response from daemon: Get "[^"]+": unknown`). Extend the comment in the same style as the existing ones, citing run 35614000906. Then continue with Steps 2–4.

## Steps

- [ ] Step 1: `internal/kernel/shell/retry.go` — build a side-by-side list of the transient **and** hard-fail patterns in bash vs Go. Add the missing transient phrases (`unexpected end of JSON input`, `Request failed with status code 5\d\d`, `Bootstrapper: An error occurred`, `Error response from daemon: Get "[^"]+": unknown`) and any missing hard-fail phrases. Update the comment to cite run 35614000906 (job 106390679078, `CreateEnvironment` PUT).
- [ ] Step 2: `internal/kernel/shell/retry_test.go` `TestClassifyError` (line 9) — add a case for the exact failing output `command failed: gh api repos/o/r/environments/production -X PUT: exit status 1\nunexpected end of JSON input` (expected: transient), plus one case for each newly added transient or hard-fail phrase.
- [ ] Step 3: `internal/kernel/shell/retry_test.go` — add `TestRetryPatternsMatchVendoredRetrySh`. It reads `.github/scripts/retry.sh` (path resolved from the test file's location, not hardcoded), pulls out the `_RETRY_RETRYABLE` and hard-fail `'...'` strings, splits them on top-level `|`, converts POSIX `[0-9]` ↔ `\d` as needed, and checks that each alternative is either present in the Go regex source or on a commented `intentionalDifferences` allowlist. When it fails, the message names the missing pattern and says whether to add it to Go or to the allowlist.
- [ ] Step 4: Verify: `go test -p 2 ./internal/kernel/shell/...` and `go vet ./internal/kernel/shell/...` (never run an unbounded `go test ./...` on Windows).

## Verification

- The user re-runs `gh-acceptance-stage` after this lands. The (monolith, monorepo, java, typescript) job should pass. If the same GitHub API problem happens again, the log should show `retry` attempts rather than an immediate FATAL.

## Open questions

- Parity-test matching strategy (inferred, not stated): compare pattern **source text** (after normalizing `[0-9]`→`\d`) instead of compiling each bash alternative and matching it against sample strings. Source comparison is simpler and deterministic. The cost is that it may need a few allowlist entries where Go words a pattern differently (e.g. Go's case-insensitive `connection reset` already covers bash's `Connection reset by peer`). Recommended: source comparison plus the allowlist.
