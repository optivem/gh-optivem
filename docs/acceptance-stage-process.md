# Acceptance Stage Monitoring Process

## Process

1. **Trigger** the acceptance stage:
   ```bash
   gh workflow run acceptance-stage.yml --repo optivem/gh-optivem
   ```

2. **Monitor** the run. Sleep 5 minutes between status checks (to avoid rate limiting):
   ```bash
   sleep 300 && gh run list --workflow acceptance-stage.yml --repo optivem/gh-optivem --limit 1
   ```
   Repeat until the run status is "completed".

3. **If the run succeeded**, report success and stop.

4. **If the run failed:**
   - Get the failed job logs:
     ```bash
     gh run view <run-id> --repo optivem/gh-optivem --log-failed
     ```
   - Investigate the root cause of the failure.
   - Fix the issue in the codebase.
   - Run **only the one failing test** locally (not the full suite). Resolve the starter path dynamically:
     ```bash
     OPTIVEM_STARTER_PATH="$(git rev-parse --show-toplevel)/../starter" \
       go test -tags=system ./internal/config/ -v -timeout 2h \
       -run "<FailingTestName>"
     ```
   - Repeat fix-and-test until the test passes locally.
   - Commit the fix:
     ```bash
     bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh"
     ```
   - Go back to step 1 (re-trigger the acceptance stage).

5. **Repeat** until the acceptance stage passes.

## Stop Conditions

Stop the loop and report to the user if:
- A test fails due to an external issue not under your control (subscription limits, third-party service outage, rate limiting).
- You cannot determine the root cause after thorough investigation.
- The same fix fails twice in CI after passing locally.

## Guidelines

- Always sleep at least 5 minutes between CI status checks to avoid GitHub API rate limiting.
- Only run the single failing test, never the full test suite.
- When investigating failures, check both the gh-optivem scaffolding code and the starter repo template files.
- Never use `git pull --rebase`. Always plain `git pull`.
- Never delete scaffolded test repos unless explicitly asked.
