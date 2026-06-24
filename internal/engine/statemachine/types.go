// The package doc — including the LoadBytes reuse contract — lives in doc.go.
package statemachine

// NodeKind enumerates the BPMN-shaped node types supported by the YAML schema.
// The vocabulary is BPMN's; the runtime is hand-coded and does not embed a
// BPMN engine.
type NodeKind int

const (
	StartEvent NodeKind = iota
	EndEvent
	ErrorEndEvent // exceptional exit; terminates the process with an error (BPMN error end event)
	ServiceTask   // mechanical action; auto-executed via the actions registry
	UserTask      // creative work; dispatches an agent (or blocks on human input)
	Gateway       // XOR decision; binding evaluated via the gates registry
	CallActivity  // embedded sub-process; runs to completion, returns to caller
)

// Outcome is what every NodeFn returns. The fields are union-style and only
// the relevant one is populated:
//
//   - Bool: gateway result for boolean bindings (when: "x == true").
//   - Value: gateway result for enum/string bindings (when: "x == story").
//   - Err: stop the run and surface this error to the user.
//
// Predicate evaluation reads Value (string-coerced) and Bool, in that order;
// see predicate.go.
type Outcome struct {
	Bool  bool
	Value string
	Err   error
}

// NodeFn is the body of a node. It receives the live Context and returns an
// Outcome. Routing decisions live in the edge list, not inside NodeFn — this
// keeps gateway nodes single-purpose ("compute one boolean") and lets the
// transitions test suite assert routing without mocking node bodies.
type NodeFn func(*Context) Outcome

// Node is a parsed YAML node bound to its NodeFn. Bindings are resolved at
// load time via the registries (gates, actions, agents).
type Node struct {
	ID    string
	Kind  NodeKind
	Fn    NodeFn
	Raw   RawNode // the original YAML record, retained for diagnostics
}

// Edge is a directed sequence flow between two nodes, optionally guarded by a
// predicate over the Context state map.
//
// When the predicate is empty (no `when:` clause in YAML), the edge is
// unconditionally true. Run picks the first edge whose predicate matches in
// declared order, so YAML authors should make `when:` clauses mutually
// exclusive (the BPMN exclusive-gateway rule).
type Edge struct {
	From      string
	To        string
	Predicate string // raw expression text from `when:`, "" means always-true
}

// Process is one named process definition (main, at_cycle, ct_subprocess, …),
// loaded from the YAML and bound to NodeFns.
//
// ID is the kebab-case identifier (the YAML map key under `processes:`),
// used for cross-process references like call-activity `process:` targets
// and as the link text on the diagram `see § <id>` suffix. Name is the
// human-readable display label from the YAML `name:` field, used as the
// diagram section heading. Authors set both explicitly; there is no
// auto-Title-Case fallback (per plan 20260526-1730 Item 4).
//
// DiagramSectionOrder, when positive, causes the expanded diagram renderer to
// emit this process as a top-level section at that position (ascending order).
//
// DiagramNoInlineExpand prevents the expanded diagram renderer from expanding
// this process as an inline subgraph inside a parent section — it renders as a
// plain "see § id" reference box instead.
type Process struct {
	ID                    string
	Name                  string
	Start                 string
	PresetState           map[string]any    // state values written at process entry before the first node runs (preset-state:)
	Outputs               []OutputSpec // structured output contract for writing-agent MIDs (key, type, optional)
	DiagramSectionOrder   int          // 0 = not a section; positive = top-level section at this position
	DiagramNoInlineExpand bool         // never expand inline as a subgraph in the expanded diagram
	Nodes                 map[string]Node
	Edges                 []Edge
	OutgoingByNode        map[string][]Edge // index for nextEdge lookup
}

// OutputSpec is one entry in a writing-agent MID's `outputs:` contract —
// the BPMN-level declaration of a structured value the dispatched agent
// must (or may) emit via `gh optivem output write KEY=VALUE`. The
// declaration is the single source of truth for three downstream
// consumers: (1) post-RUN presence-check in validate-outputs-and-scopes,
// (2) the GH_OPTIVEM_OUTPUT_KEYS env var that lets the `output write`
// CLI reject unknown keys, (3) the auto-injected `${expected_outputs}`
// prompt section that tells the agent what to emit.
//
// Key is the ctx.State key the value flattens into. Type is the
// coercion contract: "string", "bool", or "string-list". Optional
// means absence is allowed — the post-RUN validator does not fire
// missing-output for an unemitted optional key.
type OutputSpec struct {
	Key      string `yaml:"key"`
	Type     string `yaml:"type"`
	Optional bool   `yaml:"optional,omitempty"`
}

// Valid output types. Mirrors the allow-list in output_commands.go's
// coerceOutputValue — kept in lockstep so the BPMN parser rejects a
// type the CLI cannot coerce.
const (
	OutputTypeString     = "string"
	OutputTypeBool       = "bool"
	OutputTypeStringList = "string-list"
)

