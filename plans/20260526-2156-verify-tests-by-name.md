# Verify tests by exact name instead of by category

## Origin / intent

`gh optivem test run --filter-type=test-type --filter-value=acceptance`
fails today with `unknown flag: --filter-type`. The trace originates
from `VERIFY_TESTS_FAIL_ACCEPTANCE` in `process-flow.yaml:733-739`,
which passes `filter-type: test-type, filter-value: acceptance` into
`run-tests` → `execute-command`. `bindings.go::runCommand:716-721`
appends those as `--filter-type=…` / `--filter-value=…` flags, but
`test_commands.go::newTestRunCmd:50-95` only registers `--suite`,
`--test`, `--sample`, `--list`. The dispatcher and the CLI have been
out of sync since the structured-filter vocabulary was introduced.

The trivial fix would be to teach the CLI `--filter-type` /
`--filter-value`. That would unblock the trace but bake in the wrong
semantics: "verify the test you just wrote fails" today selects **all
tests of type=acceptance**, not the test the agent actually wrote. If
any other acceptance test in the file happens to pass, the verify-fail
step is meaningless; if any happens to fail for an unrelated reason,
the verify-pass step is meaningless. The inner-loop ATDD contract is
*exact-name* scoping, not category scoping. The CLI already supports
the exact-name shape via `--test name1,name2`. The missing piece is
threading the writer-agent's emitted `test_names` from `ctx.State`
into the verify call-activities.

## Dependency: 20260526-2118-cli-emit-output-channel.md

This plan depends on `plans/20260526-2118-cli-emit-output-channel.md`
landing first. 2118 makes `test_names` (already declared in
`acceptance-test-writer.md` and `outputs.go::knownOutputKeys`) reach
`ctx.State` reliably in **both** interactive and autonomous modes.
Without 2118, `ctx.State["test_names"]` is empty in interactive mode
and this plan's downstream consumer would have nothing to read.

2118 stops at "writer-agent output reaches ctx.State". This plan picks
up from there: ctx.State → BPMN params → CLI flags.

## Resolution

### BPMN vocabulary swap: `filter-type`/`filter-value` → `suite`/`test-names`

Replace the structured-filter vocabulary in every call-site:

| Today (broken) | After |
| --- | --- |
| `filter-type: test-type, filter-value: acceptance` | `suite: acceptance, test-names: ${test_names}` |
| `filter-type: test-type, filter-value: contract-real` | `suite: contract-real, test-names: ${test_names}` |
| `filter-type: test-type, filter-value: contract-stub` | `suite: contract-stub, test-names: ${test_names}` |
| `filter-type: test-type, filter-value: ${tests}` | `suite: ${tests}, test-names: ${test_names}` |

`suite` always pins the test category (it's a static property of which
writing agent ran — `acceptance-test-writer` writes to the acceptance
suite, `contract-test-writer` to the contract suite). `test-names`
carries the agent's emitted list, threaded through every hop as
`${test_names}` so the data dependency is visible in
`process-flow.yaml`.

Call-sites to edit (`process-flow.yaml`):
- `VERIFY_TESTS_PASS_ACCEPTANCE` (725-731)
- `VERIFY_TESTS_FAIL_ACCEPTANCE` (733-739)
- `VERIFY_TESTS_PASS_CONTRACT_REAL` (~880-885)
- `VERIFY_TESTS_FAIL_CONTRACT_STUB` (~887-893)
- `VERIFY_TESTS_PASS_CONTRACT_STUB` (~900-906)
- `VERIFY_TESTS_PASS_FILTERED` (1037-1042)
- `VERIFY_TESTS_FAIL_FILTERED` (1044-1050)
- The two `RUN_TESTS` nodes inside `verify-tests-pass` /
  `verify-tests-fail` (1090-1097 and 1130-1137) that re-pass the params
- `run-tests` itself (1676-1694) — `params:` shape changes to
  `command + suite + test-names`

`DISABLE_ACCEPTANCE_TESTS` (741-746) is **untouched**. It uses
`tests: acceptance` (the category, not a name list) and dispatches the
test-disabler agent, which still operates on a category. Disable is a
separate process from run-tests and never used `filter-type`/`filter-value`.

### Param substitution: `[]string` → comma-joined string

