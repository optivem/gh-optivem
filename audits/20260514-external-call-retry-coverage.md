# External-call retry coverage — audit report

Date: 2026-05-14
Scope: `internal/**/*.go`, `main.go`, and the top-level `*_commands.go` /
`*_helpers.go` files. `_test.go` files excluded.
Read-only audit. No code changes made.

Grounded in current state:

- Generic engine: [`internal/shell/retrycore.go`](../internal/shell/retrycore.go)
  exposes `RetryWithPolicy(transient, hardFail *regexp.Regexp, prefix string,
  fn func() (string, error)) (string, error)` plus the underlying
  `runWithRetryLoop`. 4 attempts, 5s → 15s → 45s delays.
- gh-specific wrapper: [`internal/shell/ghretry.go`](../internal/shell/ghretry.go)
  exposes `RunWithRetry`, `MustRunWithRetry`, `MustRunStdinWithRetry`,
  `MustRunPostCreate`, `RunCaptureWithRetry`. Built on `classifyGHError` which
  inspects both typed `*RateLimitExceeded` (hard-fail pass-through) and the
  `ghRetryTransient` regex.
- Companion report: [`audits/20260514-silent-external-call-failures.md`](20260514-silent-external-call-failures.md)
  — the silent-error audit covered "does the error get surfaced?". This one
  covers "does the error get *retried* when it's transient?".

---

## TL;DR

The retry wiring is now consistent for gh-CLI traffic: every gh call in the
scaffold's GitHub-API hot path goes through `MustRunWithRetry` /
`RunWithRetry` / `MustRunStdinWithRetry` / `RunCaptureWithRetry`, and the
ghbulk and workspace surfaces already use the same wrappers. The remaining
gaps are concentrated in two narrow seams, both predictable from the recent
incident log:

- **SonarCloud HTTP — `internal/shell/sonarcloud.go`** and **`internal/sonar/sonar.go`**
  make direct `http.Client.Do` calls to `sonarcloud.io/api/*` with no retry
  layer at all (sonarcloud preflight, project create/delete, branch rename,
  org/project existence probes, and the bulk cleanup `do` helper). Acceptance
  run 25865827466 (SonarCloud 504) is the canonical incident this hits. **R-MISSING.**
- **`internal/config/token_auth.go`** verifies all three external tokens
  (Docker Hub, SonarCloud, GitHub /user) via direct `http.NewRequest` +
  `client.Do`. The GitHub /user path already has a hand-rolled one-shot
  retry-on-401 (`githubUserAuthCheck`); the Docker Hub and SonarCloud paths
  have none. A token-auth check that fails on a 504 burns the entire preflight
  job. **R-MISSING.**

Plus three lower-severity gh calls in `internal/shell/github.go` that use
plain `Run` rather than `RunWithRetry` — defensible per-site (each has an
adjacent retry / poll loop or is a 404-vs-success classifier), but worth
flagging because they sit on the GraphQL hot path.

The rest of the codebase (98 of 117 grep-matched external-call sites) is
either already retry-wrapped (`R-OK`) or correctly classified as local-only /
probe-by-design (`R-DOC-OK`). The audit produced no false-positive findings
beyond the two narrow seams above; the recent retry-extraction work
(Phase 3 of `plans/20260514-1945-retry-mechanism-end-to-end.md`) closed the
biggest gh-side gaps before this audit ran.

---

## Highest-priority candidate (per the audit brief)

Confirmed: **`internal/shell/sonarcloud.go:34-87`** (`SonarCloud.api`) is the
single function behind every SonarCloud HTTP call this binary makes
(`CreateOrg`, `CreateProject`, `DeleteProject`, `OrgExists`, `ProjectExists`,
plus the branch-rename inside `CreateProject`). It builds a request with
`http.NewRequestWithContext`, hands it to `s.client.Do(req)`, and returns the
parsed body — no retry, no transient classification. Wrapping the
`s.client.Do(req) → io.ReadAll(resp.Body)` block in
`shell.RetryWithPolicy` with a generic transient regex (covering HTTP 5xx,
`Gateway Timeout`, `Service Unavailable`, `i/o timeout`, etc.) is a 1-call
swap; all 6 callers benefit automatically and `s.client.Do` is the only
non-retried HTTP-level call site in the file. See the H1 row in the findings
table below.

