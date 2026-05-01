// Step-sequence tests for the structural cycle — the shared sub-flow that
// system-api-task ("SYSTEM API REDESIGN") and system-ui-task ("SYSTEM UI
// REDESIGN") tickets both dispatch into via call_activity in main.
//
// Where transitions_test.go locks per-edge routing one decision at a time,
// this test walks the structural_cycle flow end-to-end under each redesign's
// dispatch params, asserting the full ordered list of nodes the runtime
// visits on the happy path (TEST=full). A YAML edit that drops, reorders,
// or splits a step in the shared cycle surfaces here as a diff against the
// canonical path.

package statemachine

import (
	"reflect"
	"testing"
)

func TestStructuralCycle_RedesignStepsExecuted(t *testing.T) {
	// Happy path: TEST=full visits every step. The skip / compile branches
	// are already covered by per-edge cases in transitions_test.go.
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

	cases := []struct {
		name       string
		mainNode   string
		wantParams map[string]string
	}{
		{
			name:     "system-api-redesign",
			mainNode: "SYSAPI_CYCLE",
			wantParams: map[string]string{
				"phase":     "SYSTEM API REDESIGN",
				"agent":     "atdd-task",
				"phase_doc": "docs/atdd/process/sysapi-redesign.md",
				"subtype":   "system-api-redesign",
			},
		},
		{
			name:     "system-ui-redesign",
			mainNode: "SYSUI_CYCLE",
			wantParams: map[string]string{
				"phase":     "SYSTEM UI REDESIGN",
				"agent":     "atdd-task",
				"phase_doc": "docs/atdd/process/sysui-redesign.md",
				"subtype":   "system-ui-redesign",
			},
		},
	}

	eng := loadSnapshot(t)
	main, ok := eng.Flows["main"]
	if !ok {
		t.Fatalf("main flow missing from loaded snapshot")
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Pin the dispatch wiring: main routes the redesign through
			// structural_cycle with the documented params. ${agent} and
			// ${phase_doc} substitution at dispatch time depends on these.
			caller, ok := main.Nodes[tc.mainNode]
			if !ok {
				t.Fatalf("main flow missing node %q", tc.mainNode)
			}
			if caller.Kind != CallActivity {
				t.Errorf("%s: kind got %v, want CallActivity", tc.mainNode, caller.Kind)
			}
			if caller.Raw.Flow != "structural_cycle" {
				t.Errorf("%s: flow got %q, want %q", tc.mainNode, caller.Raw.Flow, "structural_cycle")
			}
			if !reflect.DeepEqual(caller.Raw.Params, tc.wantParams) {
				t.Errorf("%s params:\n got=%v\nwant=%v", tc.mainNode, caller.Raw.Params, tc.wantParams)
			}

			// Walk structural_cycle with TEST=full and the redesign's params.
			gotSteps := walkStructuralCycle(t, eng, tc.wantParams, "full")
			if !reflect.DeepEqual(gotSteps, wantSteps) {
				t.Errorf("%s step order:\n got=%v\nwant=%v", tc.name, gotSteps, wantSteps)
			}
		})
	}
}

// walkStructuralCycle traces the node IDs the runtime would visit when
// running the structural_cycle flow under the given call_activity params
// and TEST mode. Walks via NextEdge alone — no NodeFn registries needed,
// since routing is decided entirely by Context.State + the YAML predicates.
func walkStructuralCycle(t *testing.T, eng *Engine, params map[string]string, testMode string) []string {
	t.Helper()
	flow, ok := eng.Flows["structural_cycle"]
	if !ok {
		t.Fatalf("structural_cycle flow missing")
	}
	ctx := NewContext()
	ctx.Params = params
	ctx.Set("structural_test_mode", testMode)
	var visited []string
	cur := flow.Start
	for cur != "" {
		visited = append(visited, cur)
		node, ok := flow.Nodes[cur]
		if !ok {
			t.Fatalf("dangling node reference %q in structural_cycle", cur)
		}
		if node.Kind == EndEvent {
			break
		}
		next, err := eng.NextEdge("structural_cycle", cur, ctx)
		if err != nil {
			t.Fatalf("NextEdge from %q: %v", cur, err)
		}
		cur = next
	}
	return visited
}
