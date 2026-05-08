package statemachine

import (
	"reflect"
	"testing"
)

// behavioralSpy mirrors the ARRANGE block used by every behavioral-cycle
// test: load the embedded snapshot, mock every registry (ActionFn / AgentFn
// → noop, GateFn → echo ctx[binding]), Bind, then wrap every service_task /
// user_task NodeFn with a spy that appends the process-qualified node ID to
// *history before firing the inner function. Gateways, call_activities, and
// start/end events are routing scaffolding, not "steps the runner executes",
// so they're excluded from the trail.
//
// Entries are qualified with the process name (e.g. "red_phase_cycle.WRITE")
// because node IDs are only unique per process — WRITE / COMPILE / RUN
// collide across red_phase_cycle and green_phase_cycle, and at_green_system
// has its own MOVE_TICKET_IN_ACCEPTANCE that shadows main's, so an
// unqualified trail would silently conflate sibling cycles.
//
// implement-ticket mode skips the picker (driver.Run mutates Start), so we
// override main.Start to MOVE_TICKET_IN_PROGRESS — the same entry point the
// real `gh optivem atdd implement-ticket --issue N` uses.
func behavioralSpy(t *testing.T, history *[]string) *Engine {
	t.Helper()
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

	for _, process := range eng.Processes {
		procName := process.Name
		for id, node := range process.Nodes {
			if node.Kind != ServiceTask && node.Kind != UserTask {
				continue
			}
			label, inner := procName+"."+node.ID, node.Fn
			node.Fn = func(ctx *Context) Outcome {
				*history = append(*history, label)
				return inner(ctx)
			}
			process.Nodes[id] = node
		}
	}

	eng.Processes["main"].Start = "MOVE_TICKET_IN_PROGRESS"
	return eng
}

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

// TestImplementTicket_Behavioral_TestOnly is the simplest behavioral happy
// path: a Story ticket where only the acceptance test is added (no DSL
// binding changes, no driver-adapter changes). The dsl_interface_changed
// gate short-circuits at_cycle straight to AT_GREEN_SYSTEM, so the only
// AT - RED - * phase dispatched is AT - RED - TEST.
func TestImplementTicket_Behavioral_TestOnly(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	var history []string
	eng := behavioralSpy(t, &history)

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
	// so move_to_in_acceptance fires twice on the behavioral happy path —
	// the qualified trail makes that explicit rather than hiding it.
	want := []string{
		"main.MOVE_TICKET_IN_PROGRESS",
		"github_intake.CLASSIFY_TICKET_TYPE",
		"github_intake.READ_TICKET_BODY",
		"github_intake.REPORT_TICKET_DETAILS",
		// AT - RED - TEST (red_phase_cycle dispatched with agent=atdd-test)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		// red_phase_cycle.COMMIT is a call_activity into the shared commit
		// sub-process — its inner APPROVE_COMMIT + EXECUTE_COMMIT show up
		// here instead of a single red_phase_cycle.COMMIT service_task.
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// AT - GREEN - SYSTEM (green_phase_cycle dispatched twice)
		"at_green_system.ENABLE_TESTS",
		"green_phase_cycle.WRITE", // backend
		"green_phase_cycle.COMPILE",
		"green_phase_cycle.RUN",
		"green_phase_cycle.WRITE", // frontend
		"green_phase_cycle.COMPILE",
		"green_phase_cycle.RUN",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		"at_green_system.TICK",
		"at_green_system.MOVE_TICKET_IN_ACCEPTANCE",
		"main.MOVE_TICKET_IN_ACCEPTANCE",
	}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("step history:\n got=%v\nwant=%v", history, want)
	}
}

