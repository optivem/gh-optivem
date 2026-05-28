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
    name: "Outer"
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call-activity
        process: middle
        name: "Synthetic Test Call"
        params:
          change-type: "SYSTEM INTERFACE REDESIGN"
      - id: OUTER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    name: "Middle"
    start: CALL_INNER
    nodes:
      - id: CALL_INNER
        type: call-activity
        process: inner
        name: "Synthetic Test Call"
        params:
          change-type: ${change-type}
      - id: MIDDLE_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_INNER, to: MIDDLE_END}

  inner:
    name: "Inner"
    start: READ
    nodes:
      - id: READ
        type: service-task
        action: read-change-type
        name: "Read"
      - id: INNER_END
        type: end-event
        name: "Synthetic Test Event"
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
    name: "Outer"
    start: CALL_INNER
    nodes:
      - id: CALL_INNER
        type: call-activity
        process: inner
        name: "Synthetic Test Call"
        params:
          change-type: "AT - GREEN - SYSTEM"
      - id: OUTER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_INNER, to: OUTER_END}

  inner:
    name: "Inner"
    start: READ
    nodes:
      - id: READ
        type: service-task
        action: read-change-type
        name: "Read"
      - id: INNER_END
        type: end-event
        name: "Synthetic Test Event"
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
    name: "Outer"
    start: CALL_CYCLE
    nodes:
      - id: CALL_CYCLE
        type: call-activity
        process: cycle
        name: "Synthetic Test Call"
        params:
          chosen: do-thing-b
      - id: OUTER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_CYCLE, to: OUTER_END}

  cycle:
    name: "Cycle"
    start: ACT
    nodes:
      - id: ACT
        type: service-task
        action: ${chosen}
        name: "Act"
      - id: CYCLE_END
        type: end-event
        name: "Synthetic Test Event"
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
// can call a parameterised sub-process (`process: ${action}`) and have
// it resolve at dispatch time to the concrete cycle the caller picked.
// Mirrors the existing ${action} / ${agent} template support on service-task /
// user-task.
//
// The structure mirrors real usage: outer is a CYCLE that calls a HIGH with
// `params: {action: inner-b}`; the HIGH (`middle`) has a call-activity
// whose `process: ${action}` resolves at dispatch time using the param
// the CYCLE pushed. (Templated `process:` cannot read params declared on the
// same call-activity — those haven't been pushed yet at resolution time.)
func TestCallActivity_ResolvesProcessTemplate(t *testing.T) {
	const yaml = `
processes:
  outer:
    name: "Outer"
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call-activity
        process: middle
        name: "Synthetic Test Call"
        params:
          action: inner-b
      - id: OUTER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    name: "Middle"
    start: CALL_CHOSEN
    nodes:
      - id: CALL_CHOSEN
        type: call-activity
        process: ${action}
        name: "Synthetic Test Call"
      - id: MIDDLE_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_CHOSEN, to: MIDDLE_END}

  inner-a:
    name: "Inner A"
    start: ACT_A
    nodes:
      - id: ACT_A
        type: service-task
        action: mark-a
        name: "Act A"
      - id: A_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: ACT_A, to: A_END}

  inner-b:
    name: "Inner B"
    start: ACT_B
    nodes:
      - id: ACT_B
        type: service-task
        action: mark-b
        name: "Act B"
      - id: B_END
        type: end-event
        name: "Synthetic Test Event"
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
	got, err := ExpandParams("agent=${agent}", params, state)
	if err != nil {
		t.Fatalf("ExpandParams: %v", err)
	}
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
	got, err := ExpandParams(`fix-${failure-kind}`, params, state)
	if err != nil {
		t.Fatalf("ExpandParams: %v", err)
	}
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
		{name: "[]string single", value: []string{"foo"}, want: "foo"},
		{name: "[]string multi", value: []string{"foo", "bar"}, want: "foo,bar"},
		{name: "[]string empty", value: []string{}, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ExpandParams("[${k}]", nil, map[string]any{"k": c.value})
			if err != nil {
				t.Fatalf("ExpandParams: %v", err)
			}
			want := "[" + c.want + "]"
			if got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

// TestExpandParams_UnresolvedPlaceholderErrors is the strict-mode
// contract: a `${name}` reference with no matching key in params or state
// is an error, not a silent literal leak. Before the strict flip the
// runtime would render `${ghost}` verbatim into downstream command lines
// (root cause of the `--suite='${suite}'` CLI failure observed
// 2026-05-27); the strict check surfaces it as a dispatch-time error
// naming the offending key.
func TestExpandParams_UnresolvedPlaceholderErrors(t *testing.T) {
	_, err := ExpandParams("${agent}-${ghost}", map[string]string{"agent": "writer"}, nil)
	if err == nil {
		t.Fatal("ExpandParams returned nil error — expected unresolved-placeholder error for ${ghost}")
	}
	if !strings.Contains(err.Error(), "${ghost}") {
		t.Errorf("error %q does not mention the unresolved placeholder ${ghost}", err.Error())
	}
}

// TestExpandParams_EmptyValueIsValid locks in the boundary: an empty
// binding is a valid resolution, NOT an unresolved-placeholder error.
// Callers like impl-and-verify-system bind `suite: ""` to mean "all
// suites"; strict mode must not reject the explicit empty.
func TestExpandParams_EmptyValueIsValid(t *testing.T) {
	got, err := ExpandParams("${foo}", map[string]string{"foo": ""}, nil)
	if err != nil {
		t.Fatalf("ExpandParams: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want %q — empty binding must resolve to empty string", got, "")
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
    name: "Outer"
    start: WRITE_STATE
    nodes:
      - id: WRITE_STATE
        type: service-task
        action: write-failure-kind
        name: "Write State"
      - id: FIX
        type: call-activity
        process: fix
        name: "Synthetic Test Call"
        params:
          task-name: "fix-${failure-kind}"
      - id: OUTER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: WRITE_STATE, to: FIX}
      - {from: FIX,         to: OUTER_END}

  fix:
    name: "Fix"
    start: READ
    nodes:
      - id: READ
        type: service-task
        action: read-task-name
        name: "Read"
      - id: FIX_END
        type: end-event
        name: "Synthetic Test Event"
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
    name: "Outer"
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call-activity
        process: middle
        name: "Synthetic Test Call"
        params:
          action: nonexistent
      - id: OUTER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    name: "Middle"
    start: CALL_CHOSEN
    nodes:
      - id: CALL_CHOSEN
        type: call-activity
        process: ${action}
        name: "Synthetic Test Call"
      - id: MIDDLE_END
        type: end-event
        name: "Synthetic Test Event"
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
	if !strings.Contains(msg, "nonexistent") || !strings.Contains(msg, "${action}") {
		t.Errorf("error %q should name both the resolved value %q and the original template %q", msg, "nonexistent", "${action}")
	}
}

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
// in turn was expanded with `${failure-kind}` resolved via ExpandParams's
// state fallback. The recorded AgentFn dispatch must be the literal
// `"command-failed-fixer"` — if Items 1, 2, the YAML wiring, or the
// 1701 task-name/agent split regresses, this test fails before runtime
// sees a missing-prompt error.
//
// Memory `feedback_statemachine_test_loop_hazard`: the four processes
// walked here (execute-command, fix, execute-agent, approve) have no
// loopback edges, so the test is bounded. `maxDispatchesPerProcess`
// catches the failure mode anyway if a future YAML edit introduces one.
func TestExecuteCommand_FailureDispatchesCommandFailedFixerAgent(t *testing.T) {
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
	// task-name mirrors the production scope: execute-command is always
	// called from inside a writing-agent phase whose outer call-activity
	// binds task-name. fix's EXECUTE_AGENT reads it via
	// `originating-task-name: ${task-name}` so the fix-agent inherits the
	// outer phase's scope identity. Under strict-mode ExpandParams the
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
		// ExpandParams loses the state-fallback path.
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
// Memory `feedback_statemachine_test_loop_hazard`: as in Item 3, the
// walked processes (execute-agent, fix, execute-agent inner, approve)
// have no loopback edges. The inner execute-agent does re-enter
// validate-outputs-and-scopes, but `fix-on-failure=false` on the inner
// call-site routes its GATE_FIX_ON_FAILURE to APPROVE_POST — no
// second-level recursion.
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
			eng, err := LoadDefault()
			if err != nil {
				t.Fatalf("LoadDefault: %v", err)
			}
			eng.ActionFn = func(name string) NodeFn {
				switch name {
				case "run-command":
					return func(ctx *Context) Outcome {
						ctx.Set("command-succeeded", false)
						ctx.Set("test-outcome", "infra")
						ctx.Set("test-infra-label", "missing executable")
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
				return func(ctx *Context) Outcome { return Outcome{} }
			}
			eng.GateFn = nil // use real gates via Bind so testOutcome routes infra
			if err := eng.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}
			ctx := NewContext()
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

// TestWrapUserTask_AgentResolvesFromAgentParam pins the post-split wiring
// (plan 20260526-1701): RUN_AGENT in the `execute-agent` sub-process now
// templates `agent: ${agent}`, NOT `agent: ${task-name}`. Callers pass two
// distinct fields — `task-name` (the verb, used for scope lookup and
// trace) and `agent` (the noun, used for dispatcher routing). A future
// refactor that silently reverts RUN_AGENT to `${task-name}` would still
// resolve to *some* registered agent (the task verbs are also valid
// strings), so a dedicated test is the only way to catch the regression.
//
// Pattern mirrors TestExecuteCommand_FailureDispatchesFixCommandFailedAgent:
// build a small in-memory process with one MID node that passes both
// fields into `execute-agent`; record the dispatched agent name via a
// recording AgentFn; assert the recorded name is the noun, not the verb.
func TestWrapUserTask_AgentResolvesFromAgentParam(t *testing.T) {
	const yaml = `
processes:
  outer:
    name: "Outer"
    start: MID
    nodes:
      - id: MID
        type: call-activity
        process: execute-agent
        name: "Synthetic MID"
        params:
          task-name: write-acceptance-tests
          agent: acceptance-test-writer
          category: test-agent
      - id: OUTER_END
        type: end-event
        name: "Outer end"
    sequence-flows:
      - {from: MID, to: OUTER_END}

  execute-agent:
    name: "Execute Agent"
    start: RUN_AGENT
    nodes:
      - id: RUN_AGENT
        type: user-task
        agent: ${agent}
        name: "Run agent ${agent} (task: ${task-name})"
      - id: EXECUTE_AGENT_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: RUN_AGENT, to: EXECUTE_AGENT_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var dispatched string
	eng.ActionFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.AgentFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			dispatched = name
			return Outcome{}
		}
	}
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := eng.RunProcess("outer", NewContext()); err != nil {
		t.Fatalf("RunProcess outer: %v", err)
	}

	if dispatched != "acceptance-test-writer" {
		t.Errorf("RUN_AGENT dispatched %q, want %q — the `agent:` param (noun) must drive dispatch, NOT `task-name:` (verb)", dispatched, "acceptance-test-writer")
	}
	if dispatched == "write-acceptance-tests" {
		t.Errorf("RUN_AGENT resolved to the verb (task-name) instead of the noun (agent) — the 1701 split has regressed; RUN_AGENT.agent must template ${agent}, not ${task-name}")
	}
}

