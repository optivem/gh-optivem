# BPMN Phase D — wire the 11 new gateway bindings + 2 new actions

> **Cross-references.** This plan picks up the **Phase D** work that the
> BPMN refactor design archive
> (`plans/20260525-1057-bpmn-refactor-design.md`) and the YAML/diagrams
> execution plan (retired in commit `f1f1f7b`) explicitly deferred:
> binding the new gateway names and service-task action names that
> appear in `internal/atdd/runtime/statemachine/process-flow.yaml`
> after the five-level migration (commit `61dc365`). The runtime-rewiring
> plan (commit `fc0a139` / `6c4b1f8`) handled prompt renames and test
> fixtures, **not** the binding wiring; that gap is what this plan closes.
>
> Per memory `feedback_new_plan_not_extend` this is a fresh plan, not an
> edit to the existing BPMN plans. Per `feedback_resolve_questions_upfront`
> every open semantic question is resolved inline below (no
> "decide-at-encoding-time" TODOs).

## Why this plan exists

Running `gh optivem implement --issue 61 --config gh-optivem-monolith-typescript.yaml`
against the rehearsal worktree produces:

```
ERROR: driver: bind engine: 17 bind error(s):
  - process "execute-agent" node "GATE_OUTPUTS_AND_SCOPES_VALID": gateway binding "outputs_and_scopes_valid" not registered
  - process "execute-command" node "RUN_COMMAND": service_task action "run_command" not registered
  ... (15 more)
```

The 17 errors collapse into **11 unique gateway bindings** + **2 unique
service-task actions** that the new YAML references but `internal/atdd/runtime/gates/bindings.go`
and `internal/atdd/runtime/actions/bindings.go` do not register. Today
the binary refuses to start any process containing one of these nodes;
that's every TOP / HIGH / MID / LOW process in the new YAML.

## Out of scope

- **Prompt content.** The 13 missing items are runtime wiring only; agent
  prompt bodies under `internal/assets/runtime/prompts/atdd/` are handled
  by the prompt-renames work that already landed.
- **Removing old bindings/actions.** `dsl_interface_changed`,
  `external_system_driver_interface_changed`, `system_driver_interface_changed`,
  `compile_ok`, `tests_failed_runtime`, `tests_pass`, `read_ticket_type`,
  `read_subtype`, `parse_ticket_body`, etc. stay registered for now. The
  old YAML shape is gone, but the registries are tolerant of extras and
  the dead bindings can be swept in a follow-up once the new shape is
  proven on a full ticket.
- **Kebab-case migration of binding/output key vocabulary.** Tracked in
  `plans/20260525-2311-kebab-case-everywhere.md`. **This plan accepts
  the current asymmetry**: `binding:` names in YAML stay snake (matching
  the registry key), and the ctx key the binding *reads* matches what
  the agent emits (today kebab — e.g. `dsl-port-changed`). If the kebab
  plan flips `binding:` names later, the registry strings change in one
  place; the binding bodies keep reading the kebab agent outputs.
- **`fix-*` agent prompt inventory.** The `fix.EXECUTE_AGENT` node uses
  `task-name: "fix-${failure-kind}"`; the closed set of `failure-kind`
  values and matching prompt files lives in the design-archive
  exploration backlog and is tracked separately. This plan dispatches
  `validate_outputs_and_scopes` to emit a `failure-kind` value; what
  the inner `fix` does with it is downstream.

## Resolved up-front design questions

> Each item below is a Q the YAML left open. Resolving here so execution
> doesn't stall.

**Q-D1. ctx-key vocabulary for `outputs:` block.** The
existing `clauderun.ParseOutputs` flattens the agent's `outputs:` YAML
inner map straight into ctx.State with **verbatim inner keys** (commit
`d680e74` shape). The new prompts emit kebab (`dsl-port-changed`,
`system-driver-ports-changed`, `external-driver-ports-changed`), so the
new bindings **read kebab ctx keys** in their body. The binding *name*
registered in Go and referenced by `binding:` in YAML stays snake
(`dsl_port_changed`). Asymmetric but unambiguous; will collapse once the
kebab-everywhere plan runs.

**Q-D2. `approval_outcome` exit semantics.** The existing
`newHumanStopDispatcher` (driver.go:728) returns `Outcome{Err: …}` on
reject — the engine halts. The new `approve` LOW expects reject to route
to `APPROVE_REJECT_END` (graceful). **Resolution:** keep `newHumanStopDispatcher`
unchanged (other STOP sites still want hard halt) and add a sibling
dispatcher `newApproveDispatcher` that the wrapper installs *only* when
the user_task is `ASK_HUMAN` inside the `approve` process. The new
dispatcher writes `ctx.State["approval_outcome"] = "approved"|"rejected"`
and returns `Outcome{}` either way. The `approval_outcome` gate binding
then reads ctx and routes.

