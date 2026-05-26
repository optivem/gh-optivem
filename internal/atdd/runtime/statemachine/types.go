// Package statemachine implements the ATDD pipeline orchestrator: a hand-coded,
// BPMN-shaped graph traversal that walks a process-flow YAML node-by-node,
// dispatching service tasks and user-task agents in turn.
//
// The engine has three job-shaped types: Outcome, NodeFn, and Process. Everything
// else (gates, actions, agents) plugs in through registries.
package statemachine

// NodeKind enumerates the BPMN-shaped node types supported by the YAML schema.
// The vocabulary is BPMN's; the runtime is hand-coded and does not embed a
// BPMN engine.
type NodeKind int

const (
	StartEvent NodeKind = iota
	EndEvent
	ErrorEndEvent // exceptional exit; terminates the process with an error (BPMN error end event)
	ServiceTask   // mechanical action; auto-executed via the actions registry
	UserTask      // creative work; dispatches an agent (or blocks on human input)
	Gateway       // XOR decision; binding evaluated via the gates registry
	CallActivity  // embedded sub-process; runs to completion, returns to caller
)

// Outcome is what every NodeFn returns. The fields are union-style and only
// the relevant one is populated:
//
//   - Bool: gateway result for boolean bindings (when: "x == true").
//   - Value: gateway result for enum/string bindings (when: "x == story").
//   - Err: stop the run and surface this error to the user.
//
// Predicate evaluation reads Value (string-coerced) and Bool, in that order;
// see predicate.go.
type Outcome struct {
	Bool  bool
	Value string
	Err   error
}

// NodeFn is the body of a node. It receives the live Context and returns an
// Outcome. Routing decisions live in the edge list, not inside NodeFn — this
// keeps gateway nodes single-purpose ("compute one boolean") and lets the
// transitions test suite assert routing without mocking node bodies.
type NodeFn func(*Context) Outcome

// Node is a parsed YAML node bound to its NodeFn. Bindings are resolved at
// load time via the registries (gates, actions, agents).
type Node struct {
	ID    string
	Kind  NodeKind
	Fn    NodeFn
	Raw   RawNode // the original YAML record, retained for diagnostics
}

// Edge is a directed sequence flow between two nodes, optionally guarded by a
// predicate over the Context state map.
//
// When the predicate is empty (no `when:` clause in YAML), the edge is
// unconditionally true. Run picks the first edge whose predicate matches in
// declared order, so YAML authors should make `when:` clauses mutually
// exclusive (the BPMN exclusive-gateway rule).
type Edge struct {
	From      string
	To        string
	Predicate string // raw expression text from `when:`, "" means always-true
}

// Process is one named process definition (main, at_cycle, ct_subprocess, …),
// loaded from the YAML and bound to NodeFns.
type Process struct {
	Name           string
	Start          string
	Outputs        []string // optional BPMN-style data outputs published by the process
	Nodes          map[string]Node
	Edges          []Edge
	OutgoingByNode map[string][]Edge // index for nextEdge lookup
}

// Engine holds every loaded Process plus the registries needed to dispatch
// nodes. Run picks a process by name and walks it.
type Engine struct {
	Processes map[string]*Process

	// Registries — set during Load by binding string references in the YAML
	// (action:, agent:, binding:) to runtime functions.
	GateFn   func(name string) NodeFn
	ActionFn func(name string) NodeFn
	AgentFn  func(name string) NodeFn
}
