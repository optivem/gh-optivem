# Plan: rename `verify-results` → `verify-failure-output`

## Why

The placeholder/field/state-key cluster currently named `verify-results` (in its five
spellings) carries **only the failure diagnostic** of a verify step — the captured
stdout/stderr (or build log) tail that the runner emits when a `gh optivem test run` /
compile goes red. On a *passing* run it is deliberately empty: `runCommand` unsets it on
success (`bindings.go:863`).

The name "results" is outcome-neutral and misleads readers into thinking it holds output
for *any* verify outcome, pass or fail. That misconception is exactly what produced the
`unexpected-passing-tests-fixer` crash (`render prompt: unresolved placeholder
${verify-results}`, rehearsal-72, 2026-06-06): the passing-fixer agent body referenced a
placeholder that its trigger condition guarantees is empty. That agent has since had the
placeholder removed (separate edit, already landed). This plan addresses the *remaining*
confusion at the source by renaming the cluster so the contract is self-evident.

Two things the new name must fix:
- **Pass/fail ambiguity** — the value exists only on red. → add `failure`.
- **Scope vagueness** — it carries compile/build logs too, not just test assertions (see
  `unexpected-failing-tests-fixer.md`: "for compile failures, the build log… For test
  failures, one block per failed test"). → keep the generic `output`, avoid `test`.

## Decision: new name `verify-failure-output`

- `failure` kills the pass/fail ambiguity — populated on red, empty on green.
- `verify` anchors it to the surrounding process vocabulary (`verify-tests-pass` /
  `verify-tests-fail` processes, `VERIFY_PASS_END` / `VERIFY_FAIL_END` nodes) so a reader
  can grep one term and pull up the producer, the gateways, and this payload together.
- `output` is honestly broad — covers both a test failure block and a build log, where
  `test-failure-output` would lie about scope.

Rejected: `failure-output` / `failure-log` (severs the `verify` navigation thread);
`red-output` (in-group jargon — `Options.RedOutput` assumes the reader knows the red/green
convention; `failure` is universally legible).

Per-layer rendering of the new name:

| Layer        | Old                         | New                              |
|--------------|-----------------------------|----------------------------------|
| Placeholder  | `${verify-results}`         | `${verify-failure-output}`       |
| Struct field | `Options.VerifyResults`     | `Options.VerifyFailureOutput`    |
| State key    | `verify_results_text`       | `verify_failure_output`          |
| Helper func  | `formatVerifyResults`       | `formatVerifyFailureOutput`      |

Note: the `_text` suffix on the state key is dropped — `output` already implies text, and
no sibling state key carries the suffix.

## Scope: pure rename, no behaviour change

This is a mechanical rename of one value threaded through five spellings. **No control
flow, no stamping conditions, no load-bearing semantics change.** The
`isTestRun && !succeeded` stamping guard, the success-path Unset, and the
register-only-when-non-empty pattern in `renderPrompt` all stay exactly as they are — only
the identifiers change. The `unexpected-passing-tests-fixer` is already off this
placeholder, so the *only* agent consuming `${verify-failure-output}` after this rename is
`unexpected-failing-tests-fixer` (correct — that path genuinely has failure output).

## Edits

### 1. `internal/atdd/runtime/actions/bindings.go`
- `:863` — `ctx.Unset("verify_results_text")` → `ctx.Unset("verify_failure_output")`
- `:878` — `ctx.Set("verify_results_text", formatVerifyResults(...))` →
  `ctx.Set("verify_failure_output", formatVerifyFailureOutput(...))`
- `:883` — doc comment `formatVerifyResults builds the ${verify-results} payload…` →
  `formatVerifyFailureOutput builds the ${verify-failure-output} payload…`
- `:896` — `func formatVerifyResults(…)` → `func formatVerifyFailureOutput(…)`

### 2. `internal/atdd/runtime/clauderun/clauderun.go`
- `:156-168` — the `VerifyResults` field doc block: update prose (mentions
  `verify_results_text`, `cOpts.VerifyResults`, `${verify-results}`) and rename the field
  `VerifyResults string` → `VerifyFailureOutput string`.
- `:738` — comment `VerifyResults is load-bearing…` → `VerifyFailureOutput is load-bearing…`
- `:745-746` — `if opts.VerifyResults != "" { params["verify-results"] = opts.VerifyResults }`
  → `if opts.VerifyFailureOutput != "" { params["verify-failure-output"] = opts.VerifyFailureOutput }`

### 3. `internal/atdd/runtime/driver/driver.go`
- `:1190` — `VerifyResults: ctx.GetString("verify_results_text"),` →
  `VerifyFailureOutput: ctx.GetString("verify_failure_output"),`

### 4. `internal/atdd/runtime/statemachine/process-flow.yaml`
- `:1302` — comment `ctx.State (verify_results_text)…` → `verify_failure_output`.
- `:1362` — comment `runCommand Unsets verify_results_text…` → `verify_failure_output`.
  (Comments only — no node/flow change.)

### 5. `internal/assets/runtime/agents/atdd/unexpected-failing-tests-fixer.md`
- `:15` — parameter label `verify_results` → `verify_failure_output` (keep the prose body;
  it accurately describes the compile-log/test-block payload).
- `:17` — `${verify-results}` → `${verify-failure-output}`.

## Tests to update

### `internal/atdd/runtime/actions/bindings_test.go`
- Rename the three test funcs:
  - `TestRunCommand_TestRunFailure_StampsVerifyResults` → `…StampsVerifyFailureOutput`
  - `TestRunCommand_TestRunSuccess_DoesNotStampVerifyResults` → `…DoesNotStampVerifyFailureOutput`
  - `TestRunCommand_NonTestRunFailure_DoesNotStampVerifyResults` → `…DoesNotStampVerifyFailureOutput`
- Every `ctx.GetString("verify_results_text")` / `ctx.State["verify_results_text"]` /
  `ctx.Set("verify_results_text", …)` and the failure-message strings (`:652-655`, `:897`,
  `:899` `${verify-results}`, `:920-930`, `:951`, `:962-963`, `:970`, `:985-986`, `:998`,
  `:1013`, `:1023`) → `verify_failure_output` / `${verify-failure-output}`.

### `internal/atdd/runtime/clauderun/clauderun_test.go`
- `:2127` — `opts.VerifyResults = "results"` → `opts.VerifyFailureOutput = "results"`.
- Confirm the surrounding test still asserts `${verify-failure-output}` renders (update the
  expected placeholder string if the test greps for it).

## Verification
- `go build ./...`
- `go test ./internal/atdd/... ./...`
- Grep guard — must return **zero** hits after the rename:
  `rg 'verify_results_text|VerifyResults|formatVerifyResults|verify-results' --type go --type yaml --type md internal/`
- Spot-check: `gh optivem` dispatch of `unexpected-failing-tests-fixer` on a red run still
  renders the failure tail under `${verify-failure-output}` (the only remaining consumer).

## Out of scope
- The passing-fixer placeholder removal (already landed separately).
- Any change to *when* the value is stamped / unset, or to the load-bearing
  register-when-non-empty pattern — this is a rename only.
