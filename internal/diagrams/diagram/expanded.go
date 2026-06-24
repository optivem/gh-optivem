// Package diagram — expanded.go renders a second Mermaid variant where
// call-activity nodes are replaced by inline subgraph blocks that show the
// subprocess steps. All call-activities are expanded recursively (with cycle
// detection), so readers can see the full flow in one diagram without
// navigating between sections.
//
// The four TOP-level entry processes (main, refine-ticket, implement-ticket,
// refactor) are rendered as sections. Subprocess call-activities become
// `subgraph … end` blocks; their nodes are scoped with the call-activity
// node ID as a prefix (e.g. CALL_ACT__INNER_NODE_ID) to avoid ID collisions
// when the same subprocess is reached from multiple call sites.
//
// Output is suitable for docs/process-diagram-expanded.md, regenerated via:
//
//	gh optivem process show --expanded > docs/process-diagram-expanded.md
package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
)

// topLevelExpansionRoots are the process IDs used as section roots in the
// expanded diagram. Their call-activities are expanded recursively.
var topLevelExpansionRoots = []string{
	"main",
	"refine-ticket",
	"implement-ticket",
	"refactor",
}

// maxExpandDepth limits how many levels of call-activity nesting are expanded.
// Depth 0 = root; depth N = Nth level of subprocesses shown as subgraphs.
// Call-activities at depth > maxExpandDepth render as plain reference boxes.
//
// The BPMN has 7 levels of nesting; a limit of 3 exposes TOP→CYCLE→HIGH→MID
// structure without the LOW primitives that repeat across every MID call site
// and would grow the diagram exponentially.
const maxExpandDepth = 3

// RenderExpanded returns a Mermaid markdown body like Render but with
// call-activity nodes replaced by inline subgraphs showing the subprocess
// steps. Only the top-level entry processes are rendered as sections.
// The output is suitable for docs/process-diagram-expanded.md.
func RenderExpanded(eng *statemachine.Engine) string {
	var b strings.Builder
	b.WriteString("# ATDD Process Flow — Expanded\n\n")
	b.WriteString("> Generated from `internal/atdd/process/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem process show --expanded > docs/process-diagram-expanded.md`.\n\n")
	b.WriteString("Each section shows a top-level process with all call-activity nodes expanded inline as subgraphs. See [process-diagram.md](process-diagram.md) for the one-process-per-section reference view.\n\n")
	writeLegend(&b)
	for _, id := range topLevelExpansionRoots {
		proc, ok := eng.Processes[id]
		if !ok {
			continue
		}
		writeExpandedSection(&b, eng, proc)
	}
	return b.String()
}

// expandAcc collects all edge and styling declarations across the whole
// expansion tree for one diagram section. A single expandAcc is passed by
// pointer to all recursive expandProcess calls so every declaration lands in
// one flat list — Mermaid allows cross-subgraph edges at any nesting level.
type expandAcc struct {
	edgeLines   []string
	service     []string
	agent       []string
	human       []string
	errorEnd    []string
	tddRed      []string
	tddGreen    []string
	tddRefactor []string
}

// subgraphTree holds the node declarations and nested subgraph blocks for
// one expanded process. It does NOT hold edges (those go into expandAcc so
// they're emitted as a flat list after all subgraph blocks).
type subgraphTree struct {
	nodeLines []string       // Mermaid node declaration lines at this level
	children  []*subgraphKid // nested subgraphs (one per expanded call-activity)
}

// subgraphKid is one expanded call-activity rendered as a Mermaid subgraph.
type subgraphKid struct {
	id    string        // Mermaid subgraph ID (= call-activity's scoped node ID)
	title string        // display title (= subprocess name)
	inner *subgraphTree // node declarations and nested children of the subprocess
}

// writeExpandedSection emits one `## heading` + `flowchart TD` block for proc
// with all call-activities recursively expanded as subgraphs.
func writeExpandedSection(b *strings.Builder, eng *statemachine.Engine, proc *statemachine.Process) {
	fmt.Fprintf(b, "## %s\n\n", proc.Name)
	b.WriteString("```mermaid\nflowchart TD\n")

	acc := &expandAcc{}
	visited := map[string]bool{proc.ID: true}
	tree, _, _ := expandProcess(acc, eng, proc, "", visited, 0)

	for _, line := range tree.nodeLines {
		b.WriteString(line + "\n")
	}
	for _, kid := range tree.children {
		writeSubgraphKid(b, kid)
	}
	b.WriteString("\n")
	for _, line := range acc.edgeLines {
		b.WriteString(line + "\n")
	}
	writeExpandedStyling(b, acc)
	b.WriteString("```\n\n")
}