**Q-D3. `ticket_kind` composition strategy.** Old YAML had three
service tasks (`read_ticket_type` / `read_subtype` / `parse_ticket_body`)
each producing one ctx key, plus three gateways. New YAML collapses all
of them into a single binding that returns one discriminator string.
**Resolution:** the binding body calls the existing `readTicketType` +
`readSubtype` action implementations (lifted into helper funcs) and
composes the discriminator per the design-locked lookup table:

| ticket_type | subtype                  | discriminator                  |
| ---         | ---                      | ---                            |
| story       | (any/none)               | `story`                        |
| bug         | (any/none)               | `bug`                          |
| task        | `cover-legacy`           | `task/cover-legacy`            |
| task        | `redesign-system`        | `task/redesign-system`         |
| task        | `refactor-system`        | `task/refactor-system`         |
| task        | `refactor-tests`         | `task/refactor-tests`          |
| task        | `onboard-external-system`| `task/onboard-external-system` |

Unrecognised combinations return `Outcome.Err` (the runtime's
no-edge-matched error then surfaces the same way as today).

**Q-D4. `expected_test_result` and `fix_on_failure_enabled`
read Params, not State.** Both values are set by the call_activity
caller via the `params:` map (e.g. `expected-test-result: failure`,
`fix-on-failure: "false"`). The existing `boolGate` / `outcomeFromBoolish`
helpers read `ctx.State` only. **Resolution:** these two bindings read
`ctx.Params[…]` directly and return the value verbatim (string for
`expected_test_result`, boolean coercion for `fix_on_failure_enabled`).
No prompt fallback — these are structural metadata of the call site,
not runtime decisions the user re-makes.

  - `expected_test_result` default: empty → halt with a clear error
    (caller forgot to pin the param).
  - `fix_on_failure_enabled` default: missing param → `true`. The only
    caller that explicitly sets `false` is `fix.EXECUTE_AGENT` (the
    recursion-bound shape).

**Q-D5. `run_command` shape.** The YAML node passes templated params
(`command`, `filter-type`, `filter-value`) via `call_activity.params:`,
which the engine already expands against the parent scope before pushing
the child scope. By the time `RUN_COMMAND` fires, `ctx.Params["command"]`
is the final command string. **Resolution:**

  - Reads `ctx.Params["command"]` as the bash command line.
  - Appends `--filter-type=…` / `--filter-value=…` flags **only when
    those params are present and non-empty**. Today only `run-tests`
    sets them; `compile` / `commit` / `build-system` / `start-system`
    leave them empty and the flags are omitted.
  - Shells out via `actions.Deps.Shell.Run` (bash-style CommandLine,
    matching the existing `runCompile`/`runShell` pattern).
  - Streams stdout/stderr to `actions.Deps.Stdout` / `Stderr`
    (the operator-visible channel the rest of the runtime uses).
  - Writes `ctx.State["command_succeeded"] = (exitCode == 0)`.
  - **For the specific command `gh optivem run-tests`**, also writes
    `ctx.State["test_outcome"] = "pass"|"fail"` (per-test-suite
    classification is downstream; the binary success/failure of the
    overall command is enough for the cycle's `verify-tests-pass` /
    `verify-tests-fail` gates today). The classifier is a substring
    match on the resolved command string starting with `gh optivem run-tests`.
  - Does **not** set `Outcome.Err` on command failure — the cycle's
    `command_succeeded == false → CALL_FIX` edge is the intended consumer.

**Q-D6. `validate_outputs_and_scopes` shape.** Called from
`execute-agent` after `RUN_AGENT` fires. Reads:

  - `ctx.Params["outputs"]` — comma-separated list of expected output
    keys (e.g. `"dsl-port-changed"` or `"system-driver-ports-changed,external-driver-ports-changed"`).
    Empty → no output expectations.
  - `ctx.Params["scopes"]` — comma-separated Family B scope tokens
    (e.g. `"at-test,dsl-port,dsl-core"`). Empty → skip scope check
    (the `update-ticket`/`refine-acceptance-criteria` MIDs do not declare scopes).
  - Agent's parsed outputs are already in ctx.State from
    `clauderun.ParseOutputs` (driver.go:849).
  - Working-tree diff via `git status --porcelain` (one-shot, scoped to
    `RepoPath`).

  Validation:
  1. Every declared output key is present in ctx.State (key set, not
     value-shape).
  2. Every dirty path falls within one of the resolved scope roots
     (joined from `internal/atdd/phase-scopes.yaml` + `gh-optivem.yaml`
     `paths:` — the same join `check_phase_scope` already does).

  Writes:
  - `ctx.State["outputs_and_scopes_valid"]` = bool.
  - On false: `ctx.State["failure-kind"]` = one of `missing-output` /
    `scope-diff` (one-of, deterministic priority: missing-output first).
    Consumed by the downstream `fix.${failure-kind}` task-name derivation.

  Does **not** set `Outcome.Err` — the cycle's `GATE_OUTPUTS_AND_SCOPES_VALID`
  routes the false branch.

