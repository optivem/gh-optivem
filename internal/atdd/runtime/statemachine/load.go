package statemachine

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/optivem/gh-optivem/internal/approval"
)

// RawNode mirrors the YAML node schema 1:1, preserving every field for both
// loading and downstream diagnostics (Mermaid generation, doctor checks).
// Optional fields use omitempty so the zero value is "absent in YAML".
//
// Read / Write are the per-phase scope lists folded inline (per plan
// 20260526-1536). They sit on the writing-agent EXECUTE_AGENT call-activity
// nodes inside each MID process; Engine.Scope(processName) looks them up.
// Both lists are always declared together when present (no flat shorthand).
//
// Scope is the doctrinal-exemption marker: `scope: none` on the EXECUTE_AGENT
// node declares the MID as artifact-only (mutates only inter-phase artifacts
// or external systems, never the repo working tree). Mutually exclusive with
// Read / Write — `scope: none` MUST be paired with absent read/write lists,
// and any other value is rejected by the build-time guard. Replaces the
// pre-fold `scope: none` frontmatter mechanism (plan 20260526-1448 Item 9).
type RawNode struct {
	ID       string            `yaml:"id"`
	Type     string            `yaml:"type"`
	Action   string            `yaml:"action,omitempty"`
	Agent    string            `yaml:"agent,omitempty"`
	Binding  string            `yaml:"binding,omitempty"`
	Process  string            `yaml:"process,omitempty"`
	Name     string            `yaml:"name,omitempty"`
	Role     string            `yaml:"role,omitempty"`
	Group    string            `yaml:"group,omitempty"`
	TDDStage string            `yaml:"tdd-stage,omitempty"`
	Params   map[string]string `yaml:"params,omitempty"`
	Scope    string            `yaml:"scope,omitempty"`
	Read     []string          `yaml:"read,omitempty"`
	Write    []string          `yaml:"write,omitempty"`
	// Category tags an approve / human node with the approval-policy
	// vocabulary (commit | fix | release | prompt | human). The driver
	// reads it at dispatch time and routes the y/n prompt through
	// approval.Confirm so --auto + --confirm can decide whether to
	// auto-yes or prompt. Default: "prompt" on approve nodes, "human" on
	// human-STOP nodes — driver-side, not encoded here.
	Category string `yaml:"category,omitempty"`
}

// approvalPrimitives is the closed set of primitive process IDs that
// terminate in an `approve` call (either directly or via threading). Every
// call-activity targeting one of these MUST resolve `category:` to a
// literal token — either directly in its `params:` or transitively from an
// ancestor caller's `params:`. validateApprovalCategories enforces this at
// load time so unresolved sites never reach the dispatch-time fallback.
//
// `fix` is intentionally absent: it self-resolves with literal
// `category: human` on its internal approve + execute-agent call-activities,
// so its callers (execute-agent.FIX / execute-command.FIX) don't need to
// pin a category.
var approvalPrimitives = map[string]bool{
	"approve":         true,
	"execute-agent":   true,
	"execute-command": true,
	"commit":          true,
}

// rawEdge mirrors the YAML sequence-flow schema. `When` carries the raw
// predicate text; evaluation lives in predicate.go.
type rawEdge struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	When string `yaml:"when,omitempty"`
}

// rawProcess mirrors one named process.
//
// Outputs is the writing-agent MID's structured output contract — a list of
// {key, type, optional} objects declaring which values the dispatched agent
// must (or may) emit via `gh optivem output write`. The legacy string-CSV
// form ("key1,key2") is rejected at unmarshal time so no silent
// backward-compat path exists.
type rawProcess struct {
	Name          string       `yaml:"name"`
	Start         string       `yaml:"start"`
	Outputs       []OutputSpec `yaml:"outputs,omitempty"`
	Nodes         []RawNode    `yaml:"nodes"`
	SequenceFlows []rawEdge    `yaml:"sequence-flows"`
}

// rawSpec is the top-level YAML document.
type rawSpec struct {
	Processes map[string]rawProcess `yaml:"processes"`
}

// LoadFile reads a process-flow YAML document from disk and returns an Engine
// whose Processes are linked but whose registries are nil. Wire registries
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
	if len(raw.Processes) == 0 {
		return nil, fmt.Errorf("process-flow YAML defines no processes under `processes:`")
	}

	eng := &Engine{Processes: map[string]*Process{}}
	for name, rp := range raw.Processes {
		process, err := buildProcess(name, rp)
		if err != nil {
			return nil, err
		}
		eng.Processes[name] = process
	}
	if err := validateApprovalCategories(eng.Processes); err != nil {
		return nil, err
	}
	return eng, nil
}

