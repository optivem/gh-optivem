# 2026-06-25 09:44:47 UTC — SonarCloud go:S3776 cognitive-complexity refactors (deferred)

## TL;DR

**Why:** SonarCloud flagged 123 functions with `go:S3776` "Cognitive Complexity too high" (>15) across 66 files; they were excluded from the prior auto-fix run because each requires a genuine function refactor.
**End result:** All 123 functions reduced to CC ≤ 15 via helper extraction / guard clauses / flattening, with tests passing, so SonarCloud clears every finding on the next commit-stage run.

## ▶ Next executable step (resume here)

Batch CC=19–20 items (next-easiest wins, ~8 items): `claude_commands.go` L486 (CC=19), `main.go` L275 (CC=19), `internal/atdd/process/clauderun/clauderun_test.go` L861 (CC=19), `internal/atdd/runtime/tracker/github/github.go` L633 (CC=19), `internal/config/verify_flags_test.go` L14 (CC=19), `internal/atdd/process/actions/outputs.go` L333 (CC=20), `internal/build/runner/config.go` L181 (CC=20), `config_commands.go` L308 (CC=20). Read each, extract helpers, run package tests, commit.

**Run started:** 2026-06-25 09:44 UTC
**Source:** `/fix-sonar-warnings` run on `gh-optivem` (SonarCloud project `optivem_gh-optivem`).

## Why deferred

The same run auto-fixed 119 mechanical + security issues (constants, empty-func
comments, script-injection env hardening, shell `[[`, godre smells) and committed
them. The 123 `go:S3776` "Cognitive Complexity too high" findings were **not**
auto-fixed: each is a genuine function refactor (extract helpers, flatten nested
branching, split responsibilities) in orchestration-critical Go. Batch-refactoring
them autonomously risks silent behavioral regressions in the ATDD engine, so they
are deferred for human-gated execution via `/execute-plan`.

## How to execute

Work one function at a time. For each:
1. Read the function and its tests.
2. Reduce cognitive complexity below 15 — typically by extracting cohesive
   sub-steps into named helpers, replacing nested `if` ladders with early returns
   / guard clauses, or hoisting a switch body into a map. **Behavior must not
   change.**
3. Run the package's existing tests (scoped — never unbounded `go test ./...` on
   Windows; use `-p 2` or a single package) plus `go build ./...` and
   `go vet ./...`.
4. Check the box. Remove completed items from this file as you go (per plan-
   processing rules); delete the file when empty.

Group by file to amortize context. SonarCloud re-analysis clears each finding on
the next commit-stage run — no API mutation needed.

## Items (84 findings across 48 files)

### `claude_commands.go` (2)
- [ ] L386 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.
- [ ] L486 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `cleanup_commands.go` (2)
- [ ] L267 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.
- [ ] L363 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.

### `compile_summary.go` (1)
- [ ] L76 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.

### `config_commands.go` (2)
- [ ] L308 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L405 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.

### `config_commands_test.go` (1)
- [ ] L111 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `cross_repo_commands.go` (3)
- [ ] L150 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.
- [ ] L511 — Refactor this method to reduce its Cognitive Complexity from 52 to the 15 allowed.
- [ ] L289 — Refactor this method to reduce its Cognitive Complexity from 37 to the 15 allowed.

### `doctor_orphans.go` (2)
- [ ] L62 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L161 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.

### `internal/atdd/phase_scopes_test.go` (1)
- [ ] L303 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.

### `internal/atdd/process/actions/bindings_test.go` (5)
- [ ] L475 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.
- [ ] L392 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L1024 — Refactor this method to reduce its Cognitive Complexity from 27 to the 15 allowed.
- [ ] L2393 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.
- [ ] L2473 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.

### `internal/atdd/process/actions/command.go` (1)
- [ ] L95 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.

### `internal/atdd/process/actions/fix_progress_test.go` (1)
- [ ] L14 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/atdd/process/actions/outputs.go` (2)
- [ ] L122 — Refactor this method to reduce its Cognitive Complexity from 42 to the 15 allowed.
- [ ] L333 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.

### `internal/atdd/process/actions/scope.go` (1)
- [ ] L164 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `internal/atdd/process/clauderun/clauderun.go` (4)
- [ ] L1122 — Refactor this method to reduce its Cognitive Complexity from 49 to the 15 allowed.
- [ ] L606 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L805 — Refactor this method to reduce its Cognitive Complexity from 31 to the 15 allowed.
- [ ] L2251 — Refactor this method (`formatStreamEvent`) to reduce its Cognitive Complexity from 39 to the 15 allowed.

### `internal/atdd/process/clauderun/clauderun_test.go` (1)
- [ ] L861 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/atdd/process/gates/bindings_test.go` (4)
- [ ] L565 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L641 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.
- [ ] L711 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.
- [ ] L799 — Refactor this method to reduce its Cognitive Complexity from 29 to the 15 allowed.

