# Plan: fix gh-optivem Go retry-coverage gaps

Date: 2026-05-14
Driven by: [`audits/20260514-external-call-retry-coverage.md`](../audits/20260514-external-call-retry-coverage.md)
Phase 6 input for: [`plans/20260514-1945-retry-mechanism-end-to-end.md`](20260514-1945-retry-mechanism-end-to-end.md)

## Status (2026-05-15)

**Not started — 0 / 10 items done.** No pickup marker. Verified by checking that `internal/shell/sonarretry.go` (created by Item 1) does not exist and that no commits since the plan was authored touch the affected files in the way the items prescribe.

All 10 items below are still pending.

## Goal

Eliminate every `R-MISSING` finding from the retry-coverage audit. After this
plan lands, every external-I/O call site in `gh-optivem`'s Go code is either:

- Wrapped in `shell.RetryWithPolicy` / `shell.RunWithRetry` /
  `shell.RunCaptureWithRetry` / `shell.MustRunWithRetry` /
  `shell.MustRunStdinWithRetry` / `shell.MustRunPostCreate`, OR
- Explicitly documented `R-DOC-OK` (local-only, probe-by-design, or
  intentional fail-silent for offline cases).

No new retry engine is introduced — every item routes through the existing
`internal/shell/retrycore.go` / `internal/shell/ghretry.go`.

## Constraints

- Order matches the audit's "Recommended order of fixes" (incident-correlated
  first, then leverage, then consistency).
- Each item is independently shippable. Items 1 and 2 share a transient
  regex; define it once in `internal/shell/sonarretry.go` (new file) and
  reuse.
- Tests: every wrap that changes a function's failure semantics needs at
  least one new unit test that asserts the retry loop fires on the chosen
  transient pattern and passes through on the chosen hard-fail pattern.
  Mirror the `internal/shell/github_test.go` pattern (table-driven, sleepFn
  stubbed to no-op).

---

## Items

### 1. Wrap `SonarCloud.api` in `RetryWithPolicy` — `internal/shell/sonarcloud.go`

**Audit ref:** H1.
**Function:** `SonarCloud.api` (lines 34-87).
**Callers (auto-benefit):** `CreateOrg`, `CreateProject` (2× — create + branch
rename), `DeleteProject`, `OrgExists`, `ProjectExists`. 6 call sites with one
fix.

**Change shape:** introduce a new file `internal/shell/sonarretry.go`
exposing:

```go
var sonarRetryTransient = regexp.MustCompile(
    `(?i)HTTP 5\d\d|Bad Gateway|Service Unavailable|Gateway Timeout|` +
        `i/o timeout|timeout|connection reset|connection refused|` +
        `TLS handshake|temporary failure in name resolution|no such host|` +
        `EOF|broken pipe`)

var sonarRetryHardFail = regexp.MustCompile(`(?i)HTTP 4\d\d`)
```

In `SonarCloud.api`, after `creds`/headers are set on the request, wrap the
`s.client.Do(req)` + `io.ReadAll(resp.Body)` block in
`shell.RetryWithPolicy(sonarRetryTransient, sonarRetryHardFail, "sonar-retry", fn)`,
where `fn` returns a string-summary of `(resp.StatusCode, body)` so the regex
sees a string comparable to what bash's `sonar-retry.sh` matches against
(e.g. `"HTTP 504\n<body>"`). On success, return the parsed map as today; on
hard-fail / exhausted retries, return the same `error` shape as today so
callers see no behaviour change for non-transient failures.

**Tests:**
- New `internal/shell/sonarcloud_test.go` (or extend an existing one) with
  a `httptest.NewServer` returning 504 twice then 200; assert one logical
  call observed three HTTP requests.
- Same shape with 401 → assert no retry, error returned immediately.

---

### 2. Wrap `internal/sonar/sonar.go` `Client.do` in `RetryWithPolicy`

**Audit ref:** H2.
**Function:** `Client.do` (lines 104-124).
**Callers (auto-benefit):** `SearchProjects`, `DeleteProject`. 2 call sites,
both invoked by `cleanup_commands.go:runCleanupSonarProjects`.

**Change shape:** import `shell` (or move the regex into a shared location;
`internal/shell` is already a dependency-free target). Wrap the
`c.HTTP.Do(req)` + `io.ReadAll(resp.Body)` + status-code check in
`shell.RetryWithPolicy(sonarRetryTransient, sonarRetryHardFail, "sonar-retry", fn)`
using the same regex pair defined in item 1.

**Tests:** same shape as item 1, against the existing `internal/sonar`
tests' fake transport.

