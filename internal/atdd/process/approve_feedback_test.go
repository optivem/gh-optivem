package process_test

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// Plan 20261110-1130, Step 3a: every approve call site whose "no" redoes a
// step passes `ask-for-changes: "true"`, so the operator's feedback reaches
// the redone agent. A redo site is one whose approval-outcome gateway routes
// "rejected" back to a step (a call activity, service task or user task);
// sites whose "rejected" ends the process (or hands it to a further gateway,
// as execute-agent's APPROVE_POST does) must not ask, or the approvals nested
// inside a redo would overwrite or clear the feedback before the redone
// agent reads it.
func TestApproveSites_RedoGatesAskForChanges(t *testing.T) {
	eng := loadSnapshot(t)
	var asking []string
	for _, proc := range eng.Processes {
		for id, node := range proc.Nodes {
			if node.Kind != statemachine.CallActivity || node.Raw.Process != "approve" {
				continue
			}
			redo := rejectedRoutesToStep(t, proc, id)
			asks := node.Raw.Params["ask-for-changes"] == "true"
			site := proc.ID + "/" + id
			switch {
			case redo && !asks:
				t.Errorf("%s: a \"no\" redoes a step but the site does not pass ask-for-changes: \"true\"", site)
			case !redo && asks:
				t.Errorf("%s: passes ask-for-changes but a \"no\" does not redo a step", site)
			}
			if asks {
				asking = append(asking, site)
			}
		}
	}
	sort.Strings(asking)
	want := []string{"characterize-layer/APPROVE_LAYER", "characterize/CONFIRM_CORE_USE_CASE"}
	if !slices.Equal(asking, want) {
		t.Errorf("approve sites asking for changes = %v, want %v", asking, want)
	}
}

// rejectedRoutesToStep reports whether the approval-outcome gateway after the
// approve call site at id routes "rejected" to a step node.
func rejectedRoutesToStep(t *testing.T, proc *statemachine.Process, id string) bool {
	t.Helper()
	out := proc.OutgoingByNode[id]
	if len(out) != 1 {
		t.Fatalf("%s/%s: approve site has %d outgoing edges, want 1", proc.ID, id, len(out))
	}
	gate, ok := proc.Nodes[out[0].To]
	if !ok || gate.Kind != statemachine.Gateway || gate.Raw.Binding != "approval-outcome" {
		t.Fatalf("%s/%s: approve site is not followed by an approval-outcome gateway", proc.ID, id)
	}
	for _, e := range proc.OutgoingByNode[out[0].To] {
		if !strings.Contains(e.Predicate, "rejected") {
			continue
		}
		switch proc.Nodes[e.To].Kind {
		case statemachine.CallActivity, statemachine.ServiceTask, statemachine.UserTask:
			return true
		}
	}
	return false
}