// Universal scope-exception envelope keys (plan 20260528-1150). Every
// prod-agent dispatch exposes these two keys via the structured-output
// channel — the runtime injects them when the dispatching MID's
// `outputs:` block does not already declare them, so an agent can emit
// `gh optivem output write scope-exception-files=...` even from a MID
// with no per-MID flag outputs (implement-system, update-system, the
// driver-adapter implementer / updater MIDs, …). The prompt-side
// statement lives in internal/atdd/assets/runtime/shared/scope.md.
const (
	EnvelopeKeyScopeExceptionFiles  = "scope-exception-files"
	EnvelopeKeyScopeExceptionReason = "scope-exception-reason"
)

// EnvelopeOutputSpecs returns a fresh slice of the universal
// scope-exception envelope contract. Both keys are Optional — an agent
// that completes its phase in-scope emits neither and the post-RUN
// presence check passes. Callers receive a fresh slice each call so
// concat-onto-MID-declared-outputs is safe without aliasing.
func EnvelopeOutputSpecs() []OutputSpec {
	return []OutputSpec{
		{Key: EnvelopeKeyScopeExceptionFiles, Type: OutputTypeStringList, Optional: true},
		{Key: EnvelopeKeyScopeExceptionReason, Type: OutputTypeString, Optional: true},
	}
}

// Engine holds every loaded Process plus the registries needed to dispatch
// nodes. Run picks a process by name and walks it.
type Engine struct {
	Processes map[string]*Process

	// Registries — set during Load by binding string references in the YAML
	// (action:, agent:, binding:) to runtime functions.
	GateFn   func(name string) NodeFn
	ActionFn func(name string) NodeFn
	AgentFn  func(name string) NodeFn
}

// Outputs returns the structured output contract declared on the named
// writing-agent MID process. Returns (nil, false) when the process does
// not exist; an empty slice with ok=true when the process exists but
// declares no outputs (no-op for the dispatcher's env-var export and
// validation).
//
// Looked up by the dispatcher via phaseTaskName (the MID's process ID =
// task-name verb), mirroring Engine.Scope. For fix-* recovery dispatches
// whose task-name is the dynamic `fix-${failure-kind}`, the inner
// validate-outputs-and-scopes uses `originating-task-name` so the outer
// MID's output contract still applies — same pattern as scope.
func (e *Engine) Outputs(processName string) ([]OutputSpec, bool) {
	proc, exists := e.Processes[processName]
	if !exists {
		return nil, false
	}
	return proc.Outputs, true
}

// Scope returns the per-phase read / write scope lists for the named
// writing-agent MID process. The convention (per plan 20260526-1536) is
// that scope lives inline on the EXECUTE_AGENT call-activity node of each
// writing-agent MID (write-acceptance-tests, implement-system, …), so the
// accessor looks up Processes[processName] and returns the Read / Write
// fields of its EXECUTE_AGENT node.
//
// Returns ok == false when the named process does not exist, when it has
// no EXECUTE_AGENT call-activity node (e.g. command-only MIDs like
// compile, build-system, run-tests, commit), or when that node carries
// neither Read nor Write (e.g. refine-acceptance-criteria — declares
// `scope: none` on the BPMN node; check IsScopeNone separately).
func (e *Engine) Scope(processName string) (read, write []string, ok bool) {
	proc, exists := e.Processes[processName]
	if !exists {
		return nil, nil, false
	}
	for _, node := range proc.Nodes {
		if node.Kind != CallActivity || node.Raw.Process != "execute-agent" {
			continue
		}
		if len(node.Raw.Read) == 0 && len(node.Raw.Write) == 0 {
			return nil, nil, false
		}
		return append([]string(nil), node.Raw.Read...), append([]string(nil), node.Raw.Write...), true
	}
	return nil, nil, false
}

// ScopeRationale returns the optional free-form rationale string declared
// on the EXECUTE_AGENT call-activity node of the named writing-agent MID
// process — the per-MID *why* that explains the shape of the `read:` /
// `write:` lists (most commonly: why a key is in `write:` but not in
// `read:`). The dispatcher renders it under the auto-derived "Write-only
// paths" annotation in ${scope-block}; absence is the common case.
//
// Returns ok == false when the named process does not exist, has no
// EXECUTE_AGENT call-activity node, or declares no rationale. Mirrors
// the lookup pattern of Scope / Outputs / IsScopeNone.
func (e *Engine) ScopeRationale(processName string) (string, bool) {
	proc, exists := e.Processes[processName]
	if !exists {
		return "", false
	}
	for _, node := range proc.Nodes {
		if node.Kind != CallActivity || node.Raw.Process != "execute-agent" {
			continue
		}
		if node.Raw.ScopeRationale == "" {
			return "", false
		}
		return node.Raw.ScopeRationale, true
	}
	return "", false
}

// IsScopeNone reports whether the named writing-agent MID declares
// `scope: none` on its EXECUTE_AGENT call-activity node — the doctrinal
// artifact-only exemption (agent mutates only an inter-phase artifact or
// external system, never the repo working tree; see runtime/shared/scope.md).
// The BPMN node is the SSoT for the exemption (plan 20260526-1448 Item 9
// moved it off the prompt frontmatter), and IsScopeNone is the canonical
// reader.
func (e *Engine) IsScopeNone(processName string) bool {
	proc, exists := e.Processes[processName]
	if !exists {
		return false
	}
	for _, node := range proc.Nodes {
		if node.Kind != CallActivity || node.Raw.Process != "execute-agent" {
			continue
		}
		return node.Raw.Scope == "none"
	}
	return false
}