---

### 3. Wrap the three `client.Do` calls in `internal/config/token_auth.go`

**Audit ref:** H3, H4, H5.
**Functions:** `verifyDockerHubAuth` (line 58), `verifySonarToken` (line 87),
`githubUserAuthCheck`'s inner `do()` closure (lines 125-133).

**Change shape:** for each function, wrap the `client.Do(req)` call (and the
subsequent body-read where present) in `shell.RetryWithPolicy` with the
generic transient regex from item 1. For `githubUserAuthCheck`, keep the
existing one-shot 401-retry (lines 142-148) **outside** the new retry layer
— it targets a different failure mode (per-token throttle returning 401, not
5xx) and the two retry layers compose correctly: the outer layer retries 5xx
within one logical attempt; if the layered attempt returns a successful 401,
the existing one-shot 401-retry path takes over.

**Why three separate items in the audit but one fix item here:** all three
share the same regex, the same wrapping shape, and the same test fixture
pattern (`httptest.NewServer` with status-code script). One PR / commit.

**Tests:** add three table-driven test cases (one per function) using
`httptest.NewServer` that returns 504 twice then 200; assert the success
path observes 3 HTTP requests. Add a 4xx → no-retry case for parity with
the gh-retry test pattern.

---

### 4. Swap `shell.RepoExists` from `Run` to `RunWithRetry`

**Audit ref:** M1.
**Function:** `RepoExists` in `internal/shell/github.go:329-342`.

**Change shape:** one-line swap — `Run(...)` → `RunWithRetry(...)`. The
existing `IsRepoNotFound(out)` classifier already distinguishes 404 from
transient, so wrapping only retries the transient case; the 404 fast path
is preserved.

**Tests:** extend `internal/shell/github_test.go` with a fake `Run` that
returns a 504-shaped error twice then a 404; assert `RepoExists` returns
`(false, nil)` (the 404-not-found outcome) after the retry loop.

---

### 5. Swap `RunWatchWorkflow`'s appear-poll `RunCapture` to `RunCaptureWithRetry`

**Audit ref:** M3.
**Function:** `RunWatchWorkflow` in `internal/shell/github.go:446-489`,
specifically the `RunCapture(listCmd, "")` call at line 460.

**Change shape:** one-line swap — `RunCapture(...)` → `RunCaptureWithRetry(...)`.
The outer 6-attempt loop still bounds total wait time.

**Tests:** add a case to the existing `github_test.go` that simulates a
transient `gh run list` failure on the first poll attempt and success on
the second.

---

### 6. Swap `pollRunUntilComplete`'s `gh run view` to `RunWithRetry`

**Audit ref:** M4.
**Function:** `pollRunUntilComplete` in `internal/shell/github.go:495-542`,
specifically the `Run(viewCmd, true, "")` at line 509. **Do not** touch the
`gh run watch` call at line 476 — that's a streaming command, retry semantics
don't compose with a stream.

**Change shape:** swap line 509 from `Run(...)` → `RunWithRetry(...)`. The
existing rate-limit handling (`errors.As(err, &rle)` + sleep) stays — gh-retry
already passes rate-limit through as hard-fail.

**Tests:** extend the existing polling tests (if any) with a transient on
the per-iteration `gh run view`.

---

### 7. Swap `waitForRepoVisible`'s inner `Run` to `RunWithRetry`

**Audit ref:** M2.
**Function:** `waitForRepoVisible` in `internal/shell/github.go:362-388`,
specifically the `Run(viewCmd, true, "")` at line 373.

**Change shape:** one-line swap. Lower priority than M1/M3/M4 because the
post-create visibility window is narrow and the loop bound is short, but
removes a fatal-on-5xx edge in the create→clone race.

**Tests:** simulate a 504 during one of the 15 poll attempts; assert
`waitForRepoVisible` does not call `log.Fatalf`.

---

### 8. Batch swap four low-priority gh sites to retry-capable wrappers

**Audit ref:** L1, L6, L7, L10.

| File:Line | Current | After |
|---|---|---|
| `internal/steps/finalize.go:27` | `shell.RunCapture("gh api licenses/...", "")` | `shell.RunCaptureWithRetry("gh api licenses/...", "")` |
| `internal/shell/github.go:235` | `RunCapture("gh api rate_limit ...", "")` | `RunCaptureWithRetry("gh api rate_limit ...", "")` |
| `internal/shell/github.go:547` | `Run("gh repo delete ... --yes", true, "")` | `RunWithRetry("gh repo delete ... --yes", true, "")` |
| `main.go:752` | `shell.Run("gh issue create ...", false, "")` | `shell.RunWithRetry("gh issue create ...", false, "")` |

