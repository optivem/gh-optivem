// Transitions test suite for the ATDD process-flow YAML.
//
// Strategy:
//   - Load the canonical embedded YAML via LoadDefault.
//   - Assert structural invariants over every process (start exists, edges
//     reference existing nodes, gateways have at least one outgoing edge,
//     no orphan nodes, every node either reaches an end or has an outgoing
//     edge).
//   - Drive every documented sequence flow through nextEdge with synthetic
//     Context state, asserting the expected target. Each gateway branch is
//     exercised explicitly so a YAML edit that drops a branch will fail
//     here.
//   - Explicitly assert the decisions encoded for the open process-audit
//     gaps (CT exit re-evaluation, smoke-test resume path, legacy-acceptance-criteria
//     interim spec) so they can no longer drift back to TBDs.
//
// Tests use the unbound Engine (no Bind()) — nextEdge does not need NodeFn
// resolution; it only walks the edge list against the Context state.
package statemachine

import (
	"testing"
)

func loadSnapshot(t *testing.T) *Engine {
	t.Helper()
	eng, err := LoadDefault()
	if err != nil {
		t.Fatalf("load embedded process-flow: %v", err)
	}
	return eng
}

// ---------------------------------------------------------------------------
// Structural invariants
// ---------------------------------------------------------------------------

func TestLoadSnapshot_AllProcessesParse(t *testing.T) {
	eng := loadSnapshot(t)
	wantProcesses := []string{
		"main",
		"github_intake",
		"run_legacy_cycle",
		"backlog_refinement",
		"run_cycle",
		"at_cycle",
		"at_green_system",
		"da_cycle",
		"sut_cycle",
		"ct_subprocess",
		"external_system_onboarding",
		"structural_cycle",
		"red_phase_cycle",
		"green_phase_cycle",
		"compile",
		"commit",
		"legacy_acceptance_criteria",
		"legacy_at_cycle",
		"legacy_ct_cycle",
	}
	for _, name := range wantProcesses {
		if _, ok := eng.Processes[name]; !ok {
			t.Errorf("process %q missing from loaded snapshot", name)
		}
	}
}

func TestStructuralIntegrity_StartNodesExist(t *testing.T) {
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		if _, ok := process.Nodes[process.Start]; !ok {
			t.Errorf("process %q: start node %q not in nodes list", name, process.Start)
		}
	}
}

func TestStructuralIntegrity_GatewaysHaveOutgoingEdges(t *testing.T) {
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind != Gateway {
				continue
			}
			if len(process.OutgoingByNode[id]) == 0 {
				t.Errorf("process %q gateway %q has no outgoing edges", name, id)
			}
		}
	}
}

func TestStructuralIntegrity_NonEndNodesHaveSuccessor(t *testing.T) {
	// A node may legitimately have no outgoing edges in two cases:
	//   - it is an end_event, or
	//   - it is a `agent: human` STOP node intended to halt the run
	//     awaiting user intervention (e.g. ASK_SUPPORT in onboarding when
	//     the smoke test fails — process-audit gap on smoke-test resume
	//     resolved as "STOP, do not auto-resume").
	// Anything else is a missing-edge bug.
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind == EndEvent {
				continue
			}
			if len(process.OutgoingByNode[id]) > 0 {
				continue
			}
			if node.Kind == UserTask && node.Raw.Agent == "human" {
				continue // intentional STOP-and-halt
			}
			t.Errorf("process %q non-end node %q has no outgoing edges", name, id)
		}
	}
}

// ---------------------------------------------------------------------------
// Transition table — one row per documented sequence flow
// ---------------------------------------------------------------------------

type transitionCase struct {
	process string
	from   string
	state  map[string]any
	params map[string]string
	wantTo string
	desc   string
}