// writeSubgraphKid emits a `subgraph … end` block, recursing into nested kids.
func writeSubgraphKid(b *strings.Builder, kid *subgraphKid) {
	fmt.Fprintf(b, "    subgraph %s[%s]\n", kid.id, mermaidLabel(kid.title))
	for _, line := range kid.inner.nodeLines {
		b.WriteString(line + "\n")
	}
	for _, child := range kid.inner.children {
		writeSubgraphKid(b, child)
	}
	b.WriteString("    end\n")
}

// expandProcess builds a subgraphTree for proc and populates acc with all
// edge and styling declarations. scopePrefix is prepended to every node ID;
// visited guards against subprocess cycles; depth tracks expansion level
// relative to the section root (depth 0 = root process itself).
//
// Returns (tree, entryNodeID, exitNodeIDs):
//   - entryNodeID is the scoped start-event ID — incoming edges in the parent
//     are rewired to point here.
//   - exitNodeIDs are the scoped normal end-event IDs — outgoing edges in the
//     parent are rewired to originate from here (one edge per exit).
func expandProcess(acc *expandAcc, eng *statemachine.Engine, proc *statemachine.Process, scopePrefix string, visited map[string]bool, depth int) (*subgraphTree, string, []string) {
	tree := &subgraphTree{}

	// entryExit caches the rewired (entry, exits) for each call-activity in
	// this process so edge rewiring below can look them up by original node ID.
	type entryExit struct {
		entry string
		exits []string
	}
	expansions := map[string]entryExit{}

	for _, id := range flowOrderedNodeIDs(proc) {
		n := proc.Nodes[id]
		scopedID := scopePrefix + id

		if n.Kind != statemachine.CallActivity {
			tree.nodeLines = append(tree.nodeLines, expandNodeDecl(scopedID, n))
			expandAccStyle(acc, scopedID, n)
			continue
		}

		targetID := n.Raw.Process
		targetProc, ok := eng.Processes[targetID]
		if !ok || visited[targetID] || depth >= maxExpandDepth {
			// Unknown process (e.g. templated "${action}"), cycle detected, or
			// depth limit reached: fall back to a plain call-activity node so
			// the diagram stays connected.
			tree.nodeLines = append(tree.nodeLines, expandNodeDecl(scopedID, n))
			expandAccStyle(acc, scopedID, n)
			expansions[id] = entryExit{entry: scopedID, exits: []string{scopedID}}
			continue
		}

		innerScope := scopedID + "__"
		innerVisited := make(map[string]bool, len(visited)+1)
		for k := range visited {
			innerVisited[k] = true
		}
		innerVisited[targetID] = true
		innerTree, innerEntry, innerExits := expandProcess(acc, eng, targetProc, innerScope, innerVisited, depth+1)

		tree.children = append(tree.children, &subgraphKid{
			id:    scopedID,
			title: targetProc.Name,
			inner: innerTree,
		})
		expansions[id] = entryExit{entry: innerEntry, exits: innerExits}
	}

	// Rewire edges: replace call-activity endpoints with their expansion's
	// entry/exit node IDs. For a call-activity with N normal exits and M
	// outgoing edges in the parent, emit N×M rewired edges.
	for _, e := range proc.Edges {
		var froms []string
		if exp, ok := expansions[e.From]; ok {
			froms = exp.exits
		} else {
			froms = []string{scopePrefix + e.From}
		}
		var to string
		if exp, ok := expansions[e.To]; ok {
			to = exp.entry
		} else {
			to = scopePrefix + e.To
		}
		for _, from := range froms {
			if e.Predicate == "" {
				acc.edgeLines = append(acc.edgeLines, fmt.Sprintf("    %s --> %s", from, to))
			} else {
				acc.edgeLines = append(acc.edgeLines, fmt.Sprintf("    %s -- %s --> %s", from, edgeLabel(e.Predicate), to))
			}
		}
	}

	entryID := scopePrefix + proc.Start
	var exitIDs []string
	endKeys := make([]string, 0)
	for id, n := range proc.Nodes {
		if n.Kind == statemachine.EndEvent {
			endKeys = append(endKeys, id)
		}
	}
	sort.Strings(endKeys)
	for _, id := range endKeys {
		exitIDs = append(exitIDs, scopePrefix+id)
	}
	return tree, entryID, exitIDs
}

