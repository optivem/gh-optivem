# 2026-08-11 22:46:00 CEST — Pin the Playwright browser install in shop's legacy acceptance-stage workflows

## TL;DR

**Why:** The `Install Playwright Browser` step in shop's four `*-acceptance-stage-legacy.yml` workflows runs `npx playwright install chromium` *before* anything has installed the project's dependencies. With no `node_modules`, npx resolves `playwright` from the npm registry at **latest** instead of the pinned version, so every fresh scaffolded app downloads whatever chromium build npm published that day. On 2026-08-11 that drifted build (chromium v1234 / 151.0.7922.34) was 403'd by the Playwright CDN for the runner's region and took down a gh-optivem acceptance-stage matrix cell.
**End result:** No legacy workflow installs an unpinned Playwright browser. The TypeScript variants drop the redundant step entirely and let `gh optivem system-test setup` do it at the pinned version; the Java variants keep the step but pin it to the Playwright version `build.gradle` declares and wrap it in the repo's existing CDN-flake retry.

## Outcomes

What we get out of this — the goals and deliverables:

- The scaffolded-app `acceptance-stage-legacy` run no longer downloads a browser build that nothing in the repo pins, so a same-day Playwright release can't break a rehearsal.
- The two TypeScript legacy workflows have the same Playwright shape as the already-correct latest-cycle `*-acceptance-stage.yml` workflows — one less divergence between legacy and latest.
- One fewer CDN round-trip per TypeScript legacy run (the redundant download that failed is simply gone), shortening the job and shrinking the transient-failure surface.
- The two Java legacy workflows install a browser matching `com.microsoft.playwright:playwright:1.44.0`, and survive a transient CDN 403 via `nick-fields/retry@v4` — the same treatment the Gradle wrapper pre-warm step in those files already gets.
- The gh-optivem `gh-acceptance-stage` matrix cell `multitier / monorepo / java / typescript` gets past `Install Playwright Browser` into the `mod02`…`mod11` suites.

## Context — the failure

