package statemachine

import (
	"reflect"
	"testing"
)

// expectDispatch is a fluent builder over the events captured by dispatchSpy.
// process(name, params) opens a scope: subsequent task / gateway / event
// methods inherit both the process name and the params snapshot, mirroring
// how call_activity push/pop bind a single Params map for the duration of a
// sub-process's frame. Re-entering a process (e.g. main → github_intake →
// … → main) is expressed by calling process(name, params) again. The
// process(...) call itself does not append an event — call_activity events
// are appended explicitly via callActivity(...) at the call site, *before*
// the process() scope switch into the called sub-process. assert(t) runs
// reflect.DeepEqual against the actual log with formatEvents diffs on
// mismatch.
type expectDispatch struct {
	actual    *[]DispatchEvent
	want      []DispatchEvent
	proc      string
	params    map[string]string
	procReady bool
}

func expect(actual *[]DispatchEvent) *expectDispatch {
	return &expectDispatch{actual: actual}
}

func (e *expectDispatch) process(name string, params map[string]string) *expectDispatch {
	e.proc = name
	e.params = params
	e.procReady = true
	return e
}

func (e *expectDispatch) serviceTask(nodeID, action string) *expectDispatch {
	e.want = append(e.want, DispatchEvent{
		Process:  e.proc,
		NodeID:   nodeID,
		Kind:     ServiceTask,
		Action:   action,
		ParamsIn: e.params,
	})
	return e
}

func (e *expectDispatch) userTask(nodeID, agent string) *expectDispatch {
	e.want = append(e.want, DispatchEvent{
		Process:  e.proc,
		NodeID:   nodeID,
		Kind:     UserTask,
		Agent:    agent,
		ParamsIn: e.params,
	})
	return e
}

// gateway asserts a gateway dispatch with its binding name and the resolved
// Outcome. outcome must be string (for enum/string-typed bindings like
// `change_type`) or bool (for predicate-typed bindings like `compile_ok`).
// The spy distinguishes the two via GateIsBool, so the false-string and
// false-bool cases stay distinguishable in failure diffs.
func (e *expectDispatch) gateway(nodeID, binding string, outcome any) *expectDispatch {
	ev := DispatchEvent{
		Process:  e.proc,
		NodeID:   nodeID,
		Kind:     Gateway,
		Binding:  binding,
		ParamsIn: e.params,
	}
	switch v := outcome.(type) {
	case string:
		ev.GateValue = v
	case bool:
		ev.GateBool = v
		ev.GateIsBool = true
	default:
		panic("expectDispatch.gateway: outcome must be string or bool")
	}
	e.want = append(e.want, ev)
	return e
}

// callActivity asserts a call_activity dispatch in the current scope. target
// is the called sub-process name; callParams is the literal raw.Params block
// declared at the call site (unexpanded — `${change_type}` stays literal).
// ParamsIn on the event is the *parent* scope at the call (the current
// builder scope); the merged effective scope inside the sub-process is
// asserted on the next event after a process(...) scope switch.
func (e *expectDispatch) callActivity(nodeID, target string, callParams map[string]string) *expectDispatch {
	e.want = append(e.want, DispatchEvent{
		Process:    e.proc,
		NodeID:     nodeID,
		Kind:       CallActivity,
		CallTarget: target,
		CallParams: callParams,
		ParamsIn:   e.params,
	})
	return e
}

func (e *expectDispatch) startEvent(nodeID string) *expectDispatch {
	e.want = append(e.want, DispatchEvent{
		Process:  e.proc,
		NodeID:   nodeID,
		Kind:     StartEvent,
		ParamsIn: e.params,
	})
	return e
}

func (e *expectDispatch) endEvent(nodeID string) *expectDispatch {
	e.want = append(e.want, DispatchEvent{
		Process:  e.proc,
		NodeID:   nodeID,
		Kind:     EndEvent,
		ParamsIn: e.params,
	})
	return e
}

// then is a no-op separator. It exists so callers can visually group scopes
// in a long chain — gofmt strips blank lines inside chained calls, so a bare
// method call is the only spacer that survives formatting.
func (e *expectDispatch) then() *expectDispatch { return e }

func (e *expectDispatch) assert(t *testing.T) {
	t.Helper()
	if !e.procReady {
		t.Fatalf("expectDispatch.assert: no process() scope was set; the builder needs at least one process() call before any task")
	}
	if !reflect.DeepEqual(*e.actual, e.want) {
		t.Errorf("dispatch events:\n got=\n%s\nwant=\n%s",
			formatEvents(*e.actual), formatEvents(e.want))
	}
}