// buildProcess turns one rawProcess into a Process with a node map, edge list,
// and outgoing-edges-by-source-node index. NodeFn is left nil — Bind* fills it.
func buildProcess(id string, rp rawProcess) (*Process, error) {
	if rp.Name == "" {
		return nil, fmt.Errorf("process %q: missing `name:` (used as the diagram section heading)", id)
	}
	if rp.Start == "" {
		return nil, fmt.Errorf("process %q: missing `start:`", id)
	}
	if err := validateOutputs(id, rp.Outputs); err != nil {
		return nil, err
	}
	process := &Process{
		ID:             id,
		Name:           rp.Name,
		Start:          rp.Start,
		Outputs:        append([]OutputSpec(nil), rp.Outputs...),
		Nodes:          make(map[string]Node, len(rp.Nodes)),
		Edges:          make([]Edge, 0, len(rp.SequenceFlows)),
		OutgoingByNode: make(map[string][]Edge),
	}
	for _, rn := range rp.Nodes {
		if rn.ID == "" {
			return nil, fmt.Errorf("process %q: node missing `id:`", id)
		}
		if _, dup := process.Nodes[rn.ID]; dup {
			return nil, fmt.Errorf("process %q: duplicate node id %q", id, rn.ID)
		}
		kind, err := parseKind(rn.Type)
		if err != nil {
			return nil, fmt.Errorf("process %q node %q: %w", id, rn.ID, err)
		}
		if err := validateNodeName(id, rn, kind); err != nil {
			return nil, err
		}
		if err := validateTDDStage(id, rn); err != nil {
			return nil, err
		}
		process.Nodes[rn.ID] = Node{ID: rn.ID, Kind: kind, Raw: rn}
	}
	if _, ok := process.Nodes[rp.Start]; !ok {
		return nil, fmt.Errorf("process %q: start node %q not in nodes list", id, rp.Start)
	}
	for _, re := range rp.SequenceFlows {
		if _, ok := process.Nodes[re.From]; !ok {
			return nil, fmt.Errorf("process %q: edge from unknown node %q", id, re.From)
		}
		if _, ok := process.Nodes[re.To]; !ok {
			return nil, fmt.Errorf("process %q: edge to unknown node %q", id, re.To)
		}
		edge := Edge{From: re.From, To: re.To, Predicate: re.When}
		process.Edges = append(process.Edges, edge)
		process.OutgoingByNode[re.From] = append(process.OutgoingByNode[re.From], edge)
	}
	return process, nil
}

// parseKind maps the YAML `type:` string onto a NodeKind.
func parseKind(s string) (NodeKind, error) {
	switch s {
	case "start-event":
		return StartEvent, nil
	case "end-event":
		return EndEvent, nil
	case "error-end-event":
		return ErrorEndEvent, nil
	case "service-task":
		return ServiceTask, nil
	case "user-task":
		return UserTask, nil
	case "gateway":
		return Gateway, nil
	case "call-activity":
		return CallActivity, nil
	default:
		return 0, fmt.Errorf("unknown node type %q", s)
	}
}

// validateNodeName enforces the plan-20260526-1730 Item-4 rule: every
// non-gateway node must carry an explicit `name:` string. The renderer
// uses that string as the visible BPMN label; no auto-Title-Case
// fallback to the screaming-snake ID, no schema-level inference.
// Gateways are exempt: an empty `name:` is allowed and the renderer
// emits a bare `{ }` diamond (Item 13). When a gateway does carry a
// `name:`, the string is treated as a human-readable display label —
// the machine key for dispatch lives in `binding:`.
func validateNodeName(processID string, rn RawNode, kind NodeKind) error {
	if kind == Gateway {
		return nil
	}
	if rn.Name == "" {
		return fmt.Errorf("process %q node %q: %s requires `name:` (used as the visible BPMN label — no fallback to the node ID)", processID, rn.ID, rn.Type)
	}
	return nil
}

// validateOutputs enforces the OutputSpec shape on a process's `outputs:`
// list: every entry must carry a non-empty `key:` and a `type:` from the
// closed enum {string, bool, string-list}. Keys must be unique within the
// list. Empty list is allowed (most processes have no structured outputs).
//
// The legacy string-CSV form ("dsl-port-changed,test-names") is rejected
// by yaml.v3 itself when it tries to unmarshal a scalar into the
// []OutputSpec slice — LoadBytes wraps that error with process context.
// This guard catches the post-unmarshal shape errors (missing key,
// missing/invalid type, duplicate keys).
func validateOutputs(processID string, outputs []OutputSpec) error {
	seen := make(map[string]bool, len(outputs))
	for i, o := range outputs {
		if o.Key == "" {
			return fmt.Errorf("process %q outputs[%d]: missing `key:`", processID, i)
		}
		switch o.Type {
		case OutputTypeString, OutputTypeBool, OutputTypeStringList:
		case "":
			return fmt.Errorf("process %q output %q: missing `type:` (want one of %s, %s, %s)",
				processID, o.Key, OutputTypeString, OutputTypeBool, OutputTypeStringList)
		default:
			return fmt.Errorf("process %q output %q: type %q is not one of %s / %s / %s",
				processID, o.Key, o.Type, OutputTypeString, OutputTypeBool, OutputTypeStringList)
		}
		if seen[o.Key] {
			return fmt.Errorf("process %q outputs: duplicate key %q", processID, o.Key)
		}
		seen[o.Key] = true
	}
	return nil
}