// TestStrictExpand_EmptySuiteBindingDoesNotLeak is the integration-style
// regression for the production trace observed 2026-05-27 ~01:55 CEDT:
// `implement-and-verify-system` dispatched `verify-tests-pass` without
// binding `suite`, the literal `${suite}` propagated through the
// call-activity push chain into runCommand, and the rendered CLI became
// `gh optivem test run --suite='${suite}'` — which the CLI rejected.
//
// The fix is twofold: (a) callers MUST bind `suite: ""` explicitly to
// mean "all suites" (bd1c958); (b) strict-mode `ExpandParams` rejects an
// unresolved `${suite}` at dispatch instead of letting the literal leak.
// This test exercises (a)+(b) together: parent binds `suite: ""`, the
// child forwards `${suite}`, and the leaf service-task sees
// ctx.Params["suite"] == "" — exactly the contract runCommand needs to
// skip the --suite=… flag.
func TestStrictExpand_EmptySuiteBindingDoesNotLeak(t *testing.T) {
	const yaml = `
processes:
  parent:
    name: "Parent"
    start: VERIFY
    nodes:
      - id: VERIFY
        type: call-activity
        process: verify
        name: "Synthetic Verify"
        params:
          suite: ""
      - id: PARENT_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: VERIFY, to: PARENT_END}

  verify:
    name: "Verify"
    start: RUN
    nodes:
      - id: RUN
        type: call-activity
        process: run-cmd
        name: "Synthetic Run"
        params:
          suite: ${suite}
      - id: VERIFY_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: RUN, to: VERIFY_END}

  run-cmd:
    name: "Run Command"
    start: READ_SUITE
    nodes:
      - id: READ_SUITE
        type: service-task
        action: read-suite
        name: "Read suite param"
      - id: RUN_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: READ_SUITE, to: RUN_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var seenSuite string
	suiteWasPresent := false
	eng.ActionFn = func(name string) NodeFn {
		if name == "read-suite" {
			return func(ctx *Context) Outcome {
				seenSuite, suiteWasPresent = ctx.Params["suite"], true
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
	if err := eng.RunProcess("parent", NewContext()); err != nil {
		t.Fatalf("RunProcess parent: %v", err)
	}
	if !suiteWasPresent {
		t.Fatal("read-suite action did not fire — call chain regressed")
	}
	if seenSuite != "" {
		t.Errorf("leaf action saw suite=%q, want %q — empty caller binding must propagate as empty, not leak ${suite}", seenSuite, "")
	}
}

// TestStrictExpand_MissingSuiteBindingFailsFast is the negative
// counterpart: when the parent omits `suite:` entirely and no state
// fallback exists, strict-mode `ExpandParams` MUST error at the
// call-activity param push — not let the literal `${suite}` propagate
// into the inner scope. The error message must name the placeholder so
// the operator can find the unbound site in process-flow.yaml.
func TestStrictExpand_MissingSuiteBindingFailsFast(t *testing.T) {
	const yaml = `
processes:
  parent:
    name: "Parent"
    start: VERIFY
    nodes:
      - id: VERIFY
        type: call-activity
        process: verify
        name: "Synthetic Verify"
      - id: PARENT_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: VERIFY, to: PARENT_END}

  verify:
    name: "Verify"
    start: RUN
    nodes:
      - id: RUN
        type: call-activity
        process: run-cmd
        name: "Synthetic Run"
        params:
          suite: ${suite}
      - id: VERIFY_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: RUN, to: VERIFY_END}

  run-cmd:
    name: "Run Command"
    start: NOOP
    nodes:
      - id: NOOP
        type: service-task
        action: noop
        name: "No-op"
      - id: RUN_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: NOOP, to: RUN_END}
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
	err = eng.RunProcess("parent", NewContext())
	if err == nil {
		t.Fatal("RunProcess succeeded; want strict-mode unresolved-placeholder error for ${suite}")
	}
	if !strings.Contains(err.Error(), "${suite}") {
		t.Errorf("error %q must name the unresolved placeholder ${suite}", err.Error())
	}
}

