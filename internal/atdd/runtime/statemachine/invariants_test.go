package statemachine

import (
	"strings"
	"testing"
)

// The embedded process-flow snapshot must satisfy every graph invariant. This
// is the quantified counterpart to the per-site assertions in
// transitions_test.go: instead of naming the four commit / fix / halt sites by
// hand, CheckInvariants walks the whole graph, so a new rule-violating site
// fails here with no test edit (plan 20260604-1644 D3).
//
// Uses the unbound Engine (loadSnapshot, no Bind()) — the rules read the static
// graph only and never dispatch a NodeFn.
func TestGraphInvariants_SnapshotIsClean(t *testing.T) {
	eng := loadSnapshot(t)
	violations := CheckInvariants(eng)
	if len(violations) == 0 {
		return
	}
	var b strings.Builder
	for _, v := range violations {
		b.WriteString("\n  ")
		b.WriteString(v.String())
	}
	t.Errorf("embedded snapshot violates %d graph invariant(s):%s", len(violations), b.String())
}
