// Package agents holds the user-task agent-dispatch registry. Every YAML
// node of type `user_task` carries an `agent:` string identifying which
// `.claude/agents/atdd/<name>.md` agent to dispatch (or `human` for STOP
// nodes that block on stdin without dispatching anything).
//
// Real dispatch implementations are added in Session 3 of the rollout plan
// (when the driver loop wires together Agent SDK calls). v1 ships with the
// registry pattern and a built-in `human` STOP so transitions tests can
// exercise STOP-only sub-flows (legacy_coverage, intake REVIEW gates) end
// to end without mocking anything.
package agents

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
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

// humanStop is the built-in dispatcher for `agent: human` nodes. It prints
// the node description (or a generic message) and reads one line from
// stdin. Any non-empty input is accepted; "abort" or "stop" returns an
// error to halt the run.
func humanStop(ctx *statemachine.Context) statemachine.Outcome {
	fmt.Fprintln(os.Stderr, "STOP — press Enter to continue, or type `abort` to halt:")
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "abort" || line == "stop" {
		return statemachine.Outcome{Err: fmt.Errorf("user aborted at STOP")}
	}
	return statemachine.Outcome{}
}
