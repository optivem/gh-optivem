// Package diagram renders the canonical Mermaid markdown for the ATDD
// process flow.
//
// gh-optivem owns one rendered diagram (`docs/process-diagram.md`),
// regenerated whenever the embedded YAML at
// `internal/atdd/runtime/statemachine/process-flow.yaml` changes.
// github.com renders Mermaid natively, so anyone browsing the repo sees
// the diagram with zero tooling.
//
// Render is intentionally mechanical — one block per YAML process,
// labels straight from the `name:` field, edges labelled with the
// `when:` predicate after light boolean → Yes/No translation. It does
// not aggregate predicates or rename nodes for prose. The aim is a
// deterministic, reviewable artifact, not a hand-polished presentation
// — for the latter, the per-phase prose docs in consumer repos are still
// the right place to go.
//
// Names are author-explicit everywhere (per plan 20260526-1730 Item 4):
// process-level `name:` is the section heading; node-level `name:` is
// the box label; the call-activity `— see § <id>` suffix appears iff
// the node label differs from the target process's name. There is no
// auto-Title-Case fallback — `titleCaseFromKebab` survives only to
// translate raw `when:` predicate RHS values for edge labels.
package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// groupAlias maps a node's `group:` annotation (a slash-delimited
// path like "structural/interface") to a human-readable subgraph
// title rendered around the grouped nodes. The renderer first looks
// up the full path; if absent, it falls back to the path's last
// segment, then to the segment verbatim.
var groupAlias = map[string]string{
	"behavioral":                    "System Behavior Change",
	"structural":                    "System Structure Change",
	"structural/interface":          "System Structure Interface Change",
	"structural/implementation":     "System Structure Implementation Change",
	"external_structural":           "External System Structure Change",
	"external_structural/interface": "External System Structure Interface Change",
}

// processOrder is the order flows are rendered in the output. Flows not
// listed here come last in lexical order. The five-level BPMN refactor
// fixes a reader-facing top-down walk: runtime bootstrap → TOP entry
// points → per-ticket CYCLEs → composite HIGH orchestrations → atomic
// MID tasks → LOW primitives. Within a level, related processes cluster
// (e.g. the three `write-and-verify-acceptance-tests*` siblings).
var processOrder = []string{
	// runtime bootstrap
	"main",
	// TOP
	"refine-ticket",
	"implement-ticket",
	"refactor",
	// CYCLE
	"refine-backlog-item",
	"change-system-behavior",
	"cover-system-behavior",
	"redesign-system-structure",
	"refactor-system-structure",
	"refactor-test-structure",
	// HIGH
	"write-and-verify-acceptance-tests-fail",
	"write-and-verify-acceptance-tests-pass",
	"write-and-verify-acceptance-tests",
	"shared-contract",
	"write-and-verify-acceptance-test-code",
	"implement-and-verify-dsl",
	"implement-and-verify-system-driver-adapters",
	"implement-and-verify-external-system-driver-adapters-contract-tests",
	"implement-and-verify-system",
	"refactor-and-verify-tests",
	"implement-test-layer",
	"verify-tests-pass",
	"verify-tests-fail",
	// MID — agent tasks
	"write-acceptance-tests",
	"write-contract-tests",
	"implement-dsl",
	"implement-system",
	"implement-system-driver-adapters",
	"implement-external-system-driver-adapters",
	"implement-external-system-stubs",
	"fix-unexpected-passing-tests",
	"fix-unexpected-failing-tests",
	"refactor-tests",
	"refactor-system",
	"refine-acceptance-criteria",
	// MID — command tasks
	"compile-tests",
	"build-system",
	"start-system",
	"commit",
	"run-tests",
	// LOW
	"approve",
	"execute-agent",
	"execute-command",
	"fix",
}

// Render returns the full Mermaid markdown body for eng's flows. The
// output is suitable for writing to `docs/process-diagram.md`.
func Render(eng *statemachine.Engine) string {
	var b strings.Builder
	writeHeader(&b)
	for _, id := range orderedProcessNames(eng) {
		process := eng.Processes[id]
		writeProcessSection(&b, eng, process)
	}
	return b.String()
}

func writeHeader(b *strings.Builder) {
	b.WriteString("# ATDD Process Flow\n\n")
	b.WriteString("> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem process show > docs/process-diagram.md`.\n\n")
	b.WriteString("Each section corresponds to one named process in the YAML. `call-activity` nodes appear as boxes pointing at the linked sub-process's heading.\n\n")
	writeLegend(b)
}

