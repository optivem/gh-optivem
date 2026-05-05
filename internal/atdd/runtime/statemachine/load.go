package statemachine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// RawNode mirrors the YAML node schema 1:1, preserving every field for both
// loading and downstream diagnostics (Mermaid generation, doctor checks).
// Optional fields use omitempty so the zero value is "absent in YAML".
type RawNode struct {
	ID          string            `yaml:"id"`
	Type        string            `yaml:"type"`
	Action      string            `yaml:"action,omitempty"`
	Agent       string            `yaml:"agent,omitempty"`
	Binding     string            `yaml:"binding,omitempty"`
	Flow        string            `yaml:"flow,omitempty"`
	PhaseDoc    string            `yaml:"phase_doc,omitempty"`
	Description string            `yaml:"description,omitempty"`
	Role        string            `yaml:"role,omitempty"`
	Group       string            `yaml:"group,omitempty"`
	Params      map[string]string `yaml:"params,omitempty"`
}

// rawEdge mirrors the YAML sequence_flow schema. `When` carries the raw
// predicate text; evaluation lives in predicate.go.
type rawEdge struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	When string `yaml:"when,omitempty"`
}

// rawFlow mirrors one named flow.
type rawFlow struct {
	Start         string    `yaml:"start"`
	Outputs       []string  `yaml:"outputs,omitempty"`
	Nodes         []RawNode `yaml:"nodes"`
	SequenceFlows []rawEdge `yaml:"sequence_flows"`
}

// rawSpec is the top-level YAML document.
type rawSpec struct {
	Flows map[string]rawFlow `yaml:"flows"`
}

// LoadFile reads a process-flow YAML document from disk and returns an Engine
// whose Flows are linked but whose registries are nil. Wire registries
// afterwards with engine.Bind* before calling Run.
//
// Loading is structural only — node bodies (NodeFn) are not resolved until
// Bind is called for each registry. This split lets transitions tests assert
// graph shape without supplying real action/agent/gate implementations.
func LoadFile(path string) (*Engine, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read process-flow file: %w", err)
	}
	return LoadBytes(data)
}

// LoadBytes parses a process-flow YAML document from an in-memory byte slice.
// Useful for tests that want to load a fixture from testdata/ via os.ReadFile
// or //go:embed.
func LoadBytes(data []byte) (*Engine, error) {
	var raw rawSpec
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse process-flow YAML: %w", err)
	}
	if len(raw.Flows) == 0 {
		return nil, fmt.Errorf("process-flow YAML defines no flows under `flows:`")
	}

	eng := &Engine{Flows: map[string]*Flow{}}
	for name, rf := range raw.Flows {
		flow, err := buildFlow(name, rf)
		if err != nil {
			return nil, err
		}
		eng.Flows[name] = flow
	}
	return eng, nil
}

// buildFlow turns one rawFlow into a Flow with a node map, edge list, and
// outgoing-edges-by-source-node index. NodeFn is left nil — Bind* fills it.
func buildFlow(name string, rf rawFlow) (*Flow, error) {
	if rf.Start == "" {
		return nil, fmt.Errorf("flow %q: missing `start:`", name)
	}
	flow := &Flow{
		Name:           name,
		Start:          rf.Start,
		Outputs:        append([]string(nil), rf.Outputs...),
		Nodes:          make(map[string]Node, len(rf.Nodes)),
		Edges:          make([]Edge, 0, len(rf.SequenceFlows)),
		OutgoingByNode: make(map[string][]Edge),
	}
	for _, rn := range rf.Nodes {
		if rn.ID == "" {
			return nil, fmt.Errorf("flow %q: node missing `id:`", name)
		}
		if _, dup := flow.Nodes[rn.ID]; dup {
			return nil, fmt.Errorf("flow %q: duplicate node id %q", name, rn.ID)
		}
		kind, err := parseKind(rn.Type)
		if err != nil {
			return nil, fmt.Errorf("flow %q node %q: %w", name, rn.ID, err)
		}
		flow.Nodes[rn.ID] = Node{ID: rn.ID, Kind: kind, Raw: rn}
	}
	if _, ok := flow.Nodes[rf.Start]; !ok {
		return nil, fmt.Errorf("flow %q: start node %q not in nodes list", name, rf.Start)
	}
	for _, re := range rf.SequenceFlows {
		if _, ok := flow.Nodes[re.From]; !ok {
			return nil, fmt.Errorf("flow %q: edge from unknown node %q", name, re.From)
		}
		if _, ok := flow.Nodes[re.To]; !ok {
			return nil, fmt.Errorf("flow %q: edge to unknown node %q", name, re.To)
		}
		edge := Edge{From: re.From, To: re.To, Predicate: re.When}
		flow.Edges = append(flow.Edges, edge)
		flow.OutgoingByNode[re.From] = append(flow.OutgoingByNode[re.From], edge)
	}
	return flow, nil
}

// parseKind maps the YAML `type:` string onto a NodeKind.
func parseKind(s string) (NodeKind, error) {
	switch s {
	case "start_event":
		return StartEvent, nil
	case "end_event":
		return EndEvent, nil
	case "service_task":
		return ServiceTask, nil
	case "user_task":
		return UserTask, nil
	case "gateway":
		return Gateway, nil
	case "call_activity":
		return CallActivity, nil
	default:
		return 0, fmt.Errorf("unknown node type %q", s)
	}
}
