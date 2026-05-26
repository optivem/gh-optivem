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

Verify-tests call-sites to edit (`process-flow.yaml`) — swap
`filter-type`/`filter-value` → `suite`/`test-names`:
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

Disable/enable call-sites — pass `test-names: ${test_names}` so the
test-disabler / test-enabler agents annotate only the writer's new
tests, not the whole suite. The agents already use bare names (Item 4
reshapes their prompts), so no `<file>:<method>` composition is
needed.
- `DISABLE_ACCEPTANCE_TESTS` (741-746): replace the dead
  `tests: acceptance` param (the subprocess never consumed it) with
  `test-names: ${test_names}`.
- `DISABLE_TESTS` inside `verify-tests-filtered` (~1052-1054): add
  `test-names: ${test_names}`.
- `ENABLE_TESTS` inside `verify-tests-filtered` (~1019-1021): add
  `test-names: ${test_names}`.

The disable-tests / enable-tests subprocesses themselves
(1405-1424, 1427-1446) don't need new params declared — the
agents read `${test_names}` directly from ctx.State at prompt
render time.

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

**Landing order.** Items 2, 4, and 5 must land in a single atomic
commit. Landing item 2 alone leaves the dispatcher reading params
the BPMN doesn't send (`gh optivem test run` with no flags → all
tests run). Landing item 5 alone leaves the BPMN sending params the
dispatcher ignores. Landing item 4 alone leaves the disable/enable
agents reading `${test_names}` that no call-site populates yet —
same empty-list brokenness disable has today. Items 1, 3, 6 are
individually safe and can land separately.

5. **`process-flow.yaml`: swap call-activity params.** Update every
   verify call-site listed in the table above to use `suite` /
   `test-names` (with `${test_names}` pulled from ctx.State at
   `verify-tests-pass` / `verify-tests-fail` invocations, and
   `${test_names}` re-piped through the nested `RUN_TESTS` nodes).
   Update the two doc-comment blocks (84-90 and 1671-1675).
   `process-flow_test.go` does not exist; the BPMN-shape tests live
   in `run_test.go`, and a grep confirms no current tests reference
   `filter-type` / `filter-value` in the statemachine package, so no
   shape-test edits are required.

   Also update the disable/enable call-sites to pass
   `test-names: ${test_names}`:
   - `DISABLE_ACCEPTANCE_TESTS` (741-746): replace the dead
     `tests: acceptance` param with `test-names: ${test_names}`.
   - `DISABLE_TESTS` inside `verify-tests-filtered` (~1052-1054):
     add `test-names: ${test_names}`.
   - `ENABLE_TESTS` inside `verify-tests-filtered` (~1019-1021):
     add `test-names: ${test_names}`.

6. ⏳ Deferred pending 2118 landing. **Verify on real cycles (requires 2118).** Run end-to-end
   `write-and-verify-acceptance-tests` **and**
   `write-and-verify-contract-tests` cycles against rehearsal
   tickets in **both** interactive and autonomous modes. Confirm
   from each trace:
   - The dispatched command line includes the right `--suite=…`
     (`acceptance` / `contract-real` / `contract-stub`) and
     `--test=<actual-names-the-agent-emitted>`.
   - `--filter-type` / `--filter-value` no longer appear anywhere
     in the trace.
   - The verify-fail path correctly reports only the new test(s)
     failing, even if other tests in the same suite are present
     (set up a fixture with one pre-existing passing test
     alongside the agent's new failing one — on both AT and CT
     sides).
   - When the verify-fail path triggers `DISABLE_ACCEPTANCE_TESTS`,
     only the new test(s) get the disable annotation — other
     acceptance tests in the file remain un-annotated. Symmetric
     check on the enable path in `verify-tests-filtered`.

## Out of scope

- **2118's territory.** This plan does not touch the
  agent-to-dispatcher output channel. Whether `test_names` arrives
  via fenced YAML (today) or `gh optivem output write` (post-2118)
  is irrelevant here — this plan starts from
  `ctx.State["test_names"]` being populated.
- **New writer-agent output keys.** No schema additions: `test_names`
  is already declared in `outputs.go::knownOutputKeys`. CT-side
  emission is covered by Item 3 (mirror the AT-side block in
  `contract-test-writer.md`); no other writer-agent prompts gain new
  outputs.
- **Renaming `test_names` for cross-tier reuse.** The key stays
  `test_names` for both AT and CT cycles; the suite scoping
  (`acceptance` vs `contract-real` vs `contract-stub`) already
  carries the tier distinction.
- **CLI changes.** `gh optivem test run`'s flags
  (`--suite`/`--test`/`--sample`/`--list`) are unchanged. The
  dispatcher now uses two flags it ignored before; nothing about
  the binary needs to change.
- **CT-side disable.** The test-disabler reason format hardcodes
  `AT` (`<TICKET-ID> - AT - <LOOP> - <PHASE>`), so today only AT
  files actually get disable markers even though the call-activity
  scope is forward-looking `[at-test, ct-test]`. Parameterising the
  cycle (`AT` vs `CT`) and wiring CT-side disable call-sites is a
  separate plan; this one only narrows the existing AT-side
  disable/enable flow.

## Open questions

None — every design decision is settled above. Plan is ready for
execution once 2118 is merged.
