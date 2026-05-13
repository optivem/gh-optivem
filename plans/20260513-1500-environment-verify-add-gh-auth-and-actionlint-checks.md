# `gh optivem environment verify` — add `gh auth status` + `actionlint` checks

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off on
> the overall shape (and the open questions in the final section).

## Context

`gh optivem environment verify` today (`internal/config/token_auth.go:228`
`VerifyEnvironment`) validates only the six gh-acceptance pipeline env vars:

- `DOCKERHUB_USERNAME` + `DOCKERHUB_TOKEN` → POST `hub.docker.com/v2/users/login`
- `SONAR_TOKEN` → GET `sonarcloud.io/api/authentication/validate`
- `GHCR_TOKEN` → GET `api.github.com/user` + read:packages scope
- `WORKFLOW_TOKEN`, `REPO_TOKEN` → GET `api.github.com/user` + repo scope

What it does **not** check, but which the rest of the CLI silently depends on:

1. **`gh` CLI authentication.** `internal/shell/github.go` shells out to `gh`
   for repo creation, secret/variable setting, label management, workflow
   dispatch, and run-watching. None of those calls succeed without a valid
   `gh auth login` for github.com. Today the user finds out at scaffolding
   time, mid-run.
2. **`actionlint` on PATH.** `internal/steps/verify.go:78`
   `VerifyScaffoldWorkflows` lints generated workflows with `actionlint` before
   any push. On dev machines it logs a warning and skips when the binary is
   missing; in CI it `log.Fatalf`s. The user only learns it's missing once
   scaffolding has already started.

Both are local-environment preconditions for the CLI to function, even though
neither is an env var. Catching them in the same preflight pass means the user
fixes everything in one go instead of fix-one-rerun-discover-next.

## Design (per author's answers 2026-05-13)

- **Placement:** extend the existing `gh optivem environment verify` command;
  no new verb, no new top-level `doctor` command. This reframes the verb's
  conceptual scope from "are my env vars valid" to "is my local environment
  ready for gh-optivem to run end-to-end". The `environment` noun broadens
  accordingly.
- **Both checks always run**, regardless of env-var presence — they have no
  dependency on the six token env vars. Missing env vars must NOT short-circuit
  these checks (current code returns early on missing env vars; that must
  change so the user sees gh/actionlint failures in the same pass).
- **`gh` host scoping:** check `gh auth status -h github.com` specifically.
  A user authenticated to a different GitHub Enterprise host but not
  github.com would otherwise pass with default `gh auth status` and then fail
  at scaffold time.
- **No version checks** for either tool. Presence is sufficient (matches the
  existing `internal/steps/verify.go` policy for actionlint).

## Steps

### Step 1 — New file `internal/config/tool_checks.go`

Two functions in the existing `config` package:

```go
// verifyGhAuth checks that the gh CLI is installed and authenticated for
// github.com. gh-optivem shells out to `gh` throughout scaffolding (see
// internal/shell/github.go); without a valid login, every one of those calls
// fails at invocation time mid-scaffold.
func verifyGhAuth() error {
    if _, err := exec.LookPath("gh"); err != nil {
        return errors.New("gh CLI not found on PATH.\n    " +
            "Install: https://cli.github.com/")
    }
    cmd := exec.Command("gh", "auth", "status", "-h", "github.com")
    out, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("gh CLI is not authenticated for github.com.\n    "+
            "Run: gh auth login\n    "+
            "Output:\n%s", string(out))
    }
    return nil
}

// verifyActionlint checks that the actionlint binary is on PATH. gh-optivem
// invokes actionlint during scaffolding (internal/steps/verify.go) to catch
// broken workflow references and syntax errors before any push — issues
// that otherwise surface ~10 min into the gh-acceptance pipeline as opaque
// HTTP 422 errors.
func verifyActionlint() error {
    if _, err := exec.LookPath("actionlint"); err != nil {
        return errors.New("actionlint not found on PATH.\n    " +
            "Install: go install github.com/rhysd/actionlint/cmd/actionlint@v1")
    }
    return nil
}
```

