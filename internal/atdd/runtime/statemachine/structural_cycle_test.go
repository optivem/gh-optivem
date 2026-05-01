package statemachine

import (
	"reflect"
	"testing"
)

// TestStructuralCycle_RedesignStepsExecuted walks the structural_cycle flow
// — the shared sub-flow main dispatches into for both system-api-task
// ("SYSTEM API REDESIGN") and system-ui-task ("SYSTEM UI REDESIGN") — and
// asserts the full ordered step sequence under TEST=full. A YAML edit that
// drops, reorders, or splits a step shows up here as a diff.
func TestStructuralCycle_RedesignStepsExecuted(t *testing.T) {
	wantSteps := []string{
		"STRUCT_WRITE",
		"STOP_STRUCT_REVIEW",
		"GATE_TEST_MODE",
		"COMPILE",
		"SAMPLE",
		"DRIFT",
		"STOP_STRUCT_TEST",
		"ASK_COMMIT",
		"COMMIT_STRUCT",
		"TICK",
		"STRUCT_END",
	}

	for _, phase := range []string{"SYSTEM API REDESIGN", "SYSTEM UI REDESIGN"} {
		t.Run(phase, func(t *testing.T) {
			eng := loadSnapshot(t)
			flow := eng.Flows["structural_cycle"]
			ctx := NewContext()
			ctx.Params = map[string]string{"phase": phase, "agent": "atdd-task"}
			ctx.Set("structural_test_mode", "full")

			var got []string
			for cur := flow.Start; cur != ""; {
				got = append(got, cur)
				if flow.Nodes[cur].Kind == EndEvent {
					break
				}
				next, err := eng.NextEdge("structural_cycle", cur, ctx)
				if err != nil {
					t.Fatalf("NextEdge from %q: %v", cur, err)
				}
				cur = next
			}
			if !reflect.DeepEqual(got, wantSteps) {
				t.Errorf("step order:\n got=%v\nwant=%v", got, wantSteps)
			}
		})
	}
}
