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
		"intake",
		"run_legacy_cycle",
		"run_cycle",
		"at_cycle",
		"at_green_system",
		"ct_subprocess",
		"external_system_onboarding",
		"structural_cycle",
		"external_api_task_cycle",
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
	// Extracted from main so the top-level diagram shows the ticket lifecycle
	// only and the type-specific intake fan-out (classify, conflict gate,
	// type gate, four agents, scenario approval) lives in its own diagram.
	{flow: "intake", from: "CLASSIFY", wantTo: "GATE_CLASSIFY_CONFIDENT"},
	{flow: "intake", from: "GATE_CLASSIFY_CONFIDENT", state: map[string]any{"classify_confident": true}, wantTo: "GATE_TICKET_TYPE"},
	{flow: "intake", from: "GATE_CLASSIFY_CONFIDENT", state: map[string]any{"classify_confident": false}, wantTo: "STOP_CLASSIFY_CONFLICT"},
	{flow: "intake", from: "STOP_CLASSIFY_CONFLICT", wantTo: "GATE_TICKET_TYPE"},
	{flow: "intake", from: "GATE_TICKET_TYPE", state: map[string]any{"ticket_type": "story"}, wantTo: "ATDD_STORY"},
	{flow: "intake", from: "GATE_TICKET_TYPE", state: map[string]any{"ticket_type": "bug"}, wantTo: "ATDD_BUG"},
	{flow: "intake", from: "GATE_TICKET_TYPE", state: map[string]any{"ticket_type": "system-api-task"}, wantTo: "ATDD_TASK"},
	{flow: "intake", from: "GATE_TICKET_TYPE", state: map[string]any{"ticket_type": "system-ui-task"}, wantTo: "ATDD_TASK"},
	{flow: "intake", from: "GATE_TICKET_TYPE", state: map[string]any{"ticket_type": "external-api-task"}, wantTo: "ATDD_TASK"},
	{flow: "intake", from: "GATE_TICKET_TYPE", state: map[string]any{"ticket_type": "chore"}, wantTo: "ATDD_CHORE"},
	{flow: "intake", from: "ATDD_STORY", wantTo: "STOP_INTAKE"},
	{flow: "intake", from: "ATDD_BUG", wantTo: "STOP_INTAKE"},
	{flow: "intake", from: "ATDD_TASK", wantTo: "STOP_INTAKE"},
	{flow: "intake", from: "ATDD_CHORE", wantTo: "STOP_INTAKE"},
	{flow: "intake", from: "STOP_INTAKE", wantTo: "INTAKE_END"},

	// ---- run_legacy_cycle ----
	// Backfill cycle for legacy acceptance criteria. Self-contained: gates internally on
	// presence and no-ops when absent so main can call it unconditionally.
	{flow: "run_legacy_cycle", from: "GATE_LEGACY_PRESENT", state: map[string]any{"legacy_acceptance_criteria_section_present": true}, wantTo: "LEGACY_CYCLE"},
	{flow: "run_legacy_cycle", from: "GATE_LEGACY_PRESENT", state: map[string]any{"legacy_acceptance_criteria_section_present": false}, wantTo: "RUN_LEGACY_END"},
	{flow: "run_legacy_cycle", from: "LEGACY_CYCLE", wantTo: "RUN_LEGACY_END"},

	// ---- run_cycle ----
	// Change cycle dispatch — type gate + five behavioral/structural cycles.
	// Legacy backfill lives in run_legacy_cycle so this flow is just dispatch.
	{flow: "run_cycle", from: "GATE_TYPE_CYCLE", state: map[string]any{"ticket_type": "story"}, wantTo: "AT_CYCLE"},
	{flow: "run_cycle", from: "GATE_TYPE_CYCLE", state: map[string]any{"ticket_type": "bug"}, wantTo: "AT_CYCLE"},
	{flow: "run_cycle", from: "GATE_TYPE_CYCLE", state: map[string]any{"ticket_type": "system-api-task"}, wantTo: "SYSAPI_CYCLE"},
	{flow: "run_cycle", from: "GATE_TYPE_CYCLE", state: map[string]any{"ticket_type": "system-ui-task"}, wantTo: "SYSUI_CYCLE"},
	{flow: "run_cycle", from: "GATE_TYPE_CYCLE", state: map[string]any{"ticket_type": "external-api-task"}, wantTo: "EXTAPI_CYCLE"},
	{flow: "run_cycle", from: "GATE_TYPE_CYCLE", state: map[string]any{"ticket_type": "chore"}, wantTo: "CHORE_CYCLE"},
	{flow: "run_cycle", from: "AT_CYCLE", wantTo: "CYCLE_END"},
	{flow: "run_cycle", from: "SYSAPI_CYCLE", wantTo: "CYCLE_END"},
	{flow: "run_cycle", from: "SYSUI_CYCLE", wantTo: "CYCLE_END"},
	{flow: "run_cycle", from: "EXTAPI_CYCLE", wantTo: "CYCLE_END"},
	{flow: "run_cycle", from: "CHORE_CYCLE", wantTo: "CYCLE_END"},

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
	{flow: "at_cycle", from: "AT_RED_SYSTEM_DRIVER", wantTo: "AT_GREEN_SYSTEM"},
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
	{flow: "ct_subprocess", from: "CT_RED_EXTERNAL_DRIVER", wantTo: "CT_GREEN_STUBS"},
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
	{flow: "structural_cycle", from: "STRUCT_WRITE", wantTo: "STOP_STRUCT_REVIEW"},
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

	// ---- external_api_task_cycle ----
	{flow: "external_api_task_cycle", from: "EXTAPI_CT", wantTo: "EXTAPI_END"},

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

