package statemachine

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// DispatchEvent is one observation from the cycle-test spy: any BPMN node
// fired, here are its identifying fields, the kind-specific resolved data
// (action / agent / gate outcome / call target), and a snapshot of the live
// ctx.Params at dispatch.
//
// Every NodeKind is captured (start_event, end_event, service_task,
// user_task, gateway, call_activity). Routing scaffolding nodes are kept so
// the trail directly proves which gates fired with what outcome and which
// call_activity sites pushed which params, instead of inferring routing
// indirectly from the next mechanical task to fire. Outcome.Err / Outputs
// are not captured: under noop mocks they carry no signal.
type DispatchEvent struct {
	Process  string
	NodeID   string
	Kind     NodeKind
	ParamsIn map[string]string // shallow copy of ctx.Params at dispatch (parent scope for call_activity)

	// service_task: the registered action name.
	Action string
	// user_task: the resolved agent name (post ${…} expansion for templated agents).
	Agent string

	// gateway: the binding name from YAML and the resolved Outcome. Exactly
	// one of GateValue / GateBool is meaningful — GateValue carries
	// string-typed outcomes (e.g. "ok", "story"), GateBool carries
	// boolean-typed outcomes. GateIsBool disambiguates the zero case
	// (GateValue=="" with GateBool=false would otherwise be ambiguous).
	Binding    string
	GateValue  string
	GateBool   bool
	GateIsBool bool

	// call_activity: the target sub-process name and the literal raw.Params
	// declared at the call site (unexpanded — `${change_type}` stays literal
	// because the runtime's wrapCallActivity merges raw.Params without
	// ExpandParams). The merged effective scope inside the sub-process is
	// observable on the next event's ParamsIn.
	CallTarget string
	CallParams map[string]string
}

