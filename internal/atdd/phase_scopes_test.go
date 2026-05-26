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

	"github.com/optivem/gh-optivem/internal/assets"
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
// EXECUTE_AGENT call-activity must declare inline scope (read/write
// lists or `scope: none`).
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
// call-activity node — either concrete `read:` / `write:` lists
// (Engine.Scope returns ok) or the doctrinal `scope: none` exemption
// (Engine.IsScopeNone returns true), the latter for artifact-only /
// external-system-only agents that never write to the repo working tree
// (see runtime/shared/scope.md).
//
// Single SSoT: the BPMN node. The pre-fold `scope: none` frontmatter
// fallback has been retired (plan 20260526-1448 Item 9).
func TestPhaseScopes_ReverseFK_WritingAgentsScoped(t *testing.T) {
	eng := loadEngine(t)
	for processName, agent := range writingAgentMIDs(eng) {
		if _, _, ok := eng.Scope(processName); ok {
			continue
		}
		if eng.IsScopeNone(processName) {
			continue
		}
		t.Errorf("writing-agent MID %q (agent %q) has no inline read/write scope and no `scope: none` declaration on its EXECUTE_AGENT node; add read: / write: lists or declare `scope: none`", processName, agent)
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
// neither (paired with a `scope: none` doctrinal exemption). Half-declared
// scope (only read or only write) is a schema error — the explicit-lists
// rule requires both keys present even when their values match.
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

// TestPhaseScopes_NodeScopeFieldShape asserts the `scope:` field on a
// writing-agent MID's EXECUTE_AGENT node — when present — is exactly
// "none", and that `scope: none` is mutually exclusive with `read:` /
// `write:`. Any other shape (a non-empty string other than "none", or
// `scope: none` co-existing with read/write lists) is a schema error.
//
// `scope: none` is the artifact-only / external-system-only doctrinal
// exemption (plan 20260526-1448 Item 9); a node with both `scope: none`
// AND read/write would be self-contradictory.
func TestPhaseScopes_NodeScopeFieldShape(t *testing.T) {
	eng := loadEngine(t)
	for processName := range writingAgentMIDs(eng) {
		proc := eng.Processes[processName]
		for _, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "execute-agent" {
				continue
			}
			scope := node.Raw.Scope
			if scope != "" && scope != "none" {
				t.Errorf("MID %q EXECUTE_AGENT: scope: %q — only %q is a recognised value", processName, scope, "none")
			}
			if scope == "none" && (len(node.Raw.Read) > 0 || len(node.Raw.Write) > 0) {
				t.Errorf("MID %q EXECUTE_AGENT: scope: none must not co-exist with read: / write: lists", processName)
			}
			break
		}
	}
}

// TestPromptFrontmatter_NoScopeField rejects any `scope:` field in any
// prompt's YAML frontmatter under internal/assets/runtime/prompts/atdd/.
// The single SSoT for per-phase scope is the EXECUTE_AGENT node in
// process-flow.yaml (plan 20260526-1448 Item 9); reintroducing
// `scope:` on the prompt body would re-fork the SSoT.
func TestPromptFrontmatter_NoScopeField(t *testing.T) {
	for _, name := range agents.Names() {
		fm := readPromptFrontmatter(t, name)
		if fm == "" {
			continue
		}
		for _, line := range strings.Split(fm, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			if strings.HasPrefix(trimmed, "scope:") {
				t.Errorf("prompt %q: frontmatter carries `%s` — scope lives on the EXECUTE_AGENT node in process-flow.yaml, not in prompt frontmatter", name, trimmed)
			}
		}
	}
}

// readPromptFrontmatter returns the YAML frontmatter block (without
// fences) for the named prompt, or "" if the prompt has no frontmatter.
// Fatals on a missing prompt — the test must not silently pass when an
// expected prompt is unreadable.
func readPromptFrontmatter(t *testing.T, name string) string {
	t.Helper()
	data, err := assets.FS.ReadFile("runtime/prompts/atdd/" + name + ".md")
	if err != nil {
		t.Fatalf("read embedded prompt %q: %v", name, err)
	}
	s := string(data)
	const marker = "---"
	first, rest, ok := strings.Cut(s, "\n")
	if !ok || strings.TrimRight(first, "\r") != marker {
		return ""
	}
	end := strings.Index(rest, "\n"+marker)
	if end < 0 {
		// No closing marker — treat as no frontmatter (matches the
		// degraded behaviour in agents.splitFrontmatter).
		return ""
	}
	return rest[:end+1]
}
