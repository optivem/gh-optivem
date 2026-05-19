// Package atdd hosts ATDD doctrinal data files that sit outside the
// runtime subtree (phase-scopes.yaml as a peer of
// internal/atdd/runtime/architecture/architecture.yaml and
// internal/atdd/runtime/statemachine/process-flow.yaml).
//
// This file holds the production loader + doctrinal allowlists.
// phase_scopes_test.go imports the same symbols for its drift guards,
// so the test surface and the production surface read from one
// definition.
package atdd

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

//go:embed phase-scopes.yaml
var phaseScopesYAML []byte

// PhaseScopes is the parsed shape of phase-scopes.yaml: BPMN phase id →
// ordered list of layer names (Family B keys from
// projectconfig.CanonicalPathKeys() plus the Family A entries in
// FamilyAPathKeysInScope).
type PhaseScopes struct {
	Phases map[string][]string `yaml:"phases"`
}

// PhasesDeferredByPlan lists writing-agent phase ids in process-flow.yaml
// that knowingly have no phase-scopes.yaml entry yet. Each entry cites
// the deferred plan that picks up the scope work, so a future audit grep
// finds the gap with its follow-up.
//
// Map exposed (not a function) so callers can index by phase id directly
// and the test file's reverse-FK check can range over the same data.
var PhasesDeferredByPlan = map[string]string{
	"AT_GREEN_BACKEND":                         "plans/deferred/20260518-1530-multitier-green-scope.md",
	"AT_GREEN_FRONTEND":                        "plans/deferred/20260518-1530-multitier-green-scope.md",
	"SYSTEM_INTERFACE_REDESIGN_CYCLE":          "plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md",
	"EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE": "plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md",
	"CHORE_CYCLE":                              "plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md",
	// CT_RED_TEST's doctrinal scope is [ct_test, dsl_port, dsl_core] but
	// `ct_test` is added to canonicalPathKeys() by the CT-vocabulary plan
	// below. Restore as a phase-scopes.yaml entry once that plan lands.
	"CT_RED_TEST": "plans/20260518-1742-family-b-stems-and-ct-vocab.md",
}

// NonWritingAgents are agent names that do not need a phase-scopes
// entry. `human` is the trusted-actor case; `fix-verify` is a retry
// helper that inherits scope from the failing phase's context.
var NonWritingAgents = map[string]bool{
	"human":      true,
	"fix-verify": true,
}

// FamilyAPathKeysInScope lists Family A path-shaped keys that are valid
// as phase-scope layers. `system_path` is the only one today;
// `system_test_path` is deliberately excluded because it is the parent
// of every Family B testkit key and admitting it would let any phase
// escape the layer partition.
var FamilyAPathKeysInScope = map[string]bool{
	"system_path": true,
}

// LoadPhaseScopes parses the embedded phase-scopes.yaml and returns the
// resulting PhaseScopes. A malformed document or an empty phases map
// returns an error.
func LoadPhaseScopes() (PhaseScopes, error) {
	var ps PhaseScopes
	if err := yaml.Unmarshal(phaseScopesYAML, &ps); err != nil {
		return PhaseScopes{}, fmt.Errorf("parse phase-scopes.yaml: %w", err)
	}
	if len(ps.Phases) == 0 {
		return PhaseScopes{}, fmt.Errorf("phase-scopes.yaml parsed to empty phases map")
	}
	return ps, nil
}