// validateTDDStage enforces the Item-19 enum on the optional `tdd-stage:`
// field. Absent → no styling (renderer leaves the node unstyled by TDD
// stage). Present → must be one of red / green / refactor. The renderer
// emits a coloured border per stage via classDef so an empty / unknown
// value would silently misrender.
func validateTDDStage(processName string, rn RawNode) error {
	switch rn.TDDStage {
	case "", "red", "green", "refactor":
		return nil
	default:
		return fmt.Errorf("process %q node %q: tdd-stage %q is not one of red / green / refactor", processName, rn.ID, rn.TDDStage)
	}
}

// validateApprovalCategories walks every call-activity that targets one of
// the approvalPrimitives and verifies its `category:` resolves to a known
// approval.Category token — either as a literal in the call-activity's own
// `params:` or transitively from an ancestor caller via the `${category}`
// threading chain.
//
// Defense-in-depth pair with driver.go::classifyApproveCategory: this
// catches unresolved sites at load time so the dispatch-time path never
// has to fall back. Missing/invalid sites error with the offending node
// named and the valid token set listed.
func validateApprovalCategories(processes map[string]*Process) error {
	// First pass: index, per primitive process, the call-sites that
	// supply (a) a literal category and (b) a threaded `${...}` category
	// from their own params. Keyed by the called primitive's process ID.
	literalCallers := map[string][]string{}    // primitive → list of literal category values from its callers
	threadedCallers := map[string][]string{}   // primitive → list of containing-process IDs whose call passes a placeholder
	for parentID, p := range processes {
		for _, n := range p.Nodes {
			if n.Kind != CallActivity {
				continue
			}
			if !approvalPrimitives[n.Raw.Process] {
				continue
			}
			cat := strings.TrimSpace(n.Raw.Params["category"])
			switch {
			case cat == "":
				// Will be flagged in the second pass.
			case isPlaceholder(cat):
				threadedCallers[n.Raw.Process] = append(threadedCallers[n.Raw.Process], parentID)
			default:
				literalCallers[n.Raw.Process] = append(literalCallers[n.Raw.Process], cat)
			}
		}
	}

	// Second pass: validate each call-activity.
	for parentID, p := range processes {
		for _, n := range p.Nodes {
			if n.Kind != CallActivity {
				continue
			}
			if !approvalPrimitives[n.Raw.Process] {
				continue
			}
			cat := strings.TrimSpace(n.Raw.Params["category"])
			switch {
			case cat == "":
				return fmt.Errorf("process %q node %q: call to %q has no category resolved (params.category required); valid: %s",
					parentID, n.ID, n.Raw.Process, approvalValidList())
			case isPlaceholder(cat):
				if !resolvesToLiteral(parentID, literalCallers, threadedCallers, map[string]bool{}) {
					return fmt.Errorf("process %q node %q: call to %q threads category %s but no ancestor caller of %q provides a literal category",
						parentID, n.ID, n.Raw.Process, cat, parentID)
				}
			default:
				if _, err := approval.ParseCategory(cat); err != nil {
					return fmt.Errorf("process %q node %q: call to %q: %w",
						parentID, n.ID, n.Raw.Process, err)
				}
			}
		}
	}
	return nil
}

// resolvesToLiteral returns true iff `proc` has at least one transitively-
// reachable caller chain that lands on a literal category token. Walks
// upward through the threadedCallers graph, terminating on either a
// literal-bearing caller or a cycle (no progress).
func resolvesToLiteral(proc string, literals map[string][]string, threaded map[string][]string, visited map[string]bool) bool {
	if visited[proc] {
		return false
	}
	visited[proc] = true
	for _, lit := range literals[proc] {
		if _, err := approval.ParseCategory(lit); err == nil {
			return true
		}
	}
	for _, parent := range threaded[proc] {
		if resolvesToLiteral(parent, literals, threaded, visited) {
			return true
		}
	}
	return false
}

// isPlaceholder reports whether a category value is a BPMN `${name}` param
// reference rather than a literal token.
func isPlaceholder(s string) bool {
	return strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}")
}

// approvalValidList renders the closed set of valid category tokens for
// inclusion in error messages, in iota order.
func approvalValidList() string {
	// Mirror approval.allCategories ordering by parsing-and-stringifying
	// each known tier. Hardcoded here to avoid exposing the slice from
	// the approval package.
	tokens := []string{"command", "prod-agent", "test-agent", "prod-commit", "test-commit", "human"}
	return strings.Join(tokens, ", ")
}
