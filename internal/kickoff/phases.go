// Package kickoff implements `gh optivem kickoff`: the guided path from a
// freshly scaffolded repo (which runs green on the shop sample) to the user's
// own product. One public command resumes at the next incomplete phase; the
// phases themselves are plain functions so tests and the hidden `phase`
// subcommands can drive each one directly.
package kickoff

import (
	"fmt"
	"strings"
)

// Phase names one step of the kickoff flow.
type Phase string

const (
	PhaseDescription Phase = "description"
	PhasePrototype   Phase = "prototype"
	PhaseDiagram     Phase = "use-case-diagram"
	PhaseUseCases    Phase = "use-cases"
	PhaseRemove      Phase = "remove-sample"
	PhaseStories     Phase = "stories"
)

// Phases lists every phase in execution order.
var Phases = []Phase{PhaseDescription, PhasePrototype, PhaseDiagram, PhaseUseCases, PhaseRemove, PhaseStories}

// approvalPhases are the phases that end at a user checkpoint and store an
// approval fingerprint in the progress file.
var approvalPhases = []Phase{PhasePrototype, PhaseDiagram, PhaseUseCases}

// NeedsApproval reports whether p ends at a stored user approval.
func NeedsApproval(p Phase) bool {
	for _, a := range approvalPhases {
		if a == p {
			return true
		}
	}
	return false
}

// ParsePhase validates a phase name.
func ParsePhase(s string) (Phase, error) {
	for _, p := range Phases {
		if string(p) == s {
			return p, nil
		}
	}
	return "", fmt.Errorf("unknown phase %q (valid: %s)", s, joinPhases(Phases))
}

// ParseRedoPhase validates a --redo target. Only approval phases can be
// reopened: the description is edited in place, and shop removal and story
// creation are not reversible by re-running.
func ParseRedoPhase(s string) (Phase, error) {
	p, err := ParsePhase(s)
	if err != nil {
		return "", err
	}
	if !NeedsApproval(p) {
		return "", fmt.Errorf("--redo %s is not supported: only the approval phases can be reopened (%s); edit docs/product.md directly to change the description, and note that removal and story creation are not undone by re-running", p, joinPhases(approvalPhases))
	}
	return p, nil
}

func joinPhases(ps []Phase) string {
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = string(p)
	}
	return strings.Join(names, ", ")
}

// laterApprovalPhases returns the approval phases strictly after p.
func laterApprovalPhases(p Phase) []Phase {
	var out []Phase
	seen := false
	for _, a := range approvalPhases {
		if seen {
			out = append(out, a)
		}
		if a == p {
			seen = true
		}
	}
	return out
}
