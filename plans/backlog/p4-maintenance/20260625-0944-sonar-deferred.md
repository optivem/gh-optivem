# 2026-06-25 09:44:47 UTC — SonarCloud go:S3776 cognitive-complexity refactors (deferred)

## ▶ Next executable step (resume here)

Test-file findings done 2026-11-09: all 36 open test-file S3776 findings were marked Accepted in SonarCloud through the API. **16 production S3776 findings are open** (re-pulled 2026-11-09, listed below by complexity, highest first; `RecordFingerprints`, `ApplyGate` and `runSpecRecord` were split the same day, CC not yet re-confirmed in Sonar). Refactor one function at a time (see How to execute). Line numbers are SonarCloud's last analysed revision and may be a few lines off.

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

- `internal/scaffolding/steps/preview_confirm.go` — L322 (CC 33)
- `internal/build/runner/testfailures.go` — L267 (CC 29)
- `internal/kickoff/run.go` — L63 (CC 24)
- `internal/scaffolding/steps/shared_pipeline.go` — L49 (CC 21)
- `internal/engine/statemachine/run.go` — L268 (CC 20)
- `internal/atdd/runtime/driver/mode_compare.go` — L43 (CC 19)
- `internal/kickoff/stories.go` — L42 (CC 18)
- `main.go` — L629 (CC 17)
- `internal/kickoff/run.go` — L211 (CC 17)
- `internal/config/tool_install.go` — L234 (CC 17)
- `cross_repo_commands.go` — L617 (CC 17)
- `internal/scaffolding/steps/shared_pipeline.go` — L283 (CC 16)
- `internal/atdd/process/actions/verify_redshape.go` — L76 (CC 16)
- `cross_repo_commands.go` — L681 (CC 16)
- `internal/build/componenttest/config.go` — L133 (CC 16)
- `internal/scaffolding/steps/verify.go` — L456 (CC 16)
