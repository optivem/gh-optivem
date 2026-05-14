# Shop workflow retry coverage — §4 audit

Date: 2026-05-14
Scope: `shop/.github/workflows/*.yml` (86 workflow files, 7 retry-engine scripts excluded) and `actions/**/action.yml` + sibling `*.sh` (42 composite actions).
Audit class: §4 only — external-I/O retry coverage. Read-only on workflow / composite files.
Rubric: `gh-optivem/.claude/agents/workflow-auditor.md` §4 (N-A / N-B / N-C, R-OK / R-DOC-OK).

---

## TL;DR

The seed gap is **`actions/publish-tag/tag.sh:36` and `actions/check-sha-on-branch/check.sh:14`** — two composite-level network calls (`git push`, `git fetch`) that every flavor's prerelease and prod stage funnels through, with no retry. A transient network blip on these will fail the whole stage and leave no recovery path inside the action. Composite-level fixes cascade across all consumer workflows in one change, so they go first.

The seed incident class is **`Wandalen/wretry.action@v3` with the wrong schedule.** It appears in 167 call sites across 51 shop workflows wrapping `docker/login-action@v4` at `attempt_limit: 3, attempt_delay: 10000`, and once wrapping `./run-sonar.sh` at `attempt_limit: 3, attempt_delay: 30000`. The 30000ms-Sonar one is the very call site that drove §4 to exist (acceptance run 25865827466 — SonarCloud 504s). All 167 are N-C: retry-present-but-misconfigured against the canonical 4×{5s,15s,45s} engine schedule. The single Sonar one is the highest-severity N-C — it diverges from `sonar-retry.sh`, which now exists specifically to cover its failure mode.

The most urgent latent failure is **`meta-release-stage.yml:334`** — it sources `gh-retry.sh` and then calls `git_push_retry`, a function that exists nowhere in the workspace. The step will fail with `command not found` the first time a meta release fires after this code path was added. Flagging it under N-B with the "broken wrapper" call-out because the intent was retry-wrapped but the implementation is dead code.

Beyond that, the long-tail items are:

- **Inline `for attempt in 1 2 3; do ... sleep $((attempt * 15)); done` retry loops** wrapping `docker pull mcr.microsoft.com/...` in 7 commit-stage workflows. The schedule (15s/30s/45s, 3 attempts) is close to canonical (4×{5,15,45}) but not identical, and is duplicated 7× — exactly the drift pattern §4 was written to prevent. N-C anti-pattern.
- **`gh extension install optivem/gh-optivem`** appears unwrapped in 12 workflow files (acceptance-stage, drift, cross-lang, prerelease-pipeline). Each is a hit to the GitHub releases API for the extension binary. Single biggest N-A class by call-site count.
- **`Install actionlint`** in `lint-workflows.yml:31` curls a raw URL with no retry.
- **`meta-prerelease-stage.yml:234`** has inline `git push origin "$TAG"` with no retry. Inconsistent with `meta-release-stage.yml`, which (correctly, in intent) wraps the equivalent call.

---

## Summary

- Files scanned: 86 shop workflows + 42 composite `action.yml` (sibling `*.sh` included for run-block analysis).
- External-I/O candidates classified: **R-OK** 14 distinct call types, **R-DOC-OK** 9 distinct call types, **N-A** 4 distinct call types (220+ call sites), **N-B** 6 distinct call types (5 inline + 5 composite-level), **N-C** 4 distinct call types (200+ call sites, dominated by the Wandalen/docker-login pattern).
- §4 findings — N-A: 4 (aggregated) · N-B: 6 · N-C: 4 (aggregated)
- §4 anti-patterns: 3 distinct (continue-on-error swallow, inline retry loop with hard-coded schedule, broken-wrapper-by-undefined-function).
- §4 healthy patterns: 4 composite `gh_retry` wrappers exercised across 30+ composite-action call sites; `meta-prerelease-dry-run.yml`, `meta-release-stage.yml`, and the 4 commit-stage workflows (java + dotnet mono + multitier) properly source the engine.

Cap of 20 findings respected (this report emits 14 distinct items: 4 N-A, 6 N-B, 4 N-C). No overflow.

---

## §4 findings