// TestImplementTicket_Behavioral_TestAndDSL extends the test-only path: the
// acceptance test exercises a new DSL binding, so AT - RED - DSL runs after
// AT - RED - TEST. The external-system and system-driver interfaces are
// unchanged, so CT_SUBPROCESS and AT_RED_SYSTEM_DRIVER remain unvisited and
// at_cycle still falls through to AT_GREEN_SYSTEM after AT_RED_DSL.
func TestImplementTicket_Behavioral_TestAndDSL(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	var history []string
	eng := behavioralSpy(t, &history)

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
	// red_phase_cycle's WRITE/STOP/COMPILE/RUN/DISABLE/COMMIT trail repeats
	// once per AT - RED - * dispatch (atdd-test then atdd-dsl). The
	// process-qualified trail can't distinguish the two dispatches by
	// agent — both render to "red_phase_cycle.<NODE>" — so the agent
	// distinction lives in the params (asserted by the bindings tests),
	// not in this trail.
	want := []string{
		"main.MOVE_TICKET_IN_PROGRESS",
		"github_intake.CLASSIFY_TICKET_TYPE",
		"github_intake.READ_TICKET_BODY",
		"github_intake.REPORT_TICKET_DETAILS",
		// AT - RED - TEST (red_phase_cycle dispatched with agent=atdd-test)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		// commit sub-process for AT - RED - TEST
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// AT - RED - DSL (red_phase_cycle dispatched with agent=atdd-dsl)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		// commit sub-process for AT - RED - DSL
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// AT - GREEN - SYSTEM (green_phase_cycle dispatched twice)
		"at_green_system.ENABLE_TESTS",
		"green_phase_cycle.WRITE", // backend
		"green_phase_cycle.COMPILE",
		"green_phase_cycle.RUN",
		"green_phase_cycle.WRITE", // frontend
		"green_phase_cycle.COMPILE",
		"green_phase_cycle.RUN",
		// commit sub-process for AT - GREEN - SYSTEM
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		"at_green_system.TICK",
		"at_green_system.MOVE_TICKET_IN_ACCEPTANCE",
		"main.MOVE_TICKET_IN_ACCEPTANCE",
	}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("step history:\n got=%v\nwant=%v", history, want)
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
	var history []string
	eng := behavioralSpy(t, &history)

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
	// five times in this run. As before, the process-qualified trail can't
	// distinguish dispatches by agent — that distinction lives in params
	// (asserted by the bindings tests), not in this trail.
	want := []string{
		"main.MOVE_TICKET_IN_PROGRESS",
		"github_intake.CLASSIFY_TICKET_TYPE",
		"github_intake.READ_TICKET_BODY",
		"github_intake.REPORT_TICKET_DETAILS",
		// AT - RED - TEST (red_phase_cycle dispatched with agent=atdd-test)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// AT - RED - DSL (red_phase_cycle dispatched with agent=atdd-dsl)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// CT_SUBPROCESS — ONBOARDING short-circuits (driver exists), no node fires.
		// CT - RED - TEST (red_phase_cycle dispatched with agent=atdd-test)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// CT - RED - DSL (red_phase_cycle dispatched with agent=atdd-dsl)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		// CT - RED - EXTERNAL DRIVER (red_phase_cycle dispatched with agent=atdd-driver)
		"red_phase_cycle.WRITE",
		"red_phase_cycle.STOP_RED_REVIEW",
		"red_phase_cycle.COMPILE",
		"red_phase_cycle.RUN",
		"red_phase_cycle.DISABLE",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		"ct_subprocess.VERIFY_CT_DRIVER",
		"ct_subprocess.CT_GREEN_STUBS",
		// AT - GREEN - SYSTEM (green_phase_cycle dispatched twice)
		"at_green_system.ENABLE_TESTS",
		"green_phase_cycle.WRITE", // backend
		"green_phase_cycle.COMPILE",
		"green_phase_cycle.RUN",
		"green_phase_cycle.WRITE", // frontend
		"green_phase_cycle.COMPILE",
		"green_phase_cycle.RUN",
		"commit.APPROVE_COMMIT",
		"commit.EXECUTE_COMMIT",
		"at_green_system.TICK",
		"at_green_system.MOVE_TICKET_IN_ACCEPTANCE",
		"main.MOVE_TICKET_IN_ACCEPTANCE",
	}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("step history:\n got=%v\nwant=%v", history, want)
	}
}
