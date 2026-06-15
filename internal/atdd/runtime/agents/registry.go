// Package agents holds the user-task agent-dispatch registry. Every YAML
// node of type `user-task` carries an `agent:` string identifying which
// `internal/assets/runtime/agents/atdd/<name>.md` agent to dispatch (or `human` for STOP
// nodes that block on stdin without dispatching anything).
//
// Real dispatch implementations are added in Session 3 of the rollout plan
// (when the driver loop wires together Agent SDK calls). v1 ships with the
// registry pattern and a built-in `human` STOP so transitions tests can
// exercise STOP-only sub-flows (e.g. intake REVIEW gates) end to end
// without mocking anything.
package agents

import (
	"fmt"
	"os"

	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Registry maps agent names from the YAML to NodeFn dispatchers.
type Registry struct {
	dispatchers map[string]statemachine.NodeFn
}

// New returns a Registry pre-populated with the `human` STOP dispatcher,
// which prints the node description and blocks on stdin for one Enter.
func New() *Registry {
	r := &Registry{dispatchers: map[string]statemachine.NodeFn{}}
	r.Register("human", humanStop)
	return r
}

// Register adds a dispatcher under the given agent name. Duplicate
// registration panics.
func (r *Registry) Register(name string, fn statemachine.NodeFn) {
	if _, dup := r.dispatchers[name]; dup {
		panic("agents: duplicate agent registration: " + name)
	}
	r.dispatchers[name] = fn
}

// Lookup returns the dispatcher registered under name, or nil if absent.
func (r *Registry) Lookup(name string) statemachine.NodeFn {
	return r.dispatchers[name]
}

// humanStop is the built-in dispatcher for `agent: human` nodes. Routes
// the y/n decision through approval.Confirm with CategoryHuman so every
// human-STOP node shares the same semantics: explicit y/n required, no
// Enter shortcut, and never short-circuits even under --auto (the BPMN
// human-STOP author chose this STOP precisely because no machine decides
// it; --auto explicitly cannot opt out).
//
// This fallback runs only on processes the driver did NOT wrap (tests
// and code paths that bypass wrapAgentDispatchers). The wrapped version
// in driver.newHumanStopDispatcher carries node-id context and prints
// the YAML description; this version just halts with the bare prompt.
// Passing a zero Resolved keeps the bare-prompt path simple — the
// invariant "CategoryHuman always prompts" holds regardless.
func humanStop(ctx *statemachine.Context) statemachine.Outcome {
	fmt.Fprintln(os.Stderr, "STOP — human approval required.")
	ok, err := approval.Confirm(approval.Resolved{}, approval.CategoryHuman, os.Stdin, os.Stderr, "Approve?")
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("read STOP confirmation: %w", err)}
	}
	if !ok {
		return statemachine.Outcome{Err: fmt.Errorf("user aborted at STOP")}
	}
	return statemachine.Outcome{}
}
