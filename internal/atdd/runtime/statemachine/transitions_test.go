// Transitions test suite for the ATDD process-flow YAML.
//
// Strategy:
//   - Load the canonical embedded YAML via LoadDefault.
//   - Assert structural invariants over every flow (start exists, edges
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

func TestLoadSnapshot_AllFlowsParse(t *testing.T) {
	eng := loadSnapshot(t)
	wantFlows := []string{
		"main",
		"github_intake",
		"run_legacy_cycle",
		"run_cycle",
		"at_cycle",
		"at_green_system",
		"da_cycle",
		"sut_cycle",
		"ct_subprocess",
		"external_system_onboarding",
		"structural_cycle",
		"red_phase_cycle",
		"legacy_acceptance_criteria",
	}
	for _, name := range wantFlows {
		if _, ok := eng.Flows[name]; !ok {
			t.Errorf("flow %q missing from loaded snapshot", name)
		}
	}
}

func TestStructuralIntegrity_StartNodesExist(t *testing.T) {
	eng := loadSnapshot(t)
	for name, flow := range eng.Flows {
		if _, ok := flow.Nodes[flow.Start]; !ok {
			t.Errorf("flow %q: start node %q not in nodes list", name, flow.Start)
		}
	}
}

func TestStructuralIntegrity_GatewaysHaveOutgoingEdges(t *testing.T) {
	eng := loadSnapshot(t)
	for name, flow := range eng.Flows {
		for id, node := range flow.Nodes {
			if node.Kind != Gateway {
				continue
			}
			if len(flow.OutgoingByNode[id]) == 0 {
				t.Errorf("flow %q gateway %q has no outgoing edges", name, id)
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
	for name, flow := range eng.Flows {
		for id, node := range flow.Nodes {
			if node.Kind == EndEvent {
				continue
			}
			if len(flow.OutgoingByNode[id]) > 0 {
				continue
			}
			if node.Kind == UserTask && node.Raw.Agent == "human" {
				continue // intentional STOP-and-halt
			}
			t.Errorf("flow %q non-end node %q has no outgoing edges", name, id)
		}
	}
}

// ---------------------------------------------------------------------------
// Transition table — one row per documented sequence flow
// ---------------------------------------------------------------------------

type transitionCase struct {
	flow   string
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
	// ---- main flow ----
	{flow: "main", from: "START", state: map[string]any{"mode": "board"}, wantTo: "PICK_TOP_READY", desc: "board mode enters via PICK_TOP_READY"},
	{flow: "main", from: "START", state: map[string]any{"mode": "specific_issue"}, wantTo: "MOVE_TO_IN_PROGRESS", desc: "specific-issue mode skips PICK_TOP_READY"},
	{flow: "main", from: "PICK_TOP_READY", wantTo: "MOVE_TO_IN_PROGRESS"},
	{flow: "main", from: "MOVE_TO_IN_PROGRESS", wantTo: "INTAKE"},
	{flow: "main", from: "INTAKE", wantTo: "RUN_LEGACY_CYCLE"},
	{flow: "main", from: "RUN_LEGACY_CYCLE", wantTo: "RUN_CYCLE"},
	{flow: "main", from: "RUN_CYCLE", wantTo: "TICKET_IN_ACCEPTANCE"},
	{flow: "main", from: "TICKET_IN_ACCEPTANCE", wantTo: "END"},

	// ---- intake ----
	// Issue Forms enforce the ticket schema upstream so intake is a pure
	// service-task pipeline (classify ticket, classify subtype, parse body)
	// with two STOPs for unhappy paths (classification conflict, parse
	// error). No LLM dispatch — no agent fan-out by ticket type.
	{flow: "github_intake", from: "CLASSIFY", wantTo: "GATE_CLASSIFY_CONFIDENT"},
	{flow: "github_intake", from: "GATE_CLASSIFY_CONFIDENT", state: map[string]any{"ticket_type_recognized": true}, wantTo: "GATE_TICKET_TYPE_INTAKE"},
	{flow: "github_intake", from: "GATE_CLASSIFY_CONFIDENT", state: map[string]any{"ticket_type_recognized": false}, wantTo: "STOP_CLASSIFY_CONFLICT"},
	{flow: "github_intake", from: "STOP_CLASSIFY_CONFLICT", wantTo: "CLASSIFY"},
	{flow: "github_intake", from: "GATE_TICKET_TYPE_INTAKE", state: map[string]any{"ticket_type": "story"}, wantTo: "PARSE_BODY"},
	{flow: "github_intake", from: "GATE_TICKET_TYPE_INTAKE", state: map[string]any{"ticket_type": "bug"}, wantTo: "PARSE_BODY"},
	{flow: "github_intake", from: "GATE_TICKET_TYPE_INTAKE", state: map[string]any{"ticket_type": "task"}, wantTo: "CLASSIFY_SUBTYPE"},
	{flow: "github_intake", from: "CLASSIFY_SUBTYPE", wantTo: "GATE_SUBTYPE_OK"},
	{flow: "github_intake", from: "GATE_SUBTYPE_OK", state: map[string]any{"subtype_ok": true}, wantTo: "PARSE_BODY"},
	{flow: "github_intake", from: "GATE_SUBTYPE_OK", state: map[string]any{"subtype_ok": false}, wantTo: "STOP_SUBTYPE_MISSING"},
	{flow: "github_intake", from: "STOP_SUBTYPE_MISSING", wantTo: "CLASSIFY_SUBTYPE"},
	{flow: "github_intake", from: "PARSE_BODY", wantTo: "GATE_PARSE_OK"},
	{flow: "github_intake", from: "GATE_PARSE_OK", state: map[string]any{"parse_ok": true}, wantTo: "REPORT_INTAKE_SUMMARY"},
	{flow: "github_intake", from: "GATE_PARSE_OK", state: map[string]any{"parse_ok": false}, wantTo: "STOP_PARSE_ERROR"},
	{flow: "github_intake", from: "STOP_PARSE_ERROR", wantTo: "PARSE_BODY"},
	{flow: "github_intake", from: "REPORT_INTAKE_SUMMARY", wantTo: "INTAKE_END"},

	// ---- run_legacy_cycle ----
	// Backfill cycle for legacy acceptance criteria. Self-contained: gates internally on
	// presence and no-ops when absent so main can call it unconditionally.
	{flow: "run_legacy_cycle", from: "GATE_LEGACY_PRESENT", state: map[string]any{"legacy_acceptance_criteria_section_present": true}, wantTo: "LEGACY_CYCLE"},
	{flow: "run_legacy_cycle", from: "GATE_LEGACY_PRESENT", state: map[string]any{"legacy_acceptance_criteria_section_present": false}, wantTo: "RUN_LEGACY_END"},
	{flow: "run_legacy_cycle", from: "LEGACY_CYCLE", wantTo: "RUN_LEGACY_END"},

	// ---- run_cycle ----
	// Change cycle dispatch — single-axis gate on the derived `change_type`.
	// Four branches: AT_CYCLE (behavioral), DA_CYCLE (system /
	// external-system interface redesign), SUT_CYCLE
	// (system-implementation-change).
	{flow: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "behavioral"}, wantTo: "AT_CYCLE"},
	{flow: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "system-interface-redesign"}, wantTo: "DA_CYCLE"},
	{flow: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "external-system-interface-redesign"}, wantTo: "DA_CYCLE"},
	{flow: "run_cycle", from: "GATE_CHANGE_TYPE", state: map[string]any{"change_type": "system-implementation-change"}, wantTo: "SUT_CYCLE"},
	{flow: "run_cycle", from: "AT_CYCLE", wantTo: "CYCLE_END"},
	{flow: "run_cycle", from: "DA_CYCLE", wantTo: "CYCLE_END"},
	{flow: "run_cycle", from: "SUT_CYCLE", wantTo: "CYCLE_END"},

	// ---- da_cycle ----
	// Driver Adapter cycle. Splits on `change_type`: system-interface-redesign →
	// shared structural_cycle (with the WRITE agent figuring out which
	// driver to modify); external-system-interface-redesign → ct_subprocess.
	{flow: "da_cycle", from: "GATE_CHANGE_TYPE_DA", state: map[string]any{"change_type": "system-interface-redesign"}, wantTo: "SYSTEM_INTERFACE_REDESIGN_CYCLE"},
	{flow: "da_cycle", from: "GATE_CHANGE_TYPE_DA", state: map[string]any{"change_type": "external-system-interface-redesign"}, wantTo: "EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE"},
	{flow: "da_cycle", from: "SYSTEM_INTERFACE_REDESIGN_CYCLE", wantTo: "DA_END"},
	{flow: "da_cycle", from: "EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE", wantTo: "DA_END"},

	// ---- sut_cycle ----
	// System Under Test cycle. Single node calling structural_cycle with
	// chore-flavour params.
	{flow: "sut_cycle", from: "CHORE_CYCLE", wantTo: "SUT_END"},

	// ---- at_cycle ----
	{flow: "at_cycle", from: "AT_RED_TEST", wantTo: "GATE_DSL_AT"},
	{flow: "at_cycle", from: "GATE_DSL_AT", state: map[string]any{"dsl_interface_changed": false}, wantTo: "AT_GREEN_SYSTEM"},
	{flow: "at_cycle", from: "GATE_DSL_AT", state: map[string]any{"dsl_interface_changed": true}, wantTo: "AT_RED_DSL"},
	{flow: "at_cycle", from: "AT_RED_DSL", wantTo: "GATE_EXT_AT"},
	{flow: "at_cycle", from: "GATE_EXT_AT", state: map[string]any{"external_system_driver_interface_changed": true}, wantTo: "CT_SUBPROCESS"},
	{flow: "at_cycle", from: "GATE_EXT_AT", state: map[string]any{"external_system_driver_interface_changed": false}, wantTo: "GATE_SYS_AT"},
	// CT exit re-evaluation: process-audit gap resolved — CT_SUBPROCESS returns
	// to GATE_SYS_AT so System Driver changes are still routed through after CT.
	{flow: "at_cycle", from: "CT_SUBPROCESS", wantTo: "GATE_SYS_AT", desc: "CT exit re-evaluates system_driver_interface_changed (process-audit gap resolved)"},
	{flow: "at_cycle", from: "GATE_SYS_AT", state: map[string]any{"system_driver_interface_changed": true}, wantTo: "AT_RED_SYSTEM_DRIVER"},
	{flow: "at_cycle", from: "GATE_SYS_AT", state: map[string]any{"system_driver_interface_changed": false}, wantTo: "AT_GREEN_SYSTEM"},
	{flow: "at_cycle", from: "AT_RED_SYSTEM_DRIVER", wantTo: "VERIFY_AT_DRIVER"},
	{flow: "at_cycle", from: "VERIFY_AT_DRIVER", wantTo: "AT_GREEN_SYSTEM"},
	{flow: "at_cycle", from: "AT_GREEN_SYSTEM", wantTo: "AT_END"},

	// ---- at_green_system ----
	{flow: "at_green_system", from: "ATDD_BACKEND", wantTo: "ATDD_FRONTEND"},
	{flow: "at_green_system", from: "ATDD_FRONTEND", wantTo: "STOP_GREEN_REVIEW"},
	{flow: "at_green_system", from: "STOP_GREEN_REVIEW", wantTo: "ATDD_RELEASE"},
	{flow: "at_green_system", from: "ATDD_RELEASE", wantTo: "GS_END"},

	// ---- ct_subprocess ----
	{flow: "ct_subprocess", from: "ONBOARDING", wantTo: "CT_RED_TEST"},
	{flow: "ct_subprocess", from: "CT_RED_TEST", wantTo: "GATE_DSL_CT"},
	{flow: "ct_subprocess", from: "GATE_DSL_CT", state: map[string]any{"dsl_interface_changed": false}, wantTo: "CT_GREEN_STUBS"},
	{flow: "ct_subprocess", from: "GATE_DSL_CT", state: map[string]any{"dsl_interface_changed": true}, wantTo: "CT_RED_DSL"},
	{flow: "ct_subprocess", from: "CT_RED_DSL", wantTo: "GATE_EXT_CT"},
	{flow: "ct_subprocess", from: "GATE_EXT_CT", state: map[string]any{"external_system_driver_interface_changed": false}, wantTo: "CT_GREEN_STUBS"},
	{flow: "ct_subprocess", from: "GATE_EXT_CT", state: map[string]any{"external_system_driver_interface_changed": true}, wantTo: "CT_RED_EXTERNAL_DRIVER"},
	{flow: "ct_subprocess", from: "CT_RED_EXTERNAL_DRIVER", wantTo: "VERIFY_CT_DRIVER"},
	{flow: "ct_subprocess", from: "VERIFY_CT_DRIVER", wantTo: "CT_GREEN_STUBS"},
	{flow: "ct_subprocess", from: "CT_GREEN_STUBS", wantTo: "CT_END"},

	// ---- external_system_onboarding ----
	// Smoke-test resume path: process-audit gap resolved — when the smoke
	// test fails the run STOPs at ASK_SUPPORT (no resume; user must pair).
	{flow: "external_system_onboarding", from: "GATE_DRIVER_EXISTS", state: map[string]any{"external_system_driver_exists": true}, wantTo: "ONBOARD_END", desc: "early return when driver already exists"},
	{flow: "external_system_onboarding", from: "GATE_DRIVER_EXISTS", state: map[string]any{"external_system_driver_exists": false}, wantTo: "GATE_INSTANCE_ACCESSIBLE"},
	{flow: "external_system_onboarding", from: "GATE_INSTANCE_ACCESSIBLE", state: map[string]any{"external_system_test_instance_accessible": true}, wantTo: "DEFINE_IFACE"},
	{flow: "external_system_onboarding", from: "GATE_INSTANCE_ACCESSIBLE", state: map[string]any{"external_system_test_instance_accessible": false}, wantTo: "PROVISION"},
	{flow: "external_system_onboarding", from: "PROVISION", wantTo: "DEFINE_IFACE"},
	{flow: "external_system_onboarding", from: "DEFINE_IFACE", wantTo: "IMPL_DRIVER"},
	{flow: "external_system_onboarding", from: "IMPL_DRIVER", wantTo: "WRITE_SMOKE"},
	{flow: "external_system_onboarding", from: "WRITE_SMOKE", wantTo: "RUN_SMOKE"},
	{flow: "external_system_onboarding", from: "RUN_SMOKE", wantTo: "GATE_SMOKE_PASS"},
	{flow: "external_system_onboarding", from: "GATE_SMOKE_PASS", state: map[string]any{"smoke_test_passes": false}, wantTo: "ASK_SUPPORT", desc: "smoke fail → STOP and ask user (no auto-resume)"},
	{flow: "external_system_onboarding", from: "GATE_SMOKE_PASS", state: map[string]any{"smoke_test_passes": true}, wantTo: "STOP_ONBOARD_REVIEW"},
	{flow: "external_system_onboarding", from: "STOP_ONBOARD_REVIEW", wantTo: "COMMIT_ONBOARD"},
	{flow: "external_system_onboarding", from: "COMMIT_ONBOARD", wantTo: "ONBOARD_END"},

	// ---- structural_cycle (shared by SYSAPI / SYSUI / CHORE via params) ----
	// Structural-cycle escape: process-audit gap resolved — the TEST=skip
	// branch jumps directly to ASK_COMMIT, bypassing both COMPILE/SAMPLE and
	// the second STOP_STRUCT_TEST review.
	{flow: "structural_cycle", from: "STRUCT_WRITE", wantTo: "VERIFY_STRUCT_DRIVER"},
	{flow: "structural_cycle", from: "VERIFY_STRUCT_DRIVER", wantTo: "GATE_STRUCT_VERIFY"},
	{flow: "structural_cycle", from: "GATE_STRUCT_VERIFY", state: map[string]any{"structural_verify_outcome": "ok"}, wantTo: "STOP_STRUCT_REVIEW", desc: "ok class continues to human review"},
	{flow: "structural_cycle", from: "GATE_STRUCT_VERIFY", state: map[string]any{"structural_verify_outcome": "red"}, wantTo: "FIX_STRUCT_VERIFY", desc: "red class dispatches the fix-verify agent (one retry)"},
	{flow: "structural_cycle", from: "FIX_STRUCT_VERIFY", wantTo: "VERIFY_STRUCT_DRIVER", desc: "fix agent loops back into verify for re-classification"},
	{flow: "structural_cycle", from: "STOP_STRUCT_REVIEW", wantTo: "GATE_TEST_MODE"},
	{flow: "structural_cycle", from: "GATE_TEST_MODE", state: map[string]any{"structural_test_mode": "skip"}, wantTo: "ASK_COMMIT", desc: "skip mode escapes the TEST sub-loop entirely"},
	{flow: "structural_cycle", from: "GATE_TEST_MODE", state: map[string]any{"structural_test_mode": "compile"}, wantTo: "COMPILE"},
	{flow: "structural_cycle", from: "GATE_TEST_MODE", state: map[string]any{"structural_test_mode": "full"}, wantTo: "COMPILE"},
	{flow: "structural_cycle", from: "COMPILE", state: map[string]any{"structural_test_mode": "full"}, wantTo: "SAMPLE"},
	{flow: "structural_cycle", from: "COMPILE", state: map[string]any{"structural_test_mode": "compile"}, wantTo: "DRIFT"},
	{flow: "structural_cycle", from: "SAMPLE", wantTo: "DRIFT"},
	{flow: "structural_cycle", from: "DRIFT", wantTo: "STOP_STRUCT_TEST"},
	{flow: "structural_cycle", from: "STOP_STRUCT_TEST", wantTo: "ASK_COMMIT"},
	{flow: "structural_cycle", from: "ASK_COMMIT", wantTo: "COMMIT_STRUCT"},
	{flow: "structural_cycle", from: "COMMIT_STRUCT", wantTo: "TICK"},
	{flow: "structural_cycle", from: "TICK", wantTo: "STRUCT_END"},

	// ---- red_phase_cycle (shared by AT/CT RED-WRITE phases via params) ----
	// Splits creative WRITE work from mechanical compile/run/disable/commit.
	// All three AT RED phases and all three CT RED phases call into this
	// flow; CT_RED_TEST additionally enables the optional verify_real_suite
	// branch by setting the same-named param.
	{flow: "red_phase_cycle", from: "WRITE", wantTo: "STOP_RED_REVIEW"},
	{flow: "red_phase_cycle", from: "STOP_RED_REVIEW", wantTo: "COMPILE"},
	{flow: "red_phase_cycle", from: "COMPILE", wantTo: "GATE_COMPILE_OK"},
	{flow: "red_phase_cycle", from: "GATE_COMPILE_OK", state: map[string]any{"compile_ok": false}, wantTo: "WRITE_PROTOTYPES", desc: "compile fail loops through prototype WRITE"},
	{flow: "red_phase_cycle", from: "GATE_COMPILE_OK", state: map[string]any{"compile_ok": true}, wantTo: "GATE_VERIFY_REAL_REQUIRED", desc: "compile pass enters the optional verify-real branch"},
	{flow: "red_phase_cycle", from: "WRITE_PROTOTYPES", wantTo: "STOP_PROTOTYPE_REVIEW"},
	{flow: "red_phase_cycle", from: "STOP_PROTOTYPE_REVIEW", wantTo: "COMPILE", desc: "prototype loop returns to COMPILE"},
	{flow: "red_phase_cycle", from: "GATE_VERIFY_REAL_REQUIRED", state: map[string]any{"verify_real_required": true}, wantTo: "VERIFY_REAL", desc: "CT_RED_TEST sets verify_real_suite → run real-suite check"},
	{flow: "red_phase_cycle", from: "GATE_VERIFY_REAL_REQUIRED", state: map[string]any{"verify_real_required": false}, wantTo: "RUN", desc: "AT phases skip the verify-real branch"},
	{flow: "red_phase_cycle", from: "VERIFY_REAL", wantTo: "GATE_VERIFY_REAL_PASS"},
	{flow: "red_phase_cycle", from: "GATE_VERIFY_REAL_PASS", state: map[string]any{"verify_real_pass": true}, wantTo: "RUN", desc: "real-suite holds → continue to stub RUN"},
	{flow: "red_phase_cycle", from: "GATE_VERIFY_REAL_PASS", state: map[string]any{"verify_real_pass": false}, wantTo: "STOP_VERIFY_REAL_FAIL", desc: "real-suite fails → STOP, contract problem"},
	{flow: "red_phase_cycle", from: "STOP_VERIFY_REAL_FAIL", wantTo: "WRITE", desc: "after STOP, retry from WRITE"},
	{flow: "red_phase_cycle", from: "RUN", wantTo: "GATE_RUN_FAILED_RUNTIME"},
	{flow: "red_phase_cycle", from: "GATE_RUN_FAILED_RUNTIME", state: map[string]any{"tests_failed_runtime": true}, wantTo: "DISABLE"},
	{flow: "red_phase_cycle", from: "GATE_RUN_FAILED_RUNTIME", state: map[string]any{"tests_failed_runtime": false}, wantTo: "STOP_RED_NOT_RUNTIME_FAIL", desc: "tests not runtime-failing → human STOP"},
	{flow: "red_phase_cycle", from: "STOP_RED_NOT_RUNTIME_FAIL", wantTo: "WRITE", desc: "after STOP, retry from WRITE"},
	{flow: "red_phase_cycle", from: "DISABLE", wantTo: "COMMIT"},
	{flow: "red_phase_cycle", from: "COMMIT", wantTo: "RED_END"},

	// ---- legacy_acceptance_criteria ----
	// Legacy Acceptance Criteria Cycle interim spec: a single STOP node, per
	// glossary.md TBD. Locked here so the placeholder cannot silently
	// regress to "TBD" by accident.
	{flow: "legacy_acceptance_criteria", from: "LEGACY_TBD", wantTo: "LEGACY_END", desc: "interim spec: single human-review STOP"},
}

