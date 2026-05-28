# Wire the infra/red classifier into `run-command` so infra failures stop masquerading as test reds

## Context

During the 2026-05-28 rehearsal of issue #71 ("Gift-wrap an order"), the
`atdd-rehearsal.sh` wrapper logged `[atdd-rehearsal] implement succeeded.`
even though the IMPLEMENT_TICKET end-of-process state dump contained:

```
command-exit-code=1
command-stderr-tail='C:\Program' is not recognized as an internal or external command,
                    operable program or batch file.
                    ERROR: suite latest - Acceptance (stub) - API: npx playwright test ... exit status 255
failure-kind=scope-diff
failing-task-name=implement-system
scope-violating-paths=system/db/migrations/V20260528113000__add_gift_wrap.sql
verify_results_text=...FAILED...
```

…alongside the contradictory `command-succeeded=true`, `test-outcome=pass`.

Two distinct issues compose to produce the false-success. **This plan
owns the structural one** (the classifier gap). The other two are
called out under [Related, separate plans](#related-separate-plans).

## Why the pipeline lied

`runCommand` (`internal/atdd/runtime/actions/bindings.go:738-763`) treats
every non-zero exit from `gh optivem test run` as `test-outcome=fail`,
regardless of whether the runner actually executed any tests:

```go
result, err := a.runShell(cmd)
succeeded := err == nil
ctx.Set("command-succeeded", succeeded)
if isTestRun {
    if succeeded {
        ctx.Set("test-outcome", "pass")
    } else {
        ctx.Set("test-outcome", "fail")   // <-- infra failure here is laundered as red
    }
}
```

That has two downstream effects in the BPMN process:

1. **`verify-tests-fail` phase** (`process-flow.yaml:1282-1317`):
   `test-outcome=fail` is the **expected** outcome →
   `GATE_TESTS_OUTCOME` routes to `VERIFY_FAIL_END` → the phase signals
   success even though no test ran. The pipeline silently advances.

2. **`verify-tests-pass` phase** (`process-flow.yaml:1245-1278`):
   `test-outcome=fail` is the **unexpected** outcome →
   `FIX_UNEXPECTED_FAILING_TESTS` is dispatched. The fix agent (which
   thinks a real assertion failed) edits production code chasing a
   ghost — in the #71 rehearsal it produced
   `V20260528113000__add_gift_wrap.sql` outside the implementer's
   declared scope, hence the `failure-kind=scope-diff` in the dump.

Both branches end with the IMPLEMENT_TICKET process walking cleanly to
`END`, so `driver.Run` returns nil and the rehearsal logs success.

## The fix already exists in-tree, just unwired

`internal/atdd/runtime/actions/verify_classify.go` defines a
fully-tested classifier (`classifyShellErr`) with patterns for:

- missing system config (cwd bug),
- **missing executable / "is not recognized as an internal or external command" / "command not found"** (exactly matches the #71 trace),
- permission denied,
- docker daemon unreachable,
- missing language toolchain.

`internal/atdd/runtime/actions/verify_classify_test.go` covers each
pattern, including the powershell and cmd.exe variants of the
"not recognized" message.

The TODO at `bindings.go:756-761` is explicit about where to wire it:

> // TODO: when reviving the infra/red classifier (see
> // verify_classify.go), this is the canonical hook point — route the
> // same (stdout, stderr) here through classifyShellErr and let the
> // gateway downstream of run-tests choose halt-on-infra vs
> // dispatch-on-red.

This plan executes that TODO.

## Design decision: a third gate value, not a new gate

`test-outcome` today is a two-state pass/fail string. Three options for
representing infra:

| Option                                                                                     | Pros                                                                                                                              | Cons                                                                                                                                                       |
|--------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **A.** Extend `test-outcome` to `pass / fail / infra`                                      | Single source of truth; every existing routing on `test-outcome` keeps working; one new sequence-flow per verify-tests-* process. | `testOutcome` gate today only accepts `pass`/`fail` and errors on unknown values (`bindings.go:285-289`) — must be widened.                                |
| **B.** Add a new `test-infra-failed` boolean gate, evaluated before `test-outcome`         | Surgical at the gate level.                                                                                                       | Two gates fire per run; every verify-tests-* sequence-flow gains a new fork; doubles the BPMN surface.                                                     |
| **C.** Treat infra as a hard `Outcome{Err}` from `runCommand`, halting the engine          | Loudest possible failure.                                                                                                         | Bypasses the BPMN entirely — no recovery, no log decoration, no end-event reached. Trace becomes opaque (the run dies mid-process, not at an end-event).   |

**Decision: A.** It composes with existing routing and matches the
classifier's own three-way `ok / red / infra` shape. The widened gate
needs one new case (`"infra"`) and two existing verify-tests-* processes
gain one new sequence-flow each to a `TESTS_INFRA_HALT` error-end-event.

## Items

### Item 1 — Wire `classifyShellErr` into `runCommand`

**File:** `internal/atdd/runtime/actions/bindings.go`

Replace the TODO block at lines 755-763 with the live classifier call.
The post-condition is that `test-outcome` carries one of three values
(`pass`, `fail`, `infra`) and that `verify_results_text` is still
stamped on `fail` and `infra` (the fixer prompts and the halt message
both consume it).

Shape:

```go
if isTestRun && !succeeded {
    class, label := classifyShellErr(string(result.Stderr), err)
    switch class {
    case classInfra:
        ctx.Set("test-outcome", "infra")
        ctx.Set("test-infra-label", label)   // for the halt message
        ctx.Set("verify_results_text", formatVerifyResults(result.Stdout, result.Stderr))
    case classRed:
        // test-outcome="fail" already set above
        ctx.Set("verify_results_text", formatVerifyResults(result.Stdout, result.Stderr))
    }
}
```

The pre-classifier `test-outcome="fail"` set at line 745 stays so the
red branch is unchanged; the infra branch overwrites with `"infra"`.

The combined-stream classification (stderr+stdout) is what `verify_classify.go:102-115`
expects — pass `result.Stderr` as the matcher input. If a runner only
emits the infra prefix on stdout, extend that hook to pass a joined
string; the matcher patterns are runner-prefix-anchored either way.

### Item 2 — Widen the `test-outcome` gate to accept `infra`

**File:** `internal/atdd/runtime/gates/bindings.go`

Extend the switch at lines 285-289 to allow `"infra"` alongside
`"pass"` and `"fail"`:

```go
switch s {
case "pass", "fail", "infra":
    return statemachine.Outcome{Value: s}
default:
    return statemachine.Outcome{Err: fmt.Errorf("test-outcome: unrecognised value %q (action stamped a value the gate does not handle)", s)}
}
```

The error message stays — drift between action and gate must still
surface loudly.

### Item 3 — Add an infra-halt end-event to both verify-tests-* processes

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

In **`verify-tests-pass`** (1245-1278) and **`verify-tests-fail`**
(1282-1317), add one node and one sequence-flow each:

```yaml
- id: TESTS_INFRA_HALT
  type: error-end-event
  name: "Test Runner Infra Failure"

# in sequence-flows:
- {from: GATE_TESTS_OUTCOME, to: TESTS_INFRA_HALT, when: "test-outcome == infra"}
```

`error-end-event` (precedent: `UNKNOWN_TESTS_OUTCOME` in the same
processes) bubbles the failure up the call-activity chain so
IMPLEMENT_TICKET, in turn, raises an error to `driver.Run`, in turn,
non-zero exit from the binary. That is the actual failure signal the
operator needs.

### Item 4 — Halt-message decoration in the trace

**File:** `internal/atdd/runtime/trace/trace.go`

When a node is `TESTS_INFRA_HALT`, the trace decorator should print a
human-readable banner instead of the generic end-event line, quoting
`test-infra-label` (set in Item 1) and the `command-stderr-tail`:

```
[trace HH:MM:SS] HALT TESTS_INFRA_HALT — infra failure: missing executable
                  command: gh optivem test run --suite=acceptance --test=...
                  stderr tail: 'C:\Program' is not recognized as an internal or external command,
                               ...
```

The operator should immediately see "the runner could not start" vs.
"a test assertion failed" without parsing the state dump.

### Item 5 — `run-tests` callers must NOT also dispatch the
`fix-command-failed` fixer on infra

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

`run-tests` (around line 2000+) sets `fix-on-failure-enabled=false` so
the inner `execute-command` FIX branch doesn't pre-empt the outer
`test-outcome` gate (documented at process-flow.yaml:2012-2020). That
contract is preserved by Item 1 — `command-succeeded=false` still
flows the same way; only `test-outcome` gains a new value. Verify
no other call-site special-cases infra/red downstream of `run-tests`.

### Item 6 — Test coverage

- `internal/atdd/runtime/actions/bindings_test.go` — add cases for
  `runCommand` when `isTestRun=true` and the shell error matches one
  of `verify_classify.go`'s infra patterns. Assert
  `test-outcome="infra"`, `test-infra-label="missing executable"`
  (etc.), and that `verify_results_text` is still stamped.
- `internal/atdd/runtime/gates/bindings_test.go` — extend the
  `testOutcome` table at line 315+ with an `"infra"` row that returns
  `Outcome{Value: "infra"}`.
- `internal/atdd/runtime/statemachine/transitions_test.go` — assert
  the new edge `GATE_TESTS_OUTCOME → TESTS_INFRA_HALT` exists in both
  `verify-tests-pass` and `verify-tests-fail`.
- `internal/atdd/runtime/statemachine/run_test.go` — end-to-end walk:
  stub `run-command` to return an infra-shaped stderr, expect the
  process to reach `TESTS_INFRA_HALT` and the engine to return an
  error.

### Item 7 — Re-run the issue #71 rehearsal as the acceptance check

After Items 1-6 land, re-dispatch `gh optivem rehearsal` on issue #71
in the same Windows environment where the original trace was captured.

Expected behaviour:

- The first `gh optivem test run` invocation still fails with the
  Windows path issue (this plan does not fix that — see [Related,
  separate plans](#related-separate-plans)).
- `runCommand` classifies the failure as `infra` ("missing executable").
- The verify-tests-* phase reaches `TESTS_INFRA_HALT`.
- IMPLEMENT_TICKET raises an error.
- `driver.Run` returns non-zero.
- `atdd-rehearsal.sh` logs `implement exited with rc=…`, not
  `implement succeeded`.

The trace decorator (Item 4) shows the halt banner with the label
("missing executable") and the stderr tail.

## Out of scope

- **The Windows path issue itself** (`'C:\Program' is not recognized`).
  This plan ensures the failure is *surfaced loudly*. The actual fix —
  whether a quoting bug in `internal/runner/runshell_windows.go`, an
  upstream npx/Node.js issue, or a malformed `PATH` on the operator's
  machine — is a separate investigation. The halt banner from Item 4
  will tell the operator exactly which runner failed and why, which is
  the prerequisite for that investigation. Track as
  `plans/upcoming/20260528-1210-investigate-windows-npx-quoting-in-test-runner.md`.
- **Stale diagnostic-state cleanup**. The keys `command-line`,
  `command-exit-code`, `command-stderr-tail`, `verify_results_text`,
  `failure-kind`, `scope-violating-paths`, `failing-task-name` are set
  only on failure and never cleared on subsequent success, so a final
  trace dump can show stale failure evidence next to a current success.
  This plan does not address that — once Item 4 lands, the halt banner
  is the primary failure surface and the dump is supplementary. Track
  as `plans/upcoming/20260528-1220-clear-stale-failure-payload-on-subsequent-success.md`
  if the dump's misleadingness keeps biting after this plan ships.
- **`fix-on-failure-enabled` semantics on non-test commands**. The
  classifier only fires for `isTestRun`. Other `run-command` callers
  (commit, build, etc.) keep today's binary `command-succeeded` shape
  and can still route to `fix-command-failed`. Adding infra-detection
  there is a worthwhile extension but requires its own gate work —
  defer until a non-test infra failure is observed in the wild.
- **The DB-migration scope widening for `implement-system`**. Covered
  by the in-flight plan
  `plans/20260528-1145-db-migrations-as-first-class-scope-key.md`. With
  the classifier wired, the fix-unexpected-failing-tests dispatch that
  *caused* the migration to be written ghost-chasing-style would never
  have fired in the first place — but the migration write is still
  legitimate work the implementer should be allowed to do directly, so
  that plan is still needed.

## Related, separate plans

| Plan                                                                                                             | Relationship                                                                                                                                       |
|------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| `plans/upcoming/20260528-1210-investigate-windows-npx-quoting-in-test-runner.md` (TBD)                           | Fixes the *specific* trigger from the #71 rehearsal. Independent: even if that fix lands first, the classifier is still required for other infra failure modes (docker down, postgres missing, etc.). |
| `plans/upcoming/20260528-1220-clear-stale-failure-payload-on-subsequent-success.md` (TBD)                        | Cosmetic improvement to the trace dump. Independent.                                                                                               |
| `plans/20260528-1145-db-migrations-as-first-class-scope-key.md`                                                  | Addresses the *consequence* of the false-success in the #71 rehearsal (the ghost-chasing fixer wrote a migration outside scope). Composable.       |

## Decisions

1. **Three-state `test-outcome` (Option A), not a new gate or a hard halt.** (Resolved 2026-05-28.)
   Composes with existing routing, matches the classifier's own ok/red/infra shape, single source of truth.
2. **Infra failure must be a process-level error, not a fixer-dispatch.** (Resolved 2026-05-28.)
   The fix agent cannot remediate "node not on PATH" or "docker daemon down" — those are operator-environment concerns. The right response is a loud halt with the runner's stderr quoted.
3. **The classifier stays runner-agnostic.** (Resolved 2026-05-28.)
   New infra patterns are added as rows in `infraPatterns`, not as new branches in `runCommand`. Keeps the action body small and the test surface stable.

## References

- `internal/atdd/runtime/actions/bindings.go:738-763` — the `runCommand` body and the TODO this plan executes.
- `internal/atdd/runtime/actions/verify_classify.go` — the pure-function classifier that already exists.
- `internal/atdd/runtime/actions/verify_classify_test.go` — the table-test that already covers the patterns this plan relies on.
- `internal/atdd/runtime/gates/bindings.go:276-290` — the `testOutcome` gate that needs the new `"infra"` case.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1245-1317` — the `verify-tests-pass` and `verify-tests-fail` processes that gain the new end-event.
- `internal/atdd/runtime/statemachine/process-flow.yaml:2010-2025` — the `run-tests` `fix-on-failure-enabled=false` contract that this plan preserves.
- `internal/runner/tests.go:256-279` — the `gh optivem test run` `runShell` that produced the Windows path error (and where any fix to the trigger itself would land).
- `internal/runner/runshell_windows.go` — the Windows `.bat`/`.cmd` quoting workaround that may need extending if Item 7 still shows the same npx failure.
- The originating trace: 2026-05-28 11:50:55, IMPLEMENT_TICKET end-of-process state dump from the issue #71 rehearsal.
