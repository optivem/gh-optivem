// Package statemachine is a generic, BPMN-shaped process engine: a hand-coded
// graph traversal that walks a process definition node-by-node, dispatching
// service tasks, user-task agents, and gateways through pluggable registries.
// It embeds no concrete process — the ATDD pipeline is one definition built on
// top of this engine (see internal/atdd/process), not part of it.
//
// # Bring your own BPMN (the process swap point)
//
// LoadBytes is the reuse entry point: hand it any YAML process definition and
// it returns an *Engine with the graph parsed but not yet bound. The flow
// contract is four steps:
//
//	eng, err := statemachine.LoadBytes(yaml) // parse the graph
//	eng.ActionFn = actionRegistry.Lookup     // plug in your service-task bodies
//	eng.AgentFn  = agentRegistry.Lookup      // ... your user-task dispatchers
//	eng.GateFn   = gateRegistry.Lookup       // ... your gateway deciders
//	eng.Bind()                               // resolve every node to a NodeFn
//	eng.RunProcess(name, statemachine.NewContext()) // drive to an end event
//
// Bind errors loudly on any action/agent/gateway the YAML references but no
// registry provides, so a misbound definition fails before the run, not midway
// through it. The engine knows nothing about the domain: node bodies are the
// closures your registries return, and routing lives in the YAML's `when:` edge
// predicates (see predicate.go), never in engine Go. A definition can use only
// service-tasks and gateways — no agent layer at all — and still run.
//
// reuse_process_test.go is the worked example: a non-ATDD document-publishing
// flow (draft → review gateway → publish/archive) driven to completion from an
// external package with nothing but LoadBytes, three closures, and RunProcess —
// proving the engine needs no change to host a second process.
//
// # The two swap points
//
// This package owns the *process* swap point (bring your own BPMN). The
// companion *agent-set* swap point — bind an alternate set of agent prompts to
// an unchanged process — lives in the agents package
// (internal/atdd/runtime/agents) via AgentSet / NewAgentSetFS. Together they
// let a reusing caller bring their own process, their own agents, or both, and
// run them on this engine.
//
// # Types
//
// The engine has three job-shaped types: Outcome, NodeFn, and Process.
// Everything else (gates, actions, agents) plugs in through the registries
// above.
package statemachine
