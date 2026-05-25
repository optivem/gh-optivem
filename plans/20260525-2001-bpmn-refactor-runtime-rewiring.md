# BPMN refactor — runtime + test fixture rewiring

Follow-up to `plans/20260525-1841-bpmn-refactor-downstream.md` (deleted; all items landed in commits `1994a47`, `8501187`, `e4aaf8c`). That plan's scope was prompt-file renames + operator-facing `agent_prompts:` → `task_prompts:` rename. It explicitly did NOT cover Go runtime rewiring — and the downstream commits (Items 1–5) broke CI because the Go code and test fixtures still reference the **old** prompt file names that were renamed / split / collapsed.

Per memory `feedback_new_plan_not_extend`: this is a fresh plan, not an extension of the prior one. CI red on commit `e4aaf8c`. ~40 unit-test failures across `clauderun`, `driver`, `agents`, `trace`, `actions`, `gates`, `projectconfig`, plus production-code references in `phase_scopes.go`, `driver.go`, `bindings.go`, `gates/bindings.go`, `diagram.go`.

## Old → new name mapping (combined from Items 1 + 2-5)

| Old MID task name                          | New disposition |
|--------------------------------------------|------------------|
| `at-red-test`                              | renamed → `write-acceptance-tests` |
| `ct-red-test`                              | renamed → `write-contract-tests` |
| `at-green-system`                          | renamed → `implement-system` |
| `at-green-system-driver`                   | renamed → `implement-system-driver-adapters` |
| `at-red-external-system-driver`            | renamed → `implement-external-system-driver-adapters` |
| `at-red-external-system-stub`              | renamed → `implement-external-system-stubs` |
| `at-refactor-implementation-refactoring`   | renamed → `refactor-system` |
| `refine-acc`                               | renamed → `refine-acceptance-criteria` |
| `at-refactor-system`                       | **deleted** (folded into `refactor-system`) |
| `fix-verify`                               | **deleted** (split into `fix-unexpected-passing-tests` + `fix-unexpected-failing-tests`) |
| `at-red-dsl`, `ct-red-dsl`                 | **deleted** (folded into parameterized `implement-dsl`) |
| `task-system-interface-redesign`           | **deleted** (folded into `implement-system` with Checklist branching) |
| `task-external-system-interface-redesign`  | **deleted** (folded into driver-adapter prompts with Checklist branching) |
| `task-system-implementation-refactoring`   | **deleted** (folded into `refactor-system`) |
| `task-system-driver-…`                     | check; likely folded |
| `legacy-*` (8 prompts)                     | **deleted** (Q16=B legacy collapse) |

## Working style

Same as the prior plan:

- `[autonomous]` — pure rename/delete with no logic change. Batch + commit without per-file review.
- `[gated]` — semantic decisions (split, fold, body rewrites). List files touched, present diffs, gate before commit.
- `[parallel-safe]` — dispatch one subagent per file when work is independent.

## Items

### 1. - [ ] **YAML — `refactor-tests` MID with missing prompt.** `[gated]` `[sequential]`

`internal/atdd/runtime/statemachine/process-flow.yaml:1334-1342` defines a MID `refactor-tests` that dispatches `task-name: refactor-tests`, but `internal/assets/runtime/prompts/atdd/refactor-tests.md` does NOT exist (only `refactor-system.md` does). The MID is reached via `CALL_REFACTOR_TEST_STRUCTURE` from the `refactor` TOP process and from `refactor-test-structure` HIGH.

Decision required: (a) author a `refactor-tests.md` prompt (separates test-refactoring concerns), (b) point the MID at `refactor-system` (one prompt for both), or (c) restructure the YAML so test refactoring uses a different MID name that already has a prompt.

**Done when:** YAML and prompt files are consistent; `agents.Names()` covers every task-name referenced from a MID's `task-name:`.

### 2. - [ ] **YAML — `fix-${failure-kind}` template resolution.** `[gated]` `[sequential]`

`process-flow.yaml:1721` has `task-name: "fix-${failure-kind}"`. Per the inline comment, `${failure-kind}` should resolve to either `unexpected-passing-tests` or `unexpected-failing-tests`, producing task-name `fix-unexpected-passing-tests` / `fix-unexpected-failing-tests` (both have prompt files). Confirm:

- Whether the binding plumbing in `internal/atdd/runtime/actions/bindings.go` actually emits `failure-kind` with the right values (`unexpected-passing-tests` / `unexpected-failing-tests`), or whether the values are something else (`passing` / `failing`, `unexpected-pass` / `unexpected-fail`, etc.).
- If the values don't match, fix the binding to emit the right kind string, OR rename the prompts to match the binding's output.

**Done when:** template resolution produces a valid `agents.Names()` entry for every reachable failure-kind value; add an assertion test if absent.

### 3. - [ ] **`phase_scopes.go::NonWritingAgents` — remove `fix-verify`.** `[gated]` `[sequential]`

`internal/atdd/phase_scopes.go:33-36` lists `"fix-verify": true` as a non-writing agent inheriting scope from the failing phase. `fix-verify` was deleted and split into two writing tasks (`fix-unexpected-passing-tests` / `fix-unexpected-failing-tests`) which carry explicit `scopes:` in the YAML.

Action: remove the `"fix-verify"` entry. Confirm no caller still relies on it; the two new fix-* tasks already declare scope at line 1304 / 1323. Update the comment on `NonWritingAgents` to drop the "fix-verify is a retry helper" sentence.

