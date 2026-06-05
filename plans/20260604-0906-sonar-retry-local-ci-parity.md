# Sonar retry: local/CI parity on one canonical mechanism

## TL;DR

**Why:** Sonar runs on three surfaces (shop CI, shop local, gh-optivem CI) but
retry is uneven — local `run-sonar.sh` has none, and transient `403`s (CDN
binary-download, `/analysis/jres`) intermittently fail commit stages.
**End result:** one canonical retry pattern (CLI scanner + canonical retry) across
all three surfaces, with the `403` classification splitting transient-download
(retryable) from auth (hard-fail), and local achieving CI-equal resilience.

**Status:** Nearly done — only two **deferred** items remain, both gated on the
Item 1 release. Done: Items 0, 1b, 2 (shop `ea1bc4da`), 1c (Gradle skip done;
dotnet/JS stagger **reconsidered → D5 (c): no cap** — rely on the Java-skip-reduced
burst + Item 1b retry), 3. **Item 1 code+test** (CDN-403 in force-retry regex,
49/49 green) committed in `optivem/actions` but **not yet released**. Decisions:
**D1 → (a)**; D2/D3 resolved-by-execution; **D4 dropped** (shop script-free);
**D5 → (c)** (no concurrency cap; revisit only on recurrence). **Open tail (both
deferred):** Item 1 release (gated — `v1` retarget hits every shop CI consumer) →
then Item 4 end-to-end verify (induced-transient retry on a real run).
**Created:** 2026-06-04 09:06 CEDT
**Updated:** 2026-06-05 (execute-plan) — confirmed Item 2 already landed in shop
`ea1bc4da`; **dropped the Item 1c dotnet/JS stagger** (D5 (a) → (c)) — the Java
skip already shrank the burst, so `max-parallel`'s wall-time cost isn't justified
against an already-mitigated flake; Item 4 static parity + inline docs confirmed,
end-to-end verify deferred behind Item 1's release.
2026-06-05 — refine-plan walk: D1/D4/D5 all resolved with rationale;
Items 1, 1c (dotnet/JS), 2 rewritten to match; Item 3's stray verify box folded
into Item 4. 2026-06-04 — Item 1c JRE-availability investigation; source-of-truth
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

1. **Local `run-sonar.sh` comments overclaim parity.** They imply CI runs the
   *identical* thing, but CI adds automatic retry and local does not.
   **Resolution (2026-06-05): this is acceptable, not a gap to close** — a manual
   run is attended, so a human re-run is the right recovery (see corrected
   decisive constraint). The honest fix is to *reword the comment*, not to add
   local retry (Items 2/D4 dropped).
2. **gh-optivem CI is the outlier form.** It uses the official composite action
   while every other surface uses the CLI scanner. The composite action is also
   the one with no canonical retry, and is what 403'd.

### The decisive constraint (CORRECTED 2026-06-05)

> The original framing here was wrong and drove a stale plan. It claimed the bash
> `sonar_retry` (`.github/scripts/sonar-retry.sh`) was the *only* retry spanning
> local and CI, and concluded we must vendor that wrapper into shop for local
> parity. Both premises are obsolete: (1) the per-tool `sonar-retry.sh` was
> **unified into a single domain-agnostic `retry.sh`** (`retry_run`); (2) shop is
> **deliberately script-free** — it consumes retry via `uses: optivem/actions/retry@v1`
> and vendors *no* retry helpers (decision recorded in `sync-shared.sh:16-18`).

**Retry is domain-agnostic and GHA-only by design.** It lives in `optivem/actions`,
is consumed in CI via `retry@v1`, and is vendored (as `retry.sh` + `retry-core.sh`)
only into **gh-optivem** (an internal tool), never into shop. On a student laptop
there is *no* GHA action — and that is fine: a manual `./run-sonar.sh` flake is
**attended**, so the natural recovery is a human re-run, not automated retry. The
unifying pattern is therefore **"CLI scanner + `retry@v1` in CI"**; local
`run-sonar.sh` is intentionally out of scope for retry. (This is why Items 2/D4 —
"vendor retry into shop for local parity" — were dropped; see Decisions.)

### Two facts that make the 403 work non-trivial

- **shop is intentionally script-free — not a gap to fill.** It has no
  `.github/scripts/` and `run-sonar.sh` sources nothing, *by design*. The 403
  reclassification (Item 1) therefore lands purely upstream in `optivem/actions`
  and reaches shop CI through `retry@v1`; nothing is vendored into shop.
- **The unified `retry.sh` classifies all `403/Forbidden` as hard-fail (never retry)
  by default.**
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

One canonical, **domain-agnostic** retry pattern — **CLI scanner wrapped in
`optivem/actions/retry@v1`** — across both **CI** Sonar surfaces (shop CI,
gh-optivem CI), with the 403 classification correctly distinguishing
transient-download/JRE from auth. Local `run-sonar.sh` is intentionally
retry-free (attended manual runs recover by human re-run); "parity" means the
*analysis* is the same, not that local replicates CI's automated retry.

