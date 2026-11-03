# 2026-06-25 09:44:47 UTC — SonarCloud go:S3776 cognitive-complexity refactors (deferred)

## TL;DR

**Why:** SonarCloud flags `go:S3776` "Cognitive Complexity too high" (>15) on gh-optivem functions; each needs a genuine refactor in orchestration-critical Go, so they were excluded from the auto-fix run.
**End result:** Production functions reduced to CC <= 15 via helper extraction / guard clauses; test-function findings triaged (see decision) rather than churned.

## Status (re-pulled from SonarCloud 2026-11-03)

Open S3776 findings now: 73 (72 after the doctor_orphans fix lands) (was 123 in the original run; line numbers in older revisions of this plan were stale and have been replaced). The other open SonarCloud rules (S1135 TODO, S978 builtin shadowing, S4036 PATH, S1192, shell/PowerShell/docs-mockup findings) are NOT part of this plan.

Done this pass: `doctor_orphans.go` `loadPidMarkers` (extracted `loadRunDirMarkers`; behavior unchanged, root package tests pass).

## Decisions

- **Test functions (`*_test.go`): not worth refactoring.** Table-style test bodies with nested asserts score high on CC but splitting them hurts readability and gains nothing; mark these Won't Fix / accept in SonarCloud (needs a human with project admin rights) rather than churn test code. Listed below for completeness.
- **Production functions:** worth fixing, one function at a time, highest-risk first only with a dedicated test run. Line numbers are as reported by SonarCloud on its last analysed revision and may be a few lines off.

## How to execute

For each production function: read it and its tests, extract cohesive helpers / guard clauses with no behavior change, run the package tests (scoped, `-p 2`) plus `go build ./...` and `go vet ./...`, commit, and delete the item from this file. Delete the file when empty.

## Production items

- `claude_commands.go` — L386 (CC 24)
- `cleanup_commands.go` — L396 (CC 23)
- `compile_summary.go` — L76 (CC 28)
- `config_commands.go` — L560 (CC 23)
- `cross_repo_commands.go` — L150 (CC 31), L330 (CC 25), L559 (CC 49)
- `internal/atdd/process/actions/command.go` — L95 (CC 23)
- `internal/atdd/process/actions/outputs.go` — L122 (CC 36)
- `internal/atdd/process/actions/scope.go` — L164 (CC 25)
- `internal/atdd/process/clauderun/clauderun.go` — L836 (CC 26), L1153 (CC 49), L2282 (CC 34)
- `internal/atdd/runtime/driver/driver.go` — L278 (CC 33), L750 (CC 19), L1245 (CC 37), L2170 (CC 22)
- `internal/atdd/runtime/driver/summary_sidecar.go` — L356 (CC 25)
- `internal/atdd/runtime/preflight/preflight.go` — L132 (CC 26), L266 (CC 33), L355 (CC 20), L502 (CC 37), L701 (CC 26)
- `internal/config/config.go` — L712 (CC 29)
- `internal/diagrams/diagram/expanded.go` — L149 (CC 25)
- `internal/engine/statemachine/invariants.go` — L93 (CC 23)
- `internal/engine/statemachine/load.go` — L175 (CC 23), L347 (CC 28)
- `internal/engine/statemachine/run.go` — L51 (CC 25), L249 (CC 30)
- `internal/kernel/projectconfig/config.go` — L767 (CC 102), L1352 (CC 24)
- `internal/spec/render.go` — L307 (CC 27)
- `internal/spec/report.go` — L121 (CC 36)

## Test-file items (decision: do not refactor; accept in SonarCloud)

- `config_commands_test.go` — L111 (CC 19)
- `internal/atdd/phase_scopes_test.go` — L303 (CC 28)
- `internal/atdd/process/actions/bindings_test.go` — L1091 (CC 27), L2503 (CC 21)
- `internal/atdd/process/actions/fix_progress_test.go` — L14 (CC 21)
- `internal/atdd/process/gates/bindings_test.go` — L575 (CC 22), L651 (CC 25), L721 (CC 26), L809 (CC 29)
- `internal/atdd/process/run_default_test.go` — L37 (CC 23), L165 (CC 42), L292 (CC 24), L388 (CC 30)
- `internal/atdd/process/transitions_test.go` — L372 (CC 22), L548 (CC 33), L696 (CC 32)
- `internal/atdd/runtime/driver/scoped_test.go` — L17 (CC 21)
- `internal/atdd/runtime/driver/summary_sidecar_test.go` — L27 (CC 25)
- `internal/build/compiler/compiler_test.go` — L119 (CC 16)
- `internal/build/componenttest/run_test.go` — L43 (CC 18), L103 (CC 17)
- `internal/config/optivemyaml/optivemyaml_test.go` — L285 (CC 24)
- `internal/kernel/shell/retrycore_test.go` — L15 (CC 32), L131 (CC 20)
- `internal/scaffolding/steps/keycloak_realm_test.go` — L41 (CC 20)
- `internal/spec/junit_test.go` — L24 (CC 26)
- `internal/spec/usecase_test.go` — L36 (CC 23)
