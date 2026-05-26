package statemachine

import (
	"strings"
	"testing"
)

// TestWrapCallActivity_ExpandsNestedParams locks in the two-hop substitution
// fix: a parent call-activity declares a literal value, the child call-activity
// rebinds the same key with ${parent-key}, and the innermost service-task
// reads ctx.Params and gets the *expanded* string — not the placeholder.
//
// Before the fix, merging raw.Params without ExpandParams pushed the literal
// `${change-type}` into the child scope, overwriting the parent's resolved
// value (see incident 2026-05-11 in the rehearsal of issue #61).
func TestWrapCallActivity_ExpandsNestedParams(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	const yaml = `
processes:
  outer:
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call-activity
        process: middle
        params:
          change-type: "SYSTEM INTERFACE REDESIGN"
      - id: OUTER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    start: CALL_INNER
    nodes:
      - id: CALL_INNER
        type: call-activity
        process: inner
        params:
          change-type: ${change-type}
      - id: MIDDLE_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_INNER, to: MIDDLE_END}

  inner:
    start: READ
    nodes:
      - id: READ
        type: service-task
        action: read-change-type
      - id: INNER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: READ, to: INNER_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var seenChangeType string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "read-change-type" {
				seenChangeType = ctx.Params["change-type"]
			}
			return Outcome{}
		}
	}
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunProcess("outer", NewContext()); err != nil {
		t.Fatalf("RunProcess outer: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	want := "SYSTEM INTERFACE REDESIGN"
	if seenChangeType != want {
		t.Errorf("innermost service-task saw change-type=%q, want %q (two-hop ${…} expansion regressed)", seenChangeType, want)
	}
}

// TestWrapCallActivity_PassesThroughLiteralValues is the idempotency
// counterpart: values without ${…} placeholders propagate unchanged so the
// fix doesn't accidentally mangle plain strings (e.g. the literal
// "AT - GREEN - SYSTEM" used by at-green-system's COMMIT call site).
func TestWrapCallActivity_PassesThroughLiteralValues(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_INNER
    nodes:
      - id: CALL_INNER
        type: call-activity
        process: inner
        params:
          change-type: "AT - GREEN - SYSTEM"
      - id: OUTER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_INNER, to: OUTER_END}

  inner:
    start: READ
    nodes:
      - id: READ
        type: service-task
        action: read-change-type
      - id: INNER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: READ, to: INNER_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var seenChangeType string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "read-change-type" {
				seenChangeType = ctx.Params["change-type"]
			}
			return Outcome{}
		}
	}
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	if err := eng.RunProcess("outer", NewContext()); err != nil {
		t.Fatalf("RunProcess outer: %v", err)
	}

	want := "AT - GREEN - SYSTEM"
	if seenChangeType != want {
		t.Errorf("inner service-task saw change-type=%q, want %q (literal value should pass through)", seenChangeType, want)
	}
}

// TestServiceTask_ResolvesActionTemplate locks in the dispatch-time action
// lookup for templated `action: ${name}` fields. The cycle YAML uses this
// to let one shared sub-process pick a different concrete action per call
// site (e.g. red-phase-cycle's COMPILE node resolves to compile-system or
// compile-system-tests depending on the `compile-action` param).
func TestServiceTask_ResolvesActionTemplate(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_CYCLE
    nodes:
      - id: CALL_CYCLE
        type: call-activity
        process: cycle
        params:
          chosen: do-thing-b
      - id: OUTER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_CYCLE, to: OUTER_END}

  cycle:
    start: ACT
    nodes:
      - id: ACT
        type: service-task
        action: ${chosen}
      - id: CYCLE_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: ACT, to: CYCLE_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var called string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			called = name
			return Outcome{}
		}
	}
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := eng.RunProcess("outer", NewContext()); err != nil {
		t.Fatalf("RunProcess outer: %v", err)
	}
	if called != "do-thing-b" {
		t.Errorf("service-task action template resolved to %q, want %q", called, "do-thing-b")
	}
}

// TestCallActivity_ResolvesProcessTemplate locks in dispatch-time process
// lookup for templated `process: ${name}` fields on call-activity. The
// five-level BPMN refactor (plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md
// Item 2) needs this so a HIGH orchestration like `implement-and-verify-system`
// can call a parameterised sub-process (`process: ${agent-action}`) and have
// it resolve at dispatch time to the concrete cycle the caller picked.
// Mirrors the existing ${action} / ${agent} template support on service-task /
// user-task.
//
// The structure mirrors real usage: outer is a CYCLE that calls a HIGH with
// `params: {agent-action: inner-b}`; the HIGH (`middle`) has a call-activity
// whose `process: ${agent-action}` resolves at dispatch time using the param
// the CYCLE pushed. (Templated `process:` cannot read params declared on the
// same call-activity — those haven't been pushed yet at resolution time.)
func TestCallActivity_ResolvesProcessTemplate(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call-activity
        process: middle
        params:
          agent-action: inner-b
      - id: OUTER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    start: CALL_CHOSEN
    nodes:
      - id: CALL_CHOSEN
        type: call-activity
        process: ${agent-action}
      - id: MIDDLE_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_CHOSEN, to: MIDDLE_END}

  inner-a:
    start: ACT_A
    nodes:
      - id: ACT_A
        type: service-task
        action: mark-a
      - id: A_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: ACT_A, to: A_END}

  inner-b:
    start: ACT_B
    nodes:
      - id: ACT_B
        type: service-task
        action: mark-b
      - id: B_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: ACT_B, to: B_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var visited string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "mark-a" || name == "mark-b" {
				visited = name
			}
			return Outcome{}
		}
	}
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := eng.RunProcess("outer", NewContext()); err != nil {
		t.Fatalf("RunProcess outer: %v", err)
	}
	if visited != "mark-b" {
		t.Errorf("call-activity process template resolved to wrong sub-process; saw %q action fire, want %q (i.e. inner-b)", visited, "mark-b")
	}
}

// TestExpandParams_ParamsTakePrecedenceOverState locks in the chosen
// resolution order (plan 20260526-1530, decision (b)): when a key appears
// in both params and state, the params value wins. Call-site overrides
// must remain authoritative — the state-fallback path is the bridge for
// keys params doesn't carry, not a back-door way for state to mask
// caller-supplied values.
func TestExpandParams_ParamsTakePrecedenceOverState(t *testing.T) {
	params := map[string]string{"agent": "from-params"}
	state := map[string]any{"agent": "from-state"}
	got := ExpandParams("agent=${agent}", params, state)
	if got != "agent=from-params" {
		t.Errorf("got %q, want %q — params must win on collision", got, "agent=from-params")
	}
}

// TestExpandParams_StateFallbackForUnknownParamKey is the load-bearing
// half of the failure-kind wiring: `failure-kind` is set in state by
// runCommand and validateOutputsAndScopes, NEVER passed as a call-site
// param, so the `fix` body's `task-name: "fix-${failure-kind}"` resolves
// only because ExpandParams consults state when params is silent.
func TestExpandParams_StateFallbackForUnknownParamKey(t *testing.T) {
	params := map[string]string{"other-key": "irrelevant"}
	state := map[string]any{"failure-kind": "command-failed"}
	got := ExpandParams(`fix-${failure-kind}`, params, state)
	if got != "fix-command-failed" {
		t.Errorf("got %q, want %q — state fallback should resolve ${failure-kind}", got, "fix-command-failed")
	}
}

// TestExpandParams_StateValueCoercion mirrors Context.GetString's
// best-effort rules so the substitution layer and the predicate-evaluation
// layer agree on stringification (bool → "true"/"false", int → fmt.Sprint).
func TestExpandParams_StateValueCoercion(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: "literal", want: "literal"},
		{name: "bool true", value: true, want: "true"},
		{name: "bool false", value: false, want: "false"},
		{name: "int", value: 42, want: "42"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExpandParams("[${k}]", nil, map[string]any{"k": c.value})
			want := "[" + c.want + "]"
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestExpandParams_NilStateBehavesLikeOldSignature is the regression
// insurance: every caller that passes nil for state must get pre-fallback
// semantics (only params substitute; ${unknown} stays literal). Tests and
// the clauderun prompt renderer rely on this.
func TestExpandParams_NilStateBehavesLikeOldSignature(t *testing.T) {
	got := ExpandParams("${agent}-${ghost}", map[string]string{"agent": "writer"}, nil)
	if got != "writer-${ghost}" {
		t.Errorf("got %q, want %q — nil state must not resolve unknown keys", got, "writer-${ghost}")
	}
}

// TestWrapCallActivity_StateValueFlowsIntoChildTemplate is the integration
// check for the failure-kind wiring. A parent service-task writes
// failure-kind into state; the child call-activity's templated param
// `task-name: "fix-${failure-kind}"` resolves via the state-fallback path
// (no `failure-kind` is declared in params). This is the path the LOW
// `execute-command` → `fix` → `execute-agent` recovery chain depends on.
func TestWrapCallActivity_StateValueFlowsIntoChildTemplate(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: WRITE_STATE
    nodes:
      - id: WRITE_STATE
        type: service-task
        action: write-failure-kind
      - id: CALL_FIX
        type: call-activity
        process: fix
        params:
          task-name: "fix-${failure-kind}"
      - id: OUTER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: WRITE_STATE, to: CALL_FIX}
      - {from: CALL_FIX,    to: OUTER_END}

  fix:
    start: READ
    nodes:
      - id: READ
        type: service-task
        action: read-task-name
      - id: FIX_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: READ, to: FIX_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var seenTaskName string
	eng.ActionFn = func(name string) NodeFn {
		switch name {
		case "write-failure-kind":
			return func(ctx *Context) Outcome {
				ctx.Set("failure-kind", "command-failed")
				return Outcome{}
			}
		case "read-task-name":
			return func(ctx *Context) Outcome {
				seenTaskName = ctx.Params["task-name"]
				return Outcome{}
			}
		}
		return nil
	}
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := eng.RunProcess("outer", NewContext()); err != nil {
		t.Fatalf("RunProcess outer: %v", err)
	}
	want := "fix-command-failed"
	if seenTaskName != want {
		t.Errorf("inner action saw task-name=%q, want %q (state-fallback in ExpandParams regressed)", seenTaskName, want)
	}
}

// TestCallActivity_ProcessTemplate_UnknownTarget locks in the error path:
// when a templated `process: ${name}` expands to a sub-process the engine
// doesn't know about, the dispatch-time error names both the resolved value
// and the original template so the YAML author can trace it back.
func TestCallActivity_ProcessTemplate_UnknownTarget(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call-activity
        process: middle
        params:
          agent-action: nonexistent
      - id: OUTER_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    start: CALL_CHOSEN
    nodes:
      - id: CALL_CHOSEN
        type: call-activity
        process: ${agent-action}
      - id: MIDDLE_END
        type: end-event
        documentation: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_CHOSEN, to: MIDDLE_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	eng.ActionFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	err = eng.RunProcess("outer", NewContext())
	if err == nil {
		t.Fatalf("RunProcess succeeded; want unknown-process error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nonexistent") || !strings.Contains(msg, "${agent-action}") {
		t.Errorf("error %q should name both the resolved value %q and the original template %q", msg, "nonexistent", "${agent-action}")
	}
}

// TestExecuteCommand_FailureDispatchesFixCommandFailedAgent is the
// end-to-end verification for the recovery-path wiring (plan
// 20260526-1530 Item 3). It loads the canonical embedded YAML and drives
// `execute-command` with a stubbed `run-command` action that simulates a
// shell failure (stamps `command-succeeded=false`, `failure-kind=command-failed`).
// The expected trail is:
//
//	execute-command.RUN_COMMAND → GATE_COMMAND_SUCCEEDED=false → CALL_FIX
//	  → fix.EXECUTE_AGENT (params task-name="fix-${failure-kind}")
//	    → execute-agent.RUN_AGENT (agent: ${task-name})
//
// At RUN_AGENT the engine resolves `${task-name}` against ctx.Params,
// which in turn was expanded with `${failure-kind}` resolved via
// ExpandParams's state fallback (Item 2). The recorded AgentFn dispatch
// must be the literal `"fix-command-failed"` — if either Items 1, 2, or
// the YAML wiring regresses, this test fails before runtime sees a
// missing-prompt error.
//
// Memory `feedback_statemachine_test_loop_hazard`: the four processes
// walked here (execute-command, fix, execute-agent, approve) have no
// loopback edges, so the test is bounded. `maxDispatchesPerProcess`
// catches the failure mode anyway if a future YAML edit introduces one.
func TestExecuteCommand_FailureDispatchesFixCommandFailedAgent(t *testing.T) {
	eng, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	var dispatchedAgents []string
	eng.ActionFn = func(name string) NodeFn {
		switch name {
		case "run-command":
			return func(ctx *Context) Outcome {
				ctx.Set("command-succeeded", false)
				ctx.Set("failure-kind", "command-failed")
				return Outcome{}
			}
		case "validate-outputs-and-scopes":
			return func(ctx *Context) Outcome {
				ctx.Set("outputs-and-scopes-valid", true)
				return Outcome{}
			}
		default:
			return func(ctx *Context) Outcome { return Outcome{} }
		}
	}
	eng.AgentFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			dispatchedAgents = append(dispatchedAgents, name)
			return Outcome{}
		}
	}
	eng.GateFn = func(name string) NodeFn {
		switch name {
		case "approval-outcome":
			return func(ctx *Context) Outcome { return Outcome{Value: "approved"} }
		case "fix-on-failure-enabled":
			// Mirrors gates.bindings.fixOnFailureEnabled at the test
			// stub level: default true, "false" param disables. The
			// inner execute-agent (called from fix) sets this to
			// "false" so the test doesn't recurse into a second fix.
			return func(ctx *Context) Outcome {
				return Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
			}
		default:
			// Generic state-reading gate: read whatever the action
			// stamped under the binding name. Covers
			// command-succeeded and outputs-and-scopes-valid in this
			// walk without per-gate stubs.
			return func(ctx *Context) Outcome {
				v, ok := ctx.State[name]
				if !ok {
					return Outcome{}
				}
				switch t := v.(type) {
				case bool:
					return Outcome{Bool: t}
				case string:
					return Outcome{Value: t}
				default:
					return Outcome{}
				}
			}
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx := NewContext()
	ctx.Params["command"] = "noisy-broken-cmd"
	if err := eng.RunProcess("execute-command", ctx); err != nil {
		t.Fatalf("RunProcess execute-command: %v", err)
	}

	// The approve sub-process dispatches the human user-task multiple
	// times during the walk — we ignore those and look for the
	// fix-command-failed dispatch.
	foundFixCommandFailed := false
	for _, name := range dispatchedAgents {
		if name == "fix-command-failed" {
			foundFixCommandFailed = true
		}
		// Guard against the original bug surface: an unresolved
		// template leaking into AgentFn. Catches a regression where
		// ExpandParams loses the state-fallback path.
		if strings.Contains(name, "${") {
			t.Errorf("agent name with unresolved template leaked into dispatch: %q (full dispatch trail: %v)", name, dispatchedAgents)
		}
	}
	if !foundFixCommandFailed {
		t.Errorf("RUN_AGENT did not dispatch fix-command-failed; full dispatch trail: %v", dispatchedAgents)
	}
}

// TestExecuteAgent_ValidationFailureDispatchesFixForFailureKind is the
// twin of TestExecuteCommand_FailureDispatchesFixCommandFailedAgent for
// the `execute-agent` → `fix` → `execute-agent` recovery branch (plan
// 20260526-1530 Item 4). `validateOutputsAndScopes` writes one of two
// failure-kinds — `missing-output` or `scope-diff` — and the recovery
// path must dispatch the matching `fix-<kind>` agent.
//
// At the time this test was written the two `fix-missing-output` and
// `fix-scope-diff` prompts did NOT yet exist (out of scope here; see
// the sibling follow-up plan
// plans/upcoming/fix-missing-output-and-scope-diff-prompts.md). The
// test uses a recording AgentFn so the dispatch landing on the right
// NAME is verified independently of prompt availability — the missing
// prompts only matter when the real agents.Lookup is in play, not in
// this synthetic registry.
//
// Memory `feedback_statemachine_test_loop_hazard`: as in Item 3, the
// walked processes (execute-agent, fix, execute-agent inner, approve)
// have no loopback edges. The inner execute-agent does re-enter
// validate-outputs-and-scopes, but `fix-on-failure=false` on the inner
// call-site routes its GATE_FIX_ON_FAILURE to APPROVE_POST — no
// second-level recursion.
func TestExecuteAgent_ValidationFailureDispatchesFixForFailureKind(t *testing.T) {
	cases := []struct {
		failureKind string
		wantAgent   string
	}{
		// validateOutputsAndScopes priority is missing-output wins
		// over scope-diff (bindings.go), so each case here pins the
		// observable kind after the action's own routing decision.
		{failureKind: "missing-output", wantAgent: "fix-missing-output"},
		{failureKind: "scope-diff", wantAgent: "fix-scope-diff"},
	}
	for _, tc := range cases {
		t.Run(tc.failureKind, func(t *testing.T) {
			eng, err := LoadDefault()
			if err != nil {
				t.Fatalf("LoadDefault: %v", err)
			}

			var dispatchedAgents []string
			// Limits the validate stub to writing failure-kind on the
			// FIRST call; the inner fix's execute-agent would otherwise
			// rewrite it (harmless here, but the trail stays simpler).
			validateCalls := 0
			eng.ActionFn = func(name string) NodeFn {
				switch name {
				case "validate-outputs-and-scopes":
					return func(ctx *Context) Outcome {
						validateCalls++
						if validateCalls == 1 {
							ctx.Set("outputs-and-scopes-valid", false)
							ctx.Set("failure-kind", tc.failureKind)
						} else {
							// Inner fix's execute-agent: pass validation
							// so the walk terminates cleanly.
							ctx.Set("outputs-and-scopes-valid", true)
						}
						return Outcome{}
					}
				default:
					return func(ctx *Context) Outcome { return Outcome{} }
				}
			}
			eng.AgentFn = func(name string) NodeFn {
				return func(ctx *Context) Outcome {
					dispatchedAgents = append(dispatchedAgents, name)
					return Outcome{}
				}
			}
			eng.GateFn = func(name string) NodeFn {
				switch name {
				case "approval-outcome":
					return func(ctx *Context) Outcome { return Outcome{Value: "approved"} }
				case "fix-on-failure-enabled":
					return func(ctx *Context) Outcome {
						return Outcome{Bool: ctx.Params["fix-on-failure"] != "false"}
					}
				default:
					return func(ctx *Context) Outcome {
						v, ok := ctx.State[name]
						if !ok {
							return Outcome{}
						}
						switch t := v.(type) {
						case bool:
							return Outcome{Bool: t}
						case string:
							return Outcome{Value: t}
						default:
							return Outcome{}
						}
					}
				}
			}
			if err := eng.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}

			ctx := NewContext()
			// Outer execute-agent call: a hypothetical writing-agent
			// dispatch. The name doesn't matter — validate-outputs-and-
			// scopes is the action that decides the recovery branch.
			ctx.Params["task-name"] = "some-writing-agent"
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
