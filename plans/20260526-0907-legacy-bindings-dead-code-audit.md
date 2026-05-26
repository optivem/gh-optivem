# Dead-code audit: pre-BPMN gate + action bindings

> **Cross-reference:** Spawned out of
> `plans/20260525-2311-kebab-case-everywhere.md` (kebab-everywhere flip).
> That plan flipped every snake-form identifier referenced by the *current*
> `process-flow.yaml` plus the one wired-through action (`pick-top-ready`).
> It deliberately left ~50 legacy snake identifiers in
> `internal/atdd/runtime/gates/bindings.go` and
> `internal/atdd/runtime/actions/bindings.go` untouched — they no longer
> appear in `process-flow.yaml` after the BPMN five-level refactor, so
> their fate is "delete as dead code" not "rename to kebab". This plan owns
> that audit + deletion pass.

## Origin / intent

User decision (2026-05-26, mid-execution of plan 2311 Item 4):

> "long term best and most correct" → flip everything in process-flow.yaml
> + its parser to kebab in this commit; **separately**, audit + delete the
> legacy bindings (don't bulk-rename dead code).

Renaming dead snake identifiers to dead kebab identifiers is wasted churn
and obscures which Phase-D bindings still need to be wired. Per
`feedback_renames_autonomous_content_gated` + `feedback_new_plan_not_extend`,
this dead-code cull is a separate refactor with a separate plan.

## Scope inventory (legacy bindings to audit)

### `gates/bindings.go` — RegisterAll registers ~30 gate bindings

Snake-named:

- `dsl_interface_changed`, `external_system_driver_interface_changed`,
  `system_driver_interface_changed`
- `ticket_type`, `subtype`, `change_type`, `ticket_type_recognized`,
  `subtype_ok`, `parse_ok`
- `legacy_acceptance_criteria_section_present`,
  `legacy_at_acceptance_criteria_present`,
  `legacy_ct_acceptance_criteria_present`,
  `legacy_at_verify_outcome`, `legacy_ct_verify_outcome`
- `refine_requested`, `refinement_changed`, `refactor_changed`
- `external_system_driver_exists`, `external_system_test_instance_accessible`,
  `smoke_test_passes`, `structural_test_mode`, `structural_verify_outcome`
- `compile_ok`, `tests_failed_runtime`, `tests_pass`
- `verify_real_required`, `verify_real_pass`, `tests_selected`
- `scope_exception_requested`, `phase_scope_clean`, `dsl_flags_present`

### `actions/bindings.go` — RegisterAll registers ~20 actions

Snake-named (excluding the already-kebab-flipped `pick-top-ready`):

- `move_to_in_progress`, `read_ticket_type`, `read_subtype`,
  `parse_ticket_body`, `materialize_parsed_concepts`,
  `report_intake_summary`, `move_to_in_acceptance`
- `run_smoke_test`
- `compile_all`, `compile_system`, `compile_system_tests`
- `commit_phase`, `tick_checklist`, `select_tests`, `build_system`,
  `start_system`, `run_tests`, `run_targeted_tests`,
  `verify_real_suite_passes`

(Re-grep at pickup; the BPMN refactor may have removed others since this
plan was written.)

## Audit procedure

For each binding/action in the inventory:

1. **Grep current `process-flow.yaml`** for the bare snake identifier in
   `binding:` / `action:` fields and `when:` predicates.
   - **Zero hits** → confirmed dead. Proceed to step 2.
   - **Any hit** → not dead. Decision: either rename the YAML reference to
     kebab + flip the `r.Register("…")` name (consistent with plan 2311's
     "one wired action" treatment of `pick-top-ready`), OR leave both as
     snake if the binding is mid-Phase-D-wiring. Flag for user review.

2. **Grep the rest of the codebase** for the snake identifier as a string
   literal (`"…"`):
   - Only hits inside `*_test.go` → likely covered by the registered name;
     deleting the binding + the test together is the right move.
   - Hits inside production `.go` files outside `gates/bindings.go` /
     `actions/bindings.go` → the binding is still wired into something.
     Investigate before deleting; may have to migrate the caller.

3. **Grep the `ctx.Set(...)` / `ctx.Get(...)` / `ctx.GetString(...)` call
   sites** for the binding's state-variable name (if different from the
   registered name — e.g. `change_type` is both a state var and a gate).
   - Confirms whether anyone actually reads the value back. Pure
     read-and-discard sites are safe to delete; sites that branch on the
     value are part of a flow that the BPMN refactor was supposed to
     replace (and may not have, fully).

4. **Decide per binding:**
   - **Delete** (zero current callers; tests delete too).
   - **Rename to kebab** (still referenced by current YAML or live caller —
     flip the registered name + every reference in lock-step).
   - **Defer** (Phase-D wiring is mid-flight; flag and stop).

## Test-side cleanup

`gates/bindings_test.go` and `actions/bindings_test.go` exercise the
legacy bindings via the registered names. Every deletion in production
removes the matching test entry; every kebab rename in production flips
the test fixture's `want` list correspondingly.

The `want` list at `actions/bindings_test.go` line 962-981 (currently 20
snake names) is the canary — if all surviving registrations after this
audit are kebab, the entire `want` list becomes kebab. If some survive as
snake (Phase-D-pending), the list is mixed and that's a smell to flag.

## What NOT to do

- Do **not** bulk-rename all 50 snake identifiers to kebab as a single
  pass. That's the "delete vs flip" decision plan 2311's Q5 deferred —
  it's a per-binding judgment.
- Do **not** delete a binding without confirming zero callers. The Phase-D
  downstream-alignment plan may register a new binding under the same
  name in the near future; deleting and re-registering is fine, but
  deleting without checking risks silently dropping an in-flight wire.
- Do **not** rename `process-flow.yaml` references in this plan. The YAML
  is fully kebab as of plan 2311 commit `5b72c40` — every snake identifier
  in the YAML is intentional (none should remain). If you find a snake
  reference in the YAML during this audit, it's a regression in plan 2311
  and should be fixed there, not here.

## Acceptance / verification

- `gh optivem implement` smoke-test on a shop config still parses
  `process-flow.yaml` without "binding not registered" / "action not
  registered" errors.
- `go test ./internal/atdd/runtime/...` passes on every commit.
- The legacy `want` list in `actions/bindings_test.go` shrinks (or flips
  to kebab) commensurately with the per-binding deletion decisions.

## Items deliberately deferred

- The corresponding Phase-D wiring of new BPMN bindings
  (`approval-outcome`, `outputs-and-scopes-valid`, `command-succeeded`,
  `ticket-kind`, `refactor-type-choice`, `dsl-port-changed`,
  `external-driver-ports-changed`, `system-driver-ports-changed`,
  `expected-test-result`, `test-outcome`, `fix-on-failure-enabled`,
  `validate-outputs-and-scopes`, `run-command`) — that's the downstream-
  alignment plan's scope.
