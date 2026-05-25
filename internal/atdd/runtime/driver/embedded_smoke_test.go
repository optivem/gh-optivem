// Smoke test for the "embedded YAML + embedded prompts" property: a
// fully-stripped consumer repo (no `.claude/`, no
// `docs/atdd/process/process-flow.yaml`) must still be able to walk the
// pipeline end-to-end. This locks in the consolidation goal — future
// schema changes that accidentally re-introduce a consumer-side file
// lookup will fail this test loudly.
//
// The test stays in package driver so it can reuse the fakeClaude /
// fakeGit / buildEngineFrom helpers from driver_test.go without
// re-implementing them.
package driver

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/actions"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// TestEmbeddedArtifacts_LoadInConsumerEmptyDir asserts the engine and
// every embedded agent prompt resolve without reading any file from the
// consumer-side scaffolding paths the v1 architecture used. The temp
// dir is deliberately empty: any code path that fell back to
// `<repoPath>/docs/atdd/process/process-flow.yaml` or
// `<repoPath>/.claude/agents/atdd/<name>.md` would surface here.
func TestEmbeddedArtifacts_LoadInConsumerEmptyDir(t *testing.T) {
	tempDir := t.TempDir()

	// Sanity: the temp dir really is empty of the v1 scaffolding paths.
	// If a future change creates these on TempDir setup (none does today,
	// but a hook could), this assertion prevents the test from passing
	// for the wrong reason.
	for _, sub := range []string{
		".claude",
		filepath.Join("docs", "atdd", "process"),
		filepath.Join("docs", "atdd", "process", "process-flow.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(tempDir, sub)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("temp dir %q must not contain %q (err: %v)", tempDir, sub, err)
		}
	}

	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	if len(eng.Processes) == 0 {
		t.Fatalf("embedded YAML produced zero flows")
	}

	for _, name := range agents.Names() {
		if _, err := agents.Prompt(name); err != nil {
			t.Errorf("agents.Prompt(%q) failed: %v", name, err)
		}
	}

	// Every static (non-templated, non-human) agent reference in the YAML
	// must have a corresponding embedded prompt. Without this, a YAML
	// node could reference a `${name}` that the consumer was expected to
	// supply — exactly the dependency this plan removes. Templated
	// `${agent}` nodes (resolved at runtime via call_activity params)
	// are skipped here; their resolved values are covered by the
	// existing TestClaudeRunDispatch_ExpandsTemplatedNodeFields.
	for processName, process := range eng.Processes {
		for nodeID, node := range process.Nodes {
			if node.Kind != statemachine.UserTask {
				continue
			}
			agent := node.Raw.Agent
			if agent == "" || agent == "human" || strings.HasPrefix(agent, "${") {
				continue
			}
			if _, err := agents.Prompt(agent); err != nil {
				t.Errorf("process %s node %s: agent %q has no embedded prompt: %v",
					processName, nodeID, agent, err)
			}
		}
	}
}

