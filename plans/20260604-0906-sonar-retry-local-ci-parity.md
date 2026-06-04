# Sonar retry: local/CI parity on one canonical mechanism

**Status:** Draft — decisions D1–D4 open, awaiting lock before execution
**Created:** 2026-06-04 09:06 CEDT

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
| **shop CI** (`*-commit-stage.yml`) | CLI scanner (`dotnet sonarscanner`, `./gradlew sonar`) | ✅ `optivem/actions/retry@v1` (canonical GHA action) |
| **shop local** (`system*/**/run-sonar.sh`) | **same** CLI scanner commands | ❌ none |
| **gh-optivem CI** (this repo, `gh-commit-stage.yml`) | `SonarSource/sonarqube-scan-action@v7` composite action | ❌ none — the failing surface |

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

## Decisions to lock (open)

- **D1 — 403 reclassification.** Split the binary-download / CDN 403 (transient,
  retryable) from the auth 403 (hard-fail). Options: (a) match on context
  (download URL / `binaries.sonarsource.com` in the line) → retry; (b) make 403
  retryable only for the *install/download* command, keep hard-fail for the
  *scan* command; (c) leave classification alone and rely on the action/CLI's own
  download retry. **Recommend (b)** — scope the leniency to the download step so
  an auth 403 on the scan still fails fast. Needs decision because it changes
  upstream `optivem/actions` policy shared by every consumer.
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
  Needs a look at how the scaffolder lays down `system*/**/run-sonar.sh` today.

## Items

### Item 0 — Unblock main (immediate, independent of the rest)
- [ ] Re-run the failed jobs: `gh run rerun 26935724762 --failed`. Proven
      transient; re-run is the correct response, not a workaround. Confirm green.

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
- [ ] **Release:** push `optivem/actions` to `main`; the `update-v1` workflow
      auto-retargets the floating `v1` tag, so shop CI (`uses:
      optivem/actions/retry@v1`) picks up the fix on its next run. No manual tag
      move needed. Until pushed, shop still runs the old behaviour.
- [ ] Re-run the two failed jobs once `v1` is updated (or immediately as a
      transient re-run): `gh run rerun 26937916287 --repo optivem/shop --failed`.

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

### Item 3 — gh-optivem CI: converge to CLI + canonical retry (per D2/D3)
- [ ] Replace the `Run Code Analysis` step in `.github/workflows/gh-commit-stage.yml`
      (`SonarSource/sonarqube-scan-action@v7`) with a CLI `sonar-scanner`
      invocation wrapped per D3, preserving the existing args
      (`projectKey=optivem_gh-optivem`, `organization=optivem`, sources/tests
      inclusions/exclusions). This also fixes the 403 surface, because the retry
      wrapper re-runs the whole command including the scanner download.
- [ ] Keep the `if: github.ref == 'refs/heads/main' && env.SONAR_TOKEN != ''`
      guard and `SONAR_TOKEN` env wiring.
- [ ] Trigger an acceptance run (or push to main) and confirm the Sonar step
      passes and that an induced transient retries (manual check / log inspection).

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
