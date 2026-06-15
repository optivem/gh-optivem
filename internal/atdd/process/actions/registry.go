// Package actions holds the service-task action registry. Every YAML node of
// type `service-task` carries an `action:` string; that string maps to a Go
// function in this registry which the engine calls to perform the
// mechanical step (pick top Ready, move to In Progress, run the smoke
// test, commit the phase, etc.).
//
// Real actions are added in Sessions 2–3 of the rollout plan; this v1
// package ships with the registry pattern so the loader can validate that
// every YAML action has a binding the moment it's added.
package actions

import "github.com/optivem/gh-optivem/internal/engine/statemachine"

// Registry maps action names from the YAML to NodeFn implementations.
type Registry struct {
	actions map[string]statemachine.NodeFn
}

// New returns an empty Registry.
func New() *Registry {
	return &Registry{actions: map[string]statemachine.NodeFn{}}
}

// Register adds an implementation under the given action name. Duplicate
// registration panics.
func (r *Registry) Register(name string, fn statemachine.NodeFn) {
	if _, dup := r.actions[name]; dup {
		panic("actions: duplicate action registration: " + name)
	}
	r.actions[name] = fn
}

// Lookup returns the implementation registered under name, or nil if absent.
func (r *Registry) Lookup(name string) statemachine.NodeFn {
	return r.actions[name]
}
