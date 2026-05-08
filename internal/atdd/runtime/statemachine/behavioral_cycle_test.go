package statemachine

import (
	"reflect"
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
	// red_phase_cycle gates (shared by every AT - RED - * dispatch):
	// compile passes; AT phases don't verify against a real suite
	// (verify_real_required=false routes straight to RUN); the desired
	// RED outcome is a runtime failure (tests_failed_runtime=true) which
	// routes to DISABLE → COMMIT.
	ctx.Set("compile_ok", true)
	ctx.Set("verify_real_required", false)
	ctx.Set("tests_failed_runtime", true)
	// green_phase_cycle gates (AT_GREEN_BACKEND + AT_GREEN_FRONTEND
	// dispatches): tests_pass=true ends each happy path at GREEN_END.
	// compile_ok is already set above and is reused by both phases.
	ctx.Set("tests_pass", true)
}

// redCycleEvents returns the trail one red_phase_cycle dispatch produces on
// the shared happy path: WRITE → STOP_RED_REVIEW → COMPILE → RUN → DISABLE,
// then the commit sub-process. params is the full ctx.Params snapshot the
// outer call_activity pushed; commitFrom(params) reflects the inner
// change_type-overlay the runtime applies inside commit.
func redCycleEvents(params map[string]string) []DispatchEvent {
	commit := commitFrom(params)
	return []DispatchEvent{
		userTask("red_phase_cycle", "WRITE", params["agent"], params),
		userTask("red_phase_cycle", "STOP_RED_REVIEW", "human", params),
		serviceTask("red_phase_cycle", "COMPILE", "compile_targeted", params),
		serviceTask("red_phase_cycle", "RUN", "run_targeted_tests", params),
		serviceTask("red_phase_cycle", "DISABLE", "disable_change_driven", params),
		userTask("commit", "APPROVE_COMMIT", "human", commit),
		serviceTask("commit", "EXECUTE_COMMIT", "commit_phase", commit),
	}
}

// greenCycleEvents returns the trail one green_phase_cycle dispatch produces
// on the shared happy path: WRITE → COMPILE → RUN. Commit is owned by the
// parent (at_green_system commits backend + frontend together), so the
// sub-process ends here.
func greenCycleEvents(params map[string]string) []DispatchEvent {
	return []DispatchEvent{
		userTask("green_phase_cycle", "WRITE", params["agent"], params),
		serviceTask("green_phase_cycle", "COMPILE", "compile_targeted", params),
		serviceTask("green_phase_cycle", "RUN", "run_targeted_tests", params),
	}
}

// atGreenSystemTail returns the trail at_green_system runs after both
// green_phase_cycle dispatches: the shared parent commit (literal
// change_type, no ${…} placeholder), then TICK and the sub-process'
// MOVE_TICKET_IN_ACCEPTANCE.
func atGreenSystemTail() []DispatchEvent {
	commit := atGreenCommitParams()
	return []DispatchEvent{
		userTask("commit", "APPROVE_COMMIT", "human", commit),
		serviceTask("commit", "EXECUTE_COMMIT", "commit_phase", commit),
		serviceTask("at_green_system", "TICK", "tick_checklist", noParams()),
		serviceTask("at_green_system", "MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance", noParams()),
	}
}

// behavioralIntakePrefix is the common opening of every behavioral test
// trail: implement-ticket entry through the github_intake reads. Story
// tickets skip CLASSIFY_TICKET_SUBTYPE (the GATE_TICKET_TYPE_INTAKE routes
// story → READ_TICKET_BODY directly).
func behavioralIntakePrefix() []DispatchEvent {
	return []DispatchEvent{
		serviceTask("main", "MOVE_TICKET_IN_PROGRESS", "move_to_in_progress", noParams()),
		serviceTask("github_intake", "CLASSIFY_TICKET_TYPE", "read_ticket_type", noParams()),
		serviceTask("github_intake", "READ_TICKET_BODY", "parse_ticket_body", noParams()),
		serviceTask("github_intake", "REPORT_TICKET_DETAILS", "report_intake_summary", noParams()),
	}
}

// concat returns a single slice concatenating its inputs.
func concat(slices ...[]DispatchEvent) []DispatchEvent {
	n := 0
	for _, s := range slices {
		n += len(s)
	}
	out := make([]DispatchEvent, 0, n)
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
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
	// at_green_system has its own MOVE_TICKET_IN_ACCEPTANCE before main's,
	// so move_to_in_acceptance fires twice on the behavioral happy path.
	want := concat(
		behavioralIntakePrefix(),
		redCycleEvents(atRedTestParams()),
		[]DispatchEvent{
			serviceTask("at_green_system", "ENABLE_TESTS", "enable_change_driven", noParams()),
		},
		greenCycleEvents(atGreenBackendParams()),
		greenCycleEvents(atGreenFrontendParams()),
		atGreenSystemTail(),
		[]DispatchEvent{
			serviceTask("main", "MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance", noParams()),
		},
	)
	if !reflect.DeepEqual(*events, want) {
		t.Errorf("dispatch events:\n got=\n%s\nwant=\n%s", formatEvents(*events), formatEvents(want))
	}
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
	want := concat(
		behavioralIntakePrefix(),
		redCycleEvents(atRedTestParams()),
		redCycleEvents(atRedDslParams()),
		[]DispatchEvent{
			serviceTask("at_green_system", "ENABLE_TESTS", "enable_change_driven", noParams()),
		},
		greenCycleEvents(atGreenBackendParams()),
		greenCycleEvents(atGreenFrontendParams()),
		atGreenSystemTail(),
		[]DispatchEvent{
			serviceTask("main", "MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance", noParams()),
		},
	)
	if !reflect.DeepEqual(*events, want) {
		t.Errorf("dispatch events:\n got=\n%s\nwant=\n%s", formatEvents(*events), formatEvents(want))
	}
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
	// EXTERNAL DRIVER) on top of the two AT - RED - * dispatches, so
	// red_phase_cycle's WRITE/STOP/COMPILE/RUN/DISABLE/COMMIT trail repeats
	// five times in this run. CT_RED_TEST is the only red_phase_cycle call
	// site that pushes verify_real_suite — the rest match the AT shape.
	want := concat(
		behavioralIntakePrefix(),
		redCycleEvents(atRedTestParams()),
		redCycleEvents(atRedDslParams()),
		// CT_SUBPROCESS — ONBOARDING short-circuits (driver exists), no
		// node fires; CT_RED_TEST → CT_RED_DSL → CT_RED_EXTERNAL_DRIVER
		// each dispatch red_phase_cycle once.
		redCycleEvents(ctRedTestParams()),
		redCycleEvents(ctRedDslParams()),
		redCycleEvents(ctRedExternalDriverParams()),
		[]DispatchEvent{
			serviceTask("ct_subprocess", "VERIFY_CT_DRIVER", "run_tests", noParams()),
			userTask("ct_subprocess", "CT_GREEN_STUBS", "atdd-stubs", noParams()),
			serviceTask("at_green_system", "ENABLE_TESTS", "enable_change_driven", noParams()),
		},
		greenCycleEvents(atGreenBackendParams()),
		greenCycleEvents(atGreenFrontendParams()),
		atGreenSystemTail(),
		[]DispatchEvent{
			serviceTask("main", "MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance", noParams()),
		},
	)
	if !reflect.DeepEqual(*events, want) {
		t.Errorf("dispatch events:\n got=\n%s\nwant=\n%s", formatEvents(*events), formatEvents(want))
	}
}
