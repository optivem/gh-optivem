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
		"shared-contract",
		"write-and-verify-acceptance-test-code",
		"implement-and-verify-dsl",
		"implement-and-verify-system-driver-adapters",
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
	if gate.Kind != Gateway {
		t.Errorf("execute-agent: GATE_SCOPE_EXCEPTION_REQUESTED kind = %v, want Gateway", gate.Kind)
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

	// 4. Exception requested -> hard halt; not requested -> normal validation.
	wantEdge(t, proc, "GATE_SCOPE_EXCEPTION_REQUESTED", "STOP_SCOPE_VIOLATION", "scope-exception-requested == true")
	wantEdge(t, proc, "GATE_SCOPE_EXCEPTION_REQUESTED", "GATE_OUTPUTS_AND_SCOPES_VALID", "scope-exception-requested == false")
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
//	verify-tests-pass : FIX_UNEXPECTED_FAILING_TESTS → RUN_TESTS
//	verify-tests-fail : FIX_UNEXPECTED_PASSING_TESTS → RUN_TESTS
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
		if node.Kind != ErrorEndEvent {
			t.Errorf("fix: FIX_REJECTED_END kind = %v, want ErrorEndEvent (reject must halt, not soft-skip)", node.Kind)
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

// Q31.a (CT nested under AT, plan 20260527-1147): shared-contract's
// external-driver gate true-branch now enters the contract-test-first CT-HIGH
// (a superset that writes+verifies the contract test, real then stub, AND
// implements the external adapter) instead of the retired thin AT-only step.
// This is a pure structural assertion over the static graph — no execution
// walk, so no statemachine loop hazard.
func TestSharedContract_ExternalDriverGate_EntersContractTestHIGH(t *testing.T) {
	eng := loadSnapshot(t)

	// 1. The thin AT-only external-adapter HIGH is retired entirely.
	if _, ok := eng.Processes["implement-and-verify-external-system-driver-adapters"]; ok {
		t.Errorf("thin AT-only HIGH implement-and-verify-external-system-driver-adapters should be retired, still present")
	}

	// 2. shared-contract's external-driver gate true-branch reaches the
	//    external-driver adapter node; that node now dispatches the CT-HIGH.
	sc, ok := eng.Processes["shared-contract"]
	if !ok {
		t.Fatalf("process shared-contract missing")
	}
	wantEdge(t, sc, "GATE_EXTERNAL_DRIVER_PORTS_CHANGED", "IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS", "at-external-driver-port-changed == true")
	node, ok := sc.Nodes["IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS"]
	if !ok {
		t.Fatalf("shared-contract: IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS node missing")
	}
	if got := node.Raw.Process; got != "implement-and-verify-external-system-driver-adapters-contract-tests" {
		t.Errorf("external-driver node process = %q, want the CT-HIGH", got)
	}
	// The CT-HIGH is self-contained except for the cascade tag (plan
	// 20260606-1525): it binds `tests: contract` so its inner writers
	// namespace their port-changed verdicts under `ct-*` and can't clobber
	// the parent AT cascade's `at-*` verdicts. It still forwards none of the
	// other caller params the retired thin step used to.
	if got := node.Raw.Params["tests"]; got != "contract" {
		t.Errorf("external-driver node should bind tests: contract (cascade tag), got %q", got)
	}
	for _, p := range []string{"expected-test-result", "task-name"} {
		if v, set := node.Raw.Params[p]; set {
			t.Errorf("external-driver node should not bind %q (CT-HIGH is self-contained), got %q", p, v)
		}
	}

	// 3. The CT-HIGH starts by dispatching write-contract-tests (the
	//    contract-test-writer agent) — proving a contract test is written.
	ct, ok := eng.Processes["implement-and-verify-external-system-driver-adapters-contract-tests"]
	if !ok {
		t.Fatalf("CT-HIGH process missing")
	}
	if ct.Start != "WRITE_CONTRACT_TESTS" {
		t.Errorf("CT-HIGH start = %q, want WRITE_CONTRACT_TESTS", ct.Start)
	}
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

	// 4. The CT-HIGH walks the contract-real -> contract-stub verify split.
	realNode, ok := ct.Nodes["VERIFY_TESTS_PASS_CONTRACT_REAL"]
	if !ok {
		t.Fatalf("CT-HIGH: VERIFY_TESTS_PASS_CONTRACT_REAL node missing")
	}
	if got := realNode.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("contract-real verify suite = %q, want contract-real", got)
	}
	stubFail, ok := ct.Nodes["VERIFY_TESTS_FAIL_CONTRACT_STUB"]
	if !ok {
		t.Fatalf("CT-HIGH: VERIFY_TESTS_FAIL_CONTRACT_STUB node missing")
	}
	if got := stubFail.Raw.Params["suite"]; got != "contract-stub" {
		t.Errorf("contract-stub fail-verify suite = %q, want contract-stub", got)
	}
	// real-pass precedes stub-fail (the split: prove tests pass against the
	// real system, then fail against the not-yet-implemented stub).
	wantEdge(t, ct, "VERIFY_TESTS_PASS_CONTRACT_REAL", "START_SYSTEM_BEFORE_STUB_FAIL", "")
	wantEdge(t, ct, "START_SYSTEM_BEFORE_STUB_FAIL", "VERIFY_TESTS_FAIL_CONTRACT_STUB", "")

	// 5. The CT-HIGH's nested DSL step verifies against suite `contract-real`,
	//    decoupled from the `tests: contract` path discriminator. Guards the
	//    bare-`--suite=contract` regression: `tests` (the AT/CT path fence)
	//    and `suite` (the runner selector) must stay independent, so the
	//    filtered verify never copies the unregistered `contract` value.
	dslCaller, ok := ct.Nodes["IMPLEMENT_AND_VERIFY_DSL"]
	if !ok {
		t.Fatalf("CT-HIGH: IMPLEMENT_AND_VERIFY_DSL node missing")
	}
	if got := dslCaller.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("CT-HIGH DSL caller suite = %q, want contract-real", got)
	}
	if got := dslCaller.Raw.Params["tests"]; got != "contract" {
		t.Errorf("CT-HIGH DSL caller tests = %q, want contract (path discriminator unchanged)", got)
	}
	// The nested implement-and-verify-dsl threads ${suite} (not ${tests}) into
	// its filtered verify, so the caller's contract-real reaches the runner.
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
		t.Errorf("VERIFY_TESTS_PASS_FILTERED suite = %q, want ${suite} (decoupled from ${tests})", got)
	}
}