// TestStrictExpand_MissingMessageBindingFailsFast mirrors the suite-
// strict-mode regression (TestStrictExpand_MissingSuiteBindingFailsFast)
// for the `commit` subprocess's required `message` input. The fix that
// introduced `message: ${message}` on the commit subprocess body relies
// on the same strict-mode dispatch contract: a caller that forgets to
// bind `message:` is a wiring bug, surfaced at dispatch time with a
// precise error that names the unresolved placeholder — not a silent
// splice of the literal `${message}` into `gh optivem commit '${message}'`
// (which would silently miscommit). The YAML shape mirrors the
// suite-strict-mode test exactly: the unresolved placeholder lives on
// the inner call-activity's `params:` push, since that is the layer
// strict-mode `ExpandParams` runs at.
func TestStrictExpand_MissingMessageBindingFailsFast(t *testing.T) {
	const yaml = `
processes:
  parent:
    name: "Parent"
    start: COMMIT
    nodes:
      - id: COMMIT
        type: call-activity
        process: commit-sub
        name: "Synthetic Commit"
      - id: PARENT_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: COMMIT, to: PARENT_END}

  commit-sub:
    name: "Commit Subprocess"
    start: EXEC
    nodes:
      - id: EXEC
        type: call-activity
        process: inner
        name: "Synthetic Execute"
        params:
          message: ${message}
      - id: COMMIT_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: EXEC, to: COMMIT_END}

  inner:
    name: "Inner"
    start: NOOP
    nodes:
      - id: NOOP
        type: service-task
        action: noop
        name: "No-op"
      - id: INNER_END
        type: end-event
        name: "Synthetic Test Event"
    sequence-flows:
      - {from: NOOP, to: INNER_END}
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
	err = eng.RunProcess("parent", NewContext())
	if err == nil {
		t.Fatal("RunProcess succeeded; want strict-mode unresolved-placeholder error for ${message}")
	}
	if !strings.Contains(err.Error(), "${message}") {
		t.Errorf("error %q must name the unresolved placeholder ${message}", err.Error())
	}
}