// writeLegend emits a "Legend" section explaining the encoding shared
// by every process diagram below: shape conveys the BPMN node type,
// fill color conveys *who* executes the task. The Mermaid block reuses
// the same classDef styles as the real diagrams so the legend renders
// as a literal sample.
func writeLegend(b *strings.Builder) {
	b.WriteString("## Legend\n\n")
	b.WriteString("Node **shape** encodes the BPMN type; **fill color** encodes the executor; **border color** (orthogonal) encodes the TDD stage where the author marked one.\n\n")
	b.WriteString("- `(( ))` — start / end event (BPMN plain start or end; empty circle, descriptive name lives in the YAML). Start vs end is read from position in the flow — start has no incoming edge, end has no outgoing edge.\n")
	b.WriteString("- `((⚡))` — error end event (BPMN exceptional exit; red border). Two flavors: **Unknown** (defensive guard — an unhandled gateway branch fired; should never happen at runtime) and **Rejected** (hard-abort — a runtime condition that intentionally halts the run, e.g. agent output rejected post-approve). The descriptive name is in the YAML source; the diagram keeps the icon small.\n")
	b.WriteString("- `{diamond}` — gateway (decision)\n")
	b.WriteString("- `[[subroutine]]` — service task — mechanical, automated step (white)\n")
	b.WriteString("- `[rectangle]` — user task — LLM agent (dark blue) or human (yellow); `call_activity` rectangles are unfilled and link to a sub-process heading\n")
	b.WriteString("- `[/skewed/]` — published outputs of a process (dashed border)\n")
	b.WriteString("- **TDD-stage border** — red = RED (failing test), green = GREEN (test passes), blue = REFACTOR (improve without changing behaviour). Only applied where the call site explicitly plays that role.\n\n")
	b.WriteString("```mermaid\nflowchart LR\n")
	b.WriteString("    EVT(( ))\n")
	b.WriteString("    ERR((⚡))\n")
	b.WriteString("    GW{Gateway}\n")
	b.WriteString("    SVC[[\"Service Task (Automated)\"]]\n")
	b.WriteString("    AGT[\"User Task (LLM Agent)\"]\n")
	b.WriteString("    HUM[\"User Task (Human)\"]\n")
	b.WriteString("    CALL[Call activity — sub-process]\n")
	b.WriteString("    TDDR[RED step]\n")
	b.WriteString("    TDDG[GREEN step]\n")
	b.WriteString("    TDDF[REFACTOR step]\n")
	b.WriteString("    OUT[/Outputs/]\n")
	b.WriteString("\n    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000\n")
	b.WriteString("    class SVC serviceNode\n")
	b.WriteString("\n    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff\n")
	b.WriteString("    class AGT agentNode\n")
	b.WriteString("\n    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000\n")
	b.WriteString("    class HUM humanNode\n")
	b.WriteString("\n    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000\n")
	b.WriteString("    class ERR errorEndNode\n")
	b.WriteString("\n    classDef tddRedNode stroke:#dc3545,stroke-width:3px\n")
	b.WriteString("    class TDDR tddRedNode\n")
	b.WriteString("\n    classDef tddGreenNode stroke:#28a745,stroke-width:3px\n")
	b.WriteString("    class TDDG tddGreenNode\n")
	b.WriteString("\n    classDef tddRefactorNode stroke:#007bff,stroke-width:3px\n")
	b.WriteString("    class TDDF tddRefactorNode\n")
	b.WriteString("\n    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000\n")
	b.WriteString("    class OUT outputNode\n")
	b.WriteString("```\n\n")
}