**Done when:** entry removed; tests pass.

### 4. - [ ] **`driver.go::fixVerifyChangedFiles` — rewire to new fix-* names or delete.** `[gated]` `[sequential]`

`internal/atdd/runtime/driver/driver.go:872-885` hard-checks `if agent != "fix-verify"`. Since `fix-verify` no longer exists, the function always returns `""`. Decide: (a) rewire the predicate to match the two new fix-* tasks (`fix-unexpected-passing-tests` / `fix-unexpected-failing-tests`) and propagate the ChangedFiles block to them, or (b) delete the function entirely if the two new tasks don't need the ChangedFiles dispatch-feedback (they can run `git status` themselves).

Audit the call-site at `driver.go:817` (`ChangedFiles: fixVerifyChangedFiles(agentName, opts.RepoPath)`) and decide together with Item 3.

**Done when:** function and call-site consistent with new task-name vocabulary; no dead code path.

### 5. - [ ] **Production code — stale-name sweep (non-test `.go` files).** `[gated]` `[parallel-safe]`

Files with old task-name references in production code:

- `process_commands.go`
- `internal/atdd/runtime/clauderun/clauderun.go`
- `internal/atdd/runtime/actions/bindings.go`
- `internal/atdd/runtime/gates/bindings.go`
- `internal/atdd/runtime/diagram/diagram.go`

For each: dispatch a subagent that greps the old names (use the mapping table at the top), updates references per the mapping, and either deletes obsolete branches (for deleted names with no replacement) or surfaces a "needs decision" report. Aggregate gate at the end.

**Done when:** no production `.go` file references a deleted/renamed old name; `go build ./...` clean.

### 6. - [ ] **Test fixture sweep (parallel subagents per file).** `[gated]` `[parallel-safe]`

Test files with old task names in fixtures / setup:

- `internal/atdd/runtime/clauderun/clauderun_test.go`
- `internal/atdd/runtime/driver/driver_test.go`
- `internal/atdd/runtime/driver/embedded_smoke_test.go`
- `internal/atdd/runtime/agents/embed_test.go`
- `internal/atdd/runtime/trace/trace_test.go`
- `internal/atdd/runtime/actions/bindings_test.go`
- `internal/atdd/runtime/statemachine/transitions_test.go`
- `internal/atdd/phase_scopes_test.go`
- `process_commands_test.go`

For each: dispatch a subagent that greps the old names, decides per-fixture whether the test's intent maps cleanly to a new name (use the mapping table) or whether the test is now obsolete (e.g. tests of split/folded prompts), and either updates or deletes. Aggregate gate at the end.

Token-efficient hint: dispatch in two waves (5 + 4) to keep individual subagent context small.

**Done when:** `go test ./... -count=1 -p 2` is green; no test references a deleted/renamed old name.

### 7. - [ ] **Comment sweep — stale references in production code.** `[autonomous]` `[parallel-safe]`

After Items 1–6, do a final grep for old names anywhere in non-test `.go` files (comments, doc strings, error messages). Replace per the mapping table or delete if the comment is now obsolete. No body changes — pure comment hygiene.

**Done when:** `grep -E '(at-red|at-green|ct-red|at-refactor|refine-acc|fix-verify|task-system-interface|task-external-system-interface|task-system-implementation)' --include='*.go' --exclude='*_test.go'` returns nothing.

## Out of scope

- **YAML structural changes** beyond the two surgical fixes in Items 1 and 2 — anything else goes in a separate plan.
- **Prompt-body content changes** — prompts are owned by `plans/20260525-1057-bpmn-refactor-design.md`'s Q-table; this plan only touches names and dispatch wiring.
- **CI pipeline tweaks** — fixing the test suite is the goal; the CI workflow itself stays as-is.

## Suggested order

1. Item 1 (YAML refactor-tests) — design decision, blocks Item 5 if it touches diagram.go.
2. Item 2 (YAML fix-${failure-kind}) — design decision; can be parallel-session with Item 1.
3. Items 3 + 4 together — both touch fix-verify; one commit.
4. Item 5 — production code stale-name sweep (parallel subagents).
5. Item 6 — test fixture sweep (parallel subagents, two waves).
6. Item 7 — comment hygiene; should be a small mechanical commit.

## Re-running `/execute-plan`

Invoke `/execute-plan plans/20260525-2001-bpmn-refactor-runtime-rewiring.md` repeatedly. Each invocation picks up the next unchecked Item, executes it, deletes the resolved item, commits.

## Standing constraints (from user memory)

- `feedback_renames_autonomous_content_gated` — rename autonomous, splits/decisions gated.
- `feedback_prefer_parallel_subagents` — Items 5–7 use subagent fan-out.
- `feedback_new_plan_not_extend` — this is a fresh plan, not an extension.
- `feedback_check_concurrent_agents` — check `git status` + `plans/*.md` for pickup markers before adding your own.
- `feedback_concurrent_agent_collision` — re-inspect `git log` before staging if mid-session new commits appear.
- `feedback_legacy_tests_no_marker` — when removing `legacy-*` test fixtures (if any survived), do not preserve any legacy marker on disk.
- `feedback_resolve_questions_upfront` — Items 1 + 2 + 4 are decision items; resolve them at /execute-plan time, not during execution.