### Category N-A — `gh` call without retry

#### N-A.1 — `gh extension install optivem/gh-optivem` (12 sites)

Network I/O to the GitHub releases API to fetch the extension binary. Not wrapped anywhere; on a transient 5xx every downstream "smoke / acceptance / e2e" suite that depends on `gh optivem` is dead in the water.

Call sites (all at the start of the `run` job):

- `shop/.github/workflows/cross-lang-system-verification.yml:144`
- `shop/.github/workflows/drift.yml:108, 295`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml:280`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage-legacy.yml:244`
- `shop/.github/workflows/monolith-java-acceptance-stage.yml:277`
- `shop/.github/workflows/monolith-java-acceptance-stage-legacy.yml:244`
- `shop/.github/workflows/monolith-typescript-acceptance-stage.yml:266`
- `shop/.github/workflows/monolith-typescript-acceptance-stage-legacy.yml:235`
- `shop/.github/workflows/multitier-dotnet-acceptance-stage.yml:285`
- `shop/.github/workflows/multitier-dotnet-acceptance-stage-legacy.yml:250`
- `shop/.github/workflows/multitier-java-acceptance-stage.yml:282`
- `shop/.github/workflows/multitier-java-acceptance-stage-legacy.yml:250`
- `shop/.github/workflows/multitier-typescript-acceptance-stage.yml:271`
- `shop/.github/workflows/multitier-typescript-acceptance-stage-legacy.yml:241`
- `shop/.github/workflows/_prerelease-pipeline.yml:194`

(Counted 12 unique files; 16 total occurrences when counting both legs of `drift.yml`.)

Recommendation: source `$GITHUB_WORKSPACE/.github/workflows/scripts/gh-retry.sh` and switch to `gh_retry extension install optivem/gh-optivem`. Cheapest possible fix: a single-line source + one-word prefix at each call site. Bulk-replaceable.

#### N-A.2 — `gh workflow run bump-patch-version.yml --repo "$repo"`

`shop/.github/workflows/bump-patch-version-multirepo.yml:43`. Cross-repo dispatch for sibling repos (intentional `--repo` — not a §1-B finding). No retry wrapper. Recommendation: source `gh-retry.sh` and call `gh_retry workflow run bump-patch-version.yml --repo "$repo"`.

#### N-A.3 — `gh optivem test setup/run`, `gh optivem system start`

These are **R-DOC-OK**, not N-A. The `gh` CLI here is acting as a thin dispatcher to the `gh-optivem` extension binary, which runs entirely locally (docker compose, dotnet test, npm run, etc.) — there is no GitHub API call on the hot path of `gh optivem test run --suite acceptance-api`. Listed here for completeness so they're not misclassified by a future audit. Total: 100+ call sites across the acceptance-stage / acceptance-stage-legacy / drift / cross-lang / _prerelease-pipeline workflows. **No action.**

#### N-A.4 — `gh release view` / `gh api .../releases` probes

R-DOC-OK. Used inside `meta-release-stage.yml` (already wrapped via `gh_retry release view`) and inside `actions/check-tag-exists`, where the rc is consumed as truth-value. Per §4's false-positive rule, probes that drive flow control are not flagged. **No action.**

### Category N-B — Other network call without retry

#### N-B.1 — `actions/publish-tag/tag.sh:36` — `git push "$push_target" "$TAG"`

Composite-level. Every flavor's `*-acceptance-stage.yml` job that mints an RC calls `optivem/actions/publish-tag@v1` after running tests — line 36 in the action's `tag.sh` is the actual push to origin. No retry. A transient `connection reset` or `5xx` from GitHub at this point fails the whole acceptance stage; the local `git tag` already exists, the push didn't happen, and the next run will re-mint a different RC number against a different timestamp.

