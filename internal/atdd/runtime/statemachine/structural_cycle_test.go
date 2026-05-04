package statemachine

import (
	"reflect"
	"testing"
)

// TestImplementTicket_SystemUiRedesign mirrors the ATDD-shaped scenario the
// driver runs for `gh optivem atdd implement-ticket --issue N` against a
// system-ui-task ticket. Agents (claude shell-outs) and github-touching
// service tasks are mocked out; the runner walks `main` from the
// implement-ticket entry point (MOVE_TO_IN_PROGRESS); a spy collects the
// ordered list of work-doing nodes it visited.
func TestImplementTicket_SystemUiRedesign(t *testing.T) {
	// ── ARRANGE ─────────────────────────────────────────────────────────
	eng := loadSnapshot(t)

	noop := func(*Context) Outcome { return Outcome{} }
	eng.AgentFn = func(string) NodeFn { return noop }  // mocks claude dispatch
	eng.ActionFn = func(string) NodeFn { return noop } // mocks gh / git side effects
	eng.GateFn = func(name string) NodeFn {            // gates echo pre-seeded routing state
		return func(ctx *Context) Outcome {
			switch v := ctx.Get(name).(type) {
			case string:
				return Outcome{Value: v}
			case bool:
				return Outcome{Bool: v}
			}
			return Outcome{}
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	// Spy: every service_task / user_task records its node ID before
	// firing the (mocked) inner function. Gateways, call_activities, and
	// start/end events are routing scaffolding, not "steps the runner
	// executes", so they're excluded.
	var history []string
	for _, flow := range eng.Flows {
		for id, node := range flow.Nodes {
			if node.Kind != ServiceTask && node.Kind != UserTask {
				continue
			}
			id, inner := id, node.Fn
			node.Fn = func(ctx *Context) Outcome {
				history = append(history, id)
				return inner(ctx)
			}
			flow.Nodes[id] = node
		}
	}

	// implement-ticket mode skips the picker (driver.Run mutates Start).
	// Pre-seed the routing state preResolveIssue + CLASSIFY would set,
	// plus the structural_cycle TEST gate choice.
	eng.Flows["main"].Start = "MOVE_TO_IN_PROGRESS"
	ctx := NewContext()
	ctx.Set("ticket_type", "system-ui-task")
	ctx.Set("classify_confident", true)
	ctx.Set("legacy_acceptance_criteria_section_present", false)
	ctx.Set("structural_test_mode", "full")

	// ── ACT ─────────────────────────────────────────────────────────────
	if err := eng.RunFlow("main", ctx); err != nil {
		t.Fatalf("RunFlow main: %v", err)
	}

	// ── ASSERT ──────────────────────────────────────────────────────────
	want := []string{
		"MOVE_TO_IN_PROGRESS",
		"CLASSIFY",
		"ATDD_TASK",
		"STOP_INTAKE",
		"STRUCT_WRITE",
		"STOP_STRUCT_REVIEW",
		"COMPILE",
		"SAMPLE",
		"DRIFT",
		"STOP_STRUCT_TEST",
		"ASK_COMMIT",
		"COMMIT_STRUCT",
		"TICK",
		"TICKET_IN_ACCEPTANCE",
	}
	if !reflect.DeepEqual(history, want) {
		t.Errorf("step history:\n got=%v\nwant=%v", history, want)
	}
}