// TestEmbeddedDispatch_RunsInConsumerEmptyDir walks a real production
// user_task (FIX_TEST in the embedded `structural_cycle` flow) against
// the fake clauderun + git pair, with RepoPath set to a temp dir that
// contains no consumer-side scaffolding. Asserts the dispatch completes
// and the rendered prompt is the embedded fix-unexpected-failing-tests body — proves
// the dispatcher reaches the embedded prompt without any consumer-file
// dependency.
//
// AT_RED_TEST was the original target; after the AT/CT creative-vs-
// mechanical split, every RED-phase node in at_cycle is a call_activity
// into red_phase_cycle, and the AT_GREEN_SYSTEM phase's backend/frontend
// nodes are now call_activities into green_phase_cycle. The remaining
// statically-bound user_tasks live inside structural_cycle (and the
// deferred CT stubs node), so FIX_TEST is used here (one of the two
// fix-unexpected-{passing,failing}-tests dispatch sites — FIX_COMPILE is its compile-RED twin).
// Any embedded user_task with a static (non-templated) agent will do.
func TestEmbeddedDispatch_RunsInConsumerEmptyDir(t *testing.T) {
	// The five-level BPMN refactor (plans/20260525-1517-bpmn-refactor-yaml-
	// and-diagrams.md Item 3) replaced the structural_cycle / red_phase_cycle
	// / green_phase_cycle shape this smoke test exercises, and the new
	// runtime gateway/action bindings (refactor_type_choice, ticket_kind,
	// run_command, validate_outputs_and_scopes, …) are not yet registered.
	// Phase D's downstream-alignment plan re-establishes the embedded-prompt
	// smoke test against the new structure once the registries land.
	t.Skip("pending Phase D: register new gates/actions for the five-level YAML")
	tempDir := t.TempDir()

	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"),
			[]byte("bbbbbbb2\n"),
			[]byte("AT-RED-TEST: scenario\n"),
		},
	}
	claudeFake := &fakeClaude{}
	opts := newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake})
	opts.RepoPath = tempDir

	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	gateReg := gates.New()
	gates.RegisterAll(gateReg, gates.Deps{})
	actionReg := actions.New()
	actions.RegisterAll(actionReg, actions.Deps{})
	agentReg := agents.New()
	registerAgentDispatchers(agentReg)
	eng.GateFn = gateReg.Lookup
	eng.ActionFn = actionReg.Lookup
	eng.AgentFn = agentReg.Lookup
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	wrapAgentDispatchers(eng, opts, nil, nil)

	process, ok := eng.Processes["structural_cycle"]
	if !ok {
		t.Fatal("embedded YAML missing structural_cycle process")
	}
	node, ok := process.Nodes["FIX_TEST"]
	if !ok {
		t.Fatal("structural_cycle process missing FIX_TEST node")
	}

	ctx := newCtxWithIssue()

	out := node.Fn(ctx)
	if out.Err != nil {
		t.Fatalf("dispatch in consumer-empty dir failed: %v", out.Err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	prompt := claudeFake.calls[0].Prompt
	if !strings.Contains(prompt, "You are the Fix-Verify Agent") {
		t.Errorf("dispatched prompt missing embedded-prompt sentinel; consumer-side fallback may have leaked in")
	}
}

// TestEmbeddedDriver_RunBypassesConsumerScaffolding pushes a step
// further: build full driver Options and call Run, but cap the engine
// to a one-node flow so we don't need a real board / gh shell-out.
// Validates that Run itself — not just the dispatcher mechanics —
// reaches no consumer-side file when invoked from a temp dir. The
// preflight is stubbed to nil since `claude` isn't on PATH in CI.
func TestEmbeddedDriver_RunBypassesConsumerScaffolding(t *testing.T) {
	tempDir := t.TempDir()

	prev := preflightFn
	t.Cleanup(func() { preflightFn = prev })
	preflightFn = func(_ context.Context) error { return nil }

	// Single-node manual-agents flow: no clauderun call, no board call.
	// The point is to assert Run() doesn't reach for consumer files
	// during its setup (engine-load, register, bind, wrap).
	yaml := `
processes:
  main:
    start: STOP
    nodes:
      - id: STOP
        type: user_task
        agent: human
        documentation: smoke
    sequence_flows: []
`
	yamlPath := filepath.Join(tempDir, "smoke-flow.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write smoke YAML: %v", err)
	}

	err := Run(context.Background(), Options{
		YAMLPath:     yamlPath,
		RepoPath:     tempDir,
		ManualAgents: true,
		Stdout:       &discardWriter{},
		Stderr:       &discardWriter{},
		Stdin:        strings.NewReader("y\n"), // approve the human STOP
	})
	if err != nil {
		t.Fatalf("Run failed in consumer-empty dir: %v", err)
	}
}

// discardWriter satisfies io.Writer with no allocation. Used in lieu of
// io.Discard for tests that pass it via Options.Stdout — Options
// declares io.Writer, and io.Discard is io.Writer at the interface
// level, so this is purely a stylistic choice.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