`ctx.State["test_names"]` is typed `[]string` (per
`outputs.go::knownOutputKeys` and coerced by 2118's CLI emitter on
write). `statemachine.ExpandParams` (run.go:316-328) substitutes
state values via `coerceStateValue` (run.go:334-346), whose default
branch is `fmt.Sprint(v)` — yielding `[foo bar]` (Go's bracket-and-
space slice format) for a `[]string`. That's not a usable CLI param.

Extend `coerceStateValue` to handle `[]string` explicitly:

```go
case []string:
    return strings.Join(t, ",")
```

That single switch case is the only engine change needed. After it,
`${test_names}` substitutes as `foo,bar`, which the CLI's `--test`
flag (`cmd.Flags().StringSliceVar(..., "test", ...)`) already accepts
as comma-separated input (test_commands.go:92 — "repeatable, also
accepts comma-separated values").

### `bindings.go::runCommand` rewire

Replace the `filter-type`/`filter-value` flag appending with
`suite`/`test-names`:

```go
// Today (716-721):
if filterType := ...; filterType != "" {
    cmd += " --filter-type=" + shellEscape(filterType)
}
if filterValue := ...; filterValue != "" {
    cmd += " --filter-value=" + shellEscape(filterValue)
}

// After:
if suite := strings.TrimSpace(ctx.Params["suite"]); suite != "" {
    cmd += " --suite=" + shellEscape(suite)
}
if testNames := strings.TrimSpace(ctx.Params["test-names"]); testNames != "" {
    cmd += " --test=" + shellEscape(testNames)
}
```

Both flags are independent and optional — omitted params produce no
flag, matching today's behaviour for unset filter params. `--test`
takes a comma-separated value (the joined `[]string` from the previous
step). `isTestRun` detection (715), `command-succeeded`/`test-outcome`
stamping (724-731), and the failure-diagnostic stamping (732-738)
all remain unchanged — they don't depend on the filter mechanism.

The doc-comment block at `bindings.go:691-695` (which currently
documents `filter-type`/`filter-value` as optional inputs) is rewritten
to reference `suite` / `test-names`.

### Parent MID `write-and-verify-acceptance-tests`

The parent MID at `process-flow.yaml:632-770` already passes
`tests: acceptance` and `expected-test-result: ${expected-test-result}`
into nested call-activities. After the swap, the same parent reads
nothing new — `${test_names}` is auto-pulled from `ctx.State` at
expansion time (it was written there by `WRITE_ACCEPTANCE_TESTS`'s
outputs handling, via 2118's JSONL channel).

The parent does **not** need to declare an explicit `test_names`
output capture — the dispatcher's `validateOutputsAndScopes` step
(post-2118) writes the value into `ctx.State` for any subsequent
expansion. The chain is:

```
WRITE_ACCEPTANCE_TESTS (writer agent emits test_names via CLI)
  ↓ dispatcher reads JSONL, writes ctx.State["test_names"]
COMPILE_TESTS
  ↓
GATE_EXPECTED_TEST_RESULT
  ↓
VERIFY_TESTS_PASS_ACCEPTANCE   { suite: acceptance, test-names: ${test_names} }
  ↓
RUN_TESTS (verify-tests-pass subprocess)
  ↓
EXECUTE_COMMAND   command="gh optivem test run", suite=acceptance, test-names=foo,bar
  ↓
runCommand → "gh optivem test run --suite=acceptance --test=foo,bar"
```

Same chain on the CT side via `write-and-verify-contract-tests`.

### Tests

New / updated:

- `internal/atdd/runtime/statemachine/run_test.go` — add a case for
  `coerceStateValue` with a `[]string` input asserting `"foo,bar"`
  output. Add an `ExpandParams` integration case verifying
  `${test_names}` substitutes from `ctx.State` as `foo,bar`.

- `internal/atdd/runtime/actions/bindings_test.go` — rename the
  existing `filter-type`/`filter-value` flag-passthrough tests to
  cover `suite`/`test-names`. Cover:
  - both unset → bare `gh optivem test run`
  - suite only → `--suite=acceptance`
  - test-names only → `--test=foo,bar`
  - both set → `--suite=acceptance --test=foo,bar`
  - shell-quoting of names containing whitespace or quotes (the test
    name comes from agent output, so defensive quoting matters)

- `internal/atdd/runtime/statemachine/process-flow_test.go` (if it
  exists; otherwise the BPMN-shape tests under
  `internal/atdd/runtime/statemachine/run_test.go`) — assert that the
  param keys on the swapped call-activities are now `suite` /
  `test-names`, not `filter-type` / `filter-value`. Catches regressions
  if someone reintroduces the old vocabulary.

- A rehearsal in autonomous mode against a real issue (per 2118's
  Item 8 pattern) confirming the dispatched command is
  `gh optivem test run --suite=acceptance --test=<actual-names>` and
  that only those tests run.

### Doc-comment updates

- `process-flow.yaml:84-90` — the `# - run-tests structured filter
  params (filter-type enum + filter-value, Q5.a resolved)` block needs
  rewriting to describe the new `suite` + `test-names` shape.
- `process-flow.yaml:1671-1675` — the `# Q5.a structured filter params`
  block above `run-tests` likewise.
- `bindings.go:691-695` — runCommand's doc-comment listing of
  `ctx.Params["filter-type"]` / `["filter-value"]`.

## Items

1. **Engine: `[]string` coerces to comma-joined string.** Extend
   `coerceStateValue` in `internal/atdd/runtime/statemachine/run.go`
   to handle `[]string` via `strings.Join(t, ",")`. Unit-test in
   `run_test.go`. This change is independent of 2118 — it can land
   on its own and only kicks in when something writes a `[]string`
   to `ctx.State`.

2. **`bindings.go::runCommand`: swap flag vocabulary.** Replace the
   `filter-type` / `filter-value` flag-appending block with
   `suite` / `test-names`. Rewrite the doc-comment at 691-695.
   Update / rename the corresponding flag-passthrough tests in
   `bindings_test.go`.

3. **`process-flow.yaml`: swap call-activity params.** Update every
   call-site listed in the table above to use `suite` /
   `test-names` (with `${test_names}` pulled from ctx.State at
   `verify-tests-pass` / `verify-tests-fail` invocations, and
   `${test-names}` re-piped through the nested `RUN_TESTS` nodes).
   Update the two doc-comment blocks (84-90 and 1671-1675). Adjust
   `process-flow_test.go` shape assertions if any reference the old
   param keys.

4. **Verify on a real cycle (requires 2118).** Run an end-to-end
   write-and-verify-acceptance-tests cycle against a rehearsal
   ticket in **both** interactive and autonomous modes. Confirm
   from the trace:
   - The dispatched command line includes `--suite=acceptance`
     and `--test=<actual-names-the-agent-emitted>`.
   - `--filter-type` / `--filter-value` no longer appear anywhere
     in the trace.
   - The verify-fail path correctly reports only the new test(s)
     failing, even if other acceptance tests are present in the
     suite (set up a fixture with one pre-existing passing test
     alongside the agent's new failing one).

## Out of scope

- **2118's territory.** This plan does not touch the
  agent-to-dispatcher output channel. Whether `test_names` arrives
  via fenced YAML (today) or `gh optivem output write` (post-2118)
  is irrelevant here — this plan starts from
  `ctx.State["test_names"]` being populated.
- **New writer-agent output keys.** `test_names` already exists in
  `outputs.go::knownOutputKeys` and in
  `acceptance-test-writer.md`'s prompt. Adding contract-side
  emission (if `contract-test-writer.md` doesn't yet emit
  `test_names`) is a parallel concern — gate it in Item 3 only as
  far as confirming the CT-side `${test_names}` expansion would
  resolve to a non-empty value at runtime; if not, raise it as a
  follow-up rather than expanding this plan's scope.
- **Renaming `test_names` for cross-tier reuse.** The key stays
  `test_names` for both AT and CT cycles; the suite scoping
  (`acceptance` vs `contract-real` vs `contract-stub`) already
  carries the tier distinction.
- **CLI changes.** `gh optivem test run`'s flags
  (`--suite`/`--test`/`--sample`/`--list`) are unchanged. The
  dispatcher now uses two flags it ignored before; nothing about
  the binary needs to change.
- **Disable-tests rewrite.** `DISABLE_ACCEPTANCE_TESTS` keeps
  category-scoping (`tests: acceptance`). Whether it should
  *also* narrow to names is a separate question — file it as a
  follow-up if the answer turns out to be yes.

## Open questions

None — every design decision is settled above. Plan is ready for
execution once 2118 is merged.
