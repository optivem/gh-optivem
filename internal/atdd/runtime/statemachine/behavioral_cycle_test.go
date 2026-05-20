package statemachine

import (
	"testing"
)

// seedBehavioralIntake sets the routing state that intake would derive for a
// Story / Bug ticket — the half of the Context that's identical across every
// behavioral-cycle variant. Per-variant gate state (dsl_interface_changed
// etc.) is set inline by the calling test so the variation stays visible at
// the call site.
func seedBehavioralIntake(ctx *Context) {
	ctx.Set("ticket_type", "story")
	ctx.Set("change_type", "behavioral")
	ctx.Set("ticket_type_recognized", true)
	ctx.Set("parse_ok", true)
	ctx.Set("legacy_acceptance_criteria_section_present", false)
	// backlog_refinement: refiner is a no-op (refinement_changed=false)
	// so the sub-process discharges through GATE_REFINEMENT_CHANGED →
	// BR_END without dispatching UPDATE_TICKET.
	ctx.Set("refinement_changed", false)
	// red_phase_cycle gates (shared by every AT - RED - * dispatch):
	// compile passes; AT phases don't verify against a real suite
	// (verify_real_required=false routes straight to RUN); the desired
	// RED outcome is a runtime failure (tests_failed_runtime=true) which
	// routes to DISABLE → COMMIT.
	ctx.Set("compile_ok", true)
	ctx.Set("verify_real_required", false)
	ctx.Set("tests_failed_runtime", true)
	// green_phase_cycle gates (collapsed AT_GREEN dispatch):
	// tests_pass=true ends the happy path at GREEN_END.
	// compile_ok is already set above and is reused.
	ctx.Set("tests_pass", true)
	// at_refactor_system GATE_REFACTOR_CHANGED: refactor agent emitted the
	// `refactor_changed` flag; happy path walks through COMMIT.
	ctx.Set("refactor_changed", true)
	// Phase-scope enforcement gates (red_phase_cycle + green_phase_cycle,
	// per plan 20260518-1144 items 5 + 6):
	//   - scope_exception_requested defaults to false (no scope_exception
	//     signal from the agent) → spy's Get returns nil → Outcome{} →
	//     routes through the happy path without needing an explicit seed,
	//     but pinned here so the gate's contract is visible at the seed
	//     site.
	//   - phase_scope_clean MUST be set true; the spy GateFn reads
	//     ctx[binding] directly, so an unset value coerces to false and
	//     routes to STOP_SCOPE_VIOLATION → WRITE, deadlocking the cycle
	//     under the iteration cap.
	ctx.Set("scope_exception_requested", false)
	ctx.Set("phase_scope_clean", true)
	// Post-RED-DSL flag-presence validation gateway (at_cycle, plan
	// 20260518-1144 item 4). Same fixture rule as phase_scope_clean:
	// unset coerces to false and routes to STOP_FLAG_UNSET → AT_RED_DSL,
	// deadlocking any test that walks dsl_interface_changed=true.
	ctx.Set("dsl_flags_present", true)
}

