package process_test

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// TestExecuteCommand_FailureDispatchesCommandFailedDiagnoserAgent is the
// end-to-end verification for the recovery-path wiring (plans 20260526-1530
// Item 3 + 20260526-1701 task-name/agent split). It loads the canonical
// embedded YAML and drives `execute-command` with a stubbed `run-command`
// action that simulates a shell failure (stamps `command-succeeded=false`,
// `failure-kind=command-failed`). The expected trail is:
//
//	execute-command.RUN_COMMAND → GATE_COMMAND_SUCCEEDED=false → FIX
//	  → fix.EXECUTE_AGENT (params task-name="fix-${failure-kind}",
//	                              agent="${failure-kind}-fixer")
//	    → execute-agent.RUN_AGENT (agent: ${agent})
//
// At RUN_AGENT the engine resolves `${agent}` against ctx.Params, which
// in turn was expanded with `${failure-kind}` resolved via statemachine.ExpandParams's
// state fallback. The recorded AgentFn dispatch must be the literal
// `"command-failed-fixer"` — if Items 1, 2, the YAML wiring, or the
// 1701 task-name/agent split regresses, this test fails before runtime
// sees a missing-prompt error.
//
// Memory `feedback_statemachine_test_loop_hazard`: execute-command now
// loops `FIX → RUN_COMMAND`, so the run-command stub MUST flip
// `command-succeeded` on the second call or the walk diverges. The
// inner execute-agent dispatched from fix has `fix-on-failure: false`
// and so does not recurse; execute-agent's own `FIX → RUN_AGENT` loop
// stays bounded because the inner validate stub returns true on its
// only call.
func TestExecuteCommand_FailureDispatchesCommandFailedFixerAgent(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	var dispatchedAgents []string
	runCommandCalls := 0
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "run-command":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				runCommandCalls++
				if runCommandCalls == 1 {
					ctx.Set("command-succeeded", false)
					ctx.Set("failure-kind", "command-failed")
				} else {
					// Second visit (after FIX → RUN_COMMAND loopback):
					// flip so the cycle exits via EXECUTE_COMMAND_END.
					ctx.Set("command-succeeded", true)
				}
				return statemachine.Outcome{}
			}
		case "validate-outputs-and-scopes":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				ctx.Set("outputs-and-scopes-valid", true)
				return statemachine.Outcome{}
			}
		default:
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		return func(ctx *statemachine.Context) statemachine.Outcome {
			dispatchedAgents = append(dispatchedAgents, name)
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = func(name string) statemachine.NodeFn {
		switch name {
		case "approval-outcome":
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{Value: "approved"} }
		case "fix-on-failure-enabled":
			// Mirrors gates.bindings.fixOnFailureEnabled at the test
			// stub level: default true, "false" param disables. The
			// inner execute-agent (called from fix) sets this to
			// "false" so the test doesn't recurse into a second fix.
			return func(ctx *statemachine.Context) statemachine.Outcome {
				return statemachine.Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
			}
		default:
			// Generic state-reading gate: read whatever the action
			// stamped under the binding name. Covers
			// command-succeeded and outputs-and-scopes-valid in this
			// walk without per-gate stubs.
			return func(ctx *statemachine.Context) statemachine.Outcome {
				v, ok := ctx.State[name]
				if !ok {
					return statemachine.Outcome{}
				}
				switch t := v.(type) {
				case bool:
					return statemachine.Outcome{Bool: t}
				case string:
					return statemachine.Outcome{Value: t}
				default:
					return statemachine.Outcome{}
				}
			}
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := statemachine.NewContext()
	ctx.Params["command"] = "noisy-broken-cmd"
	// task-name mirrors the production scope: execute-command is always
	// called from inside a writing-agent phase whose outer call-activity
	// binds task-name. fix's EXECUTE_AGENT reads it via
	// `originating-task-name: ${task-name}` so the fix-agent inherits the
	// outer phase's scope identity. Under strict-mode statemachine.ExpandParams the
	// binding is mandatory; the bare `command:` setup the test used
	// previously relied on the now-removed silent-leak behavior.
	ctx.Params["task-name"] = "synthetic-test-phase"
	// category threads through execute-command's APPROVE_PRE; in production
	// the command-MID caller supplies this. The test runs the primitive
	// directly so it must supply the equivalent literal.
	ctx.Params["category"] = "command"
	if err := eng.RunProcess("execute-command", ctx); err != nil {
		t.Fatalf("RunProcess execute-command: %v", err)
	}

	// The approve sub-process dispatches the human user-task multiple
	// times during the walk — we ignore those and look for the
	// command-failed-fixer dispatch.
	foundFixer := false
	for _, name := range dispatchedAgents {
		if name == "command-failed-fixer" {
			foundFixer = true
		}
		// Guard against the original bug surface: an unresolved
		// template leaking into AgentFn. Catches a regression where
		// statemachine.ExpandParams loses the state-fallback path.
		if strings.Contains(name, "${") {
			t.Errorf("agent name with unresolved template leaked into dispatch: %q (full dispatch trail: %v)", name, dispatchedAgents)
		}
	}
	if !foundFixer {
		t.Errorf("RUN_AGENT did not dispatch command-failed-fixer; full dispatch trail: %v", dispatchedAgents)
	}
}

// TestExecuteAgent_ValidationFailureDispatchesFixerForFailureKind is
// the twin of TestExecuteCommand_FailureDispatchesCommandFailedFixerAgent
// for the `execute-agent` → `fix` → `execute-agent` recovery branch (plan
// 20260526-1530 Item 4 + 20260526-1701 task-name/agent split).
// `validateOutputsAndScopes` writes one of two failure-kinds —
// `missing-output` or `scope-diff` — and the recovery path must dispatch
// the matching `<kind>-fixer` agent (noun form, post-1701 split).
//
// Memory `feedback_statemachine_test_loop_hazard`: execute-agent now
// loops `FIX → RUN_AGENT`, so after the outer fix dispatch returns the
// outer RUN_AGENT runs again, hitting validate-outputs-and-scopes a
// third time. The validateCalls counter below makes calls 2 and 3
// return `outputs-and-scopes-valid=true`, so the loopback exits via
// APPROVE_POST. `fix-on-failure: false` on the inner execute-agent
// keeps its FIX branch off, preventing second-level recursion.
func TestExecuteAgent_ValidationFailureDispatchesFixerForFailureKind(t *testing.T) {
	cases := []struct {
		failureKind string
		wantAgent   string
	}{
		// validateOutputsAndScopes priority is missing-output wins
		// over scope-diff (bindings.go), so each case here pins the
		// observable kind after the action's own routing decision.
		{failureKind: "missing-output", wantAgent: "missing-output-fixer"},
		{failureKind: "scope-diff", wantAgent: "scope-diff-fixer"},
	}
	for _, tc := range cases {
		t.Run(tc.failureKind, func(t *testing.T) {
			eng, err := process.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			var dispatchedAgents []string
			// Limits the validate stub to writing failure-kind on the
			// FIRST call; the inner fix's execute-agent would otherwise
			// rewrite it (harmless here, but the trail stays simpler).
			validateCalls := 0
			eng.ActionFn = func(name string) statemachine.NodeFn {
				switch name {
				case "validate-outputs-and-scopes":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						validateCalls++
						if validateCalls == 1 {
							ctx.Set("outputs-and-scopes-valid", false)
							ctx.Set("failure-kind", tc.failureKind)
						} else {
							// Inner fix's execute-agent: pass validation
							// so the walk terminates cleanly.
							ctx.Set("outputs-and-scopes-valid", true)
						}
						return statemachine.Outcome{}
					}
				default:
					return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
				}
			}
			eng.AgentFn = func(name string) statemachine.NodeFn {
				return func(ctx *statemachine.Context) statemachine.Outcome {
					dispatchedAgents = append(dispatchedAgents, name)
					return statemachine.Outcome{}
				}
			}
			eng.GateFn = func(name string) statemachine.NodeFn {
				switch name {
				case "approval-outcome":
					return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{Value: "approved"} }
				case "fix-on-failure-enabled":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						return statemachine.Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
					}
				default:
					return func(ctx *statemachine.Context) statemachine.Outcome {
						v, ok := ctx.State[name]
						if !ok {
							return statemachine.Outcome{}
						}
						switch t := v.(type) {
						case bool:
							return statemachine.Outcome{Bool: t}
						case string:
							return statemachine.Outcome{Value: t}
						default:
							return statemachine.Outcome{}
						}
					}
				}
			}
			if err := eng.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}

			ctx := statemachine.NewContext()
			// Outer execute-agent call: a hypothetical writing-agent
			// dispatch. The names don't matter — validate-outputs-and-
			// scopes is the action that decides the recovery branch.
			// Both `task-name` (verb) and `agent` (noun) must be set
			// post-1701 split — RUN_AGENT resolves `agent: ${agent}`
			// and an empty value would leak `${agent}` into dispatch.
			ctx.Params["task-name"] = "some-writing-agent"
			ctx.Params["agent"] = "some-agent-noun"
			// category threads through execute-agent's APPROVE_PRE /
			// APPROVE_POST; in production the writing-agent MID caller
			// supplies this. The test runs the primitive directly.
			ctx.Params["category"] = "prod-agent"
			if err := eng.RunProcess("execute-agent", ctx); err != nil {
				t.Fatalf("RunProcess execute-agent: %v", err)
			}

			found := false
			for _, name := range dispatchedAgents {
				if name == tc.wantAgent {
					found = true
				}
				if strings.Contains(name, "${") {
					t.Errorf("agent name with unresolved template leaked into dispatch: %q (full dispatch trail: %v)", name, dispatchedAgents)
				}
			}
			if !found {
				t.Errorf("RUN_AGENT did not dispatch %q; full dispatch trail: %v", tc.wantAgent, dispatchedAgents)
			}
		})
	}
}

