package clauderun

// Gap 6 of plan 20261110-1130: every agent's prompt gets the node params it
// needs on every call path that reaches it.
//
// A prompt's ${name} placeholders are filled from three sources (see
// seedPromptParams / renderPrompt): the fixed-schema and computed fields of
// Options, the project-wide PlaceholderMap, and NodeParams — which the driver
// builds from ctx.Params at RUN_AGENT, i.e. the params accumulated down the
// call-activity stack (buildNodeParams). State is NOT a NodeParams source.
//
// So for each agent: render its prompt with every non-NodeParams source
// maximally seeded (renderMatrixOpts) and NodeParams empty; whatever is left
// unfilled must come from node params. Then walk every call path of the
// loaded, unrolled process-flow from each root process, accumulating the
// call-activity param keys exactly as mergeCallActivityParams does, and check
// that each agent dispatch (execute-agent's RUN_AGENT, agent: ${agent}) has
// every key its prompt needs. A key whose value is still `${x}` after the
// params (a pass-through such as `use-case-id: ${use-case-id}`) is read from
// State at dispatch, so x must be a key something writes (knownStateKeys).
//
// Over-approximation: every node of a process counts as reached (gateway
// branches are not evaluated), except RUN_REVIEWER with no reviewer param.
// Agents whose name is templated from State (fix: ${failure-kind}-fixer) are
// skipped.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// agentDispatchSite is one call path reaching an agent dispatch.
type agentDispatchSite struct {
	agent  string
	params map[string]string // accumulated params (values expanded against params only)
	path   []string          // process@node chain
}

var placeholderRE = regexp.MustCompile(`\$\{([a-z0-9-]+)\}`)

// expandParamsOnly substitutes ${k} from params only (the static walk has no
// State); unresolved placeholders are left in place.
func expandParamsOnly(s string, params map[string]string) string {
	return placeholderRE.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := params[m[2:len(m)-1]]; ok {
			return v
		}
		return m
	})
}

// collectAgentDispatches walks every call path of eng from each root.
func collectAgentDispatches(t *testing.T, eng *statemachine.Engine) ([]agentDispatchSite, []string) {
	t.Helper()
	// A root is a process no call-activity names, statically or as a param
	// value feeding a templated `process: ${...}` (layer-process, action,
	// reviewer, dsl-reviewer).
	referenced := map[string]bool{}
	for _, p := range eng.Processes {
		for _, n := range p.Nodes {
			if n.Kind != statemachine.CallActivity {
				continue
			}
			referenced[n.Raw.Process] = true
			for _, v := range n.Raw.Params {
				referenced[v] = true
			}
		}
	}
	var roots []string
	for name := range eng.Processes {
		if !referenced[name] {
			roots = append(roots, name)
		}
	}
	sort.Strings(roots)

	var sites []agentDispatchSite
	paths := 0
	var walk func(proc string, params map[string]string, path []string, stack []string)
	walk = func(proc string, params map[string]string, path []string, stack []string) {
		p, ok := eng.Processes[proc]
		if !ok {
			t.Errorf("call path %v: unknown process %q", path, proc)
			return
		}
		// Each process at most twice on the stack: execute-agent re-enters
		// itself for the reviewer (RUN_REVIEWER -> review-* -> execute-agent);
		// deeper is the execute-agent -> fix -> execute-agent recursion.
		if n := len(slices.DeleteFunc(slices.Clone(stack), func(s string) bool { return s != proc })); n >= 2 {
			return
		}
		paths++
		if paths > 200000 {
			t.Fatalf("call-path walk exceeded 200000 paths")
		}
		stack = append(stack, proc)
		ids := make([]string, 0, len(p.Nodes))
		for id := range p.Nodes {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			n := p.Nodes[id]
			here := append(slices.Clone(path), proc+"@"+id)
			switch n.Kind {
			case statemachine.UserTask:
				agent := expandParamsOnly(n.Raw.Agent, params)
				if agent == "human" || strings.Contains(agent, "${") {
					continue // the human, or a State-templated fixer (fix: ${failure-kind}-fixer)
				}
				sites = append(sites, agentDispatchSite{agent: agent, params: params, path: here})
			case statemachine.CallActivity:
				callee := expandParamsOnly(n.Raw.Process, params)
				if callee == "" || (proc == "execute-agent" && callee == "${reviewer}") {
					// No reviewer param (or reviewer: ""): GATE_REVIEWER_CONFIGURED
					// never takes RUN_REVIEWER.
					continue
				}
				if strings.Contains(callee, "${") {
					t.Errorf("call path %v: process %q unresolved from params", here, n.Raw.Process)
					continue
				}
				merged := make(map[string]string, len(params)+len(n.Raw.Params))
				for k, v := range params {
					merged[k] = v
				}
				for k, v := range n.Raw.Params {
					merged[k] = expandParamsOnly(v, params)
				}
				walk(callee, merged, here, stack)
			}
		}
	}
	for _, r := range roots {
		walk(r, map[string]string{}, nil, nil)
	}
	return sites, roots
}

