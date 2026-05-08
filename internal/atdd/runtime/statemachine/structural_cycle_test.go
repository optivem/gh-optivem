package statemachine

import (
	"testing"
)

// TestImplementTicket_SystemInterfaceRedesign mirrors the ATDD-shaped
// scenario the driver runs for `gh optivem atdd implement-ticket --issue
// N` against a Task ticket carrying the `subtype:system-interface-redesign`
// label. Agents (claude shell-outs) and github-touching service tasks are
// mocked out; the runner walks `main` from the implement-ticket entry
// point (MOVE_TICKET_IN_PROGRESS); the spy captures each dispatched
// service_task / user_task with its resolved action/agent and ctx.Params
// snapshot.
//
// Asserting params alongside the trail catches three classes of regression
// the old string-trail couldn't see: call_activity param push/pop, ${…}
// expansion at user_task dispatch, and node-level params merging.
func TestImplementTicket_SystemInterfaceRedesign(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng, events := dispatchSpy(t)

	// implement-ticket mode skips the picker (driver.Run mutates Start).
	// Pre-seed the routing state intake would derive — change_type drives
	// both the run_cycle gate and the da_cycle gate; ticket_type +
	// subtype are kept around so the gates' Context-first short-circuit
	// works for the upstream intake nodes; ticket_type_recognized +
	// parse_ok pass intake; structural_test_mode picks the TEST gate.
	eng.Processes["main"].Start = "MOVE_TICKET_IN_PROGRESS"
	ctx := NewContext()
	ctx.Set("ticket_type", "task")
	ctx.Set("subtype", "system-interface-redesign")
	ctx.Set("change_type", "system-interface-redesign")
	ctx.Set("ticket_type_recognized", true)
	ctx.Set("subtype_ok", true)
	ctx.Set("parse_ok", true)
	ctx.Set("legacy_acceptance_criteria_section_present", false)
	ctx.Set("structural_test_mode", "full")
	// Happy-path verify: GATE_STRUCT_VERIFY (post-RUN_TESTS) routes ok →
	// COMMIT (call_activity into the shared commit sub-process). The test's
	// gate mock echoes whatever ctx[binding] is, so we seed the gateway's
	// binding name directly. Red would route to STOP_STRUCT_VERIFY_REVIEW →
	// FIX_STRUCT_VERIFY → CHOOSE_TESTS; gate-specific routing (retry
	// counter etc.) is exercised in gates/bindings_test.go.
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
		then().
		process("github_intake", noParams()).
		serviceTask("CLASSIFY_TICKET_TYPE", "read_ticket_type").
		serviceTask("CLASSIFY_TICKET_SUBTYPE", "read_subtype").
		serviceTask("READ_TICKET_BODY", "parse_ticket_body").
		serviceTask("REPORT_TICKET_DETAILS", "report_intake_summary").
		then().
		process("structural_cycle", siParams).
		userTask("WRITE", "atdd-task").
		userTask("APPROVE_CHANGE", "human").
		serviceTask("COMPILE", "compile_in_scope").
		serviceTask("CHOOSE_TESTS", "select_tests").
		serviceTask("RUN_TESTS", "run_tests").
		then().
		process("commit", commitFrom(siParams)).
		userTask("APPROVE_COMMIT", "human").
		serviceTask("EXECUTE_COMMIT", "commit_phase").
		then().
		process("structural_cycle", siParams).
		serviceTask("TICK_CHECKLIST", "tick_checklist").
		then().
		process("main", noParams()).
		serviceTask("MOVE_TICKET_IN_ACCEPTANCE", "move_to_in_acceptance").
		assert(t)
}