// TestContractTestHIGH_RealKindFork pins the CT-HIGH real-side restructure
// (plan 20260606-1356): the IDENTIFY service-task, the real-kind gateway, and
// both gate branches — test-instance collapsing to a single pass-verify, and
// simulator taking the red→implement→build/start→green branch that mirrors the
// stub side.
func TestContractTestHIGH_RealKindFork(t *testing.T) {
	eng, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	ct, ok := eng.Processes["implement-and-verify-external-system-driver-adapters-contract-tests"]
	if !ok {
		t.Fatalf("CT-HIGH process missing")
	}

	// 1. IDENTIFY runs AFTER the driver-adapter impl (so phase-changed-files
	//    carries the per-system paths) and is a deterministic service-task.
	wantEdge(t, ct, "IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS", "IDENTIFY_EXTERNAL_SYSTEM", "")
	identify, ok := ct.Nodes["IDENTIFY_EXTERNAL_SYSTEM"]
	if !ok {
		t.Fatalf("CT-HIGH: IDENTIFY_EXTERNAL_SYSTEM node missing")
	}
	if got := identify.Raw.Action; got != "identify-external-system" {
		t.Errorf("IDENTIFY action = %q, want identify-external-system", got)
	}
	// Build/start happen between identity and the gate.
	wantEdge(t, ct, "IDENTIFY_EXTERNAL_SYSTEM", "BUILD_SYSTEM_AFTER_DRIVER", "")
	wantEdge(t, ct, "START_SYSTEM_AFTER_DRIVER", "GATE_REAL_KIND", "")

	// 2. The real-kind gateway routes contract-real on the stamped real-kind.
	gate, ok := ct.Nodes["GATE_REAL_KIND"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_REAL_KIND node missing")
	}
	if got := gate.Raw.Binding; got != "real-kind" {
		t.Errorf("GATE_REAL_KIND binding = %q, want real-kind", got)
	}

	// 3. test-instance branch: a single contract-real pass-verify, then the
	//    stub side. (collapsed — no fail-verify, no simulator impl.)
	wantEdge(t, ct, "GATE_REAL_KIND", "VERIFY_TESTS_PASS_CONTRACT_REAL", "real-kind == test-instance")
	wantEdge(t, ct, "VERIFY_TESTS_PASS_CONTRACT_REAL", "START_SYSTEM_BEFORE_STUB_FAIL", "")

	// 4. simulator branch: contract-real RED → implement simulator →
	//    rebuild/restart → contract-real GREEN → stub side.
	wantEdge(t, ct, "GATE_REAL_KIND", "VERIFY_TESTS_FAIL_CONTRACT_REAL", "real-kind == simulator")
	wantEdge(t, ct, "VERIFY_TESTS_FAIL_CONTRACT_REAL", "IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR", "")
	wantEdge(t, ct, "IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR", "BUILD_SYSTEM_AFTER_SIMULATOR", "")
	wantEdge(t, ct, "BUILD_SYSTEM_AFTER_SIMULATOR", "START_SYSTEM_AFTER_SIMULATOR", "")
	wantEdge(t, ct, "START_SYSTEM_AFTER_SIMULATOR", "VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR", "")
	wantEdge(t, ct, "VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR", "START_SYSTEM_BEFORE_STUB_FAIL", "")

	// The RED verify targets contract-real (verify-tests-fail), the post-sim
	// GREEN verify targets contract-real (verify-tests-pass).
	if n := ct.Nodes["VERIFY_TESTS_FAIL_CONTRACT_REAL"]; n.Raw.Process != "verify-tests-fail" {
		t.Errorf("VERIFY_TESTS_FAIL_CONTRACT_REAL process = %q, want verify-tests-fail", n.Raw.Process)
	} else if got := n.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("simulator RED-verify suite = %q, want contract-real", got)
	}
	if n := ct.Nodes["VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR"]; n.Raw.Process != "verify-tests-pass" {
		t.Errorf("post-sim verify process = %q, want verify-tests-pass", n.Raw.Process)
	} else if got := n.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("simulator GREEN-verify suite = %q, want contract-real", got)
	}

	// 5. The new simulator MID dispatches the mirror agent and scopes writes
	//    to external-system-driver-adapter (fork #2) plus the shared
	//    test-transport foundation (system-driver-adapter-shared), which the
	//    simulator sits on (mirrors the stub MID).
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
	if got := ea.Raw.Write; len(got) != 2 || got[0] != "external-system-driver-adapter" || got[1] != "system-driver-adapter-shared" {
		t.Errorf("simulator MID write scope = %v, want [external-system-driver-adapter system-driver-adapter-shared]", got)
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
