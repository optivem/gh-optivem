# Sonar retry: local/CI parity on one canonical mechanism

**Status:** In progress — immediate flakes fixed (Items 1b, 3 done); parity work
(Items 1, 1c, 2, 4) still open. Item 1c investigated 2026-06-04: **Gradle skip
recommended now; dotnet/JS deferred to new decision D5** (concurrency-stagger
preferred over skip+Java). D2/D3 resolved-by-execution; D1/D4/D5 still open.
**Created:** 2026-06-04 09:06 CEDT
**Updated:** 2026-06-04 — Item 1c JRE-availability investigation; source-of-truth
corrected to `shop/`; Gradle-only recommendation + decision D5 recorded.

> Triggered by acceptance run `26935724762` (2026-06-04): the gh-optivem Commit
> Stage failed with `Action failed: Unexpected HTTP response: 403` while the
> `SonarSource/sonarqube-scan-action@v7` composite action downloaded
> `sonar-scanner-cli-8.0.1.6346-linux-x64.zip` from `binaries.sonarsource.com`.
> The **same commit** (`368f207`) passed the identical step on the push-triggered
> `gh-commit-stage` run 20 minutes earlier — a proven transient CDN blip, not a
> code or token fault. The flake is the prompt; the parity gap below is the work.

## Problem

Sonar runs on three surfaces across the academy, in two different invocation
forms, with retry on only one of them:

| Surface | Invocation | Retry today |
|---|---|---|
| **shop CI** (`*-commit-stage.yml`) | CLI scanner (`dotnet sonarscanner`, `./gradlew sonar`) | YES — `optivem/actions/retry@v1` (canonical GHA action) |
| **shop local** (`system*/**/run-sonar.sh`) | **same** CLI scanner commands | NONE |
| **gh-optivem CI** (this repo, `gh-commit-stage.yml`) | ~~composite action~~ → `sonar-scanner-cli` docker | YES — `optivem/actions/retry@v1` (converged, Item 3) |

Two gaps:

1. **Local has no retry.** The `run-sonar.sh` scripts even claim parity — "CI
   runs the same analysis … this script is for manual runs" — yet CI retries and
   local does not. They have drifted on resilience.
2. **gh-optivem CI is the outlier form.** It uses the official composite action
   while every other surface uses the CLI scanner. The composite action is also
   the one with no canonical retry, and is what 403'd.

### The decisive constraint

**Only the bash `sonar_retry` (`.github/scripts/sonar-retry.sh`, built on the
shared `retry-core.sh` engine) runs in both local and CI.** The official Sonar
action *and* `optivem/actions/retry@v1` are **GHA-only** — neither exists when a
student runs `./run-sonar.sh` on their laptop. So local parity is not achievable
with either GHA mechanism; the bash wrapper is the *only* canonical retry that
spans both environments. That makes "CLI scanner + canonical retry" the single
pattern that unifies all three surfaces, and reframes the gh-optivem composite
action as the outlier to converge — not a maintained baseline to protect.

### Two facts that make this non-trivial

- **The scaffolded repos have no `.github/scripts/`.** `sonar-retry.sh` /
  `retry-core.sh` are not synced into shop today, so `run-sonar.sh` has nothing
  to `source`. Local retry requires *first* getting the bash wrappers into
  scaffolded repos.
- **`sonar-retry.sh` classifies all `403/Forbidden` as hard-fail (never retry).**
  Correct for a scanner→SonarCloud auth 403 (bad token), but it would *also*
  refuse to retry the transient CDN/binary-download 403 that caused this run.
  These two 403s are semantically opposite and currently conflated.
