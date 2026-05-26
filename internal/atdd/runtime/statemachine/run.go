package statemachine

import (
	"fmt"
	"strings"
)

// Bind wires NodeFn values into every Node by consulting the engine's
// registries:
//
//   - service-task → ActionFn(action)
//   - user-task    → AgentFn(agent)
//   - gateway      → GateFn(binding); the bound function must Set(binding, …)
//                    on the Context so downstream `when:` predicates can read
//                    the result.
//   - call-activity → built-in dispatch into the named sub-process
//   - start-event / end-event → built-in no-ops; routing is decided entirely
//                    by `when:` predicates against the initial Context state.
//
// Bind must run after every registry has been populated; calling Run before
// Bind will dereference nil functions and panic.
func (e *Engine) Bind() error {
	var errs []string
	for _, process := range e.Processes {
		for id, node := range process.Nodes {
			fn, err := e.resolve(node)
			if err != nil {
				errs = append(errs, fmt.Sprintf("process %q node %q: %v", process.ID, id, err))
				continue
			}
			node.Fn = fn
			process.Nodes[id] = node
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d bind error(s):\n  - %s", len(errs), strings.Join(errs, "\n  - "))
	}
	return nil
}

// resolve picks the right NodeFn for one node based on its kind and the
// engine's registries.
func (e *Engine) resolve(node Node) (NodeFn, error) {
	switch node.Kind {
	case StartEvent, EndEvent, ErrorEndEvent:
		return func(ctx *Context) Outcome { return Outcome{} }, nil
	case ServiceTask:
		if e.ActionFn == nil {
			return nil, fmt.Errorf("ActionFn registry is nil but node is service-task")
		}
		// Templated action names (e.g. ${compile_action} in red_phase_cycle)
		// are resolved at dispatch time once Context.Params is set by the
		// calling call-activity. Bind validates only static names.
		if strings.Contains(node.Raw.Action, "${") {
			ref := node.Raw.Action
			lookup := e.ActionFn
			return func(ctx *Context) Outcome {
				name := ExpandParams(ref, ctx.Params, ctx.State)
				fn := lookup(name)
				if fn == nil {
					return Outcome{Err: fmt.Errorf("service-task action %q (from template %q) not registered", name, ref)}
				}
				return fn(ctx)
			}, nil
		}
		fn := e.ActionFn(node.Raw.Action)
		if fn == nil {
			return nil, fmt.Errorf("service-task action %q not registered", node.Raw.Action)
		}
		return fn, nil
	case UserTask:
		if e.AgentFn == nil {
			return nil, fmt.Errorf("AgentFn registry is nil but node is user-task")
		}
		// Templated agent names (e.g. ${agent} in structural_cycle) are
		// resolved at dispatch time once Context.Params is set by the calling
		// call-activity. Bind validates only static names.
		if strings.Contains(node.Raw.Agent, "${") {
			ref := node.Raw.Agent
			lookup := e.AgentFn
			return func(ctx *Context) Outcome {
				name := ExpandParams(ref, ctx.Params, ctx.State)
				fn := lookup(name)
				if fn == nil {
					return Outcome{Err: fmt.Errorf("user-task agent %q (from template %q) not registered", name, ref)}
				}
				return fn(ctx)
			}, nil
		}
		fn := e.AgentFn(node.Raw.Agent)
		if fn == nil {
			return nil, fmt.Errorf("user-task agent %q not registered", node.Raw.Agent)
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

// wrapCallActivity returns a NodeFn that runs the named sub-process to
// completion. Params from the call site are pushed onto the Context and
// popped on return, so the called process sees only its own substitutions.
//
// Call-site param values are template-expanded against the parent scope
// before being pushed, so a nested call-activity declaring
// `params: {change_type: ${change_type}}` propagates the parent's resolved
// value rather than the literal placeholder. ExpandParams is idempotent on
// strings without ${…} placeholders, so leaf values pass through unchanged.
//
// The `process:` field itself is also template-expanded against the caller's
// scope, mirroring the pattern already used for `action:` / `agent:` (run.go
// lines above) — so a call site like `process: ${action}` resolves the
// sub-process name at dispatch time from caller-supplied params. Static names
// (the common case) pass through unchanged because ExpandParams is idempotent.
func (e *Engine) wrapCallActivity(raw RawNode) NodeFn {
	return func(ctx *Context) Outcome {
		processName := ExpandParams(raw.Process, ctx.Params, ctx.State)
		sub, ok := e.Processes[processName]
		if !ok {
			if processName != raw.Process {
				return Outcome{Err: fmt.Errorf("call-activity references unknown process %q (from template %q)", processName, raw.Process)}
			}
			return Outcome{Err: fmt.Errorf("call-activity references unknown process %q", processName)}
		}
		// Push params; restore on exit. Caller-scoped state is preserved so
		// gateway results from outer processes remain visible to inner gateways
		// when they share binding names.
		prev := ctx.Params
		merged := make(map[string]string, len(prev)+len(raw.Params))
		for k, v := range prev {
			merged[k] = v
		}
		for k, v := range raw.Params {
			merged[k] = ExpandParams(v, prev, ctx.State)
		}
		ctx.Params = merged
		defer func() { ctx.Params = prev }()

		if err := e.runProcess(sub, ctx); err != nil {
			return Outcome{Err: err}
		}
		return Outcome{}
	}
}

// maxDispatchesPerProcess caps how many node dispatches RunProcess will
// perform in a single invocation before failing fast. Loopback edges in
// process-flow.yaml (e.g. STOP_FLAG_UNSET → AT_RED_DSL, STOP_SCOPE_VIOLATION
// → WRITE) are legitimate, but a gate whose deciding state isn't reset
// between iterations turns the loop infinite. Without a cap, the test
// harness's per-dispatch event slice grows unbounded and consumes 20 GB+ of
// RAM before being killed; production agents would just hang. 10000 is
// orders of magnitude above any legitimate single-process trail (the
// longest current cycle dispatches a few dozen nodes), so a breach is
// definitively an authoring bug rather than a deep-but-valid trail.
const maxDispatchesPerProcess = 10000

// RunProcess walks one process from its start node to an end node, identified
// by its kebab-case ID (the YAML map key under `processes:`). It uses
// nextEdge to pick the outgoing edge whose predicate matches the current
// state, and stops on the first node with no outgoing edges (treating that
// as terminal — covers both end-event and any node placed as a process tail).
//
// Nodes are dispatched after expandParams substitutes ${name} occurrences in
// the raw node fields the body may want to read (agent, etc.).
// The NodeFn itself is bound at load time and does not see the substitutions
// directly — actions/gates/agents that need params read them via the live
// Context.Params map.
//
// RunProcess fails fast on suspected dispatch loops via maxDispatchesPerProcess
// — see that constant for the rationale.
//
// External callers (driver, tests) enter through this name-keyed façade.
// Internal callers (`wrapCallActivity`) already hold the resolved *Process
// and use the runProcess helper to skip the redundant map lookup that
// previously invited name/ID confusion at the call site.
func (e *Engine) RunProcess(name string, ctx *Context) error {
	process, ok := e.Processes[name]
	if !ok {
		return fmt.Errorf("unknown process %q", name)
	}
	return e.runProcess(process, ctx)
}

func (e *Engine) runProcess(process *Process, ctx *Context) error {
	cur := process.Start
	dispatches := 0
	for cur != "" {
		if dispatches >= maxDispatchesPerProcess {
			return fmt.Errorf("process %q: exceeded %d dispatches (suspected loopback with unchanging gate state; last node %q)", process.ID, maxDispatchesPerProcess, cur)
		}
		dispatches++
		node, ok := process.Nodes[cur]
		if !ok {
			return fmt.Errorf("process %q: dangling reference to node %q", process.ID, cur)
		}
		if node.Fn == nil {
			return fmt.Errorf("process %q node %q: NodeFn not bound (call Bind first)", process.ID, cur)
		}
		out := node.Fn(ctx)
		if out.Err != nil {
			return fmt.Errorf("process %q node %q: %w", process.ID, cur, out.Err)
		}
		if node.Kind == EndEvent {
			return nil
		}
		if node.Kind == ErrorEndEvent {
			return fmt.Errorf("process %q reached error end event %q: %s", process.ID, cur, node.Raw.Name)
		}
		next, err := e.nextEdge(process, cur, ctx)
		if err != nil {
			return fmt.Errorf("process %q after node %q: %w", process.ID, cur, err)
		}
		if next == "" {
			return nil // terminal node with no outgoing edges
		}
		cur = next
	}
	return nil
}

// NextEdge is the public counterpart to nextEdge: given a process name, source
// node ID, and Context, return the node ID Run would advance to next.
//
// Errors mirror nextEdge: an unknown process / node returns a descriptive
// error; a node with no outgoing edges returns "" with nil error (terminal).
func (e *Engine) NextEdge(processName, fromNode string, ctx *Context) (string, error) {
	process, ok := e.Processes[processName]
	if !ok {
		return "", fmt.Errorf("unknown process %q", processName)
	}
	if _, ok := process.Nodes[fromNode]; !ok {
		return "", fmt.Errorf("node %q not in process %q", fromNode, processName)
	}
	return e.nextEdge(process, fromNode, ctx)
}

// nextEdge picks the first outgoing edge from `from` whose predicate matches
// the current Context state. Returns "" if there are no outgoing edges
// (terminal node). Returns an error if multiple guarded edges all evaluate
// false — that's an authoring bug in the YAML (gateway should be exhaustive).
func (e *Engine) nextEdge(process *Process, from string, ctx *Context) (string, error) {
	edges := process.OutgoingByNode[from]
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

// ExpandParams substitutes ${name} occurrences in the input string by
// looking each key up in a two-level scope chain: caller params first,
// then run-scoped state for keys not present in params. The state-fallback
// path is the bridge for binding-written values (failure-kind, test-outcome,
// command-line, etc.) that downstream templates want to consume without
// requiring every caller to declare a passthrough param at each
// call-activity site (plan 20260526-1530, decision (b) with a small
// extension). Params win on collision so call-site overrides remain
// authoritative.
//
// State values are coerced to string with the same best-effort rules as
// Context.GetString: strings pass through, bools become "true"/"false",
// every other type renders via fmt.Sprint. A nil state map disables the
// fallback (legacy callers and tests that don't carry state behave
// exactly like the pre-fallback signature).
//
// Used by the engine to resolve templated agent / process / param names
// at dispatch time, and by the driver to render user-facing strings
// (banners, phase docs) with the same substitutions the engine sees.
// Idempotent on already-substituted strings (no ${…} placeholders →
// identity); a nil params map AND a nil state map returns s unchanged.
func ExpandParams(s string, params map[string]string, state map[string]any) string {
	for k, v := range params {
		s = strings.ReplaceAll(s, "${"+k+"}", v)
	}
	for k, v := range state {
		if _, override := params[k]; override {
			// Params win on collision — already substituted above.
			continue
		}
		s = strings.ReplaceAll(s, "${"+k+"}", coerceStateValue(v))
	}
	return s
}

// coerceStateValue stringifies a state value with the same best-effort
// rules as Context.GetString. Lives next to ExpandParams so the
// substitution layer and the predicate-evaluation layer agree on what
// "value X under key Y" renders as.
func coerceStateValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}