// behavioralIntake walks the implement-ticket entry through github_intake,
// run_legacy_cycle, and into run_cycle → at_cycle for a Story/Bug ticket.
// Story tickets skip CLASSIFY_TICKET_SUBTYPE (GATE_TICKET_TYPE_INTAKE
// routes story → READ_TICKET_BODY directly). Ends with at_cycle scope
// active so the calling test can append the variant's at_cycle body.
func (e *expectDispatch) behavioralIntake() *expectDispatch {
	return e.process("main", noParams()).
		serviceTask("MOVE_TICKET_IN_PROGRESS", "move_to_in_progress").
		callActivity("INTAKE", "github_intake", noParams()).
		process("github_intake", noParams()).
		serviceTask("CLASSIFY_TICKET_TYPE", "read_ticket_type").
		gateway("GATE_CLASSIFY_CONFIDENT", "ticket_type_recognized", true).
		gateway("GATE_TICKET_TYPE_INTAKE", "ticket_type", "story").
		serviceTask("READ_TICKET_BODY", "parse_ticket_body").
		gateway("GATE_PARSE_OK", "parse_ok", true).
		serviceTask("REPORT_TICKET_DETAILS", "report_intake_summary").
		endEvent("INTAKE_END").
		process("main", noParams()).
		callActivity("RUN_LEGACY_CYCLE", "run_legacy_cycle", noParams()).
		process("run_legacy_cycle", noParams()).
		gateway("GATE_LEGACY_PRESENT", "legacy_acceptance_criteria_section_present", false).
		endEvent("RUN_LEGACY_END").
		process("main", noParams()).
		callActivity("BACKLOG_REFINEMENT", "backlog_refinement", noParams()).
		process("backlog_refinement", noParams()).
		userTask("BACKLOG_REFINEMENT", "refine-acc").
		userTask("CONFIRM_REFINEMENT", "human").
		gateway("GATE_REFINEMENT_CHANGED", "refinement_changed", false).
		endEvent("BR_END").
		process("main", noParams()).
		callActivity("RUN_CYCLE", "run_cycle", noParams()).
		process("run_cycle", noParams()).
		gateway("GATE_CHANGE_TYPE", "change_type", "behavioral").
		callActivity("AT_CYCLE", "at_cycle", noParams()).
		process("at_cycle", noParams())
}

// behavioralTail asserts the closing trail after at_cycle returns: the
// at_cycle end event, run_cycle end event, then main's MOVE TO IN
// ACCEPTANCE + main end event.
func (e *expectDispatch) behavioralTail() *expectDispatch {
	return e.process("at_cycle", noParams()).
		endEvent("AT_END").
		process("run_cycle", noParams()).
		endEvent("CYCLE_END").
		process("main", noParams()).
		serviceTask("MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance").
		endEvent("END")
}

// redCycle asserts one red_phase_cycle dispatch on the shared happy path:
// the call_activity entry in the caller's scope, then WRITE → Layer 1
// scope-exception gateway (no signal → STOP_RED_REVIEW) → COMPILE
// (call_activity into shared `compile` sub-process, compile_ok=true returns
// cleanly) → GATE_VERIFY_REAL_REQUIRED(false) → RUN →
// GATE_RUN_FAILED_RUNTIME(true) → DISABLE → Layer 2 scope check (clean →
// continue) → inner commit, then the red end event. params is the raw.Params
// declared at the call site (also the merged scope inside, since callers run
// with a noParams parent for red_phase_cycle dispatches today). callerNodeID
// is the call_activity ID in the parent process. The helper restores the
// caller's scope on exit so the chain continues cleanly.
func (e *expectDispatch) redCycle(callerNodeID string, params map[string]string) *expectDispatch {
	parentProc, parentParams := e.proc, e.params
	return e.callActivity(callerNodeID, "red_phase_cycle", params).
		process("red_phase_cycle", params).
		userTask("WRITE", params["agent"]).
		gateway("GATE_SCOPE_EXCEPTION", "scope_exception_requested", false).
		userTask("STOP_RED_REVIEW", "human").
		callActivity("COMPILE", "compile", compileFromCycleTemplateParams()).
		process("compile", compileFromCycle(params)).
		serviceTask("COMPILE", params["compile_action"]).
		gateway("GATE_COMPILE_OK", "compile_ok", true).
		endEvent("COMPILE_END").
		process("red_phase_cycle", params).
		gateway("GATE_VERIFY_REAL_REQUIRED", "verify_real_required", false).
		serviceTask("RUN", "run_targeted_tests").
		gateway("GATE_RUN_FAILED_RUNTIME", "tests_failed_runtime", true).
		userTask("DISABLE", "disable-tests").
		serviceTask("CHECK_PHASE_SCOPE", "check_phase_scope").
		gateway("GATE_PHASE_SCOPE_CLEAN", "phase_scope_clean", true).
		callActivity("COMMIT", "commit", commitFromTemplateParams()).
		process("commit", commitFrom(params)).
		userTask("APPROVE_COMMIT", "human").
		serviceTask("EXECUTE_COMMIT", "commit_phase").
		endEvent("COMMIT_END").
		process("red_phase_cycle", params).
		endEvent("RED_END").
		process(parentProc, parentParams)
}