- **A *third* transient 403 exists, and it manifests differently per tool.**
  Acceptance run `26937916287` (2026-06-04) failed `multitier-backend-java` and
  `multitier-frontend-react` on a **JRE-provisioning 403** from
  `api.sonarcloud.io/analysis/jres` (distinct from the binary-download CDN 403
  above). The decisive twist: the **Gradle Sonar plugin** prints it as the
  literal `failed with HTTP 403 Forbidden`, which matches the unified hard-fail
  list (`HTTP 4[0-9][0-9]` + `Forbidden`) and fails fast with **zero retries**;
  the **JS bootstrapper** prints the *same* failure as `Bootstrapper: An error
  occurred ... status code 403` (no `HTTP 4xx` token, no `Forbidden` word),
  which already lands in transient and retried 4×. So D1's download-step scoping
  would not have caught the Gradle case — see Item 1b (done).

### Governance (do not violate)

- `.github/scripts/*-retry.sh` and `retry-core.sh` are **GENERATED — DO NOT
  EDIT**, synced from `optivem/actions`. Any policy change (e.g. the 403
  reclassification) lands **upstream in `optivem/actions`** and is re-synced —
  never hand-edited in the generated copies.
- `run-sonar.sh` and the scaffolded workflows are produced by the scaffolder /
  shop template; change them at their source of truth, not in generated output.

## Goal

One canonical retry pattern — **CLI scanner wrapped in canonical retry** — across
all three Sonar surfaces, with local and CI achieving equal resilience, and the
403 classification correctly distinguishing transient-download from auth.

## Decisions

- **D2 — Scope of this plan. RESOLVED → (b)** by execution. gh-optivem CI was
  converted (Item 3); shop CI left as-is (already canonical). Local parity
  (Item 2) remains the open tail of this scope.
- **D3 — gh-optivem CI mechanism. RESOLVED → (a)** by execution. `gh-commit-stage.yml`
  now wraps the `sonar-scanner-cli` docker command in `optivem/actions/retry@v1`
  (commit `0976ceb`), matching shop CI.

### Still to lock (open)

- **D1 — 403 reclassification.** Split the binary-download / CDN 403 (transient,
  retryable) from the auth 403 (hard-fail). Options: (a) match on context
  (download URL / `binaries.sonarsource.com` in the line) → retry; (b) make 403
  retryable only for the *install/download* command, keep hard-fail for the
  *scan* command; (c) leave classification alone and rely on the action/CLI's own
  download retry. **Recommend (b)** — scope the leniency to the download step so
  an auth 403 on the scan still fails fast. Needs decision because it changes
  upstream `optivem/actions` policy shared by every consumer. **Note:** now moot
  for gh-optivem CI — Item 3 wraps the *entire* `docker run` (image pull +
  analysis) in `retry@v1`, so a download/pull 403 retries the whole command
  without touching the hard-fail list. D1 remains relevant only for the shop CLI
  surface, where bash `sonar-retry.sh` still hard-fails all 403 (`sonar-retry.sh:43`).
- **D2 — Scope of this plan.** (a) Local-only (`run-sonar.sh`) parity; (b) local
  + gh-optivem CI conversion; (c) all three incl. harmonizing shop CI `run:` so
  local and CI are byte-identical. **Recommend (b)** — fixes the failing surface
  and the no-coverage surface; leaves shop CI as-is (already canonical via
  `optivem/actions/retry@v1`).
- **D3 — gh-optivem CI mechanism.** When converting `gh-commit-stage.yml` off the
  composite action: wrap the CLI scanner in (a) `optivem/actions/retry@v1` (match
  shop CI exactly) or (b) bash `sonar_retry` in a `run:` step. **Recommend (a)**
  for consistency with shop CI; gh-optivem has no local `run-sonar.sh`, so the
  bash-spans-both argument doesn't apply to this repo.
- **D4 — Sync mechanism for bash wrappers into scaffolded repos.** How
  `sonar-retry.sh`+`retry-core.sh` reach shop: scaffolder copies them into
  `.github/scripts/`, vs. `run-sonar.sh` sources them from an installed location.
  **Partial answer (2026-06-04):** the scaffolder copies files verbatim *from
  `shop/`* into student repos (`apply_template.go`, `cfg.ShopPath`). So the path
  of least resistance is: add `.github/scripts/{retry-core,sonar-retry}.sh` to
  the **`shop/` repo** and have the scaffolder copy that dir alongside the rest —
  then `run-sonar.sh` can `source` a path that exists in both shop and every
  scaffolded repo. Still open: confirm the scaffolder's file-copy globs include
  `.github/scripts/`, and pick source-relative vs. installed-location sourcing.