// knownStateKeys is every key something writes to State: declared agent
// outputs, preset-state, gateway bindings (the engine stamps each binding's
// value), and the keys the actions and the driver set (scanned from source:
// ctx.Set / setOrUnset with a literal or a CtxKey constant).
func knownStateKeys(t *testing.T, eng *statemachine.Engine) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	for _, p := range eng.Processes {
		for _, o := range p.Outputs {
			keys[o.Key] = true
		}
		for k := range p.PresetState {
			keys[k] = true
		}
		for _, n := range p.Nodes {
			if n.Kind == statemachine.Gateway {
				keys[n.Raw.Binding] = true
			}
		}
	}
	for _, o := range statemachine.EnvelopeOutputSpecs() {
		keys[o.Key] = true
	}
	var files []string
	for _, dir := range []string{filepath.Join("..", "actions"), filepath.Join("..", "..", "runtime", "driver")} {
		m, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(m) == 0 {
			t.Fatalf("scan %s: %v (%d files)", dir, err, len(m))
		}
		files = append(files, m...)
	}
	consts := map[string]string{}
	var idents []string
	constRE := regexp.MustCompile(`(?m)^\s*(?:const\s+)?(\w+)\s*=\s*"([a-z0-9_-]+)"`)
	setLitRE := regexp.MustCompile(`(?:ctx\.Set\(|setOrUnset\(ctx,\s*)"([a-z0-9_-]+)"`)
	setIdentRE := regexp.MustCompile(`(?:ctx\.Set\(|setOrUnset\(ctx,\s*)([A-Za-z]\w*)`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, m := range constRE.FindAllStringSubmatch(src, -1) {
			consts[m[1]] = m[2]
		}
		for _, m := range setLitRE.FindAllStringSubmatch(src, -1) {
			keys[m[1]] = true
		}
		for _, m := range setIdentRE.FindAllStringSubmatch(src, -1) {
			idents = append(idents, m[1])
		}
	}
	for _, id := range idents {
		if v, ok := consts[id]; ok {
			keys[v] = true
		}
	}
	return keys
}

// promptNodeParams returns the placeholders agent's prompt can only get from
// node params.
func promptNodeParams(t *testing.T, agent string) []string {
	t.Helper()
	opts := renderMatrixOpts(projectconfig.ArchMonolith, "system", "")
	opts.Agent = agent
	opts.NodeParams = map[string]string{}
	// renderPrompt fails on the first unresolved placeholder (strict
	// ExpandParams); seed each one it names as a node param and re-render.
	var keys []string
	for range 50 {
		prompt, err := renderPrompt(opts)
		if err != nil {
			m := regexp.MustCompile(`unresolved placeholder \$\{([a-z0-9-]+)\}`).FindStringSubmatch(err.Error())
			if m == nil {
				t.Fatalf("renderPrompt(%s): %v", agent, err)
			}
			keys = append(keys, m[1])
			opts.NodeParams[m[1]] = "x"
			continue
		}
		for _, l := range findUnfilledPlaceholders(prompt) {
			keys = append(keys, strings.TrimSuffix(strings.TrimPrefix(l, "${"), "}"))
		}
		sort.Strings(keys)
		return slices.Compact(keys)
	}
	t.Fatalf("renderPrompt(%s): more than 50 unresolved placeholders", agent)
	return nil
}