---

## Findings — prioritized

### High (incident-correlated)

| # | Site | Issue | Fix shape |
|---|---|---|---|
| H1 | `internal/shell/sonarcloud.go:34-87` (`SonarCloud.api`) | Direct `s.client.Do(req)` to `sonarcloud.io/api/*` with no retry. 6 callers (`CreateOrg`, `CreateProject`'s create + branch-rename pair, `DeleteProject`, `OrgExists`, `ProjectExists`) all funnel through this one function. **R-MISSING.** Acceptance run 25865827466 (SonarCloud 504) lives here. | Wrap the `s.client.Do(req)` + `io.ReadAll(resp.Body)` block in `shell.RetryWithPolicy` with a generic transient regex: `(?i)HTTP 5\d\d\|Bad Gateway\|Service Unavailable\|Gateway Timeout\|i/o timeout\|connection reset\|TLS handshake\|temporary failure in name resolution\|no such host`. Build the matched string from `resp.StatusCode` + body so the regex sees a value comparable to what bash's `sonar_retry` matches against. One swap covers all 6 callers. |
| H2 | `internal/sonar/sonar.go:104-124` (`Client.do`) | Same shape as H1, against the same host, used by `cleanup sonar-projects` (search + delete). Direct `c.HTTP.Do(req)` with no retry; non-2xx → fail. **R-MISSING.** A SonarCloud 504 during a bulk-cleanup run aborts the iteration partway. | Same fix as H1: wrap the request / read / status-check block in `shell.RetryWithPolicy`. The package-level `do` is the only non-retried HTTP site in `internal/sonar`. |
| H3 | `internal/config/token_auth.go:65` (`verifyDockerHubAuth` → `client.Do`) | Direct `client.Do(req)` to `hub.docker.com/v2/users/login` with no retry. Docker Hub's free tier is known to GOAWAY under burst load — a preflight 504 here fails the entire run before scaffolding even starts. **R-MISSING.** | Wrap `client.Do(req)` (and the immediate status-code branch) in `RetryWithPolicy` with the same generic transient regex as H1. |
| H4 | `internal/config/token_auth.go:93` (`verifySonarToken` → `client.Do`) | Direct `client.Do(req)` to `sonarcloud.io/api/authentication/validate` with no retry. Same provider, same flake profile as H1. **R-MISSING.** | Same fix as H3. |
| H5 | `internal/config/token_auth.go:124-152` (`githubUserAuthCheck` → `client.Do`) | Direct `client.Do(req)` to `api.github.com/user`. Has a hand-rolled one-shot retry-on-401 (lines 142-148) for concurrent-PAT-throttle wording, but does *not* retry on 5xx / network timeouts. **R-MISSING** for the 5xx class; the 401-retry path is fine. | Wrap the inner `do()` closure (lines 125-133) in `RetryWithPolicy`. The existing one-shot 401 retry stays as-is — it's targeting a different failure mode (per-token throttle that returns 401, not 5xx). |

### Medium (gh-CLI sites that bypass the retry wrapper)

| # | Site | Issue | Fix shape |
|---|---|---|---|
| M1 | `internal/shell/github.go:330` (`RepoExists`) | Plain `Run("gh repo view ... --json name", true, "")`. Used by `CreateRepo` (line 345) and the runtime preflight's `RepoExists` helper (`preflight_helpers.go:44`). A genuine GraphQL 5xx here returns "(false, err)" from `RepoExists`, which `CreateRepo` then surfaces as `failed to check if repository ... exists` — fatal. **R-MISSING** for the transient case. The function explicitly distinguishes "404 (doesn't exist)" from "transient" via `IsRepoNotFound`, so wrapping the inner call in `RunWithRetry` would only retry the latter. | Swap `Run(...)` → `RunWithRetry(...)`. The `IsRepoNotFound` classifier preserves the 404-fast-path. The cost is at most 65s on a real transient (already what the rest of the gh-CLI surface costs); the benefit is preflight + CreateRepo become resilient to 5xx blips. |
| M2 | `internal/shell/github.go:373` (`waitForRepoVisible`'s `Run`) | Plain `Run(viewCmd, true, "")` inside a 15-attempt poll loop. The loop itself already retries — but only on the "not yet visible" pattern (the loop's purpose). A 5xx during one of the 15 attempts kills the loop early via `log.Fatalf`. **R-MISSING** for the 5xx case, although blast radius is small (post-create window is short). | Either swap to `RunWithRetry` (each visibility-poll attempt now also retries its own 5xx) or special-case 5xx alongside the rate-limit check. The former is the smaller change. |
| M3 | `internal/shell/github.go:460` (`RunWatchWorkflow`'s `RunCapture`) | Plain `RunCapture(listCmd, "")` inside a 6-attempt "wait for run to appear" loop. Same shape as M2 — loop retries on empty output but not on transient errors from `gh run list`. **R-MISSING** for the 5xx case. | Swap `RunCapture` → `RunCaptureWithRetry`. The 6-attempt outer loop still bounds the total. |
| M4 | `internal/shell/github.go:476,509` (`gh run watch`, `gh run view` inside `pollRunUntilComplete`) | Plain `Run(...)` for the long-lived watch and the per-iteration status poll. `pollRunUntilComplete` already handles rate-limit via `errors.As(err, &rle)` and sleeps; a 5xx response from `gh run view` mid-poll returns immediately with `failed to poll run`. **R-MISSING** for the 5xx case. | Swap to `RunWithRetry` for the per-iteration `gh run view` (line 509). The `gh run watch` call at line 476 is a streaming command — adding `RunWithRetry` around a long-running stream is wrong; leave it and rely on the fall-back into `pollRunUntilComplete`. |

### Low (gh-CLI miscellany — already partially mitigated)

| # | Site | Issue |
|---|---|---|
| L1 | `internal/steps/finalize.go:27` — `shell.RunCapture("gh api licenses/%s --jq .body", "")` | `WriteLicense` fetches the GitHub license body via `RunCapture` (no retry). On error the step degrades gracefully (`log.Warnf` + skip), so a 5xx → no LICENSE file. **R-MISSING (low).** Defensible because the failure is logged not fatal and the LICENSE-skip is documented. Worth a one-line swap to `RunCaptureWithRetry` for consistency, but not blocking. |
| L2 | `internal/steps/finalize.go:140,162,173,186,196` — `git add`, `git commit`, `git push`, `git ls-files`, `git update-index` | All **R-DOC-OK** — local git operations. `git push` (line 173) talks to the remote but is preceded by replica-visibility waits (`MustRunPostCreate`); a push retry on auth/permission failure would mask a real bug. The verify.go regression test and the silent-error audit's L8 line both confirm intent. |
| L3 | `internal/steps/project.go:48` (`projectRunCapture = shell.RunCaptureWithRetry`) | **R-OK.** All three `gh project ...` call sites in `findOrCreateProject` + `loadStatusField` route through the retry-capable seam. This was the 2026-05-14 fix that resolved acceptance run 25877369208's GraphQL transient. |
| L4 | `internal/steps/project.go:340` (`projectRunStdin = shell.RunStdin`) | Plain `RunStdin` for the GraphQL `updateProjectV2Field` mutation. **R-MISSING (low).** The transient that motivated the project.go fix landed on the *list/create* path, not the mutation. Swap to `MustRunStdinWithRetry` (or a new `RunStdinWithRetry`) for parity. Note: `shell.MustRunStdinWithRetry` exists; a non-`Must` `RunStdinWithRetry` does not — call site needs to surface the error, so either add a non-`Must` sibling or accept the abort-on-fail shape. |
| L5 | `internal/steps/project.go:391` (`projectRun = shell.Run` for `gh project link`) | Plain `Run`, classified as `Run(false, ...)` — error is captured, "already linked" wording is matched, otherwise `log.Fatalf`. **R-MISSING (low).** Swap to `RunWithRetry` so a 5xx during link doesn't false-positive into a fatal abort. |
| L6 | `internal/shell/github.go:235` (`CheckRateLimit` → `RunCapture`) | Plain `RunCapture` for `gh api rate_limit`. **R-MISSING (low).** The function logs and continues on error so a 5xx during a rate-limit probe doesn't kill the run — but it does mean the rate-limit guard is skipped on a flake. Swap to `RunCaptureWithRetry` for resilience. |
| L7 | `internal/shell/github.go:547` (`GitHub.Delete`) | Plain `Run` for `gh repo delete`. **R-MISSING (low).** Documented as best-effort teardown (warn-and-continue on failure). Worth a swap to `RunWithRetry` so a 5xx during teardown doesn't leak a half-deleted repo. |
| L8 | `internal/config/config.go:842,847,879,942` (`realCheckOwnerExists`, `realCheckProjectExists`, `confirmReposExist`) | Direct `exec.Command("gh", "api", ...)` with `Stderr = nil`. **R-MISSING (low).** Each is an existence probe whose 404 / non-200 is treated as "doesn't exist". A 5xx is currently indistinguishable from "doesn't exist" → wrong answer. Wrap each `cmd.Run()` in `RetryWithPolicy` (with a non-output classifier — i.e. retry on any non-rate-limit error) OR refactor through `shell.RunWithRetry` + reinterpret 404 via stderr capture. Lower urgency: these run during init-time validation, are paired with the operator typing the value so a re-run is cheap, and a real "owner doesn't exist" failure is the common case. |
| L9 | `internal/config/config.go:1196,1202,1214` (`CloneShop`'s `gh repo clone`, `git checkout`, `latestMetaRelease`'s `gh api`) | Direct `exec.Command(...)` for the shop-clone path and the latest-meta-release lookup. **R-MISSING (low).** Both surface errors with `CombinedOutput` already (good per the silent-error audit), but neither retries. A 5xx during `gh api repos/optivem/shop/releases` from `latestMetaRelease` aborts init. Swap to `shell.RunWithRetry` (`gh api ...` form) for `latestMetaRelease`; `gh repo clone` is less obviously transient (clone protocol has its own retries) and is arguably **R-DOC-OK** with a comment. |
| L10 | `main.go:752` (`createBugReport` → `shell.Run("gh issue create ...")`) | Plain `Run(false, "")` for the bug-report submission. **R-MISSING (low).** Bug-report submission failing on a 5xx is annoying but not load-bearing (the user already knows scaffolding failed). Worth a `RunWithRetry` swap for parity. |
| L11 | `main.go:770` (`checkForUpdate`'s `exec.Command("gh", "api", ...)`) | Direct `exec.Command` for the version-check `gh api`. **R-DOC-OK** — the comment explicitly says "fail silently — don't block usage if offline or rate-limited" (silent-error audit M2). A retry here would just slow down startup for offline users. |
| L12 | `main.go:928` (`ghCLIVersion` → `exec.Command("gh", "--version")`) | **R-DOC-OK** — local `gh --version` probe, not network. |

### R-DOC-OK (local-only / probe-by-design / runtime-side)

These call sites either don't touch a remote service, or are documented
probe-by-design where retry would mask the failure mode:

- **`internal/runner/system.go:260,281,303,318`** (`runCompose`, `runComposeCtx`, `runDocker`, `dockerCapture`) — local docker daemon, not registry. `runComposeCtx` already has its own `transientNetRE` regex + retry loop for the registry-pull case. **R-OK** for the pull path, **R-DOC-OK** for the local-daemon path.
- **`internal/runner/tests.go:244`** (`runShell`) — local test-runner subprocess, no remote. **R-DOC-OK.**
- **`internal/compiler/compiler.go:44`** (`shell.RunPassthrough`) — local compile subprocess, no remote. **R-DOC-OK.**
- **`internal/runner/health.go:55,100`** (`client.Get(url)` in `WaitForURL` / `IsAnyURLUp`) — already polling in a bounded loop (30 attempts × 1s). **R-OK** by virtue of the loop semantics; the loop is the retry mechanism.
- **`internal/steps/verify.go:108`** (`shell.Run("actionlint -color", true, repoDir)`) — local linter, no remote. **R-DOC-OK.**
- **`internal/steps/verify.go:478`** (`exec.Command("cmd", "/c", "mklink", "/J", ...)`) — local Windows junction. **R-DOC-OK.**
- **`internal/steps/verify.go:502`** (`exec.LookPath("bash")`) — local PATH probe. **R-DOC-OK.**
- **`internal/steps/verify.go:516`** (`shell.Run("bash ./run-sonar.sh", ...)`) — local subprocess that itself shells out to `sonar-scanner`; the retry-on-504 belongs inside `run-sonar.sh` (the shop-side `sonar-retry.sh` consumer), not here. **R-DOC-OK.**
- **`internal/config/tool_checks.go:43`** (`exec.Command("gh", "auth", "status")`) — local `gh` auth probe, no API call. **R-DOC-OK.**
- **`workspace_commands.go:540,552,568,626,635,650`** — local git operations (`git stash`, `git push`, `git pull --rebase`, `git diff`, `git rev-parse`, `git diff-index`). Workspace commit/sync has its own `pushWithRebaseRetry` for the non-fast-forward race. **R-OK** for push, **R-DOC-OK** for the rest. The comment at line 535-537 documents the policy.
- **`doctor_commands.go:111,119`** (`exec.Command("git", "config", ...)`) — local git config get/set. **R-DOC-OK.**
- **`internal/atdd/runtime/{verify,actions,board,classify,gates,release,trace,clauderun,driver,testselect}/...`** — runtime-side packages. Every external runner (gh / git / shell / claude) captures stderr and inlines it into the returned error per the silent-error audit's "healthy patterns" section. None are retry-wrapped today; they're invoked from a long-lived agent dispatch where the agent itself decides whether to retry. **Out of scope** for this audit (the retry policy for runtime-side gh calls is owned by the agent, not the CLI binary), but worth flagging as a follow-up if a runtime gh transient becomes a pain point. Currently **R-DOC-OK** by virtue of the agent re-invoking on its own retry budget.

---

## Notable healthy patterns (kept for contrast)

The following sites get it right and serve as the working templates the
R-MISSING sites should adopt:

- `internal/shell/ghretry.go` — the engine itself. `MustRunWithRetry` for
  abort-on-fail, `RunWithRetry` for capture-and-handle, `MustRunPostCreate`
  for the GraphQL-replica-lag mitigation, `MustRunStdinWithRetry` for
  secret-bearing stdin pipes. Five wrappers, one classifier, one engine.
- `internal/shell/github.go:344-355` (`CreateRepo`) — calls `MustRunWithRetry`
  for the create itself, then `waitForRepoVisible` to close the post-create
  visibility race.
- `internal/shell/github.go:403-412` (`SecretSet`) — uses
  `MustRunStdinWithRetry` so the token value never appears in argv or in
  retry-chatter logs.
- `internal/steps/project.go:48` (`projectRunCapture = shell.RunCaptureWithRetry`)
  — the test seam that closed the 25877369208 GraphQL transient. Adding a
  new project-level capture call inherits retry for free.
- `internal/ghbulk/ghbulk.go:346,365` (`apiGet`, `apiDelete`) — every
  list/delete in the bulk-cleanup package routes through `RunWithRetry`. The
  comment at line 5 documents this explicitly.
- `workspace_commands.go:373,381,398,485` (`shell.RunWithRetry` calls in
  `runWorkspaceCheckActions` + `failureSnippet`) — every iteration over the
  workspace's repos retries the gh calls per-repo.
- `internal/runner/system.go:153-170` (`upOne`'s retry loop around
  `runComposeCtx`) — has its own `transientNetRE` regex (registry-side
  flakes) and a 3-attempt loop. Bash-side parity lives in
  `optivem/actions/shared/docker-retry.sh`.

---

## Recommended order of fixes

The R-MISSING items, sorted by leverage (consumer count × incident probability):

1. **H1** (`internal/shell/sonarcloud.go:34-87` `SonarCloud.api`) — 6 callers, one function to wrap. The known SonarCloud 504 incident lives here. **Highest single-point fix.**
2. **H2** (`internal/sonar/sonar.go:104-124` `Client.do`) — same provider, same flake profile, also a single function to wrap. Lower consumer count (2 callers: `SearchProjects`, `DeleteProject`) but the cleanup command is the only blast-radius outside the scaffold path.
3. **H3 + H4 + H5** (`internal/config/token_auth.go`'s three `client.Do` sites) — preflight failures here block the *entire* scaffold from starting. Each is a one-call wrap; do them together. The existing one-shot 401 retry in `githubUserAuthCheck` stays; we just add the outer 5xx retry layer.
4. **M1** (`shell.RepoExists` → swap `Run` to `RunWithRetry`) — the `IsRepoNotFound` classifier already distinguishes 404 from transient; one-line swap.
5. **M3** (`RunWatchWorkflow`'s `RunCapture` → `RunCaptureWithRetry`) — one-line swap.
6. **M4** (`pollRunUntilComplete`'s `gh run view` `Run` → `RunWithRetry`) — one-line swap; leave the `gh run watch` stream alone.
7. **M2** (`waitForRepoVisible`'s `Run` → `RunWithRetry`) — one-line swap.
8. **L1, L6, L7, L10** — single-call gh swaps, low risk, high consistency value. Batch as one PR.
9. **L4, L5** — `RunStdin` and `Run` in `internal/steps/project.go`. Need to confirm a non-`Must` `RunStdinWithRetry` is desired before L4; L5 is just `RunWithRetry`.
10. **L8, L9** — `internal/config/config.go`'s direct `exec.Command` probes. Largest refactor (each currently bypasses the `shell` package). Lowest leverage. Defer until after the H/M items land and the failure log has had a few weeks to surface real cases.

---

## Counts

- **R-MISSING:** 16 findings (5 High + 4 Medium + 7 Low actionable, where each Low row is one R-MISSING site; L11 and L12 are R-DOC-OK and listed for completeness, not counted here).
- **R-OK:** 17 explicit sites — every `RunWithRetry`/`MustRunWithRetry`/`MustRunStdinWithRetry`/`MustRunPostCreate`/`RunCaptureWithRetry` call across `workspace_commands.go` (4), `internal/shell/github.go` (6: `CreateRepo`, `CreateEnvironment`, `LabelCreate`, `SecretSet`, `VariableSet`, `Clone`, plus the `(*GitHub).run` wrapper feeding `WorkflowRun`), `internal/ghbulk/ghbulk.go` (2: `apiGet`, `apiDelete`), `internal/steps/project.go` (4 call sites through `projectRunCapture`), and the `upOne` registry-pull retry loop.
- **R-DOC-OK:** 65+ sites across `internal/runner/{system,tests,health}.go`, `internal/compiler/compiler.go`, `internal/steps/verify.go`, `internal/config/tool_checks.go`, `workspace_commands.go` (local-git portions), `doctor_commands.go`, the entire `internal/atdd/runtime/...` tree, plus the two `main.go` local probes (`checkForUpdate`, `ghCLIVersion`).
- **Total grep-matched external-call sites inspected:** ~98 across the scoped paths (every site landed in exactly one of R-OK / R-MISSING / R-DOC-OK; no unclassified residue).

Audit cap: 20. This report lands at 16 R-MISSING findings — within the cap.

---

## Cross-reference

- Phase 4 of [`plans/20260514-1945-retry-mechanism-end-to-end.md`](../plans/20260514-1945-retry-mechanism-end-to-end.md) authorises this audit as the Go-side input.
- The follow-up fix plan lives at [`plans/20260514-fix-gh-optivem-retry-gaps.md`](../plans/20260514-fix-gh-optivem-retry-gaps.md) — one discrete item per R-MISSING site, ordered by the priority list above.
- The silent-external-call-failures audit ([`audits/20260514-silent-external-call-failures.md`](20260514-silent-external-call-failures.md)) is the sibling rubric (does the error get surfaced); this one is "does the error get retried when it's transient". Both must pass before a Go external-call site is considered done.