- **D5 — dotnet/JS `/analysis/jres` handling (NEW, from Item 1c investigation).**
  Skip-provisioning is unsafe for dotnet/JS — they have no JRE on their CI
  runners (no `setup-java`) or on local dev machines, and pass today only *by*
  provisioning. Options: (a) **stagger the meta-prerelease fan-out / cap Sonar
  concurrency** so `/analysis/jres` isn't hammered by 7 simultaneous pipelines —
  fixes all toolchains, adds no dependency; (b) add `setup-java` to the 6
  dotnet/JS workflows + set skip + document a new **local** Java requirement for
  .NET/frontend devs; (c) leave as-is on the Item 1b retry backstop.
  **Recommend (a)** — least invasive, no new local Java dependency, and it
  addresses the actual root cause (concurrency) rather than working around it.

## Items

### Item 0 — Unblock main (immediate, independent of the rest) — DONE
- [x] Re-run the failed jobs: `gh run rerun 26935724762 --failed`. Proven
      transient; re-run is the correct response, not a workaround. Subsequently
      made permanently moot by Item 3, which converted this surface off the
      composite action that 403'd.

### Item 1 — Upstream: 403 reclassification (per D1)
- [ ] In `optivem/actions`, adjust `sonar-retry.sh` (and/or `retry-core` policy)
      so a transient download/CDN 403 is retryable while auth 403 stays hard-fail.
- [ ] Add/extend the upstream retry test harness to cover both 403 shapes.
- [ ] Bump the source SHA; re-sync into gh-optivem (and any other consumer) via
      `bash optivem/actions/scripts/sync-shared.sh`. Verify the regenerated
      `.github/scripts/sonar-retry.sh` header SHA updates and content matches.

### Item 1b — Upstream: JRE-provisioning 403 force-retry override (DONE)
> Done 2026-06-04, triggered by acceptance run `26937916287`. Lands in
> `optivem/actions` (canonical), re-synced to gh-optivem. Independent of D1 —
> this is the `analysis/jres` 403, not the `binaries.sonarsource.com` 403.
- [x] Add a **force-retry override** tier to `retry-core.sh`, checked *before*
      hard-fail (an optional `_RETRY_CORE_FORCE_RETRY` regex). When matched, the
      failure is routed to the retry path even if it also matches the hard-fail
      list. Backward-compatible: empty by default, function signature unchanged,
      all pre-existing tests pass.
- [x] Define the override in `retry.sh` as
      `_RETRY_FORCE_RETRY='Failed to query JRE metadata|/analysis/jres'` and
      wire it via `_RETRY_CORE_FORCE_RETRY` in `retry_run`. Narrow by design:
      scoped to the JRE-provisioning endpoint, so a genuine auth 403 on the
      analysis submission still fails fast. Worst case for a truly-bad token is
      exhausting ~65s of retries before the same non-zero exit.
- [x] Tests: `_test-retry-core.sh` (override wins over hard-fail / unset still
      hard-fails / override exhausts) and `_test-retry.sh` (real Gradle
      JRE-metadata 403 retries; a non-JRE `HTTP 403 Forbidden` still hard-fails).
      25/25 core + 45/45 wrapper green.
- [x] Re-synced vendored copies into gh-optivem via `sync-shared.sh` (retry-core
      blob `b746f07`, retry blob `40dd2a5`).
- [x] **Released:** pushed `optivem/actions` `055d586` to `main`; `update-v1`
      run `26940130663` retargeted the floating `v1` tag to `055d586`. shop CI
      (`uses: optivem/actions/retry@v1`) now resolves to the fix.
