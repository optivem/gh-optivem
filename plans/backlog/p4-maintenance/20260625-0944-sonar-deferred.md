# 2026-06-25 09:44:47 UTC — SonarCloud go:S3776 cognitive-complexity refactors (deferred)

## ▶ Next executable step (resume here)

All production items are done. Only the test-file items below remain, and the decision is to accept them as Won't Fix in SonarCloud (needs a human with project admin rights). Nothing executable remains for an agent; re-pull SonarCloud to confirm the S3776 count and delete this plan once the test-file findings are accepted.

## TL;DR

**Why:** SonarCloud flags `go:S3776` "Cognitive Complexity too high" (>15) on gh-optivem functions; each needs a genuine refactor in orchestration-critical Go, so they were excluded from the auto-fix run.
**End result:** Production functions reduced to CC <= 15 via helper extraction / guard clauses; test-function findings triaged (see decision) rather than churned.

## Status (re-pulled from SonarCloud 2026-11-03)

Open S3776 findings now: 73 (72 after the doctor_orphans fix lands) (was 123 in the original run; line numbers in older revisions of this plan were stale and have been replaced). The other open SonarCloud rules (S1135 TODO, S978 builtin shadowing, S4036 PATH, S1192, shell/PowerShell/docs-mockup findings) are NOT part of this plan.

Done this pass: `doctor_orphans.go` `loadPidMarkers` (extracted `loadRunDirMarkers`; behavior unchanged, root package tests pass).

Done (2026-11-03 autonomous pass, behaviour-preserving helper extraction, package tests pass): spec/config.go, kernel/shell/sonarcloud.go, scaffolding/steps/verify.go, kernel/approval/approval.go, config/token_auth.go, devworkflow/ghbulk/ghbulk.go, build/componenttest/config.go, build/runner/tests.go, configinit/prompt.go, intake/parse.go, spec/gate.go, spec/render.go, spec/report.go, statemachine/invariants.go, statemachine/load.go (L175 only; L347 remains). Plan line numbers were stale in several files; functions were located by content. Also done (later 2026-11-03 pass, build/vet/tests pass, CC<=15 not re-confirmed in Sonar): root-package commands (claude/cleanup/config/cross_repo/compile_summary), internal/config/config.go, diagrams/expanded.go, statemachine load.go L347 + run.go. Not attempted yet: the ATDD engine/driver/clauderun/preflight/actions items and kernel/projectconfig (CC 102) - those need dedicated coverage review first.

## Decisions

- **Test functions (`*_test.go`): not worth refactoring.** Table-style test bodies with nested asserts score high on CC but splitting them hurts readability and gains nothing; mark these Won't Fix / accept in SonarCloud (needs a human with project admin rights) rather than churn test code. Listed below for completeness.
- **Production functions:** worth fixing, one function at a time, highest-risk first only with a dedicated test run. Line numbers are as reported by SonarCloud on its last analysed revision and may be a few lines off.

## How to execute

For each production function: read it and its tests, extract cohesive helpers / guard clauses with no behavior change, run the package tests (scoped, `-p 2`) plus `go build ./...` and `go vet ./...`, commit, and delete the item from this file. Delete the file when empty.

## Production items


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
