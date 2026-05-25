// Transitions test suite for the ATDD process-flow YAML.
//
// Scope: structural invariants over the loaded YAML plus predicate-
// evaluator unit tests. The detailed transition-table coverage that used
// to live here was authored for the pre-refactor (AT-cycle / CT-subprocess /
// structural-cycle / legacy-acceptance-criteria) shape; the BPMN five-level
// refactor (plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md Item 3)
// replaced that wholesale and the per-edge rows no longer apply. Phase D's
// downstream-alignment plan re-establishes execution-flow coverage when
// the runtime registries (ActionFn / AgentFn / GateFn) for the new
// vocabulary land.
//
// Tests use the unbound Engine (no Bind()) — nextEdge does not need NodeFn
// resolution; it only walks the edge list against the Context state.
package statemachine

import (
	"testing"
)

func loadSnapshot(t *testing.T) *Engine {
	t.Helper()
	eng, err := LoadDefault()
	if err != nil {
		t.Fatalf("load embedded process-flow: %v", err)
	}
	return eng
}

// ---------------------------------------------------------------------------
// Snapshot inventory — every named process the embedded YAML defines
// ---------------------------------------------------------------------------

func TestLoadSnapshot_AllProcessesParse(t *testing.T) {
	eng := loadSnapshot(t)
	wantProcesses := []string{
		// runtime bootstrap
		"main",
		// TOP
		"refine-ticket",
		"implement-ticket",
		"refactor",
		// CYCLE
		"refine-backlog",
		"onboard-external-system",
		"change-system-behavior",
		"cover-system-behavior",
		"redesign-system-structure",
		"refactor-system-structure",
		"refactor-test-structure",
		// HIGH
		"write-and-verify-acceptance-tests-fail",
		"write-and-verify-acceptance-tests-pass",
		"write-and-verify-acceptance-tests",
		"write-and-verify-acceptance-test-code",
		"implement-and-verify-dsl",
		"implement-and-verify-system-driver-adapters",
		"implement-and-verify-external-system-driver-adapters",
		"implement-and-verify-external-system-driver-adapters-contract-tests",
		"implement-and-verify-system",
		"refactor-and-verify-tests",
		"implement-test-layer",
		"verify-tests-pass",
		"verify-tests-fail",
		// MID — agent tasks
		"write-acceptance-tests",
		"write-contract-tests",
		"implement-dsl",
		"implement-system",
		"implement-system-driver-adapters",
		"implement-external-system-driver-adapters",
		"implement-external-system-stubs",
		"disable-tests",
		"enable-tests",
		"fix-unexpected-passing-tests",
		"fix-unexpected-failing-tests",
		"refactor-tests",
		"refactor-system",
		"refine-acceptance-criteria",
		"update-ticket",
		// MID — command tasks
		"compile",
		"compile-system",
		"compile-tests",
		"build-system",
		"start-system",
		"commit",
		"run-tests",
		// LOW
		"approve",
		"execute-agent",
		"execute-command",
		"fix",
	}
	for _, name := range wantProcesses {
		if _, ok := eng.Processes[name]; !ok {
			t.Errorf("process %q missing from loaded snapshot", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Structural invariants
// ---------------------------------------------------------------------------

func TestStructuralIntegrity_StartNodesExist(t *testing.T) {
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		if _, ok := process.Nodes[process.Start]; !ok {
			t.Errorf("process %q: start node %q not in nodes list", name, process.Start)
		}
	}
}

func TestStructuralIntegrity_GatewaysHaveOutgoingEdges(t *testing.T) {
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind != Gateway {
				continue
			}
			if len(process.OutgoingByNode[id]) == 0 {
				t.Errorf("process %q gateway %q has no outgoing edges", name, id)
			}
		}
	}
}

func TestStructuralIntegrity_NonEndNodesHaveSuccessor(t *testing.T) {
	// A node may legitimately have no outgoing edges in two cases:
	//   - it is an end_event, or
	//   - it is a `agent: human` STOP node intended to halt the run
	//     awaiting user intervention.
	// Anything else is a missing-edge bug.
	eng := loadSnapshot(t)
	for name, process := range eng.Processes {
		for id, node := range process.Nodes {
			if node.Kind == EndEvent {
				continue
			}
			if len(process.OutgoingByNode[id]) > 0 {
				continue
			}
			if node.Kind == UserTask && node.Raw.Agent == "human" {
				continue // intentional STOP-and-halt
			}
			t.Errorf("process %q non-end node %q has no outgoing edges", name, id)
		}
	}
}

// ---------------------------------------------------------------------------
// Predicate evaluator unit tests
// ---------------------------------------------------------------------------

func TestPredicate_EmptyAlwaysTrue(t *testing.T) {
	ctx := NewContext()
	got, err := evalPredicate("", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("empty predicate: got false, want true")
	}
}

func TestPredicate_Equality(t *testing.T) {
	cases := []struct {
		state    map[string]any
		expr     string
		want     bool
		wantErr  bool
		caseName string
	}{
		{map[string]any{"ticket_type": "story"}, "ticket_type == story", true, false, "bare value matches"},
		{map[string]any{"ticket_type": "story"}, `ticket_type == "story"`, true, false, "quoted value matches"},
		{map[string]any{"ticket_type": "bug"}, "ticket_type == story", false, false, "mismatch returns false"},
		{map[string]any{}, "ticket_type == story", false, false, "missing key treated as empty string"},
	}
	for _, tc := range cases {
		t.Run(tc.caseName, func(t *testing.T) {
			ctx := NewContext()
			for k, v := range tc.state {
				ctx.Set(k, v)
			}
			got, err := evalPredicate(tc.expr, ctx)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("evalPredicate(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestPredicate_BoolEquality(t *testing.T) {
	ctx := NewContext()
	ctx.Set("approval_outcome", true)
	got, err := evalPredicate("approval_outcome == true", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("bool equality: got false, want true")
	}
}

func TestPredicate_InList(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "bug")
	got, err := evalPredicate("ticket_type in [story, bug]", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if !got {
		t.Errorf("`in` membership: got false, want true")
	}
}

func TestPredicate_InListNegative(t *testing.T) {
	ctx := NewContext()
	ctx.Set("ticket_type", "spike")
	got, err := evalPredicate("ticket_type in [story, bug]", ctx)
	if err != nil {
		t.Fatalf("evalPredicate: %v", err)
	}
	if got {
		t.Errorf("`in` non-membership: got true, want false")
	}
}
