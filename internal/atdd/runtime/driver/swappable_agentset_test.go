// Agent-axis swap proof (child #8, Step 8). Demonstrates the second swap
// point: the *same* process flow, rebound to a different agent set, dispatches
// the alternate prompts — with no change to any YAML.
//
// The stub set is built in memory from testing/fstest, mirroring the default
// ATDD roster name-for-name but replacing every prompt body with a
// recognizable marker. Building it in-memory (rather than embedding fixture
// prompts under internal/assets/) keeps the stub out of the shipped binary —
// the agents.NewAgentSetFS filesystem seam exists precisely so an alternate
// set need not live in gh-optivem's own asset tree.
package driver

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/actions"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/clauderun"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/gates"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

const stubAgentBodyMarker = "STUB-AGENT-SET BODY"

// newStubAgentSet builds an in-memory agent set covering the same agent names
// as the built-in ATDD set, so it satisfies every static agent reference the
// process flow makes — but every body is the stub marker, and every tuning is
// a cheap fixed pair. agents.NewAgentSetFS binds it from the fstest.MapFS, so
// nothing is written to gh-optivem's embedded assets.
func newStubAgentSet(t *testing.T) *agents.AgentSet {
	t.Helper()
	stubFS := fstest.MapFS{}
	for _, name := range agents.DefaultAgentSet().Names() {
		stubFS["stub/"+name+".md"] = &fstest.MapFile{
			Data: []byte("---\nmodel: haiku\neffort: low\n---\n" + stubAgentBodyMarker + " for " + name + "\n"),
		}
	}
	return agents.NewAgentSetFS(stubFS, "stub")
}

// TestAgentAxis_StubSetSwapsDispatchedPrompt drives a real agent-dispatch
// (the acceptance-test-writer user-task in the standard minimalYAML harness)
// with the stub set bound via Options.AgentSet, and asserts the prompt handed
// to the fake claude came from the stub — not the default ATDD body. The YAML
// is byte-for-byte the same one the default-set dispatch tests use; only the
// bound agent set differs.
func TestAgentAxis_StubSetSwapsDispatchedPrompt(t *testing.T) {
	stub := newStubAgentSet(t)

	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1\n"), // pre rev-parse HEAD
			[]byte("aaaaaaa1\n"), // post rev-parse HEAD (unchanged)
		},
	}
	claudeFake := &fakeClaude{}
	opts := newDriverOpts(clauderun.Deps{Claude: claudeFake, Git: gitFake})
	opts.AgentSet = stub

	// Build the engine from the unchanged minimalYAML, but register and bind
	// the dispatchers against the stub set (matching what Run would do when
	// opts.AgentSet is the stub).
	eng, err := statemachine.LoadBytes([]byte(minimalYAML))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	agentReg := agents.New()
	registerAgentDispatchers(agentReg, stub)
	eng.AgentFn = agentReg.Lookup
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	wrapAgentDispatchers(eng, opts, defaultTestConfig(), nil)
	fn := eng.Processes["main"].Nodes["AT_RED_TEST"].Fn

	out := fn(newCtxWithIssue())
	if out.Err != nil {
		t.Fatalf("dispatch: %v", out.Err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	got := claudeFake.calls[0].Prompt
	if !strings.Contains(got, stubAgentBodyMarker) {
		t.Errorf("dispatched prompt did not come from the bound stub agent set:\n%s", got)
	}
	// And it must NOT carry the default ATDD acceptance-test-writer body —
	// otherwise the AgentSet binding had no effect on dispatch.
	if strings.Contains(got, "The Acceptance Criteria below were parsed") {
		t.Errorf("dispatched prompt still carries the default ATDD body; AgentSet swap had no effect")
	}
	// The shared dispatch chunks stay global regardless of the bound set:
	// the universal preamble's ${ticket-id} substitution still resolved, so
	// the swap replaced only the per-agent body, not the doctrine.
	if !strings.Contains(got, "#42") {
		t.Errorf("shared preamble ticket context missing; chunks should stay global across sets:\n%s", got)
	}
}

// TestAgentAxis_RealProcessBindsToStubSet proves the negative: the embedded
// production process-flow.yaml — loaded unchanged via process.Load — binds
// cleanly when its agent dispatchers are registered from the stub set. Because
// the stub mirrors the full ATDD roster, every static `agent:` reference
// resolves, so Bind succeeds without touching the YAML. This is the
// unit-level statement of "process-flow.yaml needs no change to swap agents".
func TestAgentAxis_RealProcessBindsToStubSet(t *testing.T) {
	stub := newStubAgentSet(t)

	eng, err := process.Load()
	if err != nil {
		t.Fatalf("process.Load: %v", err)
	}
	gateReg := gates.New()
	gates.RegisterAll(gateReg, gates.Deps{})
	actionReg := actions.New()
	actions.RegisterAll(actionReg, actions.Deps{})
	agentReg := agents.New()
	registerAgentDispatchers(agentReg, stub)
	eng.GateFn = gateReg.Lookup
	eng.ActionFn = actionReg.Lookup
	eng.AgentFn = agentReg.Lookup
	if err := eng.Bind(); err != nil {
		t.Fatalf("embedded process-flow.yaml failed to bind against the stub agent set: %v", err)
	}
}