- [x] Re-ran the two downstream commit-stage runs on the fixed `v1` (the meta
      run `26937916287` itself is a concluded scheduled orchestrator and cannot
      be retried). `multitier-backend-java` (`26937963251`) → **green** — the
      previously-broken Gradle path. `multitier-frontend-react` (`26937963918`)
      → green on a fresh re-run after one more sustained-403 exhaustion (see
      Item 1c).

### Item 1c — Eliminate the `/analysis/jres` flake at its source (skip JRE provisioning)
> Surfaced while verifying Item 1b: retry is necessary but not sufficient.
> `multitier-frontend-react` failed its rerun even with a *correctly working*
> 4-attempt retry — the SonarCloud JRE-provisioning endpoint
> `api.sonarcloud.io/analysis/jres` 403s in **sustained** bursts that outlast
> the 5s+15s+45s (~65s) window. Root cause is concurrency: the meta-prerelease
> fans out all 7 language pipelines simultaneously, and they hammer
> `/analysis/jres` at once, so it rate-limits/403s. Retrying around a saturated
> endpoint is a band-aid; the durable fix is to **stop calling it**.

The scanner only hits `/analysis/jres` to download a JRE to run the analyzer
(SonarScanner is a JVM program). `skipJreProvisioning=true` removes that call —
**but only works if a JRE is already present**, since the analyzer still needs a
JVM. That precondition is the whole story (see investigation below).

#### Investigation (2026-06-04) — findings that reshape this item

