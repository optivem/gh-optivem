# Plan: close shop dependency-resolution cache gaps

Date: 2026-05-15
Source audit: `gh-optivem/audits/20260515-dependency-resolution-retry-coverage.md` (the second §4 pass).
Parent plan: `gh-optivem/plans/20260514-fix-shop-workflow-retry-gaps.md` Item 20 (now replaced by this plan).
Scope: cache configuration for `npm ci` / `./gradlew` / `dotnet build|test` registry fetches in two specific places. Not a retry-engine extension.

---

## Why this plan exists (and why it's NOT a retry plan)

The first §4 pass deferred dependency-resolution registry fetches to a second pass on the assumption that *"the marginal failure rate is much lower because they're backed by setup-* cache paths and lockfile pins."* The second-pass audit verified the assumption and found it **holds for TypeScript and Java fleets, but breaks in two specific places**:

1. **`_prerelease-pipeline.yml`** has zero cache configuration of any kind — `setup-node@v6` / `setup-java@v5` / `setup-dotnet@v5` are all invoked without `cache:`. Every release-day prerelease hits npmjs / Maven Central / NuGet from cold. **11 sites uncached on the release path.**
2. **dotnet commit-stage workflows** invoke `setup-dotnet@v5` without `cache:` AND have no `actions/cache@v5` NuGet block (unlike their acceptance-stage siblings, which DO have one). **9 sites uncached on the commit path.**

The audit's explicit recommendation (Tier 3.5) is **do NOT extend the retry engine** with `npm-retry.sh` / `gradle-retry.sh` / `dotnet-retry.sh`. The canonical tool for the dependency-resolution registry-transient class is **caching + lockfile pin**, not retry. The incident history that drove §4 to exist (SonarCloud 504, GitHub GraphQL transient) has no comparable analogue in this workspace's npm / Maven Central / NuGet history.

This plan closes the two cache gaps. That's it.

---

## Tier 1 — cache configuration fixes

### Item C-1 — Add `cache:` parameters to `_prerelease-pipeline.yml` setup steps

**File:** `shop/.github/workflows/_prerelease-pipeline.yml`

Three setup steps, each missing `cache:`:

- Line ~117 — `actions/setup-node@v6` → add `cache: 'npm'` plus a multi-line `cache-dependency-path` covering the lockfiles touched by the job (`system/monolith/typescript/package-lock.json`, `system/multitier/backend-typescript/package-lock.json`, `system/multitier/frontend-react/package-lock.json`, `system-test/typescript/package-lock.json`).
- Line ~123 — `actions/setup-java@v5` → either add `cache: 'gradle'`, OR (preferred) replace the step with `gradle/actions/setup-gradle@v6` to match the commit-stage/acceptance-stage pattern across the rest of the repo (gives wrapper cache + dependency cache + an upstream-maintained surface).
- Line ~130 — `actions/setup-dotnet@v5` → add `cache: true` and `cache-dependency-path: '**/*.csproj'` (or `**/packages.lock.json` if the repo standardizes on lockfile-pinned NuGet).

Closes audit N-B.2 (11 sites: 4 `npm ci`, 4 `./gradlew compileJava compileTestJava`, 3 `dotnet build`).

**Severity rationale:** release path. A registry blip on a release-day prerelease blocks the entire pipeline, and there's no retry wrapper anywhere in this file.

Acceptance criteria:
- One green `_prerelease-pipeline.yml` run exercises the cached path (look for `Cache restored from key:` in the runner log).
- One green run on cache miss (first run after `package-lock.json` / `*.csproj` change) confirms the cache-key glob is correct.

### Item C-2 — Add NuGet cache to dotnet commit-stage workflows

**Files:**
- `shop/.github/workflows/monolith-dotnet-commit-stage.yml`
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml`

Pick one approach (preference A) — apply uniformly to both files:

**(A, preferred) `setup-dotnet@v5` built-in cache** — add `cache: true` and `cache-dependency-path: 'system/monolith/dotnet/**/*.csproj'` (resp. `system/multitier/backend-dotnet/**/*.csproj`) to the existing `setup-dotnet@v5` step. One-line change per file. Matches the shape used by the TypeScript fleet's `setup-node@v6 + cache: 'npm'`.

**(B, alternative) `actions/cache@v5` NuGet block** — copy the existing pattern from `monolith-dotnet-acceptance-stage.yml` (the block around line 236–242 with `path: ~/.nuget/packages, key: nuget-${{ runner.os }}-${{ hashFiles('**/*.csproj') }}`). Insert the block right after `setup-dotnet@v5`. 6 lines per file. More boilerplate but identical to the acceptance-stage shape; reviewers don't have to remember which `cache:` knob `setup-dotnet@v5` understands.

Closes audit N-B.1 (6 sites in commit-stage: 2 `dotnet build` + 2 `dotnet test` + 2 `dotnet build` inside Sonar block).

**Severity rationale:** medium. Commit-stage failures are re-runnable per push, but a registry blip on a PR commit-stage is exactly the developer-experience papercut caching is built to prevent.

Acceptance criteria:
- One green commit-stage run for each of the 2 dotnet projects exercises the cached path.
- The cache miss flow (after a `.csproj` change) restores in under ~10 seconds.

---

## Out of scope (explicitly rejected by the audit)

- **No new `npm-retry.sh` / `gradle-retry.sh` / `dotnet-retry.sh`.** See audit Tier 3.5 reasoning.
- **No matrix retry wrapper around `dotnet build` / `npm ci`.** Treating cache misconfiguration with `nick-fields/retry@v4` would re-introduce the AP.3 anti-pattern §4 was built to prevent.
- **No engine schedule change** for the existing `Pre-warm Gradle Wrapper` (R-OK at 24 sites). Leave it alone.

---

## Deferred to a future §3 (not §4) pass

- **Composite extraction `optivem/actions/dotnet-test-job@v1`.** The acceptance-stage-cloud dotnet workflows duplicate the `setup-dotnet` + `Cache NuGet` + `dotnet build` + `dotnet test` block 11× per file (22 sites × 2 files = 44 sites). The audit flagged this as AP.1 — it's not a retry gap (the retry posture is healthy R-DOC-OK via the NuGet cache), but a future cache-key change would have to land in 44 places. Worth a §3-G pass when bandwidth permits. Out of scope for this plan.
- **`dotnet tool install --global dotnet-sonarscanner` retry coverage.** Appears in 2 dotnet commit-stage Sonar blocks. Audit flagged it as out-of-scope for the *dependency-resolution* class (it's a tool install, not a dependency restore), but it does hit NuGet without retry. Per audit note, `cache: true` on `setup-dotnet@v5` (Item C-2 option A) covers tool installs too — so option A double-pays.

---

## Acceptance criteria (whole plan)

Plan is done when:
1. Items C-1 and C-2 land in `shop/`.
2. One green `_prerelease-pipeline.yml` run on cache-hit AND one on cache-miss.
3. One green commit-stage run for each of `monolith-dotnet` and `multitier-backend-dotnet` exercises the cached path.
4. `gh-optivem/plans/20260514-fix-shop-workflow-retry-gaps.md` Item 20 is deleted (the plan it pointed at — this one — has replaced it).