// orderedProcessNames returns process names in the canonical order: every
// name in processOrder that exists, followed by any extras in lexical
// order.
func orderedProcessNames(eng *statemachine.Engine) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range processOrder {
		if _, ok := eng.Processes[name]; ok {
			out = append(out, name)
			seen[name] = true
		}
	}
	var extras []string
	for name := range eng.Processes {
		if !seen[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	return append(out, extras...)
}

// flowOrderedNodeIDs returns process node IDs in breadth-first flow order
// from process.Start, following process.Edges in declared order. Nodes
// unreachable from Start (none expected in a well-formed process) are
// appended in lexical order so output stays total and deterministic. The
// edge list is the only order-preserving structure on Process (Nodes is a
// map), so this is what reconstructs a readable top-down node listing.
func flowOrderedNodeIDs(process *statemachine.Process) []string {
	visited := make(map[string]bool, len(process.Nodes))
	order := make([]string, 0, len(process.Nodes))
	queue := make([]string, 0, len(process.Nodes))
	if _, ok := process.Nodes[process.Start]; ok {
		queue = append(queue, process.Start)
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if visited[id] {
			continue
		}
		if _, ok := process.Nodes[id]; !ok {
			continue
		}
		visited[id] = true
		order = append(order, id)
		for _, e := range process.Edges {
			if e.From == id && !visited[e.To] {
				queue = append(queue, e.To)
			}
		}
	}
	rest := make([]string, 0)
	for id := range process.Nodes {
		if !visited[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	return append(order, rest...)
}

func writeProcessSection(b *strings.Builder, eng *statemachine.Engine, process *statemachine.Process) {
	fmt.Fprintf(b, "## %s\n\n", process.Name)
	b.WriteString("```mermaid\nflowchart TD\n")

	// Node render order: flow order (breadth-first from Start along the
	// declared edges), not alphabetical. statemachine.Process.Nodes is a
	// map and the YAML source order is lost at parse time, but the edge
	// list preserves declaration order — so a BFS reconstructs a stable,
	// readable top-down walk where each gateway's branch targets appear in
	// the order the edges were written (positive branch first, catch-all
	// last; see edgeLabel / the positive-first convention note). This makes
	// a fork like "Expected Test Result?" list its Pass node before Fail
	// before the ⚡ catch-all, matching the reading order. Mermaid's TD
	// layout still uses this as a heuristic, not a guarantee, so it nudges
	// rather than pins left/right placement.
	ids := flowOrderedNodeIDs(process)

	// Partition nodes into ungrouped and a tree of nested groups
	// keyed by slash-delimited `group:` paths (e.g. "structural" or
	// "structural/interface"). Ungrouped nodes render at the top
	// level; grouped nodes render inside nested `subgraph` blocks
	// matching the path hierarchy. Mermaid supports nested subgraphs
	// natively — the inner blocks are drawn as labelled boxes inside
	// the outer block.
	root := newGroupTree("")
	var ungrouped []string
	for _, id := range ids {
		g := process.Nodes[id].Raw.Group
		if g == "" {
			ungrouped = append(ungrouped, id)
			continue
		}
		root.insert(strings.Split(g, "/"), id)
	}
	for _, id := range ungrouped {
		writeNode(b, eng, process.Nodes[id])
	}
	for _, child := range root.sortedChildren() {
		writeGroupSubgraph(b, eng, process, child)
	}
	b.WriteString("\n")

	// Edges in YAML declaration order — that's what process.Edges
	// preserves.
	for _, e := range process.Edges {
		writeEdge(b, e)
	}

	writeOutputsBlock(b, process)
	writeExecutorStyling(b, process)
	b.WriteString("```\n\n")
}

// writeOutputsBlock emits a BPMN-style data-object node listing the
// process's published outputs and a dashed `produces` edge from every
// reachable end-event to that node. No-op when the process has no
// outputs declared.
func writeOutputsBlock(b *strings.Builder, process *statemachine.Process) {
	if len(process.Outputs) == 0 {
		return
	}
	outputsNodeID := strings.ToUpper(process.ID) + "_OUTPUTS"
	parts := make([]string, 0, len(process.Outputs))
	for _, o := range process.Outputs {
		parts = append(parts, outputSpecLabel(o))
	}
	// One output per line (<br/> inside the data-object label) rather than a
	// single comma-joined run — long output contracts (e.g. the writing-agent
	// MIDs' four-key lists) are far easier to scan stacked, and it matches the
	// per-line rendering of call-activity params. The <br/> forces mermaidLabel
	// to quote the label.
	label := strings.Join(parts, "<br/>")
	fmt.Fprintf(b, "    %s[/%s/]\n", outputsNodeID, mermaidLabel(label))

	endIDs := make([]string, 0)
	for id, node := range process.Nodes {
		if node.Kind == statemachine.EndEvent {
			endIDs = append(endIDs, id)
		}
	}
	sort.Strings(endIDs)
	for _, id := range endIDs {
		fmt.Fprintf(b, "    %s -. produces .-> %s\n", id, outputsNodeID)
	}
	b.WriteString("\n    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000\n")
	fmt.Fprintf(b, "    class %s outputNode\n", outputsNodeID)
}

// outputSpecLabel renders one OutputSpec for the data-object node label.
// Optional outputs gain a trailing "?" so the diagram surfaces the
// required-vs-optional distinction at a glance.
func outputSpecLabel(o statemachine.OutputSpec) string {
	suffix := ""
	if o.Optional {
		suffix = "?"
	}
	return o.Key + suffix + ": " + o.Type
}

// groupTree is a node in the slash-delimited subgraph hierarchy. The
// root has fullPath="" and represents the (implicit) outermost scope;
// its direct children are top-level groups.
type groupTree struct {
	fullPath string                // joined path, e.g. "structural/interface"
	name     string                // last path segment
	nodes    []string              // process node IDs grouped at this level
	children map[string]*groupTree // segment → subtree
}

func newGroupTree(name string) *groupTree {
	return &groupTree{name: name, children: map[string]*groupTree{}}
}

// insert places nodeID under the path segments. Creates intermediate
// subtrees as needed; idempotent on the path.
func (g *groupTree) insert(segments []string, nodeID string) {
	cur := g
	path := ""
	for _, seg := range segments {
		if path == "" {
			path = seg
		} else {
			path = path + "/" + seg
		}
		child, ok := cur.children[seg]
		if !ok {
			child = newGroupTree(seg)
			child.fullPath = path
			cur.children[seg] = child
		}
		cur = child
	}
	cur.nodes = append(cur.nodes, nodeID)
}

// sortedChildren returns the direct children in deterministic order
// (alphabetical by segment name) so diagram output is stable across
// runs.
func (g *groupTree) sortedChildren() []*groupTree {
	keys := make([]string, 0, len(g.children))
	for k := range g.children {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*groupTree, 0, len(keys))
	for _, k := range keys {
		out = append(out, g.children[k])
	}
	return out
}

// writeGroupSubgraph emits a `subgraph` block for g, then recurses
// into nested children. The subgraph's stable Mermaid ID derives from
// the full path with "/" replaced by "_" (Mermaid disallows "/" in
// identifiers). Title resolution: full path → last segment → raw
// segment.
func writeGroupSubgraph(b *strings.Builder, eng *statemachine.Engine, process *statemachine.Process, g *groupTree) {
	title := groupAlias[g.fullPath]
	if title == "" {
		title = groupAlias[g.name]
	}
	if title == "" {
		title = g.name
	}
	sid := strings.ReplaceAll(g.fullPath, "/", "_")
	fmt.Fprintf(b, "    subgraph %s[%s]\n", sid, mermaidLabel(title))
	for _, id := range g.nodes {
		writeNode(b, eng, process.Nodes[id])
	}
	for _, child := range g.sortedChildren() {
		writeGroupSubgraph(b, eng, process, child)
	}
	b.WriteString("    end\n")
}

// writeNode emits one Mermaid node line. Shape depends on the YAML
// node type; the visible label comes from the node's explicit `name:`
// field — required on every non-gateway node by the schema (see
// load.go); there is no fallback to the node ID. call-activity nodes
// append a "see § <id>" suffix pointing the reader at the sub-process's
// heading, unless the node label already equals the target process's
// `name:` — in which case the suffix would be redundant and is dropped.
//
// Shape mapping (BPMN-shaped vocabulary):
//
//	start-event / end-event → circle              `(( ))` (empty — no inside label; BPMN plain start/end)
//	error-end-event         → circle              `((⚡))` (icon only — no inside label; red border via classDef; Legend lists the two flavors)
//	gateway                 → diamond             `{label}` (or `{ }` when silent — Item 13)
//	service-task            → subroutine          `[[label]]`
//	user-task               → plain rectangle     `[label]`
//	call-activity           → plain rectangle     `[label]`  (with "see § <id>" suffix unless redundant)
//
// Shape conveys the BPMN node type; executor coloring (applied later
// in writeExecutorStyling) conveys *who* runs each task: white =
// service-task (Go runtime), dark blue = LLM agent, yellow = human.
// TDD-stage border colours (red / green / blue) overlay the executor
// fill so the two signals coexist without conflict.
//
// Silent gateways (Item 13): a gateway whose `name:` is empty is
// rendered as a bare diamond `GW{ }`, never with the node ID as a
// fallback label. The pattern arises when an upstream user_task owns
// the elicitation so the gateway itself has nothing to say;
// Hungarian-style `{GATE_…}` in the diagram would just be noise.
func writeNode(b *strings.Builder, eng *statemachine.Engine, n statemachine.Node) {
	label := n.Raw.Name
	switch n.Kind {
	case statemachine.StartEvent, statemachine.EndEvent:
		// Empty small circle — matches mainstream BPMN where plain
		// start / end events have no inner symbol and the descriptive
		// name sits outside. Mermaid can't place labels outside shapes,
		// so we drop the name; the YAML retains it. End-event labels
		// were mostly past-tense restatements of the section heading
		// ("Backlog Item Refined" closing `## Refine Backlog Item`) —
		// high redundancy, low signal. The few processes with multiple
		// end events (approve/reject patterns in `approve`, `fix`,
		// `execute-agent`, `execute-command`) discriminate via the
		// upstream gateway's edge label (`approval-outcome ==
		// approved` vs `rejected`), so the empty terminal carries no
		// information loss. Start and end are distinguished by position
		// in the flow (no incoming edge vs no outgoing edge).
		fmt.Fprintf(b, "    %s(( ))\n", n.ID)
	case statemachine.ErrorEndEvent:
		// Icon-only circle, no inside label — matches mainstream BPMN
		// where event circles stay small and the descriptive name sits
		// outside. Mermaid can't place labels outside shapes, so we
		// drop the name from the diagram and let the Legend teach the
		// two flavors instead (Unknown = defensive guard for an
		// unhandled gateway branch; Rejected = intentional hard-abort).
		// The authoritative name remains in the YAML.
		fmt.Fprintf(b, "    %s((⚡))\n", n.ID)
	case statemachine.Gateway:
		if label == "" {
			fmt.Fprintf(b, "    %s{ }\n", n.ID)
		} else {
			fmt.Fprintf(b, "    %s{%s}\n", n.ID, mermaidLabel(label))
		}
	case statemachine.ServiceTask:
		fmt.Fprintf(b, "    %s[[%s]]\n", n.ID, mermaidLabel(label))
	case statemachine.CallActivity:
		target := n.Raw.Process
		var targetName string
		if targetProcess, ok := eng.Processes[target]; ok {
			targetName = targetProcess.Name
		}
		// Drop the redundant "see § <id>" suffix when the node label
		// already equals the target process's name (per plan
		// 20260526-1730 Item 4 — trivial equality, no auto-Title-Case
		// derivation, no alias map).
		var full string
		if label == targetName {
			full = label
		} else {
			full = fmt.Sprintf("%s — see § %s", label, target)
		}
		if lines := callActivityParamLines(n); len(lines) > 0 {
			full = full + "<br/>" + strings.Join(lines, "<br/>")
		}
		fmt.Fprintf(b, "    %s[%s]\n", n.ID, mermaidLabel(full))
	default:
		fmt.Fprintf(b, "    %s[%s]\n", n.ID, mermaidLabel(label))
	}
}

// callActivityParamLines renders a call-activity's pinned LITERAL params
// as "key = value" lines for display under the node label. Only literals
// are shown: `${…}` values are caller-forwarded variables (noise on the
// diagram and identical across call sites), so they're skipped. This is
// what makes otherwise-identical thin wrappers legible — e.g. the
// write-and-verify-acceptance-tests-{fail,pass} wrappers differ only by a
// pinned `expected-test-result`, and surfacing it stops the two diagrams
// from looking like the same picture. Keys are sorted for deterministic
// output (Params is a map). Non-call-activity nodes carry no params, so
// callers gate on node kind.
func callActivityParamLines(n statemachine.Node) []string {
	if len(n.Raw.Params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(n.Raw.Params))
	for k := range n.Raw.Params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		v := n.Raw.Params[k]
		if strings.Contains(v, "${") {
			continue
		}
		lines = append(lines, k+" = "+v)
	}
	return lines
}

// writeEdge emits one Mermaid edge line. Edges with a `when:` predicate
// get a labelled arrow; the label comes from translateWhen.
//
// Edges render in YAML declaration order (Process.Edges preserves it),
// which both the runtime (first matching predicate wins) and this diagram
// rely on. The authoring convention is positive branch first: for a
// boolean gateway declare the `== true` (Yes) edge before `== false` (No),
// and always leave the unguarded catch-all edge (the ⚡ error / fallback)
// LAST — the runtime treats it as the always-true default, so reordering it
// earlier would mis-route. Multi-value gateways follow the same "catch-all
// last" rule. This keeps every fork reading Yes→No (and Pass→Fail→⚡) in
// both the edge list and, via flowOrderedNodeIDs, the node listing. Note:
// Mermaid's flowchart TD treats declaration order as a layout heuristic, so
// it nudges Yes left / No right rather than guaranteeing it.
func writeEdge(b *strings.Builder, e statemachine.Edge) {
	if e.Predicate == "" {
		fmt.Fprintf(b, "    %s --> %s\n", e.From, e.To)
		return
	}
	fmt.Fprintf(b, "    %s -- %s --> %s\n", e.From, edgeLabel(e.Predicate), e.To)
}

// writeExecutorStyling colors nodes by who executes them and overlays
// TDD-stage borders where authors marked the call site's role in the
// red-green-refactor triad. Reader signals:
//
//	serviceNode      white fill, black text   — service-task (Automated)
//	agentNode        dark blue, white text    — user-task with agent: <name>
//	humanNode        yellow, black text       — user-task with agent: human
//	errorEndNode     light red fill, red border — error-end-event (BPMN exceptional exit)
//	tddRedNode       red border (stroke only)   — RED step (write failing test)
//	tddGreenNode     green border (stroke only) — GREEN step (test passes)
//	tddRefactorNode  blue border (stroke only)  — REFACTOR step (improve without changing behaviour)
//
// Empty classes are omitted. start-event / end-event / gateway /
// call-activity are unstyled by executor — they're shape-distinguished
// and not "executed by" anyone in the same sense. TDD-stage classes
// only set the stroke so they coexist with executor fill on the same
// node (border + fill convey orthogonal signals).
func writeExecutorStyling(b *strings.Builder, process *statemachine.Process) {
	var service, agent, human, errorEnd []string
	var tddRed, tddGreen, tddRefactor []string
	ids := make([]string, 0, len(process.Nodes))
	for id := range process.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := process.Nodes[id]
		switch n.Kind {
		case statemachine.ServiceTask:
			service = append(service, id)
		case statemachine.UserTask:
			if n.Raw.Agent == "human" {
				human = append(human, id)
			} else if n.Raw.Agent != "" {
				agent = append(agent, id)
			}
		case statemachine.ErrorEndEvent:
			errorEnd = append(errorEnd, id)
		}
		switch n.Raw.TDDStage {
		case "red":
			tddRed = append(tddRed, id)
		case "green":
			tddGreen = append(tddGreen, id)
		case "refactor":
			tddRefactor = append(tddRefactor, id)
		}
	}
	if len(service) > 0 {
		b.WriteString("\n    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000\n")
		fmt.Fprintf(b, "    class %s serviceNode\n", strings.Join(service, ","))
	}
	if len(agent) > 0 {
		b.WriteString("\n    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff\n")
		fmt.Fprintf(b, "    class %s agentNode\n", strings.Join(agent, ","))
	}
	if len(human) > 0 {
		b.WriteString("\n    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000\n")
		fmt.Fprintf(b, "    class %s humanNode\n", strings.Join(human, ","))
	}
	if len(errorEnd) > 0 {
		b.WriteString("\n    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000\n")
		fmt.Fprintf(b, "    class %s errorEndNode\n", strings.Join(errorEnd, ","))
	}
	if len(tddRed) > 0 {
		b.WriteString("\n    classDef tddRedNode stroke:#dc3545,stroke-width:3px\n")
		fmt.Fprintf(b, "    class %s tddRedNode\n", strings.Join(tddRed, ","))
	}
	if len(tddGreen) > 0 {
		b.WriteString("\n    classDef tddGreenNode stroke:#28a745,stroke-width:3px\n")
		fmt.Fprintf(b, "    class %s tddGreenNode\n", strings.Join(tddGreen, ","))
	}
	if len(tddRefactor) > 0 {
		b.WriteString("\n    classDef tddRefactorNode stroke:#007bff,stroke-width:3px\n")
		fmt.Fprintf(b, "    class %s tddRefactorNode\n", strings.Join(tddRefactor, ","))
	}
}

// titleCaseAbbreviations are tokens preserved verbatim (upper-case)
// during kebab → Title Case transformation. Match is case-insensitive
// against kebab segments; output is the canonical form.
var titleCaseAbbreviations = map[string]string{
	"dsl":  "DSL",
	"at":   "AT",
	"ct":   "CT",
	"bpmn": "BPMN",
	"tdd":  "TDD",
	"atdd": "ATDD",
	"bdd":  "BDD",
	"sut":  "SUT",
	"api":  "API",
	"url":  "URL",
	"db":   "DB",
	"io":   "IO",
}

// titleCaseSmallWords are articles, conjunctions, and short prepositions
// lower-cased mid-phrase per Title Case convention. The first word of a
// phrase is always capitalised regardless of this set.
var titleCaseSmallWords = map[string]bool{
	"a":    true,
	"an":   true,
	"and":  true,
	"as":   true,
	"by":   true,
	"for":  true,
	"in":   true,
	"of":   true,
	"on":   true,
	"or":   true,
	"the":  true,
	"to":   true,
	"with": true,
}

// titleCaseFromKebab converts a kebab- and/or slash-delimited identifier
// into Title Case using BPMN-style activity-naming conventions: major
// words capitalised, short articles/prepositions/conjunctions lowercased
// mid-phrase, abbreviations from titleCaseAbbreviations preserved as
// uppercase. Slash-bearing inputs (`task/cover-legacy`) render with
// spaces around the slash (`Task / Cover Legacy`).
//
// Shared by writeProcessSection (Mermaid section headings) and
// edgeLabel (gateway edge labels). The transformation is render-time
// only — YAML identifiers stay kebab.
func titleCaseFromKebab(s string) string {
	parts := strings.Split(s, "/")
	for i, part := range parts {
		parts[i] = titleCaseFromKebabSegment(part)
	}
	return strings.Join(parts, " / ")
}

func titleCaseFromKebabSegment(s string) string {
	segments := strings.Split(s, "-")
	out := make([]string, 0, len(segments))
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		lower := strings.ToLower(seg)
		if abbrev, ok := titleCaseAbbreviations[lower]; ok {
			out = append(out, abbrev)
			continue
		}
		if i > 0 && titleCaseSmallWords[lower] {
			out = append(out, lower)
			continue
		}
		out = append(out, strings.ToUpper(seg[:1])+seg[1:])
	}
	return strings.Join(out, " ")
}