// All transitions documented in process-flow.yaml. Unguarded edges are
// asserted with state=nil; guarded edges are asserted under the state that
// satisfies the predicate.
var transitionTable = []transitionCase{
	// ---- main process ----
	{process: "main", from: "START", state: map[string]any{"mode": "board"}, wantTo: "PICK_TOP_READY", desc: "board mode enters via PICK_TOP_READY"},
	{process: "main", from: "START", state: map[string]any{"mode": "specific_issue"}, wantTo: "MOVE_TICKET_IN_PROGRESS", desc: "specific-issue mode skips PICK_TOP_READY"},
	{process: "main", from: "PICK_TOP_READY", wantTo: "MOVE_TICKET_IN_PROGRESS"},
	{process: "main", from: "MOVE_TICKET_IN_PROGRESS", wantTo: "INTAKE"},
	{process: "main", from: "INTAKE", wantTo: "RUN_LEGACY_CYCLE"},
	{process: "main", from: "RUN_LEGACY_CYCLE", wantTo: "BACKLOG_REFINEMENT"},
	{process: "main", from: "BACKLOG_REFINEMENT", wantTo: "RUN_CYCLE"},
	{process: "main", from: "RUN_CYCLE", wantTo: "MOVE_TICKET_IN_ACCEPTANCE"},
	{process: "main", from: "MOVE_TICKET_IN_ACCEPTANCE", wantTo: "END"},

	// ---- intake ----
	// Issue Forms enforce the ticket schema upstream so intake is a pure
	// service-task pipeline (classify ticket, classify subtype, parse body)
	// with two STOPs for unhappy paths (classification conflict, parse
	// error). No LLM dispatch — no agent fan-out by ticket type.
	{process: "github_intake", from: "CLASSIFY_TICKET_TYPE", wantTo: "GATE_CLASSIFY_CONFIDENT"},
	{process: "github_intake", from: "GATE_CLASSIFY_CONFIDENT", state: map[string]any{"ticket_type_recognized": true}, wantTo: "GATE_TICKET_TYPE_INTAKE"},
	{process: "github_intake", from: "GATE_CLASSIFY_CONFIDENT", state: map[string]any{"ticket_type_recognized": false}, wantTo: "STOP_CLASSIFY_CONFLICT"},
	{process: "github_intake", from: "STOP_CLASSIFY_CONFLICT", wantTo: "CLASSIFY_TICKET_TYPE"},
	{process: "github_intake", from: "GATE_TICKET_TYPE_INTAKE", state: map[string]any{"ticket_type": "story"}, wantTo: "READ_TICKET_BODY"},
	{process: "github_intake", from: "GATE_TICKET_TYPE_INTAKE", state: map[string]any{"ticket_type": "bug"}, wantTo: "READ_TICKET_BODY"},
	{process: "github_intake", from: "GATE_TICKET_TYPE_INTAKE", state: map[string]any{"ticket_type": "task"}, wantTo: "CLASSIFY_TICKET_SUBTYPE"},
	{process: "github_intake", from: "CLASSIFY_TICKET_SUBTYPE", wantTo: "GATE_SUBTYPE_OK"},
	{process: "github_intake", from: "GATE_SUBTYPE_OK", state: map[string]any{"subtype_ok": true}, wantTo: "READ_TICKET_BODY"},
	{process: "github_intake", from: "GATE_SUBTYPE_OK", state: map[string]any{"subtype_ok": false}, wantTo: "STOP_SUBTYPE_MISSING"},
	{process: "github_intake", from: "STOP_SUBTYPE_MISSING", wantTo: "CLASSIFY_TICKET_SUBTYPE"},
	{process: "github_intake", from: "READ_TICKET_BODY", wantTo: "GATE_PARSE_OK"},
	{process: "github_intake", from: "GATE_PARSE_OK", state: map[string]any{"parse_ok": true}, wantTo: "REPORT_TICKET_DETAILS"},
	{process: "github_intake", from: "GATE_PARSE_OK", state: map[string]any{"parse_ok": false}, wantTo: "STOP_PARSE_ERROR"},
	{process: "github_intake", from: "STOP_PARSE_ERROR", wantTo: "READ_TICKET_BODY"},
	{process: "github_intake", from: "REPORT_TICKET_DETAILS", wantTo: "INTAKE_END"},

	// ---- run_legacy_cycle ----
	// Backfill cycle for legacy acceptance criteria. Self-contained: gates internally on
	// presence and no-ops when absent so main can call it unconditionally.
	{process: "run_legacy_cycle", from: "GATE_LEGACY_PRESENT", state: map[string]any{"legacy_acceptance_criteria_section_present": true}, wantTo: "LEGACY_CYCLE"},
	{process: "run_legacy_cycle", from: "GATE_LEGACY_PRESENT", state: map[string]any{"legacy_acceptance_criteria_section_present": false}, wantTo: "RUN_LEGACY_END"},
	{process: "run_legacy_cycle", from: "LEGACY_CYCLE", wantTo: "RUN_LEGACY_END"},

	// ---- backlog_refinement ----
	// Materializes the parsed-concepts artifact, then asks the operator y/n
	// whether to invoke refine-acc at all (GATE_REFINE_REQUESTED). On yes,
	// refines parsed acceptance criteria (Gherkin form + coverage rubric),
	// human-confirms, then conditionally writes the refined sections back
	// to the ticket source. A no-op refinement (refinement_changed == false)
	// discharges without writing. On no, the sub-process skips the refine
	// pass entirely and discharges at BR_END with the artifact unread.
	{process: "backlog_refinement", from: "MATERIALIZE_PARSED_CONCEPTS", wantTo: "GATE_REFINE_REQUESTED"},
	{process: "backlog_refinement", from: "GATE_REFINE_REQUESTED", state: map[string]any{"refine_requested": true}, wantTo: "BACKLOG_REFINEMENT"},
	{process: "backlog_refinement", from: "GATE_REFINE_REQUESTED", state: map[string]any{"refine_requested": false}, wantTo: "BR_END"},
	{process: "backlog_refinement", from: "BACKLOG_REFINEMENT", wantTo: "CONFIRM_REFINEMENT"},
	{process: "backlog_refinement", from: "CONFIRM_REFINEMENT", wantTo: "GATE_REFINEMENT_CHANGED"},
	{process: "backlog_refinement", from: "GATE_REFINEMENT_CHANGED", state: map[string]any{"refinement_changed": true}, wantTo: "UPDATE_TICKET"},
	{process: "backlog_refinement", from: "GATE_REFINEMENT_CHANGED", state: map[string]any{"refinement_changed": false}, wantTo: "BR_END"},
	{process: "backlog_refinement", from: "UPDATE_TICKET", wantTo: "BR_END"},

	// ---- run_cycle ----
	// Change cycle dispatch — single-axis gate on the derived `change_type`.
	// Four branches: AT_CYCLE (behavioral), DA_CYCLE (system /
	// external-system interface redesign), SUT_CYCLE
	// (system-implementation-refactoring).
	{process: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "behavioral"}, wantTo: "AT_CYCLE"},
	{process: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "system-interface-redesign"}, wantTo: "DA_CYCLE"},
	{process: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "external-system-interface-redesign"}, wantTo: "DA_CYCLE"},
	{process: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "system-implementation-refactoring"}, wantTo: "SUT_CYCLE"},
	{process: "run_cycle", from: "AT_CYCLE", wantTo: "CYCLE_END"},
	{process: "run_cycle", from: "DA_CYCLE", wantTo: "CYCLE_END"},
	{process: "run_cycle", from: "SUT_CYCLE", wantTo: "CYCLE_END"},

	// ---- da_cycle ----
	// Driver Adapter cycle. Splits on `change_type`; both branches route to
	// structural_cycle (system-interface-redesign rewrites an own Driver
	// Adapter; external-system-interface-redesign rewrites an external Driver
	// Adapter + its stub, leaving the DSL-level CT and DSL untouched).
	{process: "da_cycle", from: "GATE_CHANGE_TYPE_DA", state: map[string]any{"change_type": "system-interface-redesign"}, wantTo: "SYSTEM_INTERFACE_REDESIGN_CYCLE"},
	{process: "da_cycle", from: "GATE_CHANGE_TYPE_DA", state: map[string]any{"change_type": "external-system-interface-redesign"}, wantTo: "EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE"},
	{process: "da_cycle", from: "SYSTEM_INTERFACE_REDESIGN_CYCLE", wantTo: "DA_END"},
	{process: "da_cycle", from: "EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE", wantTo: "DA_END"},

	// ---- sut_cycle ----
	// System Under Test cycle. Single node calling structural_cycle with
	// system-implementation-refactoring-flavour params.
	{process: "sut_cycle", from: "SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE", wantTo: "SUT_END"},

	// ---- at_cycle ----
	{process: "at_cycle", from: "AT_RED_TEST", wantTo: "GATE_DSL_AT"},
	{process: "at_cycle", from: "GATE_DSL_AT", state: map[string]any{"dsl_interface_changed": false}, wantTo: "AT_GREEN_SYSTEM"},
	{process: "at_cycle", from: "GATE_DSL_AT", state: map[string]any{"dsl_interface_changed": true}, wantTo: "AT_RED_DSL"},
	{process: "at_cycle", from: "AT_RED_DSL", wantTo: "GATE_DSL_FLAGS_PRESENT"},
	// Post-RED-DSL flag-presence validation gateway (plan 20260518-1144 item 4):
	// the two RED-DSL phase-output flags MUST be explicitly emitted; unset is
	// an authoring bug, not a default no. STOP_FLAG_UNSET routes back to
	// AT_RED_DSL for a re-dispatch with the reminder.
	{process: "at_cycle", from: "GATE_DSL_FLAGS_PRESENT", state: map[string]any{"dsl_flags_present": true}, wantTo: "GATE_EXT_AT", desc: "both RED-DSL flags emitted → continue to existing branch gates"},
	{process: "at_cycle", from: "GATE_DSL_FLAGS_PRESENT", state: map[string]any{"dsl_flags_present": false}, wantTo: "STOP_FLAG_UNSET", desc: "either flag unset → human STOP, then re-run RED-DSL"},
	{process: "at_cycle", from: "STOP_FLAG_UNSET", wantTo: "AT_RED_DSL", desc: "after STOP, re-dispatch RED-DSL so the agent emits both flags"},
	{process: "at_cycle", from: "GATE_EXT_AT", state: map[string]any{"external_system_driver_interface_changed": true}, wantTo: "CT_SUBPROCESS"},
	{process: "at_cycle", from: "GATE_EXT_AT", state: map[string]any{"external_system_driver_interface_changed": false}, wantTo: "GATE_SYS_AT"},
	// CT exit re-evaluation: process-audit gap resolved — CT_SUBPROCESS returns
	// to GATE_SYS_AT so System Driver changes are still routed through after CT.
	{process: "at_cycle", from: "CT_SUBPROCESS", wantTo: "GATE_SYS_AT", desc: "CT exit re-evaluates system_driver_interface_changed (process-audit gap resolved)"},
	{process: "at_cycle", from: "GATE_SYS_AT", state: map[string]any{"system_driver_interface_changed": true}, wantTo: "AT_RED_SYSTEM_DRIVER"},
	{process: "at_cycle", from: "GATE_SYS_AT", state: map[string]any{"system_driver_interface_changed": false}, wantTo: "AT_GREEN_SYSTEM"},
	{process: "at_cycle", from: "AT_RED_SYSTEM_DRIVER", wantTo: "VERIFY_AT_DRIVER"},
	{process: "at_cycle", from: "VERIFY_AT_DRIVER", wantTo: "AT_GREEN_SYSTEM"},
	{process: "at_cycle", from: "AT_GREEN_SYSTEM", wantTo: "AT_REFACTOR_SYSTEM"},
	{process: "at_cycle", from: "AT_REFACTOR_SYSTEM", wantTo: "AT_END"},

	// ---- at_green_system ----
	// Decomposed per the AT/CT split plan: ENABLE_TESTS (re-enable disabled
	// tests) → AT_GREEN (single channel-agnostic call_activity into the
	// shared green_phase_cycle) → COMMIT (call_activity into the shared
	// commit sub-process) → TICK → MOVE_TICKET_IN_ACCEPTANCE. The legacy
	// ATDD_RELEASE user_task is replaced by the existing service_task
	// actions.
	{process: "at_green_system", from: "ENABLE_TESTS", wantTo: "AT_GREEN"},
	{process: "at_green_system", from: "AT_GREEN", wantTo: "COMMIT"},
	{process: "at_green_system", from: "COMMIT", wantTo: "TICK"},
	{process: "at_green_system", from: "TICK", wantTo: "MOVE_TICKET_IN_ACCEPTANCE"},
	{process: "at_green_system", from: "MOVE_TICKET_IN_ACCEPTANCE", wantTo: "GS_END"},

	// ---- at_refactor_system ----
	// Post-GREEN housekeeping refactor on production code. Mirrors
	// at_green_system minus the ENABLE_TESTS / TICK / MOVE_TICKET_IN_ACCEPTANCE
	// wrapper. GATE_REFACTOR_CHANGED gates the terminal COMMIT — no-op
	// refactors discharge directly to AR_END.
	{process: "at_refactor_system", from: "AT_REFACTOR", wantTo: "GATE_REFACTOR_CHANGED"},
	{process: "at_refactor_system", from: "GATE_REFACTOR_CHANGED", state: map[string]any{"refactor_changed": true}, wantTo: "COMMIT"},
	{process: "at_refactor_system", from: "GATE_REFACTOR_CHANGED", state: map[string]any{"refactor_changed": false}, wantTo: "AR_END"},
	{process: "at_refactor_system", from: "COMMIT", wantTo: "AR_END"},

	// ---- ct_subprocess ----
	{process: "ct_subprocess", from: "ONBOARDING", wantTo: "CT_RED_TEST"},
	{process: "ct_subprocess", from: "CT_RED_TEST", wantTo: "GATE_DSL_CT"},
	{process: "ct_subprocess", from: "GATE_DSL_CT", state: map[string]any{"dsl_interface_changed": false}, wantTo: "CT_GREEN_EXTERNAL_SYSTEM_STUB"},
	{process: "ct_subprocess", from: "GATE_DSL_CT", state: map[string]any{"dsl_interface_changed": true}, wantTo: "CT_RED_DSL"},
	{process: "ct_subprocess", from: "CT_RED_DSL", wantTo: "GATE_EXT_CT"},
	{process: "ct_subprocess", from: "GATE_EXT_CT", state: map[string]any{"external_system_driver_interface_changed": false}, wantTo: "CT_GREEN_EXTERNAL_SYSTEM_STUB"},
	{process: "ct_subprocess", from: "GATE_EXT_CT", state: map[string]any{"external_system_driver_interface_changed": true}, wantTo: "CT_RED_EXTERNAL_SYSTEM_DRIVER"},
	{process: "ct_subprocess", from: "CT_RED_EXTERNAL_SYSTEM_DRIVER", wantTo: "VERIFY_CT_DRIVER"},
	{process: "ct_subprocess", from: "VERIFY_CT_DRIVER", wantTo: "CT_GREEN_EXTERNAL_SYSTEM_STUB"},
	{process: "ct_subprocess", from: "CT_GREEN_EXTERNAL_SYSTEM_STUB", wantTo: "CT_END"},

	// ---- external_system_onboarding ----
	// Smoke-test resume path: process-audit gap resolved — when the smoke
	// test fails the run STOPs at ASK_SUPPORT (no resume; user must pair).
	{process: "external_system_onboarding", from: "GATE_DRIVER_EXISTS", state: map[string]any{"external_system_driver_exists": true}, wantTo: "ONBOARD_END", desc: "early return when driver already exists"},
	{process: "external_system_onboarding", from: "GATE_DRIVER_EXISTS", state: map[string]any{"external_system_driver_exists": false}, wantTo: "GATE_INSTANCE_ACCESSIBLE"},
	{process: "external_system_onboarding", from: "GATE_INSTANCE_ACCESSIBLE", state: map[string]any{"external_system_test_instance_accessible": true}, wantTo: "DEFINE_IFACE"},
	{process: "external_system_onboarding", from: "GATE_INSTANCE_ACCESSIBLE", state: map[string]any{"external_system_test_instance_accessible": false}, wantTo: "PROVISION"},
	{process: "external_system_onboarding", from: "PROVISION", wantTo: "DEFINE_IFACE"},
	{process: "external_system_onboarding", from: "DEFINE_IFACE", wantTo: "IMPL_DRIVER"},
	{process: "external_system_onboarding", from: "IMPL_DRIVER", wantTo: "WRITE_SMOKE"},
	{process: "external_system_onboarding", from: "WRITE_SMOKE", wantTo: "RUN_SMOKE"},
	{process: "external_system_onboarding", from: "RUN_SMOKE", wantTo: "GATE_SMOKE_PASS"},
	{process: "external_system_onboarding", from: "GATE_SMOKE_PASS", state: map[string]any{"smoke_test_passes": false}, wantTo: "ASK_SUPPORT", desc: "smoke fail → STOP and ask user (no auto-resume)"},
	{process: "external_system_onboarding", from: "GATE_SMOKE_PASS", state: map[string]any{"smoke_test_passes": true}, wantTo: "COMMIT"},
	{process: "external_system_onboarding", from: "COMMIT", wantTo: "ONBOARD_END"},

	// ---- structural_cycle (shared by SYSAPI / SYSUI / SYSTEM_IMPLEMENTATION_REFACTORING via params) ----
	// COMPILE always runs after APPROVE_CHANGE. Compile or test RED routes
	// through a human STOP (Enter = dispatch fix-agent, abort = halt) and
	// then FIX_COMPILE / FIX_TEST — one shared fix-verify agent
	// branching on failure_type. Compile retries from COMPILE; test retries
	// from BUILD_SYSTEM so the fix-agent's edits land in a rebuilt image
	// before the same selected commands re-run. GATE_TESTS_SELECTED routes
	// the "operator skipped tests" path past BUILD/START/RUN to COMMIT.
	{process: "structural_cycle", from: "WRITE", wantTo: "APPROVE_CHANGE"},
	{process: "structural_cycle", from: "APPROVE_CHANGE", wantTo: "COMPILE"},
	{process: "structural_cycle", from: "COMPILE", wantTo: "CHOOSE_TESTS", desc: "COMPILE is a call_activity to the shared `compile` sub-process; it returns only on compile_ok, so the parent edge is unconditional"},
	{process: "structural_cycle", from: "CHOOSE_TESTS", wantTo: "GATE_TESTS_SELECTED"},
	{process: "structural_cycle", from: "GATE_TESTS_SELECTED", state: map[string]any{"tests_selected": true}, wantTo: "BUILD_SYSTEM", desc: "selected → rebuild SUT before run"},
	{process: "structural_cycle", from: "GATE_TESTS_SELECTED", state: map[string]any{"tests_selected": false}, wantTo: "COMMIT", desc: "no tests selected → skip build/start/run, advance to commit"},
	{process: "structural_cycle", from: "BUILD_SYSTEM", wantTo: "START_SYSTEM"},
	{process: "structural_cycle", from: "START_SYSTEM", wantTo: "RUN_TESTS"},
	{process: "structural_cycle", from: "RUN_TESTS", wantTo: "GATE_STRUCT_VERIFY"},
	{process: "structural_cycle", from: "GATE_STRUCT_VERIFY", state: map[string]any{"structural_verify_outcome": "ok"}, wantTo: "COMMIT", desc: "ok class continues directly to commit gate"},
	{process: "structural_cycle", from: "GATE_STRUCT_VERIFY", state: map[string]any{"structural_verify_outcome": "red"}, wantTo: "STOP_TEST_FAIL_REVIEW", desc: "red class halts at a human review STOP before any fix-agent dispatch"},
	{process: "structural_cycle", from: "STOP_TEST_FAIL_REVIEW", wantTo: "FIX_TEST", desc: "human approves the dispatch and the fix-verify agent runs in test mode"},
	{process: "structural_cycle", from: "FIX_TEST", wantTo: "BUILD_SYSTEM", desc: "fix agent loops back through BUILD_SYSTEM so the rebuilt image picks up its edits before the same selected commands re-run"},
	{process: "structural_cycle", from: "COMMIT", wantTo: "TICK_CHECKLIST"},
	{process: "structural_cycle", from: "TICK_CHECKLIST", wantTo: "STRUCT_END"},

	// ---- red_phase_cycle (shared by AT/CT RED-WRITE phases via params) ----
	// Splits creative WRITE work from mechanical compile/run/disable/commit.
	// All three AT RED phases and all three CT RED phases call into this
	// process; CT_RED_TEST additionally enables the optional verify_real_suite
	// branch by setting the same-named param.
	{process: "red_phase_cycle", from: "WRITE", wantTo: "GATE_SCOPE_EXCEPTION"},
	// Layer 1 phase-scope enforcement (plan 20260518-1144 item 6): the
	// agent-triggered escape hatch fires immediately after WRITE. On
	// signal, the cycle routes to a shared STOP and loops back to WRITE
	// (current single-outflow models "Revert + rerun"; a four-option
	// decision gate is a noted follow-up).
	{process: "red_phase_cycle", from: "GATE_SCOPE_EXCEPTION", state: map[string]any{"scope_exception_requested": true}, wantTo: "STOP_SCOPE_VIOLATION", desc: "agent signalled scope exception → human STOP"},
	{process: "red_phase_cycle", from: "GATE_SCOPE_EXCEPTION", state: map[string]any{"scope_exception_requested": false}, wantTo: "STOP_RED_REVIEW", desc: "no exception signal → normal review path"},
	{process: "red_phase_cycle", from: "STOP_SCOPE_VIOLATION", wantTo: "WRITE", desc: "after STOP, revert + rerun from WRITE"},
	{process: "red_phase_cycle", from: "STOP_RED_REVIEW", wantTo: "COMPILE"},
	{process: "red_phase_cycle", from: "COMPILE", wantTo: "GATE_VERIFY_REAL_REQUIRED", desc: "COMPILE is a call_activity to the shared `compile` sub-process; compile_ok is enforced inside it, so the parent edge is unconditional. WRITE_PROTOTYPES is gone — the WRITE agent now produces a compiling failing test (test + DSL stubs together)."},
	{process: "red_phase_cycle", from: "GATE_VERIFY_REAL_REQUIRED", state: map[string]any{"verify_real_required": true}, wantTo: "VERIFY_REAL", desc: "CT_RED_TEST sets verify_real_suite → run real-suite check"},
	{process: "red_phase_cycle", from: "GATE_VERIFY_REAL_REQUIRED", state: map[string]any{"verify_real_required": false}, wantTo: "RUN", desc: "AT phases skip the verify-real branch"},
	{process: "red_phase_cycle", from: "VERIFY_REAL", wantTo: "GATE_VERIFY_REAL_PASS"},
	{process: "red_phase_cycle", from: "GATE_VERIFY_REAL_PASS", state: map[string]any{"verify_real_pass": true}, wantTo: "RUN", desc: "real-suite holds → continue to stub RUN"},
	{process: "red_phase_cycle", from: "GATE_VERIFY_REAL_PASS", state: map[string]any{"verify_real_pass": false}, wantTo: "STOP_VERIFY_REAL_FAIL", desc: "real-suite fails → STOP, contract problem"},
	{process: "red_phase_cycle", from: "STOP_VERIFY_REAL_FAIL", wantTo: "WRITE", desc: "after STOP, retry from WRITE"},
	{process: "red_phase_cycle", from: "RUN", wantTo: "GATE_RUN_FAILED_RUNTIME"},
	{process: "red_phase_cycle", from: "GATE_RUN_FAILED_RUNTIME", state: map[string]any{"tests_failed_runtime": true}, wantTo: "DISABLE"},
	{process: "red_phase_cycle", from: "GATE_RUN_FAILED_RUNTIME", state: map[string]any{"tests_failed_runtime": false}, wantTo: "STOP_RED_NOT_RUNTIME_FAIL", desc: "tests not runtime-failing → human STOP"},
	{process: "red_phase_cycle", from: "STOP_RED_NOT_RUNTIME_FAIL", wantTo: "WRITE", desc: "after STOP, retry from WRITE"},
	{process: "red_phase_cycle", from: "DISABLE", wantTo: "CHECK_PHASE_SCOPE"},
	// Layer 2 phase-scope enforcement (plan 20260518-1144 item 5): the
	// post-phase scripted diff fires after DISABLE, before COMMIT. The
	// check_phase_scope action stamps phase_scope_clean +
	// phase_scope_violating_paths; the gate consumes the boolean.
	{process: "red_phase_cycle", from: "CHECK_PHASE_SCOPE", wantTo: "GATE_PHASE_SCOPE_CLEAN"},
	{process: "red_phase_cycle", from: "GATE_PHASE_SCOPE_CLEAN", state: map[string]any{"phase_scope_clean": true}, wantTo: "COMMIT", desc: "scope clean → commit"},
	{process: "red_phase_cycle", from: "GATE_PHASE_SCOPE_CLEAN", state: map[string]any{"phase_scope_clean": false}, wantTo: "STOP_SCOPE_VIOLATION", desc: "scope violated → same STOP as Layer 1"},
	{process: "red_phase_cycle", from: "COMMIT", wantTo: "RED_END"},

	// ---- green_phase_cycle (shared by AT GREEN backend/frontend WRITEs) ----
	// Mirrors red_phase_cycle but with success-pass semantics: each gate's
	// "wrong" branch routes to a STOP for human review and loops back to
	// WRITE so the agent re-dispatches with fresh failure context. There
	// is no DISABLE/COMMIT inside — at_green_system commits backend and
	// frontend together at the parent level after both call_activities end.
	{process: "green_phase_cycle", from: "WRITE", wantTo: "GATE_SCOPE_EXCEPTION"},
	// Layer 1 phase-scope enforcement, green side (plan 20260518-1144 item
	// 6): symmetrical with the red side, but the no-signal branch routes
	// to COMPILE rather than STOP_RED_REVIEW (green has no review STOP).
	{process: "green_phase_cycle", from: "GATE_SCOPE_EXCEPTION", state: map[string]any{"scope_exception_requested": true}, wantTo: "STOP_SCOPE_VIOLATION", desc: "agent signalled scope exception → human STOP"},
	{process: "green_phase_cycle", from: "GATE_SCOPE_EXCEPTION", state: map[string]any{"scope_exception_requested": false}, wantTo: "COMPILE", desc: "no exception signal → continue to compile"},
	{process: "green_phase_cycle", from: "STOP_SCOPE_VIOLATION", wantTo: "WRITE", desc: "after STOP, revert + rerun from WRITE"},
	{process: "green_phase_cycle", from: "COMPILE", wantTo: "RUN", desc: "COMPILE is a call_activity to the shared `compile` sub-process; it returns only on compile_ok, so the parent edge is unconditional"},
	{process: "green_phase_cycle", from: "RUN", wantTo: "GATE_TESTS_PASS"},
	{process: "green_phase_cycle", from: "GATE_TESTS_PASS", state: map[string]any{"tests_pass": true}, wantTo: "CHECK_PHASE_SCOPE", desc: "all tests pass → Layer 2 scope check before parent's COMMIT"},
	{process: "green_phase_cycle", from: "GATE_TESTS_PASS", state: map[string]any{"tests_pass": false}, wantTo: "STOP_GREEN_TEST_FAIL", desc: "any test fails → human STOP"},
	{process: "green_phase_cycle", from: "STOP_GREEN_TEST_FAIL", wantTo: "WRITE", desc: "after STOP, retry from WRITE"},
	// Layer 2 phase-scope enforcement, green side (plan 20260518-1144 item
	// 5): fires after tests pass, before GREEN_END returns control to the
	// parent (which owns COMMIT for the full-stack-as-one-commit convention).
	{process: "green_phase_cycle", from: "CHECK_PHASE_SCOPE", wantTo: "GATE_PHASE_SCOPE_CLEAN"},
	{process: "green_phase_cycle", from: "GATE_PHASE_SCOPE_CLEAN", state: map[string]any{"phase_scope_clean": true}, wantTo: "GREEN_END", desc: "scope clean → end"},
	{process: "green_phase_cycle", from: "GATE_PHASE_SCOPE_CLEAN", state: map[string]any{"phase_scope_clean": false}, wantTo: "STOP_SCOPE_VIOLATION", desc: "scope violated → same STOP as Layer 1"},

	// ---- compile (shared sub-process: pairs COMPILE with GATE + human-gated FIX) ----
	// Every compile in the orchestration is dispatched through this sub-process;
	// extracting it makes "compile failure is human-gated before a fixer is
	// dispatched" a structural invariant rather than a pattern any caller could
	// quietly break. Same construct as `commit` — callers reference the
	// sub-process, not the underlying mechanical task. Resolves only when
	// compile_ok; compile-fail trips STOP → FIX_COMPILE → COMPILE retry loop.
	{process: "compile", from: "COMPILE", wantTo: "GATE_COMPILE_OK"},
	{process: "compile", from: "GATE_COMPILE_OK", state: map[string]any{"compile_ok": true}, wantTo: "COMPILE_END", desc: "compile ok → resolve the sub-process"},
	{process: "compile", from: "GATE_COMPILE_OK", state: map[string]any{"compile_ok": false}, wantTo: "STOP_COMPILE_FAIL_REVIEW", desc: "compile fail halts at a human review STOP before fix-agent dispatch"},
	{process: "compile", from: "STOP_COMPILE_FAIL_REVIEW", wantTo: "FIX_COMPILE", desc: "human approves the dispatch and the fix agent runs (fix-verify in STRUCT; the cycle's WRITE agent in RED/GREEN)"},
	{process: "compile", from: "FIX_COMPILE", wantTo: "COMPILE", desc: "fix agent loops back to COMPILE for re-verify"},

	// ---- commit (shared sub-process: pairs APPROVE_COMMIT with EXECUTE_COMMIT) ----
	// Every commit in the orchestration is dispatched through this sub-process;
	// extracting it makes "approval precedes execution" a structural invariant
	// rather than a pattern any caller could quietly break.
	{process: "commit", from: "APPROVE_COMMIT", wantTo: "EXECUTE_COMMIT"},
	{process: "commit", from: "EXECUTE_COMMIT", wantTo: "COMMIT_END"},

	// ---- legacy_acceptance_criteria ----
	// Dispatch wrapper for the legacy coverage cycle. Two sequential presence
	// gates branch into legacy_at_cycle / legacy_ct_cycle; tickets that carry
	// both kinds of legacy criteria run both sub-cycles. See
	// plans/20260518-1116-legacy-coverage-cycle.md item 1b.
	{process: "legacy_acceptance_criteria", from: "GATE_LEGACY_AT_PRESENT", state: map[string]any{"legacy_at_acceptance_criteria_present": true}, wantTo: "LEGACY_AT_CYCLE"},
	{process: "legacy_acceptance_criteria", from: "GATE_LEGACY_AT_PRESENT", state: map[string]any{"legacy_at_acceptance_criteria_present": false}, wantTo: "GATE_LEGACY_CT_PRESENT"},
	{process: "legacy_acceptance_criteria", from: "LEGACY_AT_CYCLE", wantTo: "GATE_LEGACY_CT_PRESENT"},
	{process: "legacy_acceptance_criteria", from: "GATE_LEGACY_CT_PRESENT", state: map[string]any{"legacy_ct_acceptance_criteria_present": true}, wantTo: "LEGACY_CT_CYCLE"},
	{process: "legacy_acceptance_criteria", from: "GATE_LEGACY_CT_PRESENT", state: map[string]any{"legacy_ct_acceptance_criteria_present": false}, wantTo: "LEGACY_END"},
	{process: "legacy_acceptance_criteria", from: "LEGACY_CT_CYCLE", wantTo: "LEGACY_END"},

	// ---- legacy_at_cycle ----
	// Legacy AT coverage cycle. Mirrors at_cycle's RED-side shape (test →
	// DSL → system driver) with the existing interface-changed flags gating
	// which layers run; ends with an inverted-RED verify gate. On red verify,
	// route to STOP - HUMAN REVIEW (no loopback) — operator edits the
	// offending layer and re-runs the legacy cycle from scratch.
	{process: "legacy_at_cycle", from: "LEGACY_AT_TEST", wantTo: "GATE_DSL_LEGACY_AT"},
	{process: "legacy_at_cycle", from: "GATE_DSL_LEGACY_AT", state: map[string]any{"dsl_interface_changed": true}, wantTo: "LEGACY_AT_DSL"},
	{process: "legacy_at_cycle", from: "GATE_DSL_LEGACY_AT", state: map[string]any{"dsl_interface_changed": false}, wantTo: "GATE_SYS_LEGACY_AT"},
	{process: "legacy_at_cycle", from: "LEGACY_AT_DSL", wantTo: "GATE_SYS_LEGACY_AT"},
	{process: "legacy_at_cycle", from: "GATE_SYS_LEGACY_AT", state: map[string]any{"system_driver_interface_changed": true}, wantTo: "LEGACY_AT_SYSTEM_DRIVER"},
	{process: "legacy_at_cycle", from: "GATE_SYS_LEGACY_AT", state: map[string]any{"system_driver_interface_changed": false}, wantTo: "VERIFY_LEGACY_AT"},
	{process: "legacy_at_cycle", from: "LEGACY_AT_SYSTEM_DRIVER", wantTo: "VERIFY_LEGACY_AT"},
	{process: "legacy_at_cycle", from: "VERIFY_LEGACY_AT", wantTo: "GATE_VERIFY_LEGACY_AT"},
	{process: "legacy_at_cycle", from: "GATE_VERIFY_LEGACY_AT", state: map[string]any{"legacy_at_verify_outcome": "ok"}, wantTo: "LEGACY_AT_END", desc: "inverted-RED: assembled test passed on first run as expected"},
	{process: "legacy_at_cycle", from: "GATE_VERIFY_LEGACY_AT", state: map[string]any{"legacy_at_verify_outcome": "red"}, wantTo: "STOP_LEGACY_AT_VERIFY_FAILED", desc: "inverted-RED fail: test/DSL/driver is suspect, SUT never modified"},
	{process: "legacy_at_cycle", from: "STOP_LEGACY_AT_VERIFY_FAILED", wantTo: "LEGACY_AT_END"},

	// ---- legacy_ct_cycle ----
	// Legacy CT coverage cycle. Mirrors ct_subprocess's RED-side shape (test
	// → DSL → external driver) with the existing interface-changed flags
	// gating which layers run; the external-system stub phase is always run
	// (stub is test infrastructure, not production code). Ends with an
	// inverted-RED verify gate.
	{process: "legacy_ct_cycle", from: "LEGACY_CT_TEST", wantTo: "GATE_DSL_LEGACY_CT"},
	{process: "legacy_ct_cycle", from: "GATE_DSL_LEGACY_CT", state: map[string]any{"dsl_interface_changed": true}, wantTo: "LEGACY_CT_DSL"},
	{process: "legacy_ct_cycle", from: "GATE_DSL_LEGACY_CT", state: map[string]any{"dsl_interface_changed": false}, wantTo: "LEGACY_CT_EXTERNAL_SYSTEM_STUB"},
	{process: "legacy_ct_cycle", from: "LEGACY_CT_DSL", wantTo: "GATE_EXT_LEGACY_CT"},
	{process: "legacy_ct_cycle", from: "GATE_EXT_LEGACY_CT", state: map[string]any{"external_system_driver_interface_changed": true}, wantTo: "LEGACY_CT_EXTERNAL_SYSTEM_DRIVER"},
	{process: "legacy_ct_cycle", from: "GATE_EXT_LEGACY_CT", state: map[string]any{"external_system_driver_interface_changed": false}, wantTo: "LEGACY_CT_EXTERNAL_SYSTEM_STUB"},
	{process: "legacy_ct_cycle", from: "LEGACY_CT_EXTERNAL_SYSTEM_DRIVER", wantTo: "LEGACY_CT_EXTERNAL_SYSTEM_STUB"},
	{process: "legacy_ct_cycle", from: "LEGACY_CT_EXTERNAL_SYSTEM_STUB", wantTo: "VERIFY_LEGACY_CT"},
	{process: "legacy_ct_cycle", from: "VERIFY_LEGACY_CT", wantTo: "GATE_VERIFY_LEGACY_CT"},
	{process: "legacy_ct_cycle", from: "GATE_VERIFY_LEGACY_CT", state: map[string]any{"legacy_ct_verify_outcome": "ok"}, wantTo: "LEGACY_CT_END", desc: "inverted-RED: assembled test passed on first run as expected"},
	{process: "legacy_ct_cycle", from: "GATE_VERIFY_LEGACY_CT", state: map[string]any{"legacy_ct_verify_outcome": "red"}, wantTo: "STOP_LEGACY_CT_VERIFY_FAILED", desc: "inverted-RED fail: test/DSL/driver/stub is suspect, SUT never modified"},
	{process: "legacy_ct_cycle", from: "STOP_LEGACY_CT_VERIFY_FAILED", wantTo: "LEGACY_CT_END"},

	// refactor-system-structure (Phase C.1 prototype — five-level BPMN refactor)
	{process: "refactor-system-structure", from: "IMPLEMENT_AND_VERIFY_SYSTEM", wantTo: "RSS_END"},
}

