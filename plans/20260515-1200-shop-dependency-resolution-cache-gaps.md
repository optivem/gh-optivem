# Plan: close dependency-resolution cache gaps in shop commit-stage and prerelease workflows

Date: 2026-05-15
Source: extracted from `audits/20260515-dependency-resolution-retry-coverage.md` (now deleted) Tier 1 recommendations. The audit covered §4 retry coverage for `npm ci` / `./gradlew build|test|assemble|check` / `dotnet restore|build|test` calls across `shop/.github/workflows/*.yml`. Its conclusion was that the right intervention for the two observed gaps is **caching + lockfile pin**, not retry — so this is a cache-gap plan, not a retry-gap plan.
Scope: cross-repo. All file changes land in `optivem/shop/.github/workflows/`; this plan lives in `gh-optivem` because that is where the audit it derives from lived and where this workspace tracks retry / external-I/O policy.

---

## Why a plan, not the audit

The audit concluded that:

- **TypeScript fleet** (22 `npm ci` sites) is well-cached via `actions/setup-node@v6` with `cache: 'npm'` and `cache-dependency-path:` — R-DOC-OK.
- **Java fleet** (44 `./gradlew` sites) is cached via `gradle/actions/setup-gradle@v6` plus an explicit `Pre-warm Gradle Wrapper` step using `nick-fields/retry@v4` (24 sites) — R-OK / R-DOC-OK.
- **.NET commit-stage** (9 sites) and **`_prerelease-pipeline.yml`** (8 npm/gradle + 3 dotnet = 11 sites) are uncached and untouched by any retry wrapper — N-B.

Two N-B clusters remain to close. The audit explicitly rejected extending the retry engine for this class (Tier 3.5: no `npm-retry.sh` / `gradle-retry.sh` / `dotnet-retry.sh`). The right tool is caching, and caching configuration is already the workspace convention everywhere except these two gaps.

## Items

### Item 1 — Add cache to `_prerelease-pipeline.yml` setup steps

**File:** `shop/.github/workflows/_prerelease-pipeline.yml`

This reusable compile-only stage runs before GoReleaser. Its `Setup Node.js` (line 117), `Setup Java` (line 123), and `Setup .NET` (line 130) steps all omit `cache:`. `grep -c cache _prerelease-pipeline.yml` → 0. Every prerelease pipeline invocation hits npmjs, Maven Central, and NuGet from cold. **Severity: high (release-path).**

Sites closed: 11 (npm/gradle/dotnet) in `_prerelease-pipeline.yml`:

- `_prerelease-pipeline.yml:137` — `npm ci && npx tsc --noEmit` (monolith typescript)
- `_prerelease-pipeline.yml:142` — `./gradlew compileJava compileTestJava` (monolith java)
- `_prerelease-pipeline.yml:147` — `dotnet build` (monolith dotnet)
- `_prerelease-pipeline.yml:153` — `npm ci && npx tsc --noEmit` (multitier backend typescript)
- `_prerelease-pipeline.yml:158` — `./gradlew compileJava compileTestJava` (multitier backend java)
- `_prerelease-pipeline.yml:163` — `dotnet build` (multitier backend dotnet)
- `_prerelease-pipeline.yml:169` — `npm ci && npm run build` (multitier frontend react)
- `_prerelease-pipeline.yml:175` — `npm ci && npx tsc --noEmit` (system tests typescript)
- `_prerelease-pipeline.yml:180` — `./gradlew compileJava compileTestJava` (system tests java)
- `_prerelease-pipeline.yml:185` — `dotnet build` (system tests dotnet)
- (lockfile globs cover the 11th site automatically once `cache-dependency-path` is multi-line.)

**Changes:**

- `actions/setup-node@v6` → add `cache: 'npm'` and a multi-line `cache-dependency-path:` covering every `package-lock.json` referenced by the matrix (`system/**/package-lock.json`, `system-test/typescript/package-lock.json`). `actions/setup-node`'s `cache-dependency-path:` accepts a multi-line glob — supported.
- `actions/setup-java@v5` → add `cache: 'gradle'`. **Or** replace with `gradle/actions/setup-gradle@v6` to match the commit-stage / acceptance-stage shape (gives both wrapper cache and dependency cache). Either is acceptable; prefer `setup-gradle@v6` for shape parity with the rest of the workspace.
- `actions/setup-dotnet@v5` → add `cache: true`. Provide `cache-dependency-path:` if the matrix has multiple `.csproj` globs; otherwise rely on auto-detect via `packages.lock.json`.

