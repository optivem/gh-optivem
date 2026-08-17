# 2026-08-17 08:24:41 UTC — Fix scaffolded multitier Java backend's external-simulator docker context

## TL;DR

**Why:** The scaffolded multitier Java backend's commit stage fails at `Run External System Contract Tests (Real ERP)` with `unable to prepare context: path "external-systems/simulators" not found`. Shop's `externalSimulatorImage` Gradle task hardcodes shop's monorepo depth (`file("${rootDir}/../../..")`), and the scaffolder neither rewrites that depth for the flattened `<repo>/backend` layout nor copies `external-systems/` into the split backend repo.
**End result:** Both scaffolded multitier layouts (monorepo and multirepo) build the external-system simulator image successfully, so the `external-contract-real` suite passes in the scaffolded backend commit stage — and a scaffolding test pins both halves so the regression cannot come back silently.

## Outcomes

What we get out of this — the goals and deliverables:

- `gh optivem` acceptance stage passes for `multitier / multirepo / java` — the case that failed in run [31865352299](https://github.com/optivem/gh-optivem/actions/runs/31865352299/attempts/1).
- `multitier / monorepo / java` (the same latent bug, simply absent from that run's smoke matrix) is fixed in the same change rather than surfacing as the next red run.
- The split multirepo backend repo carries the external-system simulator sources it needs to build its own contract-test image — no dependency on the root repo at commit-stage time.
- The scaffolded `backend/build.gradle` resolves `external-systems/simulators` correctly in both layouts, via the same scaffold-time path-rewrite idiom already used for Flyway migrations, Pact contracts and TS migration dirs.
- Scaffolding tests assert both halves (files copied, path rewritten), so a future change to shop's `build.gradle` depth or to the multirepo copy set fails fast in `go test` instead of 20 minutes into an acceptance-stage smoke run.
- Shop's `build.gradle` carries an explicit note that the literal is depth-coupled to a scaffolder rewrite, matching the warnings the Flyway/Pact paths already carry.

## Context — the failure and the root cause

**Failure chain (three repos deep):**

1. `gh-optivem` acceptance stage run `31865352299`, job `Smoke (ubuntu-latest, multitier, multirepo, java)` — the *only* failing job; the other three smoke legs are green on both attempts.
2. Go test `TestValidMultitierConfigurations/multitier_multirepo_java_ts_typescript` fails at `internal/config/config_system_test.go:292` — `expected exit code 0, got 1`.
3. The real failure is in the scaffolded student repo's backend commit stage, step **Run External System Contract Tests (Real ERP)**:

   ```
   ERROR: component backend suite External System Contract (Real):
     .\gradlew.bat externalSimulatorImage contractTest --tests '*RealParityContractTest': exit status 1
   ERROR: failed to build: unable to prepare context: path "external-systems/simulators" not found
   > Task :externalSimulatorImage FAILED
   ```

**Deterministic across both attempts — not a flake.** Attempt 1 (`test-app-f6457ce1-be0c9c83e96ab2ea-backend`, backend run `31865503736`) and attempt 2 (`test-app-02bb0e63-5234b8340d590f13-backend`, backend run `32006353486`) scaffold different repos and fail with byte-identical output at the same step. Everything before it in the backend commit stage passes, including `Run External System Contract Tests` (the *stub* half) — see the "stubs are provably unused" note under Step 2.

**Root cause, pinned:** `shop/system/multitier/backend-java/build.gradle:156-157`

```groovy
workingDir = file("${rootDir}/../../..")
commandLine 'docker', 'build', '-t', 'myshop/external-system-simulators:contract-test', 'external-systems/simulators'
```

In shop, `rootDir` = `system/multitier/backend-java`, so 3-up lands on the shop root where `external-systems/simulators` lives — which is why shop's own CI is green. Both scaffolded multitier layouts flatten the backend to `<repo>/backend`, so 3-up climbs clear out of the repo.

**Two defects, both in `internal/scaffolding/steps/apply_template.go`:**

- **(a) The path depth is never rewritten.** `applyMultitierMonorepo` (`apply_template.go:470-480`) and `applyMultitierMultirepo` (`apply_template.go:610-615`) already rewrite the analogous Flyway / Pact-contracts / TS-migrations paths via `flywayPathReplacements()`, `contractsPathReplacements()`, `tsMigrationsPathReplacements()`. There is simply no equivalent rule for the simulator docker-build context.
- **(b) The split backend repo never receives the simulators.** `applyMultitierMultirepo` calls `copyDbMigrations(shop, bDir)` (`apply_template.go:578`) and `copyContracts(shop, bDir)` (`:583`) but never `copyExternals(shop, bDir)`. Both the multitier monorepo repo (`:437`) and the multirepo *root* repo (`:513`) do get `external-systems/` — only the split backend repo is missed. So monorepo needs fix (a) alone; multirepo backend needs both.

**Scope notes:**

- **Java only.** `externalSimulatorImage` and the `external-contract` / `external-contract-real` suites exist solely in `shop/system/multitier/backend-java/` (`build.gradle:153-159`, `component-tests.yaml:31-52`). A repo-wide grep of `shop/system/` finds no other definition — the .NET and TypeScript multitier backends and all three monolith systems have no such task or suite. No language twins to fix.
- **Multitier monorepo Java has the identical latent bug.** In monorepo, `rootDir` = `<repo>/backend`, so `../../..` climbs two levels *above* the repo, while `external-systems/` sits at `<repo>/`. It was not in this run's smoke matrix (which covered `multitier/multirepo/java`, `monolith/monorepo/java`, `multitier/monorepo/typescript`, `monolith/multirepo/dotnet`), so it has simply never been exercised. The fix must cover both multitier layouts.
- **The rewrite literal is unique and self-contained.** `${rootDir}` appears exactly twice in `backend-java/build.gradle` — line 12 (`config/checkstyle/checkstyle.xml`) and line 156. Matching on the full `file("${rootDir}/../../..")` token cannot partial-match line 12. The docker context itself is self-contained (`external-systems/simulators/` holds `Dockerfile`, `package.json`, `package-lock.json`, `mock-server.js`, `.dockerignore`) — nothing is pulled from a parent directory, so the copy alone is sufficient.
- **`FixupAllTextFiles` already covers `.gradle`** (`internal/scaffolding/templates/templates.go:230`), so no new fixup entry point is needed.
- **Not reproducible locally, by construction — and that absence is itself the finding.** Shop's own layout resolves correctly (`rootDir` = `system/multitier/backend-java`, 3 up = shop root, where `external-systems/` lives), which is why shop CI is green. No existing `go test` asserts the scaffolded `build.gradle` content or the backend repo's file set, so nothing local fails today either — that gap is exactly what Step 3 closes. Verified by reading the scaffolder and both layouts, not by a local docker build.

**Approach chosen:** scaffold-time path rewrite, matching the existing Flyway / contracts / migrations idiom.
**Alternative considered and rejected:** make the Gradle task probe both candidate depths and use whichever directory exists. Self-healing across layouts, but it ships layout-guessing logic into the student-facing teaching repo and diverges from how every other cross-layout path is handled in this codebase.

## ▶ Next executable step (resume here)

Step 1 — in `gh-optivem`, add a `simulatorContextPathReplacements()` helper to `internal/scaffolding/steps/apply_template.go`, placed alongside `flywayPathReplacements()` / `tsMigrationsPathReplacements()` (~line 935), returning the single literal rewrite:

```go
{`file("${rootDir}/../../..")`, `file("${rootDir}/..")`}
```

Give it a doc comment in the same style as `flywayPathReplacements()`, spelling out the depth arithmetic per layout (shop: `system/multitier/backend-java` → 3 up → repo root; scaffold: `backend/` → 1 up → repo root). Then wire it in via `templates.FixupAllTextFiles` — whose extension allowlist already covers `.gradle` — in **both** `applyMultitierMonorepo` (target `repoDir`) and `applyMultitierMultirepo` (target `bDir`, the split backend repo). No-op on the .NET / TS backends, which don't carry the literal. Stop after `go build ./...` passes; this unblocks Step 3's test assertions.

## Steps

- [ ] **Step 1 — Add the path-rewrite rule.** In `internal/scaffolding/steps/apply_template.go`, add `simulatorContextPathReplacements()` next to `flywayPathReplacements()` / `tsMigrationsPathReplacements()` (~line 935) returning `{`file("${rootDir}/../../..")`, `file("${rootDir}/..")`}`, with a doc comment explaining the per-layout depth arithmetic. Apply it via `templates.FixupAllTextFiles` in **both** `applyMultitierMonorepo` (`repoDir`) and `applyMultitierMultirepo` (`bDir`).
- [ ] **Step 2 — Copy the simulators into the split backend repo.** In `applyMultitierMultirepo`'s backend-repo block (`apply_template.go:578-583`), add `copyExternals(shop, bDir)` next to the existing `copyDbMigrations(shop, bDir)` / `copyContracts(shop, bDir)` calls, preceded by the existing `log.Info(infoCopyingExternals)` line — matching the root-repo block at `:512-513`. **Decision (settled, do not re-open):** reuse `copyExternals` (which ships both `simulators/` and `stubs/`) rather than adding a simulators-only helper. Run `32006353486` proves `stubs/` is not *needed* — the `external-contract` (stub) suite passed in the backend commit stage with no `external-systems/` in the repo at all, so the stub side runs entirely in-process on WireMock. This is therefore a symmetry choice, not a correctness one: layout parity with the root repo and no second near-identical helper beats eliding one unused directory. Record that reasoning in a comment on the call.
- [ ] **Step 3 — Pin both halves with scaffolding tests.** Add to `internal/scaffolding/steps/replacements_test.go`, alongside the existing `TestFlywayPathReplacementsRewritesAndIsIdempotent` (`:922`) and `TestTsMigrationsPathReplacementsRewritePerArch` (`:1017`): (a) a unit test on `simulatorContextPathReplacements()` asserting the rewrite and its idempotency, mirroring the Flyway test's shape; (b) a full-apply assertion in the style of `TestMonolithFullApplyFlattensConfigNoSystemResidual` (`:807`) covering **both** multitier layouts — the scaffolded `backend/build.gradle` contains `file("${rootDir}/..")` and no longer contains `file("${rootDir}/../../..")`, and for **multirepo** the backend repo contains `external-systems/simulators/Dockerfile`.
- [ ] **Step 4 — Verify in gh-optivem.** Run `go build ./...`, `go test ./internal/scaffolding/...` and `go test ./internal/config/...` (unit tests only — not the long-running system test).
- [ ] **Step 5 — Annotate the shop-side literal (comment-only).** In `shop/system/multitier/backend-java/build.gradle:156`, note that `file("${rootDir}/../../..")` is depth-coupled to the gh-optivem scaffolder rewrite (`simulatorContextPathReplacements`), so a future edit must be mirrored there — same warning style the Flyway and Pact-contracts paths already carry. Behaviour-neutral; this is the only shop-side change.
- [ ] **Step 6 — Re-run the acceptance stage.** Re-run the gh-optivem acceptance stage and confirm the scaffolded backend commit stage's `Run External System Contract Tests (Real ERP)` step passes. Note that `multitier/monorepo/java` is **not** in the default smoke matrix (which runs `multitier/multirepo/java`, `monolith/monorepo/java`, `multitier/monorepo/typescript`, `monolith/multirepo/dotnet`), so covering the second half needs an explicit `workflow_dispatch` for that combination — a plain re-run only re-proves the multirepo half.

## Verification

- `go build ./...` in `gh-optivem`.
- `go test ./internal/scaffolding/...` and `go test ./internal/config/...` (unit tests, not the long system test).
- gh-optivem acceptance stage green for `multitier/multirepo/java` **and** `multitier/monorepo/java`, with the scaffolded backend commit stage's `Run External System Contract Tests (Real ERP)` step passing.
- No shop compile sweep needed (Step 5 is a comment). If shop's `build.gradle` is touched beyond the comment, run `./gradlew build` in `system/multitier/backend-java`.

## Open questions

- ~~**Simulators only, or the whole `external-systems/` tree, in the split backend repo?**~~ **Resolved — reuse `copyExternals`.** Run `32006353486` showed the stub suite passing with no `external-systems/` present, so `stubs/` is provably optional and the choice is about layout symmetry, not correctness. Rationale folded into Step 2.
- **Should Step 5 be stronger than a comment?** The cross-repo literal coupling is real fragility: shop can silently break gh-optivem scaffolding by editing one line. A comment is the cheap mitigation; a shop-side test asserting the exact literal, or moving the depth into a Gradle property the scaffolder rewrites by name, would be sturdier. Deferred unless the coupling bites a second time.