func TestTransitions(t *testing.T) {
	eng := loadSnapshot(t)
	for _, tc := range transitionTable {
		t.Run(tc.process+"/"+tc.from+"->"+tc.wantTo, func(t *testing.T) {
			process, ok := eng.Processes[tc.process]
			if !ok {
				t.Fatalf("process %q not in loaded engine", tc.process)
			}
			ctx := NewContext()
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			if tc.params != nil {
				ctx.Params = tc.params
			}
			got, err := eng.nextEdge(process, tc.from, ctx)
			if err != nil {
				t.Fatalf("nextEdge from %q: %v", tc.from, err)
			}
			if got != tc.wantTo {
				t.Errorf("from %q under state %v: got next=%q, want %q (%s)", tc.from, tc.state, got, tc.wantTo, tc.desc)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Coverage assertion — every documented edge has at least one positive case
// ---------------------------------------------------------------------------

func TestTransitionTable_CoversEverySequenceFlow(t *testing.T) {
	eng := loadSnapshot(t)
	covered := make(map[string]bool)
	for _, tc := range transitionTable {
		key := tc.process + ":" + tc.from + "->" + tc.wantTo
		covered[key] = true
	}
	for name, process := range eng.Processes {
		for _, edge := range process.Edges {
			key := name + ":" + edge.From + "->" + edge.To
			if !covered[key] {
				t.Errorf("uncovered edge in process %q: %s -> %s (when=%q)", name, edge.From, edge.To, edge.Predicate)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Process-audit gap decisions — explicit anchors so they cannot drift back
// ---------------------------------------------------------------------------

func TestGapDecision_CTExitReturnsToSystemDriverGate(t *testing.T) {
	eng := loadSnapshot(t)
	process := eng.Processes["at_cycle"]
	for _, edge := range process.OutgoingByNode["CT_SUBPROCESS"] {
		if edge.To != "GATE_SYS_AT" {
			t.Errorf("CT_SUBPROCESS exit edge: got to=%q, want GATE_SYS_AT", edge.To)
		}
	}
}

func TestGapDecision_SmokeTestFailStopsAtAskSupport(t *testing.T) {
	eng := loadSnapshot(t)
	process := eng.Processes["external_system_onboarding"]
	ctx := NewContext()
	ctx.Set("smoke_test_passes", false)
	got, err := eng.nextEdge(process, "GATE_SMOKE_PASS", ctx)
	if err != nil {
		t.Fatalf("nextEdge: %v", err)
	}
	if got != "ASK_SUPPORT" {
		t.Errorf("smoke fail: got next=%q, want ASK_SUPPORT (no auto-resume)", got)
	}
}

func TestGapDecision_RunCycleRoutesByChangeType(t *testing.T) {
	// run_cycle is single-axis: a derived `change_type` decides between
	// AT / DA / SUT cycles. The previous two-gate (ticket_type →
	// subtype) shape is collapsed to one. The deleted change_subtype /
	// change_scope / change_channel fields are gone; the only carried
	// axis is change_type ∈ {behavioral, system-interface-redesign,
	// external-system-interface-redesign, system-implementation-refactoring}.
	eng := loadSnapshot(t)
	process := eng.Processes["run_cycle"]
	if process == nil {
		t.Fatalf("run_cycle process missing")
	}
	top, ok := process.Nodes["GATE_CHANGE_TYPE"]
	if !ok {
		t.Fatalf("GATE_CHANGE_TYPE node missing from run_cycle")
	}
	if top.Kind != Gateway || top.Raw.Binding != "change_type" {
		t.Errorf("GATE_CHANGE_TYPE: kind=%v binding=%q, want Gateway/change_type", top.Kind, top.Raw.Binding)
	}
	for id, node := range process.Nodes {
		if node.Kind != Gateway {
			continue
		}
		switch node.Raw.Binding {
		case "change_subtype", "change_scope", "change_channel":
			t.Errorf("run_cycle gate %q still binds to deprecated %q", id, node.Raw.Binding)
		}
	}
}

func TestGapDecision_StubsOwnershipPlaceholder(t *testing.T) {
	// Stubs ownership is a recorded TBD — the YAML currently uses
	// `agent: ct-green-external-system-stub` as a placeholder. Lock that here so
	// a future edit that resolves the gap will fail this test, prompting an
	// explicit update + decision record.
	eng := loadSnapshot(t)
	stubs := eng.Processes["ct_subprocess"].Nodes["CT_GREEN_EXTERNAL_SYSTEM_STUB"]
	if stubs.Raw.Agent != "ct-green-external-system-stub" {
		t.Errorf("CT_GREEN_EXTERNAL_SYSTEM_STUB agent: got %q, want %q (placeholder pending stubs-ownership decision)", stubs.Raw.Agent, "ct-green-external-system-stub")
	}
}

// ---------------------------------------------------------------------------
// Predicate evaluator unit tests
// ---------------------------------------------------------------------------

func TestPredicate_EmptyAlwaysTrue(t *testing.T) {
	ctx := NewContext()
	if ok, err := evalPredicate("", ctx); !ok || err != nil {
		t.Errorf("empty predicate: got ok=%v err=%v, want true/nil", ok, err)
	}
}

func TestPredicate_Equality(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "story")
	cases := []struct {
		expr string
		want bool
	}{
		{"ticket_type == story", true},
		{"ticket_type == bug", false},
		{`ticket_type == "story"`, true},
	}
	for _, c := range cases {
		ok, err := evalPredicate(c.expr, ctx)
		if err != nil {
			t.Errorf("expr %q: %v", c.expr, err)
		}
		if ok != c.want {
			t.Errorf("expr %q: got %v, want %v", c.expr, ok, c.want)
		}
	}
}

func TestPredicate_BoolEquality(t *testing.T) {
	ctx := NewContext()
	ctx.Set("dsl_interface_changed", true)
	if ok, _ := evalPredicate("dsl_interface_changed == true", ctx); !ok {
		t.Errorf("expected dsl_interface_changed == true to match")
	}
	if ok, _ := evalPredicate("dsl_interface_changed == false", ctx); ok {
		t.Errorf("expected dsl_interface_changed == false to NOT match")
	}
}

func TestPredicate_InList(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "bug")
	expr := "ticket_type in [story, bug]"
	ok, err := evalPredicate(expr, ctx)
	if err != nil {
		t.Errorf("expr %q: %v", expr, err)
	}
	if !ok {
		t.Errorf("expected %q to match", expr)
	}
}

func TestPredicate_InListNegative(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "task")
	expr := "ticket_type in [story, bug]"
	ok, _ := evalPredicate(expr, ctx)
	if ok {
		t.Errorf("expected task to NOT be in behavioral list")
	}
}

// ---------------------------------------------------------------------------
// Engine-level Run smoke test (uses humanStop dispatcher implicitly via the
// agents.Registry built-in, except we don't bind here — instead we wire
// stub NodeFns directly to keep the test deterministic).
// ---------------------------------------------------------------------------

func TestEngine_RunProcess_LegacyAcceptanceCriteria_StopsCleanly(t *testing.T) {
	eng := loadSnapshot(t)
	// Bind every user_task to a no-op so legacy_acceptance_criteria runs without
	// blocking on stdin; gateways/actions aren't reached in this process.
	noop := func(ctx *Context) Outcome { return Outcome{} }
	eng.AgentFn = func(name string) NodeFn { return noop }
	eng.ActionFn = func(name string) NodeFn { return noop }
	eng.GateFn = func(name string) NodeFn { return noop }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := eng.RunProcess("legacy_acceptance_criteria", NewContext()); err != nil {
		t.Errorf("RunProcess legacy_acceptance_criteria: %v", err)
	}
}
