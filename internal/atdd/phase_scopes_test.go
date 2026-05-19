// Drift guards for phase-scopes.yaml. The production loader lives in
// phase_scopes.go; this file consumes it (one definition, no duplicate
// embedded byte slice, no duplicate allowlist).
package atdd

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

func loadPhaseScopes(t *testing.T) PhaseScopes {
	t.Helper()
	ps, err := LoadPhaseScopes()
	if err != nil {
		t.Fatalf("load phase-scopes.yaml: %v", err)
	}
	return ps
}

func loadEngine(t *testing.T) *statemachine.Engine {
	t.Helper()
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("load embedded process-flow.yaml: %v", err)
	}
	return eng
}

// concreteAgent returns the writing-agent name dispatched by a node, or
// "" if the node has no dispatch, dispatches a non-writing agent
// (`human`, `fix-verify`), or dispatches a templated agent (e.g.
// `${agent}` — the concrete name lives on the parent call_activity, not
// here, so checking the parent is sufficient).
func concreteAgent(node statemachine.Node) string {
	var agent string
	switch node.Kind {
	case statemachine.UserTask:
		agent = node.Raw.Agent
	case statemachine.CallActivity:
		agent = node.Raw.Params["agent"]
	default:
		return ""
	}
	if agent == "" || NonWritingAgents[agent] || strings.HasPrefix(agent, "${") {
		return ""
	}
	return agent
}

// writingAgentNodeIDs returns the set of node ids across every process
// that dispatch a concrete writing agent — i.e. the set of phase ids
// that must be either in phase-scopes.yaml or on the deferred allowlist.
func writingAgentNodeIDs(eng *statemachine.Engine) map[string]bool {
	out := map[string]bool{}
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if concreteAgent(node) != "" {
				out[node.ID] = true
			}
		}
	}
	return out
}

// allNodeIDs returns the set of every node id across every process —
// used for the forward FK check (phase-scopes → process-flow).
func allNodeIDs(eng *statemachine.Engine) map[string]bool {
	out := map[string]bool{}
	for _, proc := range eng.Processes {
		for id := range proc.Nodes {
			out[id] = true
		}
	}
	return out
}

// TestPhaseScopes_ForwardFK_PhasesExistInBPMN asserts every phase id in
// phase-scopes.yaml exists as a node somewhere in process-flow.yaml.
// Catches typos and stale entries.
func TestPhaseScopes_ForwardFK_PhasesExistInBPMN(t *testing.T) {
	ps := loadPhaseScopes(t)
	nodeIDs := allNodeIDs(loadEngine(t))
	for phaseID := range ps.Phases {
		if !nodeIDs[phaseID] {
			t.Errorf("phase %q in phase-scopes.yaml has no matching node id in process-flow.yaml", phaseID)
		}
	}
}

// TestPhaseScopes_ReverseFK_WritingAgentsScopedOrAllowlisted asserts
// every node that dispatches a writing agent has scope declared — either
// in phase-scopes.yaml or with an explicit deferred-plan citation on
// the allowlist. Filtering by writing-agent (rather than e.g.
// user_task-only) means CT_GREEN_STUBS (currently a bare user_task) and
// the templated call_activities (AT_RED_*, CT_RED_*, structure-cycle
// agents) are all caught by one rule.
func TestPhaseScopes_ReverseFK_WritingAgentsScopedOrAllowlisted(t *testing.T) {
	ps := loadPhaseScopes(t)
	for nodeID := range writingAgentNodeIDs(loadEngine(t)) {
		_, inScopes := ps.Phases[nodeID]
		_, inAllowlist := PhasesDeferredByPlan[nodeID]
		switch {
		case !inScopes && !inAllowlist:
			t.Errorf("writing-agent node %q is neither in phase-scopes.yaml nor in PhasesDeferredByPlan; add scope or document the deferral", nodeID)
		case inScopes && inAllowlist:
			t.Errorf("writing-agent node %q is in BOTH phase-scopes.yaml AND the deferred allowlist; remove from allowlist", nodeID)
		}
	}
}

// TestPhaseScopes_AllowlistEntriesStillExistInBPMN asserts every entry
// on the deferred allowlist still corresponds to a real node id in
// process-flow.yaml. Catches stale allowlist entries left behind after
// a BPMN rename or removal.
func TestPhaseScopes_AllowlistEntriesStillExistInBPMN(t *testing.T) {
	nodeIDs := allNodeIDs(loadEngine(t))
	for phaseID, plan := range PhasesDeferredByPlan {
		if !nodeIDs[phaseID] {
			t.Errorf("allowlist entry %q (deferred to %s) has no matching node id in process-flow.yaml; remove the stale allowlist entry", phaseID, plan)
		}
	}
}

// TestPhaseScopes_LayersAreCanonical asserts every layer name referenced
// in phase-scopes.yaml is either in canonicalPathKeys() (Family B) or
// in the explicitly-allowed Family A path-shaped key set
// (`system_path`).
func TestPhaseScopes_LayersAreCanonical(t *testing.T) {
	ps := loadPhaseScopes(t)
	canonical := map[string]bool{}
	for _, k := range projectconfig.CanonicalPathKeys() {
		canonical[k] = true
	}
	for phaseID, layers := range ps.Phases {
		for _, layer := range layers {
			if canonical[layer] || FamilyAPathKeysInScope[layer] {
				continue
			}
			t.Errorf("phase %q references layer %q not in canonicalPathKeys() or {system_path}", phaseID, layer)
		}
	}
}

// TestPhaseScopes_NoDuplicateLayersWithinPhase asserts no layer appears
// more than once within a single phase's list.
func TestPhaseScopes_NoDuplicateLayersWithinPhase(t *testing.T) {
	ps := loadPhaseScopes(t)
	for phaseID, layers := range ps.Phases {
		seen := map[string]bool{}
		for _, layer := range layers {
			if seen[layer] {
				t.Errorf("phase %q lists layer %q more than once", phaseID, layer)
			}
			seen[layer] = true
		}
	}
}

// TestPhaseScopes_NonEmptyLayerLists asserts every phase declares at
// least one layer. An empty list would mean "agent may modify nothing",
// which is never the intended scope shape — empty is always a mistake.
func TestPhaseScopes_NonEmptyLayerLists(t *testing.T) {
	ps := loadPhaseScopes(t)
	for phaseID, layers := range ps.Phases {
		if len(layers) == 0 {
			t.Errorf("phase %q has empty layer list", phaseID)
		}
	}
}
