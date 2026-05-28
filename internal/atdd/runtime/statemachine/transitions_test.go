// Transitions test suite for the ATDD process-flow YAML.
//
// Scope: structural invariants over the loaded YAML plus predicate-
// evaluator unit tests. The detailed transition-table coverage that used
// to live here was authored for the pre-refactor (AT-cycle / CT-subprocess /
// structural-cycle / legacy-acceptance-criteria) shape; the BPMN five-level
// refactor (plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md Item 3)
// replaced that wholesale and the per-edge rows no longer apply. Phase D's
// downstream-alignment plan re-establishes execution-flow coverage when
// the runtime registries (ActionFn / AgentFn / GateFn) for the new
// vocabulary land.
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
// Snapshot inventory — every named process the embedded YAML defines
// ---------------------------------------------------------------------------

func TestLoadSnapshot_AllProcessesParse(t *testing.T) {
	eng := loadSnapshot(t)
	wantProcesses := []string{
		// runtime bootstrap
		"main",
		// TOP
		"refine-ticket",
		"implement-ticket",
		"refactor",
		// CYCLE
		"refine-backlog-item",
		"change-system-behavior",
		"cover-system-behavior",
		"redesign-system-structure",
		"refactor-system-structure",
		"refactor-test-structure",
		// HIGH
		"write-and-verify-acceptance-tests-fail",
		"write-and-verify-acceptance-tests-pass",
		"write-and-verify-acceptance-tests",
		"write-and-verify-acceptance-test-code",
		"implement-and-verify-dsl",
		"implement-and-verify-system-driver-adapters",
		"implement-and-verify-external-system-driver-adapters",
		"implement-and-verify-external-system-driver-adapters-contract-tests",
		"implement-and-verify-system",
		"refactor-and-verify-tests",
		"implement-test-layer",
		"verify-tests-pass",
		"verify-tests-fail",
		// MID — agent tasks
		"write-acceptance-tests",
		"write-contract-tests",
		"implement-dsl",
		"implement-system",
		"implement-system-driver-adapters",
		"implement-external-system-driver-adapters",
		"implement-external-system-stubs",
		"disable-tests",
		"enable-tests",
		"fix-unexpected-passing-tests",
		"fix-unexpected-failing-tests",
		"refactor-tests",
		"refactor-system",
		"refine-acceptance-criteria",
		// MID — command tasks
		"compile",
		"compile-system",
		"compile-tests",
		"build-system",
		"start-system",
		"start-system-restart",
		"commit",
		"run-tests",
		// LOW
		"approve",
		"execute-agent",
		"execute-command",
		"fix",
	}
	for _, name := range wantProcesses {
		if _, ok := eng.Processes[name]; !ok {
			t.Errorf("process %q missing from loaded snapshot", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Structural invariants
// ---------------------------------------------------------------------------

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
	// A node may legitimately have no outgoing edges in three cases:
	//   - it is an end-event or error-end-event (BPMN terminal events), or
	//   - it is a `agent: human` STOP node intended to halt the run
	//     awaiting user intervention.
	// Anything else is a missing-edge bug.
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind == EndEvent || node.Kind == ErrorEndEvent {
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
// Per-process shape regressions
// ---------------------------------------------------------------------------

// execute-command's command-failure branch must route through
// GATE_FIX_ON_FAILURE, not straight to FIX. Without this gate, run-tests
// callers (verify-tests-pass, verify-tests-fail) cannot opt out of the
// inner FIX dispatch, and expected acceptance-test failures mis-route to
// FIX instead of VERIFY_FAIL_END.
func TestExecuteCommand_FailureRoutesThroughFixOnFailureGate(t *testing.T) {
	eng := loadSnapshot(t)
	proc, ok := eng.Processes["execute-command"]
	if !ok {
		t.Fatalf("process execute-command missing")
	}

	// 1. GATE_FIX_ON_FAILURE node exists and is a gateway on the right binding.
	gate, ok := proc.Nodes["GATE_FIX_ON_FAILURE"]
	if !ok {
		t.Fatalf("execute-command: GATE_FIX_ON_FAILURE node missing")
	}
	if gate.Kind != Gateway {
		t.Errorf("execute-command: GATE_FIX_ON_FAILURE kind = %v, want Gateway", gate.Kind)
	}
	if gate.Raw.Binding != "fix-on-failure-enabled" {
		t.Errorf("execute-command: GATE_FIX_ON_FAILURE binding = %q, want %q", gate.Raw.Binding, "fix-on-failure-enabled")
	}

	// 2. command-succeeded == false routes to GATE_FIX_ON_FAILURE, not FIX.
	wantEdge(t, proc, "GATE_COMMAND_SUCCEEDED", "GATE_FIX_ON_FAILURE", "command-succeeded == false")
	notEdge(t, proc, "GATE_COMMAND_SUCCEEDED", "FIX")

	// 3. GATE_FIX_ON_FAILURE branches: true → FIX, false → END.
	wantEdge(t, proc, "GATE_FIX_ON_FAILURE", "FIX", "fix-on-failure-enabled == true")
	wantEdge(t, proc, "GATE_FIX_ON_FAILURE", "EXECUTE_COMMAND_END", "fix-on-failure-enabled == false")
}

// Both verify-tests-pass and verify-tests-fail must route
// test-outcome=="infra" to TESTS_INFRA_HALT (an error-end-event), not
// to the same node that pass/fail routes to. An infra failure means
// the runner could not start — neither fixer (failing-tests, passing-
// tests) is appropriate and the pre-classifier behaviour of treating
// it as test-red silently advanced verify-tests-fail past a runner
// that never produced a report. The error-end-event ensures the
// failure bubbles up to driver.Run as a non-zero exit.
func TestVerifyTests_InfraOutcomeRoutesToHalt(t *testing.T) {
	eng := loadSnapshot(t)
	for _, proc := range []string{"verify-tests-pass", "verify-tests-fail"} {
		t.Run(proc, func(t *testing.T) {
			p, ok := eng.Processes[proc]
			if !ok {
				t.Fatalf("process %q missing", proc)
			}
			node, ok := p.Nodes["TESTS_INFRA_HALT"]
			if !ok {
				t.Fatalf("%s: TESTS_INFRA_HALT node missing", proc)
			}
			if node.Kind != ErrorEndEvent {
				t.Errorf("%s: TESTS_INFRA_HALT kind = %v, want ErrorEndEvent (must bubble up, not soft-end)", proc, node.Kind)
			}
			wantEdge(t, p, "GATE_TESTS_OUTCOME", "TESTS_INFRA_HALT", "test-outcome == infra")
		})
	}
}

// Every fix dispatch must loop back to the upstream step that
// produced the failure, so the operator (and the engine) get a
// re-verification cycle after every fix. Four call-sites, four
// loopback edges:
//
//	execute-command   : FIX → RUN_COMMAND
//	execute-agent     : FIX → RUN_AGENT
//	verify-tests-pass : FIX_UNEXPECTED_FAILING_TESTS → RUN_TESTS
//	verify-tests-fail : FIX_UNEXPECTED_PASSING_TESTS → RUN_TESTS
//
// A regression that re-points any of these edges back to an end-event
// would silently strip the re-verification — the fix would dispatch
// and the process would exit without confirming the fix worked.
func TestFixDispatch_LoopsBackToOriginatingStep(t *testing.T) {
	eng := loadSnapshot(t)
	cases := []struct {
		proc     string
		from, to string
	}{
		{"execute-command", "FIX", "RUN_COMMAND"},
		{"execute-agent", "FIX", "RUN_AGENT"},
		{"verify-tests-pass", "FIX_UNEXPECTED_FAILING_TESTS", "RUN_TESTS"},
		{"verify-tests-fail", "FIX_UNEXPECTED_PASSING_TESTS", "RUN_TESTS"},
	}
	for _, tc := range cases {
		t.Run(tc.proc, func(t *testing.T) {
			p, ok := eng.Processes[tc.proc]
			if !ok {
				t.Fatalf("process %q missing", tc.proc)
			}
			wantEdge(t, p, tc.from, tc.to, "")
		})
	}
}

// run-tests must opt out of execute-command's FIX branch so the
// verify-tests-pass / verify-tests-fail callers can route on
// test-outcome instead.
func TestRunTests_DisablesFixOnFailure(t *testing.T) {
	eng := loadSnapshot(t)
	proc, ok := eng.Processes["run-tests"]
	if !ok {
		t.Fatalf("process run-tests missing")
	}
	node, ok := proc.Nodes["EXECUTE_COMMAND"]
	if !ok {
		t.Fatalf("run-tests: EXECUTE_COMMAND node missing")
	}
	got := node.Raw.Params["fix-on-failure"]
	if got != "false" {
		t.Errorf("run-tests EXECUTE_COMMAND params[fix-on-failure] = %q, want %q", got, "false")
	}
}

func wantEdge(t *testing.T, proc *Process, from, to, predicate string) {
	t.Helper()
	for _, e := range proc.OutgoingByNode[from] {
		if e.To == to && e.Predicate == predicate {
			return
		}
	}
	t.Errorf("process %q: missing edge %s -> %s when %q", proc.ID, from, to, predicate)
}

func notEdge(t *testing.T, proc *Process, from, to string) {
	t.Helper()
	for _, e := range proc.OutgoingByNode[from] {
		if e.To == to {
			t.Errorf("process %q: unexpected edge %s -> %s (predicate %q)", proc.ID, from, to, e.Predicate)
		}
	}
}

// ---------------------------------------------------------------------------
// Predicate evaluator unit tests
// ---------------------------------------------------------------------------

func TestPredicate_EmptyAlwaysTrue(t *testing.T) {
	ctx := NewContext()
	got, err := evalPredicate("", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("empty predicate: got false, want true")
	}
}

func TestPredicate_Equality(t *testing.T) {
	cases := []struct {
		state    map[string]any
		expr     string
		want     bool
		wantErr  bool
		caseName string
	}{
		{map[string]any{"ticket_type": "story"}, "ticket_type == story", true, false, "bare value matches"},
		{map[string]any{"ticket_type": "story"}, `ticket_type == "story"`, true, false, "quoted value matches"},
		{map[string]any{"ticket_type": "bug"}, "ticket_type == story", false, false, "mismatch returns false"},
		{map[string]any{}, "ticket_type == story", false, false, "missing key treated as empty string"},
	}
	for _, tc := range cases {
		t.Run(tc.caseName, func(t *testing.T) {
			ctx := NewContext()
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			got, err := evalPredicate(tc.expr, ctx)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("evalPredicate(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestPredicate_BoolEquality(t *testing.T) {
	ctx := NewContext()
	ctx.Set("approval-outcome", true)
	got, err := evalPredicate("approval-outcome == true", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("bool equality: got false, want true")
	}
}

func TestPredicate_InList(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "bug")
	got, err := evalPredicate("ticket_type in [story, bug]", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("`in` membership: got false, want true")
	}
}

func TestPredicate_InListNegative(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "spike")
	got, err := evalPredicate("ticket_type in [story, bug]", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if got {
		t.Errorf("`in` non-membership: got true, want false")
	}
}