func TestTransitions(t *testing.T) {
	eng := loadSnapshot(t)
	for _, tc := range transitionTable {
		t.Run(tc.flow+"/"+tc.from+"->"+tc.wantTo, func(t *testing.T) {
			flow, ok := eng.Flows[tc.flow]
			if !ok {
				t.Fatalf("flow %q not in loaded engine", tc.flow)
			}
			ctx := NewContext()
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			if tc.params != nil {
				ctx.Params = tc.params
			}
			got, err := eng.nextEdge(flow, tc.from, ctx)
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
		key := tc.flow + ":" + tc.from + "->" + tc.wantTo
		covered[key] = true
	}
	for name, flow := range eng.Flows {
		for _, edge := range flow.Edges {
			key := name + ":" + edge.From + "->" + edge.To
			if !covered[key] {
				t.Errorf("uncovered edge in flow %q: %s -> %s (when=%q)", name, edge.From, edge.To, edge.Predicate)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Process-audit gap decisions — explicit anchors so they cannot drift back
// ---------------------------------------------------------------------------

func TestGapDecision_LegacyAcceptanceCriteriaSingleStop(t *testing.T) {
	eng := loadSnapshot(t)
	flow, ok := eng.Flows["legacy_acceptance_criteria"]
	if !ok {
		t.Fatalf("legacy_acceptance_criteria flow missing")
	}
	if flow.Start != "LEGACY_TBD" {
		t.Errorf("legacy_acceptance_criteria start: got %q, want LEGACY_TBD", flow.Start)
	}
	if got := len(flow.Nodes); got != 2 {
		t.Errorf("legacy_acceptance_criteria node count: got %d, want 2 (STOP + END)", got)
	}
	stop, ok := flow.Nodes["LEGACY_TBD"]
	if !ok {
		t.Fatalf("LEGACY_TBD node missing")
	}
	if stop.Kind != UserTask || stop.Raw.Agent != "human" || stop.Raw.Role != "review" {
		t.Errorf("LEGACY_TBD: got kind=%v agent=%q role=%q, want UserTask/human/review", stop.Kind, stop.Raw.Agent, stop.Raw.Role)
	}
}

func TestGapDecision_CTExitReturnsToSystemDriverGate(t *testing.T) {
	eng := loadSnapshot(t)
	flow := eng.Flows["at_cycle"]
	for _, edge := range flow.OutgoingByNode["CT_SUBPROCESS"] {
		if edge.To != "GATE_SYS_AT" {
			t.Errorf("CT_SUBPROCESS exit edge: got to=%q, want GATE_SYS_AT", edge.To)
		}
	}
}

func TestGapDecision_SmokeTestFailStopsAtAskSupport(t *testing.T) {
	eng := loadSnapshot(t)
	flow := eng.Flows["external_system_onboarding"]
	ctx := NewContext()
	ctx.Set("smoke_test_passes", false)
	got, err := eng.nextEdge(flow, "GATE_SMOKE_PASS", ctx)
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
	// external-system-interface-redesign, system-implementation-change}.
	eng := loadSnapshot(t)
	flow := eng.Flows["run_cycle"]
	if flow == nil {
		t.Fatalf("run_cycle flow missing")
	}
	top, ok := flow.Nodes["GATE_CHANGE_TYPE"]
	if !ok {
		t.Fatalf("GATE_CHANGE_TYPE node missing from run_cycle")
	}
	if top.Kind != Gateway || top.Raw.Binding != "change_type" {
		t.Errorf("GATE_CHANGE_TYPE: kind=%v binding=%q, want Gateway/change_type", top.Kind, top.Raw.Binding)
	}
	for id, node := range flow.Nodes {
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
	// `agent: atdd-stubs` as a placeholder. Lock that here so a future edit
	// that resolves the gap will fail this test, prompting an explicit
	// update + decision record.
	eng := loadSnapshot(t)
	stubs := eng.Flows["ct_subprocess"].Nodes["CT_GREEN_STUBS"]
	if stubs.Raw.Agent != "atdd-stubs" {
		t.Errorf("CT_GREEN_STUBS agent: got %q, want %q (placeholder pending stubs-ownership decision)", stubs.Raw.Agent, "atdd-stubs")
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

func TestEngine_RunFlow_LegacyAcceptanceCriteria_StopsCleanly(t *testing.T) {
	eng := loadSnapshot(t)
	// Bind every user_task to a no-op so legacy_acceptance_criteria runs without
	// blocking on stdin; gateways/actions aren't reached in this flow.
	noop := func(ctx *Context) Outcome { return Outcome{} }
	eng.AgentFn = func(name string) NodeFn { return noop }
	eng.ActionFn = func(name string) NodeFn { return noop }
	eng.GateFn = func(name string) NodeFn { return noop }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := eng.RunFlow("legacy_acceptance_criteria", NewContext()); err != nil {
		t.Errorf("RunFlow legacy_acceptance_criteria: %v", err)
	}
}
