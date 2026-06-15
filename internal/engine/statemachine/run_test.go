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

// TestStrictExpand_MissingLayerBindingFailsFast mirrors the missing-
// message regression for the `commit` subprocess's required `layer`
// input. The layer-qualified message format
// `#${ticket-id} ${issue-title} - ${layer}` relies on the same strict-
// mode dispatch contract: a caller that forgets to bind `layer:` is a
// wiring bug, surfaced at dispatch time with a precise error that names
// the unresolved placeholder — not a silent splice of the literal
// `${layer}` into the rendered git commit message.
func TestStrictExpand_MissingLayerBindingFailsFast(t *testing.T) {
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
          layer: ${layer}
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
		t.Fatal("RunProcess succeeded; want strict-mode unresolved-placeholder error for ${layer}")
	}
	if !strings.Contains(err.Error(), "${layer}") {
		t.Errorf("error %q must name the unresolved placeholder ${layer}", err.Error())
	}
}

// TestMaxVisits_RoutesToOnMaxVisitsBeforeOverCapDispatch is the isolated
// proof of the per-node visit cap (plan 20260530-1339 Item 1). WORK
// self-loops unconditionally — under today's engine it would spin until
// maxDispatchesPerProcess fires ~10000 dispatches later. With
// `max-visits: 2` + `on-max-visits: GAVE_UP`, the engine dispatches WORK
// exactly twice (attempt 1, attempt 2) and on the third arrival routes to
// GAVE_UP (an error-end-event) WITHOUT executing the node body a third
// time. The assertions pin both halves of the contract:
//   - the action fires exactly N=2 times (the over-cap pass is never spent), and
//   - the run terminates via GAVE_UP, NOT via the 10000-dispatch backstop.
func TestMaxVisits_RoutesToOnMaxVisitsBeforeOverCapDispatch(t *testing.T) {
	const yaml = `
processes:
  loop:
    name: "Loop"
    start: WORK
    nodes:
      - id: WORK
        type: service-task
        action: do-work
        name: "Work"
        max-visits: 2
        on-max-visits: GAVE_UP
      - id: GAVE_UP
        type: error-end-event
        name: "Gave Up After 2 Attempts"
    sequence-flows:
      - {from: WORK, to: WORK}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	workCalls := 0
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "do-work" {
				workCalls++
			}
			return Outcome{}
		}
	}
	eng.AgentFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.GateFn = func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	err = eng.RunProcess("loop", NewContext())
	if err == nil {
		t.Fatal("RunProcess returned nil; want error-end-event halt from GAVE_UP")
	}
	if !strings.Contains(err.Error(), "GAVE_UP") {
		t.Errorf("halt error = %q; want it to reference GAVE_UP (the on-max-visits target)", err.Error())
	}
	if strings.Contains(err.Error(), "exceeded") {
		t.Errorf("halt error = %q; the run hit the maxDispatchesPerProcess backstop instead of the max-visits cap", err.Error())
	}
	if workCalls != 2 {
		t.Errorf("do-work fired %d time(s), want 2 — the cap must route BEFORE the (N+1)th node-body dispatch", workCalls)
	}
}

// TestMaxVisits_LoadValidation pins the buildProcess guards on the paired
// max-visits / on-max-visits fields: they are declared together and the
// target must exist in the same process. Each case is a wiring bug that
// must fail fast at load time, not surface as a dangling route mid-run.
func TestMaxVisits_LoadValidation(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "max-visits without on-max-visits",
			wantErr: "without an on-max-visits target",
			yaml: `
processes:
  p:
    name: "P"
    start: A
    nodes:
      - id: A
        type: service-task
        action: noop
        name: "A"
        max-visits: 2
    sequence-flows:
      - {from: A, to: A}
`,
		},
		{
			name:    "on-max-visits without max-visits",
			wantErr: "without a positive max-visits",
			yaml: `
processes:
  p:
    name: "P"
    start: A
    nodes:
      - id: A
        type: service-task
        action: noop
        name: "A"
        on-max-visits: B
      - id: B
        type: error-end-event
        name: "B"
    sequence-flows:
      - {from: A, to: A}
`,
		},
		{
			name:    "on-max-visits targets unknown node",
			wantErr: "references unknown node",
			yaml: `
processes:
  p:
    name: "P"
    start: A
    nodes:
      - id: A
        type: service-task
        action: noop
        name: "A"
        max-visits: 2
        on-max-visits: NOWHERE
    sequence-flows:
      - {from: A, to: A}
`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadBytes([]byte(c.yaml))
			if err == nil {
				t.Fatalf("LoadBytes succeeded; want error containing %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %q; want it to contain %q", err.Error(), c.wantErr)
			}
		})
	}
}