// TestVerifyTests_InfraOutcomeReachesInfraHalt walks both
// `verify-tests-pass` and `verify-tests-fail` end-to-end with a
// run-command stub that stamps `test-outcome="infra"` — mirroring
// what `runCommand` does when the runner shell-out fails with stderr
// matching one of the `verify_classify.go` infra patterns. The walk
// must reach TESTS_INFRA_HALT (an error-end-event) and the engine
// must surface a non-nil error from RunProcess so the failure
// propagates up the call-activity chain instead of being silently
// absorbed.
//
// This is the seam-composition test: the action stamping infra
// (covered by TestRunCommand_RunTestsClassifiesInfraFailure in the
// actions package), the gate accepting infra (covered by
// gates/bindings_test.go), and the YAML routing (covered by
// transitions_test.go's wantEdge) are each tested in isolation;
// this confirms they compose so an infra failure cannot reach a
// plain end-event by some unanticipated path.
func TestVerifyTests_InfraOutcomeReachesInfraHalt(t *testing.T) {
	for _, proc := range []string{"verify-tests-pass", "verify-tests-fail"} {
		t.Run(proc, func(t *testing.T) {
			eng, err := process.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			eng.ActionFn = func(name string) statemachine.NodeFn {
				switch name {
				case "run-command":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						ctx.Set("command-succeeded", false)
						ctx.Set("test-outcome", "infra")
						ctx.Set("test-infra-label", "missing executable")
						return statemachine.Outcome{}
					}
				case "validate-outputs-and-scopes":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						ctx.Set("outputs-and-scopes-valid", true)
						return statemachine.Outcome{}
					}
				default:
					return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
				}
			}
			eng.AgentFn = func(name string) statemachine.NodeFn {
				return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
			}
			// Gate stub mirrors TestExecuteCommand_FailureDispatchesCommandFailedFixerAgent:
			// auto-approve every approval gate (the walk hits APPROVE_PRE
			// inside execute-command), pin fix-on-failure-enabled to the
			// param value (run-tests sets it false), and otherwise read
			// state — which is how testOutcome surfaces the stub-stamped
			// "infra" through the verify-tests-* sequence-flow guarded by
			// `test-outcome == infra`.
			eng.GateFn = func(name string) statemachine.NodeFn {
				switch name {
				case "approval-outcome":
					return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{Value: "approved"} }
				case "fix-on-failure-enabled":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						return statemachine.Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
					}
				default:
					return func(ctx *statemachine.Context) statemachine.Outcome {
						v, ok := ctx.State[name]
						if !ok {
							return statemachine.Outcome{}
						}
						switch t := v.(type) {
						case bool:
							return statemachine.Outcome{Bool: t}
						case string:
							return statemachine.Outcome{Value: t}
						default:
							return statemachine.Outcome{}
						}
					}
				}
			}
			if err := eng.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}
			ctx := statemachine.NewContext()
			ctx.Params["suite"] = "acceptance"
			ctx.Params["test-names"] = "synthetic-test"
			err = eng.RunProcess(proc, ctx)
			if err == nil {
				t.Fatalf("RunProcess(%s) returned nil; want error from TESTS_INFRA_HALT", proc)
			}
			if !strings.Contains(err.Error(), "TESTS_INFRA_HALT") {
				t.Fatalf("RunProcess(%s) error = %q; want it to reference TESTS_INFRA_HALT (infra-classified test runs must halt, not silently advance)", proc, err.Error())
			}
		})
	}
}

