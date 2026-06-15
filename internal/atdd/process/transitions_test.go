// Transitions / structural test suite for the concrete ATDD process-flow.
//
// Scope: structural invariants over the loaded ATDD process document plus the
// per-process routing regressions. These load the concrete process via
// process.Load(), so they live in the process package and reference the
// generic engine types through the statemachine. qualifier.
package process_test

import (
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
	// 20260606-1525): it binds `test-category: contract` so its inner writers
	// namespace their port-changed verdicts under `ct-*` and can't clobber
	// the parent AT cascade's `at-*` verdicts. It still forwards none of the
	// other caller params the retired thin step used to.
	if got := node.Raw.Params["test-category"]; got != "contract" {
		t.Errorf("external-driver node should bind test-category: contract (cascade tag), got %q", got)
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

	// 4. The CT-HIGH walks the contract-real -> contract-stub probe split
	//    (plan 20260606-1943): both legs run the suite via run-tests and branch
	//    on the observed test-outcome — no asserted polarity.
	realNode, ok := ct.Nodes["PROBE_CONTRACT_REAL"]
	if !ok {
		t.Fatalf("CT-HIGH: PROBE_CONTRACT_REAL node missing")
	}
	if got := realNode.Raw.Process; got != "run-tests" {
		t.Errorf("PROBE_CONTRACT_REAL process = %q, want run-tests", got)
	}
	if got := realNode.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("contract-real probe suite = %q, want contract-real", got)
	}
	stubProbe, ok := ct.Nodes["PROBE_CONTRACT_STUB"]
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
	wantEdge(t, ct, "GATE_CONTRACT_REAL_OUTCOME", "START_SYSTEM_BEFORE_STUB_PROBE", "test-outcome == pass")
	wantEdge(t, ct, "START_SYSTEM_BEFORE_STUB_PROBE", "PROBE_CONTRACT_STUB", "")

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
	// Build/start happen between identity and the probe.
	wantEdge(t, ct, "IDENTIFY_EXTERNAL_SYSTEM", "BUILD_SYSTEM_AFTER_DRIVER", "")
	wantEdge(t, ct, "START_SYSTEM_AFTER_DRIVER", "PROBE_CONTRACT_REAL", "")

	// 2. The probe runs the suite via run-tests (no asserted polarity); the
	//    outcome gateway routes on the stamped test-outcome.
	probe, ok := ct.Nodes["PROBE_CONTRACT_REAL"]
	if !ok {
		t.Fatalf("CT-HIGH: PROBE_CONTRACT_REAL node missing")
	}
	if got := probe.Raw.Process; got != "run-tests" {
		t.Errorf("PROBE_CONTRACT_REAL process = %q, want run-tests", got)
	}
	wantEdge(t, ct, "PROBE_CONTRACT_REAL", "GATE_CONTRACT_REAL_OUTCOME", "")
	outGate, ok := ct.Nodes["GATE_CONTRACT_REAL_OUTCOME"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_REAL_OUTCOME node missing")
	}
	if got := outGate.Raw.Binding; got != "test-outcome" {
		t.Errorf("GATE_CONTRACT_REAL_OUTCOME binding = %q, want test-outcome", got)
	}

	// 3. GREEN: external system already honors the contract → straight to the
	//    stub side. infra/unknown halt.
	wantEdge(t, ct, "GATE_CONTRACT_REAL_OUTCOME", "START_SYSTEM_BEFORE_STUB_PROBE", "test-outcome == pass")
	wantEdge(t, ct, "GATE_CONTRACT_REAL_OUTCOME", "GATE_CONTRACT_REAL_RED_KIND", "test-outcome == fail")
	wantEdge(t, ct, "GATE_CONTRACT_REAL_OUTCOME", "TESTS_INFRA_HALT", "test-outcome == infra")
	wantEdge(t, ct, "GATE_CONTRACT_REAL_OUTCOME", "UNKNOWN_TESTS_OUTCOME", "")

	// 4. RED: the red-kind sub-gateway routes on the stamped real-kind.
	redKind, ok := ct.Nodes["GATE_CONTRACT_REAL_RED_KIND"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_REAL_RED_KIND node missing")
	}
	if got := redKind.Raw.Binding; got != "real-kind" {
		t.Errorf("GATE_CONTRACT_REAL_RED_KIND binding = %q, want real-kind", got)
	}
	// simulator: we own it → implement → rebuild/restart → GREEN → stub side.
	wantEdge(t, ct, "GATE_CONTRACT_REAL_RED_KIND", "IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR", "real-kind == simulator")
	wantEdge(t, ct, "IMPLEMENT_EXTERNAL_SYSTEM_REAL_SIMULATOR", "BUILD_SYSTEM_AFTER_SIMULATOR", "")
	wantEdge(t, ct, "BUILD_SYSTEM_AFTER_SIMULATOR", "START_SYSTEM_AFTER_SIMULATOR", "")
	wantEdge(t, ct, "START_SYSTEM_AFTER_SIMULATOR", "VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR", "")
	wantEdge(t, ct, "VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR", "START_SYSTEM_BEFORE_STUB_PROBE", "")
	// test-instance: we do NOT own it → upstream contract-gap hard halt (an
	// error-end-event so it bubbles up, never the code-fixer).
	wantEdge(t, ct, "GATE_CONTRACT_REAL_RED_KIND", "CONTRACT_REAL_UPSTREAM_GAP_HALT", "real-kind == test-instance")
	if n := ct.Nodes["CONTRACT_REAL_UPSTREAM_GAP_HALT"]; n.Kind != statemachine.ErrorEndEvent {
		t.Errorf("CONTRACT_REAL_UPSTREAM_GAP_HALT kind = %v, want statemachine.ErrorEndEvent", n.Kind)
	}

	// The post-sim GREEN verify targets contract-real (verify-tests-pass).
	if n := ct.Nodes["VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR"]; n.Raw.Process != "verify-tests-pass" {
		t.Errorf("post-sim verify process = %q, want verify-tests-pass", n.Raw.Process)
	} else if got := n.Raw.Params["suite"]; got != "contract-real" {
		t.Errorf("simulator GREEN-verify suite = %q, want contract-real", got)
	}

	// 4b. The contract-stub leg is likewise an outcome probe (no fail-verify):
	//     GREEN → done, RED → implement stubs → red→green.
	stubGate, ok := ct.Nodes["GATE_CONTRACT_STUB_OUTCOME"]
	if !ok {
		t.Fatalf("CT-HIGH: GATE_CONTRACT_STUB_OUTCOME node missing")
	}
	if got := stubGate.Raw.Binding; got != "test-outcome" {
		t.Errorf("GATE_CONTRACT_STUB_OUTCOME binding = %q, want test-outcome", got)
	}
	wantEdge(t, ct, "PROBE_CONTRACT_STUB", "GATE_CONTRACT_STUB_OUTCOME", "")
	wantEdge(t, ct, "GATE_CONTRACT_STUB_OUTCOME", "IMPL_EXT_DRIVER_CT_END", "test-outcome == pass")
	wantEdge(t, ct, "GATE_CONTRACT_STUB_OUTCOME", "IMPLEMENT_EXTERNAL_SYSTEM_STUBS", "test-outcome == fail")

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
// the cover wrapper pins verify-mode, each AT layer pins its plumbing scope, the
// CT-HIGH overrides back to red so green mode can't leak in, the two verify
// gates route on the mode-aware at-verify-expectation binding, and the case-D
// terminal AT-green tail exists. The change wrapper must NOT pin verify-mode so
// the gate defaults to red and the change path is unchanged.
func TestCoverPath_GreenWhenComplete_Wiring(t *testing.T) {
	eng := loadSnapshot(t)
	proc := func(id string) *statemachine.Process {
		p, ok := eng.Processes[id]
		if !ok {
			t.Fatalf("process %q missing", id)
		}
		return p
	}

	// 1. Cover wrapper pins green-when-complete; change wrapper leaves it unset.
	if got := proc("write-and-verify-acceptance-tests-pass").Nodes["WRITE_AND_VERIFY_ACCEPTANCE_TESTS"].Raw.Params["verify-mode"]; got != "green-when-complete" {
		t.Errorf("cover wrapper verify-mode = %q, want green-when-complete", got)
	}
	if got, set := proc("write-and-verify-acceptance-tests-fail").Nodes["WRITE_AND_VERIFY_ACCEPTANCE_TESTS"].Raw.Params["verify-mode"]; set {
		t.Errorf("change wrapper must NOT pin verify-mode (defaults red), got %q", got)
	}

	// 2. Each AT layer pins its plumbing scope (inherits down to its verify gate).
	sc := proc("shared-contract")
	if got := sc.Nodes["WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE"].Raw.Params["verify-pending-on"]; got != "dsl" {
		t.Errorf("test-code layer verify-pending-on = %q, want dsl", got)
	}
	if got := sc.Nodes["IMPLEMENT_AND_VERIFY_DSL"].Raw.Params["verify-pending-on"]; got != "drivers" {
		t.Errorf("DSL layer verify-pending-on = %q, want drivers", got)
	}
	if got := proc("write-and-verify-acceptance-tests").Nodes["IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS"].Raw.Params["verify-pending-on"]; got != "none" {
		t.Errorf("adapter layer verify-pending-on = %q, want none", got)
	}

	// 3. The CT-HIGH excursion overrides back to red so green mode can't leak in.
	if got := sc.Nodes["IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS"].Raw.Params["verify-mode"]; got != "red" {
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

	// 5. Case-D terminal AT-green tail in shared-contract.
	wantEdge(t, sc, "IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS", "GATE_AT_TERMINAL_GREEN", "")
	wantEdge(t, sc, "GATE_AT_TERMINAL_GREEN", "START_SYSTEM_AT_TERMINAL", "at-external-terminal-verify-needed == true")
	wantEdge(t, sc, "GATE_AT_TERMINAL_GREEN", "SHARED_CONTRACT_END", "at-external-terminal-verify-needed == false")
	wantEdge(t, sc, "START_SYSTEM_AT_TERMINAL", "VERIFY_TESTS_PASS_ACCEPTANCE_TERMINAL", "")
	wantEdge(t, sc, "VERIFY_TESTS_PASS_ACCEPTANCE_TERMINAL", "SHARED_CONTRACT_END", "")
	if got := sc.Nodes["GATE_AT_TERMINAL_GREEN"].Raw.Binding; got != "at-external-terminal-verify-needed" {
		t.Errorf("GATE_AT_TERMINAL_GREEN binding = %q, want at-external-terminal-verify-needed", got)
	}
	term := sc.Nodes["VERIFY_TESTS_PASS_ACCEPTANCE_TERMINAL"]
	if got := term.Raw.Process; got != "verify-tests-pass" {
		t.Errorf("terminal verify process = %q, want verify-tests-pass", got)
	}
	if got := term.Raw.Params["suite"]; got != "acceptance" {
		t.Errorf("terminal verify suite = %q, want acceptance", got)
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
