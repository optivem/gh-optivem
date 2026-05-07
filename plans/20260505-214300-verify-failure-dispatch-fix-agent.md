# Verify-time test failures should not silently flow into human review

> **Status:** Items 1–6 landed. Manual rehearsal still owed.

## Symptom

Running an `AT - RED - SYSTEM DRIVER - WRITE` cycle (chore on a Page Object
helper, see sibling plan `20260505-220000-tracer-bridge-page-object-helpers.md`):

```
WARNING: tracer could not stage 1 adapter method(s) — running full suite for safety:
  - system-test/typescript/.../pages/NewOrderPage.ts::inputSku

$ gh optivem test system --suite acceptance-api
(test run failed: shell "gh optivem test system --suite acceptance-api":
  exit status 1
  (stderr: ERROR: read system config ./system.json: open ./system.json:
   The system cannot find the file specified.) — continuing)
$ gh optivem test system --suite acceptance-ui
(test run failed: ... — continuing)
$ gh optivem test system --suite contract-stub
(test run failed: ... — continuing)
[trace 21:42:23] OK VERIFY_STRUCT_DRIVER -> (no result)  (3m49.115s)
[trace 21:42:23] > STOP_STRUCT_REVIEW  kind=user_task agent=human

[STOP_STRUCT_REVIEW] STOP - HUMAN REVIEW — approve implementation
  Press Enter to continue, or type `abort` to halt:
```

