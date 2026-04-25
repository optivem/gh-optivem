# Plan: Absorb Run-SystemTests orchestration into gh-optivem

## Context

`shop/system-test/{typescript,dotnet,java}/Run-SystemTests.ps1` are **byte-identical** (533 lines × 3 copies). Each lang dir also carries 4 sibling config PS1s + 2 arch sub-dir configs — for a total of 15 PS1 files where only the configs vary by lang/arch. The orchestrator script gets duplicated again into every project that gh-optivem scaffolds.

The script does three things: (1) start/restart `docker compose` for the chosen `architecture` × `externalMode` pair and wait for health, (2) run lang-specific build commands, (3) run test suites (latest or legacy) with optional `-Suite`/`-Test`/`-Sample` filters and a summary table at the end. It runs on Windows (dev) and Ubuntu (CI, via `pwsh`).

The user wants the orchestrator absorbed into `gh-optivem` (already a Go-based `gh` extension) as new subcommands. Scaffolded projects then ship **only config files** — no orchestrator script. This eliminates duplication and removes the cross-platform `pwsh` dependency on Linux CI runners.

## Recommended approach

Add four Go subcommands to `gh-optivem` (`build system`, `run system`, `stop system`, `run system tests`), replace the 15 PS1 files in `shop/system-test/` with **12 JSON files** split between `system-*.json` (compose + health) and `tests-*.json` (setup + suites), update both invocation sites (`shop/.github/workflows/_prerelease-pipeline.yml` and `gh-optivem/internal/steps/verify.go`) to call the new subcommands, and delete the script + related templating.

### New subcommands (gh-optivem) — fully agnostic

The runner has **no concept of "monolith" / "multitier" / "latest" / "legacy" / language / etc.** It just accepts JSON config and does what it says. Architecture/suite-flavor/language terminology lives only in shop's filenames; the binary never parses paths or interprets keys.

Working-dir contract: the runner is invoked from whatever cwd the user is in. Default config paths are `./system.json` (for `build`/`run`/`stop system` and the up-probe in `run system tests`) and `./tests.json` (for the test step in `run system tests`). Override with `--system <path>` and/or `--tests <path>` for shop's multi-scenario case. The runner has no opinion on what dir you're in.

