package statemachine

import (
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