All three suites errored before the test runner even started (cwd bug —
`./system.json` doesn't exist at repo root). The state machine logged
`OK VERIFY_STRUCT_DRIVER` and immediately offered the human an approve
prompt, as if verification had passed. The user is now staring at an
"approve implementation" gate with zero signal about whether the
implementation works.

## Root cause

`runVerifyCommand` (`internal/atdd/runtime/actions/bindings.go:910`)
prints failures and returns nothing:

```go
func (a actions) runVerifyCommand(cmd string) {
    fmt.Fprintf(a.deps.Stdout, "\n$ %s\n", cmd)
    out, err := a.deps.Shell.Run(context.Background(), cmd)
    if len(out) > 0 {
        fmt.Fprintln(a.deps.Stdout, string(out))
    }
    if err != nil {
        fmt.Fprintf(a.deps.Stderr, "(test run failed: %v — continuing)\n", err)
    }
}
```

The header comment at `bindings.go:907` codifies this as a design
decision:

> *"runVerifyCommand shells out and surfaces test failures as
> informational output — the action does not halt the state machine on
> test failure (per plan: verification is feedback, not gating)."*

That stance came from `20260504-130000-minimal-test-set-after-driver-adapter-change.md`,
where the original concern was *WRITE-phase verifies* (post-driver
edits in AT/CT cycles), and "feedback, not gating" made sense because
RED is expected during WRITE.

What it missed:

1. **Two failure classes are being conflated.** A real-red test run
   (the runner executes, a test fails) is feedback. A runner failure
   (the binary errors out before any test runs) is an *infrastructure*
   problem. The current code swallows both identically.
2. **`VERIFY_STRUCT_DRIVER` is structural.** Structural cycles (chore
   et al.) are by definition behaviour-preserving — RED is *not*
   expected; tests should stay green. Treating verify failure as
   advisory there hides regressions, which is exactly the case the
   structural cycle is supposed to catch before merge.
3. **No human-readable summary.** Even when it does surface the
   failure inline, the summary line at `[trace ...] OK VERIFY_STRUCT_DRIVER`
   says `OK`, which directly contradicts the inline output.

## Proposed mechanism

Three pieces, smallest first:

### A. Capture per-command outcomes

Make `runVerifyCommand` return a struct rather than nothing:

```go
type verifyCommandResult struct {
    Cmd      string
    ExitErr  error      // nil = success
    Stdout   string
    Stderr   string
    Class    failureClass // ok | infra | red
}
```

`failureClass` classification rules (kept in `internal/atdd/runtime/
actions/verify_classify.go`, table-driven so the patterns are reviewable
in one place):

- `ok`: `ExitErr == nil`.
- `infra`: stderr matches one of: `read system config`, `open .*\.json: The system cannot find the file`, `executable file not found`, `permission denied`, runner-specific "could not connect to docker", missing language toolchain. The runner never produced a test report.
- `red`: `ExitErr != nil` AND classification didn't match `infra`. Tests
  ran, at least one failed.

`runTracerVerify` and `runAffectedSetVerify` collect `[]verifyCommandResult`
and return them in `Outcome.Params` (or a new `Outcome.VerifyResults`
field — see Item 2 below).

### B. New state-machine node: `FIX_VERIFY_FAILURE`

Insert between `VERIFY_STRUCT_DRIVER` and `STOP_STRUCT_REVIEW` (and
analogously between `VERIFY_AT_DRIVER` / `VERIFY_CT_DRIVER` and their
respective successor nodes — though the WRITE-phase semantics differ;
see "Per-cycle policy" below):

```yaml
- id: VERIFY_STRUCT_DRIVER
  type: service_task
  action: verify_run_tests_after_driver

- id: GATE_STRUCT_VERIFY
  type: gateway
  binding: verify_failure_class
  description: "Verify outcome? (ok | infra | red)"

- id: FIX_STRUCT_VERIFY
  type: user_task
  agent: atdd-fix-verify
  description: "Dispatch fix agent on verify failure"
  retry_limit: 2

- id: STOP_STRUCT_REVIEW
  type: user_task
  agent: human
  role: review
```

Gateway routes:

- `ok` → `STOP_STRUCT_REVIEW` (current behaviour).
- `infra` → halt with a clear error: orchestrator misconfiguration is
  not for the user-facing fix agent to solve. Print the captured
  classification and which runner config path was tried (cwd, flag
  values) and exit non-zero. Surfaces the `system.json` cwd bug as a
  bug, not as a phantom approval.
- `red` → `FIX_STRUCT_VERIFY` once. After the fix attempt, re-enter
  `VERIFY_STRUCT_DRIVER`. Cap at one retry (chore should not need
  more); on second failure, halt with the captured reds and let the
  human take over.

### C. New agent: `atdd-fix-verify`

Add `internal/atdd/runtime/agents/prompts/atdd-fix-verify.md`. The
prompt is dispatched via the existing `clauderun.Dispatch` path
(`internal/atdd/runtime/clauderun/clauderun.go`). Inputs:

- `${verify_results}` — one block per failed command: the suite, the
  test (when known), the captured stderr.
- `${changed_files}` — same list `verifyRunTestsAfterDriver` already
  printed at the top of the cycle.
- `${phase_doc}` — the calling cycle's phase document, so the agent
  knows the structural-vs-WRITE policy ("if you're here, behaviour
  must be preserved" vs "RED is expected, fix only if obviously
  wrong").
- `${allowed_roots}` — same multi-line block the other agents already
  receive; restricts edits to the SUT.

The prompt's instructions:

1. Read each failed command's stderr. Decide whether the fix is in
   the system under test or in the test code.
2. Apply the smallest change that turns the failure green. Do not
   refactor.
3. Do not commit. The orchestrator stages and commits after re-verify.
4. Report the change set on stdout in the same shape `git status -s`
   produces, so the orchestrator can re-run verify against it.

## Per-cycle policy

`runVerifyCommand` is shared across three call sites:

- `VERIFY_STRUCT_DRIVER` (structural / chore). Behaviour-preserving by
  definition. **Policy:** infra → halt; red → one fix-agent retry → halt
  on second failure.
- `VERIFY_AT_DRIVER` (AT WRITE). RED is expected during WRITE.
  **Policy:** infra → halt; red → continue (current behaviour stays;
  feedback-not-gating still correct here). The fix agent does *not*
  fire — RED is the whole point.
- `VERIFY_CT_DRIVER` (CT WRITE — external driver). Same as
  AT_DRIVER. RED expected.

So Item B's gateway needs the calling cycle's policy. Surface it as a
new field on the verify action's outcome:

```go
type verifyOutcome struct {
    Class            failureClass
    StructuralCycle  bool   // true for VERIFY_STRUCT_DRIVER
    Results          []verifyCommandResult
}
```

The gateway dispatches the fix agent only when
`StructuralCycle && Class == red`, halts when `Class == infra`
regardless of cycle, and otherwise continues.

## Out of scope

- **WRITE-phase fix loop.** `VERIFY_AT_DRIVER` / `VERIFY_CT_DRIVER`
  keep current "feedback, not gating" semantics. RED is expected
  during WRITE; auto-fixing it would short-circuit the test → driver
  → DSL → SUT loop the cycle is designed to drive.
- **Retry > 1 for structural cycles.** A chore that needs two
  fix-agent passes to stay green is a chore that wasn't structural;
  surfacing that to the human is the right answer.
- **Fixing the cwd bug itself.** Sibling plan
  `20260505-220100-verify-runs-from-wrong-cwd.md` covers the
  immediate cause of the failures observed in this morning's trace.
  Land that first; this plan adds the *policy* for handling whatever
  failures remain after the cwd fix.
- **Tracer staging (the `inputSku` warning).** Sibling plan
  `20260505-220000-tracer-bridge-page-object-helpers.md`. Independent.
- **Replacing `clauderun` with the Task tool / parent-claude harness.**
  Existing `clauderun` is fine for dispatching the fix agent;
  redesigning the dispatch model is an unrelated v2 question.

## Manual rehearsal (still owed)

Reproduce the failing structural cycle, confirm: (a) infra failure
halts with the cross-link, (b) a real red routes to the fix agent,
(c) the fix agent's edits trigger a re-verify, (d) on green the
human sees `OK STOP_STRUCT_REVIEW` for real.
