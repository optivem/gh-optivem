package statemachine

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// DispatchEvent is one observation from the cycle-test spy: a service_task or
// user_task fired, here are its identifying fields, what action/agent
// resolved (post ${…} expansion for templated user_tasks), and a snapshot of
// the live ctx.Params at dispatch.
//
// Gateways, call_activities, start/end events are excluded — they're routing
// scaffolding, not "steps the runner executes". Outcome / Outputs are
// excluded too: under noop mocks they carry no signal, and routing is
// already proven indirectly by the next event in the trail firing per the
// expected `when:` predicate.
type DispatchEvent struct {
	Process  string
	NodeID   string
	Kind     NodeKind
	Action   string            // service_task: the registered action name
	Agent    string            // user_task: the resolved agent name
	ParamsIn map[string]string // shallow copy of ctx.Params at dispatch time
}

// dispatchSpy returns a bound Engine plus a pointer to the event log. The
// caller is responsible for setting Processes["main"].Start and seeding ctx.
//
// Capture mechanism — three coordinated pieces:
//
//  1. Per-node decorator wraps every service_task / user_task NodeFn so it
//     appends a partially-filled DispatchEvent to *events* before calling
//     the inner function.
//  2. AgentFn / ActionFn registry mocks — invoked from inside the inner
//     function — back-fill the most recently appended event with the
//     resolved name and a shallow copy of ctx.Params.
//  3. GateFn echoes pre-seeded routing state, unchanged from the previous
//     spy shape.
//
// Ordering invariant: decorator appends → decorator calls inner → inner is
// the Bind-returned wrapper (a ServiceTask closure or, for templated
// user_tasks, the run.go wrapper that calls AgentFn(resolvedName) at
// dispatch) → that body back-fills events[len-1]. Single goroutine,
// deterministic, no synchronisation needed.
func dispatchSpy(t *testing.T) (*Engine, *[]DispatchEvent) {
	t.Helper()
	eng := loadSnapshot(t)

	events := &[]DispatchEvent{}

	eng.AgentFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			e := &(*events)[len(*events)-1]
			e.Agent = name
			e.ParamsIn = cloneParams(ctx.Params)
			return Outcome{}
		}
	}
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			e := &(*events)[len(*events)-1]
			e.Action = name
			e.ParamsIn = cloneParams(ctx.Params)
			return Outcome{}
		}
	}
	eng.GateFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			switch v := ctx.Get(name).(type) {
			case string:
				return Outcome{Value: v}
			case bool:
				return Outcome{Bool: v}
			}
			return Outcome{}
		}
	}
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	for _, process := range eng.Processes {
		procName := process.Name
		for id, node := range process.Nodes {
			if node.Kind != ServiceTask && node.Kind != UserTask {
				continue
			}
			proc, nid, kind, inner := procName, node.ID, node.Kind, node.Fn
			node.Fn = func(ctx *Context) Outcome {
				*events = append(*events, DispatchEvent{Process: proc, NodeID: nid, Kind: kind})
				return inner(ctx)
			}
			process.Nodes[id] = node
		}
	}
	return eng, events
}

