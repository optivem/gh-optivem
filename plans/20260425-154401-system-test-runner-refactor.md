# Plan: Absorb Run-SystemTests orchestration into gh-optivem (PR 2 — shop cutover)

## Status

**PR 1 (gh-optivem runner package) is complete in this branch.** The `internal/runner` package ships with `LoadSystem` / `LoadTests` / `Build` / `Up` / `Down` / `RunTests`, plus 4 new subcommands (`build system`, `run system`, `stop system`, `run system tests`). All unit tests pass; nothing else regressed. The package is unused by the rest of the codebase — `verify.go` still calls `pwsh ./Run-SystemTests.ps1` and shop's PS1 files are untouched.

**This file now tracks PR 2 only:** the shop conversion + the gh-optivem-side cleanup that depends on the JSON files existing in scaffolded projects.

## Decision captured during PR 1: env vars split out of `command`

The original schema had a single `command` field. That doesn't work cross-platform: TypeScript suites use PowerShell-specific `$env:VAR = 'X'; npx ...` syntax that bash can't parse, and the whole point of dropping pwsh from Linux CI is that `sh -c` can't run it either. Dotnet/Java suites are pwsh-clean (they use `-e VAR=value` arg passing), but TypeScript needs translation.

**Resolution: env vars live in a separate `env` JSON field.** The runner sets them on the test process before exec'ing the command. So:

```jsonc
// PowerShell source:
//   "`$env:CHANNEL = 'API'; `$env:ENVIRONMENT = 'local'; npx playwright test ..."
// becomes:
{
  "command": "npx playwright test --project=acceptance-test tests/latest/acceptance",
  "env": { "CHANNEL": "API", "EXTERNAL_SYSTEM_MODE": "stub", "ENVIRONMENT": "local" }
}
```

For dotnet, the `-e VAR=value` flags can stay in `command` as-is (dotnet test consumes them). Same for Java's gradle args. **Only TypeScript suites need env extraction during translation.**

## Context (kept for PR 2 reviewers)

`shop/system-test/{typescript,dotnet,java}/Run-SystemTests.ps1` are **byte-identical** (533 lines × 3 copies). Each lang dir also carries 4 sibling config PS1s + 2 arch sub-dir configs — for a total of 18 PS1 files where only the configs vary by lang/arch. The orchestrator script gets duplicated again into every project that gh-optivem scaffolds.

The script does three things: (1) start/restart `docker compose` for the chosen `architecture` × `externalMode` pair and wait for health, (2) run lang-specific build commands, (3) run test suites (latest or legacy) with optional `-Suite`/`-Test`/`-Sample` filters and a summary table at the end. It runs on Windows (dev) and Ubuntu (CI, via `pwsh`).

After PR 2, scaffolded projects ship **only config files** — no orchestrator script. This eliminates duplication and removes the cross-platform `pwsh` dependency from the orchestration layer (note: dotnet's per-suite `pwsh bin/.../playwright.ps1 install` still requires pwsh — that is a separate, pre-existing pwsh dependency this refactor does not address).

## Config schema reference

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
    { "id": "smoke", "name": "...", "command": "...", "env": { "MODE": "stub" },
      "path": ".", "testReportPath": "...",
      "testInstallCommands": [...], "sampleTest": "..." }
  ]
}
```

Field notes:
- `env` is the new field decided during PR 1 (see above).
- `testInstallCommands` is always an array of strings in JSON. Source PS1 had it as `null` / string / array — normalize to array.
- `systems` is an array (not `{real, stub}` dict). `label` is just a log string; the runner doesn't interpret it.
- Filenames like `system-monolith.json` and `tests-latest.json` carry scenario identity but are **opaque file paths** to the runner.
- Splitting `system.json` from `tests.json` lets shop share one `system.json` across latest+legacy of the same arch — zero duplication of the systems block.

Shop layout — **12 JSON files**:
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

A run is the cross-product of one arch-dir's `system.json` × one lang-root `tests-*.json`. CI matrix passes both flags explicitly.

Scaffolded project — 2 or 3 JSONs (depending on `--exclude-legacy`):
```
my-project/system-test/
  system.json                   # picked up by default
  tests.json                    # latest, picked up by default
  tests-legacy.json             # only present when not --exclude-legacy
  docker-compose.local.{real,stub}.yml
