# 2026-06-25 09:44:47 UTC — SonarCloud go:S3776 cognitive-complexity refactors (deferred)

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

## Items (123 findings across 66 files)

### `claude_commands.go` (4)
- [ ] L169 — Refactor this method to reduce its Cognitive Complexity from 18 to the 15 allowed.
- [ ] L107 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L349 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.
- [ ] L449 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `cleanup_commands.go` (2)
- [ ] L260 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.
- [ ] L356 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.

### `cleanup_commands_test.go` (1)
- [ ] L33 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `compile_summary.go` (1)
- [ ] L76 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.

### `config_commands.go` (2)
- [ ] L308 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L405 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.

### `config_commands_test.go` (1)
- [ ] L111 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `cross_repo_commands.go` (3)
- [ ] L142 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.
- [ ] L503 — Refactor this method to reduce its Cognitive Complexity from 52 to the 15 allowed.
- [ ] L281 — Refactor this method to reduce its Cognitive Complexity from 37 to the 15 allowed.

### `doctor_orphans.go` (2)
- [ ] L62 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L161 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.

### `internal/atdd/phase_scopes_test.go` (2)
- [ ] L298 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.
- [ ] L238 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `internal/atdd/process/actions/bindings_test.go` (5)
- [ ] L475 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.
- [ ] L392 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L1024 — Refactor this method to reduce its Cognitive Complexity from 27 to the 15 allowed.
- [ ] L2407 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.
- [ ] L2487 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.

### `internal/atdd/process/actions/command.go` (1)
- [ ] L91 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.

### `internal/atdd/process/actions/fix_progress_test.go` (1)
- [ ] L14 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/atdd/process/actions/outputs.go` (2)
- [ ] L114 — Refactor this method to reduce its Cognitive Complexity from 42 to the 15 allowed.
- [ ] L325 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.

### `internal/atdd/process/actions/scope.go` (2)
- [ ] L79 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.
- [ ] L159 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `internal/atdd/process/actions/worktree.go` (1)
- [ ] L22 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/atdd/process/clauderun/clauderun.go` (4)
- [ ] L1116 — Refactor this method to reduce its Cognitive Complexity from 49 to the 15 allowed.
- [ ] L600 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L799 — Refactor this method to reduce its Cognitive Complexity from 31 to the 15 allowed.
- [ ] L2235 — Refactor this method to reduce its Cognitive Complexity from 39 to the 15 allowed.

### `internal/atdd/process/clauderun/clauderun_test.go` (3)
- [ ] L861 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.
- [ ] L2632 — Refactor this method to reduce its Cognitive Complexity from 18 to the 15 allowed.
- [ ] L2549 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/atdd/process/gates/bindings_test.go` (12)
- [ ] L458 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L540 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L731 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.
- [ ] L499 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L375 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L416 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L579 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L620 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L661 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.
- [ ] L819 — Refactor this method to reduce its Cognitive Complexity from 29 to the 15 allowed.
- [ ] L900 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L939 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/atdd/process/run_default_test.go` (4)
- [ ] L37 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.
- [ ] L165 — Refactor this method to reduce its Cognitive Complexity from 51 to the 15 allowed.
- [ ] L292 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.
- [ ] L388 — Refactor this method to reduce its Cognitive Complexity from 39 to the 15 allowed.

### `internal/atdd/process/transitions_test.go` (4)
- [ ] L895 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.
- [ ] L689 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.
- [ ] L541 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.
- [ ] L365 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.

### `internal/atdd/runtime/driver/driver.go` (4)
- [ ] L256 — Refactor this method to reduce its Cognitive Complexity from 55 to the 15 allowed.
- [ ] L2117 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L1200 — Refactor this method to reduce its Cognitive Complexity from 48 to the 15 allowed.
- [ ] L705 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/atdd/runtime/driver/driver_test.go` (1)
- [ ] L1358 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

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

### `internal/atdd/runtime/trace/trace.go` (1)
- [ ] L307 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/atdd/runtime/tracker/github/github.go` (1)
- [ ] L549 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/build/compiler/compiler_test.go` (1)
- [ ] L119 — Refactor this method to reduce its Cognitive Complexity from 27 to the 15 allowed.

### `internal/build/componenttest/config.go` (1)
- [ ] L84 — Refactor this method to reduce its Cognitive Complexity from 28 to the 15 allowed.

### `internal/build/componenttest/run_test.go` (1)
- [ ] L43 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/import_guard_test.go` (1)
- [ ] L22 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/build/runner/config.go` (2)
- [ ] L181 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L143 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/status.go` (1)
- [ ] L33 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/build/runner/testcount_test.go` (1)
- [ ] L12 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/testnames_test.go` (1)
- [ ] L15 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/build/runner/tests.go` (2)
- [ ] L421 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.
- [ ] L73 — Refactor this method to reduce its Cognitive Complexity from 35 to the 15 allowed.

### `internal/config/config.go` (3)
- [ ] L707 — Refactor this method to reduce its Cognitive Complexity from 29 to the 15 allowed.
- [ ] L1396 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.
- [ ] L1496 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `internal/config/configinit/prompt.go` (1)
- [ ] L336 — Refactor this method to reduce its Cognitive Complexity from 26 to the 15 allowed.

### `internal/config/env_file.go` (1)
- [ ] L47 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/config/optivemyaml/optivemyaml_test.go` (1)
- [ ] L252 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.

