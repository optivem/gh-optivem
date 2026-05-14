# Plan: retry `Run Sonar Analysis` on transient SonarCloud failures

Mirrors the existing transient-failure retry pattern used in the same acceptance-stage workflows for Docker Hub and GHCR logins (`Wandalen/wretry.action@v3`). Extends it to cover the SonarCloud upload step, which has been observed failing on HTTP 504 from `api.sonarcloud.io`.

## Context

On 2026-05-13, the meta-prerelease pipeline run [25805401826](https://github.com/optivem/shop/actions/runs/25805401826) had both `.NET` acceptance-stage jobs (`monolith-dotnet` run `25806401223`, `multitier-dotnet` run `25806336675`) fail in the `sonar` job. The other four language/stack combos on the same meta-run passed, ruling out a code or config regression.

Both failures terminated with the same root cause inside `dotnet sonarscanner end`:

```
Caused by: com.sonarsource.scanner.engine.webapi.client.HttpException:
  Error 504 on https://api.sonarcloud.io/analysis/analyses :
  {"message": "Endpoint request timed out"}
```

This is a transient SonarCloud-side timeout, exactly the same class of flakiness already absorbed by `Wandalen/wretry.action@v3` for the Docker Hub and GHCR login steps in these same workflow files. Without retry, every Sonar 504 surfaces as a hard pipeline failure that aborts the meta-prerelease pipeline and forces a manual rerun.

The fix is to wrap the `Run Sonar Analysis` step with `Wandalen/wretry.action@v3` in `command:` mode, with the same `attempt_limit: 3` policy used by the existing login steps but a longer `attempt_delay` (30s) because SonarCloud 504s tend to be load-related and benefit from a slightly longer back-off than registry blips.

## Critical files

All in the `optivem/shop` repo (`academy/shop/.github/workflows/`):

- `monolith-dotnet-acceptance-stage.yml` — sonar step at line 387
- `multitier-dotnet-acceptance-stage.yml` — sonar step (same shape)
- `monolith-java-acceptance-stage.yml` — sonar step (same shape)
- `multitier-java-acceptance-stage.yml` — sonar step at line 396
- `monolith-typescript-acceptance-stage.yml` — sonar step at line 376
- `multitier-typescript-acceptance-stage.yml` — sonar step (same shape)

(No edits to `system-test/<lang>/run-sonar.sh` — see Out of scope.)

## Reuse references

- `monolith-dotnet-acceptance-stage.yml` lines 93–100 — existing `Wandalen/wretry.action@v3` wrapper for `docker/login-action@v4` (Docker Hub login). Same idiom we want to mirror.
- `monolith-dotnet-acceptance-stage.yml` lines 103–111 — second instance, GHCR login. Confirms `attempt_limit: 3` is the project's standard for transient external-service failures.
- All 6 target workflows already use the same uniform sonar step shape:
  ```yaml
  - name: Run Sonar Analysis
    working-directory: system-test/<lang>
    env:
      SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
    run: ./run-sonar.sh
  ```
  so the diff is identical across files modulo the language token in the path.

## Steps

For each of the 6 acceptance-stage workflows listed above, replace:

```yaml
      - name: Run Sonar Analysis
        working-directory: system-test/<lang>
        env:
          SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
        run: ./run-sonar.sh
```

with:

```yaml
      - name: Run Sonar Analysis
        uses: Wandalen/wretry.action@v3
        with:
          attempt_limit: 3
          attempt_delay: 30000
          command: cd system-test/<lang> && ./run-sonar.sh
        env:
          SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
```

Notes:

- `Wandalen/wretry.action@v3`'s `command:` mode runs a shell snippet, but does **not** honour the step-level `working-directory:` key. We fold the `cd` into the command itself so the script runs from the correct path.
- `attempt_delay` is in milliseconds. 30000 = 30s, vs the 10000 (10s) used for Docker/GHCR logins. Sonar 504s tend to be load-related; a slightly longer back-off avoids hammering the API during a SonarCloud incident while still adding under 1 minute to the worst-case green-path runtime if the first attempt happens to time out.
- `attempt_limit: 3` matches the project convention for transient external-service retries.
- The `env:` block remains step-level so `SONAR_TOKEN` is available to the wrapped command. (`Wandalen/wretry.action` inherits the step's `env`.)

## Out of scope

- **Retry inside `run-sonar.sh` itself.** The failure observed today was in CI, not in manual local runs. Adding retry at the workflow step is sufficient and keeps language-specific scripts (`system-test/dotnet/run-sonar.sh`, `system-test/java/run-sonar.sh`, `system-test/typescript/run-sonar.sh`) simple and identical to what a developer would run locally. If we later observe transient failures during local runs, the same retry can be added at the script layer separately.
- **`*-acceptance-stage-legacy.yml` and `*-acceptance-stage-cloud.yml` variants.** The legacy stage does not run Sonar (verify by grep — only the 6 listed workflows match `Run Sonar Analysis`). The cloud variants likewise do not contain the step. No change needed.
- **Commit-stage and QA-stage workflows.** Sonar runs only at the acceptance stage in this repo.
- **Bumping `Wandalen/wretry.action` past `@v3`.** The existing pinned version is already in use across this workflow file and matches the project's convention.
- **gh-optivem scaffolder changes.** These workflow files are checked-in artefacts in the `optivem/shop` repo, not generated from a gh-optivem template — search confirms no template under `gh-optivem/internal/templates/` produces them. Rollout is a direct edit + commit in `shop`.

## Verification

1. **Static / lint**: `actionlint` (if configured locally) on each modified file should remain clean — `Wandalen/wretry.action@v3` is already used elsewhere in the same files so the action is known to the linter's action graph.

2. **Re-run the failing acceptance-stage runs** once the change is merged on `main`:
   ```bash
   gh run rerun 25806336675 --failed --repo optivem/shop   # multitier-dotnet
   gh run rerun 25806401223 --failed --repo optivem/shop   # monolith-dotnet
   ```
   - Healthy SonarCloud: sonar job goes green on first attempt with no retry logs.
   - Transient SonarCloud 504: logs show `[wretry] attempt N/3 failed … retrying in 30s` (the wrapper's standard notice format), then the next attempt succeeds and the job goes green without any manual intervention.

3. **Trigger a fresh meta-prerelease pipeline** (`gh workflow run meta-prerelease-stage.yml --repo optivem/shop` or via the UI) and observe that all six acceptance-stage children run their sonar steps under the retry wrapper. On a healthy day this is invisible (one attempt, no retry logs); the test for the wrapper's correctness is just that the green-path runtime is unchanged.

4. **Negative check** (optional, manual): temporarily set `SONAR_TOKEN` to a value that causes a hard 401 on a feature branch, push, and verify that the wrapper does **not** mask the auth failure — it should still surface after 3 attempts. (The retry policy retries any non-zero exit, so a hard 401 will be retried but ultimately fails the step. This is the same behaviour the Docker login retries already exhibit and is considered acceptable: hard auth failures are rare and the extra 60s of retry delay is preferable to introducing per-error-class branching.)

## Rollout

- Edit all 6 files in a single commit in `optivem/shop` on a feature branch.
- Open a PR; squash-merge once green.
- No coordinated release with `optivem/actions` or `gh-optivem` is required — the change is self-contained within `shop`.