// expandNodeDecl returns a Mermaid node declaration line (4-space indent)
// using the pre-computed scopedID. Shape rules mirror writeNode.
func expandNodeDecl(scopedID string, n statemachine.Node) string {
	label := n.Raw.Name
	switch n.Kind {
	case statemachine.StartEvent, statemachine.EndEvent:
		return fmt.Sprintf("    %s(( ))", scopedID)
	case statemachine.ErrorEndEvent:
		return fmt.Sprintf("    %s((⚡))", scopedID)
	case statemachine.Gateway:
		if label == "" {
			return fmt.Sprintf("    %s{ }", scopedID)
		}
		return fmt.Sprintf("    %s{%s}", scopedID, mermaidLabel(label))
	case statemachine.ServiceTask:
		return fmt.Sprintf("    %s[[%s]]", scopedID, mermaidLabel(label))
	case statemachine.CallActivity:
		// Fallback path (cycle or unknown process): show as a plain box.
		target := n.Raw.Process
		var full string
		if label != "" {
			full = fmt.Sprintf("%s — see § %s", label, target)
		} else {
			full = fmt.Sprintf("see § %s", target)
		}
		if paramLines := callActivityParamLines(n); len(paramLines) > 0 {
			full = full + "<br/>" + strings.Join(paramLines, "<br/>")
		}
		return fmt.Sprintf("    %s[%s]", scopedID, mermaidLabel(full))
	default:
		return fmt.Sprintf("    %s[%s]", scopedID, mermaidLabel(label))
	}
}

// expandAccStyle records a node's executor and TDD-stage classification into
// acc so that writeExpandedStyling can emit the corresponding classDef lines.
func expandAccStyle(acc *expandAcc, scopedID string, n statemachine.Node) {
	switch n.Kind {
	case statemachine.ServiceTask:
		acc.service = append(acc.service, scopedID)
	case statemachine.UserTask:
		switch n.Raw.Agent {
		case "human":
			acc.human = append(acc.human, scopedID)
		case "":
			// No executor annotation — no color applied.
		default:
			acc.agent = append(acc.agent, scopedID)
		}
	case statemachine.ErrorEndEvent:
		acc.errorEnd = append(acc.errorEnd, scopedID)
	}
	switch n.Raw.TDDStage {
	case "red":
		acc.tddRed = append(acc.tddRed, scopedID)
	case "green":
		acc.tddGreen = append(acc.tddGreen, scopedID)
	case "refactor":
		acc.tddRefactor = append(acc.tddRefactor, scopedID)
	}
}

// writeExpandedStyling emits all classDef + class lines from acc. Mirrors
// writeExecutorStyling but operates on the flat accumulated node ID lists.
func writeExpandedStyling(b *strings.Builder, acc *expandAcc) {
	if len(acc.service) > 0 {
		b.WriteString("\n    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000\n")
		fmt.Fprintf(b, "    class %s serviceNode\n", strings.Join(acc.service, ","))
	}
	if len(acc.agent) > 0 {
		b.WriteString("\n    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff\n")
		fmt.Fprintf(b, "    class %s agentNode\n", strings.Join(acc.agent, ","))
	}
	if len(acc.human) > 0 {
		b.WriteString("\n    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000\n")
		fmt.Fprintf(b, "    class %s humanNode\n", strings.Join(acc.human, ","))
	}
	if len(acc.errorEnd) > 0 {
		b.WriteString("\n    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000\n")
		fmt.Fprintf(b, "    class %s errorEndNode\n", strings.Join(acc.errorEnd, ","))
	}
	if len(acc.tddRed) > 0 {
		b.WriteString("\n    classDef tddRedNode stroke:#dc3545,stroke-width:3px\n")
		fmt.Fprintf(b, "    class %s tddRedNode\n", strings.Join(acc.tddRed, ","))
	}
	if len(acc.tddGreen) > 0 {
		b.WriteString("\n    classDef tddGreenNode stroke:#28a745,stroke-width:3px\n")
		fmt.Fprintf(b, "    class %s tddGreenNode\n", strings.Join(acc.tddGreen, ","))
	}
	if len(acc.tddRefactor) > 0 {
		b.WriteString("\n    classDef tddRefactorNode stroke:#007bff,stroke-width:3px\n")
		fmt.Fprintf(b, "    class %s tddRefactorNode\n", strings.Join(acc.tddRefactor, ","))
	}
}