Imports: `errors`, `fmt`, `os/exec`. **Must not** import
`internal/shell` — `internal/shell` already imports `internal/config`, so the
reverse direction would create a cycle. Use `os/exec` directly.

### Step 2 — Rewire `VerifyEnvironment` in `internal/config/token_auth.go`

Refactor the function so local-tool checks (Step 1) and HTTP checks run in
the same parallel pass, and missing env vars are reported alongside check
failures rather than short-circuiting:

```go
func VerifyEnvironment() error {
    e := readEnvTokens()

    required := []struct{ name, val string }{
        {"DOCKERHUB_USERNAME", e.dockerHubUsername},
        // ... unchanged ...
    }
    var missing []string
    for _, r := range required {
        if r.val == "" {
            missing = append(missing, r.name)
        }
    }

    log.Info("Verifying environment...")

    type check struct {
        name string
        fn   func() error
    }
    // Local-tool checks always run; they don't depend on env vars.
    checks := []check{
        {"gh CLI auth", verifyGhAuth},
        {"actionlint",  verifyActionlint},
    }
    // HTTP checks only run if all required env vars are present, since each
    // needs its token value. Missing-var errors are aggregated below.
    if len(missing) == 0 {
        client := &http.Client{Timeout: tokenAuthTimeout}
        checks = append(checks,
            check{"DOCKERHUB_TOKEN", /* ... */},
            // ... existing five HTTP checks ...
        )
    }

    // ... parallel run unchanged ...

    if len(missing) == 0 && len(failures) == 0 {
        return nil
    }
    // Build aggregated error: missing-env block + failures block.
}
```

While here:

- **Rename `tokenAuthResult` → `checkResult`** (internal, single-file). The
  type is no longer token-specific.
- **Error message wording:** "Credential verification failed for N token(s)"
  → "Verification failed for N check(s)".
- **File-level doc comment:** widen scope from "token authentication checks"
  to "environment verification — local-tool presence + token authentication".

### Step 3 — Update `environment_commands.go` help text

In `newEnvironmentVerifyCmd`:

- `Long`: append the two new checks to the bulleted list:

  ```
  gh CLI auth         — `gh auth status -h github.com`
  actionlint          — `actionlint` binary on PATH
  ```

- Tweak the lead-in line from "every environment variable the gh-optivem CLI
  consumes is present and (for credentials) accepted by its provider" to
  acknowledge the broader scope (local tools + env vars).
- `Short` likely stays similar; consider "Verify the local environment is
  ready to run the gh-acceptance pipeline" if the env-var-centric phrasing
  reads wrong.

In the file-level doc comment, update the `environment verify` line: it now
checks "every credential and required local tool" rather than only
"credential".

### Step 4 — Tests

Existing repo grep shows no current tests for `VerifyEnvironment`. Don't
introduce one as part of this change unless the author asks for it —
matches the existing pattern. The two new helpers are thin wrappers over
`exec.LookPath` / `exec.Command`; spot-test by running the command locally
with `gh` logged out and `actionlint` removed from PATH.

## Open questions

1. **Is `gh auth status -h github.com` the right scope?** A user on GitHub
   Enterprise would fail this check even with valid alternate-host auth.
   Today's `internal/shell/github.go` calls always target github.com, so
   limiting to github.com is correct — but worth confirming this is a hard
   constraint of the tool, not a soft default.

2. **Should missing `gh` or `actionlint` be a hard failure or a warning?**
   `internal/steps/verify.go:91` already differentiates: actionlint missing
   is `Fatalf` under CI (`GITHUB_ACTIONS=true`) and `Warnf` locally.
   `environment verify` is itself a CI-preflight design — should it
   unconditionally fail, or mirror the steps-time soft/hard split? Default
   in this plan: unconditionally fail, since the user invoked `verify`
   specifically to learn what's broken.

3. **Should `gh auth status` output be included in the error message verbatim
   (as drafted) or summarized?** Verbatim is more diagnosable but noisier in
   aggregated failure output. Open to either.

4. **`tokenAuthResult` → `checkResult` rename — drift or worth it?** Single
   file, internal type, but it's churn beyond what the user literally asked
   for. Easy to drop if the author prefers a smaller diff.