func TestGapDecision_TypeCycleRoutesByTicketType(t *testing.T) {
	// GATE_TYPE_CYCLE replaced the dead GATE_CHANGE_DRIVEN gate, which
	// computed a `change_driven_ac_produced` bool that no outgoing edge ever
	// read. Lock the new shape: the gateway binds to `ticket_type` (already
	// populated by CLASSIFY) and fans out to one cycle per type — no prompt
	// round-trip in the no-Legacy path.
	//
	// The gate moved from `main` to `run_cycle` when the cycle dispatch
	// was extracted into its own flow; assert it lives there now and is
	// absent from main.
	eng := loadSnapshot(t)
	flow := eng.Flows["run_cycle"]
	if flow == nil {
		t.Fatalf("run_cycle flow missing")
	}
	gate, ok := flow.Nodes["GATE_TYPE_CYCLE"]
	if !ok {
		t.Fatalf("GATE_TYPE_CYCLE node missing from run_cycle")
	}
	if gate.Kind != Gateway {
		t.Errorf("GATE_TYPE_CYCLE: kind got %v, want Gateway", gate.Kind)
	}
	if gate.Raw.Binding != "ticket_type" {
		t.Errorf("GATE_TYPE_CYCLE: binding got %q, want %q", gate.Raw.Binding, "ticket_type")
	}
	if _, dead := flow.Nodes["GATE_CHANGE_DRIVEN"]; dead {
		t.Errorf("GATE_CHANGE_DRIVEN must not exist after deletion")
	}
	if _, leftover := eng.Flows["main"].Nodes["GATE_TYPE_CYCLE"]; leftover {
		t.Errorf("GATE_TYPE_CYCLE should have moved out of main into run_cycle")
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
	ctx.Set("ticket_type", "system-ui-task")
	expr := "ticket_type in [system-api-task, system-ui-task, external-api-task]"
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
	ctx.Set("ticket_type", "story")
	expr := "ticket_type in [system-api-task, system-ui-task, external-api-task]"
	ok, _ := evalPredicate(expr, ctx)
	if ok {
		t.Errorf("expected story to NOT be in task list")
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