**Q-D7. `refactor_type_choice` menu values.** Always prompts the
operator (loopable menu — the BPMN loops back, the binding itself
prompts once per call). Accepts:
`refactor-system-structure` | `refactor-test-structure` | `redesign-system-structure` | `none`.
Reads `ctx.State["refactor_type_choice"]` first for hand-debugging /
preseed; otherwise prompts. Empty answer → `none` (exit the loop).

**Implementation shape:** mirrors the existing `structuralTestMode`
binding (`internal/atdd/runtime/gates/bindings.go:403`) — inline
`b.deps.Prompter.Ask` → `strings.ToLower` + `strings.TrimSpace` →
switch on the four enum values → `Outcome{Value: answer}`. Unrecognised
input → `Outcome.Err` (matching `structuralTestMode`). No new
`promptio` helper — the package has only y/n primitives today and the
inline pattern is the established shape for enum menus across gates
(see also `ticketType` at line 281, plus the two prompts at lines 304
and 330).

## Items

> Standard execution rhythm: each item is a one-shot edit + a unit-test
> add. Commit after each item; do not batch.

### Item 1 — Wire `command_succeeded` + `run_command` (the unblock-anything path)

`run-tests`, `compile`, `commit`, `build-system`, `start-system` all
funnel through `execute-command`, so this pair unblocks the most YAML.

- `internal/atdd/runtime/actions/bindings.go`:
  - Add `runCommand` per Q-D5. Reuse `actions.Deps.Shell.Run` + the
    existing `runShell` banner.
  - Register `r.Register("run_command", a.runCommand)`.
- `internal/atdd/runtime/gates/bindings.go`:
  - Add `commandSucceeded` binding: reads `ctx.State["command_succeeded"]`
    (bool), halts with `Outcome.Err` if unset (the upstream action must
    have run).
  - Register `r.Register("command_succeeded", b.commandSucceeded)`.
- Tests:
  - `actions/bindings_test.go`: golden test with a fake `Shell` that
    captures the command line and returns `(nil, nil)` / `(nil, error)`.
    Assert `ctx.State["command_succeeded"]` + (for `gh optivem run-tests`)
    `ctx.State["test_outcome"]`.
  - `gates/bindings_test.go`: state seeded with each polarity + the
    unset-halt case.

### Item 2 — Wire `test_outcome`

Pure read of `ctx.State["test_outcome"]` (string `pass`|`fail`). Halt
with `Outcome.Err` on unset (upstream `run_command` must have run with
the `gh optivem run-tests` prefix). Unrecognised value halts with a
clear "action stamped a value the gate does not handle" error, mirroring
`structuralVerifyOutcome`.

- `gates/bindings.go`: add `testOutcome` binding + register.
- `gates/bindings_test.go`: seeded-state cases.

### Item 3 — Wire `expected_test_result` and `fix_on_failure_enabled`

Both read `ctx.Params[…]` per Q-D4.

- `gates/bindings.go`:
  - `expectedTestResult`: returns `Outcome{Value: ctx.Params["expected-test-result"]}`;
    empty → halt with "caller did not pin expected-test-result".
  - `fixOnFailureEnabled`: reads `ctx.Params["fix-on-failure"]`; empty
    → default `true`; coerce via `promptio.ParseYN`.
  - Register both.
- Tests: param-seeded cases, default case, malformed case.

### Item 4 — Wire `dsl_port_changed`, `system_driver_ports_changed`, `external_driver_ports_changed`

All three are agent-output flags. Per Q-D1 the binding body reads the
kebab ctx key the agent emits:

- `dsl_port_changed` reads `ctx.State["dsl-port-changed"]`.
- `system_driver_ports_changed` reads `ctx.State["system-driver-ports-changed"]`.
- `external_driver_ports_changed` reads `ctx.State["external-driver-ports-changed"]`.

Each missing/unset → halt with `Outcome.Err` (the writing-agent prompts
list these in `outputs:` precisely so unset is a bug — same doctrine as
the existing `dsl_flags_present` gate). Boolean coercion via the
existing `outcomeFromBoolish` helper.

- `gates/bindings.go`: three bindings + register.
- Tests: each polarity, missing-key halt.

### Item 5 — Wire `refactor_type_choice`

Per Q-D7. Prompt-driven enum. Reuse the existing `Prompter` / `boolGate`
pattern but for a fixed string menu (not yes/no). Empty reply → `none`.