// mermaidLabel returns label as-is when safe, or wrapped in double
// quotes when it contains characters Mermaid would otherwise parse as
// shape / edge syntax (`|`, parens, braces, brackets, `<`, `>`, `&`,
// `"`, `#`, `;`). The COMMIT-message convention `<Ticket> | <PHASE>`
// is the most common case; wrapping is mechanical.
func mermaidLabel(s string) string {
	if !strings.ContainsAny(s, "|(){}[]<>&\"#;") {
		return s
	}
	// Escape embedded double quotes by replacing with the HTML entity —
	// Mermaid does not have a backslash escape inside quoted labels.
	escaped := strings.ReplaceAll(s, `"`, "&quot;")
	return `"` + escaped + `"`
}

// edgeLabel translates a YAML `when:` predicate into a short Mermaid
// edge label. Kebab and slash-bearing routing values render in Title
// Case via titleCaseFromKebab so edges read as human-readable
// conditions, not YAML identifiers. Common cases:
//   - `x == true`  → "Yes"
//   - `x == false` → "No"
//   - `x == kebab-value` → "Kebab Value"
//   - `x == task/cover-legacy` → "Task / Cover Legacy"
//   - `x in [a, b]` → "A / B"
//
// Anything that doesn't match these patterns is returned verbatim
// (wrapped in mermaidLabel for safety).
func edgeLabel(pred string) string {
	p := strings.TrimSpace(pred)
	if i := strings.Index(p, "=="); i >= 0 {
		rhs := strings.TrimSpace(p[i+2:])
		switch rhs {
		case "true":
			return "Yes"
		case "false":
			return "No"
		}
		return mermaidLabel(titleCaseFromKebab(rhs))
	}
	if i := strings.Index(p, " in "); i >= 0 {
		rhs := strings.TrimSpace(p[i+4:])
		rhs = strings.TrimPrefix(rhs, "[")
		rhs = strings.TrimSuffix(rhs, "]")
		parts := strings.Split(rhs, ",")
		for j, part := range parts {
			parts[j] = titleCaseFromKebab(strings.TrimSpace(part))
		}
		return mermaidLabel(strings.Join(parts, " / "))
	}
	return mermaidLabel(p)
}
