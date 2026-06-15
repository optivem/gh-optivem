// Process-axis reuse proof (child #8, Step 8). Demonstrates the engine's
// swap contract from the *outside*: an external package (statemachine_test)
// brings its own BPMN — a small document-publishing flow that has nothing to
// do with ATDD — loads it via the public LoadBytes entry point, binds plain
// Go closures for its actions and its one gateway, and drives it to
// completion with RunProcess.
//
// The point is the negative space: the engine needs no change and no
// knowledge of this domain. There is no "publish-doc" anything in the engine
// — the flow's vocabulary lives entirely in the YAML string below and the
// closures this test registers. This is the worked example referenced by
// internal/engine/statemachine/doc.go's "bring your own BPMN" contract.
package statemachine_test

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// publishFlowYAML is an arbitrary, non-ATDD business process: draft a
// document, route on a reviewer's decision, then either publish or archive.
// A start-event, two branching service-tasks, one gateway and two distinct
// end-events — enough to exercise sequential dispatch and predicate routing
// without any ATDD concept.
const publishFlowYAML = `
processes:
  publish-doc:
    name: "Publish a document"
    start: DRAFT
    nodes:
      - id: DRAFT
        type: service-task
        action: write-draft
        name: "Write the draft"
      - id: REVIEW
        type: gateway
        binding: review-decision
        name: "Reviewer decision"
      - id: PUBLISH
        type: service-task
        action: publish
        name: "Publish the document"
      - id: ARCHIVE
        type: service-task
        action: archive
        name: "Archive the rejected draft"
      - id: PUBLISHED
        type: end-event
        name: "Published"
      - id: REJECTED
        type: end-event
        name: "Rejected"
    sequence-flows:
      - { from: DRAFT, to: REVIEW }
      - { from: REVIEW, to: PUBLISH, when: "review-decision == approved" }
      - { from: REVIEW, to: ARCHIVE, when: "review-decision == rejected" }
      - { from: PUBLISH, to: PUBLISHED }
      - { from: ARCHIVE, to: REJECTED }
`

func TestReuse_SecondBPMNDrivesOnGenericEngine(t *testing.T) {
	tests := []struct {
		name        string
		decision    string   // value the gateway closure returns
		wantActions []string // expected service-task execution order
	}{
		{
			name:        "approved routes to publish",
			decision:    "approved",
			wantActions: []string{"write-draft", "publish"},
		},
		{
			name:        "rejected routes to archive",
			decision:    "rejected",
			wantActions: []string{"write-draft", "archive"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ── ARRANGE ─────────────────────────────────────────────────
			eng, err := statemachine.LoadBytes([]byte(publishFlowYAML))
			if err != nil {
				t.Fatalf("LoadBytes: %v", err)
			}

			var ran []string
			eng.ActionFn = func(name string) statemachine.NodeFn {
				return func(ctx *statemachine.Context) statemachine.Outcome {
					ran = append(ran, name)
					return statemachine.Outcome{}
				}
			}
			// The single gateway returns the reviewer's verdict as its
			// Outcome value; the engine records it under the binding name
			// (review-decision) so the two `when:` clauses route on it.
			eng.GateFn = func(binding string) statemachine.NodeFn {
				return func(ctx *statemachine.Context) statemachine.Outcome {
					return statemachine.Outcome{Value: tt.decision}
				}
			}
			// No AgentFn: this flow has no user-task, proving the engine
			// drives an action+gateway-only process with no agent layer at
			// all — the agent set is genuinely a separable concern.

			if err := eng.Bind(); err != nil {
				t.Fatalf("Bind: %v", err)
			}

			// ── ACT ─────────────────────────────────────────────────────
			if err := eng.RunProcess("publish-doc", statemachine.NewContext()); err != nil {
				t.Fatalf("RunProcess: %v", err)
			}

			// ── ASSERT ───────────────────────────────────────────────────
			if len(ran) != len(tt.wantActions) {
				t.Fatalf("ran actions %v, want %v", ran, tt.wantActions)
			}
			for i, want := range tt.wantActions {
				if ran[i] != want {
					t.Errorf("action %d = %q, want %q (full sequence %v)", i, ran[i], want, ran)
				}
			}
		})
	}
}
