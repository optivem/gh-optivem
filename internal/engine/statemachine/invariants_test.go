package statemachine

import (
	"strings"
	"testing"
)

// The embedded process-flow snapshot must satisfy every graph invariant. This
// is the quantified counterpart to the per-site assertions in
// transitions_test.go: instead of naming the four commit / fix / halt sites by
// hand, CheckInvariants walks the whole graph, so a new rule-violating site
// fails here with no test edit.
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

// The post-green carve-out of commit-is-verified only excuses an
// apply-ac-policy (or remove-wip-gate) predecessor that is itself reached from
// implement-and-verify-system; a bare apply → commit chain still violates.
func TestGraphInvariants_AcPolicyCommitNeedsVerifiedSystem(t *testing.T) {
	const stub = `
  %s:
    name: "stub"
    start: S
    nodes:
      - {id: S, type: start-event, name: S}
      - {id: E, type: end-event, name: E}
    sequence-flows:
      - {from: S, to: E}
`
	build := func(head string) string {
		y := "processes:\n  p:\n    name: \"p\"\n    start: " + head + "\n    nodes:\n" +
			"      - {id: SYS, type: call-activity, process: implement-and-verify-system, name: SYS}\n" +
			"      - {id: REC, type: service-task, action: apply-ac-policy, name: REC}\n" +
			"      - {id: COMMIT, type: call-activity, process: commit, name: COMMIT, params: {category: test-commit}}\n" +
			"      - {id: END, type: end-event, name: END}\n" +
			"    sequence-flows:\n"
		if head == "SYS" {
			y += "      - {from: SYS, to: REC}\n"
		}
		y += "      - {from: REC, to: COMMIT}\n      - {from: COMMIT, to: END}\n"
		for _, p := range []string{"implement-and-verify-system", "commit"} {
			y += strings.Replace(stub, "%s", p, 1)
		}
		return y
	}
	count := func(head string) int {
		eng, err := LoadBytes([]byte(build(head)))
		if err != nil {
			t.Fatalf("LoadBytes: %v", err)
		}
		n := 0
		for _, v := range CheckInvariants(eng) {
			if v.Rule == "commit-is-verified" {
				n++
			}
		}
		return n
	}
	if n := count("SYS"); n != 0 {
		t.Errorf("apply-ac-policy after implement-and-verify-system: want no commit-is-verified violation, got %d", n)
	}
	if count("REC") == 0 {
		t.Error("apply-ac-policy without a preceding implement-and-verify-system must violate commit-is-verified")
	}
}
