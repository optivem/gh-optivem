// Transitions / structural test suite for the concrete ATDD process-flow.
//
// Scope: structural invariants over the loaded ATDD process document plus the
// per-process routing regressions. These load the concrete process via
// process.Load(), so they live in the process package and reference the
// generic engine types through the statemachine. qualifier.
package process_test

import (
	"slices"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

func loadSnapshot(t *testing.T) *statemachine.Engine {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load embedded process-flow: %v", err)
	}
	return eng
}

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
		"redesign-external-system-structure",
		"refactor-system-structure",
		"refactor-test-structure",
		// HIGH
		"implement-external-drivers-if-needed",
		"write-acceptance-tests-and-system-adapters",
		"write-acceptance-tests-and-dsl",
		"write-and-verify-acceptance-test-code",
		"implement-and-verify-dsl",
		"implement-and-verify-system-driver-adapters",
		"implement-and-verify-external-system-driver-adapters-contract-tests",
		"reconcile-external-contract-producer",
		"redesign-external-system-per-system",
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
		"fix-unexpected-passing-tests",
		"fix-unexpected-failing-tests",
		"refactor-tests",
		"refactor-system",
		"refine-acceptance-criteria",
		// MID — command tasks
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
			if node.Kind != statemachine.Gateway {
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
			if node.Kind == statemachine.EndEvent || node.Kind == statemachine.ErrorEndEvent {
				continue
			}
			if len(process.OutgoingByNode[id]) > 0 {
				continue
			}
			if node.Kind == statemachine.UserTask && node.Raw.Agent == "human" {
				continue // intentional STOP-and-halt
			}
			t.Errorf("process %q non-end node %q has no outgoing edges", name, id)
		}
	}
}

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
	if gate.Kind != statemachine.Gateway {
		t.Errorf("execute-command: GATE_FIX_ON_FAILURE kind = %v, want statemachine.Gateway", gate.Kind)
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

// execute-agent must route the agent's explicit scope-exception envelope
// to STOP_SCOPE_VIOLATION (a hard halt), not fall through to the FIX loop.
// GATE_SCOPE_EXCEPTION_REQUESTED sits ahead of GATE_OUTPUTS_AND_SCOPES_VALID
// so the explicit refusal takes precedence over an unrelated validation
// failure. Without this gate the scope-exception signal lands in ctx and
// nothing routes on it — the cycle re-dispatches RUN_AGENT against the same
// too-narrow scope: and loops.
func TestExecuteAgent_ScopeExceptionRoutesToStopViolation(t *testing.T) {
	eng := loadSnapshot(t)
	proc, ok := eng.Processes["execute-agent"]
	if !ok {
		t.Fatalf("process execute-agent missing")
	}

	// 1. GATE_SCOPE_EXCEPTION_REQUESTED node exists and binds the right gate.
	gate, ok := proc.Nodes["GATE_SCOPE_EXCEPTION_REQUESTED"]
	if !ok {
		t.Fatalf("execute-agent: GATE_SCOPE_EXCEPTION_REQUESTED node missing")
	}
	if gate.Kind != statemachine.Gateway {
		t.Errorf("execute-agent: GATE_SCOPE_EXCEPTION_REQUESTED kind = %v, want statemachine.Gateway", gate.Kind)
	}
	if gate.Raw.Binding != "scope-exception-requested" {
		t.Errorf("execute-agent: GATE_SCOPE_EXCEPTION_REQUESTED binding = %q, want %q", gate.Raw.Binding, "scope-exception-requested")
	}

	// 2. STOP_SCOPE_VIOLATION's error-end-event kind (deliberate halt, must
	//    bubble up — not a soft end-event) is now asserted by the quantified
	//    halt-terminals-are-error-end rule (`STOP_` marker) in
	//    invariants_test.go. This test keeps the process-specific routing.

	// 3. Validation feeds the exception gate first, and the old direct edge
	//    VALIDATE_OUTPUTS_AND_SCOPES -> GATE_OUTPUTS_AND_SCOPES_VALID is gone.
	wantEdge(t, proc, "VALIDATE_OUTPUTS_AND_SCOPES", "GATE_SCOPE_EXCEPTION_REQUESTED", "")
	notEdge(t, proc, "VALIDATE_OUTPUTS_AND_SCOPES", "GATE_OUTPUTS_AND_SCOPES_VALID")

	// 4. Exception requested -> Guard B categorizer (which then forks the halt);
	//    not requested -> normal validation. The old direct
	//    GATE_SCOPE_EXCEPTION_REQUESTED -> STOP_SCOPE_VIOLATION edge is gone:
	//    STOP_SCOPE_VIOLATION is now reached via GATE_SCOPE_EXCEPTION_NEEDS_ESCC
	//    (see TestExecuteAgent_GuardB_ESCCReroute).
	wantEdge(t, proc, "GATE_SCOPE_EXCEPTION_REQUESTED", "CATEGORIZE_SCOPE_EXCEPTION", "scope-exception-requested == true")
	notEdge(t, proc, "GATE_SCOPE_EXCEPTION_REQUESTED", "STOP_SCOPE_VIOLATION")
	wantEdge(t, proc, "GATE_SCOPE_EXCEPTION_REQUESTED", "GATE_OUTPUTS_AND_SCOPES_VALID", "scope-exception-requested == false")
}

// Guard B (plan 20260620-2348): a scope-exception is categorized before it
// halts. CATEGORIZE_SCOPE_EXCEPTION stamps scope-exception-needs-escc, and
// GATE_SCOPE_EXCEPTION_NEEDS_ESCC forks the loud ESCC_UNDECLARED_HALT (the
// ticket lacks its ## External System Contract Criteria section) from the
// generic STOP_SCOPE_VIOLATION (the BPMN scope: is too narrow). This pins the
// three reroute cases at the graph level; the categorizer's bool decision for
// each case is unit-tested in actions/TestCategorizeScopeException.
func TestExecuteAgent_GuardB_ESCCReroute(t *testing.T) {
	eng := loadSnapshot(t)
	proc, ok := eng.Processes["execute-agent"]
	if !ok {
		t.Fatalf("process execute-agent missing")
	}

	// 1. The categorizer is a service-task bound to the Guard B action.
	cat, ok := proc.Nodes["CATEGORIZE_SCOPE_EXCEPTION"]
	if !ok {
		t.Fatalf("execute-agent: CATEGORIZE_SCOPE_EXCEPTION node missing")
	}
	if cat.Kind != statemachine.ServiceTask {
		t.Errorf("CATEGORIZE_SCOPE_EXCEPTION kind = %v, want statemachine.ServiceTask", cat.Kind)
	}
	if cat.Raw.Action != "categorize-scope-exception" {
		t.Errorf("CATEGORIZE_SCOPE_EXCEPTION action = %q, want %q", cat.Raw.Action, "categorize-scope-exception")
	}

	// 2. The reroute gate binds the categorizer's stamped bool.
	gate, ok := proc.Nodes["GATE_SCOPE_EXCEPTION_NEEDS_ESCC"]
	if !ok {
		t.Fatalf("execute-agent: GATE_SCOPE_EXCEPTION_NEEDS_ESCC node missing")
	}
	if gate.Raw.Binding != "scope-exception-needs-escc" {
		t.Errorf("GATE_SCOPE_EXCEPTION_NEEDS_ESCC binding = %q, want %q", gate.Raw.Binding, "scope-exception-needs-escc")
	}

	// 3. ESCC_UNDECLARED_HALT is an error-end-event so the halt bubbles up (its
	//    id ends `_HALT` but not `_INFRA_HALT`, so the quantified
	//    halt-terminals-are-error-end rule does not cover it — pinned here,
	//    mirroring CONTRACT_REAL_UPSTREAM_GAP_HALT).
	if n := proc.Nodes["ESCC_UNDECLARED_HALT"]; n.Kind != statemachine.ErrorEndEvent {
		t.Errorf("ESCC_UNDECLARED_HALT kind = %v, want statemachine.ErrorEndEvent", n.Kind)
	}

	// 4. The categorizer feeds the reroute gate; the gate forks the two halts.
	wantEdge(t, proc, "CATEGORIZE_SCOPE_EXCEPTION", "GATE_SCOPE_EXCEPTION_NEEDS_ESCC", "")
	wantEdge(t, proc, "GATE_SCOPE_EXCEPTION_NEEDS_ESCC", "ESCC_UNDECLARED_HALT", "scope-exception-needs-escc == true")
	wantEdge(t, proc, "GATE_SCOPE_EXCEPTION_NEEDS_ESCC", "STOP_SCOPE_VIOLATION", "scope-exception-needs-escc == false")
}

// Both verify-tests-pass and verify-tests-fail must route
// test-outcome=="infra" to TESTS_INFRA_HALT, not to the same node that
// pass/fail routes to. An infra failure means the runner could not start
// — neither fixer (failing-tests, passing-tests) is appropriate and the
// pre-classifier behaviour of treating it as test-red silently advanced
// verify-tests-fail past a runner that never produced a report.
//
// That TESTS_INFRA_HALT is an error-end-event (so the failure bubbles up
// to driver.Run as a non-zero exit) is now asserted by the quantified
// halt-terminals-are-error-end rule (`_INFRA_HALT` marker) in
// invariants_test.go; this test keeps the process-specific routing edge,
// which the graph rule does not cover.
func TestVerifyTests_InfraOutcomeRoutesToHalt(t *testing.T) {
	eng := loadSnapshot(t)
	for _, proc := range []string{"verify-tests-pass", "verify-tests-fail"} {
		t.Run(proc, func(t *testing.T) {
			p, ok := eng.Processes[proc]
			if !ok {
				t.Fatalf("process %q missing", proc)
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
//	verify-tests-pass : FIX_UNEXPECTED_FAILING_TESTS → GATE_FIX_FLOW_APPROVED → RUN_TESTS
//	verify-tests-fail : FIX_UNEXPECTED_PASSING_TESTS → GATE_FIX_FLOW_APPROVED → RUN_TESTS
//
// The two verify loops route the fixer back through GATE_FIX_FLOW_APPROVED:
// the predicateless default edge continues to RUN_TESTS (re-verify), while
// a rejected dispatch diverts to FIX_FLOW_NOT_APPROVED instead of counting
// as a fix attempt. We pin both hops so the re-verification cycle stays
// intact and the reject diversion can't silently swallow the back-edge.
//
// A regression that re-points any of these edges back to an end-event
// would silently strip the re-verification — the fix would dispatch
// and the process would exit without confirming the fix worked.
//
// The quantified fix-loops-back rule in invariants_test.go now asserts
// that every fix dispatch loops back *somehow* (reachability), over all
// sites; this test is kept as the focused regression that pins the exact
// origin edge per site, which the reachability-based rule does not.
func TestFixDispatch_LoopsBackToOriginatingStep(t *testing.T) {
	eng := loadSnapshot(t)
	cases := []struct {
		proc     string
		from, to string
	}{
		{"execute-command", "FIX", "RUN_COMMAND"},
		{"execute-agent", "FIX", "RUN_AGENT"},
		{"verify-tests-pass", "FIX_UNEXPECTED_FAILING_TESTS", "GATE_FIX_FLOW_APPROVED"},
		{"verify-tests-pass", "GATE_FIX_FLOW_APPROVED", "RUN_TESTS"},
		{"verify-tests-fail", "FIX_UNEXPECTED_PASSING_TESTS", "GATE_FIX_FLOW_APPROVED"},
		{"verify-tests-fail", "GATE_FIX_FLOW_APPROVED", "RUN_TESTS"},
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

// Both fix-dispatch loops must be bounded (plan 20260530-1604). The FIX
// node in execute-command and execute-agent gets a max-visits cap routing
// to a process-specific error-end-event, and the shared `fix` process's
// PRE-reject terminal becomes an error-end-event so rejecting a fix halts
// the run instead of looping back to re-run the failed step. Without the
// cap, the FIX -> RUN_COMMAND / FIX -> RUN_AGENT back-edge spins until the
// 10000-dispatch backstop (rehearsal-71's endless re-prompt); without the
// reject=halt, `n` (or EOF-auto-reject under --auto) loops forever too.
func TestFixDispatch_LoopsAreBounded(t *testing.T) {
	eng := loadSnapshot(t)

	// 1. The shared `fix` PRE-reject terminal is now an error-end-event.
	t.Run("fix reject is a hard halt", func(t *testing.T) {
		proc, ok := eng.Processes["fix"]
		if !ok {
			t.Fatalf("process fix missing")
		}
		node, ok := proc.Nodes["FIX_REJECTED_END"]
		if !ok {
			t.Fatalf("fix: FIX_REJECTED_END node missing")
		}
		if node.Kind != statemachine.ErrorEndEvent {
			t.Errorf("fix: FIX_REJECTED_END kind = %v, want statemachine.ErrorEndEvent (reject must halt, not soft-skip)", node.Kind)
		}
		// The reject edge still targets it.
		wantEdge(t, proc, "GATE_APPROVED_PRE", "FIX_REJECTED_END", "approval-outcome == rejected")
	})

	// 2. Both loops cap the FIX node and route to a distinct exhausted terminal.
	caps := []struct {
		proc, halt string
	}{
		{"execute-command", "COMMAND_FIX_EXHAUSTED"},
		{"execute-agent", "AGENT_FIX_EXHAUSTED"},
	}
	for _, c := range caps {
		t.Run(c.proc+" FIX is capped", func(t *testing.T) {
			proc, ok := eng.Processes[c.proc]
			if !ok {
				t.Fatalf("process %q missing", c.proc)
			}
			fix, ok := proc.Nodes["FIX"]
			if !ok {
				t.Fatalf("%s: FIX node missing", c.proc)
			}
			if fix.Raw.MaxVisits != 2 {
				t.Errorf("%s: FIX max-visits = %d, want 2", c.proc, fix.Raw.MaxVisits)
			}
			if fix.Raw.OnMaxVisits != c.halt {
				t.Errorf("%s: FIX on-max-visits = %q, want %q", c.proc, fix.Raw.OnMaxVisits, c.halt)
			}
			// The exhausted terminal's error-end-event kind is asserted by
			// the quantified halt-terminals-are-error-end rule (`_EXHAUSTED`
			// marker) in invariants_test.go; here we keep the process-
			// specific cap wiring (max-visits, on-max-visits target).
			if _, ok := proc.Nodes[c.halt]; !ok {
				t.Fatalf("%s: %s node missing", c.proc, c.halt)
			}
			// The back-edge to the originating step is preserved (the cap
			// intercepts on the 3rd arrival; the loop itself stays intact).
			origin := map[string]string{"execute-command": "RUN_COMMAND", "execute-agent": "RUN_AGENT"}[c.proc]
			wantEdge(t, proc, "FIX", origin, "")
		})
	}
}

// verify-tests-pass's no-progress guard (plan 20260615-1845 Step 4) sits on
// the fail branch BEFORE the fixer: GATE_TESTS_OUTCOME's `fail` edge now
// routes through CHECK_FIX_PROGRESS → GATE_FIX_PROGRESSING, which either
// re-dispatches the fixer (progressing == true) or halts at a distinct
// error-end terminal (progressing == false) when two consecutive runs fail
// identically. The fixer's own loop-back (FIX → GATE_FIX_FLOW_APPROVED →
// RUN_TESTS) and the count cap stay intact — this guard layers under them, it
// does not replace them.
func TestVerifyTestsPass_NoProgressGuardWiring(t *testing.T) {
	eng := loadSnapshot(t)
	p, ok := eng.Processes["verify-tests-pass"]
	if !ok {
		t.Fatalf("process verify-tests-pass missing")
	}

	// 1. The fail branch now enters the progress check, not the fixer directly.
	wantEdge(t, p, "GATE_TESTS_OUTCOME", "CHECK_FIX_PROGRESS", "test-outcome == fail")
	notEdge(t, p, "GATE_TESTS_OUTCOME", "FIX_UNEXPECTED_FAILING_TESTS")

	// 2. The check feeds the progress gateway, which forks dispatch vs. halt.
	wantEdge(t, p, "CHECK_FIX_PROGRESS", "GATE_FIX_PROGRESSING", "")
	wantEdge(t, p, "GATE_FIX_PROGRESSING", "FIX_UNEXPECTED_FAILING_TESTS", "fix-loop-progressing == true")
	wantEdge(t, p, "GATE_FIX_PROGRESSING", "FIX_LOOP_NO_PROGRESS_EXHAUSTED", "fix-loop-progressing == false")

	// 3. CHECK_FIX_PROGRESS is a service-task bound to the check-fix-progress
	//    action; GATE_FIX_PROGRESSING binds the fix-loop-progressing gate.
	if got := p.Nodes["CHECK_FIX_PROGRESS"].Raw.Action; got != "check-fix-progress" {
		t.Errorf("CHECK_FIX_PROGRESS action = %q, want check-fix-progress", got)
	}
	if got := p.Nodes["GATE_FIX_PROGRESSING"].Raw.Binding; got != "fix-loop-progressing" {
		t.Errorf("GATE_FIX_PROGRESSING binding = %q, want fix-loop-progressing", got)
	}

	// 4. The no-progress terminal is an error-end-event so the halt bubbles
	//    up as a non-zero exit (the `_EXHAUSTED` marker also makes the
	//    halt-terminals-are-error-end invariant enforce this).
	if k := p.Nodes["FIX_LOOP_NO_PROGRESS_EXHAUSTED"].Kind; k != statemachine.ErrorEndEvent {
		t.Errorf("FIX_LOOP_NO_PROGRESS_EXHAUSTED kind = %v, want statemachine.ErrorEndEvent", k)
	}

	// 5. The fixer loop-back routes through GATE_FIX_FLOW_APPROVED: the
	//    predicateless default continues to RUN_TESTS (re-verify), while a
	//    rejected dispatch diverts to FIX_FLOW_NOT_APPROVED so it is not
	//    counted as a fix attempt. The count cap is untouched.
	wantEdge(t, p, "FIX_UNEXPECTED_FAILING_TESTS", "GATE_FIX_FLOW_APPROVED", "")
	wantEdge(t, p, "GATE_FIX_FLOW_APPROVED", "RUN_TESTS", "")
	wantEdge(t, p, "GATE_FIX_FLOW_APPROVED", "FIX_FLOW_NOT_APPROVED", "approval-outcome == rejected")
	if k := p.Nodes["FIX_FLOW_NOT_APPROVED"].Kind; k != statemachine.ErrorEndEvent {
		t.Errorf("FIX_FLOW_NOT_APPROVED kind = %v, want statemachine.ErrorEndEvent", k)
	}
	if fix := p.Nodes["FIX_UNEXPECTED_FAILING_TESTS"]; fix.Raw.MaxVisits != 2 || fix.Raw.OnMaxVisits != "FIX_LOOP_EXHAUSTED" {
		t.Errorf("FIX_UNEXPECTED_FAILING_TESTS cap = (%d, %q), want (2, FIX_LOOP_EXHAUSTED)", fix.Raw.MaxVisits, fix.Raw.OnMaxVisits)
	}
}

// TestVerifyLoops_RejectedFixDispatchNotCounted is the structural guarantee for
// plan 20260620-1624 Step 3 (Option 2): a rejected fixer dispatch must NOT
// count as a fix attempt and must NOT masquerade as FIX_LOOP_EXHAUSTED. Both
// verify loops route the fixer through GATE_FIX_FLOW_APPROVED, where a
// rejected dispatch diverts to the FIX_FLOW_NOT_APPROVED terminal instead of
// looping back to RUN_TESTS. We pin, for each loop:
//
//  1. the reject branch lands on FIX_FLOW_NOT_APPROVED (distinct from the
//     FIX_LOOP_EXHAUSTED count cap);
//  2. FIX_FLOW_NOT_APPROVED is a terminal error-end-event with NO outgoing
//     edge — so a reject leaves the loop and can never re-arrive at the fixer
//     node, which is what makes it "not a counted visit";
//  3. the gateway's predicateless default still continues to RUN_TESTS so the
//     approved-fix re-verification cycle is intact;
//  4. the reject terminal is NOT the count-cap terminal — they are different
//     nodes so an honest decline never reads as "2 fix attempts".
func TestVerifyLoops_RejectedFixDispatchNotCounted(t *testing.T) {
	eng := loadSnapshot(t)
	cases := []struct {
		proc, fixer string
	}{
		{"verify-tests-pass", "FIX_UNEXPECTED_FAILING_TESTS"},
		{"verify-tests-fail", "FIX_UNEXPECTED_PASSING_TESTS"},
	}
	for _, tc := range cases {
		t.Run(tc.proc, func(t *testing.T) {
			p, ok := eng.Processes[tc.proc]
			if !ok {
				t.Fatalf("process %q missing", tc.proc)
			}
			// Fixer routes through the approval gateway, never straight to RUN_TESTS.
			wantEdge(t, p, tc.fixer, "GATE_FIX_FLOW_APPROVED", "")
			notEdge(t, p, tc.fixer, "RUN_TESTS")
			// Reject diverts to the honest terminal; default continues the loop.
			wantEdge(t, p, "GATE_FIX_FLOW_APPROVED", "FIX_FLOW_NOT_APPROVED", "approval-outcome == rejected")
			wantEdge(t, p, "GATE_FIX_FLOW_APPROVED", "RUN_TESTS", "")
			// The reject terminal is a halt with no outgoing edge — a rejected
			// dispatch leaves the loop, so it never re-arrives at the fixer and
			// is never counted toward max-visits.
			term, ok := p.Nodes["FIX_FLOW_NOT_APPROVED"]
			if !ok {
				t.Fatalf("%s: FIX_FLOW_NOT_APPROVED node missing", tc.proc)
			}
			if term.Kind != statemachine.ErrorEndEvent {
				t.Errorf("%s: FIX_FLOW_NOT_APPROVED kind = %v, want statemachine.ErrorEndEvent", tc.proc, term.Kind)
			}
			if outs := p.OutgoingByNode["FIX_FLOW_NOT_APPROVED"]; len(outs) != 0 {
				t.Errorf("%s: FIX_FLOW_NOT_APPROVED must be terminal (no outgoing edges), got %d", tc.proc, len(outs))
			}
			// The reject terminal must be distinct from the count-cap terminal,
			// so a deliberate decline can never surface as FIX_LOOP_EXHAUSTED.
			notEdge(t, p, "GATE_FIX_FLOW_APPROVED", "FIX_LOOP_EXHAUSTED")
		})
	}
}

// Q31.a (CT nested under AT, plan 20260527-1147): implement-external-drivers-if-needed
// is guarded by GATE_TICKET_HAS_ESCC (ESCC is the sole signal — the broken
// GATE_EXTERNAL_DRIVER_PORTS_CHANGED fallback is removed). The true-branch enters
// the contract-test-first CT-HIGH (a superset that writes+verifies the contract
// test, real then stub, AND implements the external adapter).
// This is a pure structural assertion over the static graph — no execution
// walk, so no statemachine loop hazard.
func TestSharedContract_ExternalDriverGate_EntersContractTestHIGH(t *testing.T) {
	eng := loadSnapshot(t)

	// 1. The thin AT-only external-adapter HIGH is retired entirely.
	if _, ok := eng.Processes["implement-and-verify-external-system-driver-adapters"]; ok {
		t.Errorf("thin AT-only HIGH implement-and-verify-external-system-driver-adapters should be retired, still present")
	}

	// 2. implement-external-drivers-if-needed's ESCC gate true-branch reaches
	//    the external-driver adapter node; that node dispatches the CT-HIGH.
	ied, ok := eng.Processes["implement-external-drivers-if-needed"]
	if !ok {
		t.Fatalf("process implement-external-drivers-if-needed missing")
	}
	// The true-branch routes through the upfront registration check, then into
	// the external-driver adapter anchor (plan 20260615-0755). On the snapshot
	// graph the anchor is still the single template node (the unroll is an
	// in-memory load-time rewrite the driver applies, not baked into the YAML).
	wantEdge(t, ied, "GATE_TICKET_HAS_ESCC", "VALIDATE_EXTERNAL_SYSTEMS_REGISTERED", "ticket-has-escc == true")
	wantEdge(t, ied, "VALIDATE_EXTERNAL_SYSTEMS_REGISTERED", "IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS", "")
	if vnode, ok := ied.Nodes["VALIDATE_EXTERNAL_SYSTEMS_REGISTERED"]; !ok {
		t.Errorf("implement-external-drivers-if-needed: VALIDATE_EXTERNAL_SYSTEMS_REGISTERED node missing")
	} else if got := vnode.Raw.Action; got != "validate-external-systems-registered" {
		t.Errorf("VALIDATE action = %q, want validate-external-systems-registered", got)
	}
	node, ok := ied.Nodes["IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS"]
	if !ok {
		t.Fatalf("implement-external-drivers-if-needed: IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS node missing")
	}
	if got := node.Raw.Process; got != "implement-and-verify-external-system-driver-adapters-contract-tests" {
		t.Errorf("external-driver node process = %q, want the CT-HIGH", got)
	}
	// The CT-HIGH is self-contained except for the cascade tag (plan
	// 20260606-1525): it binds `test-category: contract` so its inner writers
	// namespace their port-changed verdicts under `ct-*` and can't clobber
	// the parent AT cascade's `at-*` verdicts.
	if got := node.Raw.Params["test-category"]; got != "contract" {
		t.Errorf("external-driver node should bind test-category: contract (cascade tag), got %q", got)
	}
	for _, p := range []string{"expected-test-result", "task-name"} {
		if v, set := node.Raw.Params[p]; set {
			t.Errorf("external-driver node should not bind %q (CT-HIGH is self-contained), got %q", p, v)
		}
	}

	// 3. The CT-HIGH starts with the per-clone RESOLVE_EXTERNAL_SYSTEM +
	//    GATE_EXTERNAL_SYSTEM_TOUCHED guard (plan 20260615-0755); a touched clone
	//    routes into write-contract-tests (the contract-test-writer agent) —
	//    proving a contract test is written.
	ct, ok := eng.Processes["implement-and-verify-external-system-driver-adapters-contract-tests"]
	if !ok {
		t.Fatalf("CT-HIGH process missing")
	}
	recon, ok := eng.Processes["reconcile-external-contract-producer"]
	if !ok {
		t.Fatalf("reconcile-external-contract-producer process missing")
	}
	if ct.Start != "RESOLVE_EXTERNAL_SYSTEM" {
		t.Errorf("CT-HIGH start = %q, want RESOLVE_EXTERNAL_SYSTEM", ct.Start)
	}
	wantEdge(t, ct, "GATE_EXTERNAL_SYSTEM_TOUCHED", "WRITE_CONTRACT_TESTS", "external-system-touched == true")
	start, ok := ct.Nodes["WRITE_CONTRACT_TESTS"]
	if !ok {
		t.Fatalf("CT-HIGH: WRITE_CONTRACT_TESTS node missing")
	}
	if got := start.Raw.Process; got != "write-contract-tests" {
		t.Errorf("CT-HIGH start node process = %q, want write-contract-tests", got)
	}
	wct, ok := eng.Processes["write-contract-tests"]
	if !ok {
		t.Fatalf("process write-contract-tests missing")
	}
	if ea, ok := wct.Nodes["EXECUTE_AGENT"]; !ok {
		t.Errorf("write-contract-tests: EXECUTE_AGENT node missing")
	} else if got := ea.Raw.Params["agent"]; got != "contract-test-writer" {
		t.Errorf("write-contract-tests agent = %q, want contract-test-writer", got)
	}

	// 4. The CT-HIGH walks the contract-real -> contract-stub probe split
	//    (plan 20260606-1943): both legs run the suite via run-tests and branch
	//    on the observed test-outcome — no asserted polarity.
	realNode, ok := recon.Nodes["PROBE_CONTRACT_REAL"]
	if !ok {
		t.Fatalf("CT-HIGH: PROBE_CONTRACT_REAL node missing")
	}
	if got := realNode.Raw.Process; got != "run-tests" {
		t.Errorf("PROBE_CONTRACT_REAL process = %q, want run-tests", got)
	}
	if got := realNode.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("contract-real probe suite = %q, want contract-real", got)
	}
	stubProbe, ok := recon.Nodes["PROBE_CONTRACT_STUB"]
	if !ok {
		t.Fatalf("CT-HIGH: PROBE_CONTRACT_STUB node missing")
	}
	if got := stubProbe.Raw.Process; got != "run-tests" {
		t.Errorf("PROBE_CONTRACT_STUB process = %q, want run-tests", got)
	}
	if got := stubProbe.Raw.Params["suite"]; got != "contract-stub" {
		t.Errorf("contract-stub probe suite = %q, want contract-stub", got)
	}
	// real-green precedes the stub probe (the split: if the real system already
	// honors the contract, proceed to the stub side; restart between them).
	wantEdge(t, recon, "GATE_CONTRACT_REAL_OUTCOME", "START_SYSTEM_BEFORE_STUB_PROBE", "test-outcome == pass")
	wantEdge(t, recon, "START_SYSTEM_BEFORE_STUB_PROBE", "PROBE_CONTRACT_STUB", "")

	// 5. The CT-HIGH's nested DSL step verifies against suite `contract-real`,
	//    decoupled from the `test-category: contract` path discriminator. Guards
	//    the bare-`--suite=contract` regression: `test-category` (the AT/CT path
	//    fence) and `suite` (the runner selector) must stay independent, so the
	//    filtered verify never copies the unregistered `contract` value.
	dslCaller, ok := ct.Nodes["IMPLEMENT_AND_VERIFY_DSL"]
	if !ok {
		t.Fatalf("CT-HIGH: IMPLEMENT_AND_VERIFY_DSL node missing")
	}
	if got := dslCaller.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("CT-HIGH DSL caller suite = %q, want contract-real", got)
	}
	if got := dslCaller.Raw.Params["test-category"]; got != "contract" {
		t.Errorf("CT-HIGH DSL caller test-category = %q, want contract (path discriminator unchanged)", got)
	}
	// The nested implement-and-verify-dsl threads ${suite} (not ${test-category})
	// into its filtered verify, so the caller's contract-real reaches the runner.
	dsl, ok := eng.Processes["implement-and-verify-dsl"]
	if !ok {
		t.Fatalf("process implement-and-verify-dsl missing")
	}
	if itl, ok := dsl.Nodes["IMPLEMENT_TEST_LAYER"]; !ok {
		t.Fatalf("implement-and-verify-dsl: IMPLEMENT_TEST_LAYER node missing")
	} else if got := itl.Raw.Params["suite"]; got != "${suite}" {
		t.Errorf("implement-and-verify-dsl IMPLEMENT_TEST_LAYER suite = %q, want ${suite}", got)
	}
	itlProc, ok := eng.Processes["implement-test-layer"]
	if !ok {
		t.Fatalf("process implement-test-layer missing")
	}
	if vf, ok := itlProc.Nodes["VERIFY_TESTS_PASS_FILTERED"]; !ok {
		t.Fatalf("implement-test-layer: VERIFY_TESTS_PASS_FILTERED node missing")
	} else if got := vf.Raw.Params["suite"]; got != "${suite}" {
		t.Errorf("VERIFY_TESTS_PASS_FILTERED suite = %q, want ${suite} (decoupled from ${test-category})", got)
	}
}

// TestContractTestHIGH_OutcomeDrivenFork pins the CT-HIGH real-side restructure
// (plan 20260606-1943, supersedes the polarity-prediction of plan 20260606-1356):
// the IDENTIFY service-task, the contract-real OUTCOME probe + gateway, and the
// red-kind sub-gateway — GREEN proceeds to the stub side, RED+simulator takes the
// implement→build/start→green branch that mirrors the stub side, and
// RED+test-instance halts on an upstream contract gap.
func TestContractTestHIGH_OutcomeDrivenFork(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ct, ok := eng.Processes["implement-and-verify-external-system-driver-adapters-contract-tests"]
	if !ok {
		t.Fatalf("CT-HIGH process missing")
	}
	recon, ok := eng.Processes["reconcile-external-contract-producer"]
	if !ok {
		t.Fatalf("reconcile-external-contract-producer process missing")
	}

	// 1. IDENTIFY is retired (plan 20260615-0755): identity + real-kind are
	//    baked at load by UnrollExternalSystems. The cycle starts with the
	//    per-clone RESOLVE_EXTERNAL_SYSTEM service-task (copies the baked
	//    real-kind into state + stamps external-system-touched), then the
	//    GATE_EXTERNAL_SYSTEM_TOUCHED self-guard routes touched clones into the
	//    cycle and untouched ones to the skip end-event.
	if got := ct.Start; got != "RESOLVE_EXTERNAL_SYSTEM" {
		t.Errorf("CT-HIGH start = %q, want RESOLVE_EXTERNAL_SYSTEM", got)
	}
	resolve, ok := ct.Nodes["RESOLVE_EXTERNAL_SYSTEM"]
	if !ok {
		t.Fatalf("CT-HIGH: RESOLVE_EXTERNAL_SYSTEM node missing")
	}
	if got := resolve.Raw.Action; got != "resolve-external-system" {
		t.Errorf("RESOLVE action = %q, want resolve-external-system", got)
	}
	wantEdge(t, ct, "RESOLVE_EXTERNAL_SYSTEM", "GATE_EXTERNAL_SYSTEM_TOUCHED", "")
	wantEdge(t, ct, "GATE_EXTERNAL_SYSTEM_TOUCHED", "WRITE_CONTRACT_TESTS", "external-system-touched == true")
	wantEdge(t, ct, "GATE_EXTERNAL_SYSTEM_TOUCHED", "EXTERNAL_SYSTEM_SKIPPED", "external-system-touched == false")
	if _, ok := ct.Nodes["IDENTIFY_EXTERNAL_SYSTEM"]; ok {
		t.Errorf("CT-HIGH: IDENTIFY_EXTERNAL_SYSTEM should be retired")
	}
	// The driver-adapter impl now flows straight into build/start → probe.
	wantEdge(t, ct, "IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS", "RECONCILE_EXTERNAL_CONTRACT_PRODUCER", "")
	wantEdge(t, recon, "START_SYSTEM_AFTER_DRIVER", "PROBE_CONTRACT_REAL", "")

	// 2. The probe runs the suite via run-tests (no asserted polarity); the
	//    outcome gateway routes on the stamped test-outcome.
	probe, ok := recon.Nodes["PROBE_CONTRACT_REAL"]
	if !ok {
		t.Fatalf("CT-HIGH: PROBE_CONTRACT_REAL node missing")
	}
	if got := probe.Raw.Process; got != "run-tests" {
		t.Errorf("PROBE_CONTRACT_REAL process = %q, want run-tests", got)
	}
	wantEdge(t, recon, "PROBE_CONTRACT_REAL", "GATE_CONTRACT_REAL_OUTCOME", "")
	outGate, ok := recon.Nodes["GATE_CONTRACT_REAL_OUTCOME"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_REAL_OUTCOME node missing")
	}
	if got := outGate.Raw.Binding; got != "test-outcome" {
		t.Errorf("GATE_CONTRACT_REAL_OUTCOME binding = %q, want test-outcome", got)
	}

	// 3. GREEN: external system already honors the contract → straight to the
	//    stub side. infra/unknown halt.
	wantEdge(t, recon, "GATE_CONTRACT_REAL_OUTCOME", "START_SYSTEM_BEFORE_STUB_PROBE", "test-outcome == pass")
	wantEdge(t, recon, "GATE_CONTRACT_REAL_OUTCOME", "GATE_CONTRACT_REAL_RED_KIND", "test-outcome == fail")
	wantEdge(t, recon, "GATE_CONTRACT_REAL_OUTCOME", "TESTS_INFRA_HALT", "test-outcome == infra")
	wantEdge(t, recon, "GATE_CONTRACT_REAL_OUTCOME", "UNKNOWN_TESTS_OUTCOME", "")

	// 4. RED: the red-kind sub-gateway routes on the stamped real-kind.
	redKind, ok := recon.Nodes["GATE_CONTRACT_REAL_RED_KIND"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_REAL_RED_KIND node missing")
	}
	if got := redKind.Raw.Binding; got != "real-kind" {
		t.Errorf("GATE_CONTRACT_REAL_RED_KIND binding = %q, want real-kind", got)
	}
	// simulator: we own it → implement → rebuild/restart → GREEN → stub side.
	wantEdge(t, recon, "GATE_CONTRACT_REAL_RED_KIND", "IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR", "real-kind == simulator")
	wantEdge(t, recon, "IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR", "BUILD_SYSTEM_AFTER_SIMULATOR", "")
	wantEdge(t, recon, "BUILD_SYSTEM_AFTER_SIMULATOR", "START_SYSTEM_AFTER_SIMULATOR", "")
	wantEdge(t, recon, "START_SYSTEM_AFTER_SIMULATOR", "VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR", "")
	wantEdge(t, recon, "VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR", "START_SYSTEM_BEFORE_STUB_PROBE", "")
	// test-instance: we do NOT own it → upstream contract-gap hard halt (an
	// error-end-event so it bubbles up, never the code-fixer).
	wantEdge(t, recon, "GATE_CONTRACT_REAL_RED_KIND", "CONTRACT_REAL_UPSTREAM_GAP_HALT", "real-kind == test-instance")
	if n := recon.Nodes["CONTRACT_REAL_UPSTREAM_GAP_HALT"]; n.Kind != statemachine.ErrorEndEvent {
		t.Errorf("CONTRACT_REAL_UPSTREAM_GAP_HALT kind = %v, want statemachine.ErrorEndEvent", n.Kind)
	}

	// The post-sim GREEN verify targets contract-real (verify-tests-pass).
	if n := recon.Nodes["VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR"]; n.Raw.Process != "verify-tests-pass" {
		t.Errorf("post-sim verify process = %q, want verify-tests-pass", n.Raw.Process)
	} else if got := n.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("simulator GREEN-verify suite = %q, want contract-real", got)
	}

	// 4b. The contract-stub leg is likewise an outcome probe (no fail-verify):
	//     GREEN → done, RED → implement stubs → red→green.
	stubGate, ok := recon.Nodes["GATE_CONTRACT_STUB_OUTCOME"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_STUB_OUTCOME node missing")
	}
	if got := stubGate.Raw.Binding; got != "test-outcome" {
		t.Errorf("GATE_CONTRACT_STUB_OUTCOME binding = %q, want test-outcome", got)
	}
	wantEdge(t, recon, "PROBE_CONTRACT_STUB", "GATE_CONTRACT_STUB_OUTCOME", "")
	// Both stub-green paths (already-green and implemented+verified) now converge
	// on the stub-fidelity leg's presence gate (plan 20260620-2348), not directly
	// on the cycle end.
	wantEdge(t, recon, "GATE_CONTRACT_STUB_OUTCOME", "GATE_STUB_FIDELITY_PRESENT", "test-outcome == pass")
	wantEdge(t, recon, "GATE_CONTRACT_STUB_OUTCOME", "IMPLEMENT_EXTERNAL_SYSTEM_STUBS", "test-outcome == fail")
	wantEdge(t, recon, "VERIFY_TESTS_PASS_CONTRACT_STUB", "GATE_STUB_FIDELITY_PRESENT", "")

	// 4c. The stub-only fidelity leg (plan 20260620-2348): a presence gate skips
	//     the whole leg when no `Stub only:` register was authored (empty
	//     ct-isolated-test-names), otherwise an outcome-driven probe → implement
	//     stubs → rebuild/restart → verify, all selecting the isolated register
	//     (test-names: ${ct-isolated-test-names}) against suite contract-stub.
	presGate, ok := recon.Nodes["GATE_STUB_FIDELITY_PRESENT"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_STUB_FIDELITY_PRESENT node missing")
	}
	if got := presGate.Raw.Binding; got != "stub-fidelity-tests-present" {
		t.Errorf("GATE_STUB_FIDELITY_PRESENT binding = %q, want stub-fidelity-tests-present", got)
	}
	wantEdge(t, recon, "GATE_STUB_FIDELITY_PRESENT", "PROBE_CONTRACT_STUB_ISOLATED", "stub-fidelity-tests-present == true")
	wantEdge(t, recon, "GATE_STUB_FIDELITY_PRESENT", "RECONCILE_PRODUCER_END", "stub-fidelity-tests-present == false")
	if got := ct.Nodes["WRITE_STUB_FIDELITY_TESTS"].Raw.Process; got != "write-stub-fidelity-tests" {
		t.Errorf("WRITE_STUB_FIDELITY_TESTS process = %q, want write-stub-fidelity-tests", got)
	}
	wantEdge(t, ct, "WRITE_CONTRACT_TESTS", "WRITE_STUB_FIDELITY_TESTS", "")
	wantEdge(t, ct, "WRITE_STUB_FIDELITY_TESTS", "GATE_DSL_PORT_CHANGED", "")
	isoGate, ok := recon.Nodes["GATE_CONTRACT_STUB_ISOLATED_OUTCOME"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_STUB_ISOLATED_OUTCOME node missing")
	}
	if got := isoGate.Raw.Binding; got != "test-outcome" {
		t.Errorf("GATE_CONTRACT_STUB_ISOLATED_OUTCOME binding = %q, want test-outcome", got)
	}
	wantEdge(t, recon, "PROBE_CONTRACT_STUB_ISOLATED", "GATE_CONTRACT_STUB_ISOLATED_OUTCOME", "")
	wantEdge(t, recon, "GATE_CONTRACT_STUB_ISOLATED_OUTCOME", "RECONCILE_PRODUCER_END", "test-outcome == pass")
	wantEdge(t, recon, "GATE_CONTRACT_STUB_ISOLATED_OUTCOME", "IMPLEMENT_EXTERNAL_SYSTEM_STUBS_ISOLATED", "test-outcome == fail")
	wantEdge(t, recon, "IMPLEMENT_EXTERNAL_SYSTEM_STUBS_ISOLATED", "BUILD_SYSTEM_AFTER_STUBS_ISOLATED", "")
	wantEdge(t, recon, "BUILD_SYSTEM_AFTER_STUBS_ISOLATED", "START_SYSTEM_AFTER_STUBS_ISOLATED", "")
	wantEdge(t, recon, "START_SYSTEM_AFTER_STUBS_ISOLATED", "VERIFY_TESTS_PASS_CONTRACT_STUB_ISOLATED", "")
	wantEdge(t, recon, "VERIFY_TESTS_PASS_CONTRACT_STUB_ISOLATED", "RECONCILE_PRODUCER_END", "")
	if n := recon.Nodes["VERIFY_TESTS_PASS_CONTRACT_STUB_ISOLATED"]; n.Raw.Params["test-names"] != "${ct-isolated-test-names}" {
		t.Errorf("isolated verify test-names = %q, want ${ct-isolated-test-names}", n.Raw.Params["test-names"])
	}

	// 5. The simulator MID dispatches the mirror agent and WRITES the
	//    producer-side simulator stand-in dir (external-system-simulator — the
	//    registry-projected scope key, plan 20260622-1739 Step 3), NOT the
	//    consumer testkit driver-adapter, plus the shared test-transport
	//    foundation (system-driver-adapter-shared), the shared common primitives
	//    (common), and the shared domain value types (domain-value-types). It
	//    READS the full coupled trio (driver-adapter + simulator + stub) so it
	//    emits a shape consistent with the consumer DTO and the sibling stub.
	sim, ok := eng.Processes["implement-external-system-real-simulator"]
	if !ok {
		t.Fatalf("process implement-external-system-real-simulator missing")
	}
	ea, ok := sim.Nodes["EXECUTE_AGENT"]
	if !ok {
		t.Fatalf("implement-external-system-real-simulator: EXECUTE_AGENT node missing")
	}
	if got := ea.Raw.Params["agent"]; got != "external-system-real-simulator-implementer" {
		t.Errorf("simulator MID agent = %q, want external-system-real-simulator-implementer", got)
	}
	wantSimRead := []string{"external-system-driver-adapter", "external-system-simulator", "external-system-stub", "system-driver-adapter-shared", "common", "domain-value-types"}
	if got := ea.Raw.Read; !slices.Equal(got, wantSimRead) {
		t.Errorf("simulator MID read scope = %v, want %v", got, wantSimRead)
	}
	wantSimWrite := []string{"external-system-simulator", "system-driver-adapter-shared", "common", "domain-value-types"}
	if got := ea.Raw.Write; !slices.Equal(got, wantSimWrite) {
		t.Errorf("simulator MID write scope = %v, want %v", got, wantSimWrite)
	}

	// 5b. The stub MID is the sibling of the simulator MID: it WRITES the
	//    producer-side stub stand-in dir (external-system-stub), NOT the
	//    consumer testkit driver-adapter, and READS the same coupled trio.
	stub, ok := eng.Processes["implement-external-system-stubs"]
	if !ok {
		t.Fatalf("process implement-external-system-stubs missing")
	}
	stubEA, ok := stub.Nodes["EXECUTE_AGENT"]
	if !ok {
		t.Fatalf("implement-external-system-stubs: EXECUTE_AGENT node missing")
	}
	wantStubRead := []string{"external-system-driver-adapter", "external-system-simulator", "external-system-stub", "system-driver-adapter-shared", "common", "domain-value-types"}
	if got := stubEA.Raw.Read; !slices.Equal(got, wantStubRead) {
		t.Errorf("stub MID read scope = %v, want %v", got, wantStubRead)
	}
	wantStubWrite := []string{"external-system-stub", "system-driver-adapter-shared", "common", "domain-value-types"}
	if got := stubEA.Raw.Write; !slices.Equal(got, wantStubWrite) {
		t.Errorf("stub MID write scope = %v, want %v", got, wantStubWrite)
	}
}

// TestRedesignExternalSystemStructure_Wiring pins the redesign-external cycle's
// reuse of the extracted reconcile leg (plan 20260622-1739 Step 4b/4c): the cycle
// fronts the per-system unroll anchor with the ESCC-required guard + the
// registered-systems guard, the anchor is a call-activity into the per-system
// body, and the body runs the same resolve+touched self-guard as the CT clone,
// then reshapes the consumer adapter and calls reconcile-external-contract-producer
// with ct-test-names pinned empty (whole-suite probe; no contract tests are
// authored on the redesign path). The final full regression runs after the
// unrolled clones. Loaded from the static (un-unrolled) snapshot, so the anchor is
// still the single template node here.
func TestRedesignExternalSystemStructure_Wiring(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	cyc, ok := eng.Processes["redesign-external-system-structure"]
	if !ok {
		t.Fatalf("redesign-external-system-structure process missing")
	}

	// 1. Upfront guards: ESCC-required (the redesign path's only selection source)
	//    then the shared registered-systems guard, before the per-system anchor.
	if cyc.Start != "VALIDATE_REDESIGN_EXTERNAL_REQUIRES_ESCC" {
		t.Errorf("redesign-external start = %q, want VALIDATE_REDESIGN_EXTERNAL_REQUIRES_ESCC", cyc.Start)
	}
	if got := cyc.Nodes["VALIDATE_REDESIGN_EXTERNAL_REQUIRES_ESCC"].Raw.Action; got != "validate-redesign-external-requires-escc" {
		t.Errorf("ESCC guard action = %q, want validate-redesign-external-requires-escc", got)
	}
	if got := cyc.Nodes["VALIDATE_EXTERNAL_SYSTEMS_REGISTERED"].Raw.Action; got != "validate-external-systems-registered" {
		t.Errorf("registered guard action = %q, want validate-external-systems-registered", got)
	}
	wantEdge(t, cyc, "VALIDATE_REDESIGN_EXTERNAL_REQUIRES_ESCC", "VALIDATE_EXTERNAL_SYSTEMS_REGISTERED", "")
	wantEdge(t, cyc, "VALIDATE_EXTERNAL_SYSTEMS_REGISTERED", "REDESIGN_EXTERNAL_SYSTEM", "")

	// 2. The per-system anchor is a call-activity into the per-system body, and the
	//    final full-regression re-green runs after it (after the unrolled clones).
	anchor, ok := cyc.Nodes["REDESIGN_EXTERNAL_SYSTEM"]
	if !ok {
		t.Fatalf("redesign-external: REDESIGN_EXTERNAL_SYSTEM anchor missing")
	}
	if anchor.Raw.Process != "redesign-external-system-per-system" {
		t.Errorf("anchor calls %q, want redesign-external-system-per-system", anchor.Raw.Process)
	}
	wantEdge(t, cyc, "REDESIGN_EXTERNAL_SYSTEM", "IMPLEMENT_AND_VERIFY_SYSTEM", "")
	if got := cyc.Nodes["IMPLEMENT_AND_VERIFY_SYSTEM"].Raw.Params["action"]; got != "update-system" {
		t.Errorf("final regression action = %q, want update-system", got)
	}

	// 3. The per-system body: resolve + touched self-guard (the CT clone's), then
	//    reshape consumer adapter → reconcile producer with ct-test-names pinned
	//    empty; an untouched clone skips.
	body, ok := eng.Processes["redesign-external-system-per-system"]
	if !ok {
		t.Fatalf("redesign-external-system-per-system process missing")
	}
	if body.Start != "RESOLVE_EXTERNAL_SYSTEM" {
		t.Errorf("per-system start = %q, want RESOLVE_EXTERNAL_SYSTEM", body.Start)
	}
	if got := body.Nodes["RESOLVE_EXTERNAL_SYSTEM"].Raw.Action; got != "resolve-external-system" {
		t.Errorf("per-system resolve action = %q, want resolve-external-system", got)
	}
	if got := body.Nodes["GATE_EXTERNAL_SYSTEM_TOUCHED"].Raw.Binding; got != "external-system-touched" {
		t.Errorf("per-system guard binding = %q, want external-system-touched", got)
	}
	wantEdge(t, body, "RESOLVE_EXTERNAL_SYSTEM", "GATE_EXTERNAL_SYSTEM_TOUCHED", "")
	wantEdge(t, body, "GATE_EXTERNAL_SYSTEM_TOUCHED", "UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS", "external-system-touched == true")
	wantEdge(t, body, "GATE_EXTERNAL_SYSTEM_TOUCHED", "EXTERNAL_SYSTEM_SKIPPED", "external-system-touched == false")
	if got := body.Nodes["UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS"].Raw.Process; got != "update-external-system-driver-adapters" {
		t.Errorf("consumer reshape process = %q, want update-external-system-driver-adapters", got)
	}
	wantEdge(t, body, "UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS", "RECONCILE_EXTERNAL_CONTRACT_PRODUCER", "")
	reconCall, ok := body.Nodes["RECONCILE_EXTERNAL_CONTRACT_PRODUCER"]
	if !ok {
		t.Fatalf("per-system: RECONCILE_EXTERNAL_CONTRACT_PRODUCER node missing")
	}
	if reconCall.Raw.Process != "reconcile-external-contract-producer" {
		t.Errorf("reconcile call process = %q, want reconcile-external-contract-producer", reconCall.Raw.Process)
	}
	// ct-test-names pinned empty so the reused leg's strict ${ct-test-names}
	// expansions resolve and the probes run the whole suite (no new tests written).
	if got, ok := reconCall.Raw.Params["ct-test-names"]; !ok || got != "" {
		t.Errorf("reconcile call ct-test-names param = %q (present=%v), want \"\" (present)", got, ok)
	}
	wantEdge(t, body, "RECONCILE_EXTERNAL_CONTRACT_PRODUCER", "REDESIGN_EXTERNAL_PER_SYSTEM_END", "")
}

// TestChannelTouchedGuard_Wiring asserts the in-cycle channel guard wiring
// (plan 20260619-1139): both per-channel cycles start with a resolve-channel
// service-task followed by a GATE_CHANNEL_TOUCHED gateway that routes a touched
// channel into the cycle and an untouched one to a CHANNEL_SKIPPED end-event,
// and write-acceptance-tests-and-dsl runs validate-channels-registered once
// after the RED acceptance verify. This is the static template the channel
// unrolls clone, so the guard rides into every per-channel clone.
func TestChannelTouchedGuard_Wiring(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// System GREEN step (UnrollSystemChannels target).
	sys, ok := eng.Processes["implement-and-verify-system"]
	if !ok {
		t.Fatalf("process implement-and-verify-system missing")
	}
	if got := sys.Start; got != "RESOLVE_CHANNEL" {
		t.Errorf("implement-and-verify-system start = %q, want RESOLVE_CHANNEL", got)
	}
	if got := sys.Nodes["RESOLVE_CHANNEL"].Raw.Action; got != "resolve-channel" {
		t.Errorf("RESOLVE_CHANNEL action = %q, want resolve-channel", got)
	}
	if got := sys.Nodes["GATE_CHANNEL_TOUCHED"].Raw.Binding; got != "channel-touched" {
		t.Errorf("GATE_CHANNEL_TOUCHED binding = %q, want channel-touched", got)
	}
	if n := sys.Nodes["CHANNEL_SKIPPED"]; n.Kind != statemachine.EndEvent {
		t.Errorf("CHANNEL_SKIPPED kind = %v, want statemachine.EndEvent (a benign skip, not a halt)", n.Kind)
	}
	wantEdge(t, sys, "RESOLVE_CHANNEL", "GATE_CHANNEL_TOUCHED", "")
	wantEdge(t, sys, "GATE_CHANNEL_TOUCHED", "RUN_ACTION", "channel-touched == true")
	wantEdge(t, sys, "GATE_CHANNEL_TOUCHED", "CHANNEL_SKIPPED", "channel-touched == false")

	// Test-side System Driver adapter step (UnrollSystemDriverAdapterChannels
	// target) — same guard, gating IMPLEMENT_TEST_LAYER.
	adapt, ok := eng.Processes["implement-and-verify-system-driver-adapters"]
	if !ok {
		t.Fatalf("process implement-and-verify-system-driver-adapters missing")
	}
	if got := adapt.Start; got != "RESOLVE_CHANNEL" {
		t.Errorf("implement-and-verify-system-driver-adapters start = %q, want RESOLVE_CHANNEL", got)
	}
	if got := adapt.Nodes["RESOLVE_CHANNEL"].Raw.Action; got != "resolve-channel" {
		t.Errorf("driver-adapter RESOLVE_CHANNEL action = %q, want resolve-channel", got)
	}
	wantEdge(t, adapt, "GATE_CHANNEL_TOUCHED", "IMPLEMENT_TEST_LAYER", "channel-touched == true")
	wantEdge(t, adapt, "GATE_CHANNEL_TOUCHED", "CHANNEL_SKIPPED", "channel-touched == false")

	// Upfront no-silent-skip guard runs once in write-acceptance-tests-and-dsl,
	// between the RED acceptance verify and the DSL gate (so the report exists
	// and no clone has run yet).
	watd, ok := eng.Processes["write-acceptance-tests-and-dsl"]
	if !ok {
		t.Fatalf("process write-acceptance-tests-and-dsl missing")
	}
	if got := watd.Nodes["VALIDATE_CHANNELS_REGISTERED"].Raw.Action; got != "validate-channels-registered" {
		t.Errorf("VALIDATE_CHANNELS_REGISTERED action = %q, want validate-channels-registered", got)
	}
	wantEdge(t, watd, "WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE", "VALIDATE_CHANNELS_REGISTERED", "")
	wantEdge(t, watd, "VALIDATE_CHANNELS_REGISTERED", "GATE_DSL_PORT_CHANGED", "")
}

func wantEdge(t *testing.T, proc *statemachine.Process, from, to, predicate string) {
	t.Helper()
	for _, e := range proc.OutgoingByNode[from] {
		if e.To == to && e.Predicate == predicate {
			return
		}
	}
	t.Errorf("process %q: missing edge %s -> %s when %q", proc.ID, from, to, predicate)
}

func notEdge(t *testing.T, proc *statemachine.Process, from, to string) {
	t.Helper()
	for _, e := range proc.OutgoingByNode[from] {
		if e.To == to {
			t.Errorf("process %q: unexpected edge %s -> %s (predicate %q)", proc.ID, from, to, e.Predicate)
		}
	}
}

// TestCoverPath_GreenWhenComplete_Wiring asserts the plan 20260606-1518 wiring:
// cover-system-behavior pins verify-mode on its WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS
// call-activity, each AT layer pins its plumbing scope, the CT-HIGH overrides
// back to red so green mode can't leak in, the two verify gates route on the
// mode-aware at-verify-expectation binding, and external driver adapters are
// lifted to cycle level (precede acceptance tests; no terminal AT-green tail).
// change-system-behavior must NOT pin verify-mode so the gate defaults to red.
func TestCoverPath_GreenWhenComplete_Wiring(t *testing.T) {
	eng := loadSnapshot(t)
	proc := func(id string) *statemachine.Process {
		p, ok := eng.Processes[id]
		if !ok {
			t.Fatalf("process %q missing", id)
		}
		return p
	}

	// 1. Cover path pins green-when-complete on the WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS
	//    call-activity inside cover-system-behavior; change path leaves it unset.
	coverCsb := proc("cover-system-behavior")
	changeCsb := proc("change-system-behavior")
	if got := coverCsb.Nodes["WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS"].Raw.Params["verify-mode"]; got != "green-when-complete" {
		t.Errorf("cover-system-behavior WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS verify-mode = %q, want green-when-complete", got)
	}
	if got, set := changeCsb.Nodes["WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS"].Raw.Params["verify-mode"]; set {
		t.Errorf("change-system-behavior must NOT pin verify-mode (defaults red), got %q", got)
	}

	// 2. Each AT layer pins its plumbing scope (inherits down to its verify gate).
	watd := proc("write-acceptance-tests-and-dsl")
	if got := watd.Nodes["WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE"].Raw.Params["verify-pending-on"]; got != "dsl" {
		t.Errorf("test-code layer verify-pending-on = %q, want dsl", got)
	}
	if got := watd.Nodes["IMPLEMENT_AND_VERIFY_DSL"].Raw.Params["verify-pending-on"]; got != "system-drivers" {
		t.Errorf("DSL layer verify-pending-on = %q, want system-drivers", got)
	}
	if got := proc("write-acceptance-tests-and-system-adapters").Nodes["IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS"].Raw.Params["verify-pending-on"]; got != "none" {
		t.Errorf("adapter layer verify-pending-on = %q, want none", got)
	}

	// 3. The CT-HIGH excursion overrides back to red so green mode can't leak in.
	ied := proc("implement-external-drivers-if-needed")
	if got := ied.Nodes["IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS"].Raw.Params["verify-mode"]; got != "red" {
		t.Errorf("CT-HIGH verify-mode = %q, want red", got)
	}

	// 4. Both verify gates route on the mode-aware binding.
	for _, p := range []struct{ proc, node string }{
		{"write-and-verify-acceptance-test-code", "GATE_EXPECTED_TEST_RESULT"},
		{"implement-test-layer", "GATE_EXPECTED_TEST_RESULT"},
	} {
		if got := proc(p.proc).Nodes[p.node].Raw.Binding; got != "at-verify-expectation" {
			t.Errorf("%s/%s binding = %q, want at-verify-expectation", p.proc, p.node, got)
		}
	}

	// 5. External drivers are lifted to cycle level: implement-external-drivers-if-needed
	//    precedes write-acceptance-tests-and-system-adapters in both cycle entry processes.
	wantEdge(t, changeCsb, "IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED", "WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS", "")
	wantEdge(t, coverCsb, "IMPLEMENT_EXTERNAL_DRIVERS_IF_NEEDED", "WRITE_ACCEPTANCE_TESTS_AND_SYSTEM_ADAPTERS", "")
	if _, ok := watd.Nodes["GATE_AT_TERMINAL_GREEN"]; ok {
		t.Errorf("GATE_AT_TERMINAL_GREEN should be gone — adapters now precede acceptance tests")
	}
}

// TestContractDSL_CompileOnly_VerifyModeNone locks plan 20260606-2330: the
// CT-HIGH DSL/port layer compiles + commits only and never asserts the contract
// suite's polarity, so the generic fix-unexpected-failing-tests can't fire on
// the pre-adapter contract red and front-run the dedicated
// external-system-driver-adapter-implementer. These assertions FAIL on HEAD
// before the plan lands (no `none` edge, no verify-mode pin) — proof the change
// bites — and pass after Items 1–3.
func TestContractDSL_CompileOnly_VerifyModeNone(t *testing.T) {
	eng := loadSnapshot(t)

	// 1. implement-test-layer gains the compile-only skip edge: a `none`
	//    outcome routes straight to commit, bypassing both VERIFY_TESTS_* nodes.
	itl, ok := eng.Processes["implement-test-layer"]
	if !ok {
		t.Fatalf("process implement-test-layer missing")
	}
	wantEdge(t, itl, "GATE_EXPECTED_TEST_RESULT", "COMMIT_LAYER", "at-verify-expectation == none")

	// The predicate evaluator agrees: at-verify-expectation == none lands on
	// COMMIT_LAYER, NOT VERIFY_TESTS_PASS_FILTERED / VERIFY_TESTS_FAIL_FILTERED.
	ctx := statemachine.NewContext()
	ctx.Set("at-verify-expectation", "none")
	to, err := eng.NextEdge("implement-test-layer", "GATE_EXPECTED_TEST_RESULT", ctx)
	if err != nil {
		t.Fatalf("NextEdge(GATE_EXPECTED_TEST_RESULT, none): %v", err)
	}
	if to != "COMMIT_LAYER" {
		t.Errorf("none outcome routes to %q, want COMMIT_LAYER (no contract-suite verify between DSL impl and adapter impl)", to)
	}

	// 2. The CT-HIGH DSL/port step pins verify-mode: none, overriding the
	//    verify-mode: red inherited from the shared-contract CT-HIGH wrapper, so
	//    no contract-suite verify sits between the DSL impl and
	//    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS.
	ct, ok := eng.Processes["implement-and-verify-external-system-driver-adapters-contract-tests"]
	if !ok {
		t.Fatalf("CT-HIGH process missing")
	}
	if got := ct.Nodes["IMPLEMENT_AND_VERIFY_DSL"].Raw.Params["verify-mode"]; got != "none" {
		t.Errorf("CT-HIGH DSL caller verify-mode = %q, want none", got)
	}
	// The DSL impl is immediately followed by the dedicated adapter implementer
	// (no verify node interposed in the topology).
	wantEdge(t, ct, "IMPLEMENT_AND_VERIFY_DSL", "IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS", "")

	// 3. Regression guard: the success/failure edges other callers rely on are
	//    untouched, so this change is additive — only `none` is new.
	successCtx := statemachine.NewContext()
	successCtx.Set("at-verify-expectation", "success")
	if got, err := eng.NextEdge("implement-test-layer", "GATE_EXPECTED_TEST_RESULT", successCtx); err != nil {
		t.Fatalf("NextEdge(success): %v", err)
	} else if got != "VERIFY_TESTS_PASS_FILTERED" {
		t.Errorf("success outcome routes to %q, want VERIFY_TESTS_PASS_FILTERED (unchanged)", got)
	}
}

// TestImplementTicket_TwoAxisRouting asserts the wired two-axis ticket
// gateway (plan 20260606-1637): with the bindings emitting bare `task` on the
// kind axis and the bare subtype on the subtype axis, a task ticket routes
// *past* GATE_TICKET_KIND into GATE_TASK_SUBTYPE and on to its cycle — not into
// the UNKNOWN_TICKET_KIND error end that the old composite `task/<sub>` value
// fell through to. Story/bug still route to CHANGE_SYSTEM_BEHAVIOR. Uses
// NextEdge (predicate eval over seeded state) rather than a full walk, per the
// statemachine-test-loop hazard.
func TestImplementTicket_TwoAxisRouting(t *testing.T) {
	eng := loadSnapshot(t)

	route := func(t *testing.T, from string, state map[string]string) string {
		t.Helper()
		ctx := statemachine.NewContext()
		for k, v := range state {
			ctx.Set(k, v)
		}
		to, err := eng.NextEdge("implement-ticket", from, ctx)
		if err != nil {
			t.Fatalf("NextEdge(%s) with %v: %v", from, state, err)
		}
		return to
	}

	// Kind axis: story/bug → change cycle; task → the subtype gateway, NOT
	// the unknown-kind error end.
	if got := route(t, "GATE_TICKET_KIND", map[string]string{"ticket-kind": "story"}); got != "CHANGE_SYSTEM_BEHAVIOR" {
		t.Errorf("story routes to %q, want CHANGE_SYSTEM_BEHAVIOR", got)
	}
	if got := route(t, "GATE_TICKET_KIND", map[string]string{"ticket-kind": "bug"}); got != "CHANGE_SYSTEM_BEHAVIOR" {
		t.Errorf("bug routes to %q, want CHANGE_SYSTEM_BEHAVIOR", got)
	}
	if got := route(t, "GATE_TICKET_KIND", map[string]string{"ticket-kind": "task"}); got != "GATE_TASK_SUBTYPE" {
		t.Errorf("task routes to %q, want GATE_TASK_SUBTYPE (regression: must not dead-end at UNKNOWN_TICKET_KIND)", got)
	}

	// Subtype axis: each of the 5 subtypes reaches its cycle.
	for _, tc := range []struct{ subtype, cycle string }{
		{"legacy-coverage", "COVER_SYSTEM_BEHAVIOR"},
		{"system-redesign", "REDESIGN_SYSTEM_STRUCTURE"},
		{"external-system-redesign", "REDESIGN_EXTERNAL_SYSTEM_STRUCTURE"},
		{"system-refactor", "REFACTOR_SYSTEM_STRUCTURE"},
		{"test-refactor", "REFACTOR_TEST_STRUCTURE"},
	} {
		if got := route(t, "GATE_TASK_SUBTYPE", map[string]string{"task-subtype": tc.subtype}); got != tc.cycle {
			t.Errorf("subtype %q routes to %q, want %s", tc.subtype, got, tc.cycle)
		}
	}
}

// SETUP_TESTS must be the unconditional first activity of implement-ticket,
// upstream of the ticket-kind gateway and outside verify-tests-pass's fix
// loop, so test-harness deps are installed before any path reaches the first
// run-tests — for every ticket kind, not just the redesign path that crashed
// with ERR_MODULE_NOT_FOUND (plan 20260617-1456). implement-ticket is the
// only top-level entry that reaches run-tests, so making setup its start node
// is complete coverage. Placing it before the FIX_* → RUN_TESTS back-edge
// also keeps repeated fix iterations from each paying a fresh `npm ci`.
func TestSetupTests_PrecedesImplementTicketGateway(t *testing.T) {
	eng := loadSnapshot(t)

	// 1. setup-tests is the start node of implement-ticket and a call-activity
	//    into the setup-tests sub-process.
	it, ok := eng.Processes["implement-ticket"]
	if !ok {
		t.Fatalf("process implement-ticket missing")
	}
	if it.Start != "SETUP_TESTS" {
		t.Fatalf("implement-ticket start = %q, want SETUP_TESTS", it.Start)
	}
	setupNode, ok := it.Nodes["SETUP_TESTS"]
	if !ok {
		t.Fatalf("implement-ticket: SETUP_TESTS node missing")
	}
	if setupNode.Kind != statemachine.CallActivity {
		t.Errorf("SETUP_TESTS kind = %v, want CallActivity", setupNode.Kind)
	}
	if got := setupNode.Raw.Process; got != "setup-tests" {
		t.Errorf("SETUP_TESTS process = %q, want setup-tests", got)
	}

	// 2. Setup precedes the ticket-kind gateway: its only successor is
	//    MARK_IN_PROGRESS, which leads into PARSE_TICKET → GATE_TICKET_KIND.
	//    This puts setup above the kind/subtype routing for every ticket kind.
	wantEdge(t, it, "SETUP_TESTS", "MARK_IN_PROGRESS", "")
	if got := len(it.OutgoingByNode["SETUP_TESTS"]); got != 1 {
		t.Errorf("SETUP_TESTS outgoing edges = %d, want 1 (unconditional)", got)
	}

	// 3. The setup-tests sub-process dispatches `gh optivem system-test setup`, the
	//    language-agnostic command that resolves the active tier's setupCommands.
	st, ok := eng.Processes["setup-tests"]
	if !ok {
		t.Fatalf("process setup-tests missing")
	}
	cmdNode, ok := st.Nodes[st.Start]
	if !ok {
		t.Fatalf("setup-tests: start node %q missing", st.Start)
	}
	if got := cmdNode.Raw.Process; got != "execute-command" {
		t.Errorf("setup-tests start node process = %q, want execute-command", got)
	}
	if got := cmdNode.Raw.Params["command"]; got != "gh optivem system-test setup" {
		t.Errorf("setup-tests command = %q, want `gh optivem system-test setup`", got)
	}
}
