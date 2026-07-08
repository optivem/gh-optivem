package actions

import (
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// contractStubScopeLayers are the write-scope layers of the external-contract /
// external-stub writing agents (plan 20260620-2348, Q3; producer dirs added by
// plan 20260622-1739 Step 3):
//
//   - contract-test-writer              — ct-test, dsl-port, dsl-core
//   - stub-fidelity-test-writer         — ct-test
//   - external-system-driver-adapter-updater / -implementer — external-system-driver-adapter
//   - external-system-stub-implementer  — external-system-stub
//   - external-system-real-simulator-implementer — external-system-simulator
//
// A scope-exception that names a file under any of these is "external
// contract/stub work" — exactly the work that requires a ## External System
// Contract Criteria section on the ticket. The broadly-shared layers those
// agents ALSO write (common, system-driver-adapter-shared) are deliberately
// EXCLUDED: a shared-foundation edit is not contract/stub-specific and must not
// route a vanilla scope-exception into the ESCC-undeclared halt. The set is
// pinned as explicit placeholder keys (resolved via ResolveLayerPaths — the
// registry-projected simulator/stub keys skip cleanly on a registry-less
// project), never prose patterns ([[feedback_paths_deterministic_no_guessing]],
// [[feedback_substitutable_paths_in_docs]]).
var contractStubScopeLayers = []string{
	"ct-test",
	"dsl-port",
	"dsl-core",
	"external-system-driver-adapter",
	"external-system-stub",
	"external-system-simulator",
}

// atTestScopeLayers is the write-scope layer that identifies a contradictory-
// tests scope-exception (plan 20260708-1038): a prod-writing agent (e.g.
// system-implementer) refuses to touch acceptance-test files because doing so
// would satisfy one test only by breaking another. Resolved the same
// directory-keyed way as contractStubScopeLayers, not reason-string parsing.
var atTestScopeLayers = []string{"at-test"}

// categorizeScopeException is Guard B (plan 20260620-2348, generalized by plan
// 20260708-1038). It runs on the scope-exception==true branch of execute-agent,
// immediately before GATE_SCOPE_EXCEPTION_NEEDS_ESCC, and stamps
// ctx.State["scope-exception-kind"] to one of:
//
//   - "escc-undeclared":    ≥1 scope-exception-files entry sits under a
//     contractStubScopeLayers family, AND ticket-has-escc == false.
//   - "contradictory-tests": (escc-undeclared does not apply) AND ≥1
//     scope-exception-files entry sits under the at-test family — a
//     contradictory-tests case (per system-implementer.md's own
//     "contradictory tests" refusal doctrine).
//   - "other":              neither of the above (the generic, unchanged case).
//
// escc-undeclared routes to ESCC_UNDECLARED_HALT (actionable "add a ##
// External System Contract Criteria section" diagnostic); contradictory-tests
// routes to CONTRADICTORY_TESTS_HALT (actionable "reconcile the test via
// acceptance-test-writer" diagnostic — never "widen scope", which is the wrong
// fix for a contradictory-tests halt); other keeps STOP_SCOPE_VIOLATION
// unchanged.
//
// Categorization is by directory-prefix match (pathInScope) against the
// resolved families — the same directory-keyed contract the scope check
// itself uses ([[feedback_port_changed_flags_directory_keyed]]) — not
// method/pattern guessing. escc-undeclared is checked first: the two families
// are disjoint path prefixes in practice, but this ordering is deliberate and
// pins the case an entry could ever satisfy both.
//
// scope-exception-files and ticket-has-escc are both already in ctx.State
// (validate-outputs-and-scopes flattens the agent's scope-exception envelope;
// parse-ticket stamps ticket-has-escc once per ticket), so this action reads
// state and resolves config — no agent, no shell-out. Hard config errors
// (gh-optivem.yaml missing, a layer unresolvable) surface as Outcome.Err since
// they indicate a wiring/config problem, not an agent-output one; the layers
// are validated at the preflight scope sweep, so an unresolvable layer here is
// a genuine config break.
func (a actions) categorizeScopeException(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("categorize-scope-exception: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}

	files, _ := ctx.State["scope-exception-files"].([]string)
	hasESCC, _ := ctx.State["ticket-has-escc"].(bool)

	contractFamilies, err := ResolveLayerPaths(contractStubScopeLayers, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("categorize-scope-exception: %w", err)}
	}
	atTestFamilies, err := ResolveLayerPaths(atTestScopeLayers, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("categorize-scope-exception: %w", err)}
	}

	var contractFiles, atTestFiles []string
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if pathInScope(f, contractFamilies) {
			contractFiles = append(contractFiles, f)
		}
		if pathInScope(f, atTestFamilies) {
			atTestFiles = append(atTestFiles, f)
		}
	}

	kind := "other"
	switch {
	case len(contractFiles) > 0 && !hasESCC:
		kind = "escc-undeclared"
	case len(atTestFiles) > 0:
		kind = "contradictory-tests"
	}
	ctx.Set("scope-exception-kind", kind)

	// Surface the actionable, system-named diagnostic on stderr — the
	// error-end-event name is rendered literally (run.go does not
	// ExpandParams terminal names), so the dynamic detail rides here rather
	// than in the terminal event's label.
	switch kind {
	case "escc-undeclared":
		who := "the external system"
		if systems := contractStubSystemNames(contractFiles, cfg); len(systems) > 0 {
			who = "external system(s) " + strings.Join(systems, ", ")
		}
		fmt.Fprintf(a.deps.Stderr,
			"categorize-scope-exception: the agent refused to write external contract/stub files for %s, but the ticket declares no External System Contract Criteria. Add a `## External System Contract Criteria` section naming the external system and re-run.\n  contract/stub files: %s\n",
			who, strings.Join(contractFiles, ", "))
	case "contradictory-tests":
		fmt.Fprintf(a.deps.Stderr,
			"categorize-scope-exception: the agent refused to write acceptance-test files because doing so would satisfy this change only by contradicting a pre-existing test. Reconcile the conflicting test via acceptance-test-writer — do not widen this agent's write scope — and re-run.\n  acceptance-test files: %s\n",
			strings.Join(atTestFiles, ", "))
	}
	return statemachine.Outcome{}
}

// contractStubSystemNames derives the external-system names a contract/stub
// scope-exception names, SOLELY from the entries under the external-system
// driver roots (driver-adapter / driver-port), using the deterministic
// <root>/<name>/ first-path-segment rule shared with
// externalSystemNamesFromChangedPaths. Entries under ct-test / dsl-* carry no
// per-system path segment and contribute no name — the diagnostic falls back to
// the generic "the external system" phrasing in that case rather than guessing a
// name out of a filename. Returns a sorted, de-duplicated slice.
func contractStubSystemNames(files []string, cfg *projectconfig.Config) []string {
	roots, err := ResolveLayerPaths([]string{"external-system-driver-adapter", "external-system-driver-port"}, cfg)
	if err != nil {
		return nil
	}
	set := map[string]struct{}{}
	joined := strings.Join(files, "\n")
	for _, root := range roots {
		for name := range externalSystemNamesFromChangedPaths(joined, root) {
			set[name] = struct{}{}
		}
	}
	return sortedSetKeys(set)
}
