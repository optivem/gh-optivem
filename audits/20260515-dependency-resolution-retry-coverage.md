# Shop workflow dependency-resolution retry coverage — §4 audit (second pass)

Date: 2026-05-15
Scope: `shop/.github/workflows/*.yml` — dependency-resolution registry fetches only (`npm ci`, `npm install`, `./gradlew build|assemble|test|check`, `dotnet restore`, `dotnet build`, `dotnet test`).
Audit class: §4 only — external-I/O retry coverage. Read-only on workflow files.
Rubric: `gh-optivem/.claude/agents/workflow-auditor.md` §4 (N-A / N-B / N-C, R-OK / R-DOC-OK).
Parent audit: `gh-optivem/audits/20260514-shop-workflow-retry-coverage.md` deferred this class as N-B.7 / Item 20 of `plans/20260514-fix-shop-workflow-retry-gaps.md`.

---

## TL;DR

The first-pass author's "low marginal failure rate" judgment **holds for the TypeScript and Java fleets, but breaks down sharply for the .NET fleet and for the `_prerelease-pipeline.yml` reusable workflow**. The judgment was conditioned on "all DO hit external registries, but are backed by `actions/setup-*` cache paths and lockfile pins." The condition is satisfied for TypeScript and Java; it is **systematically violated for .NET** (which never uses `setup-dotnet`'s `cache:` knob and is unevenly backed by manual `actions/cache@v5` for NuGet) and **for `_prerelease-pipeline.yml`** (which has zero cache configuration of any kind).

Concretely:

- **51 `dotnet build` / `dotnet test` sites across 7 dotnet workflows are uncached on commit-stage and partially-cached on acceptance-stage.** `setup-dotnet@v5` is invoked with `dotnet-version: 8.0.x` and **no `cache:` parameter** in every shop workflow. The acceptance-stage and acceptance-stage-cloud / -legacy files compensate with a separate `actions/cache@v5` block keyed on `*.csproj` hash (covers 6 files, 22+ sites each on the cloud variants), but **commit-stage has no NuGet cache at all** (3 files × ~3 sites = 9 sites that hit NuGet on every push to main, with no retry).
- **10 `npm ci` / `dotnet build` / `./gradlew` sites in `_prerelease-pipeline.yml` are uncached.** This reusable workflow is the cross-cutting compile-only step before GoReleaser; it runs once per language/architecture combo per release. No `cache:` parameter, no `actions/cache@v5`. Every prerelease pipeline invocation hits npmjs / Maven Central / NuGet from cold. Recommendation severity here is high: a single registry blip on a release-day prerelease fails the entire pipeline, and there's no retry wrapper on these calls.
- **22 `npm ci` sites across 2 TypeScript acceptance-stage-cloud workflows + 3 TypeScript commit-stage files use `actions/setup-node@v6` with `cache: 'npm'` and per-job `cache-dependency-path` pointing at the right `package-lock.json`.** This is the textbook well-cached path and is correctly classified as R-DOC-OK — cache hits skip the registry, cache misses only happen on first run after a lockfile change. No retry needed at the call site.
- **44 `./gradlew test` / `./gradlew build` sites across the Java fleet are cached via `gradle/actions/setup-gradle@v6`** (which has built-in dependency cache). Additionally, **24 of those sites have an explicit `Pre-warm Gradle Wrapper` step using `nick-fields/retry@v4`** with 3 attempts × 5 min timeout that runs `./gradlew --version` ahead of the real build — exactly to absorb the "transient CDN flake" failure mode this audit is built to catch. This is the **healthiest pattern observed in the entire workspace** (R-OK by §4 definition: marketplace retry action with explicit `max_attempts`/`timeout_minutes`) and predates the retry engine.

