package statemachine

// Graph-level invariant helpers over a loaded *Engine.
//
// These quantify cross-process structural rules over EVERY matching node in
// the embedded snapshot, rather than enumerating known call-sites the way the
// per-site assertions in transitions_test.go do. The value is catching the
// *unenumerated* new site: wire a fifth `commit` without an upstream verify, a
// new fixer dispatch with no back-edge, or a deliberate-halt terminal left as a
// soft end-event, and CheckInvariants flags it without anyone editing a test.
//
// The helpers are deliberately plain Go over the existing graph types (no DSL,
// no fluent builder) and live in a non-test file so the
// precedence / reachability / terminal-kind vocabulary can be reused later by a
// runtime-event invariant checker over the readable-trace plan's []Event seam
// (D2/D4). They return structured Violations rather than calling t.Errorf, so
// the same rules can run against any graph source.
//
// The runtime-event form (asserting over what a real run actually dispatched)
// is deferred until that []Event seam lands; this file is graph-only.

import (
	"fmt"
	"sort"
	"strings"
)

// Violation is one breach of a graph invariant, naming the offending site and
// the rule that flagged it. Message is a human-readable diagnostic.
type Violation struct {
	Process string // process ID the offending node lives in
	Node    string // offending node ID
	Rule    string // rule name (e.g. "commit-is-verified")
	Message string // human-readable description of the breach
}

func (v Violation) String() string {
	return fmt.Sprintf("[%s] %s/%s: %s", v.Rule, v.Process, v.Node, v.Message)
}

// invariantRule checks one rule over the whole engine and returns any breaches.
type invariantRule func(*Engine) []Violation

// invariantRules is the registered rule set — three seed rules spanning
// precedence, loop-back, and terminal-kind shapes.
var invariantRules = []invariantRule{
	ruleCommitIsVerified,
	ruleFixLoopsBack,
	ruleHaltTerminalsAreErrorEnd,
}

// CheckInvariants runs every registered graph invariant over eng and returns
// the aggregated violations in a deterministic order (rule, then process, then
// node). A nil/empty result means the graph satisfies every rule.
func CheckInvariants(eng *Engine) []Violation {
	var violations []Violation
	for _, rule := range invariantRules {
		violations = append(violations, rule(eng)...)
	}
	sort.Slice(violations, func(i, j int) bool {
		a, b := violations[i], violations[j]
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		if a.Process != b.Process {
			return a.Process < b.Process
		}
		return a.Node < b.Node
	})
	return violations
}

// ---------------------------------------------------------------------------
// Rule: commit-is-verified
// ---------------------------------------------------------------------------