```

Running legacy in a scaffolded project needs explicit `--tests tests-legacy.json`.

## Files to modify (PR 2)

### gh-optivem — modify

- `gh-optivem/internal/steps/verify.go:354-366` — replace the two `pwsh -NonInteractive -Command ./Run-SystemTests.ps1` calls (latest, then `-Legacy`) with two calls into the runner package directly (not shelling out — avoids PATH lookup of a not-yet-installed binary): first against `./system.json` + `./tests.json` (latest), then against `./system.json` + `tests-legacy.json`. The "legacy" semantic lives in verify.go's call site, not in the runner. Also replace the `Run-SystemTests.ps1` precondition check in `canRunLocalTests` with a `system.json` check.
- `gh-optivem/internal/steps/systemtest_prune.go` — delete `pruneRunSystemTests` (rewrites the now-deleted .ps1) and simplify `pruneSystemTestReadme` (or drop entirely if README no longer carries arch examples). `PruneSystemTestArch` becomes "delete `system-test/<unused-arch>/` directory only" (which `templates.SelectDockerCompose` already does — verify whether `PruneSystemTestArch` is still needed at all).
- `gh-optivem/internal/steps/systemtest_prune_test.go` — drop `TestStripPowerShellArchBlockRealScaffold` and `TestPruneReadmeLinesMonolithStripsMultitier`. Add a directory-deletion test if any logic remains.
- `gh-optivem/internal/steps/apply_template.go` — `copySystemTests` no longer needs `templates.StripFixedDimensions()` for the .ps1 (drop the call).
- `gh-optivem/internal/templates/templates.go` + `dimensions.go` — drop `StripFixedDimensions` (and `dimensions.go` entirely if nothing else references it). The README-stripping branch may still apply to the per-arch READMEs but the .ps1 surgery is gone.
- `gh-optivem/.github/actions/acceptance-test/action.yml` — no change needed if the action runs Go tests that internally call `gh optivem run system tests`. Verify.

### shop — convert (mechanical translation)

For each of 3 langs, write at `shop/system-test/<lang>/`: `tests-latest.json`, `tests-legacy.json` (lang root); `monolith/system.json`, `multitier/system.json` (per-arch dir) = 4 files per lang × 3 = 12 files.

Translation cheat-sheet per source PS1:
| Source PS1 | Target JSON | Notes |
|---|---|---|
| `Run-SystemTests.Config.ps1` | merged into both `tests-*.json` (testFilter + setupCommands) | Same setup/filter applies to both latest and legacy |
| `Run-SystemTests.Latest.Config.ps1` `Suites = @(...)` | `tests-latest.json` `suites: [...]` | TypeScript: pull `$env:` prefixes out of `Command` into `env` field |
| `Run-SystemTests.Legacy.Config.ps1` `Suites = @(...)` | `tests-legacy.json` `suites: [...]` | Same env extraction for TS |
| `<arch>/Run-SystemTests.Config.Architecture.ps1` `$SystemConfig = @{ "real" = ..., "stub" = ... }` | `<arch>/system.json` `systems: [{ label: "real", ...}, { label: "stub", ...}]` | Compose-file path becomes `docker-compose.local.<mode>.yml` (no `<arch>/` prefix — runner is in the arch dir already) |

Suite count audit (for review verification — counts taken during PR 1):
- TypeScript Latest: 12 suites · Legacy: 26 suites
- Dotnet    Latest: 11 suites · Legacy: 28 suites
- Java      Latest: ?         · Legacy: ?  (read these during PR 2)
- Total: ~120+ suite entries

`testInstallCommands` normalization:
- Source PS1: `$null` → JSON: omit field (or `[]`)
- Source PS1: `"pwsh bin/Debug/net8.0/playwright.ps1 install"` (string) → JSON: `["pwsh bin/Debug/net8.0/playwright.ps1 install"]` (array)
- Source PS1: array → JSON: keep as array

### shop — delete

- The 18 PS1 files: `Run-SystemTests.ps1`, `Run-SystemTests.Config.ps1`, `Run-SystemTests.Latest.Config.ps1`, `Run-SystemTests.Legacy.Config.ps1`, `monolith/Run-SystemTests.Config.Architecture.ps1`, `multitier/Run-SystemTests.Config.Architecture.ps1` × 3 langs.
- `shop/Run-AllSystemTests.ps1` — replace with `shop/run-all-system-tests.sh` (small bash wrapper that loops over `system-test/*/tests-*.json` paired with the matching `system-*.json` and calls `gh optivem run system tests --system <path> --tests <path>`, accumulating a Lang/Suite/Result/Duration summary table).
- Compose files stay where they are (the JSON references them by relative path).

### shop — modify

- `shop/.github/workflows/_prerelease-pipeline.yml:175,181` — replace `pwsh -NonInteractive -Command "./Run-SystemTests.ps1 -Architecture ${{ inputs.architecture }} -Sample"` with `gh optivem run system tests --system ${{ inputs.architecture }}/system.json --tests tests-latest.json --sample` (and `--tests tests-legacy.json` for the legacy step). Also need a step earlier in the workflow that installs the gh-optivem extension (`gh extension install optivem/gh-optivem`).
- `shop/docs/running-system-tests.md` — rewrite usage examples to `gh optivem run system tests ...`.
- `shop/system-test/<lang>/README.md` (3 files) — update example commands to the new CLI.

## Verification (PR 2)

1. **Local end-to-end** (Windows dev): in `shop/system-test/typescript/`, run `gh optivem run system --system monolith/system.json` then `gh optivem run system tests --system monolith/system.json --tests tests-latest.json --sample` — expect identical behavior to the old `pwsh ./Run-SystemTests.ps1 -Architecture monolith -Sample` (same suites pass, same report paths, same docker state at the end).
2. **Linux CI dry-run**: push to a branch, watch `_prerelease-pipeline.yml` go green on ubuntu-latest with `pwsh` no longer involved at the orchestration layer (per-suite `playwright.ps1 install` for dotnet still uses pwsh — pre-existing).
3. **gh-optivem self-test**: `bash scripts/manual-test.sh --no-cleanup ...` (per `CONTRIBUTING.md:40-43`) — verifies the scaffolder copies the JSON files, prunes the unused arch dir, and that the verify step calls into the runner package without shelling out to pwsh.
4. **Diff the suite results table** before/after on the same checkout, in all 3 langs, to confirm no behavior drift.

## Release coordination

Shop's CI in PR 2 will install `gh-optivem` from a release tag. PR 1 ships the runner package but does NOT cut a release. **Before merging PR 2:** tag and release a new gh-optivem version that includes the runner package. The shop CI step should pin to that tag (or use `@latest` if acceptable).

## Phasing reminder

PR 1 (this branch): runner package + subcommands + tests. **Done.**
PR 2 (next session): everything in this file. Recommended to do in a fresh conversation — fresh context budget, and PR 1's work is locked in.