func loadUnrolledFlow(t *testing.T) *statemachine.Engine {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := eng.UnrollSystemChannels([]string{"api", "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.UnrollSystemDriverAdapterChannels([]string{"api", "ui"}); err != nil {
		t.Fatal(err)
	}
	if err := eng.UnrollExternalSystems([]string{"clock", "erp"}, map[string]string{"clock": "test-instance", "erp": "simulator"}); err != nil {
		t.Fatal(err)
	}
	return eng
}

func TestAgentNodeParams_SuppliedOnEveryCallPath(t *testing.T) {
	eng := loadUnrolledFlow(t)
	sites, roots := collectAgentDispatches(t, eng)
	if len(sites) == 0 {
		t.Fatal("no agent dispatches found")
	}
	t.Logf("roots: %v; %d agent dispatch paths", roots, len(sites))

	for _, m := range missingNodeParams(t, eng, sites) {
		t.Error(m)
	}
	required := map[string][]string{}
	for _, s := range sites {
		if _, ok := required[s.agent]; !ok {
			required[s.agent] = promptNodeParams(t, s.agent)
		}
	}
	names := mapKeys(required)
	sort.Strings(names)
	for _, a := range names {
		t.Logf("%s needs node params %v", a, required[a])
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The characterize agents' params, pinned by value (the general test only
// checks presence).
func TestCharacterizeAgents_NodeParamValues(t *testing.T) {
	eng := loadUnrolledFlow(t)
	sites, _ := collectAgentDispatches(t, eng)
	byAgent := map[string][]agentDispatchSite{}
	for _, s := range sites {
		byAgent[s.agent] = append(byAgent[s.agent], s)
	}

	// The prompts reference these, so the general test enforces presence.
	for agent, keys := range map[string][]string{
		"legacy-use-case-narrative-writer": {"use-case-id", "discovered-external-port-paths"},
		"legacy-use-case-criteria-writer":  {"use-case-id", "discovered-external-port-paths"},
		"characterization-test-fixer":      {"task-name"},
	} {
		got := promptNodeParams(t, agent)
		for _, k := range keys {
			if !slices.Contains(got, k) {
				t.Errorf("%s prompt no longer needs node param %q (needs %v); update this test", agent, k, got)
			}
		}
	}

	for _, writer := range []string{"legacy-use-case-narrative-writer", "legacy-use-case-criteria-writer"} {
		writers := byAgent[writer]
		if len(writers) == 0 {
			t.Fatalf("%s is never dispatched", writer)
		}
		for _, s := range writers {
			if s.params["use-case-id"] != "${use-case-id}" {
				t.Errorf("%s use-case-id = %q (want the State use case, ${use-case-id}) on %v", writer, s.params["use-case-id"], s.path)
			}
			// "" on the criteria layer, the DSL's changed external port paths
			// on the late-discovery route.
			if v := s.params["discovered-external-port-paths"]; v != "" && v != "${external-driver-port-changed-paths}" {
				t.Errorf("%s discovered-external-port-paths = %q on %v", writer, v, s.path)
			}
		}
	}

	fixers := byAgent["characterization-test-fixer"]
	if len(fixers) == 0 {
		t.Fatal("characterization-test-fixer is never dispatched")
	}
	tasks := map[string]bool{}
	for _, s := range fixers {
		task := s.params["task-name"]
		tasks[task] = true
		switch task {
		case "fix-characterization-failing-tests":
			// verify-failure-output comes from State (the failing run's output).
		case "fix-characterization-passing-tests":
			// A passing run leaves no failure output in State, so the node
			// param is the only source.
			if s.params["verify-failure-output"] == "" {
				t.Errorf("passing-tests fixer has no verify-failure-output node param on %v", s.path)
			}
		default:
			t.Errorf("characterization-test-fixer task-name = %q on %v", task, s.path)
		}
	}
	for _, want := range []string{"fix-characterization-failing-tests", "fix-characterization-passing-tests"} {
		if !tasks[want] {
			t.Errorf("characterization-test-fixer never dispatched with task-name %s", want)
		}
	}
	// The fixer's prompt needs verify-failure-output: with neither State
	// (verify_failure_output) nor a node param, the render fails.
	opts := renderMatrixOpts(projectconfig.ArchMonolith, "system", "")
	opts.Agent = "characterization-test-fixer"
	opts.VerifyFailureOutput = ""
	opts.NodeParams = map[string]string{"task-name": "fix-characterization-passing-tests"}
	_, err := renderPrompt(opts)
	if err == nil || !strings.Contains(err.Error(), "${verify-failure-output}") {
		t.Errorf("without State or node param, verify-failure-output must be unresolved; renderPrompt err = %v", err)
	}
}

// The general check reports a dropped node param (self-test): without
// ADD_ESCC's discovered-external-port-paths, the inner pass-through
// `discovered-external-port-paths: ${discovered-external-port-paths}` reads a
// State key nothing writes.
func TestAgentNodeParams_DetectsDroppedParam(t *testing.T) {
	eng := loadUnrolledFlow(t)
	delete(eng.Processes["characterize-dsl-layer"].Nodes["ADD_ESCC_FOR_DISCOVERED_SYSTEMS"].Raw.Params, "discovered-external-port-paths")
	sites, _ := collectAgentDispatches(t, eng)
	if got := missingNodeParams(t, eng, sites); !slices.ContainsFunc(got, func(m string) bool {
		return strings.Contains(m, "legacy-use-case-criteria-writer") && strings.Contains(m, "ADD_ESCC_FOR_DISCOVERED_SYSTEMS") &&
			strings.Contains(m, "discovered-external-port-paths")
	}) {
		t.Errorf("dropped param not reported; reports: %v", got)
	}
}

// missingNodeParams returns one report per call path on which an agent lacks
// a node param its prompt needs.
func missingNodeParams(t *testing.T, eng *statemachine.Engine, sites []agentDispatchSite) []string {
	t.Helper()
	stateKeys := knownStateKeys(t, eng)
	var reports []string
	known := map[string]bool{}
	for _, name := range agents.DefaultAgentSet().Names() {
		known[name] = true
	}
	required := map[string][]string{}
	for _, s := range sites {
		if !known[s.agent] {
			reports = append(reports, fmt.Sprintf("call path %v dispatches agent %q, which has no prompt", s.path, s.agent))
			continue
		}
		req, ok := required[s.agent]
		if !ok {
			req = promptNodeParams(t, s.agent)
			required[s.agent] = req
		}
		var missing []string
		for _, k := range req {
			v, ok := s.params[k]
			if !ok {
				missing = append(missing, k)
				continue
			}
			// Left unresolved by the params, ${x} is read from State at
			// dispatch (strict ExpandParams): something must write x.
			for _, m := range placeholderRE.FindAllStringSubmatch(v, -1) {
				if !stateKeys[m[1]] {
					missing = append(missing, fmt.Sprintf("%s (= %q; nothing passes or writes %s)", k, v, m[1]))
				}
			}
		}
		if len(missing) > 0 {
			reports = append(reports, fmt.Sprintf("agent %s lacks node param(s) %v on call path\n  %s", s.agent, missing, strings.Join(s.path, "\n  ")))
		}
	}
	return reports
}
