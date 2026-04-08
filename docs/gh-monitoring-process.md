# Acceptance Stage Monitoring Process

## Process

1. **Check for an existing run** before triggering a new one:
   ```bash
   gh run list --workflow acceptance-stage.yml --repo optivem/gh-optivem --limit 1
   ```
   - If a run is **in_progress** or **queued**, skip to step 3 (monitor it).
   - If a run **completed with failure**, skip to step 5 (investigate it).
   - If no recent run exists, or the latest run **completed successfully**, trigger a new one:
     ```bash
     gh workflow run acceptance-stage.yml --repo optivem/gh-optivem
     ```

2. **Wait for the run to appear** (if you just triggered one). Sleep 30 seconds, then fetch the latest run to get its ID.

3. **Monitor** the run. Sleep 5 minutes between status checks (to avoid rate limiting):
   ```bash
   sleep 300 && gh run list --workflow acceptance-stage.yml --repo optivem/gh-optivem --limit 1
   ```
   Repeat until the run status is "completed".

4. **If the run succeeded**, report success and stop.

5. **If the run failed:**
   - Get the failed job logs:
     ```bash
     gh run view <run-id> --repo optivem/gh-optivem --log-failed
     ```
   - If the failure involves a scaffolded test repo, clone it locally for investigation (one git operation, no API calls):
     ```bash
     git clone https://github.com/<owner>/<repo>.git /tmp/<repo>
     ```
     Then read all files from the local clone. Delete the clone when done.
   - Investigate the root cause using local files only (the clone, gh-optivem, and starter repos).
   - Fix the issue in the codebase.
   - Run **only the one failing test** locally (not the full suite):
     ```bash
     go test -tags=system ./internal/config/ -v -timeout 2h \
       -run "<FailingTestName>"
     ```
   - Repeat fix-and-test until the test passes locally.
   - Commit the fix:
     ```bash
     bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh"
     ```
   - Go back to step 1 (re-trigger the acceptance stage).

6. **Repeat** until the acceptance stage passes.

## Stop Conditions

Stop the loop and report to the user if:
- A test fails due to an external issue not under your control (subscription limits, third-party service outage, rate limiting).
- You cannot determine the root cause after thorough investigation.
- The same fix fails twice in CI after passing locally.

## Guidelines

- Always sleep at least 5 minutes between CI status checks to avoid GitHub API rate limiting.
- Only run the single failing test, never the full test suite.
- When investigating failures, check both the gh-optivem scaffolding code and the starter repo template files.
- Only make changes to the gh-optivem repo. Do NOT modify the starter repo. If you believe a starter repo change is needed, stop and ask the user for approval first.
- Never use `git pull --rebase`. Always plain `git pull`.
- Never delete scaffolded test repos unless explicitly asked.

## Rate Limiting Rules

The GitHub API quota is 5000 requests/hour and is shared across all agents and the user. Exceeding it blocks everyone.

- **Investigation must use local files only.** Read the gh-optivem and starter repos from the local filesystem. For scaffolded test repos, clone them locally (`git clone`) instead of using `gh api repos/.../contents/...`. Never fetch file contents via the GitHub API.
- **Only use `gh` for CI operations**: triggering workflows, checking run status, and fetching failed logs (`--log-failed`). These are the only `gh` calls allowed.
- **Never browse historical runs.** Only look at the current/latest run. Do not use `--limit 50` or inspect old runs to find patterns.
- **Maximum 20 `gh` calls per monitoring cycle** (trigger + poll + fetch logs). If you exceed this, stop and report to the user.
- **If you hit a rate limit error**, stop immediately and report to the user with the reset time. Do not retry.