- `gates/bindings.go`: add `refactorTypeChoice`, register.
- Tests: state-preseed (each value) + prompt fake (each value).

### Item 6 — Wire `approval_outcome` + `newApproveDispatcher`

Per Q-D2.

- `internal/atdd/runtime/driver/driver.go`:
  - Add `newApproveDispatcher(opts, raw, nodeID)` that prints the
    `${question}` (already expanded), calls `promptio.ConfirmYN`, writes
    `ctx.State["approval_outcome"] = "approved"|"rejected"`, returns
    `Outcome{}`.
  - In `wrapAgentDispatchers` (`driver.go:693`) add a new switch case
    **before** the existing `case raw.Agent == "human":` (line 705)
    that matches `process.Name == "approve" && raw.Agent == "human"`
    and installs `newApproveDispatcher` instead of
    `newHumanStopDispatcher`. The `process` variable from the outer
    `for _, process := range eng.Processes` loop is already in scope
    (verified: `statemachine.Process.Name` exists at `types.go:69`),
    so no extra plumbing is needed.
- `internal/atdd/runtime/gates/bindings.go`:
  - Add `approvalOutcome`: returns `Outcome{Value: ctx.GetString("approval_outcome")}`;
    empty → halt.
  - Register.
- Tests:
  - `driver_test.go`: approve dispatcher writes both polarities; reject
    routes graceful (no Err).
  - `gates/bindings_test.go`: each polarity + unset halt.

### Item 7 — Wire `validate_outputs_and_scopes` + `outputs_and_scopes_valid`

Per Q-D6.

- `internal/atdd/runtime/actions/bindings.go`:
  - Add `validateOutputsAndScopes` action body. Reuse `checkPhaseScope`'s
    path-resolution helper (lift into a shared func if necessary) so the
    scopes→allowed-roots join logic is not duplicated.
  - Register `r.Register("validate_outputs_and_scopes", a.validateOutputsAndScopes)`.
- `internal/atdd/runtime/gates/bindings.go`:
  - Add `outputsAndScopesValid` binding: reads
    `ctx.State["outputs_and_scopes_valid"]` (bool); unset → halt
    "action did not run".
  - Register.
- Tests:
  - `actions/bindings_test.go`: missing-output case, scope-diff case,
    both-clean case. Assert `failure-kind` value on failure.
  - `gates/bindings_test.go`: seeded each polarity + unset.

### Item 8 — Wire `ticket_kind`

Per Q-D3. The composition can lift `readTicketType` and `readSubtype`
bodies into private helpers without changing their behaviour (they
already exist as actions).

- `internal/atdd/runtime/gates/bindings.go`:
  - Add `ticketKind` binding that:
    1. Reads `ctx.State["ticket_kind"]` first (preseed/hand-debug). Return as-is.
    2. Else: invoke the existing classify code path (ticket_type via
       tracker, then `subtype:*` label if `ticket_type == task`).
    3. Compose per Q-D3 table.
    4. Unrecognised → `Outcome.Err`.
  - Register.
- Tests:
  - Each row of the Q-D3 table, plus unrecognised composition halt.
  - Preseeded `ctx.State["ticket_kind"]` short-circuit.

  Tracker access: `gates.Deps.Tracker` is already fully wired
  (`gates/bindings.go:55`, populated in `withDefaults` at lines 86–98,
  consumed by `legacyAcceptanceCriteriaSectionPresent` at line 472).
  `b.ticketKind` calls `b.deps.Tracker` directly — no new `Deps` field,
  no shared helper package. The `readTicketType` / `readSubtype`
  classify shapes from `actions/bindings.go` are lifted into private
  helpers in the gates package (or a small internal subpackage if the
  imports get awkward); the lift is behaviour-preserving so the
  existing actions can be left alone for now and swept in the
  follow-up old-binding cleanup mentioned in "Out of scope".

### Item 9 — Smoke-run + commit

Run `bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 --config gh-optivem-monolith-typescript.yaml`
in the rehearsal worktree and confirm the binary now reaches the first
`approve` STOP (`Approve task update-ticket to run?`) instead of erroring
at bind time. We are not asserting end-to-end ticket completion — just
that the bind errors are gone and a normal run can start.

If new bind errors surface (the runtime sometimes uncovers a
second-order missing binding only once the first wave is fixed), append
items rather than editing existing ones.

Commit message: `atdd/runtime: BPMN Phase D — bind the 11 new gates + 2 new actions`.

## Walking-back guidance

If any item turns out to need a different shape than Q-D{1..7}
prescribes, treat that as a finding and either (a) update this plan's
"Resolved up-front design questions" inline with the new resolution and
re-derive the affected item, or (b) carve a fresh plan if the change is
disruptive enough to need its own scope review. Do not silently drift
the implementation from the resolution table.