// TestVerifyTests_FixLoopHaltsAtCap is the flow-level proof of the
// fix-attempt cap on the REAL embedded process-flow (plan 20260530-1339
// Items 2-4). It drives verify-tests-pass / verify-tests-fail with a
// run-command stub that NEVER converges — it always re-stamps the outcome
// that routes back into the fixer — so under today's flow the FIX_* node
// would re-dispatch until maxDispatchesPerProcess fires ~10000 dispatches
// later. With `max-visits: 2` + `on-max-visits: FIX_LOOP_EXHAUSTED`, the
// walk must instead halt at FIX_LOOP_EXHAUSTED after exactly N=2 fix
// attempts.
//
// The assertions pin the SPECIFIC halt, not the 10000 backstop:
//   - the error references FIX_LOOP_EXHAUSTED and NOT "exceeded … dispatches", and
//   - run-command fired exactly N+1=3 times (RUN_TESTS runs once per lap;
//     the 3rd lap's FIX arrival is intercepted before re-dispatch).
//
// feedback_statemachine_test_loop_hazard: the cap (and, failing it, the
// 10000 backstop) bounds this fixture, so it cannot 20GB the suite even
// if the cap regressed — but we assert on the cap, so a regression to the
// backstop fails the run-command-count check loudly rather than hanging.
func TestVerifyTests_FixLoopHaltsAtCap(t *testing.T) {
	cases := []struct {
		proc        string
		testOutcome string
		succeeded   bool
	}{
		// verify-tests-pass routes to FIX when tests unexpectedly FAIL.
		{proc: "verify-tests-pass", testOutcome: "fail", succeeded: false},
		// verify-tests-fail routes to FIX when must-fail tests unexpectedly PASS.
		{proc: "verify-tests-fail", testOutcome: "pass", succeeded: true},
	}
	for _, c := range cases {
		t.Run(c.proc, func(t *testing.T) {
			eng, err := process.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			runCommandCalls := 0
			eng.ActionFn = func(name string) statemachine.NodeFn {
				switch name {
				case "run-command":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						runCommandCalls++
						// Never converge: re-stamp the outcome that routes
						// back into the fixer on every lap.
						ctx.Set("command-succeeded", c.succeeded)
						ctx.Set("test-outcome", c.testOutcome)
						return statemachine.Outcome{}
					}
				case "validate-outputs-and-scopes":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						ctx.Set("outputs-and-scopes-valid", true)
						return statemachine.Outcome{}
					}
				case "check-fix-progress":
					// verify-tests-pass's no-progress guard (plan 20260615-1845
					// Step 4) sits on the fail branch before the fixer. This
					// fixture exercises the COUNT cap, so it models a loop that
					// keeps making progress (never the identical failure twice)
					// — stamp progressing=true so the no-progress guard never
					// short-circuits and FIX_LOOP_EXHAUSTED stays the halt under
					// test. (verify-tests-fail has no such node; the case is a
					// no-op there.)
					return func(ctx *statemachine.Context) statemachine.Outcome {
						ctx.Set("fix-loop-progressing", true)
						return statemachine.Outcome{}
					}
				default:
					return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
				}
			}
			eng.AgentFn = func(name string) statemachine.NodeFn {
				return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
			}
			eng.GateFn = func(name string) statemachine.NodeFn {
				switch name {
				case "approval-outcome":
					return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{Value: "approved"} }
				case "fix-on-failure-enabled":
					return func(ctx *statemachine.Context) statemachine.Outcome {
						return statemachine.Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
					}
				default:
					return func(ctx *statemachine.Context) statemachine.Outcome {
						v, ok := ctx.State[name]
						if !ok {
							return statemachine.Outcome{}
						}
						switch t := v.(type) {
						case bool:
							return statemachine.Outcome{Bool: t}
						case string:
							return statemachine.Outcome{Value: t}
						default:
							return statemachine.Outcome{}
						}
					}
				}
			}
			if err := eng.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}

			ctx := statemachine.NewContext()
			ctx.Params["suite"] = "acceptance"
			ctx.Params["test-names"] = "synthetic-test"
			err = eng.RunProcess(c.proc, ctx)
			if err == nil {
				t.Fatalf("RunProcess(%s) returned nil; want halt at FIX_LOOP_EXHAUSTED", c.proc)
			}
			if !strings.Contains(err.Error(), "FIX_LOOP_EXHAUSTED") {
				t.Fatalf("RunProcess(%s) error = %q; want it to reference FIX_LOOP_EXHAUSTED", c.proc, err.Error())
			}
			if strings.Contains(err.Error(), "exceeded") {
				t.Errorf("RunProcess(%s) hit the maxDispatchesPerProcess backstop, not the max-visits cap: %q", c.proc, err.Error())
			}
			// N=2 fix attempts => RUN_TESTS runs on laps 1, 2, and 3 (the
			// 3rd lap routes to the halt at the FIX arrival, before re-run).
			if runCommandCalls != 3 {
				t.Errorf("run-command fired %d time(s), want 3 (N+1) — the cap must halt after exactly 2 fix attempts", runCommandCalls)
			}
		})
	}
}

