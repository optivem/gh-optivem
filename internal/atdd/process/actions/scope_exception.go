package actions

import (
	"fmt"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// contractStubScopeLayers are the Family B write-scope layers of the three
// external-contract / external-stub writing agents (plan 20260620-2348, Q3):
//
//   - contract-test-writer       — ct-test, dsl-port, dsl-core
//   - stub-fidelity-test-writer  — ct-test
//   - external-system-stub-implementer — external-system-driver-adapter
//
// A scope-exception that names a file under any of these is "external
// contract/stub work" — exactly the work that requires a ## External System
// Contract Criteria section on the ticket. The broadly-shared layers those
// agents ALSO write (common, system-driver-adapter-shared) are deliberately
// EXCLUDED: a shared-foundation edit is not contract/stub-specific and must not
// route a vanilla scope-exception into the ESCC-undeclared halt. The set is
// pinned as explicit Family B placeholder keys (resolved via ResolveLayerPaths),
// never prose patterns ([[feedback_paths_deterministic_no_guessing]],
// [[feedback_substitutable_paths_in_docs]]).
var contractStubScopeLayers = []string{
	"ct-test",
	"dsl-port",
	"dsl-core",
	"external-system-driver-adapter",
}

// categorizeScopeException is Guard B (plan 20260620-2348). It runs on the
// scope-exception==true branch of execute-agent, immediately before
// GATE_SCOPE_EXCEPTION_NEEDS_ESCC, and stamps
// ctx.State["scope-exception-needs-escc"] = true IFF
//
//	(≥1 scope-exception-files entry sits under a contractStubScopeLayers family)
//	AND (ticket-has-escc == false).
//
// True routes the cycle to ESCC_UNDECLARED_HALT with an actionable "add a ##
// External System Contract Criteria section" diagnostic instead of the cryptic
// generic STOP_SCOPE_VIOLATION; false keeps STOP_SCOPE_VIOLATION unchanged (a
// non-contract scope-exception, or one on a ticket that already declares ESCC).
//
// Categorization is by directory-prefix match (pathInScope) against the
// resolved contract/stub families — the same directory-keyed contract the
// scope check itself uses ([[feedback_port_changed_flags_directory_keyed]]) —
// not method/pattern guessing.
//
// scope-exception-files and ticket-has-escc are both already in ctx.State
// (validate-outputs-and-scopes flattens the agent's scope-exception envelope;
// parse-ticket stamps ticket-has-escc once per ticket), so this action reads
// state and resolves config — no agent, no shell-out. Hard config errors
// (gh-optivem.yaml missing, a contract/stub layer unresolvable) surface as
// Outcome.Err since they indicate a wiring/config problem, not an agent-output
// one; the contract/stub layers are validated at the preflight scope sweep, so
// an unresolvable layer here is a genuine config break.
func (a actions) categorizeScopeException(ctx *statemachine.Context) statemachine.Outcome {
	cfg := a.deps.Config
	if cfg == nil {
		return statemachine.Outcome{Err: fmt.Errorf("categorize-scope-exception: gh-optivem.yaml not loaded — driver must inject actions.Deps.Config")}
	}

	files, _ := ctx.State["scope-exception-files"].([]string)
	hasESCC, _ := ctx.State["ticket-has-escc"].(bool)

	families, err := ResolveLayerPaths(contractStubScopeLayers, cfg)
	if err != nil {
		return statemachine.Outcome{Err: fmt.Errorf("categorize-scope-exception: %w", err)}
	}

	var contractFiles []string
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f != "" && pathInScope(f, families) {
			contractFiles = append(contractFiles, f)
		}
	}

	needsESCC := len(contractFiles) > 0 && !hasESCC
	ctx.Set("scope-exception-needs-escc", needsESCC)

	if needsESCC {
		// Surface the actionable, system-named diagnostic on stderr — the
		// error-end-event name is rendered literally (run.go does not
		// ExpandParams terminal names), so the dynamic system name rides here
		// rather than in ESCC_UNDECLARED_HALT's label.
		who := "the external system"
		if systems := contractStubSystemNames(contractFiles, cfg); len(systems) > 0 {
			who = "external system(s) " + strings.Join(systems, ", ")
		}
		fmt.Fprintf(a.deps.Stderr,
			"categorize-scope-exception: the agent refused to write external contract/stub files for %s, but the ticket declares no External System Contract Criteria. Add a `## External System Contract Criteria` section naming the external system and re-run.\n  contract/stub files: %s\n",
			who, strings.Join(contractFiles, ", "))
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