Each is a one-line swap with no semantic change beyond resilience. Ship as
one PR for consistency.

**Tests:** existing tests for each call path keep passing (retry wrappers
are drop-in compatible signature-wise). No new test required unless we want
to assert the retry behaviour explicitly per site.

---

### 9. Swap `internal/steps/project.go`'s `projectRunStdin` and `projectRun` seams

**Audit ref:** L4, L5.

- **L5 (`projectRun` for `gh project link`):** rename and rewire to
  `shell.RunWithRetry`. The seam at `internal/steps/project.go:50` is
  test-only — the change is a one-line edit in the seam plus updates to
  any test that asserts the exact wrapper used.
- **L4 (`projectRunStdin` for the GraphQL `updateProjectV2Field` mutation):**
  swap to `shell.MustRunStdinWithRetry` if abort-on-fail is acceptable
  semantically (current site is followed by `log.Fatalf` on error anyway),
  OR add a new non-`Must` `shell.RunStdinWithRetry` if we want the error
  to propagate to the caller. **Decision needed before execution.**

**Tests:** the existing `internal/steps/project_test.go` table tests will
need their seam replacements updated; the failure-mode tests (transient →
retry → success) can be added once.

---

### 10. Add retry to `internal/config/config.go`'s direct `exec.Command` probes

**Audit ref:** L8, L9.
**Functions:** `realCheckOwnerExists` (line 841), `realCheckProjectExists`
(line 873), `confirmReposExist` (line 936), `CloneShop` (line 1180) — the
`gh api ...` call only —, `latestMetaRelease` (line 1213).

**Change shape:** each currently builds a raw `exec.Command("gh", "api", ...)`
and calls `cmd.Run()` / `cmd.CombinedOutput()`. The minimal-touch fix is to
route each through `shell.RunWithRetry` (or `RunCaptureWithRetry` where
stdout is parsed), reinterpreting the existing "non-zero exit means not
found" classifier into a stderr / output pattern check.

**Caveat:** `internal/config/config.go` currently uses `exec.Command`
directly to suppress stderr on the expected-404 case (line 843, "Stderr is
suppressed so the first 404 doesn't leak when we fall back"). Migrating to
`shell.RunWithRetry` means stderr always lands in the returned error string
— callers need to start matching on the IS-NOT-FOUND wording in the error
instead of the cmd's exit code. This is a behaviour change worth landing in
its own commit, not a one-line swap.

**Lowest priority** in the audit. Defer until after items 1-9 ship and the
failure log has had a few weeks to surface real init-time 5xx incidents
against `gh api users/...` / `gh api orgs/...` / `gh api repos/...`.

---

## Out of scope

- The `gh repo clone` call at `internal/config/config.go:1196`. Audit
  classified as Low; clone has its own protocol-level retries inside the
  git client.
- The `git checkout` call at `internal/config/config.go:1202`. Local-only.
- The `runtime/...` packages. Their gh calls are dispatched from
  long-running agent contexts where retry semantics belong to the agent's
  budget, not to the per-call wrapper. Audit classifies these as
  R-DOC-OK; if a future incident says otherwise, revisit in a separate plan.
- `internal/runner/system.go`'s docker-compose retry (`upOne`'s
  `transientNetRE` + 3-attempt loop). Already R-OK; bash parity lives in
  the shop-side `docker-retry.sh`.

## Verification

After each item lands:

1. `go build ./...` clean.
2. `go test ./...` passes (no test should depend on a wrapper *not* being
   retry-capable).
3. Audit re-run (manual grep) confirms the corresponding R-MISSING entry
   no longer matches. When all items in this plan land, the audit's
   R-MISSING count drops from 16 to 0 and the audit can be re-issued with
   a closing note (mirroring the "2026-05-14: H1-H5 fixed" footer in
   `audits/20260514-silent-external-call-failures.md`).

---

## Cross-reference

- Companion audit: [`audits/20260514-external-call-retry-coverage.md`](../audits/20260514-external-call-retry-coverage.md)
- Parent program: [`plans/20260514-1945-retry-mechanism-end-to-end.md`](20260514-1945-retry-mechanism-end-to-end.md) (Phase 6)
- Sibling audit (silent errors, not retries): [`audits/20260514-silent-external-call-failures.md`](../audits/20260514-silent-external-call-failures.md)
- Engine sources: [`internal/shell/retrycore.go`](../internal/shell/retrycore.go), [`internal/shell/ghretry.go`](../internal/shell/ghretry.go)
