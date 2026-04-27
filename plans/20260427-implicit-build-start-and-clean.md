# Plan: Implicit build/start in `test system`, plus `clean system`

Inspired by `dotnet test` and `./gradlew test`: those tools implicitly build the test code before running, with `--no-build` / `--no-restore` / `--rerun-tasks` escape hatches, and a separate `clean` task for "start fresh." The current `gh optivem test system` errors out unless the user has already run `gh optivem run system` first ([internal/runner/tests.go:51-58](../internal/runner/tests.go#L51-L58)). This plan brings the runner closer to the dotnet/gradle UX without losing the inner-loop speed.

## Goal

`gh optivem test system` should "just work" from a cold start (build images, start containers, then test) while keeping fast re-runs cheap and giving the user explicit knobs to skip steps or force rebuilds.

## CLI surface — before vs after

| Command | Today | After |
|---|---|---|
| `gh optivem test system` | Errors if system not up | Builds (incremental) → starts (skips if already up) → tests |
| `gh optivem test system --no-build` | n/a | Skip our explicit build step (compose `up` may still implicitly build missing images — see "Semantics" below) |
| `gh optivem test system --no-start` | n/a | Skip implicit start; error if not already up (today's behavior) |
| `gh optivem test system --restart` | n/a | Force tear-down + restart before tests (forwards to `SystemOptions.Restart`) |
| `gh optivem build system` | `docker compose build` | unchanged |
| `gh optivem build system --rebuild` | n/a | Force full rebuild — internally `docker compose build --no-cache` (analog of gradle `--rerun-tasks` / dotnet `--no-incremental`) |
| `gh optivem run system` | start + health-wait | unchanged |
| `gh optivem run system --restart` | force tear-down + restart | unchanged |
| `gh optivem stop system` | `docker compose down` | unchanged |
| `gh optivem clean system` | n/a | `docker compose down -v --rmi local` — delete containers, volumes, locally-built images |

Naming follows dotnet/gradle precedent: `clean` is its own command (not a `--clean` flag), and the chain is explicit (`gh optivem clean system && gh optivem test system`), mirroring `mvn clean test` / `./gradlew clean test` / `dotnet clean && dotnet test`.

## Mapping back to the inspirations

| Concept | dotnet | gradle | gh optivem (after) |
|---|---|---|---|
| Build outputs | `bin/`, `obj/` | `build/` | docker images, containers, volumes |
| Implicit build before test | yes | yes | yes (new) |
| Skip build flag | `--no-build` | (exclude task) | `--no-build` (new) |
| Skip start | n/a | n/a | `--no-start` (new — gh optivem-specific) |
| Force rebuild w/o clean | `build --no-incremental` | `--rerun-tasks` | `build system --rebuild` (new) |
| Delete outputs | `dotnet clean` | `./gradlew clean` | `clean system` (new) |

## Implementation

### 1. Runner: drop the "system not running" error, add gating flags

[internal/runner/tests.go:51-58](../internal/runner/tests.go#L51-L58) — the `if sys != nil { ... IsAnyURLUp ... }` block currently returns an error when the system is down. Replace with a decision driven by new fields on `TestOptions`:

- Add `NoBuild bool`, `NoStart bool`, `Restart bool` to `TestOptions` ([internal/runner/tests.go:17-28](../internal/runner/tests.go#L17-L28)).
- New flow inside `RunTests`:
  1. If `sys != nil` and `!opts.NoBuild`: call `Build(sys, cwd)` (incremental — `docker compose build` reuses layer cache).
  2. If `sys != nil` and `!opts.NoStart`: call `Up(sys, cwd, SystemOptions{Restart: opts.Restart, Health: opts.Health})`. `Up` already short-circuits when `IsAnyURLUp` is true, so re-runs stay cheap.
  3. If `sys != nil` and `opts.NoStart`: keep today's `IsAnyURLUp` check and error out with the same "start it first" message — that's the explicit opt-out path.
  4. Continue to setup commands + suites as today.

Update doc comment on `RunTests` to describe the new flow.

### 2. Runner: add `Clean`

New function in [internal/runner/system.go](../internal/runner/system.go) alongside `Build` / `Up` / `Down`:

```go
// Clean runs `docker compose -f <composeFile> down -v --rmi local` for every
// entry in sys. Removes containers, named volumes, and images built locally
// from this compose file. External images (pulled from registries) are left
// alone — same scope as `./gradlew clean` (deletes build outputs, not the
// dependency cache).
func Clean(sys *SystemConfig, cwd string) error { ... }
```

Reuse `runCompose` / `downOne` patterns. Like `Down`, errors per-system should be logged but not short-circuit the loop.

### 3. CLI wiring

[runner_commands.go](../runner_commands.go):

- **`runTestSystem`** ([runner_commands.go:108-136](../runner_commands.go#L108-L136)): add `--no-build`, `--no-start`, `--restart` flags; populate `TestOptions{NoBuild, NoStart, Restart}`. Update the doc comment ([runner_commands.go:100-107](../runner_commands.go#L100-L107)) to describe implicit build/start as the default.
- **`runBuildSystem`** ([runner_commands.go:42-56](../runner_commands.go#L42-L56)): add `--rebuild` flag; pass through to a new `BuildOptions{Rebuild bool}` on `runner.Build`. When set, append `--no-cache` to the `docker compose build` invocation. Outcome-oriented naming consistent with `--restart` (force) rather than mechanism-oriented `--no-cache` (skip).
- **New `runCleanSystem`**: same shape as `runStopSystem`, calls `runner.Clean`.
- **`dispatchClean`**: new dispatcher routing `gh optivem clean <noun>` (only `system` for now). Wire into `main.go`'s top-level switch.

### 4. Tests

- [internal/runner/tests_test.go](../internal/runner/tests_test.go): add cases covering `NoBuild`, `NoStart`, and the default implicit build+start path. The existing test harness mocks `runShell` style — extend that to capture which steps were invoked.
- New `system_clean_test.go` (or extend existing system tests): verify `Clean` invokes `down -v --rmi local` per system.
- Manual test: extend [scripts/manual-test-runner-shop.sh](../scripts/manual-test-runner-shop.sh) with a cold-start path (`stop system && test system`) to prove implicit build/start works end-to-end.

### 5. Docs

- [README.md](../README.md): update the runner-subcommand section with the new flag matrix and the `clean system` command.
- [docs/gh-monitoring-process.md](../docs/gh-monitoring-process.md): if any examples reference `run system && test system`, adjust to show the simpler `test system` cold-start path.
- Anywhere the "start it first" error message is documented as expected behavior — that error now only fires under `--no-start`.

## Semantics — what `--no-build` means precisely

`--no-build` skips **only our explicit `Build()` step**. It does **not** pass `--no-build` to `docker compose up`. So the subsequent `Up` retains compose's default behavior: if an image referenced by a `build:` section is missing, compose will build it implicitly during up.

Why not the strict (dotnet-faithful) variant that errors on missing images?

- **Newcomer-friendly default:** no scary errors, system always comes up. The flag is a soft hint ("skip the redundant cache check"), not a contract.
- **Closer to gradle's task-graph philosophy:** missing prereqs trigger a build automatically rather than failing.
- **Strict-mode escape hatch already exists:** the CI use case ("fail fast if a separate build stage didn't run") is better served by `--no-build --no-start` together — system was pre-started by an earlier stage, no implicit rebuild possible.

The perf benefit of `--no-build` is preserved in either variant: when the image already exists, we skip the `docker compose build` cache lookup (~1–3s).

## Open questions

1. **Should `clean system` imply `stop system` first?** `docker compose down -v --rmi local` already does the stop, so this is a non-issue — clean is strictly a superset of stop. Documentation should call this out so users don't run them in sequence unnecessarily.
2. **Should `--no-build` imply `--no-start`?** In dotnet, `--no-build` implies `--no-restore` because you can't build without restored packages. The analog here would be: if you're not building, you might still want to start (using whatever images already exist). They're independent — keep them as separate flags.
3. **What about `--rerun` semantics?** Gradle's `--rerun-tasks` forces re-execution without deleting outputs. The closest docker-compose analog is `build --rebuild` (forces rebuild, keeps existing volumes). That's already covered by item 3 above. Not adding a `test system --rerun` for now — if needed later, it would map to `--restart` (force re-up) and is already there.

## Out of scope

- Changing `system.json` / `tests.json` schema. All new behavior is CLI-flag driven; configs are unchanged.
- Cross-language differences in the inspiration tools (Maven `clean`, npm `test`, pytest, etc.) — the dotnet/gradle pattern is the reference and is consistent enough.
- Pre-pull of base images (`docker compose pull`) before build. That's a separate optimization, not part of the core dotnet/gradle analogy.