- gh-optivem run [31528223285](https://github.com/optivem/gh-optivem/actions/runs/31528223285), job `93913041256`, matrix cell `Run (multitier, monorepo, java, typescript)`.
- Go-level symptom: `config_system_test.go:292: expected exit code 0, got 1` → `TestValidMultitierConfigurations/multitier_monorepo_java_ts_typescript` FAILED after 1129s.
- Real failure, one level down: gh-optivem Phase 8/11 *Verify acceptance stage* watched the scaffolded app's `acceptance-stage-legacy` run `31532578870` (repo `valentinajemuovic/test-app-1f1bb631-c3d658c8be4f3e16`, job `93916043330`) fail at step **Install Playwright Browser**:

  ```
  npm warn exec The following package was not found and will be installed: playwright@1.62.1
  WARNING: It looks like you are running 'npx playwright install' without first
           installing your project's dependencies.
  Downloading Chrome for Testing 151.0.7922.34 (playwright chromium v1234)
    from https://cdn.playwright.dev/builds/cft/151.0.7922.34/linux64/chrome-linux64.zip
  Error: Download failed: server returned code 403 body
    '<Error><Code>AccessDenied</Code>…<Details>We're sorry, but this service is not
     available in your location</Details></Error>'
  (5 retries, all 403) → Failed to install browsers → exit code 1
  ```

**Root cause, pinned:** `shop/.github/workflows/multitier-typescript-acceptance-stage-legacy.yml:203-206`. The step runs in `system-test/typescript` but sits *above* `Setup Test Harness` (line 213), which is what runs `npm ci`. No `node_modules` ⇒ npx pulls `playwright@1.62.1` from the registry rather than the pinned `playwright ^1.55.0` (`shop/system-test/typescript/package.json:32`).

**Why the drift is causal, not incidental:** the gh-optivem smoke jobs eight minutes earlier — using project-pinned deps — pulled chromium **v1217 / 147.0.7727.15** from the *same CDN host* with no trouble. Only the drifted-to-latest build was denied. This is not a blanket CDN outage.

**Not a gh-optivem defect.** gh-optivem copies the shop workflow verbatim: `internal/scaffolding/steps/apply_template.go:185-190` (`addLegacyWorkflow`), `internal/scaffolding/steps/names.go:98,106,115` (`monolith-${testLang}-acceptance-stage-legacy.yml` / `multitier-${testLang}-acceptance-stage-legacy.yml` → `acceptance-stage-legacy.yml`), copied by `templates.CopyWorkflows(…, shop, repoDir)` at `internal/scaffolding/templates/templates.go:53`. **No gh-optivem code change is needed** — all edits land in `shop`.

### Scope — every `*acceptance-stage*.yml` in `shop/.github/workflows` was enumerated

| Variant | Playwright shape | Verdict |
|---|---|---|
| `*-legacy.yml` (4 files, below) | install step **before** any `npm ci` | **defective — fix** |
| `*-cloud.yml` | explicit `npm ci` step precedes the install (e.g. `monolith-typescript-acceptance-stage-cloud.yml:262-277`) | correct — **do not touch** |
| `*-acceptance-stage.yml` (latest cycle) | no standalone install step; keeps only `Cache Playwright Browsers` (`id: cache-playwright`) and lets `Setup Test Harness` do it (`multitier-typescript-acceptance-stage.yml:236-251`) | correct — **reference shape** |
| `*-dotnet-*-legacy.yml` | no Playwright step at all | unaffected |

The four defective files (paths relative to the academy workspace root — the executor must `cd` into `shop`):

1. `shop/.github/workflows/multitier-typescript-acceptance-stage-legacy.yml:203-206` ← the cell that failed
2. `shop/.github/workflows/monolith-typescript-acceptance-stage-legacy.yml:197-200`
3. `shop/.github/workflows/multitier-java-acceptance-stage-legacy.yml:212-215`
4. `shop/.github/workflows/monolith-java-acceptance-stage-legacy.yml:206-209`

### Considered and not chosen

- **Retry-only on the TypeScript step.** Wrapping the failing step in `nick-fields/retry@v4` would have retried the *wrong* browser download. It treats the symptom and leaves the drift in place.
- **Gradle-native CLI invocation for the Java variants.** `shop/system-test/java/build.gradle:74` already declares a `com.microsoft.playwright.CLI` mainClass, so `./gradlew <task> --args="install chromium"` would be the more principled install path and would inherit the version from the dependency graph automatically. It's a larger change (new/renamed Gradle task, Windows/Linux wrapper differences) than this fix warrants; pinned-npx + retry is the minimal correct step. Worth revisiting separately.

## ▶ Next executable step (resume here)

`cd` into the `shop` repo (sibling of `gh-optivem` in the academy workspace) and apply Step 1: delete the `Install Playwright Browser` step from the two TypeScript legacy workflows — `.github/workflows/multitier-typescript-acceptance-stage-legacy.yml:203-206` and `.github/workflows/monolith-typescript-acceptance-stage-legacy.yml:197-200`. Delete the whole 4-line step block (`- name:` / `if:` / `working-directory:` / `run:`) and nothing else — in particular **keep** the `Cache Playwright Browsers` step above it, `id: cache-playwright` included, even though its `if:` consumer is going away (the latest-cycle workflows keep the same unused id, and the cache is what makes the harness's own install a no-op on a hit). Stop after both files are edited and re-read to confirm the `Setup Test Harness` step now directly follows `Install gh-optivem CLI extension`. This unblocks Step 2 (the Java variants), which is independent and can follow immediately.

## Steps

- [ ] **Step 1 — Delete the redundant install step from the TypeScript legacy workflows.**
  Files: `shop/.github/workflows/multitier-typescript-acceptance-stage-legacy.yml:203-206`, `shop/.github/workflows/monolith-typescript-acceptance-stage-legacy.yml:197-200`.
  Remove the entire `Install Playwright Browser` step. It is redundant: `shop/system-test/typescript/tests.legacy.yaml:13-17` already declares `setupCommands` = `npm ci --fetch-retries=5 …` followed by `npx playwright install chromium`, and `Setup Test Harness` (`gh optivem system-test setup`) runs both in the correct order at the pinned version.
  **Keep** the `Cache Playwright Browsers` step and its `id: cache-playwright` — consistent with the latest-cycle workflows, which carry the same now-unconsumed id.

- [ ] **Step 2 — Pin and retry the install step in the Java legacy workflows.**
  Files: `shop/.github/workflows/multitier-java-acceptance-stage-legacy.yml:212-215`, `shop/.github/workflows/monolith-java-acceptance-stage-legacy.yml:206-209`.
  This step *cannot* be deleted here: `system-test/java` has no `package.json`, and `shop/system-test/java/tests.legacy.yaml:13-15` `setupCommands` only runs Gradle (`.\gradlew.bat clean compileJava compileTestJava`) — so the workflow step is the only place browsers get installed. Instead:
  - (i) Pin the CLI: `npx playwright@1.44.0 install chromium`, matching `com.microsoft.playwright:playwright:1.44.0` at `shop/system-test/java/build.gradle:44`.
  - (ii) Wrap it in `nick-fields/retry@v4` with `timeout_minutes: 10`, `max_attempts: 3`, mirroring the *Pre-warm Gradle Wrapper (retry transient CDN flakes)* step that already sits directly above it (`multitier-java-acceptance-stage-legacy.yml:198-204`). Preserve the existing `if: steps.cache-playwright.outputs.cache-hit != 'true'` guard and the `working-directory: system-test/java`.

- [ ] **Step 3 — Comment the duplicated version pin.**
  The `1.44.0` in Step 2 now exists in both `build.gradle` and the two Java workflows. Add a one-line YAML comment on each pinned step naming `system-test/java/build.gradle:44` as the source of truth, so a future Playwright bump updates both.

- [ ] **Step 4 — Lint the four edited workflows.**
  Run `actionlint` in `shop` if available; otherwise parse each edited YAML to confirm it's still well-formed and the step sequence is intact.

- [ ] **Step 5 — Confirm the blast radius.**
  `git diff --stat` in `shop` must show exactly the four `*-acceptance-stage-legacy.yml` files and nothing else. No `-cloud.yml`, no latest-cycle `*-acceptance-stage.yml`, no `gh-optivem` file.

## Verification

- Re-run the gh-optivem `gh-acceptance-stage` workflow and confirm the previously-failing matrix cell `multitier / monorepo / java / typescript` gets past the scaffolded app's `Install Playwright Browser` / `Setup Test Harness` steps and into the `mod02`…`mod11` suites.
- Confirm at least one matrix cell whose `testLang` is `java` (exercising Step 2) also passes its scaffolded `acceptance-stage-legacy` run.
