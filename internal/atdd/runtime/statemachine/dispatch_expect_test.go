package statemachine

import (
	"reflect"
	"testing"
)

// expectDispatch is a fluent builder over the events captured by dispatchSpy.
// process(name, params) opens a scope: subsequent serviceTask/userTask calls
// inherit both the process name and the params snapshot, mirroring how
// call_activity push/pop bind a single Params map for the duration of a
// sub-process's frame. Re-entering a process (e.g. main → github_intake →
// … → main) is expressed by calling process(name, params) again.
// assert(t) runs reflect.DeepEqual against the actual log with formatEvents
// diffs on mismatch.
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
