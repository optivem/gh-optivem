// Drift guards for the per-phase scope inline on every writing-agent
// MID's EXECUTE_AGENT call-activity node in
// internal/atdd/runtime/statemachine/process-flow.yaml. The previous
// build-time guards used a sidecar SSoT (phase-scopes.yaml) joined with
// process-flow.yaml; the fold made that join unnecessary, and these
// guards now read scope directly via Engine.Scope.
package atdd

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

func loadEngine(t *testing.T) *statemachine.Engine {
	t.Helper()
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("load embedded process-flow.yaml: %v", err)
	}
	return eng
}

// writingAgentMIDs returns the sorted slice of MID process names that
// dispatch a writing agent — every process containing a CallActivity
// node whose `process:` is the LOW execute-agent primitive with a
// concrete (non-templated) task-name. This is the set whose
// EXECUTE_AGENT call-activity must declare inline scope (or its agent
// must declare `scope: none` in the prompt frontmatter).
//
// Templated task-names (`task-name: "fix-${failure-kind}"` on the `fix`
// LOW process) are skipped: fix dispatches resolve to a concrete MID at
// runtime, and that MID is already covered by its own entry in this map.
func writingAgentMIDs(eng *statemachine.Engine) map[string]string {
	out := map[string]string{}
	for name, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			task := node.Raw.Params["task-name"]
			if task == "" || strings.Contains(task, "${") {
				continue
			}
			out[name] = task
			break
		}
	}
	return out
}

// TestPhaseScopes_ReverseFK_WritingAgentsScoped asserts every
// writing-agent MID has scope declared inline on its EXECUTE_AGENT
// call-activity node (Engine.Scope returns ok), unless the agent's
// prompt frontmatter declares `scope: none` (the artifact-only /
// external-system-only doctrinal exemption — see runtime/shared/scope.md).
//
// The SSoT for the `scope: none` exemption is the prompt frontmatter
// itself, read here via agents.HasNoneScope — no sibling Go allowlist.
func TestPhaseScopes_ReverseFK_WritingAgentsScoped(t *testing.T) {
	eng := loadEngine(t)
	for processName, agent := range writingAgentMIDs(eng) {
		if _, _, ok := eng.Scope(processName); ok {
			continue
		}
		none, err := agents.HasNoneScope(agent)
		if err != nil {
			t.Errorf("writing-agent MID %q (agent %q): probe scope frontmatter: %v", processName, agent, err)
			continue
		}
		if none {
			continue
		}
		t.Errorf("writing-agent MID %q (agent %q) has no inline read/write scope on its EXECUTE_AGENT node and the prompt frontmatter does not declare `scope: none`; either add read: / write: to the node or declare `scope: none`", processName, agent)
	}
}

// TestPhaseScopes_LayersAreCanonical asserts every layer name in every
// writing-agent MID's read / write list is either in canonicalPathKeys()
// (Family B) or in the explicitly-allowed Family A path-shaped key set
// (`system-path`).
func TestPhaseScopes_LayersAreCanonical(t *testing.T) {
	eng := loadEngine(t)
	canonical := map[string]bool{}
	for _, k := range projectconfig.CanonicalPathKeys() {
		canonical[k] = true
	}
	check := func(processName, list string, layers []string) {
		for _, layer := range layers {
			if canonical[layer] || FamilyAPathKeysInScope[layer] {
				continue
			}
			t.Errorf("MID %q %s: layer %q not in canonicalPathKeys() or {system-path}", processName, list, layer)
		}
	}
	for processName := range writingAgentMIDs(eng) {
		read, write, ok := eng.Scope(processName)
		if !ok {
			continue
		}
		check(processName, "read", read)
		check(processName, "write", write)
	}
}

// TestPhaseScopes_NoDuplicateLayersWithinList asserts no layer appears
// more than once within a single MID's read list, and likewise within
// its write list. Identical entries across `read:` and `write:` are not
// duplicates — they're the symmetric case the explicit-lists rule
// accepts (and is the default at the fold).
func TestPhaseScopes_NoDuplicateLayersWithinList(t *testing.T) {
	eng := loadEngine(t)
	checkList := func(processName, list string, layers []string) {
		seen := map[string]bool{}
		for _, layer := range layers {
			if seen[layer] {
				t.Errorf("MID %q %s list mentions layer %q more than once", processName, list, layer)
			}
			seen[layer] = true
		}
	}
	for processName := range writingAgentMIDs(eng) {
		read, write, ok := eng.Scope(processName)
		if !ok {
			continue
		}
		checkList(processName, "read", read)
		checkList(processName, "write", write)
	}
}

// TestPhaseScopes_NonEmptyLayerLists asserts both `read:` and `write:`
// are non-empty per scoped writing-agent MID. Even placeholder-writing
// phases like write-acceptance-tests read at-test + dsl-port while
// writing dsl-core placeholders, so neither list is ever legitimately
// empty for a node that has scope declared at all.
func TestPhaseScopes_NonEmptyLayerLists(t *testing.T) {
	eng := loadEngine(t)
	for processName := range writingAgentMIDs(eng) {
		read, write, ok := eng.Scope(processName)
		if !ok {
			continue
		}
		if len(read) == 0 {
			t.Errorf("MID %q has empty read list", processName)
		}
		if len(write) == 0 {
			t.Errorf("MID %q has empty write list", processName)
		}
	}
}

// TestPhaseScopes_ReadWriteShape asserts every writing-agent MID's
// EXECUTE_AGENT node either carries BOTH read: and write: keys, or
// neither (the scope: none doctrinal exemption). Half-declared scope
// (only read or only write) is a schema error — the explicit-lists rule
// requires both keys present even when their values match.
func TestPhaseScopes_ReadWriteShape(t *testing.T) {
	eng := loadEngine(t)
	for processName := range writingAgentMIDs(eng) {
		proc := eng.Processes[processName]
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			readN, writeN := len(node.Raw.Read), len(node.Raw.Write)
			bothPresent := readN > 0 && writeN > 0
			bothAbsent := readN == 0 && writeN == 0
			if !bothPresent && !bothAbsent {
				t.Errorf("MID %q EXECUTE_AGENT: read (len=%d) and write (len=%d) must both be present or both absent — no half-declared scope", processName, readN, writeN)
			}
			break
		}
	}
}