// TestExecuteCommand_FixLoopHaltsAtCap is the flow-level proof of the
// fix-attempt cap on the REAL embedded execute-command (plan 20260530-1604
// Step 2). It drives execute-command with a run-command stub that NEVER
// succeeds, so under the old flow FIX -> RUN_COMMAND would spin until the
// maxDispatchesPerProcess backstop (~2500 cycles — the rehearsal-71
// endless re-prompt). With max-visits: 2 + on-max-visits:
// COMMAND_FIX_EXHAUSTED, the walk halts at COMMAND_FIX_EXHAUSTED after
// exactly 2 fix attempts.
//
// The assertions pin the SPECIFIC halt, not the 10000 backstop:
//   - the error references COMMAND_FIX_EXHAUSTED and NOT "exceeded … dispatches", and
//   - run-command fired exactly N+1=3 times (RUN_COMMAND runs once per lap;
//     the 3rd lap's FIX arrival is intercepted before re-dispatch).
//
// feedback_statemachine_test_loop_hazard: the cap (and, failing it, the
// 10000 backstop) bounds this fixture, so a regression fails the
// run-command-count check loudly rather than hanging the suite.
func TestExecuteCommand_FixLoopHaltsAtCap(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	runCommandCalls := 0
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "run-command":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				runCommandCalls++
				// Never converge; stamp the failure-kind the fix dispatch
				// templates ${failure-kind}-fixer / fix-${failure-kind} on.
				ctx.Set("command-succeeded", false)
				ctx.Set("failure-kind", "command-failed")
				return statemachine.Outcome{}
			}
		case "validate-outputs-and-scopes":
			// The inner fixer's execute-agent validates clean so the fix
			// subprocess terminates each lap (we're testing the OUTER cap).
			return func(ctx *statemachine.Context) statemachine.Outcome {
				ctx.Set("outputs-and-scopes-valid", true)
				return statemachine.Outcome{}
			}
		default:
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
	}
	eng.GateFn = capLoopGateFn()
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := statemachine.NewContext()
	ctx.Params["command"] = "synthetic-cmd"
	ctx.Params["category"] = "command"
	ctx.Params["task-name"] = "synthetic-task"
	err = eng.RunProcess("execute-command", ctx)
	if err == nil {
		t.Fatalf("RunProcess(execute-command) returned nil; want halt at COMMAND_FIX_EXHAUSTED")
	}
	if !strings.Contains(err.Error(), "COMMAND_FIX_EXHAUSTED") {
		t.Fatalf("RunProcess(execute-command) error = %q; want it to reference COMMAND_FIX_EXHAUSTED", err.Error())
	}
	if strings.Contains(err.Error(), "exceeded") {
		t.Errorf("RunProcess(execute-command) hit the maxDispatchesPerProcess backstop, not the max-visits cap: %q", err.Error())
	}
	if runCommandCalls != 3 {
		t.Errorf("run-command fired %d time(s), want 3 (N+1) — the cap must halt after exactly 2 fix attempts", runCommandCalls)
	}
}