## Decisions

- **D2 — Scope of this plan. RESOLVED → (b)** by execution. gh-optivem CI was
  converted (Item 3); shop CI left as-is (already canonical). Local-retry parity
  (Item 2) was subsequently **dropped** (2026-06-05) — shop is deliberately
  script-free; see corrected decisive constraint + D4.
- **D3 — gh-optivem CI mechanism. RESOLVED → (a)** by execution. `gh-commit-stage.yml`
  now wraps the `sonar-scanner-cli` docker command in `optivem/actions/retry@v1`
  (commit `0976ceb`), matching shop CI.

### Still to lock (open)

- **D1 — 403 reclassification. RESOLVED → (a)** (2026-06-05). Make the transient
  binary-download/CDN 403 retryable by **adding its host to Item 1b's existing
  force-retry override regex** (`_RETRY_FORCE_RETRY`), not by per-command scoping.

  *Why (a) over (b)/(c):* `403` is an overloaded code — auth 403 (bad token) is
  **deterministic** (must hard-fail) while CDN/JRE 403 is **transient** (must
  retry), so the status code can't classify; you need a second signal. (a) uses
  the **endpoint/host** (what failed); (b) uses the **command/phase** (which step
  failed). (a) wins because:
  1. The host is the signal that actually discriminates transient from auth.
  2. (b) **structurally cannot** express the real failures — `./gradlew sonar`
     does JRE-provisioning *and* scan in **one process invocation**, so there is
     no separate "download command" to scope; the transient 403 and a
     hypothetical auth 403 exit the same command. This is exactly why Item 1b had
     to match on `/analysis/jres` content, not a command. (b) would have failed
     the 1b case.
  3. (a) reuses 1b's proven, tested override tier → **one-line** change; (b) needs
     new command-context plumbing in `retry-core` for strictly less coverage.
  4. The default stays **fail-closed** (hard-fail all 403); the override is a
     narrow allowlist of *named transient endpoints*, so a genuine auth 403
     (carrying neither `binaries.sonarsource.com` nor `/analysis/jres`) never
     matches and still fails fast in ~0 retries.

  *Caveat (accepted):* the allowlist is coupled to the CDN host string — if
  SonarSource moves the binary host it stops matching, but that fails **safe**
  (reverts to today's hard-fail, never a wrong retry). The "purest" fix
  (classify inside the scanner's HTTP client) isn't available — the bash wrapper
  sits outside the scanner process and only sees stdout/stderr text.

  **Scope note:** moot for gh-optivem CI — Item 3 wraps the *entire* `docker run`
  (image pull + analysis) in `retry@v1`, so a pull/download 403 retries the whole
  command without touching the hard-fail list. D1 applies only to the shop CLI
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
- **D4 — Sync mechanism for bash wrappers into scaffolded repos. RESOLVED →
    DROPPED / MOOT** (2026-06-05, superseded). The question only existed to serve
  Item 2 (local retry in `run-sonar.sh`), which is itself dropped. **shop is
  deliberately script-free** — it consumes retry via `uses: optivem/actions/retry@v1`
  and vendors no retry helpers (`sync-shared.sh:16-18`; original decision
  `20260515-0723-shop-zero-retry-scripts`). Vendoring `retry.sh`/`retry-core.sh`
  into shop would reverse that architecture and re-duplicate domain-agnostic
  retry into a student-facing repo. My earlier "shop-copy + repo-relative"
  resolution was made without knowing shop was intentionally script-free — it is
  withdrawn. Nothing is copied into shop.

- **D5 — dotnet/JS `/analysis/jres` handling. RESOLVED → (c)** (2026-06-05; revised
  from an initial (a) during execution). **Leave dotnet/JS as-is — no concurrency
  cap — relying on the Java skip (which shrank the burst) + Item 1b retry as the
  backstop.** Skip-provisioning stays unsafe for dotnet/JS (no `setup-java` on CI,
  no JRE on local dev machines; they pass only *by* provisioning), so (b) is out.

  *Why (c), revised from (a):* the initial (a) chose a concurrency-stagger
  (`max-parallel`) because retry-alone had exhausted on `multitier-frontend-react`
  under a **7-wide** `/analysis/jres` burst. But the Java skip (Item 1c Gradle,
  shipped the same day) removes the 2 Java provisioners → the burst is now
  **5-wide**, and there is **no evidence** the reduced burst still defeats the
  4-attempt retry. Weighed against that, `max-parallel` is too blunt and too
  costly: it serializes the *entire* ~30-min commit/acceptance stages to space out
  a ~1-min Sonar call, tripling Phase 1b and doubling Phase 2 critical path on an
  hours-long release pipeline. The flake is transient and the pipeline is
  **attended** (3h-gated / weekly) → a recurrence is a re-run, the same recovery
  model the plan accepts for local `run-sonar.sh`. **Revisit:** add the cap (commit
  matrix first) only if `/analysis/jres` 403s actually recur on a real run — at
  which point there'd be evidence for the right `max-parallel` value. (b) remains
  rejected: it trades a transient flake for a permanent new local Java requirement
  on .NET/frontend devs.

## Items

### Item 0 — Unblock main (immediate, independent of the rest) — DONE
- [x] Re-run the failed jobs: `gh run rerun 26935724762 --failed`. Proven
      transient; re-run is the correct response, not a workaround. Subsequently
      made permanently moot by Item 3, which converted this surface off the
      composite action that 403'd.

### Item 1 — Upstream: 403 reclassification (per D1 → (a))
> Reuses Item 1b's force-retry override tier (already in `retry-core.sh`); this is
> an *extension of the override regex*, not new classification machinery.
> Code+test done 2026-06-05 (`actions/shared/retry.sh`, `_test-retry.sh`, 49/49
> green) — committed but **not yet released**; release is the gated tail below.
- [ ] ⏳ Deferred (gated — third-party-visible): Release + re-sync. Push
      `optivem/actions`, retarget the floating `v1` tag (this hits **every** shop
      CI consumer instantly — needs explicit go-ahead), then re-sync vendored
      copies into gh-optivem via `bash optivem/actions/scripts/sync-shared.sh`.
      Verify the regenerated `.github/scripts/retry.sh` blob updates and matches.

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

#### Gradle edit set — DONE 2026-06-05 (all in `shop/`)
> Appended `-Dsonar.scanner.skipJreProvisioning=true` to all 3 Java
> `run-sonar.sh` (monolith, multitier backend, system-test) and both Java
> commit-stage workflows (`monolith-java`, `multitier-backend-java`, line 139).
> Java acceptance-stage CI runs `bash ./run-sonar.sh` so it inherits the local
> edit. Item 1b retry remains the backstop for the *other* transient 403s.

#### dotnet / JS — no stagger (D5 reconsidered → (c)) — RESOLVED 2026-06-05
> D5's concurrency-stagger (a) was **reconsidered and dropped during execution**.
> The Java skip (Gradle, above) already shrank the `/analysis/jres` burst from
> 7-wide to 5-wide, and the only evidence that retry-alone is insufficient
> (frontend-react exhausting its 4 attempts in run `26937916287`) was under the
> *old 7-wide* burst — nothing shows the reduced 5-wide burst still trips the 403.
> Against that speculative, already-mitigated risk, `max-parallel` is a blunt,
> costly lever: it serializes the *entire* ~30-min commit/acceptance stages to
> space out a ~1-min Sonar call, tripling Phase 1b and doubling Phase 2 critical
> path. dotnet/JS stay on provisioning + Item 1b retry as the backstop; a
> recurrence on this *attended* release pipeline is a re-run. Revisit the cap
> (commit matrix first) only if `/analysis/jres` 403s actually recur on a real run.

### Item 2 — DONE 2026-06-05 (reword `run-sonar.sh` comments; local-retry vendoring dropped)
> The original "vendor bash retry into shop" premise was dropped (it reverses the
> deliberate **script-free shop** architecture; see D4 + the corrected decisive
> constraint). The honest replacement — reword the `run-sonar.sh` header comments
> so they no longer overclaim CI parity — was committed in shop `ea1bc4da`: all 10
> headers now read "auto-retried in CI via `optivem/actions` ... this script is for
> manual runs." Comment-only, no behavioural retry added locally.

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
- Real-run verification (Sonar step passes; induced transient actually retries)
  folded into Item 4's consolidated verification pass.

### Item 4 — Verify parity + document — ⏳ Deferred: blocked on Item 1 release
> Static parity + docs confirmed 2026-06-05. All three surfaces run the CLI
> scanner: shop CI (`*-commit-stage.yml` inline `dotnet sonarscanner` /
> `./gradlew sonar`, wrapped in `retry@v1`), gh-optivem CI (`sonar-scanner-cli`
> docker in `retry@v1`, `gh-commit-stage.yml`), local `run-sonar.sh`
> (intentionally retry-free, attended). The unified pattern is already documented
> inline at both CI surfaces — `gh-commit-stage.yml:37-43` names the canonical
> pattern and the composite it replaced, and the 10 `run-sonar.sh` headers note
> CI auto-retry; no drift-prone prose doc exists, so the doc sub-item needs
> nothing further.
- [ ] ⏳ Deferred (needs Item 1 released): on a real gh-optivem CI run, confirm
      the Sonar step passes and an induced transient (binary-download 403) retries
      via the Item 1 reclassification (log inspection) — proving the 403 split
      behaves end-to-end (auth 403 fails fast, download 403 retries).

## Out of scope / explicitly not doing

- Hand-editing the generated `.github/scripts/*-retry.sh` in this repo.
- Introducing `nick-fields/retry` or any non-`optivem/actions` retry mechanism —
  contrary to the one-canonical-mechanism goal (and it can't wrap a `uses:` step
  anyway).
- Caching the scanner binary to dodge the CDN as a *substitute* for retry (could
  be a later optimization, but it's not the consistency fix).