### `internal/config/token_auth.go` (1)
- [ ] L343 — Refactor this method to reduce its Cognitive Complexity from 18 to the 15 allowed.

### `internal/config/token_auth_test.go` (1)
- [ ] L36 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `internal/config/verify_environment_tools_test.go` (1)
- [ ] L111 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/config/verify_flags_test.go` (1)
- [ ] L14 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/config/yaml_input.go` (1)
- [ ] L22 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/devworkflow/ghbulk/ghbulk.go` (2)
- [ ] L157 — Refactor this method to reduce its Cognitive Complexity from 32 to the 15 allowed.
- [ ] L243 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `internal/devworkflow/sonar/sonar.go` (1)
- [ ] L111 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/diagrams/diagram/diagram.go` (1)
- [ ] L566 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `internal/diagrams/diagram/expanded.go` (1)
- [ ] L149 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `internal/engine/statemachine/channels.go` (1)
- [ ] L285 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `internal/engine/statemachine/invariants.go` (2)
- [ ] L93 — Refactor this method to reduce its Cognitive Complexity from 23 to the 15 allowed.
- [ ] L166 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `internal/engine/statemachine/load.go` (2)
- [ ] L347 — Refactor this method to reduce its Cognitive Complexity from 32 to the 15 allowed.
- [ ] L175 — Refactor this method to reduce its Cognitive Complexity from 30 to the 15 allowed.

### `internal/engine/statemachine/reuse_process_test.go` (1)
- [ ] L62 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.

### `internal/engine/statemachine/run.go` (3)
- [ ] L153 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.
- [ ] L230 — Refactor this method to reduce its Cognitive Complexity from 32 to the 15 allowed.
- [ ] L44 — Refactor this method to reduce its Cognitive Complexity from 33 to the 15 allowed.

### `internal/kernel/approval/approval.go` (1)
- [ ] L159 — Refactor this method to reduce its Cognitive Complexity from 21 to the 15 allowed.

### `internal/kernel/projectconfig/config.go` (4)
- [ ] L1139 — Refactor this method to reduce its Cognitive Complexity from 24 to the 15 allowed.
- [ ] L1247 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.
- [ ] L1101 — Refactor this method to reduce its Cognitive Complexity from 20 to the 15 allowed.
- [ ] L673 — Refactor this method to reduce its Cognitive Complexity from 124 to the 15 allowed.

### `internal/kernel/projectconfig/config_test.go` (1)
- [ ] L491 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.

### `internal/kernel/shell/retrycore_test.go` (1)
- [ ] L15 — Refactor this method to reduce its Cognitive Complexity from 34 to the 15 allowed.

### `internal/kernel/shell/sonarcloud.go` (1)
- [ ] L34 — Refactor this method to reduce its Cognitive Complexity from 25 to the 15 allowed.

### `internal/scaffolding/steps/project_test.go` (1)
- [ ] L47 — Refactor this method to reduce its Cognitive Complexity from 18 to the 15 allowed.

### `main.go` (2)
- [ ] L275 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.
- [ ] L569 — Refactor this method to reduce its Cognitive Complexity from 17 to the 15 allowed.

### `output_commands.go` (1)
- [ ] L87 — Refactor this method to reduce its Cognitive Complexity from 16 to the 15 allowed.

### `output_commands_test.go` (1)
- [ ] L342 — Refactor this method to reduce its Cognitive Complexity from 22 to the 15 allowed.

### `run_commands.go` (1)
- [ ] L46 — Refactor this method to reduce its Cognitive Complexity from 19 to the 15 allowed.
