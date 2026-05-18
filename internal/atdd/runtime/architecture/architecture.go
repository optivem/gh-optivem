// Package architecture renders the canonical Mermaid markdown for the
// ATDD layered-architecture diagram.
//
// gh-optivem owns one rendered diagram (`docs/architecture-diagram.md`),
// regenerated whenever the embedded YAML at
// `internal/atdd/runtime/architecture/architecture.yaml` changes.
// github.com renders Mermaid natively, so anyone browsing the repo sees
// the diagram with zero tooling.
//
// The renderer is intentionally mechanical: one `## <heading>` section
// per YAML section, one Mermaid `flowchart` block per section, nodes
// then edges in YAML declaration order. Cross-section references emit
// a "<label> — see § <target-heading>" suffix when the node carries a
// `ref:` field. Three edge styles are supported: plain directional
// (`-->`), undirected (`---`), and labelled directional (`-->|label|`).
package architecture

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Document is the top-level YAML schema: an ordered list of sections.
type Document struct {
	Sections []Section `yaml:"sections"`
}

// Section is one `## <heading>` block with one Mermaid flowchart inside.
type Section struct {
	Name      string `yaml:"name"`
	Heading   string `yaml:"heading"`
	Direction string `yaml:"direction,omitempty"`
	Nodes     []Node `yaml:"nodes"`
	Edges     []Edge `yaml:"edges"`
}

// Node is one Mermaid node line. Ref optionally points at another
// section's name; the renderer appends " — see § <heading>".
type Node struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Ref   string `yaml:"ref,omitempty"`
}

// Edge is one Mermaid edge line. Style controls the arrow type
// ("" → directional `-->`, "undirected" → `---`). Label, when set,
// produces a labelled arrow `-->|label|`.
type Edge struct {
	From  string `yaml:"from"`
	To    string `yaml:"to"`
	Style string `yaml:"style,omitempty"`
	Label string `yaml:"label,omitempty"`
}

// Parse decodes raw YAML bytes into a Document. Validation is minimal —
// the renderer treats whatever it gets as the source of truth and the
// golden test surfaces any drift.
func Parse(data []byte) (*Document, error) {
	doc := &Document{}
	if err := yaml.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("architecture: parse yaml: %w", err)
	}
	return doc, nil
}

// Render returns the full Mermaid markdown body for doc. The output is
// suitable for writing to `docs/architecture-diagram.md`.
func Render(doc *Document) string {
	var b strings.Builder
	writeHeader(&b)
	headings := sectionHeadingIndex(doc)
	for _, sec := range doc.Sections {
		writeSection(&b, sec, headings)
	}
	return b.String()
}

func writeHeader(b *strings.Builder) {
	b.WriteString("# Architecture Diagram\n\n")
	b.WriteString("> Generated from `internal/atdd/runtime/architecture/architecture.yaml` by `internal/atdd/runtime/architecture`. Do not edit by hand — edit the YAML and regenerate via `gh optivem architecture show > docs/architecture-diagram.md`.\n\n")
}

// sectionHeadingIndex returns a slug → heading lookup used to resolve
// `ref:` fields on nodes. A node with `ref: dsl_port` renders as
// "<label> — see § DSL Port" via this map.
func sectionHeadingIndex(doc *Document) map[string]string {
	out := make(map[string]string, len(doc.Sections))
	for _, sec := range doc.Sections {
		out[sec.Name] = sec.Heading
	}
	return out
}

func writeSection(b *strings.Builder, sec Section, headings map[string]string) {
	fmt.Fprintf(b, "## %s\n\n", sec.Heading)
	dir := sec.Direction
	if dir == "" {
		dir = "TD"
	}
	fmt.Fprintf(b, "```mermaid\nflowchart %s\n", dir)
	for _, n := range sec.Nodes {
		writeNode(b, n, headings)
	}
	if len(sec.Edges) > 0 {
		b.WriteString("\n")
	}
	for _, e := range sec.Edges {
		writeEdge(b, e)
	}
	b.WriteString("```\n\n")
}

// writeNode emits one Mermaid node line. Labels are emitted verbatim
// inside `[...]` brackets. When ref is set and resolves to a known
// section heading, the label gets a " — see § <heading>" suffix.
func writeNode(b *strings.Builder, n Node, headings map[string]string) {
	label := n.Label
	if n.Ref != "" {
		heading := headings[n.Ref]
		if heading == "" {
			heading = n.Ref
		}
		label = fmt.Sprintf("%s — see § %s", label, heading)
	}
	fmt.Fprintf(b, "    %s[%s]\n", n.ID, label)
}

// writeEdge emits one Mermaid edge line. Three style/label
// combinations are recognised:
//
//	directional plain      A --> B
//	directional + label    A -->|label| B
//	undirected             A --- B
//
// Undirected + label is not a syntax the source diagram uses and is
// rejected at render time — the renderer treats it as a YAML authoring
// bug, not a feature.
func writeEdge(b *strings.Builder, e Edge) {
	switch e.Style {
	case "", "directional":
		if e.Label != "" {
			fmt.Fprintf(b, "    %s -->|%s| %s\n", e.From, e.Label, e.To)
		} else {
			fmt.Fprintf(b, "    %s --> %s\n", e.From, e.To)
		}
	case "undirected":
		fmt.Fprintf(b, "    %s --- %s\n", e.From, e.To)
	default:
		fmt.Fprintf(b, "    # unknown edge style %q for %s -> %s\n", e.Style, e.From, e.To)
	}
}
