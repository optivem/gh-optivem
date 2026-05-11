package statemachine

import (
	"testing"
)

// TestImplementTicket_SystemInterfaceRedesign mirrors the ATDD-shaped
// scenario the driver runs for `gh optivem atdd implement-ticket --issue
// N` against a Task ticket carrying the `subtype:system-interface-redesign`
// label. Agents (claude shell-outs) and github-touching service tasks are
// mocked out; the runner walks `main` from the implement-ticket entry
// point (MOVE_TICKET_IN_PROGRESS); the spy captures every node — service
// tasks, user tasks, gateways, call_activities, and end_events — with its
// resolved action / agent / outcome / call target and ctx.Params snapshot.
//
// Asserting the full BPMN trail catches three classes of regression the
// old service/user-only spy couldn't see: gateway routing decisions
// (binding + chosen branch), call_activity push/pop param shape at every
// call site, and end-event termination of each sub-process. ${…}
// expansion at user_task dispatch and node-level params merging fall out
// of the same trail.
func TestImplementTicket_SystemInterfaceRedesign(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng, events := dispatchSpy(t)

	// implement-ticket mode skips the picker (driver.Run mutates Start).
	// Pre-seed the routing state intake would derive — change_type drives
	// both the run_cycle gate and the da_cycle gate; ticket_type +
	// subtype are kept around so the gates' Context-first short-circuit
	// works for the upstream intake nodes; ticket_type_recognized +
	// parse_ok pass intake; compile_ok lets the structural compile gate
	// fall through to CHOOSE_TESTS.
	eng.Processes["main"].Start = "MOVE_TICKET_IN_PROGRESS"
	ctx := NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("subtype", "system-interface-redesign")
	ctx.Set("change_type", "system-interface-redesign")
	ctx.Set("ticket_type_recognized", true)
	ctx.Set("subtype_ok", true)
	ctx.Set("parse_ok", true)
	ctx.Set("legacy_acceptance_criteria_section_present", false)
	ctx.Set("compile_ok", true)
	// Happy-path verify: GATE_STRUCT_VERIFY (post-RUN_TESTS) routes ok →
	// COMMIT (call_activity into the shared commit sub-process). The test's
	// gate mock echoes whatever ctx[binding] is, so we seed the gateway's
	// binding name directly. Red would route to STOP_TEST_FAIL_REVIEW →
	// FIX_TEST → RUN_TESTS; gate-specific routing (retry counter etc.) is
	// exercised in gates/bindings_test.go.
	ctx.Set("structural_verify_outcome", "ok")

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunProcess("main", ctx); err != nil {
		t.Fatalf("RunProcess main: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	siParams := systemInterfaceRedesignParams()
	expect(events).
		process("main", noParams()).
		serviceTask("MOVE_TICKET_IN_PROGRESS", "move_to_in_progress").
		callActivity("INTAKE", "github_intake", noParams()).
		then().
		process("github_intake", noParams()).
		serviceTask("CLASSIFY_TICKET_TYPE", "read_ticket_type").
		gateway("GATE_CLASSIFY_CONFIDENT", "ticket_type_recognized", true).
		gateway("GATE_TICKET_TYPE_INTAKE", "ticket_type", "task").
		serviceTask("CLASSIFY_TICKET_SUBTYPE", "read_subtype").
		gateway("GATE_SUBTYPE_OK", "subtype_ok", true).
		serviceTask("READ_TICKET_BODY", "parse_ticket_body").
		gateway("GATE_PARSE_OK", "parse_ok", true).
		serviceTask("REPORT_TICKET_DETAILS", "report_intake_summary").
		endEvent("INTAKE_END").
		then().
		process("main", noParams()).
		callActivity("RUN_LEGACY_CYCLE", "run_legacy_cycle", noParams()).
		then().
		process("run_legacy_cycle", noParams()).
		gateway("GATE_LEGACY_PRESENT", "legacy_acceptance_criteria_section_present", false).
		endEvent("RUN_LEGACY_END").
		then().
		process("main", noParams()).
		callActivity("RUN_CYCLE", "run_cycle", noParams()).
		then().
		process("run_cycle", noParams()).
		gateway("GATE_CHANGE_TYPE", "change_type", "system-interface-redesign").
		callActivity("DA_CYCLE", "da_cycle", noParams()).
		then().
		process("da_cycle", noParams()).
		gateway("GATE_CHANGE_TYPE_DA", "change_type", "system-interface-redesign").
		callActivity("SYSTEM_INTERFACE_REDESIGN_CYCLE", "structural_cycle", siParams).
		then().
		process("structural_cycle", siParams).
		userTask("WRITE", "atdd-task").
		userTask("APPROVE_CHANGE", "human").
		serviceTask("COMPILE", "compile_in_scope").
		gateway("GATE_COMPILE_OK", "compile_ok", true).
		serviceTask("CHOOSE_TESTS", "select_tests").
		serviceTask("RUN_TESTS", "run_tests").
		gateway("GATE_STRUCT_VERIFY", "structural_verify_outcome", "ok").
		callActivity("COMMIT", "commit", commitFromTemplateParams()).
		then().
		process("commit", commitFrom(siParams)).
		userTask("APPROVE_COMMIT", "human").
		serviceTask("EXECUTE_COMMIT", "commit_phase").
		endEvent("COMMIT_END").
		then().
		process("structural_cycle", siParams).
		serviceTask("TICK_CHECKLIST", "tick_checklist").
		endEvent("STRUCT_END").
		then().
		process("da_cycle", noParams()).
		endEvent("DA_END").
		then().
		process("run_cycle", noParams()).
		endEvent("CYCLE_END").
		then().
		process("main", noParams()).
		serviceTask("MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance").
		endEvent("END").
		assert(t)
}