// greenCycle asserts one green_phase_cycle dispatch on the shared happy
// path: WRITE → Layer 1 scope-exception gateway (no signal → COMPILE) →
// COMPILE (call_activity into shared `compile` sub-process, compile_ok=true
// returns cleanly) → RUN → GATE_TESTS_PASS(true) → Layer 2 scope check
// (clean → GREEN_END). Commit is owned by the parent (at_green_system
// commits backend + frontend together), so the sub-process ends here.
// Restores the caller's scope on exit.
func (e *expectDispatch) greenCycle(callerNodeID string, params map[string]string) *expectDispatch {
	parentProc, parentParams := e.proc, e.params
	return e.callActivity(callerNodeID, "green_phase_cycle", params).
		process("green_phase_cycle", params).
		userTask("WRITE", params["agent"]).
		gateway("GATE_SCOPE_EXCEPTION", "scope_exception_requested", false).
		callActivity("COMPILE", "compile", compileFromCycleTemplateParams()).
		process("compile", compileFromCycle(params)).
		serviceTask("COMPILE", params["compile_action"]).
		gateway("GATE_COMPILE_OK", "compile_ok", true).
		endEvent("COMPILE_END").
		process("green_phase_cycle", params).
		serviceTask("RUN", "run_targeted_tests").
		gateway("GATE_TESTS_PASS", "tests_pass", true).
		serviceTask("CHECK_PHASE_SCOPE", "check_phase_scope").
		gateway("GATE_PHASE_SCOPE_CLEAN", "phase_scope_clean", true).
		endEvent("GREEN_END").
		process(parentProc, parentParams)
}

// atGreenSystem asserts the at_green_system sub-process from the at_cycle
// call site: the AT_GREEN_SYSTEM call_activity, ENABLE_TESTS prelude, the
// single channel-agnostic green_phase_cycle dispatch (collapsed from the
// former backend/frontend duality), the parent-owned commit (literal
// change_type, no ${…} placeholder), TICK + MOVE_TICKET_IN_ACCEPTANCE,
// then GS_END. Restores the caller's scope on exit.
func (e *expectDispatch) atGreenSystem() *expectDispatch {
	parentProc, parentParams := e.proc, e.params
	return e.callActivity("AT_GREEN_SYSTEM", "at_green_system", noParams()).
		process("at_green_system", noParams()).
		userTask("ENABLE_TESTS", "enable-tests").
		greenCycle("AT_GREEN", atGreenParams()).
		callActivity("COMMIT", "commit", atGreenCommitParams()).
		process("commit", atGreenCommitParams()).
		userTask("APPROVE_COMMIT", "human").
		serviceTask("EXECUTE_COMMIT", "commit_phase").
		endEvent("COMMIT_END").
		process("at_green_system", noParams()).
		serviceTask("TICK", "tick_checklist").
		serviceTask("MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance").
		endEvent("GS_END").
		process(parentProc, parentParams)
}

// atRefactorSystem asserts the at_refactor_system sub-process from the
// at_cycle call site: the AT_REFACTOR_SYSTEM call_activity, the
// green_phase_cycle dispatch with the refactor agent, the
// refactor_changed gateway, and (on the changed=true branch) the
// terminal COMMIT then AR_END. Restores the caller's scope on exit.
func (e *expectDispatch) atRefactorSystem() *expectDispatch {
	parentProc, parentParams := e.proc, e.params
	return e.callActivity("AT_REFACTOR_SYSTEM", "at_refactor_system", noParams()).
		process("at_refactor_system", noParams()).
		greenCycle("AT_REFACTOR", atRefactorParams()).
		gateway("GATE_REFACTOR_CHANGED", "refactor_changed", true).
		callActivity("COMMIT", "commit", atRefactorCommitParams()).
		process("commit", atRefactorCommitParams()).
		userTask("APPROVE_COMMIT", "human").
		serviceTask("EXECUTE_COMMIT", "commit_phase").
		endEvent("COMMIT_END").
		process("at_refactor_system", noParams()).
		endEvent("AR_END").
		process(parentProc, parentParams)
}