// dispatchSpy returns a bound Engine plus a pointer to the event log. The
// caller is responsible for setting Processes["main"].Start and seeding ctx.
//
// Capture mechanism — three coordinated pieces:
//
//  1. Per-node decorator wraps every node's NodeFn so it appends a
//     DispatchEvent (with kind-specific raw fields + ParamsIn snapshot)
//     before calling the inner function. For Gateway, after inner returns
//     the decorator back-fills the captured Outcome (Value or Bool).
//  2. AgentFn / ActionFn registry mocks — invoked from inside the inner
//     function — back-fill the most recently appended event with the
//     resolved name. (For service/user tasks the decorator's append is
//     always the latest event when the inner runs because those kinds
//     don't recurse into sub-events.)
//  3. GateFn echoes pre-seeded routing state.
//
// Ordering invariant: decorator appends → decorator captures index →
// decorator calls inner → inner is the Bind-returned wrapper (a ServiceTask
// closure, a UserTask wrapper that calls AgentFn at dispatch, wrapGateway
// for Gateway, or wrapCallActivity for CallActivity) → for ServiceTask /
// UserTask the body back-fills events[len-1]; for Gateway / CallActivity
// the decorator back-fills events[idx] *after* inner returns. CallActivity
// is the only kind whose inner appends sub-events, hence the captured idx
// rather than relying on len-1 after inner.
func dispatchSpy(t *testing.T) (*Engine, *[]DispatchEvent) {
	t.Helper()
	eng := loadSnapshot(t)

	events := &[]DispatchEvent{}

	eng.AgentFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			(*events)[len(*events)-1].Agent = name
			return Outcome{}
		}
	}
	eng.ActionFn = func(name string) NodeFn {
		return func(ctx *Context) Outcome {
			(*events)[len(*events)-1].Action = name
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
			proc, nid, kind, raw, inner := procName, node.ID, node.Kind, node.Raw, node.Fn
			node.Fn = func(ctx *Context) Outcome {
				ev := DispatchEvent{
					Process:  proc,
					NodeID:   nid,
					Kind:     kind,
					ParamsIn: cloneParams(ctx.Params),
				}
				switch kind {
				case Gateway:
					ev.Binding = raw.Binding
				case CallActivity:
					ev.CallTarget = raw.Process
					ev.CallParams = cloneParams(raw.Params)
				}
				*events = append(*events, ev)
				idx := len(*events) - 1
				out := inner(ctx)
				if kind == Gateway {
					if out.Value != "" {
						(*events)[idx].GateValue = out.Value
					} else {
						(*events)[idx].GateBool = out.Bool
						(*events)[idx].GateIsBool = true
					}
				}
				return out
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
// `params: {change_type: ${change_type}}`. wrapCallActivity expands raw.Params
// values against the parent scope before merging, so the inner change_type
// is the parent's resolved string (re-binding ${change_type} to itself is a
// no-op once expansion runs); all other parent keys carry through unchanged.
func commitFrom(parent map[string]string) map[string]string {
	return cloneParams(parent)
}

// commitFromTemplateParams is the literal raw.Params declared at every
// COMMIT call_activity inside structural_cycle / red_phase_cycle —
// `{change_type: ${change_type}}` (unexpanded). Used to assert the
// CallParams field on those CallActivity events.
func commitFromTemplateParams() map[string]string {
	return map[string]string{"change_type": "${change_type}"}
}

// compileFromStructTemplateParams is the literal raw.Params declared at
// structural_cycle.COMPILE — `{compile_action: compile_all, fix_agent:
// fix-verify, phase_doc: ${phase_doc}}` (unexpanded). STRUCT
// hardcodes the compile tier and the fix agent because there is no
// parent cycle context to forward.
func compileFromStructTemplateParams() map[string]string {
	return map[string]string{
		"compile_action": "compile_all",
		"fix_agent":      "fix-verify",
		"phase_doc":      "${phase_doc}",
	}
}

// compileFromCycleTemplateParams is the literal raw.Params declared at
// red_phase_cycle.COMPILE and green_phase_cycle.COMPILE — both forward
// the parent cycle's compile_action and agent verbatim via `${...}`.
func compileFromCycleTemplateParams() map[string]string {
	return map[string]string{
		"compile_action": "${compile_action}",
		"fix_agent":      "${agent}",
		"phase_doc":      "${phase_doc}",
	}
}

// compileFromStruct returns the params snapshot seen *inside* the shared
// compile sub-process when STRUCT invokes it. STRUCT's parent scope does
// not carry compile_action or fix_agent, so the call_activity introduces
// them as new keys (compile_all + fix-verify). phase_doc rebinds to
// itself (no-op).
func compileFromStruct(parent map[string]string) map[string]string {
	out := cloneParams(parent)
	out["compile_action"] = "compile_all"
	out["fix_agent"] = "fix-verify"
	return out
}

// compileFromCycle returns the params snapshot seen *inside* the shared
// compile sub-process when red_phase_cycle / green_phase_cycle invokes
// it. Both forward compile_action and phase_doc verbatim (no-op rebind
// since parent already has the same keys/values) and introduce fix_agent
// = parent's `agent` (the cycle's WRITE agent doubles as the fix agent
// because compile-fail in RED/GREEN is a re-dispatch of the same
// creative step).
func compileFromCycle(parent map[string]string) map[string]string {
	out := cloneParams(parent)
	out["fix_agent"] = parent["agent"]
	return out
}

// Per-call-site param baselines — one helper per distinct call_activity
// `params:` block in the YAML. Each represents the ctx.Params snapshot
// observed at the *first* dispatch inside the called sub-process — and,
// equivalently for these call sites (parent scope is empty for all of
// them), the literal raw.Params declared at the call site that's asserted
// on the CallActivity event's CallParams field.

// red_phase_cycle dispatched from at_cycle.AT_RED_TEST.
func atRedTestParams() map[string]string {
	return map[string]string{
		"agent":          "at-red-test",
		"phase_doc":      "docs/atdd/process/change/behavior/at-red-test.md",
		"phase_label":    "AT - RED - TEST",
		"change_type":    "AT - RED - TEST",
		"compile_action": "compile_system_tests",
	}
}

// red_phase_cycle dispatched from at_cycle.AT_RED_DSL.
func atRedDslParams() map[string]string {
	return map[string]string{
		"agent":          "at-red-dsl",
		"phase_doc":      "docs/atdd/process/change/behavior/at-red-dsl.md",
		"phase_label":    "AT - RED - DSL",
		"change_type":    "AT - RED - DSL",
		"compile_action": "compile_system_tests",
	}
}

// red_phase_cycle dispatched from ct_subprocess.CT_RED_TEST. CT_RED_TEST is
// the only red_phase_cycle call site that pushes verify_real_suite.
func ctRedTestParams() map[string]string {
	return map[string]string{
		"agent":             "ct-red-test",
		"phase_doc":         "docs/atdd/process/change/behavior/ct-red-test.md",
		"phase_label":       "CT - RED - TEST",
		"change_type":       "CT - RED - TEST",
		"verify_real_suite": "<suite-contract-real>",
		"compile_action":    "compile_system_tests",
	}
}

// red_phase_cycle dispatched from ct_subprocess.CT_RED_DSL.
func ctRedDslParams() map[string]string {
	return map[string]string{
		"agent":          "ct-red-dsl",
		"phase_doc":      "docs/atdd/process/change/behavior/ct-red-dsl.md",
		"phase_label":    "CT - RED - DSL",
		"change_type":    "CT - RED - DSL",
		"compile_action": "compile_system_tests",
	}
}

// red_phase_cycle dispatched from ct_subprocess.CT_RED_EXTERNAL_SYSTEM_DRIVER.
func ctRedExternalDriverParams() map[string]string {
	return map[string]string{
		"agent":          "ct-red-external-system-driver",
		"phase_doc":      "docs/atdd/process/change/behavior/ct-red-external-system-driver.md",
		"phase_label":    "CT - RED - EXTERNAL SYSTEM DRIVER",
		"change_type":    "CT - RED - EXTERNAL SYSTEM DRIVER",
		"compile_action": "compile_system_tests",
	}
}

// green_phase_cycle dispatched from at_green_system.AT_GREEN_BACKEND.
func atGreenBackendParams() map[string]string {
	return map[string]string{
		"agent":              "at-green-system-backend",
		"phase_doc":          "docs/atdd/process/change/behavior/at-green-system.md",
		"phase_label":        "AT - GREEN - SYSTEM (backend)",
		"suite":              "<acceptance-api>",
		"rebuild_before_run": "true",
		"compile_action":     "compile_system",
	}
}

// green_phase_cycle dispatched from at_green_system.AT_GREEN_FRONTEND.
func atGreenFrontendParams() map[string]string {
	return map[string]string{
		"agent":              "at-green-system-frontend",
		"phase_doc":          "docs/atdd/process/change/behavior/at-green-system.md",
		"phase_label":        "AT - GREEN - SYSTEM (frontend)",
		"suite":              "<acceptance-ui>",
		"rebuild_before_run": "true",
		"compile_action":     "compile_system",
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
		"agent":       "task-system-interface-redesign",
		"phase_doc":   "docs/atdd/process/change/structure/system-interface-redesign.md",
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
	case Gateway:
		if e.GateIsBool {
			sel = fmt.Sprintf("binding=%s bool=%t", e.Binding, e.GateBool)
		} else {
			sel = fmt.Sprintf("binding=%s value=%s", e.Binding, e.GateValue)
		}
	case CallActivity:
		sel = fmt.Sprintf("target=%s call_params=%s", e.CallTarget, formatParams(e.CallParams))
	}
	if sel == "" {
		return fmt.Sprintf("%s.%s [%s] params=%s", e.Process, e.NodeID, kindLabel(e.Kind), formatParams(e.ParamsIn))
	}
	return fmt.Sprintf("%s.%s [%s] %s params=%s",
		e.Process, e.NodeID, kindLabel(e.Kind), sel, formatParams(e.ParamsIn))
}

func kindLabel(k NodeKind) string {
	switch k {
	case StartEvent:
		return "start_event"
	case EndEvent:
		return "end_event"
	case ServiceTask:
		return "service_task"
	case UserTask:
		return "user_task"
	case Gateway:
		return "gateway"
	case CallActivity:
		return "call_activity"
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
