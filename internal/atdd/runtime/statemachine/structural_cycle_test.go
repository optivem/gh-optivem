package statemachine

import (
	"reflect"
	"testing"
)

// TestImplementTicket_SystemInterfaceRedesign mirrors the ATDD-shaped
// scenario the driver runs for `gh optivem atdd implement-ticket --issue
// N` against a Task ticket carrying the `subtype:system-interface-redesign`
// label. Agents (claude shell-outs) and github-touching service tasks are
// mocked out; the runner walks `main` from the implement-ticket entry
// point (MOVE_TICKET_IN_PROGRESS); a spy collects the ordered list of
// work-doing nodes it visited.
func TestImplementTicket_SystemInterfaceRedesign(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng := loadSnapshot(t)

	noop := func(*Context) Outcome { return Outcome{} }
	eng.AgentFn = func(string) NodeFn { return noop }  // mocks claude dispatch
	eng.ActionFn = func(string) NodeFn { return noop } // mocks gh / git side effects
	eng.GateFn = func(name string) NodeFn {            // gates echo pre-seeded routing state
		return func(ctx *Context) Outcome {
			switch v := ctx.Get(name).(type) {
			case string:
				return Outcome{Value: v}
			case bool:
				return Outcome{Bool: v}
			}
			return Outcome{}
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// Spy: every service_task / user_task records its node ID before
	// firing the (mocked) inner function. Gateways, call_activities, and
	// start/end events are routing scaffolding, not "steps the runner
	// executes", so they're excluded. Entries are qualified with the
	// process name (e.g. "structural_cycle.COMPILE") because node IDs are
	// only unique per process — COMPILE, TICK, and WRITE collide across
	// structural_cycle / red_phase_cycle / green_phase_cycle /
	// at_green_system, so an unqualified trail would silently accept the
	// wrong call site once a sibling test exercises one of those cycles.
	var history []string
	for _, process := range eng.Processes {
		procName := process.Name
		for id, node := range process.Nodes {
			if node.Kind != ServiceTask && node.Kind != UserTask {
				continue
			}
			label, inner := procName+"."+node.ID, node.Fn
			node.Fn = func(ctx *Context) Outcome {
				history = append(history, label)
				return inner(ctx)
			}
			process.Nodes[id] = node
		}
	}

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
	// APPROVE_COMMIT. The test's gate mock echoes whatever ctx[binding]
	// is, so we seed the gateway's binding name directly. Red would route
	// to STOP_STRUCT_VERIFY_REVIEW → FIX_STRUCT_VERIFY → CHOOSE_TESTS;
	// gate-specific routing (retry counter etc.) is exercised in
	// gates/bindings_test.go.
	ctx.Set("structural_verify_outcome", "ok")

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunProcess("main", ctx); err != nil {
		t.Fatalf("RunProcess main: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	want := []string{
		"main.MOVE_TICKET_IN_PROGRESS",
		"github_intake.CLASSIFY_TICKET_TYPE",
		"github_intake.CLASSIFY_TICKET_SUBTYPE",
		"github_intake.READ_TICKET_BODY",
		"github_intake.REPORT_TICKET_DETAILS",
		"structural_cycle.WRITE",
		"structural_cycle.APPROVE_CHANGE",
		"structural_cycle.COMPILE",
		"structural_cycle.CHOOSE_TESTS",
		"structural_cycle.RUN_TESTS",
		// structural_cycle.COMMIT is a call_activity into the shared commit
		// sub-process — its inner APPROVE_COMMIT + EXECUTE_COMMIT show up
		// here instead of a single structural_cycle.COMMIT service_task.
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		"structural_cycle.TICK_CHECKLIST",
		"main.MOVE_TICKET_IN_ACCEPTANCE",
	}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("step history:\n got=%v\nwant=%v", history, want)
	}
}
