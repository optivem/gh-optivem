// Package atdd hosts ATDD doctrinal allowlists that sit outside the runtime
// subtree. The per-phase scope SSoT itself lives inline on the
// EXECUTE_AGENT call-activity nodes inside each writing-agent MID in
// internal/atdd/runtime/statemachine/process-flow.yaml — see Engine.Scope
// for the accessor. This file retains the two layer-classification
// allowlists that the scope-check actions and the build-time test guards
// still consume.
package atdd

// NonWritingAgents are agent names that do not need a phase-scope entry.
// `human` is the trusted-actor case — the operator is trusted to scope
// their own edits.
var NonWritingAgents = map[string]bool{
	"human": true,
}

// FamilyAPathKeysInScope lists Family A path-shaped keys that are valid
// as phase-scope layers. `system-path` is the only one today;
// `system-test-path` is deliberately excluded because it is the parent
// of every Family B testkit key and admitting it would let any phase
// escape the layer partition.
var FamilyAPathKeysInScope = map[string]bool{
	"system-path": true,
}
