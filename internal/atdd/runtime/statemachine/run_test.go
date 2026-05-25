package statemachine

import (
	"strings"
	"testing"
)

// TestWrapCallActivity_ExpandsNestedParams locks in the two-hop substitution
// fix: a parent call_activity declares a literal value, the child call_activity
// rebinds the same key with ${parent_key}, and the innermost service_task
// reads ctx.Params and gets the *expanded* string — not the placeholder.
//
// Before the fix, merging raw.Params without ExpandParams pushed the literal
// `${change_type}` into the child scope, overwriting the parent's resolved
// value (see incident 2026-05-11 in the rehearsal of issue #61).
func TestWrapCallActivity_ExpandsNestedParams(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	const yaml = `
processes:
  outer:
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call_activity
        process: middle
        params:
          change_type: "SYSTEM INTERFACE REDESIGN"
      - id: OUTER_END
        type: end_event
    sequence_flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    start: CALL_INNER
    nodes:
      - id: CALL_INNER
        type: call_activity
        process: inner
        params:
          change_type: ${change_type}
      - id: MIDDLE_END
        type: end_event
    sequence_flows:
      - {from: CALL_INNER, to: MIDDLE_END}

  inner:
    start: READ
    nodes:
      - id: READ
        type: service_task
        action: read_change_type
      - id: INNER_END
        type: end_event
    sequence_flows:
      - {from: READ, to: INNER_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var seenChangeType string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "read_change_type" {
				seenChangeType = ctx.Params["change_type"]
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
		t.Errorf("innermost service_task saw change_type=%q, want %q (two-hop ${…} expansion regressed)", seenChangeType, want)
	}
}

// TestWrapCallActivity_PassesThroughLiteralValues is the idempotency
// counterpart: values without ${…} placeholders propagate unchanged so the
// fix doesn't accidentally mangle plain strings (e.g. the literal
// "AT - GREEN - SYSTEM" used by at_green_system's COMMIT call site).
func TestWrapCallActivity_PassesThroughLiteralValues(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_INNER
    nodes:
      - id: CALL_INNER
        type: call_activity
        process: inner
        params:
          change_type: "AT - GREEN - SYSTEM"
      - id: OUTER_END
        type: end_event
    sequence_flows:
      - {from: CALL_INNER, to: OUTER_END}

  inner:
    start: READ
    nodes:
      - id: READ
        type: service_task
        action: read_change_type
      - id: INNER_END
        type: end_event
    sequence_flows:
      - {from: READ, to: INNER_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var seenChangeType string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "read_change_type" {
				seenChangeType = ctx.Params["change_type"]
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
		t.Errorf("inner service_task saw change_type=%q, want %q (literal value should pass through)", seenChangeType, want)
	}
}

// TestServiceTask_ResolvesActionTemplate locks in the dispatch-time action
// lookup for templated `action: ${name}` fields. The cycle YAML uses this
// to let one shared sub-process pick a different concrete action per call
// site (e.g. red_phase_cycle's COMPILE node resolves to compile_system or
// compile_system_tests depending on the `compile_action` param).
func TestServiceTask_ResolvesActionTemplate(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_CYCLE
    nodes:
      - id: CALL_CYCLE
        type: call_activity
        process: cycle
        params:
          chosen: do_thing_b
      - id: OUTER_END
        type: end_event
    sequence_flows:
      - {from: CALL_CYCLE, to: OUTER_END}

  cycle:
    start: ACT
    nodes:
      - id: ACT
        type: service_task
        action: ${chosen}
      - id: CYCLE_END
        type: end_event
    sequence_flows:
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
	if called != "do_thing_b" {
		t.Errorf("service_task action template resolved to %q, want %q", called, "do_thing_b")
	}
}

// TestCallActivity_ResolvesProcessTemplate locks in dispatch-time process
// lookup for templated `process: ${name}` fields on call_activity. The
// five-level BPMN refactor (plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md
// Item 2) needs this so a HIGH orchestration like `implement-and-verify-system`
// can call a parameterised sub-process (`process: ${agent-action}`) and have
// it resolve at dispatch time to the concrete cycle the caller picked.
// Mirrors the existing ${action} / ${agent} template support on service_task /
// user_task.
//
// The structure mirrors real usage: outer is a CYCLE that calls a HIGH with
// `params: {agent_action: inner_b}`; the HIGH (`middle`) has a call_activity
// whose `process: ${agent_action}` resolves at dispatch time using the param
// the CYCLE pushed. (Templated `process:` cannot read params declared on the
// same call_activity — those haven't been pushed yet at resolution time.)
func TestCallActivity_ResolvesProcessTemplate(t *testing.T) {
	const yaml = `
processes:
  outer:
    start: CALL_MIDDLE
    nodes:
      - id: CALL_MIDDLE
        type: call_activity
        process: middle
        params:
          agent_action: inner_b
      - id: OUTER_END
        type: end_event
    sequence_flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    start: CALL_CHOSEN
    nodes:
      - id: CALL_CHOSEN
        type: call_activity
        process: ${agent_action}
      - id: MIDDLE_END
        type: end_event
    sequence_flows:
      - {from: CALL_CHOSEN, to: MIDDLE_END}

  inner_a:
    start: ACT_A
    nodes:
      - id: ACT_A
        type: service_task
        action: mark_a
      - id: A_END
        type: end_event
    sequence_flows:
      - {from: ACT_A, to: A_END}

  inner_b:
    start: ACT_B
    nodes:
      - id: ACT_B
        type: service_task
        action: mark_b
      - id: B_END
        type: end_event
    sequence_flows:
      - {from: ACT_B, to: B_END}
`
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}

	var visited string
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			if name == "mark_a" || name == "mark_b" {
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
	if visited != "mark_b" {
		t.Errorf("call_activity process template resolved to wrong sub-process; saw %q action fire, want %q (i.e. inner_b)", visited, "mark_b")
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
        type: call_activity
        process: middle
        params:
          agent_action: nonexistent
      - id: OUTER_END
        type: end_event
    sequence_flows:
      - {from: CALL_MIDDLE, to: OUTER_END}

  middle:
    start: CALL_CHOSEN
    nodes:
      - id: CALL_CHOSEN
        type: call_activity
        process: ${agent_action}
      - id: MIDDLE_END
        type: end_event
    sequence_flows:
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
	if !strings.Contains(msg, "nonexistent") || !strings.Contains(msg, "${agent_action}") {
		t.Errorf("error %q should name both the resolved value %q and the original template %q", msg, "nonexistent", "${agent_action}")
	}
}
