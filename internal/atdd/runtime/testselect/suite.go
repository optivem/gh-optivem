package testselect

import (
	"path/filepath"
	"strings"
)

// tagSuites returns the suites the test should run under, given its
// channel hints, layer, and file path. A test annotated `@Channel(API)`
// runs under acceptance-api; `@Channel(UI)` runs under acceptance-ui;
// both annotations produce two entries. Contract-layer tests default to
// contract-stub (the WRITE phase per ct-cycle-conventions.md).
//
// An empty result signals the caller to fall back to all suites for the
// layer.
func tagSuites(h testHit, layer string, lay *layout) []string {
	if layer == "external" {
		// Per the plan: WRITE phase exercises the stub side only.
		return []string{"contract-stub"}
	}
	// Contract test path hint trumps absent annotations.
	if lay.ContractTestPathHint != "" &&
		strings.Contains(filepath.ToSlash(strings.ToLower(h.File)), lay.ContractTestPathHint) {
		return []string{"contract-stub"}
	}
	if len(h.Channels) == 0 {
		return nil
	}
	out := make([]string, 0, len(h.Channels))
	saw := map[string]bool{}
	for _, c := range h.Channels {
		var suite string
		switch strings.ToUpper(c) {
		case "API":
			suite = "acceptance-api"
		case "UI":
			suite = "acceptance-ui"
		default:
			continue
		}
		if !saw[suite] {
			saw[suite] = true
			out = append(out, suite)
		}
	}
	return out
}

// fallbackSuitesForLayer returns the conservative suite set when a test
// can't be tagged precisely. Shop layer fans out to both acceptance-api
// and acceptance-ui; external layer fans out to contract-stub only (the
// WRITE phase contract).
func fallbackSuitesForLayer(layer string) []string {
	if layer == "external" {
		return []string{"contract-stub"}
	}
	return []string{"acceptance-api", "acceptance-ui"}
}

// AcceptanceSuites returns the canonical list of acceptance suites — the
// dispatch fallback used by run_targeted_tests when its call-activity is
// invoked without a specific `suite:` param (as is the case for the
// collapsed AT_GREEN node, which writes once and verifies both channels).
// A future channel-execution plan may introduce sentinel suites like
// `<acceptance>` that union these explicitly; today, the absent-key case
// resolves to this fixed list.
func AcceptanceSuites() []string {
	return []string{"acceptance-api", "acceptance-ui"}
}

// suiteGroups is the registry of group-alias names. Each alias maps to
// the canonical suite ids it expands to. Today the only group is
// "acceptance"; the registry is shaped this way so adding contract /
// e2e groups later is a one-line edit.
var suiteGroups = map[string][]string{
	"acceptance": AcceptanceSuites(),
}

// ExpandSuiteGroups maps known group-alias names from suiteGroups to
// their constituent suite ids and passes any non-alias name through
// unchanged. Duplicates after expansion are de-duped while preserving
// first-seen order. Unknown names pass through so that the downstream
// "suite(s) not found" check in the runner still catches typos.
//
// Used by the `gh optivem test run` CLI to let `--suite=acceptance`
// resolve to `acceptance-api,acceptance-ui` — and by the BPMN
// runtime, which emits `--suite=acceptance` from the
// verify-tests-pass/fail call-activities in
// `write-and-verify-acceptance-test-code`.
func ExpandSuiteGroups(names []string) []string {
	if len(names) == 0 {
		return names
	}
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	add := func(s string) {
		if seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, n := range names {
		if group, ok := suiteGroups[n]; ok {
			for _, s := range group {
				add(s)
			}
			continue
		}
		add(n)
	}
	return out
}