// TestImplementTicket_Behavioral_TestOnly is the simplest behavioral happy
// path: a Story ticket where only the acceptance test is added (no DSL
// binding changes, no driver-adapter changes). The dsl_interface_changed
// gate short-circuits at_cycle straight to AT_GREEN_SYSTEM, so the only
// AT - RED - * phase dispatched is AT - RED - TEST.
func TestImplementTicket_Behavioral_TestOnly(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng, events := dispatchSpy(t)

	eng.Processes["main"].Start = "MOVE_TICKET_IN_PROGRESS"
	ctx := NewContext()
	seedBehavioralIntake(ctx)
	// at_cycle: dsl=false short-circuits straight to AT_GREEN_SYSTEM, so
	// AT_RED_DSL / CT_SUBPROCESS / AT_RED_SYSTEM_DRIVER stay unvisited.
	// The downstream at_cycle gates (GATE_EXT_AT, GATE_SYS_AT) are
	// unreached on this branch and don't need seeding.
	ctx.Set("dsl_interface_changed", false)

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunProcess("main", ctx); err != nil {
		t.Fatalf("RunProcess main: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	expect(events).
		behavioralIntake().
		redCycle("AT_RED_TEST", atRedTestParams()).
		gateway("GATE_DSL_AT", "dsl_interface_changed", false).
		atGreenSystem().
		atRefactorSystem().
		behavioralTail().
		assert(t)
}

// TestImplementTicket_Behavioral_TestAndDSL extends the test-only path: the
// acceptance test exercises a new DSL binding, so AT - RED - DSL runs after
// AT - RED - TEST. The external-system and system-driver interfaces are
// unchanged, so CT_SUBPROCESS and AT_RED_SYSTEM_DRIVER remain unvisited and
// at_cycle still falls through to AT_GREEN_SYSTEM after AT_RED_DSL.
func TestImplementTicket_Behavioral_TestAndDSL(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng, events := dispatchSpy(t)

	eng.Processes["main"].Start = "MOVE_TICKET_IN_PROGRESS"
	ctx := NewContext()
	seedBehavioralIntake(ctx)
	// at_cycle: dsl=true routes through AT_RED_DSL. The downstream gates
	// then ask whether external-system / system driver interfaces also
	// changed — both are seeded false so AT_RED_DSL is the only added
	// AT - RED - * dispatch and at_cycle still ends in AT_GREEN_SYSTEM.
	ctx.Set("dsl_interface_changed", true)
	ctx.Set("external_system_driver_interface_changed", false)
	ctx.Set("system_driver_interface_changed", false)

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunProcess("main", ctx); err != nil {
		t.Fatalf("RunProcess main: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	expect(events).
		behavioralIntake().
		redCycle("AT_RED_TEST", atRedTestParams()).
		gateway("GATE_DSL_AT", "dsl_interface_changed", true).
		redCycle("AT_RED_DSL", atRedDslParams()).
		gateway("GATE_DSL_FLAGS_PRESENT", "dsl_flags_present", true).
		gateway("GATE_EXT_AT", "external_system_driver_interface_changed", false).
		gateway("GATE_SYS_AT", "system_driver_interface_changed", false).
		atGreenSystem().
		atRefactorSystem().
		behavioralTail().
		assert(t)
}

// TestImplementTicket_Behavioral_TestAndDSLAndExternal extends test+DSL: the
// new DSL binding also reaches a new External System Driver, so AT_RED_DSL is
// followed by CT_SUBPROCESS. The system-side driver is unchanged
// (system_driver_interface_changed=false), so AT_RED_SYSTEM_DRIVER and
// VERIFY_AT_DRIVER stay unvisited and at_cycle still ends in AT_GREEN_SYSTEM
// after CT_SUBPROCESS.
//
// external_system_onboarding short-circuits via external_system_driver_exists
// = true — the Driver and Test Instance are assumed to already exist, so the
// sub-process exits at GATE_DRIVER_EXISTS without firing any service_task or
// user_task. Onboarding's own happy path (provision / define iface / impl
// driver / smoke / commit) belongs in a separate test.
func TestImplementTicket_Behavioral_TestAndDSLAndExternal(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng, events := dispatchSpy(t)

	eng.Processes["main"].Start = "MOVE_TICKET_IN_PROGRESS"
	ctx := NewContext()
	seedBehavioralIntake(ctx)
	// at_cycle: dsl=true routes through AT_RED_DSL, ext=true routes through
	// CT_SUBPROCESS, sys=false skips AT_RED_SYSTEM_DRIVER and falls through
	// to AT_GREEN_SYSTEM.
	ctx.Set("dsl_interface_changed", true)
	ctx.Set("external_system_driver_interface_changed", true)
	ctx.Set("system_driver_interface_changed", false)
	// external_system_onboarding: Driver already exists, so the sub-process
	// exits before any service_task / user_task fires.
	ctx.Set("external_system_driver_exists", true)

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunProcess("main", ctx); err != nil {
		t.Fatalf("RunProcess main: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	// CT_SUBPROCESS dispatches red_phase_cycle three times (TEST, DSL,
	// EXTERNAL SYSTEM DRIVER) on top of the two AT - RED - * dispatches, so
	// red_phase_cycle's WRITE/STOP/COMPILE/RUN/DISABLE/COMMIT trail repeats
	// five times in this run. CT_RED_TEST is the only red_phase_cycle call
	// site that pushes verify_real_suite — the rest match the AT shape.
	// CT_SUBPROCESS — ONBOARDING short-circuits (driver exists), no node
	// fires beyond GATE_DRIVER_EXISTS → ONBOARD_END; CT_RED_TEST → CT_RED_DSL
	// → CT_RED_EXTERNAL_SYSTEM_DRIVER each dispatch red_phase_cycle once.
	expect(events).
		behavioralIntake().
		redCycle("AT_RED_TEST", atRedTestParams()).
		gateway("GATE_DSL_AT", "dsl_interface_changed", true).
		redCycle("AT_RED_DSL", atRedDslParams()).
		gateway("GATE_DSL_FLAGS_PRESENT", "dsl_flags_present", true).
		gateway("GATE_EXT_AT", "external_system_driver_interface_changed", true).
		callActivity("CT_SUBPROCESS", "ct_subprocess", noParams()).
		process("ct_subprocess", noParams()).
		callActivity("ONBOARDING", "external_system_onboarding", noParams()).
		process("external_system_onboarding", noParams()).
		gateway("GATE_DRIVER_EXISTS", "external_system_driver_exists", true).
		endEvent("ONBOARD_END").
		process("ct_subprocess", noParams()).
		redCycle("CT_RED_TEST", ctRedTestParams()).
		gateway("GATE_DSL_CT", "dsl_interface_changed", true).
		redCycle("CT_RED_DSL", ctRedDslParams()).
		gateway("GATE_EXT_CT", "external_system_driver_interface_changed", true).
		redCycle("CT_RED_EXTERNAL_SYSTEM_DRIVER", ctRedExternalDriverParams()).
		serviceTask("VERIFY_CT_DRIVER", "run_tests").
		userTask("CT_GREEN_EXTERNAL_SYSTEM_STUB", "ct-green-external-system-stub").
		endEvent("CT_END").
		process("at_cycle", noParams()).
		gateway("GATE_SYS_AT", "system_driver_interface_changed", false).
		atGreenSystem().
		atRefactorSystem().
		behavioralTail().
		assert(t)
}
