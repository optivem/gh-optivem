# Acceptance Stage Monitoring Process

## Process

1. **Check for an existing run** before triggering a new one.
   - If the `--fresh` flag was passed, skip directly to step 1b (trigger a new run).
   - Otherwise (default), check for an existing run:
     ```bash
     gh run list --workflow acceptance-stage.yml --repo optivem/gh-optivem --limit 1
     ```
     - If a run is **in_progress** or **queued**, go to step 3 (monitor it). Do NOT sleep first.
     - If a run **completed with failure**, go to step 5 (investigate it). Do NOT sleep or trigger a new run first.
     - If no recent run exists, or the latest run completed with **success** or **cancelled**, go to step 1b.

   **1b. Trigger a new run:**
   ```bash
   gh workflow run acceptance-stage.yml --repo optivem/gh-optivem
   ```
   Then go to step 2. After triggering, clear the `--fresh` flag so that subsequent cycles (after fixes) use the default existing-run behavior.

2. **Wait for the triggered run to appear.** This step only applies after triggering a new run in step 1. Sleep 5 minutes, then fetch the latest run to get its ID.

3. **Monitor** the run. First check the status immediately (no sleep), then sleep 5 minutes between subsequent checks:
   ```bash
   gh run list --workflow acceptance-stage.yml --repo optivem/gh-optivem --limit 1
   ```
   - If already **completed**, go to step 4 or 5 immediately.
   - If still **in_progress** or **queued**, sleep 5 minutes and check again. Repeat until "completed".
   - **Stuck queue timeout**: If the run stays in **queued** status for more than 15 minutes, cancel it:
     ```bash
     gh run cancel <run-id> --repo optivem/gh-optivem
     ```
     Wait 30 seconds for the cancellation to take effect, then go back to step 1. Step 1 will see the cancelled run and trigger a fresh one.
   - **Important**: Never trigger a new run without first checking step 1. Always go through step 1 to avoid duplicate runs.

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
   - Investigate the root cause using local files only (the clone, gh-optivem, and starter repos).
   - **All fixes must be applied and verified in the clone first — never commit to starter or gh-optivem until the clone fully passes.** Apply fixes directly in the cloned scaffolded repo:
     1. Run the specific failing suite first (e.g. `Run-SystemTests.ps1 -Architecture <arch> -Suite acceptance-ui`) to quickly confirm the fix.
     2. Then run the full system test suite (`Run-SystemTests.ps1 -Architecture <arch>` with no `-Suite` filter) to catch any additional failures.
     3. If more failures appear, fix them in the clone and re-run the full suite again.
   - Repeat until the full suite passes in the clone with no failures.
   - **Only after the clone fully passes locally**, push the fix to the cloned repo and verify in CI:
     1. Commit and push the fix to the cloned scaffolded repo on GitHub.
     2. Wait for the clone's **commit stage** to pass (check every 1 minute).
     3. Trigger and wait for the clone's **acceptance stage** to pass (check every 5 minutes).
   - **Only after the clone's CI fully passes**, apply the same fixes to the source:
     - If the fix belongs in **gh-optivem** (scaffolding logic), apply it there.
     - If the fix belongs in the **starter repo**, stop and ask the user for approval before modifying it.
   - Commit the source fix:
     ```bash
     bash "$(git rev-parse --show-toplevel)/../github-utils/scripts/commit.sh"
     ```
   - Wait for the **starter repo's commit stage** to pass (check every 1 minute).
   - Trigger the **starter repo's affected acceptance stage** (the one matching the architecture/lang that failed) and wait for it to pass (check every 5 minutes).
   - Delete the clone when done.
   - If the fix was only in the **starter repo** (no gh-optivem code changes), re-run the failed jobs:
     ```bash
     gh run rerun <run-id> --failed --repo optivem/gh-optivem
     ```
   - If the fix included **gh-optivem code changes**, a re-run will use the old code snapshot. Trigger a fresh run instead:
     ```bash
     gh workflow run acceptance-stage.yml --repo optivem/gh-optivem
     ```
   - Go back to step 3 (monitor the re-run).

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
