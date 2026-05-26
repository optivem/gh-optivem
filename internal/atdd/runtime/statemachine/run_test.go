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