### Item 2 — Add NuGet cache to dotnet commit-stage workflows

**Files:**

- `shop/.github/workflows/monolith-dotnet-commit-stage.yml`
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml`

`actions/setup-dotnet@v5` is invoked with `dotnet-version: 8.0.x` and **no `cache:` parameter**. There is **no separate `actions/cache@v5`** step for `~/.nuget/packages`. Every push to main triggers a fresh `dotnet restore` against `api.nuget.org` with no recovery path. **Severity: medium (PR commit-stage; user can re-run).**

Sites closed: 6:

- `monolith-dotnet-commit-stage.yml:95` (`dotnet build` — compile source)
- `monolith-dotnet-commit-stage.yml:99` (`dotnet test`)
- `monolith-dotnet-commit-stage.yml:127` (`dotnet build` inside Sonar block — local cache hit, only after line 95 succeeds)
- `multitier-backend-dotnet-commit-stage.yml:95` (`dotnet build`)
- `multitier-backend-dotnet-commit-stage.yml:99` (`dotnet test`)
- `multitier-backend-dotnet-commit-stage.yml:124` (`dotnet build` inside Sonar block)

**Changes — pick one option per file, consistently across both files:**

- **(preferred) Option a:** Add `cache: true` to the `setup-dotnet@v5` step. One-line edit per file. Matches the shape used by `actions/setup-node@v6` with `cache: 'npm'` in the TypeScript fleet — minimal config drift across the workspace.
- **(alternative) Option b:** Insert an `actions/cache@v5` block for `~/.nuget/packages` keyed on `hashFiles('**/*.csproj')`. Matches the shape used by the dotnet acceptance-stage workflows already (e.g. `monolith-dotnet-acceptance-stage.yml:236-242`). More boilerplate per call site, but unambiguous.

**Rejected escape hatch:** wrapping each `dotnet build` / `dotnet test` with `nick-fields/retry@v4` is **not** the right fix — re-introduces ad-hoc retry in 9 places, fragments policy, treats the symptom rather than the cause. The audit's §4 rubric anti-pattern AP.3 explicitly warns against this.

## Out of scope

- **`actions/` repo composite extraction (AP.1).** A future `optivem/actions/dotnet-test-job@v1` composite that bundles `setup-dotnet` + NuGet cache + `dotnet build` + `dotnet test` would collapse the 22-sites-per-file duplication in `monolith-dotnet-acceptance-stage-cloud.yml` and `multitier-dotnet-acceptance-stage-cloud.yml`. Out of scope here; defer to a §3 audit pass.
- **Pre-warm Gradle schedule canonicalization.** The 24 `nick-fields/retry@v4` Pre-warm Gradle Wrapper sites in the Java acceptance-stage / -cloud / -legacy files use a schedule (3 attempts, 5-min timeout) that diverges slightly from the canonical engine's 4×{5s, 15s, 45s} ramp. The audit classified this as R-OK, not N-C; alignment is optional and can be done in a single sweep later if the engine schedule is canonicalized further.
- **`dotnet tool install --global dotnet-sonarscanner`.** Appears in 2 dotnet commit-stage files at the start of the Sonar block. Hits NuGet for the tool binary, no retry. Out of scope: build-tool install, not dependency restore. Note: setting `cache: true` on `setup-dotnet@v5` per Item 2 covers this for free per upstream docs.
- **Extending the retry engine** with `npm-retry.sh` / `gradle-retry.sh` / `dotnet-retry.sh`. Rejected by the audit's Tier 3.5. The failure modes that drove §4 (SonarCloud 504, GitHub GraphQL transient) have no comparable incident history on npm / Maven Central / NuGet in this workspace.

## Acceptance criteria

1. `_prerelease-pipeline.yml` has `cache:` parameters on its three `setup-*` steps (Item 1).
2. Both dotnet commit-stage workflows have NuGet caching enabled, via either `cache: true` on `setup-dotnet@v5` or an `actions/cache@v5` block (Item 2).
3. A re-run of the audit produces zero N-B findings in the dependency-resolution class.
4. One successful prerelease pipeline run (manual `workflow_dispatch` on a throwaway branch is sufficient) confirms the cached path executes end-to-end.

## Re-audit trigger

If a registry-transient failure surfaces on a commit-stage or `_prerelease-pipeline.yml` run **after** the items above land, the right response is to extend the existing `nick-fields/retry@v4` Pre-warm Gradle Wrapper pattern (already R-OK by §4 rule) — **not** to invent a new bash-sourced wrapper.
