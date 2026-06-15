// Package gates holds the gateway-evaluator registry. Every YAML node of
// type `gateway` carries a `binding:` string; that string maps to a Go
// function in this registry which the engine calls to decide which outgoing
// edge to follow.
//
// The registry is intentionally minimal — Register / Lookup, no runtime
// state. Bindings should be pure functions of the live Context (read flags
// the upstream phases set; do not mutate state outside Context.Set).
//
// Real bindings (dsl_interface_changed, ticket_type, structural_test_mode, …)
// are added in Session 2 of the rollout plan, when the engine is wired up
// with real `gh` and git diff calls. v1 ships with the registry pattern so
// later additions slot in without engine changes.
package gates

import "github.com/optivem/gh-optivem/internal/engine/statemachine"

// Registry maps binding names from the YAML to NodeFn evaluators.
type Registry struct {
	bindings map[string]statemachine.NodeFn
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{bindings: map[string]statemachine.NodeFn{}}
}

// Register adds an evaluator under the given binding name. Duplicate
// registration panics — wiring bugs surface early, not at first dispatch.
func (r *Registry) Register(name string, fn statemachine.NodeFn) {
	if _, dup := r.bindings[name]; dup {
		panic("gates: duplicate binding registration: " + name)
	}
	r.bindings[name] = fn
}

// Lookup returns the evaluator registered under name, or nil if absent.
// The engine calls Lookup at Bind time and refuses to start with an unknown
// binding — see statemachine.Engine.resolve.
func (r *Registry) Lookup(name string) statemachine.NodeFn {
	return r.bindings[name]
}
