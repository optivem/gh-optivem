# Plan: harden `gh optivem init` against GraphQL transients

## Context

Today's `gh-acceptance-stage` run (https://github.com/optivem/gh-optivem/actions/runs/25877369208) failed on all 4 smoke jobs at `Ensure project board` with:

```
GraphQL: Something went wrong while executing your query on 2026-05-14T18:21:12Z.
Please include `8036:38296:317A531:C349697:6A061297` when reporting this issue.
```

That message is GitHub's standard wording for GraphQL internal-server errors. It is transient — a retry should pass — but `internal/steps/project.go` bypasses the existing retry infrastructure entirely, and even if it didn't, the retry classifier wouldn't recognise this wording as transient. A secondary consequence is that the always-run `Commit and push` step then panics with `chdir /tmp/scaffold-.../repo: no such file or directory` because `Clone repos` hadn't run yet, producing a misleading second error in the summary.

This plan makes three changes:

- **(A)** route `gh project ...` calls through the existing retry loop;
- **(B)** teach both the Go and bash retry classifiers about GraphQL internal-error wording;
- **(C)** stop the always-run `Commit and push` from emitting a misleading error when there is nothing local to commit.

A + B together fix the real failure. C cleans up the reporting.

## Critical files

- `internal/shell/github.go` — `RunCapture` lives here (line 120); the new retrying sibling goes alongside.
- `internal/shell/ghretry.go` — retry loop, classifier, and transient regex (lines 24-32, 88-100, 105-112).
- `internal/shell/ghretry_test.go` — classifier and loop tests; add cases for the new wording and the new wrapper.
- `internal/steps/project.go` — three `projectRunCapture` call sites at lines 255, 272, 293; test seam at line 48.
- `internal/steps/project_test.go` — stub in `install()` will keep working; the seam swap is invisible to existing tests.
- `internal/steps/finalize.go` — `commitAndPushRepo` at lines 129-167; needs an early skip when the repo dir is absent.
- `.github/scripts/gh-retry.sh` — bash retry regex on line 29; must be updated in lockstep with (B) so workflow-level retries get the same coverage.

## Existing utilities to reuse

- `runWithRetryLoop` (`internal/shell/ghretry.go:57`) — the single retry loop; (A) parameterises it with a `RunCapture`-based attempt.
- `classifyGHError` (`internal/shell/ghretry.go:88`) — keep as-is; (B) extends the `ghRetryTransient` regex it reads.
- `ghRetryAttempts` / `ghRetryDelays` (`internal/shell/ghretry.go:16-21`) — reuse the same 4-attempt, 5/15/45s schedule.
- `log.Infof` / `log.Successf` / `log.Warnf` (`internal/log`) — for the C skip message, match the tone of existing `commitAndPushRepo` logging.

## Implementation

### (A) Add `RunCaptureWithRetry` and route project-board calls through it

1. In `internal/shell/ghretry.go` (keeps the retry surface in one file), add:

   ```go
   // RunCaptureWithRetry is the retry-wrapped sibling of RunCapture. Use for
   // `gh` CLI calls that read API output (gh project list/field-list, etc.)
   // and need retry on transient GraphQL/network failures.
   func RunCaptureWithRetry(cmdStr, cwd string) (string, error) {
       return runWithRetryLoop(
           func() (string, error) { return RunCapture(cmdStr, cwd) },
           classifyGHError,
           ghRetryAttempts,
           ghRetryDelays,
       )
   }
   ```

2. In `internal/steps/project.go:48`, repoint the test seam:

   ```go
   projectRunCapture = shell.RunCaptureWithRetry
   ```

   The three call sites (lines 255, 272, 293) keep their current syntax; they go through the seam.

3. The test stub in `project_test.go:54-65` is unaffected — it replaces `projectRunCapture` wholesale and never touches the production wiring.

### (B) Recognise GitHub's GraphQL internal-error wording as transient

1. In `internal/shell/ghretry.go:24-32`, extend `ghRetryTransient`. Add one alternative:

   ```
   Something went wrong while executing your query
   ```

   GitHub emits this with HTTP 200 (GraphQL errors do not surface as 5xx), so none of the existing alternatives catch it.

2. In `internal/shell/ghretry_test.go:9-41`, add a `TestClassifyGHError` case:

   ```go
   {"graphql internal", "GraphQL: Something went wrong while executing your query on 2026-05-14T18:21:12Z.", err, true},
   ```

3. In `.github/scripts/gh-retry.sh:29`, append the same alternative to `_GH_RETRY_RETRYABLE`. Keep the regex character class identical to the Go version so the two stay in sync.

### (C) Skip `Commit and push` cleanly when nothing has been cloned

1. In `internal/steps/finalize.go`, add an early return at the top of `commitAndPushRepo` (line 129):

   ```go
   if _, err := os.Stat(repoDir); os.IsNotExist(err) {
       log.Infof("Skipping commit/push for %s: local dir %s not created (earlier step failed before clone)",
           fullRepo, repoDir)
       return
   }
   ```

   Use `os` — already imported in this file (line 6). The check is per-repo so multirepo cases still attempt the ones that did clone.

2. No new test file is required for this single guard; `commitAndPushRepo` has no existing test file (`finalize_test.go` does not exist). If a test is wanted, add a minimal one that creates a temp dir, deletes it, calls the function, and asserts it returns without panic — but this is optional and not in the critical path.

## Verification

End-to-end (real GitHub):
- Run `go build ./...` and `go test ./...` from the repo root — must pass clean.
- Trigger `gh optivem init` in a smoke scenario (re-dispatch the failed `gh-acceptance-stage` workflow run, or run a local `gh optivem init` against a throwaway repo). With (A)+(B), if the GraphQL blip recurs, expect a `[gh-retry] attempt N/4 failed, retrying in 5s` warning in the log followed by success.
- To exercise (C) deterministically: temporarily make `EnsureProjectBoard` fail (e.g. by setting `cfg.Owner` to a non-existent user via a local edit, or by revoking the gh token mid-run). The summary should report exactly one error — `Ensure project board` — and no `git add failed: chdir ... no such file or directory` line.

Unit tests:
- `go test ./internal/shell/...` — verifies the new classifier case and that `RunCaptureWithRetry` compiles and threads through.
- `go test ./internal/steps/...` — verifies the seam change in project.go is invisible to existing tests.

Bash sync check:
- Grep `_GH_RETRY_RETRYABLE` in `.github/scripts/gh-retry.sh` against `ghRetryTransient` in `internal/shell/ghretry.go` — the alternative lists must match.
