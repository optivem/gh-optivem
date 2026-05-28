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
// as phase-scope layers. `system-path` and `system-db-migration-path`
// are eligible today; `system-test-path` is deliberately excluded
// because it is the parent of every Family B testkit key and admitting
// it would let any phase escape the layer partition.
//
// `system-db-migration-path` names the shared canonical migration set
// (Flyway-ordered SQL files under `system/db/migrations` by default)
// consumed by every SUT (3 languages × 2 architectures). It is a sibling
// of `system/monolith/` and `system/multitier/`, not a child of either —
// schema migrations are architecture- and language-agnostic.
var FamilyAPathKeysInScope = map[string]bool{
	"system-path":              true,
	"system-db-migration-path": true,
}