// cloneParams returns a shallow copy of params. Necessary because
// wrapCallActivity reassigns ctx.Params per scope; a captured reference
// would be safe today but a copy is cheap and isolates events from any
// future mutation by actions/agents.
func cloneParams(params map[string]string) map[string]string {
	if params == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

// noParams is the empty map expected at dispatches whose enclosing scope is
// a call_activity that pushed no `params:` (e.g. main / github_intake /
// at_green_system).
func noParams() map[string]string { return map[string]string{} }

// commitFrom returns the params snapshot seen *inside* the shared commit
// sub-process when the COMMIT call_activity at the call site declares
// `params: {change_type: ${change_type}}`. The runtime's wrapCallActivity
// merges raw.Params on top of the parent scope without expanding ${…}, so
// the inner change_type is the literal placeholder string. Locking that in
// here means a future fix that adds ExpandParams to wrapCallActivity will
// surface as a clear diff in test output rather than silently changing
// runtime behavior.
func commitFrom(parent map[string]string) map[string]string {
	out := cloneParams(parent)
	out["change_type"] = "${change_type}"
	return out
}

// Per-call-site param baselines — one helper per distinct call_activity
// `params:` block in the YAML. Each represents the ctx.Params snapshot
// observed at the *first* dispatch inside the called sub-process.

// red_phase_cycle dispatched from at_cycle.AT_RED_TEST.
func atRedTestParams() map[string]string {
	return map[string]string{
		"agent":       "atdd-test",
		"phase_doc":   "docs/atdd/process/at-red-test.md",
		"phase_label": "AT - RED - TEST",
		"change_type": "AT - RED - TEST",
	}
}

// red_phase_cycle dispatched from at_cycle.AT_RED_DSL.
func atRedDslParams() map[string]string {
	return map[string]string{
		"agent":       "atdd-dsl",
		"phase_doc":   "docs/atdd/process/at-red-dsl.md",
		"phase_label": "AT - RED - DSL",
		"change_type": "AT - RED - DSL",
	}
}

// red_phase_cycle dispatched from ct_subprocess.CT_RED_TEST. CT_RED_TEST is
// the only red_phase_cycle call site that pushes verify_real_suite.
func ctRedTestParams() map[string]string {
	return map[string]string{
		"agent":             "atdd-test",
		"phase_doc":         "docs/atdd/process/ct-red-test.md",
		"phase_label":       "CT - RED - TEST",
		"change_type":       "CT - RED - TEST",
		"verify_real_suite": "<suite-contract-real>",
	}
}

// red_phase_cycle dispatched from ct_subprocess.CT_RED_DSL.
func ctRedDslParams() map[string]string {
	return map[string]string{
		"agent":       "atdd-dsl",
		"phase_doc":   "docs/atdd/process/ct-red-dsl.md",
		"phase_label": "CT - RED - DSL",
		"change_type": "CT - RED - DSL",
	}
}

// red_phase_cycle dispatched from ct_subprocess.CT_RED_EXTERNAL_DRIVER.
func ctRedExternalDriverParams() map[string]string {
	return map[string]string{
		"agent":       "atdd-driver",
		"phase_doc":   "docs/atdd/process/ct-red-external-driver.md",
		"phase_label": "CT - RED - EXTERNAL DRIVER",
		"change_type": "CT - RED - EXTERNAL DRIVER",
	}
}

// green_phase_cycle dispatched from at_green_system.AT_GREEN_BACKEND.
func atGreenBackendParams() map[string]string {
	return map[string]string{
		"agent":              "atdd-backend",
		"phase_doc":          "docs/atdd/process/at-green-system.md",
		"phase_label":        "AT - GREEN - SYSTEM (backend)",
		"suite":              "<acceptance-api>",
		"rebuild_before_run": "true",
	}
}

// green_phase_cycle dispatched from at_green_system.AT_GREEN_FRONTEND.
func atGreenFrontendParams() map[string]string {
	return map[string]string{
		"agent":              "atdd-frontend",
		"phase_doc":          "docs/atdd/process/at-green-system.md",
		"phase_label":        "AT - GREEN - SYSTEM (frontend)",
		"suite":              "<acceptance-ui>",
		"rebuild_before_run": "true",
	}
}

// commit dispatched from at_green_system.COMMIT — pushes a literal
// change_type, not a ${…} placeholder, so this is the *expanded* value.
func atGreenCommitParams() map[string]string {
	return map[string]string{"change_type": "AT - GREEN - SYSTEM"}
}

// structural_cycle dispatched from da_cycle.SYSTEM_INTERFACE_REDESIGN_CYCLE.
func systemInterfaceRedesignParams() map[string]string {
	return map[string]string{
		"change_type": "SYSTEM INTERFACE REDESIGN",
		"agent":       "atdd-task",
		"phase_doc":   "docs/atdd/process/system-interface-redesign.md",
		"subtype":     "system-interface-redesign",
	}
}

// formatEvent renders one DispatchEvent in a single line for human-readable
// failure diffs. Map keys are sorted so two equal maps render identically.
func formatEvent(e DispatchEvent) string {
	sel := ""
	switch e.Kind {
	case ServiceTask:
		sel = "action=" + e.Action
	case UserTask:
		sel = "agent=" + e.Agent
	}
	return fmt.Sprintf("%s.%s [%s] %s params=%s",
		e.Process, e.NodeID, kindLabel(e.Kind), sel, formatParams(e.ParamsIn))
}

func kindLabel(k NodeKind) string {
	switch k {
	case ServiceTask:
		return "service_task"
	case UserTask:
		return "user_task"
	}
	return fmt.Sprintf("kind%d", k)
}

func formatParams(p map[string]string) string {
	if len(p) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%q", k, p[k]))
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

// formatEvents joins one event per line, indented for readability inside a
// t.Errorf message.
func formatEvents(events []DispatchEvent) string {
	if len(events) == 0 {
		return "  (empty)"
	}
	lines := make([]string, len(events))
	for i, e := range events {
		lines[i] = "  " + formatEvent(e)
	}
	return strings.Join(lines, "\n")
}