// TestExecuteAgent_FixLoopHaltsAtCap mirrors the above for execute-agent
// (plan 20260530-1604 Step 3). The validate stub NEVER validates, so the
// outer agent routes to FIX every lap; under the old flow FIX -> RUN_AGENT
// re-dispatches the (opus·high) writing agent unguarded under --auto. With
// max-visits: 2 + on-max-visits: AGENT_FIX_EXHAUSTED the walk halts after
// exactly 2 fix attempts.
//
// We count dispatches of the OUTER agent by name (the inner fixer dispatches
// a distinct ${failure-kind}-fixer), and assert N+1=3 — RUN_AGENT runs once
// per lap; the 3rd lap's FIX arrival is intercepted before re-dispatch.
func TestExecuteAgent_FixLoopHaltsAtCap(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	outerAgentCalls := 0
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "validate-outputs-and-scopes":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				// Never converge for the outer agent; stamp failure-kind so
				// the fix dispatch templates resolve to a distinct fixer name.
				ctx.Set("outputs-and-scopes-valid", false)
				ctx.Set("failure-kind", "agent-output-invalid")
				return statemachine.Outcome{}
			}
		default:
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		n := name
		return func(ctx *statemachine.Context) statemachine.Outcome {
			if n == "synthetic-writer" {
				outerAgentCalls++
			}
			return statemachine.Outcome{}
		}
	}
	eng.GateFn = capLoopGateFn()
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := statemachine.NewContext()
	ctx.Params["agent"] = "synthetic-writer"
	ctx.Params["task-name"] = "synthetic-task"
	ctx.Params["category"] = "prod-agent"
	err = eng.RunProcess("execute-agent", ctx)
	if err == nil {
		t.Fatalf("RunProcess(execute-agent) returned nil; want halt at AGENT_FIX_EXHAUSTED")
	}
	if !strings.Contains(err.Error(), "AGENT_FIX_EXHAUSTED") {
		t.Fatalf("RunProcess(execute-agent) error = %q; want it to reference AGENT_FIX_EXHAUSTED", err.Error())
	}
	if strings.Contains(err.Error(), "exceeded") {
		t.Errorf("RunProcess(execute-agent) hit the maxDispatchesPerProcess backstop, not the max-visits cap: %q", err.Error())
	}
	if outerAgentCalls != 3 {
		t.Errorf("outer RUN_AGENT fired %d time(s), want 3 (N+1) — the cap must halt after exactly 2 fix attempts", outerAgentCalls)
	}
}