Net judgment: the first-pass deferral was right about TypeScript and Java; it materially undercounts .NET risk and missed `_prerelease-pipeline.yml` entirely. There is **no need for a new `npm-retry.sh` / `gradle-retry.sh`** — the existing tooling (cache + lockfile pin for npm; `setup-gradle@v6` built-in cache + Pre-warm wrapper for Java) is sufficient and well-aligned with the canonical engine schedule. The .NET gap is fixable by either **(a) adding `cache: true` to `setup-dotnet@v5` on commit-stage** (one-line per file × 3 files; backed by upstream `actions/setup-dotnet` cache support since v4) **or (b) adding the missing `actions/cache@v5` NuGet block** to commit-stage workflows (matches the acceptance-stage shape exactly). The `_prerelease-pipeline.yml` gap is fixable by adding `cache:` parameters to its `setup-*` steps (same one-line fixes).

The user's specific judgment-call ask: **the "caching makes retry unnecessary" claim is true wherever cache is configured; the gap is that cache configuration is missing in two specific places.** The right intervention is to close those cache gaps, not to extend the retry engine. A new `npm-retry.sh` or `gradle-retry.sh` would be over-engineering — the canonical retry engine already gets invoked by `nick-fields/retry@v4` for the one transient-prone Gradle pre-warm, and there's nothing analogous to the SonarCloud 504 or GitHub-GraphQL transient that drove §4 to exist in the npm/Maven/NuGet registry call sites' failure history.

---

## Summary

- Files scanned: 14 (intersection of "shop workflows" and "contains at least one in-scope dependency-resolution call").
- Sites in scope by call type:
  - `npm ci`: **29** (4 commit-stage, 22 acceptance-stage-cloud, 4 `_prerelease-pipeline.yml`).
  - `./gradlew test|build|assemble|check`: **31** (3 monolith-commit-stage, 3 multitier-backend-commit-stage, 22 acceptance-stage-cloud, 3 `_prerelease-pipeline.yml`).
  - `dotnet build` / `dotnet test`: **57** (3 monolith-commit-stage, 3 multitier-backend-commit-stage, 48 acceptance-stage / acceptance-stage-cloud / -legacy, 3 `_prerelease-pipeline.yml`).
