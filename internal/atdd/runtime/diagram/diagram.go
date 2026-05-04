// Package diagram renders the canonical Mermaid markdown for the ATDD
// process flow.
//
// gh-optivem owns one rendered diagram (`docs/process-diagram.md`),
// regenerated whenever the embedded YAML at
// `internal/atdd/runtime/statemachine/process-flow.yaml` changes.
// github.com renders Mermaid natively, so anyone browsing the repo sees
// the diagram with zero tooling.
//
// Render is intentionally mechanical — one block per YAML flow,
// labels straight from the `description:` field, edges labelled with
// the `when:` predicate after light boolean → Yes/No translation. It
// does not aggregate predicates or rename nodes for prose. The aim is a
// deterministic, reviewable artifact, not a hand-polished presentation
// — for the latter, the per-phase prose docs in consumer repos are still
// the right place to go.
package diagram

import (
	"fmt"
	"sort"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// flowAlias maps internal flow IDs to human-readable section heading
// names. Flows not in the map render with the raw ID — that surfaces
// "you added a flow without giving it a heading alias" loudly when a
// new flow appears in the YAML.
var flowAlias = map[string]string{
	"main":                       "Ticket Lifecycle",
	"intake":                     "Intake",
	"run_legacy_cycle":           "Run Legacy Cycle",
	"run_cycle":                  "Run Cycle",
	"at_cycle":                   "AT Cycle",
	"at_green_system":            "AT - GREEN - SYSTEM",
	"ct_subprocess":              "Contract Test Sub-Process",
	"external_system_onboarding": "External System Onboarding Sub-Process",
	"structural_cycle":           "Structural Cycle (shared)",
	"external_api_task_cycle":    "External API Task Cycle",
	"legacy_acceptance_criteria":            "Legacy Acceptance Criteria Cycle",
}

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

// flowOrder is the order flows are rendered in the output. Flows not
// listed here come last in lexical order.
var flowOrder = []string{
	"main",
	"intake",
	"run_legacy_cycle",
	"run_cycle",
	"at_cycle",
	"at_green_system",
	"ct_subprocess",
	"external_system_onboarding",
	"structural_cycle",
	"external_api_task_cycle",
	"legacy_acceptance_criteria",
}

// Render returns the full Mermaid markdown body for eng's flows. The
// output is suitable for writing to `docs/process-diagram.md`.
func Render(eng *statemachine.Engine) string {
	var b strings.Builder
	writeHeader(&b)
	for _, name := range orderedFlowNames(eng) {
		flow := eng.Flows[name]
		writeFlowSection(&b, name, flow)
	}
	return b.String()
}

func writeHeader(b *strings.Builder) {
	b.WriteString("# ATDD Process Flow\n\n")
	b.WriteString("> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem atdd show diagram > docs/process-diagram.md`.\n\n")
	b.WriteString("Each section corresponds to one named flow in the YAML. `call_activity` nodes appear as boxes pointing at the linked sub-flow's heading.\n\n")
}

// orderedFlowNames returns flow names in the canonical order: every
// name in flowOrder that exists, followed by any extras in lexical
// order.
func orderedFlowNames(eng *statemachine.Engine) []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range flowOrder {
		if _, ok := eng.Flows[name]; ok {
			out = append(out, name)
			seen[name] = true
		}
	}
	var extras []string
	for name := range eng.Flows {
		if !seen[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	return append(out, extras...)
}

func writeFlowSection(b *strings.Builder, name string, flow *statemachine.Flow) {
	heading := flowAlias[name]
	if heading == "" {
		heading = name
	}
	fmt.Fprintf(b, "## %s\n\n", heading)
	b.WriteString("```mermaid\nflowchart TD\n")

	// Stable node order: walk flow.Nodes in YAML insertion order.
	// statemachine.Flow.Nodes is a map, so we sort by ID for
	// deterministic output. (The YAML source order is lost at parse
	// time; alphabetical is the next-best stable choice, and node
	// rendering order does not affect Mermaid layout.)
	ids := make([]string, 0, len(flow.Nodes))
	for id := range flow.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

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
		g := flow.Nodes[id].Raw.Group
		if g == "" {
			ungrouped = append(ungrouped, id)
			continue
		}
		root.insert(strings.Split(g, "/"), id)
	}
	for _, id := range ungrouped {
		writeNode(b, flow.Nodes[id])
	}
	for _, child := range root.sortedChildren() {
		writeGroupSubgraph(b, flow, child)
	}
	b.WriteString("\n")

	// Edges in YAML declaration order — that's what flow.Edges
	// preserves.
	for _, e := range flow.Edges {
		writeEdge(b, e)
	}

	writeExecutorStyling(b, flow)
	b.WriteString("```\n\n")
}

// groupTree is a node in the slash-delimited subgraph hierarchy. The
// root has fullPath="" and represents the (implicit) outermost scope;
// its direct children are top-level groups.
type groupTree struct {
	fullPath string                // joined path, e.g. "structural/interface"
	name     string                // last path segment
	nodes    []string              // flow node IDs grouped at this level
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
func writeGroupSubgraph(b *strings.Builder, flow *statemachine.Flow, g *groupTree) {
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
		writeNode(b, flow.Nodes[id])
	}
	for _, child := range g.sortedChildren() {
		writeGroupSubgraph(b, flow, child)
	}
	b.WriteString("    end\n")
}

// writeNode emits one Mermaid node line. Shape depends on the YAML
// node type; label comes from the `description:` field, falling back
// to the node ID when absent. call_activity nodes get a "see § …"
// suffix pointing the reader at the sub-flow's heading.
//
// Shape mapping (BPMN-shaped vocabulary):
//
//	start_event / end_event → circle              `((label))`
//	gateway                 → diamond             `{label}`
//	service_task            → subroutine          `[[label]]`
//	user_task               → plain rectangle     `[label]`
//	call_activity           → plain rectangle     `[label]`  (with "see § …" suffix)
//
// Shape conveys the BPMN node type; executor coloring (applied later
// in writeExecutorStyling) conveys *who* runs each task: white =
// service_task (Go runtime), dark blue = LLM agent, yellow = human.
func writeNode(b *strings.Builder, n statemachine.Node) {
	label := n.Raw.Description
	if label == "" {
		label = n.ID
	}
	switch n.Kind {
	case statemachine.StartEvent:
		fmt.Fprintf(b, "    %s((%s))\n", n.ID, mermaidLabel("Start"))
	case statemachine.EndEvent:
		fmt.Fprintf(b, "    %s((%s))\n", n.ID, mermaidLabel("End"))
	case statemachine.Gateway:
		fmt.Fprintf(b, "    %s{%s}\n", n.ID, mermaidLabel(label))
	case statemachine.ServiceTask:
		fmt.Fprintf(b, "    %s[[%s]]\n", n.ID, mermaidLabel(label))
	case statemachine.CallActivity:
		target := n.Raw.Flow
		linkLabel := flowAlias[target]
		if linkLabel == "" {
			linkLabel = target
		}
		full := fmt.Sprintf("%s — see § %s", label, linkLabel)
		fmt.Fprintf(b, "    %s[%s]\n", n.ID, mermaidLabel(full))
	default:
		fmt.Fprintf(b, "    %s[%s]\n", n.ID, mermaidLabel(label))
	}
}

// writeEdge emits one Mermaid edge line. Edges with a `when:` predicate
// get a labelled arrow; the label comes from translateWhen.
func writeEdge(b *strings.Builder, e statemachine.Edge) {
	if e.Predicate == "" {
		fmt.Fprintf(b, "    %s --> %s\n", e.From, e.To)
		return
	}
	fmt.Fprintf(b, "    %s -- %s --> %s\n", e.From, edgeLabel(e.Predicate), e.To)
}

// writeExecutorStyling colors nodes by who executes them, so a reader
// can see at a glance which steps the Go runtime runs, which an LLM
// agent runs, and which a human runs. Three classes:
//
//	serviceNode  white fill, black text   — service_task (Go runtime)
//	agentNode    dark blue, white text    — user_task with agent: <name>
//	humanNode    yellow, black text       — user_task with agent: human
//
// Empty classes are omitted. start_event / end_event / gateway /
// call_activity are unstyled — they're shape-distinguished and not
// "executed by" anyone in the same sense.
func writeExecutorStyling(b *strings.Builder, flow *statemachine.Flow) {
	var service, agent, human []string
	ids := make([]string, 0, len(flow.Nodes))
	for id := range flow.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := flow.Nodes[id]
		switch n.Kind {
		case statemachine.ServiceTask:
			service = append(service, id)
		case statemachine.UserTask:
			if n.Raw.Agent == "human" {
				human = append(human, id)
			} else if n.Raw.Agent != "" {
				agent = append(agent, id)
			}
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
// edge label. Common cases:
//   - `x == true`  → "Yes"
//   - `x == false` → "No"
//   - `x == value` → "value"
//   - `x in [a, b]` → "a / b"
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
		return mermaidLabel(rhs)
	}
	if i := strings.Index(p, " in "); i >= 0 {
		rhs := strings.TrimSpace(p[i+4:])
		rhs = strings.TrimPrefix(rhs, "[")
		rhs = strings.TrimSuffix(rhs, "]")
		parts := strings.Split(rhs, ",")
		for j, part := range parts {
			parts[j] = strings.TrimSpace(part)
		}
		return mermaidLabel(strings.Join(parts, " / "))
	}
	return mermaidLabel(p)
}