Note: the script already has a defensive recovery for the concurrent-push race at lines 41-46 (re-probes the tag's SHA after a push failure and tolerates an exact match). That handles the rare race but does nothing for a transient — the failure surfaces as the `::error::Failed to push tag` at line 48.

Recommendation: source `optivem/actions/shared/retry-core.sh` in `tag.sh` and wrap the push as `retry_with_policy '<git push transient regex>' '<auth/permission hard-fail regex>' git-push -- git push "$push_target" "$TAG"`. Two patterns to encode in the regex: `Could not resolve host`, `Connection reset by peer`, `RPC failed; HTTP 5\d\d`, `Operation timed out`, `unable to access`, `EOF`. Hard-fail set: `Permission denied`, `403`, `! \[remote rejected\]`, `pre-receive hook declined`. The simplest implementation is a thin `git-retry.sh` wrapper sibling to `gh-retry.sh` / `docker-retry.sh`.

#### N-B.2 — `actions/check-sha-on-branch/check.sh:14` — `git fetch origin "$BASE_BRANCH"`

Composite-level. `check-sha-on-branch` is called by every commit-stage / acceptance-stage gate to decide `on-branch=true|false`. A transient on `git fetch` kills the workflow at the gate, before any work begins, with no recovery. Recommendation: wrap with the same `git_retry` helper proposed for N-B.1. Note the `--quiet` flag — the wrapper will need to capture stderr explicitly to keep the retry policy regex working.

#### N-B.3 — `actions/cleanup-prereleases/action.yml:43` — `git fetch --tags --force`

Composite-level. Runs at the start of every cleanup pass. Same shape as N-B.2 — failure here aborts the cleanup. Recommendation: same wrapper.

#### N-B.4 — `meta-prerelease-stage.yml:234` — inline `git push origin "$TAG"`

The `tag-meta-rc` job pushes the meta-rc annotated tag with no retry. Inconsistent with `meta-release-stage.yml`'s `tag-meta-release` step, which intentionally wraps the equivalent operation (though see N-B.5 — that wrapping is broken). A transient at this point loses the meta-rc tag and forces the entire meta-prerelease pipeline to re-run from scratch. Recommendation: source `gh-retry.sh` (already in the workspace), define / source a `git_push_retry` helper, and call `git_push_retry origin "$TAG"`. See plans/20260514-2200-retry-helpers-canonical-home.md for the proposed home of the helper.

#### N-B.5 — `meta-release-stage.yml:334` — `git_push_retry origin "$RELEASE_TAG"` references an **undefined function**

The step sources `gh-retry.sh` (line 329), which does not define `git_push_retry`. No other sourced file in this run defines it either. On execution, bash will exit with `git_push_retry: command not found` (or under `set -euo pipefail`, fail the step). This is currently latent because `meta-release-stage` runs rarely, but it is a **guaranteed-fail on next invocation** — strictly worse than a missing retry, because the original `git push` is gone too.

Recommendation, ranked: (a) add `git_push_retry` to a new `shared/git-retry.sh` (preferred — same shape as `gh-retry.sh` / `docker-retry.sh`, fits the existing engine); (b) define it inline above the call as a quick stopgap; (c) revert to `git push origin "$TAG"` without retry (matches N-B.4 but loses the retry the author clearly intended). Option (a) also resolves N-B.1 / N-B.2 / N-B.3 / N-B.4 in one stroke and is the right long-term home.

#### N-B.6 — `lint-workflows.yml:31` — `bash <(curl -fsSL https://raw.githubusercontent.com/rhysd/actionlint/.../download-actionlint.bash)`

Pure `curl` install from raw.githubusercontent.com. No retry. Recommendation: wrap with `nick-fields/retry@v4` at `max_attempts: 4`, `retry_wait_seconds: 5`, `polling_interval_seconds: 0`, `timeout_minutes: 1` (cheap enough that 4×{5,15,45} matches the engine schedule). Lower priority than N-B.1–5 because lint-workflows is the only job hit and it is itself non-critical to the release path.

#### N-B.7 — `npm ci`, `npx playwright install chromium`, `./gradlew build`, `dotnet build` (deferred to examined-and-rejected)

The §4 rubric lists `npm install`, `mvn deploy`, `dotnet restore` as N-B candidates. The shop workflows that run these (12+ `npm ci` in `monolith-typescript-acceptance-stage-cloud.yml` alone, 12+ in the multitier variant; `./gradlew build` in 2 commit-stage files; `dotnet build` in 30+ sites in the acceptance-stage-cloud files) are all backed by `actions/setup-node@v5` / `actions/setup-java@v5` / `actions/setup-dotnet@v5` with cache-fetch paths and lockfile pins. The transient-failure surface is registry-side (npmjs, Maven Central, NuGet) and is real, but a single retry sweep across that many call sites is much larger than this audit's cap. See **Examined-and-rejected** for the deferral rationale and the order in which they'd be addressed in a follow-up §4 pass.

### Category N-C — Retry present but misconfigured

#### N-C.1 — `Wandalen/wretry.action@v3` wrapping `docker/login-action@v4` at `attempt_limit: 3, attempt_delay: 10000`

167 call sites across 51 shop workflows. Wraps DockerHub login + GHCR login at the start of every commit-stage / qa-stage / acceptance-stage / prod-stage / acceptance-stage-legacy / acceptance-stage-cloud / qa-stage-cloud / prod-stage-cloud variant. Schedule is 3 attempts × 10s fixed delay. Diverges from the canonical 4 attempts × {5s, 15s, 45s} engine schedule on both the attempt count (3 vs 4) and the back-off curve (flat vs exponential).

Representative examples (one per workflow archetype — others omitted but identical shape):

- `shop/.github/workflows/monolith-dotnet-commit-stage.yml:80, 92` (DockerHub + GHCR login)
- `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml:93, 103, 176, 186, 388`
- `shop/.github/workflows/monolith-dotnet-acceptance-stage-cloud.yml:77, 87, 148, 197, 242` (3× GHCR login, one per deploy job)
- `shop/.github/workflows/monolith-dotnet-prod-stage.yml:125, 135`
- `shop/.github/workflows/monolith-dotnet-prod-stage-cloud.yml:107, 134, 197`
- `shop/.github/workflows/drift.yml:111, 121` (DockerHub + GHCR for cross-lang)
- `shop/.github/workflows/cross-lang-system-verification.yml:147, 157`

(167 total. Full list omitted — the pattern is mechanical and the file/line set is reproducible by `rg -nU '^\s+uses: Wandalen/wretry\.action@v3$' shop/.github/workflows`.)

Recommendation: replace the `Wandalen/wretry.action@v3` wrapper with the upstream `docker/login-action@v4` invoked directly under a thin `nick-fields/retry@v4` step (or via a new `optivem/actions/docker-login@v1` composite that hides the retry policy). Either route should encode `max_attempts: 4, retry_wait_seconds: 5, polling_interval_seconds: 0, on_retry_command: ...` to match the engine's 4×{5,15,45} schedule. Note: `docker/login-action@v4` is idempotent (re-logins are safe), so the retry semantics are equivalent — only the schedule changes.

This is the single largest §4 finding by call-site count. Recommend handling it as one workspace-wide swap (one composite, one PR) rather than per-workflow.

#### N-C.2 — `Wandalen/wretry.action@v3` wrapping `./run-sonar.sh` at `attempt_limit: 3, attempt_delay: 30000`

Six call sites — one in each non-cloud acceptance-stage workflow's `sonar` job:

- `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml:388`
- `shop/.github/workflows/monolith-java-acceptance-stage.yml:~388` (same shape; line by inspection)
- `shop/.github/workflows/monolith-typescript-acceptance-stage.yml:~388`
- `shop/.github/workflows/multitier-dotnet-acceptance-stage.yml:~388`
- `shop/.github/workflows/multitier-java-acceptance-stage.yml:~388`
- `shop/.github/workflows/multitier-typescript-acceptance-stage.yml:~388`

(Locations are mechanical — same shape across the 6 acceptance-stage files. Exact lines vary by ~10 lines depending on the matrix above.)

This is the **incident-driven N-C**. Run 25865827466 failed all 4 dotnet matrix combos on SonarCloud 504; the `sonar-retry.sh` engine was extracted to handle that failure mode specifically. The current `Wandalen` wrapper here is a stop-gap patch: 3×30s flat is incidentally close to "wait through a brief outage," but it does not pass through hard-fail signals (401 / 403 / project-not-found) — those will be silently retried 3 times before surfacing — and it does not encode the SonarCloud-specific `Error 5XX on https://` / `Endpoint request timed out` transient regex that `sonar-retry.sh` is built around.

Recommendation: replace the `Wandalen/wretry.action@v3` step with an inline `run:` that sources `$GITHUB_WORKSPACE/.github/workflows/scripts/sonar-retry.sh` and invokes `sonar_retry bash ./run-sonar.sh` (or, if `run-sonar.sh` itself contains the scanner invocation, edit it to `sonar_retry`-wrap the scanner call internally). The TypeScript variants additionally have the marketplace `SonarSource/sonarqube-scan-action@v7` path (see N-C.4) which needs a separate decision.

This is the highest-severity N-C — small surface, high incident weight, exact wrapper exists.

#### N-C.3 — Inline `for attempt in 1 2 3; do ... sleep $((attempt * 15)); done` wrapping `docker pull mcr.microsoft.com/...`

7 commit-stage workflows, all bash bodies identical except for the image list:

- `shop/.github/workflows/monolith-dotnet-commit-stage.yml:179-189`
- `shop/.github/workflows/monolith-java-commit-stage.yml:185-195`
- `shop/.github/workflows/monolith-typescript-commit-stage.yml:183-193`
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml:176-186`
- `shop/.github/workflows/multitier-backend-java-commit-stage.yml:185-195`
- `shop/.github/workflows/multitier-backend-typescript-commit-stage.yml:183-193`
- `shop/.github/workflows/multitier-frontend-react-commit-stage.yml:184-194`

Step name in all: "Pre-pull base images (retry transient registry 403s)". Schedule: 3 attempts at `attempt * 15`s back-off = 15s, 30s, 45s. Diverges from canonical 4×{5,15,45}. Per the §4 rule the schedule will drift further the first time the canonical regex moves. Also note the step name's claim "retry transient registry 403s" — `docker-retry.sh`'s hard-fail regex would *not* retry HTTP 4xx (403 is auth/permission, not a transient — the canonical engine treats it as hard-fail and surfaces it immediately). The inline loop's "retry on any failure" approach masks real auth misconfigurations.

Recommendation: source `$GITHUB_WORKSPACE/.github/workflows/scripts/docker-retry.sh` and replace the loop with `for image in ...; do docker_retry pull "$image"; done`. This also picks up §3-G consolidation (the 7 inline copies become one source line) and the §4-anti-pattern fix (inline retry loop with hard-coded schedule).

#### N-C.4 — `SonarSource/sonarqube-scan-action@v7` (no explicit attempt knobs)

Three call sites:

- `shop/.github/workflows/monolith-typescript-commit-stage.yml:130`
- `shop/.github/workflows/multitier-backend-typescript-commit-stage.yml:130`
- `shop/.github/workflows/multitier-frontend-react-commit-stage.yml:131`

The action delegates to `sonar-scanner`, which has the same transient-failure profile as `dotnet sonarscanner end` (the call site that drove §4). The action itself does not expose `attempt_limit` / `max_attempts` inputs, so per the §4 rubric's R-OK definition ("marketplace retry actions used with explicit attempt_limit/max_attempts") this is **not R-OK** — it is N-C-adjacent.

Two possible classifications: (a) **N-B** if the action has no internal retry (treat as raw scanner invocation with no retry wrapper), (b) **N-C** if it has some internal retry that differs from the engine. Without consulting upstream release notes inside this audit, I am classifying as N-C with a verification step: confirm whether `sonarqube-scan-action@v7` retries internally before deciding the fix shape.

Recommendation:
- If it retries internally — document the schedule in a comment at each call site so a future audit pass can mark R-DOC-OK, and leave the call as-is.
- If it does not — switch to a `run:` step that sources `sonar-retry.sh` and invokes `sonar_retry sonar-scanner ...` (the same fix as N-C.2 but starting from a marketplace action instead of a `run:` script).

The action's failure mode is identical to the dotnet path's; aligning is cheap.

### §4 anti-patterns

#### AP.1 — `continue-on-error: true` as retry substitute

Not observed in shop workflows. Composite scripts have a sibling pattern: `git push origin --delete "refs/tags/$tag" 2>/dev/null || echo "Warning: ..."` in `actions/cleanup-prereleases/cleanup-prereleases.sh:162`, and `git fetch --tags --force origin >/dev/null 2>&1 || true` in `actions/check-changes-since-tag/check.sh:7` and in `actions/cleanup-prereleases/cleanup-prereleases.sh` (similar shape). These are best-effort cleanup writes / probes — the swallow is intentional and the user gets a warning. They are not strictly the "retry substitute" anti-pattern (no transient is hidden — failure is reported as a warning) but they would benefit from the engine's hard-fail vs transient distinction.

Recommendation: leave as-is in this audit pass; revisit if these are observed failing in the wild. No fix item in the plan.

#### AP.2 — `if: failure()` re-running non-idempotent work

Not observed in scope. The `meta-release-stage.yml` `tag-meta-release` step handles its idempotency in-band (lines 348-362 — checks for `already_exists` after `gh_retry release create` returns non-zero, then re-probes via `gh_retry release view`). This is the *correct* idempotent-pattern that the anti-pattern flag is designed to encourage, not avoid. No fix item.

#### AP.3 — Inline retry loops with hard-coded schedules

Already covered as **N-C.3** above (the docker-pull `for attempt in 1 2 3` pattern in 7 commit-stage files). Listed here for cross-rubric completeness.

#### AP.4 — Long-running command wrapped without per-attempt timeout

Not strongly observed. The Sonar N-C.2 step is the candidate — Sonar scans can hang on a connection that never times out at the TCP layer. Once it migrates to `sonar_retry ./run-sonar.sh`, wrapping in `timeout 5m` per attempt would be the belt-and-braces fix. Captured under the N-C.2 recommendation, not as a separate item.

#### AP.5 — Broken-wrapper-by-undefined-function (new — propose adding to §4 rubric)

`meta-release-stage.yml:334` invokes `git_push_retry`, an undefined function. This is a guaranteed-fail at runtime, not just a missing retry. Suggest the §4 rubric explicitly capture this class — "retry wrapper invoked but no source file defines it" — under N-B (or as a new N-D) so future audits surface it before the step fires. Captured as **N-B.5** above for this audit.

### §4 healthy patterns (R-OK / R-DOC-OK)

#### R-OK observed

- **`gh_retry`** — sourced and used correctly in:
  - `shop/.github/workflows/meta-prerelease-dry-run.yml:146-147` (`gh_retry workflow run`)
  - `shop/.github/workflows/meta-release-stage.yml:141-153` (probe via `gh_retry api`, hard-fail-aware)
  - `shop/.github/workflows/meta-release-stage.yml:222-228` (`gh_retry api .../statuses --paginate`)
  - `shop/.github/workflows/meta-release-stage.yml:340-368` (release create with idempotent already-exists handling)
  - `shop/.github/workflows/meta-release-stage.yml:438` (cross-repo dispatch)
  - `actions/cleanup-prereleases/cleanup-prereleases.sh` (28 usages)
  - `actions/bulk-update-project-item-status/bulk-update-project-item-status.sh:28, 71` (GraphQL — the exact call site that drove §4 from incident 25877369208)
  - `actions/resolve-project-status-field/resolve-project-status-field.sh:24` (GraphQL probe)
  - `actions/wait-for-workflow/wait.sh`, `actions/trigger-and-wait-for-workflow/{trigger,wait}.sh`, `actions/create-deployment/create.sh`, `actions/create-commit-status/create.sh`, `actions/get-commit-status/read.sh`, `actions/get-last-workflow-run/get.sh`, `actions/cleanup-deployments/cleanup-deployments.sh`, `actions/cleanup-ghcr-orphan-manifests/cleanup-ghcr-orphan-manifests.sh`, `actions/check-commit-status-exists/check.sh`, `actions/commit-files/commit.sh`, `actions/resolve-latest-deployed-prerelease/resolve.sh`, `actions/resolve-latest-prerelease-with-status/resolve.sh`. (15+ composite actions wrap `gh` correctly.)
- **`docker_retry`** — `actions/tag-docker-images/tag-docker-images.sh`, `actions/resolve-docker-image-digests/resolve-docker-image-digests.sh`, `actions/deploy-docker-compose/start.sh` (all use `docker_retry buildx imagetools` / `docker_retry inspect` / `docker_retry pull`).
- **`sonar_retry`** — `shop/.github/workflows/monolith-dotnet-commit-stage.yml:132` (`sonar_retry dotnet sonarscanner end ...`), `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml:129`, `shop/.github/workflows/monolith-java-commit-stage.yml:138` (`sonar_retry ./gradlew sonar --info`), `shop/.github/workflows/multitier-backend-java-commit-stage.yml:138`.
- **`docker/build-push-action@v6`/`@v7`** — 7 commit-stage files. The §4 rubric calls out this action as having documented internal retry semantics (R-OK).

#### R-DOC-OK observed

- `gh release view <tag> >/dev/null 2>&1 || ...` probe — 1 site in `meta-release-stage.yml` (line 353 — inside the `already_exists` handler, where the exit code is consumed as truth).
- `git rev-parse --verify --quiet refs/tags/...` — 4 sites in `_meta-prerelease-pipeline.yml` (probes for tag existence; local-only).
- `git describe --tags --abbrev=0 --match=...` — 1 site in `meta-prerelease-stage.yml:81` (probe with documented `No names found` recovery).
- `git ls-remote --tags` — 3 sites in `actions/publish-tag/tag.sh`, `actions/check-tag-exists/check.sh`, `actions/bump-patch-versions/bump.sh`, `actions/validate-tag-exists/validate.sh`, `actions/resolve-latest-tag-from-sha/resolve.sh`, `actions/resolve-latest-prerelease-tag/resolve.sh` — all probes consumed as truth-values.
- `git merge-base --is-ancestor` — local-only computation. `actions/check-sha-on-branch/check.sh:20`.
- `gh optivem test setup/run`, `gh optivem system start` — local extension dispatch (covered under N-A.3).
- `dotnet build`, `dotnet test`, `npx playwright test`, `./gradlew compileJava compileTestJava`, `./gradlew build` (the build/test paths, not the registry-fetch paths) — local builds.
- `docker compose up/down`, `docker compose build` — local container orchestration.
- `if docker pull "$image"; then ...; fi` — this construct in the commit-stage workflows IS embedded inside an N-C.3 retry loop; the loop is the finding, not the individual probe.

---

## Examined-and-rejected

These were considered for inclusion and deliberately not flagged.

- **`npm ci`, `./gradlew build`, `dotnet build`, `dotnet test`, `npx playwright install`, `npx playwright test`.** Per §4's "common signals" list, `npm install` and `mvn dependency:resolve` are N-B candidates. The shop workflows that run these are all backed by `actions/setup-node@v5` / `actions/setup-java@v5` / `actions/setup-dotnet@v5` with cache-fetch paths and lockfile pins, which limits the network hop to first-build-of-PR. Furthermore, a `gh optivem test setup`-driven flow handles most invocations behind the `gh optivem` extension. The inline `npm ci` calls in `*-acceptance-stage-cloud.yml` files are the most exposed (12 per file × 6 files = ~72 sites) and would be the right N-B class for a dedicated follow-up §4 pass. Not flagged in this audit to stay under the 20-finding cap and because the marginal failure rate is much lower than the docker / sonar incidents that drove §4 to exist.

- **`google-github-actions/auth@v3`, `google-github-actions/setup-gcloud@v3`, `google-github-actions/deploy-cloudrun@v3`** (in the `*-stage-cloud.yml` files). These are marketplace actions whose primary author handles retry semantics for their own network calls (token exchange, gcloud SDK install, Cloud Run deploy). Per the §4 R-OK rule they qualify as long as the documented retry semantics are present. Not verified inside this audit; flagged here so a future pass can confirm or downgrade to N-C.4-style "verify-or-flag."

- **`actions/setup-node@v5`, `actions/setup-java@v5`, `actions/setup-dotnet@v8`, `actions/setup-go@v6`, `actions/cache@v5`, `actions/upload-artifact@v5`, `actions/download-artifact@v6`, `gradle/actions/setup-gradle@v6`, `docker/setup-buildx-action@v4`** — all marketplace actions with documented internal retry on the cache-fetch / artifact-upload paths. R-OK by §4 rule. Not flagged.

- **`cleanup-prereleases.sh:162` — `git push origin --delete "refs/tags/$tag" 2>/dev/null` with `Warning: Could not delete remote tag ...`** — best-effort cleanup write, swallow-and-warn is intentional and documented in the surrounding code. Adding a retry would extend the cleanup pass duration without changing behavior (a transient on a delete that's about to retry the loop next run is not user-visible). Not flagged — but noted for follow-up if observed failing.

- **`actions/check-changes-since-tag/check.sh:7` — `git fetch --tags --force origin >/dev/null 2>&1 || true`** — defensive probe before the inner `git tag --list` walk. The `|| true` swallow is correct: if the fetch fails, the inner walk falls back to whatever tags are already in the local clone (which the caller is required to have fetch-depth: 0'd). Adding retry has marginal value. Not flagged.

- **`meta-release-stage.yml:213` — `git fetch origin "refs/tags/${rc}:refs/tags/${rc}" 2>/dev/null || true`** — defensive recovery inside the manifest-verification loop. The `|| true` is followed by a fail-loud `git rev-parse --verify --quiet` check that exits 1 if the tag still isn't visible. Correct pattern. Not flagged.

- **`gh release view "$RELEASE_TAG" >/dev/null 2>"$view_err"` in `meta-release-stage.yml:353`** — probe consumed as truth-value inside the `already_exists` recovery. R-DOC-OK per §4 false-positive rule.

- **`gh extension install optivem/gh-optivem` in `gh-optivem`'s own repo workflows** (out of audit scope but mentioned for cross-reference): the scope of this audit is `shop/` and `actions/` only. The gh-optivem repo's workflows are not re-audited here.

---

## Recommended order of fixes

Composite-level fixes go first because each cascades to every workflow consumer. Within tier, ordered by blast radius (sites failed per incident).

### Tier 1 — composite-level (cascade)

1. **N-B.5 (`meta-release-stage.yml:334` undefined `git_push_retry`)** — guaranteed-fail at next invocation. Resolved by Tier 1.2.
2. **N-B.1 / N-B.2 / N-B.3 + N-B.4** — add `optivem/actions/shared/git-retry.sh` defining `git_push_retry` (and optionally `git_fetch_retry`). One file, four call-site swaps. Resolves all of these in one PR.
3. **N-C.2 (Sonar — incident-driven)** — 6 acceptance-stage files. Swap the `Wandalen/wretry.action@v3` step for an inline `run:` sourcing `sonar-retry.sh`. Highest-severity N-C.
4. **N-C.4 (SonarSource action — verify-or-fix)** — 3 typescript commit-stage files. Either verify internal retry semantics and document, or migrate to `sonar_retry`.

### Tier 2 — workflow-level batch swaps

5. **N-C.1 (Wandalen + docker/login-action @ 167 sites)** — wrap into a single composite `optivem/actions/docker-login@v1` (or equivalent). Cascade-replace across 51 files in one PR.
6. **N-C.3 (inline docker-pull retry loop @ 7 sites)** — replace each `for attempt in 1 2 3` block with `docker_retry pull "$image"`. Also closes the §3-G duplication.

### Tier 3 — leaf workflow fixes

7. **N-A.1 (`gh extension install optivem/gh-optivem` @ 12 sites)** — bulk-replace with `gh_retry extension install ...`.
8. **N-A.2 (`bump-patch-version-multirepo.yml` cross-repo dispatch)** — single-line fix.
9. **N-B.6 (`lint-workflows.yml` curl)** — wrap with `nick-fields/retry@v4`. Lowest priority — non-release path.

### Tier 4 — deferred / verify-only

10. N-B.7 (`npm ci` / `./gradlew build` / `dotnet restore` registry-fetch paths) — defer to a follow-up §4 pass with its own report.
11. `google-github-actions/*@v3` verification — confirm marketplace retry semantics in upstream release notes, mark R-OK or open a new audit item.

---

## Audit metadata

- Generated by: `workflow-auditor` agent, §4-only mode.
- Reviewer: this audit is read-only on workflow files. The matching plan (`plans/20260514-fix-shop-workflow-retry-gaps.md`) names one fix item per finding in the order above.
- Counts: 14 distinct findings (4 N-A, 6 N-B, 4 N-C) plus 5 anti-pattern observations. Cap of 20 respected.