// TestExecuteCommand_FixRejectHalts proves D2/D3 (plan 20260530-1604):
// rejecting the fix's PRE approval halts the run (FIX_REJECTED_END is now an
// error-end-event) instead of looping back to re-run the failed command. The
// approval stub rejects ONLY the fix remediation prompt — keyed on
// failure-kind, which run-command stamps before the fix dispatch and which is
// absent at execute-command's own pre-approval — so the error propagates up
// through the FIX call-activity and run-command fires only once. (Keying on
// state, not a call counter, is robust: each approval point is evaluated
// twice — once by the inner approve subprocess, once by the caller's
// GATE_APPROVED_PRE.)
func TestExecuteCommand_FixRejectHalts(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	runCommandCalls := 0
	eng.ActionFn = func(name string) statemachine.NodeFn {
		switch name {
		case "run-command":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				runCommandCalls++
				ctx.Set("command-succeeded", false)
				ctx.Set("failure-kind", "command-failed")
				return statemachine.Outcome{}
			}
		default:
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
		}
	}
	eng.AgentFn = func(name string) statemachine.NodeFn {
		return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{} }
	}
	eng.GateFn = func(name string) statemachine.NodeFn {
		switch name {
		case "approval-outcome":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				// Reject only the fix remediation prompt: failure-kind is set
				// by run-command before the fix dispatch and is absent at the
				// outer command's pre-approval.
				if _, isFix := ctx.State["failure-kind"]; isFix {
					return statemachine.Outcome{Value: "rejected"}
				}
				return statemachine.Outcome{Value: "approved"}
			}
		case "fix-on-failure-enabled":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				return statemachine.Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
			}
		default:
			return capDefaultGate(name)
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := statemachine.NewContext()
	ctx.Params["command"] = "synthetic-cmd"
	ctx.Params["category"] = "command"
	ctx.Params["task-name"] = "synthetic-task"
	err = eng.RunProcess("execute-command", ctx)
	if err == nil {
		t.Fatalf("RunProcess(execute-command) returned nil; want halt at FIX_REJECTED_END")
	}
	if !strings.Contains(err.Error(), "FIX_REJECTED_END") {
		t.Fatalf("RunProcess(execute-command) error = %q; want it to reference FIX_REJECTED_END (reject must halt)", err.Error())
	}
	if runCommandCalls != 1 {
		t.Errorf("run-command fired %d time(s), want 1 — fix-reject must halt, not loop back to re-run", runCommandCalls)
	}
}

// capLoopGateFn is the shared GateFn for the two cap-halt flow tests:
// approve everything, evaluate fix-on-failure from params, and read any
// other binding from state (so run-command / validate stubs steer routing).
func capLoopGateFn() func(string) statemachine.NodeFn {
	return func(name string) statemachine.NodeFn {
		switch name {
		case "approval-outcome":
			return func(ctx *statemachine.Context) statemachine.Outcome { return statemachine.Outcome{Value: "approved"} }
		case "fix-on-failure-enabled":
			return func(ctx *statemachine.Context) statemachine.Outcome {
				return statemachine.Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
			}
		default:
			return capDefaultGate(name)
		}
	}
}

// capDefaultGate reads a binding's value from state, mirroring the default
// gate in TestVerifyTests_FixLoopHaltsAtCap.
func capDefaultGate(name string) statemachine.NodeFn {
	return func(ctx *statemachine.Context) statemachine.Outcome {
		v, ok := ctx.State[name]
		if !ok {
			return statemachine.Outcome{}
		}
		switch t := v.(type) {
		case bool:
			return statemachine.Outcome{Bool: t}
		case string:
			return statemachine.Outcome{Value: t}
		default:
			return statemachine.Outcome{}
		}
	}
}