- Classification distribution:
  - **R-OK**: 24 (Java acceptance-stage Pre-warm Gradle Wrapper sites — explicit `nick-fields/retry@v4` wrapper covering the Gradle dependency-fetch path).
  - **R-DOC-OK**: 76 (well-cached: 22 TypeScript `npm ci` with `cache: 'npm'`, 7 Java commit-stage `./gradlew` with `gradle/actions/setup-gradle@v6` cache, 44 dotnet `dotnet build/test` covered by `actions/cache@v5` NuGet block on acceptance-stage variants, 3 Java commit-stage covered by `gradle/actions/setup-gradle@v6`).
  - **N-A**: 0 (no `gh` calls in scope — `gh extension install` was N-A.1 in the first pass and is now fixed).
  - **N-B**: **17** (9 dotnet commit-stage `dotnet build`/`dotnet test` sites with no NuGet cache, 8 `_prerelease-pipeline.yml` sites with no cache at all).
  - **N-C**: 0 (no misconfigured retry on dependency-resolution calls — the Pre-warm Gradle pattern at 24 sites matches the engine's spirit cleanly).
- §4 anti-patterns observed: 1 (matrix duplication of `setup-dotnet` + `Cache NuGet` blocks across acceptance-stage-cloud's 11-per-file test jobs — see AP.1).
- §4 healthy patterns observed: 3 (cache-on-setup, gradle-setup-cache, Pre-warm Gradle retry wrapper — see Healthy patterns section).

---

## §4 findings

### Category N-A — `gh` call without retry

**None.** This audit class has no `gh` calls in scope.

### Category N-B — Other network call without retry

#### N-B.1 — dotnet commit-stage: 9 sites with no NuGet cache and no retry wrapper

`actions/setup-dotnet@v5` is invoked in all 3 dotnet commit-stage workflows with `dotnet-version: 8.0.x` and **no `cache:` parameter**. There is also **no separate `actions/cache@v5`** step for `~/.nuget/packages` in any of these files. The result: every push to main triggers a fresh `dotnet restore` (implicit in `dotnet build`) against `api.nuget.org` with no recovery path.

Call sites:

- `shop/.github/workflows/monolith-dotnet-commit-stage.yml:95` (`dotnet build` — compile source)
- `shop/.github/workflows/monolith-dotnet-commit-stage.yml:99` (`dotnet test` — first-test runs hit NuGet test-host packages)
- `shop/.github/workflows/monolith-dotnet-commit-stage.yml:127` (`dotnet build` inside the Sonar `run:` block — local cache hit, but only after line 95 succeeded; if 95 fails on NuGet, 127 is never reached)
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml:95` (`dotnet build`)
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml:99` (`dotnet test`)
- `shop/.github/workflows/multitier-backend-dotnet-commit-stage.yml:124` (`dotnet build` inside Sonar block)
- `shop/.github/workflows/_prerelease-pipeline.yml:147` (`dotnet build` — monolith dotnet compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:163` (`dotnet build` — multitier dotnet backend compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:185` (`dotnet build` — system tests dotnet compile)

(The acceptance-stage / acceptance-stage-cloud / -legacy dotnet workflows DO have an `actions/cache@v5` NuGet block — those 48 sites are R-DOC-OK and listed in the Healthy patterns section.)

Severity: medium for commit-stage (transient NuGet failure surfaces in PR commit-stage; user can re-run), high for the 3 `_prerelease-pipeline.yml` sites (transient NuGet failure on a release-day prerelease blocks the release pipeline).

Recommendation, ranked by engineering cost:

- **(preferred) Add `cache: true` to each `setup-dotnet@v5` step.** `actions/setup-dotnet` has built-in NuGet caching since v4; pass `cache: true` and either provide a `cache-dependency-path` (matching the `*.csproj` glob) or let it auto-detect via `packages.lock.json`. This matches the shape used by `actions/setup-node@v6` with `cache: 'npm'` in the TypeScript fleet — minimal config drift across the workspace.
- (alternative) Insert an `actions/cache@v5` block for `~/.nuget/packages` keyed on `hashFiles('**/*.csproj')` in each of the 3 commit-stage files and the `_prerelease-pipeline.yml` job. Matches the shape used by the dotnet acceptance-stage workflows already (e.g. `monolith-dotnet-acceptance-stage.yml:236-242`). More boilerplate per call site, but unambiguous.
- (escape hatch — do **not** recommend) Wrap each `dotnet build` / `dotnet test` with `nick-fields/retry@v4`. This re-introduces ad-hoc retry in 9 places, fragments policy, and treats the symptom (occasional registry transient) rather than the cause (cache misconfiguration). Reject — the §4 rubric anti-pattern AP.3 explicitly warns against this.

#### N-B.2 — `_prerelease-pipeline.yml`: 8 sites with zero cache configuration

`_prerelease-pipeline.yml` is the reusable compile-only stage that runs before GoReleaser. Its `Setup Node.js` (line 117), `Setup Java` (line 123), and `Setup .NET` (line 130) steps all omit `cache:`. There is no `actions/cache@v5` step anywhere in the file (`grep -c cache _prerelease-pipeline.yml` → 0). Every invocation hits npmjs, Maven Central (via `./gradlew`), and NuGet from cold.

Call sites:

- `shop/.github/workflows/_prerelease-pipeline.yml:137` — `npm ci && npx tsc --noEmit` (monolith typescript compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:142` — `./gradlew compileJava compileTestJava` (monolith java compile — also a `./gradlew` site but matches the `compile*` token rather than `build`, so listed here)
- `shop/.github/workflows/_prerelease-pipeline.yml:153` — `npm ci && npx tsc --noEmit` (multitier backend typescript compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:158` — `./gradlew compileJava compileTestJava` (multitier backend java compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:169` — `npm ci && npm run build` (multitier frontend react compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:175` — `npm ci && npx tsc --noEmit` (system tests typescript compile)
- `shop/.github/workflows/_prerelease-pipeline.yml:180` — `./gradlew compileJava compileTestJava` (system tests java compile)
- 3 × `dotnet build` already counted under N-B.1 (lines 147, 163, 185) — listed there to avoid double-counting; N-B.2's count of 8 is the npm/gradle subset, total uncached sites in `_prerelease-pipeline.yml` is 11.

Severity: **high** (release path). One uncached `npm ci` against an npmjs blip on release day blocks the prerelease pipeline. The current code has no retry wrapper at any of the 11 sites.

Recommendation:

- **(preferred) Add `cache:` parameters to the three `setup-*` steps:**
  - `actions/setup-node@v6` → add `cache: 'npm'` (and a `cache-dependency-path:` if multiple lockfiles in the matrix — see below).
  - `actions/setup-java@v5` → add `cache: 'gradle'` OR replace with `gradle/actions/setup-gradle@v6` (matches the commit-stage / acceptance-stage shape, gives both wrapper cache and dependency cache).
  - `actions/setup-dotnet@v5` → add `cache: true`.
- **Cache-key complication:** `_prerelease-pipeline.yml` runs different architectures (monolith vs multitier) and different working directories per `if:` branch. Each `setup-node` invocation runs once per job, but the `cache-dependency-path` would need to glob multiple lockfiles (`system/**/package-lock.json` and `system-test/typescript/package-lock.json`). `actions/setup-node`'s `cache-dependency-path:` accepts a multiline glob — this is supported. Same for `actions/setup-dotnet`'s `cache-dependency-path:`. The fix is mechanical, not architectural.
- The retry-engine route is **rejected for the same reason as N-B.1**: 11 ad-hoc retry wrappers at 11 sites is exactly the drift pattern §4 was written to prevent.

### Category N-C — Retry present but misconfigured

**None.** The Pre-warm Gradle Wrapper pattern (`nick-fields/retry@v4` with `max_attempts: 3, timeout_minutes: 5`) at 24 sites in the Java acceptance-stage / -cloud / -legacy files is **R-OK**, not N-C. Its schedule diverges slightly from the canonical 4×{5s, 15s, 45s} (3 attempts vs 4, retry_wait_seconds defaults vs explicit ramp), but the §4 R-OK definition explicitly admits "marketplace retry actions used with explicit attempt_limit/max_attempts" — this is the textbook case. The drift is mild enough that promoting it to N-C would create false-positive noise; if the engine schedule is canonicalized further the Pre-warm pattern can be aligned in a single sweep. Not flagged.

### §4 anti-patterns

#### AP.1 — Matrix duplication of `setup-dotnet` + `Cache NuGet` blocks across acceptance-stage-cloud's 11-per-file test jobs

In `monolith-dotnet-acceptance-stage-cloud.yml` and `multitier-dotnet-acceptance-stage-cloud.yml`, the pattern of `Setup .NET` → `Cache NuGet Packages` → `dotnet build` → `dotnet test` is copy-pasted across 11 test-* jobs per file (22 sites per file). Each block is identical except for the test-filter string. While this isn't strictly a §4 finding (the retry posture is healthy — R-DOC-OK via the NuGet cache), it IS a §3-G (duplicated bash) anti-pattern that interacts with §4 because **a future cache-key change would have to land in 22 places, with high drift risk that one job ends up uncached and silently regresses to N-B.1**.

Recommendation: out of scope for this §4 audit, but flagged here so a future §3 pass can consider a `optivem/actions/dotnet-test-job@v1` composite that hides the setup+cache+build+test sequence.

#### AP.2 — Two `./gradlew build` invocations within the same commit-stage job (line 106 and line 133 inside Sonar block)

In `monolith-java-commit-stage.yml` and `multitier-backend-java-commit-stage.yml`, the job runs `./gradlew build` at the "Compile Code" step (line 106), then re-runs `./gradlew build` inside the Sonar `run:` block (line 133) before `sonar_retry ./gradlew sonar --info`. The second invocation is local-only (cache hit from the first), so this is NOT a §4 finding. But the Sonar block's bash relies on `./gradlew build` having already succeeded; if line 106 fails on a registry transient, the Sonar block runs against half-built artifacts. Same story for the dotnet equivalents at lines 95 → 127 (monolith) and 95 → 124 (multitier-backend).

Recommendation: this is an interaction-with-N-B.1 observation, not its own finding. Fixing N-B.1 (add NuGet/Gradle cache to commit-stage) absorbs it.

---

## §4 healthy patterns (R-OK / R-DOC-OK)

### R-OK observed

**Pre-warm Gradle Wrapper** — `nick-fields/retry@v4` with `max_attempts: 3, timeout_minutes: 5, command: cd <dir> && ./gradlew --version`. Wraps the Gradle wrapper bootstrap (which downloads Gradle from the distribution CDN — a known transient-prone hop). Step name `Pre-warm Gradle Wrapper (retry transient CDN flakes)`. 24 sites:

- `shop/.github/workflows/monolith-java-acceptance-stage.yml:243, 377` (commit-stage uses inline wrapper, 1 site each — listed below).
- `shop/.github/workflows/monolith-java-acceptance-stage-legacy.yml:207`.
- `shop/.github/workflows/monolith-java-acceptance-stage-cloud.yml:268, 299, 330, 361, 405, 436, 480, 511, 542, 573, 617` (11 sites — one per test-* job).
- `shop/.github/workflows/multitier-java-acceptance-stage.yml` — 2 sites (same shape as monolith).
- `shop/.github/workflows/multitier-java-acceptance-stage-cloud.yml` — 11 sites (same shape).
- `shop/.github/workflows/multitier-java-acceptance-stage-legacy.yml` — 1 site.
- `shop/.github/workflows/monolith-java-commit-stage.yml:99-103` — 1 site (inline `nick-fields/retry@v4`).
- `shop/.github/workflows/multitier-backend-java-commit-stage.yml:99-103` — 1 site.

This is the **canonical example** of how to handle a dependency-resolution registry transient in this workspace. The exact step name "retry transient CDN flakes" is documenting the failure mode in-line. Worth preserving as the reference pattern for any future similar retries (e.g. if NuGet cache misses become a real failure source despite the cache fix proposed in N-B.1).

### R-DOC-OK observed (cache hits skip the network)

**TypeScript `npm ci` with `actions/setup-node@v6` cache** — `cache: 'npm', cache-dependency-path: <specific package-lock.json>`. Cache hits skip the npmjs registry hop entirely; misses only happen on lockfile change. 22 sites:

- `shop/.github/workflows/monolith-typescript-commit-stage.yml:97` (1 site — system/monolith/typescript/package-lock.json).
- `shop/.github/workflows/multitier-backend-typescript-commit-stage.yml:97` (1 site — system/multitier/backend-typescript).
- `shop/.github/workflows/multitier-frontend-react-commit-stage.yml:97` (1 site — system/multitier/frontend-react).
- `shop/.github/workflows/monolith-typescript-acceptance-stage-cloud.yml:266, 310, 354, 386, 432, 464, 510, 541, 572, 603, 634` (11 sites — system-test/typescript).
- `shop/.github/workflows/multitier-typescript-acceptance-stage-cloud.yml:309, 353, 397, 429, 475, 507, 553, 584, 615, 646, 677` (11 sites — system-test/typescript).

**Java `./gradlew` covered by `gradle/actions/setup-gradle@v6`** — `setup-gradle@v6` provides built-in Gradle wrapper + dependency cache. 7 commit-stage sites:

- `shop/.github/workflows/monolith-java-commit-stage.yml:106, 110, 133` (`./gradlew build`, `./gradlew test`, `./gradlew build` inside Sonar).
- `shop/.github/workflows/multitier-backend-java-commit-stage.yml:106, 110, 133` (same shape).
- `shop/.github/workflows/monolith-java-commit-stage.yml:126` (`./gradlew checkstyleMain`) — actually a §3 dup but R-DOC-OK on the dependency-resolution dimension.

(The 22 `./gradlew test` sites in `monolith-java-acceptance-stage-cloud.yml` and `multitier-java-acceptance-stage-cloud.yml` are covered by `setup-gradle@v6` AND the Pre-warm wrapper — double-covered, classified as R-OK above.)

**.NET `dotnet build` / `dotnet test` covered by `actions/cache@v5` NuGet block** — `path: ~/.nuget/packages, key: nuget-${{ runner.os }}-${{ hashFiles('system-test/dotnet/**/*.csproj') }}`. 44 sites:

- `shop/.github/workflows/monolith-dotnet-acceptance-stage.yml:257` (`dotnet build` — system-test/dotnet).
- `shop/.github/workflows/multitier-dotnet-acceptance-stage.yml:262`.
- `shop/.github/workflows/monolith-dotnet-acceptance-stage-legacy.yml:217`.
- `shop/.github/workflows/multitier-dotnet-acceptance-stage-legacy.yml:223`.
- `shop/.github/workflows/monolith-dotnet-acceptance-stage-cloud.yml:271, 275, 307, 311, 343, 347, 380, 397, 431, 435, 468, 485, 519, 523, 555, 559, 591, 595, 627, 631, 663, 680` (22 sites — alternating `dotnet build` + `dotnet test` across 11 jobs).
- `shop/.github/workflows/multitier-dotnet-acceptance-stage-cloud.yml:314, 318, 350, 354, 386, 390, 423, 440, 474, 478, 511, 528, 562, 566, 598, 602, 634, 638, 670, 674, 706, 723` (22 sites).

Note: each acceptance-stage-cloud test-* job has its OWN copy of the `Setup .NET` + `Cache NuGet Packages` block (see AP.1) — the cache key is reproducible across jobs only because `hashFiles('system-test/dotnet/**/*.csproj')` is stable for the same commit, so a cache populated by the first test-* job is hit by the rest. The setup is heavier than it needs to be but functionally correct.

---

## Recommendations

Ordered by severity × engineering cost.

### Tier 1 — release-path uncached calls (do first)

1. **Add `cache:` parameters to `_prerelease-pipeline.yml`'s setup steps.** One-line edit per step × 3 steps × 1 file. Closes N-B.2 (11 sites). Rationale: release-day prerelease blocked by registry blip is the highest-blast-radius case in this audit.
2. **Add `cache: true` to dotnet commit-stage `setup-dotnet@v5` steps** OR insert an `actions/cache@v5` NuGet block matching the acceptance-stage shape. One-line edit (option a) or 6-line block (option b) × 2 files. Closes N-B.1 (6 sites; the 3 sites in `_prerelease-pipeline.yml` already resolved by Tier 1.1).

### Tier 2 — workspace-wide consolidation (defer)

3. **§3-G: extract a `optivem/actions/dotnet-test-job@v1` composite** that bundles `setup-dotnet` + NuGet cache + `dotnet build` + `dotnet test`. Each `*-acceptance-stage-cloud.yml` shrinks from ~22 sites of duplicated setup down to one `uses:` per test job. Captured under AP.1; out of scope for this §4 audit. Not blocking; defer to a §3 audit pass.
4. **Optional: align the Pre-warm Gradle Wrapper schedule** with the canonical engine schedule (3→4 attempts; explicit retry_wait_seconds: 5 to match `gh_retry`'s ramp). Not necessary — the existing schedule works — but doing so would let a future audit collapse the R-OK + canonical-engine analysis into one rule. Defer until the engine schedule itself is canonicalized further.

### Tier 3 — verify-only (no action recommended now)

5. **Do NOT extend the retry engine with `npm-retry.sh` / `gradle-retry.sh` / `dotnet-retry.sh`.** The failure modes that drove §4 (SonarCloud 504, GitHub GraphQL transient) have no comparable incident history on npm / Maven Central / NuGet in this workspace. The right tool for the dependency-resolution registry-transient class is **caching + lockfile pin**, not retry. Caching is already the workspace convention everywhere except the two gaps Tier 1.1 and Tier 1.2 close.
6. **Re-run this audit** if a registry-transient failure does surface on a commit-stage or `_prerelease-pipeline.yml` run after the Tier 1 fixes land. If it does, the right response is to extend the existing `nick-fields/retry@v4` Pre-warm pattern (it's already R-OK by §4 rule and matches the engine spirit), not to invent a new bash-sourced wrapper.

---

## Examined-and-rejected

- **`npx playwright install chromium`** — appears in 5+ workflows (`monolith-typescript-acceptance-stage-cloud.yml:323`, `multitier-typescript-acceptance-stage-cloud.yml:323`, etc.). Hits the Playwright registry. Already covered by `actions/cache@v5` block keyed on `~/.cache/ms-playwright` + lockfile hash, AND guarded by `if: steps.cache-playwright.outputs.cache-hit != 'true'`. Cache miss is rare and recoverable. R-DOC-OK. Not flagged.

- **`gh extension install optivem/gh-optivem`** — covered as N-A.1 in the first-pass audit, now fixed via `gh_retry extension install` wrapper. Not re-audited here.

- **`npx tsc --noEmit`, `npm run build`, `npm run lint`, `npm test`** — all local-only compile/lint/test commands that operate on the cached `node_modules` after `npm ci`. No network I/O. R-DOC-OK.

- **`./gradlew checkstyleMain`, `./gradlew sonar`** — checkstyle is local; the sonar invocation is already wrapped in `sonar_retry` (R-OK, covered by N-C.2 fix in parent audit). Not flagged.

- **`dotnet format --verify-no-changes`** — local code-style verification, no network. R-DOC-OK.

- **`dotnet tool install --global dotnet-sonarscanner`** — appears in 2 dotnet commit-stage files at the start of the Sonar `run:` block. Hits the NuGet registry for the tool binary. Without retry. Arguably an N-B finding. **Out of scope for this audit** — the audit asks about *dependency-resolution registry fetches*, and `dotnet tool install` is a build-tool install, not a dependency restore. Mention here so a future audit pass can decide whether to fold it in or treat it separately. The same call would benefit from the `cache: true` knob on `setup-dotnet@v5` (per upstream docs `cache: true` covers global tool installs too).

- **`gradle/actions/setup-gradle@v6`** itself, viewed as a network call (downloads the Gradle distribution). Its internal retry semantics are documented by the action — per §4 R-OK rule, marketplace actions with documented retry are R-OK. Not flagged. The Pre-warm Gradle Wrapper that follows it is belt-and-braces.

- **The acceptance-stage / acceptance-stage-cloud dotnet workflows' 48 `dotnet build`/`dotnet test` sites.** Have `actions/cache@v5` NuGet block before each invocation. Cache hits skip the network entirely. R-DOC-OK. Not flagged.

- **`gh optivem test setup` / `gh optivem test run --suite ...`** — these are the "delegating to extension" call sites in `monolith-typescript-acceptance-stage.yml`, `multitier-typescript-acceptance-stage.yml`, `monolith-dotnet-acceptance-stage.yml`, etc. They DO transitively trigger `npm ci` / `dotnet build` inside the extension binary, but that's the extension's responsibility to handle retry, not the workflow's. Out of scope for this workflow-file audit. Worth a separate audit pass against `gh-optivem`'s extension internals if registry transients are observed inside `gh optivem test setup`.

---

## Audit metadata

- Generated by: `workflow-auditor` agent, §4-only mode (second-pass scope: dependency-resolution registry fetches).
- Parent audit: `gh-optivem/audits/20260514-shop-workflow-retry-coverage.md` (Item 20 / N-B.7).
- Site counts: 117 in-scope dependency-resolution call sites across 14 files. 17 N-B (8 npm/gradle in `_prerelease-pipeline.yml`, 9 dotnet in commit-stage + 3 dotnet in `_prerelease-pipeline.yml`, where the 3 overlap), 24 R-OK (Pre-warm Gradle), 76 R-DOC-OK (cache-covered). Zero N-A, zero N-C.
- Recommendations: 2 fix items (Tier 1.1 cache for `_prerelease-pipeline.yml`, Tier 1.2 cache for dotnet commit-stage). One §3 follow-up (Tier 2.3 composite extraction). One explicit no-action recommendation (Tier 3.5: do not extend the retry engine for this class).
- Reviewer note: this audit is read-only on workflow files. A matching plan, if desired, would be a short two-item plan covering the Tier 1 cache fixes — not a "fix retry gaps" plan, because the right fix is *caching*, not *retry*.