1. **Source of truth = the `shop/` repo, not gh-optivem.** The scaffolder copies
   *from* `shop/` into student repos (`internal/steps/apply_template.go`:
   `cfg.ShopPath`, `../../../system/`). The "generated output" is the student
   repos (e.g. `page-turner-30`), **not** shop. The earlier wording ("don't
   hand-edit run-sonar.sh in the shop repo / regenerate shop") was wrong — edit
   `shop/system*/**/run-sonar.sh` and `shop/.github/workflows/*.yml` directly;
   there is nothing to "regenerate" in gh-optivem.

2. **Only the Gradle/Java pipelines have a JRE — so only they can safely skip.**
   Audited every Sonar-running workflow + every `run-sonar.sh`:

   | Toolchain | `setup-java` in CI? | Local dev has Java? | Safe to skip? |
   |---|---|---|---|
   | **Gradle/Java** | YES (temurin) | YES (it's a Java build) | **YES** |
   | **dotnet** | NONE | NO (scanner provisions its own JRE today) | NO |
   | **JS (ts/react)** | NONE (`setup-node` ≠ JRE) | NO (frontend devs have no Java) | NO |

   dotnet/JS pass today **precisely because they provision** the JRE we'd be
   skipping. Forcing skip there would break them unless we *also* add a JRE.
   The original premise — "the runner already has a JDK/JRE (Setup Java / Setup
   Node)" — holds **only for Java**; Setup Node provides no JVM.

3. **Third surface (gh-optivem CI) is unaffected.** It runs the
   `sonarsource/sonar-scanner-cli` docker image, which **bundles its own JRE** →
   never calls `/analysis/jres`, and the whole `docker run` is already
   retry-wrapped (Item 3). Out of scope for the three toolchains here.

#### Recommendation — Gradle now; concurrency-stagger (not skip) for dotnet/JS

**Do the Gradle skip (clean win), leave dotnet/JS on provisioning.** Rationale:

- **Gradle skip is a pure win:** Java already has a JRE everywhere, so the flag
  costs nothing and removes the flaky call on the exact surface that broke
  (`multitier-backend-java` run `26937916287`).
- **dotnet/JS skip is a bad trade, not a win:** a .NET/frontend dev running
  `run-sonar.sh` needs **no Java today**. Skip would trade a *transient,
  already-retried* flake for a **permanent new local Java requirement** — which
  undercuts this plan's own local-parity/UX goal. They already have Item 1b's
  `/analysis/jres` force-retry as a backstop.
- **The better dotnet/JS lever is concurrency-staggering, not skip+Java.** The
  root cause is 7 pipelines hammering `/analysis/jres` at once. Staggering the
  meta-prerelease fan-out (or capping Sonar concurrency) fixes it for *all*
  toolchains **without** forcing Java onto anyone — strictly less invasive than
  adding `setup-java` to 6 workflows. Captured as decision **D5** below.

#### Gradle edit set (all in `shop/`, append `-Dsonar.scanner.skipJreProvisioning=true`)
- [ ] **Local `run-sonar.sh` (×3):** `system/monolith/java`,
      `system/multitier/backend-java`, `system-test/java`.
- [ ] **CI commit-stage (×2):** `monolith-java-commit-stage.yml:139` and
      `multitier-backend-java-commit-stage.yml:139` (the `./gradlew sonar --info`
      line). Java **acceptance-stage** CI runs `bash ./run-sonar.sh`, so it
      inherits the local edit — no separate change.
- [ ] Keep Item 1b retry as the backstop for the *other* transient 403s
      (binary-download CDN, genuine 5xx) and for dotnet/JS `/analysis/jres`.

#### dotnet / JS — deferred pending D5 (do NOT naked-skip)
- [ ] Decide D5: concurrency-stagger (recommended) vs. add `setup-java` + skip +
      local Java requirement. Until then dotnet/JS stay on provisioning + Item 1b
      retry — no regression.

### Item 2 — Local parity: retry in `run-sonar.sh` (the gap with zero coverage)
- [ ] Get `sonar-retry.sh` + `retry-core.sh` into scaffolded repos (per D4) —
      they have no `.github/scripts/` today.
- [ ] Wrap the scanner calls in every `run-sonar.sh` (monolith dotnet/java/ts,
      multitier backends + frontend, system-test dotnet/java/ts) with
      `sonar_retry`: `source …/sonar-retry.sh` then
      `sonar_retry dotnet sonarscanner end …` / `sonar_retry ./gradlew … sonar …`.
- [ ] Decide whether the flaky `dotnet tool install --global dotnet-sonarscanner`
      (the local analogue of today's download 403) is also wrapped — it should be,
      since it hits an external registry.
- [ ] Make the edits at the scaffolder/template source of truth, then regenerate
      shop. Do not hand-edit generated `run-sonar.sh` in the shop repo.

### Item 3 — gh-optivem CI: converge to CLI + canonical retry (per D2/D3) — DONE
> Done 2026-06-04, commit `0976ceb`. Resolved D2→(b) and D3→(a).
- [x] Replaced the `Run Code Analysis` step in `.github/workflows/gh-commit-stage.yml`
      (`SonarSource/sonarqube-scan-action@v7`) with a `sonarsource/sonar-scanner-cli`
      `docker run` wrapped in `optivem/actions/retry@v1`, preserving the existing
      args (`projectKey=optivem_gh-optivem`, `organization=optivem`, sources/tests
      inclusions/exclusions). The whole `docker run` (image pull + analysis) sits
      inside the retry envelope, so a transient pull/download 403 retries the
      entire command — this is also why D1 is now moot for this surface.
- [x] Kept the `if: github.ref == 'refs/heads/main' && env.SONAR_TOKEN != ''`
      guard and `SONAR_TOKEN` env wiring (`gh-commit-stage.yml:45`).
- [ ] Still to verify: confirm on a real run that the Sonar step passes and that
      an induced transient actually retries (log inspection).

### Item 4 — Verify parity + document
- [ ] Confirm all three surfaces now run the CLI scanner through a canonical
      retry, and that the 403 split behaves (auth 403 fails fast, download 403
      retries) in both local and CI.
- [ ] Note the unified pattern wherever the Sonar setup is documented so it
      doesn't drift back to the composite action.

## Out of scope / explicitly not doing

- Hand-editing the generated `.github/scripts/*-retry.sh` in this repo.
- Introducing `nick-fields/retry` or any non-`optivem/actions` retry mechanism —
  contrary to the one-canonical-mechanism goal (and it can't wrap a `uses:` step
  anyway).
- Caching the scanner binary to dodge the CDN as a *substitute* for retry (could
  be a later optimization, but it's not the consistency fix).