| Command | Purpose |
| --- | --- |
| `gh optivem build system` | `docker compose build` for every entry in `systems[]`. Flags: `--system <path>` (default: `./system.json`). |
| `gh optivem run system` | docker compose up + wait for health for every entry in `systems[]`. Flags: `--system <path>`, `--restart`, `--log-lines 50`. |
| `gh optivem stop system` | docker compose down + container cleanup for every entry in `systems[]`. Flags: `--system <path>`. |
| `gh optivem run system tests` | Test-runner setup (npm ci / dotnet build / gradle compileTestJava) + run all suites. Idempotently ensures the system is up via a fast HTTP probe (no flag — if already up, the probe is the only cost). Flags: `--system <path>` (default `./system.json`), `--tests <path>` (default `./tests.json`), `--suite <id>` (narrow to one suite by id), `--test <name>` (narrow to one test), `--sample` (use each suite's `sampleTest` field). |

Rationale on shape:
- `build system` and `stop system` use verb-noun (matches `docker build`, `docker stop`, `git push` muscle memory). `--rebuild` flag is dropped from `run system` since `build system && run system` is explicit and composable.
- `run system` and `run system tests` keep the user's exact wording. The `run` umbrella groups "do active work"; `run system tests` is the only nested form (`os.Args[2]=="system" && len(os.Args)>=4 && os.Args[3]=="tests"`).
- Test-runner setup commands stay inside `run system tests` rather than getting their own subcommand — they're tightly coupled to the test step, and the SUT-image build (`build system`) is the only "build" worth surfacing.

### Config schema — two file types, fully agnostic

Replace the 15 PS1 files (3× shared Config, 3× Latest, 3× Legacy, 3× monolith arch, 3× multitier arch) with **two file types**:

**`system.json`** — used by `build`/`run`/`stop system` and by `run system tests`'s up-probe:

```json
{
  "systems": [
    { "label": "real",
      "composeFile": "monolith/docker-compose.local.real.yml",
      "containerName": "...",
      "components":      [{ "name": "...", "url": "...", "containerName": "..." }],
      "externalSystems": [{ "name": "...", "url": "...", "containerName": "..." }] },
    { "label": "stub", "composeFile": "monolith/docker-compose.local.stub.yml", "...": "..." }
  ]
}
```

**`tests.json`** — used by `run system tests`:

```json
{
  "setupCommands": [{ "name": "Install Dependencies", "command": "npm ci" }],
  "testFilter": "--grep '<test>'",
  "suites": [
    { "id": "smoke", "name": "...", "command": "...", "path": "...",
      "testReportPath": "...", "testInstallCommands": [...], "sampleTest": "..." }
  ]
}
```

Notes on agnosticism:
- `systems` is an array (not a `{real, stub}` dict). `label` is just a log string; the runner doesn't interpret it.
- No `archs.monolith` / `suites.latest` etc. keys anywhere — the runner has no concept of arch/suite-flavor/language.
- Filenames like `system-monolith.json` and `tests-latest.json` carry scenario identity but are **opaque file paths** to the runner.
- Splitting `system.json` from `tests.json` lets shop share one `system.json` across latest+legacy of the same arch — zero duplication of the systems block.
- Field name is `setupCommands` (not `buildCommands`) to avoid collision with the `build system` subcommand. They're test-runner setup (`npm ci`, `dotnet build`, `gradle compileTestJava`), not SUT-image builds.

Shop layout — **12 JSON files** (filenames carry no arch/flavor vocabulary; arch is a directory concept):
```
shop/system-test/typescript/
  tests-latest.json                          # shared across both archs (suites don't vary by arch)
  tests-legacy.json
  monolith/
    system.json
    docker-compose.local.{real,stub}.yml
  multitier/
    system.json
    docker-compose.local.{real,stub}.yml
shop/system-test/dotnet/    # same shape
shop/system-test/java/      # same shape
```
A run is the cross-product of one arch-dir's `system.json` × one lang-root `tests-*.json`. CI matrix passes both flags explicitly; no duplication anywhere in the JSON. Filenames never mention "monolith" / "multitier" / "real" / "stub" — those concepts live only in directory names, which the runner treats as opaque path components.

Scaffolded project — 2 or 3 JSONs (depending on `--exclude-legacy`):
```
my-project/system-test/
  system.json                   # picked up by default
  tests.json                    # latest, picked up by default
  tests-legacy.json             # only present when not --exclude-legacy
  docker-compose.local.{real,stub}.yml
```
No lang dir nesting — the scaffolded project is already locked to one lang at scaffold time. Running legacy needs explicit `--tests tests-legacy.json`.

### Run-AllSystemTests.ps1

shop runs all 3 langs sequentially with a summary table ([shop/Run-AllSystemTests.ps1](../../shop/Run-AllSystemTests.ps1)). The runner stays language-blind, so the multi-lang loop is shop's concern: replace with a small bash script at shop root that iterates over `shop/system-test/*/tests-*.json` (paired with the matching `system-*.json`) and calls `gh optivem run system tests --system <path> --tests <path>` per file. The summary table (Lang/Suite/Result/Duration) belongs in that bash wrapper, since "languages" is a shop-only concept. Then delete `Run-AllSystemTests.ps1`.

## Files to modify

### gh-optivem — add

- `gh-optivem/main.go` — add three new top-level cases alongside existing `case "init":` ([main.go:52-81](../main.go)): `case "build":` (expects `system`), `case "stop":` (expects `system`), `case "run":` (expects `system` then optionally `tests`).
- `gh-optivem/internal/runner/config.go` — JSON unmarshaling for `SystemConfig` (systems[]) and `TestsConfig` (setupCommands + testFilter + suites). Loaders take a path; no auto-detection or path interpretation.
- `gh-optivem/internal/runner/system.go` — `Build()`, `Up()`, `Down()`, `Restart(force bool)` iterating `systems[]` and wrapping `docker compose -f <each entry's composeFile>`. Reuse `internal/shell/ghretry.go`'s `runWithRetryLoop` for the transient-network retry currently in `Start-System` ([Run-SystemTests.ps1:236-260](../../shop/system-test/typescript/Run-SystemTests.ps1)).
- `gh-optivem/internal/runner/health.go` — HTTP polling helper (`net/http` with 2s timeout, 30 attempts, 1s sleep). New file; no existing helper to reuse (`token_auth.go` is for one-shot token validation, not polling).
- `gh-optivem/internal/runner/tests.go` — setup commands + suite execution + filter injection (the `--grep '<test>'` substitution at [Run-SystemTests.ps1:294-307](../../shop/system-test/typescript/Run-SystemTests.ps1)) + summary table.
- Tests: `internal/runner/config_test.go`, `internal/runner/health_test.go` (with `httptest.Server`), `internal/runner/tests_test.go`.

### gh-optivem — modify

- `gh-optivem/internal/steps/verify.go:354-366` — replace the two `pwsh -NonInteractive -Command ./Run-SystemTests.ps1` calls (latest, then `-Legacy`) with two calls to the runner against different `tests.json` files: first with defaults (`./system.json`, `./tests.json` for latest), then with `--tests tests-legacy.json`. The "legacy" semantic lives in verify.go's call site, not in the runner. Self-recursion: gh-optivem is invoking itself — call the runner package directly instead of shelling out, to avoid PATH lookup of the not-yet-installed binary.
- `gh-optivem/internal/steps/systemtest_prune.go` — delete `pruneRunSystemTests` (rewrites the now-deleted .ps1) and `pruneSystemTestReadme` (or simplify if README still has arch examples). Step becomes "delete `system-test/<unused-arch>/` directory only".
- `gh-optivem/internal/steps/systemtest_prune_test.go` — drop `TestStripPowerShellArchBlockRealScaffold`, replace with a directory-deletion test.
- `gh-optivem/internal/steps/apply_template.go` — `copySystemTests` no longer needs `templates.StripFixedDimensions()` for the .ps1.
- `gh-optivem/internal/templates/templates.go` + `dimensions.go` — drop dimension-stripping for the now-deleted .ps1 (review what else `StripFixedDimensions` touches; keep the parts that still apply to README/compose).
- `gh-optivem/.github/actions/acceptance-test/action.yml` — no change needed if the action runs Go tests that internally call `gh optivem run system tests`.

### shop — convert

- For each of the 3 langs, write at `shop/system-test/<lang>/`: `tests-latest.json`, `tests-legacy.json` (lang root); `monolith/system.json`, `multitier/system.json` (per-arch dir) = 4 files per lang × 3 = 12 files. The JSON values are direct field-by-field translations of the existing `$Config`/`$Suites`/`$SystemConfig` objects.
- Delete the 15 PS1 files (Run-SystemTests.ps1, Run-SystemTests.Config.ps1, Run-SystemTests.Latest.Config.ps1, Run-SystemTests.Legacy.Config.ps1, monolith/Run-SystemTests.Config.Architecture.ps1, multitier/Run-SystemTests.Config.Architecture.ps1, × 3 langs).
- Compose files stay where they are (the JSON references them by relative path).

### shop — modify

- `shop/.github/workflows/_prerelease-pipeline.yml:175,181` — replace `pwsh -NonInteractive -Command "./Run-SystemTests.ps1 -Architecture ${{ inputs.architecture }} -Sample"` with `gh optivem run system tests --system ${{ inputs.architecture }}/system.json --tests tests-latest.json --sample` (and `--tests tests-legacy.json` for the legacy step). Add a step that installs the gh-optivem extension in the runner.
- `shop/Run-AllSystemTests.ps1` — delete; replace with a small `run-all-system-tests.sh` at shop root that loops over `system-test/*/tests-*.json` (paired with the matching `system-*.json`) and calls `gh optivem run system tests --system <path> --tests <path>` per file, accumulating a Lang/Suite/Result/Duration summary table.
- `shop/docs/running-system-tests.md` — rewrite usage examples to `gh optivem run system tests ...`.
- `shop/system-test/<lang>/README.md` (3 files) — update example commands.

## Verification

1. **Unit tests in gh-optivem**: `go test ./internal/runner/...` covers JSON loading, filter injection, health polling against a fake `httptest.Server`, summary-table formatting.
2. **Local end-to-end** (Windows dev): in `shop/system-test/typescript/`, run `gh optivem run system tests --system monolith/system.json --tests tests-latest.json --sample` — expect identical behavior to the old `pwsh ./Run-SystemTests.ps1 -Architecture monolith -Sample` (same suites pass, same report paths, same docker state at the end).
3. **Linux CI dry-run**: push to a branch, watch `_prerelease-pipeline.yml` go green on ubuntu-latest without `pwsh` involvement.
4. **gh-optivem self-test**: `bash scripts/manual-test.sh --no-cleanup ...` (per [CONTRIBUTING.md:40-43](../CONTRIBUTING.md)) — verifies the scaffolder copies the JSON files, prunes the unused arch dir, and that the verify step calls into the runner package without shelling out to pwsh.
5. **Diff the suite results table** before/after on the same checkout, in all 3 langs, to confirm no behavior drift.

## Phasing

Single PR is feasible but heavy. Recommended split:

- **PR 1** (gh-optivem): add the runner package + 4 subcommands + tests. Ship a release tag.
- **PR 2** (shop): convert PS1 → JSON, switch CI + verify.go, delete the 15 PS1 files. Depends on PR 1's tag being installed in CI.

This keeps PR 1 reviewable on its own (no shop changes, no breaking change for existing consumers) and PR 2 a focused cleanup.

## Open choices to confirm at approval

- Subcommand wording: `build system` / `run system` / `stop system` / `run system tests` (**recommended**, mixed verb-noun and the user's `run system tests` form) vs fully verb-noun `build system` / `start system` / `stop system` / `test system`.
- Noun choice: `system` (**recommended**, higher abstraction, doesn't lock the runner to docker-compose) vs `docker` (more honest about the implementation today).
- Config layout: split `system.json` + `tests.json` (**recommended** — clean separation, no duplication across latest/legacy of same arch) vs single combined `system-test.json` per scenario.
- `systems` value shape: array with free-form `label` field (**recommended** — runner has no semantic vocabulary for "real"/"stub") vs dict keyed by mode name.
- `Run-AllSystemTests.ps1` fate: delete + replace with a small bash wrapper at shop root that loops over configs (**recommended** — runner stays lang-agnostic) vs keep as a thin pwsh wrapper.
- Phase as 2 PRs (**recommended**) vs single PR.
