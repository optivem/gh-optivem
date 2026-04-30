package statemachine

import (
	"fmt"
	"strings"
)

// Bind wires NodeFn values into every Node by consulting the engine's
// registries:
//
//   - service_task → ActionFn(action)
//   - user_task    → AgentFn(agent)
//   - gateway      → GateFn(binding); the bound function must Set(binding, …)
//                    on the Context so downstream `when:` predicates can read
//                    the result.
//   - call_activity → built-in dispatch into the named sub-flow
//   - start_event / end_event → built-in no-ops; routing is decided entirely
//                    by `when:` predicates against the initial Context state.
//
// Bind must run after every registry has been populated; calling Run before
// Bind will dereference nil functions and panic.
func (e *Engine) Bind() error {
	for _, flow := range e.Flows {
		for id, node := range flow.Nodes {
			fn, err := e.resolve(flow, node)
			if err != nil {
				return fmt.Errorf("flow %q node %q: %w", flow.Name, id, err)
			}
			node.Fn = fn
			flow.Nodes[id] = node
		}
	}
	return nil
}

// resolve picks the right NodeFn for one node based on its kind and the
// engine's registries.
func (e *Engine) resolve(flow *Flow, node Node) (NodeFn, error) {
	switch node.Kind {
	case StartEvent, EndEvent:
		return func(ctx *Context) Outcome { return Outcome{} }, nil
	case ServiceTask:
		if e.ActionFn == nil {
			return nil, fmt.Errorf("ActionFn registry is nil but node is service_task")
		}
		fn := e.ActionFn(node.Raw.Action)
		if fn == nil {
			return nil, fmt.Errorf("service_task action %q not registered", node.Raw.Action)
		}
		return fn, nil
	case UserTask:
		if e.AgentFn == nil {
			return nil, fmt.Errorf("AgentFn registry is nil but node is user_task")
		}
		fn := e.AgentFn(node.Raw.Agent)
		if fn == nil {
			return nil, fmt.Errorf("user_task agent %q not registered", node.Raw.Agent)
		}
		return fn, nil
	case Gateway:
		if e.GateFn == nil {
			return nil, fmt.Errorf("GateFn registry is nil but node is gateway")
		}
		fn := e.GateFn(node.Raw.Binding)
		if fn == nil {
			return nil, fmt.Errorf("gateway binding %q not registered", node.Raw.Binding)
		}
		return e.wrapGateway(node.Raw.Binding, fn), nil
	case CallActivity:
		return e.wrapCallActivity(node.Raw), nil
	default:
		return nil, fmt.Errorf("unhandled NodeKind %d", node.Kind)
	}
}

// wrapGateway records the bound function's Outcome under the binding name in
// the Context state map, so later `when:` clauses (and gates that depend on
// upstream gate decisions) can read it back. Gates SHOULD return an Outcome
// whose Value or Bool is meaningful; either is recorded.
func (e *Engine) wrapGateway(binding string, fn NodeFn) NodeFn {
	return func(ctx *Context) Outcome {
		out := fn(ctx)
		if out.Err != nil {
			return out
		}
		switch {
		case out.Value != "":
			ctx.Set(binding, out.Value)
		default:
			ctx.Set(binding, out.Bool)
		}
		return out
	}
}

// wrapCallActivity returns a NodeFn that runs the named sub-flow to
// completion. Params from the call site are pushed onto the Context and
// popped on return, so the called flow sees only its own substitutions.
func (e *Engine) wrapCallActivity(raw RawNode) NodeFn {
	return func(ctx *Context) Outcome {
		sub, ok := e.Flows[raw.Flow]
		if !ok {
			return Outcome{Err: fmt.Errorf("call_activity references unknown flow %q", raw.Flow)}
		}
		// Push params; restore on exit. Caller-scoped state is preserved so
		// gateway results from outer flows remain visible to inner gateways
		// when they share binding names.
		prev := ctx.Params
		merged := make(map[string]string, len(prev)+len(raw.Params))
		for k, v := range prev {
			merged[k] = v
		}
		for k, v := range raw.Params {
			merged[k] = v
		}
		ctx.Params = merged
		defer func() { ctx.Params = prev }()

		if err := e.RunFlow(sub.Name, ctx); err != nil {
			return Outcome{Err: err}
		}
		return Outcome{}
	}
}

// RunFlow walks one flow from its start node to an end node. It uses
// nextEdge to pick the outgoing edge whose predicate matches the current
// state, and stops on the first node with no outgoing edges (treating that
// as terminal — covers both end_event and any node placed as a flow tail).
//
// Nodes are dispatched after expandParams substitutes ${name} occurrences in
// the raw node fields the body may want to read (agent, phase_doc, etc.).
// The NodeFn itself is bound at load time and does not see the substitutions
// directly — actions/gates/agents that need params read them via the live
// Context.Params map.
func (e *Engine) RunFlow(name string, ctx *Context) error {
	flow, ok := e.Flows[name]
	if !ok {
		return fmt.Errorf("unknown flow %q", name)
	}
	cur := flow.Start
	for cur != "" {
		node, ok := flow.Nodes[cur]
		if !ok {
			return fmt.Errorf("flow %q: dangling reference to node %q", name, cur)
		}
		if node.Fn == nil {
			return fmt.Errorf("flow %q node %q: NodeFn not bound (call Bind first)", name, cur)
		}
		out := node.Fn(ctx)
		if out.Err != nil {
			return fmt.Errorf("flow %q node %q: %w", name, cur, out.Err)
		}
		if node.Kind == EndEvent {
			return nil
		}
		next, err := e.nextEdge(flow, cur, ctx)
		if err != nil {
			return fmt.Errorf("flow %q after node %q: %w", name, cur, err)
		}
		if next == "" {
			return nil // terminal node with no outgoing edges
		}
		cur = next
	}
	return nil
}

// NextEdge is the public counterpart to nextEdge: given a flow name, source
// node ID, and Context, return the node ID Run would advance to next.
// Used by the `gh optivem atdd debug next-phase` diagnostic helper.
//
// Errors mirror nextEdge: an unknown flow / node returns a descriptive
// error; a node with no outgoing edges returns "" with nil error (terminal).
func (e *Engine) NextEdge(flowName, fromNode string, ctx *Context) (string, error) {
	flow, ok := e.Flows[flowName]
	if !ok {
		return "", fmt.Errorf("unknown flow %q", flowName)
	}
	if _, ok := flow.Nodes[fromNode]; !ok {
		return "", fmt.Errorf("node %q not in flow %q", fromNode, flowName)
	}
	return e.nextEdge(flow, fromNode, ctx)
}

// nextEdge picks the first outgoing edge from `from` whose predicate matches
// the current Context state. Returns "" if there are no outgoing edges
// (terminal node). Returns an error if multiple guarded edges all evaluate
// false — that's an authoring bug in the YAML (gateway should be exhaustive).
func (e *Engine) nextEdge(flow *Flow, from string, ctx *Context) (string, error) {
	edges := flow.OutgoingByNode[from]
	if len(edges) == 0 {
		return "", nil
	}
	var lastErr error
	for _, edge := range edges {
		ok, err := evalPredicate(edge.Predicate, ctx)
		if err != nil {
			lastErr = err
			continue
		}
		if ok {
			return edge.To, nil
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("no outgoing edge predicate matched current state")
}

// expandParams substitutes ${name} occurrences in the input string using the
// given params map. Used by diagnostic helpers and the diagram generator;
// the runtime itself doesn't need to mutate node fields because NodeFns read
// params via Context.Params at dispatch time.
func expandParams(s string, params map[string]string) string {
	for k, v := range params {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	return s
}