### `internal/atdd/process/run_default_test.go` (4)
- [ ] L37 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.
- [ ] L165 — Refactor this method to reduce its Cognitive Complexity from 51 to the 15 allowed.
- [ ] L292 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.
- [ ] L388 — Refactor this method to reduce its Cognitive Complexity from 39 to the 15 allowed.

### `internal/atdd/process/transitions_test.go` (3)
- [ ] L365 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L541 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.
- [ ] L689 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.

### `internal/atdd/runtime/driver/driver.go` (3)
- [ ] L256 — Refactor this method to reduce its Cognitive Complexity from 55 to the 15 allowed.
- [ ] L2117 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L1200 — Refactor this method to reduce its Cognitive Complexity from 48 to the 15 allowed.

### `internal/atdd/runtime/driver/embedded_smoke_test.go` (1)
- [ ] L36 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/atdd/runtime/driver/scoped_test.go` (1)
- [ ] L17 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/atdd/runtime/driver/summary_sidecar.go` (1)
- [ ] L356 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `internal/atdd/runtime/driver/summary_sidecar_test.go` (1)
- [ ] L27 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.

### `internal/atdd/runtime/preflight/preflight.go` (5)
- [ ] L355 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.
- [ ] L502 — Refactor this method to reduce its Cognitive Complexity from 37 to the 15 allowed.
- [ ] L266 — Refactor this method to reduce its Cognitive Complexity from 41 to the 15 allowed.
- [ ] L701 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.
- [ ] L132 — Refactor this method to reduce its Cognitive Complexity from 37 to the 15 allowed.

### `internal/atdd/runtime/tracker/github/github.go` (1)
- [ ] L633 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/build/compiler/compiler_test.go` (1)
- [ ] L119 — Refactor this method to reduce its Cognitive Complexity from 27 to the 15 allowed.

### `internal/build/componenttest/config.go` (1)
- [ ] L84 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.

### `internal/build/componenttest/run_test.go` (1)
- [ ] L43 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/config.go` (2)
- [ ] L181 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L143 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/testcount_test.go` (1)
- [ ] L12 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/testnames_test.go` (1)
- [ ] L15 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/tests.go` (2)
- [ ] L441 — Refactor this method (`runOneSuite`) to reduce its Cognitive Complexity from 25 to the 15 allowed.
- [ ] L73 — Refactor this method to reduce its Cognitive Complexity from 35 to the 15 allowed.

### `internal/config/config.go` (1)
- [ ] L707 — Refactor this method to reduce its Cognitive Complexity from 29 to the 15 allowed.

### `internal/config/configinit/prompt.go` (1)
- [ ] L336 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.

### `internal/config/optivemyaml/optivemyaml_test.go` (1)
- [ ] L252 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.

### `internal/config/verify_flags_test.go` (1)
- [ ] L14 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/config/yaml_input.go` (1)
- [ ] L22 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/devworkflow/ghbulk/ghbulk.go` (1)
- [ ] L157 — Refactor this method to reduce its Cognitive Complexity from 32 to the 15 allowed.

### `internal/diagrams/diagram/expanded.go` (1)
- [ ] L149 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `internal/engine/statemachine/invariants.go` (1)
- [ ] L93 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.

### `internal/engine/statemachine/load.go` (2)
- [ ] L347 — Refactor this method to reduce its Cognitive Complexity from 32 to the 15 allowed.
- [ ] L175 — Refactor this method to reduce its Cognitive Complexity from 30 to the 15 allowed.

### `internal/engine/statemachine/run.go` (2)
- [ ] L249 — Refactor this method (`runProcess`) to reduce its Cognitive Complexity from 32 to the 15 allowed.
- [ ] L51 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.

### `internal/kernel/approval/approval.go` (1)
- [ ] L159 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/kernel/projectconfig/config.go` (4)
- [ ] L1139 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.
- [ ] L1247 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L1101 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L673 — Refactor this method to reduce its Cognitive Complexity from 124 to the 15 allowed.

### `internal/kernel/shell/retrycore_test.go` (1)
- [ ] L15 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.

### `internal/kernel/shell/sonarcloud.go` (1)
- [ ] L34 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `main.go` (1)
- [ ] L275 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `output_commands_test.go` (1)
- [ ] L342 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.

### `run_commands.go` (1)
- [ ] L46 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.