// ruleCommitIsVerified asserts that every `commit` call-activity is reached
// only through a verify-tests-pass / verify-tests-fail call-activity within the
// same process. It walks back from each commit site through non-dispatching
// nodes (gateways, events) to the nearest dispatching ancestors; each of those
// must be a verify-tests-* call-activity. Grounds on COMMIT_TEST_CODE /
// COMMIT_SYSTEM / COMMIT_TESTS / COMMIT_LAYER.
func ruleCommitIsVerified(eng *Engine) []Violation {
	const rule = "commit-is-verified"
	var out []Violation
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if !isCallTo(node, "commit") {
				continue
			}
			preds := dispatchingPredecessors(proc, node.ID)
			if len(preds) == 0 {
				out = append(out, Violation{
					Process: proc.ID, Node: node.ID, Rule: rule,
					Message: "commit has no dispatching predecessor — not gated by a verify-tests-* call-activity",
				})
				continue
			}
			for _, pred := range preds {
				pn := proc.Nodes[pred]
				if isCallTo(pn, "verify-tests-pass") || isCallTo(pn, "verify-tests-fail") {
					continue
				}
				out = append(out, Violation{
					Process: proc.ID, Node: node.ID, Rule: rule,
					Message: fmt.Sprintf("commit's dispatching predecessor %q (process %q) is not a verify-tests-* call-activity", pred, pn.Raw.Process),
				})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Rule: fix-loops-back
// ---------------------------------------------------------------------------

// ruleFixLoopsBack asserts that every fixer dispatch (a call-activity into the
// `fix` or a `fix-*` process) has an outgoing edge that loops back to a node
// from which the fix node is reachable again — the generalization of
// TestFixDispatch_LoopsBackToOriginatingStep from a 4-row table to a quantifier
// over all fix sites. A fix node that only routes forward (e.g. straight to a
// terminal) would silently strip the re-verification cycle.
func ruleFixLoopsBack(eng *Engine) []Violation {
	const rule = "fix-loops-back"
	var out []Violation
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			if !isFixDispatch(node) {
				continue
			}
			loopsBack := false
			for _, e := range proc.OutgoingByNode[node.ID] {
				if reaches(proc, e.To, node.ID) {
					loopsBack = true
					break
				}
			}
			if !loopsBack {
				out = append(out, Violation{
					Process: proc.ID, Node: node.ID, Rule: rule,
					Message: fmt.Sprintf("fixer dispatch (process %q) has no outgoing edge that loops back to it — re-verification cycle stripped", node.Raw.Process),
				})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Rule: halt-terminals-are-error-end
// ---------------------------------------------------------------------------

// ruleHaltTerminalsAreErrorEnd asserts that every terminal node whose id marks
// a deliberate halt is an ErrorEndEvent, never a soft EndEvent — so the failure
// bubbles up to driver.Run as a non-zero exit instead of looking like a clean
// finish. Generalizes the per-process ErrorEndEvent-kind checks scattered
// across TestFixDispatch_LoopsAreBounded, TestVerifyTests_InfraOutcomeRoutesToHalt,
// and TestExecuteAgent_ScopeExceptionRoutesToStopViolation.
//
// The halt markers are the *unambiguous* ones: `*_EXHAUSTED` (loop caps),
// `*_INFRA_HALT` (runner could not start), and `STOP_*` (hard refusals).
// `*_REJECTED_END` is deliberately NOT a marker: rejection is bimodal — a PRE
// rejection (EXECUTE_AGENT_REJECTED_END / EXECUTE_COMMAND_REJECTED_END) is an
// intentional soft skip with no artifact produced, while FIX_REJECTED_END is a
// hard halt — so the suffix is not a reliable halt signal.
// FIX_REJECTED_END's error-end kind stays covered by
// TestFixDispatch_LoopsAreBounded.
func ruleHaltTerminalsAreErrorEnd(eng *Engine) []Violation {
	const rule = "halt-terminals-are-error-end"
	var out []Violation
	for _, proc := range eng.Processes {
		for _, node := range proc.Nodes {
			// Only terminal event nodes are in scope — a human-STOP
			// user-task that blocks for input is not a terminal.
			if node.Kind != EndEvent && node.Kind != ErrorEndEvent {
				continue
			}
			if !isHaltID(node.ID) {
				continue
			}
			if node.Kind != ErrorEndEvent {
				out = append(out, Violation{
					Process: proc.ID, Node: node.ID, Rule: rule,
					Message: "deliberate-halt terminal is a soft end-event; must be error-end-event so the failure bubbles up",
				})
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Shared graph primitives
// ---------------------------------------------------------------------------

// isCallTo reports whether node is a call-activity targeting the named process.
func isCallTo(node Node, process string) bool {
	return node.Kind == CallActivity && node.Raw.Process == process
}

// isFixDispatch reports whether node dispatches a fixer subprocess (`fix` or
// any `fix-*`, e.g. fix-unexpected-failing-tests).
func isFixDispatch(node Node) bool {
	if node.Kind != CallActivity {
		return false
	}
	return node.Raw.Process == "fix" || strings.HasPrefix(node.Raw.Process, "fix-")
}

// isDispatching reports whether a node kind dispatches work (a sub-process,
// agent, or action) as opposed to routing/terminating. The commit-precedence
// walk stops at these boundaries.
func isDispatching(kind NodeKind) bool {
	return kind == CallActivity || kind == UserTask || kind == ServiceTask
}

// isHaltID reports whether a node id marks a deliberate halt. See
// ruleHaltTerminalsAreErrorEnd for why `*_REJECTED_END` is excluded.
func isHaltID(id string) bool {
	return strings.HasSuffix(id, "_EXHAUSTED") ||
		strings.HasSuffix(id, "_INFRA_HALT") ||
		strings.HasPrefix(id, "STOP_")
}

// dispatchingPredecessors returns the ids of the nearest dispatching ancestor
// nodes of start, reached by walking backward through non-dispatching nodes
// (gateways, events) only. A dispatching node terminates the walk on its branch
// — its own predecessors are not explored. The result is the set of "what
// dispatched immediately before this node" across every inbound path.
func dispatchingPredecessors(proc *Process, start string) []string {
	incoming := map[string][]string{}
	for _, e := range proc.Edges {
		incoming[e.To] = append(incoming[e.To], e.From)
	}
	var result []string
	seen := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		for _, pred := range incoming[id] {
			if seen[pred] {
				continue
			}
			seen[pred] = true
			if isDispatching(proc.Nodes[pred].Kind) {
				result = append(result, pred)
				continue // boundary — do not walk past a dispatching node
			}
			walk(pred)
		}
	}
	walk(start)
	return result
}

// reaches reports whether target is forward-reachable from `from` over the
// process's sequence flows (BFS over OutgoingByNode).
func reaches(proc *Process, from, target string) bool {
	seen := map[string]bool{}
	queue := []string{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if seen[cur] {
			continue
		}
		seen[cur] = true
		for _, e := range proc.OutgoingByNode[cur] {
			if e.To == target {
				return true
			}
			if !seen[e.To] {
				queue = append(queue, e.To)
			}
		}
	}
	return false
}
