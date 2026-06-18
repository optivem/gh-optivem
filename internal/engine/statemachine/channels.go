package statemachine

import (
	"fmt"
	"maps"
	"strings"
)

// The constants below name the (process, anchor, expected-callee) triples the
// two channel unrolls rewrite. Each anchor is a single template call-activity
// node that unrollChannelAnchor replaces with one cloned node per channel; its
// `params:` carry over verbatim and only the channel-specific keys are
// overridden by the caller-supplied perChannelParams.
const (
	// System GREEN step — change-system-behavior's IMPLEMENT_AND_VERIFY_SYSTEM
	// (plan 20260530-1702 Item 4).
	changeSystemBehaviorProcess     = "change-system-behavior"
	implementAndVerifySystemAnchor  = "IMPLEMENT_AND_VERIFY_SYSTEM"
	implementAndVerifySystemProcess = "implement-and-verify-system"

	// System Driver Adapter step — write-and-verify-acceptance-tests'
	// IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS (plan 20260530-1725 Item 0).
	// Unlike the system anchor, this one sits on the TRUE branch of the
	// GATE_SYSTEM_DRIVER_PORTS_CHANGED gateway, so its incoming edge carries a
	// `when:` predicate the unroll must preserve (see unrollAnchor).
	writeAndVerifyAcceptanceTestsProcess          = "write-and-verify-acceptance-tests"
	implementSystemDriverAdaptersAnchor           = "IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS"
	implementAndVerifySystemDriverAdaptersProcess = "implement-and-verify-system-driver-adapters"

	// External System driver-adapter contract cycle — shared-contract's
	// IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS anchor (plan 20260615-0755).
	// External systems are project-declared (config `external-systems:`), so —
	// like channels — the anchor unrolls into one cloned call-activity per
	// registered external system at load time. The anchor sits on the TRUE
	// branch of GATE_EXTERNAL_DRIVER_PORTS_CHANGED (via the upfront
	// VALIDATE_EXTERNAL_SYSTEMS_REGISTERED node), so the unroll preserves the
	// seam predicate just like the System Driver adapter anchor.
	sharedContractProcess                         = "shared-contract"
	implementExternalDriverAdaptersAnchor         = "IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS"
	implementAndVerifyExternalDriverAdaptersProc  = "implement-and-verify-external-system-driver-adapters-contract-tests"
)

// UnrollSystemChannels statically unrolls the channel loop in the
// change-system-behavior CYCLE (plan 20260530-1702 Item 4, A2 per D7). The
// channel set is project-declared (config `channels:`), so the unroll cannot
// live in the shared static process-flow.yaml — it is a load-time, in-memory
// rewrite the driver applies once per run from the loaded config.
//
// The single IMPLEMENT_AND_VERIFY_SYSTEM template node is replaced by one
// call-activity per channel, each invoking the *unchanged* static
// implement-and-verify-system chain with caller-bound params:
//
//   - channel: the channel token (api / ui) — threads through the call-
//     activity param push (run.go) into the implement-system agent prompt
//     via NodeParams (driver.go), making the dispatch channel-aware.
//   - common:  "true" on the FIRST channel only (D5) — that dispatch builds
//     the channel-agnostic common layer (DTO / entity / service / migration)
//     plus its adapter; later channels ("false") add only their adapter
//     delta, so the common layer and its migration are not re-paid.
//   - suite:   acceptance-<channel> (D1 selector) — each channel verifies
//     only its own acceptance partitions. `acceptance-<channel>` is the
//     per-channel GROUP alias (testselect.AcceptanceSuites / defaultSuiteGroups),
//     so the CLI expands it to BOTH acceptance-parallel-<channel> and
//     acceptance-isolated-<channel> — the isolated partition can never be
//     silently dropped from the per-channel verify. The literal is kept here
//     (rather than importing testselect) because this generic engine hardcodes
//     the convention; the group expansion happens downstream in the CLI.
func (e *Engine) UnrollSystemChannels(channels []string) error {
	return e.unrollAnchor(
		changeSystemBehaviorProcess,
		implementAndVerifySystemAnchor,
		implementAndVerifySystemProcess,
		channels,
		func(i int, ch string, anchorParams map[string]string) map[string]string {
			params := make(map[string]string, len(anchorParams)+4)
			maps.Copy(params, anchorParams)
			params["channel"] = ch
			params["common"] = boolParam(i == 0)
			params["suite"] = "acceptance-" + ch
			// Per-channel commit-label discriminator (e.g. " (api)") appended to
			// the hard-coded "SYSTEM" label at COMMIT_SYSTEM, so each channel's
			// system commit reads distinctly. The static anchor binds "".
			params["layer-suffix"] = fmt.Sprintf(" (%s)", ch)
			return params
		},
		func(ch string) string {
			return fmt.Sprintf("Implement System (%s)", strings.ToUpper(ch))
		},
	)
}

// UnrollSystemDriverAdapterChannels statically unrolls the System Driver
// adapter step in the RED write-and-verify-acceptance-tests cascade into one
// dispatch per channel (plan 20260530-1725 Item 0, D-adapter-ownership option
// A). The test-side driver adapter is inherently channel-specific (each
// channel owns its own adapter folder under system-driver-adapter, configured
// per channel via system-test.system-driver-adapter-channels.<ch>), so
// like the system step (1702) it must become one node per channel for the
// channel team to own its slice.
//
// `channel` and `suite` are overridden per node — the adapter has no common
// layer (it is channel-shaped by nature), and its verify suite is narrowed to
// `acceptance-<channel>` so each per-channel node verifies ONLY its own
// channel. `acceptance-<channel>` is the per-channel GROUP alias, so it still
// expands to BOTH that channel's partitions (parallel + isolated) downstream —
// the narrowing is across channels, never across a channel's own partitions.
// Previously each clone inherited the union `suite: acceptance`, so
// for N channels every node re-ran all N suites (2N suite runs for 2 channels)
// — pure redundancy: the adapter is channel-specific, so a node only needs to
// exercise its own channel, and each channel's RED is still verified exactly
// once. (The channel-AGNOSTIC layers — the DSL verify and the test-code
// pass/fail verifies — deliberately KEEP the union: there the second channel
// is the only guard that catches a per-channel vacuous/false-green acceptance
// test at RED, so it is real signal, not redundancy.) The agent prompt
// (system-driver-adapter-implementer.md) reads `${channel}` and writes only
// that channel's adapter, leaving the other channels' stubs to their own
// dispatch.
//
// The anchor sits on the TRUE branch of GATE_SYSTEM_DRIVER_PORTS_CHANGED, so
// the rewrite preserves that `when:` predicate on the edge into the first
// channel node — the gate stays in force (the whole per-channel block runs
// only when the driver port changed), which keeps the no-arg full run's
// behaviour intact. The unroll is strictly linear (gate → ch0 → … → chN-1 →
// WAV_AT_END), no loopback.
func (e *Engine) UnrollSystemDriverAdapterChannels(channels []string) error {
	return e.unrollAnchor(
		writeAndVerifyAcceptanceTestsProcess,
		implementSystemDriverAdaptersAnchor,
		implementAndVerifySystemDriverAdaptersProcess,
		channels,
		func(_ int, ch string, anchorParams map[string]string) map[string]string {
			params := make(map[string]string, len(anchorParams)+3)
			maps.Copy(params, anchorParams)
			params["channel"] = ch
			params["suite"] = "acceptance-" + ch
			// Per-channel commit-label discriminator (e.g. " (api)") appended to
			// the hard-coded "SYSTEM DRIVER ADAPTERS" label, so each channel's
			// adapter commit reads distinctly. The static anchor binds "".
			params["layer-suffix"] = fmt.Sprintf(" (%s)", ch)
			return params
		},
		func(ch string) string {
			return fmt.Sprintf("Implement System Driver Adapters (%s)", strings.ToUpper(ch))
		},
	)
}

// UnrollExternalSystems statically unrolls the external-system driver-adapter
// contract cycle (plan 20260615-0755). External systems are project-declared
// (config `external-systems:`), so — exactly like channels — the single
// IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS anchor in shared-contract is
// replaced at load time by one cloned call-activity per registered external
// system, each invoking the unchanged contract cycle with caller-bound params:
//
//   - external-system-name: the registry key (erp / clock) — threads through
//     the call-activity param push (run.go) into the cycle, where the
//     resolve-external-system service-task reads it to (a) self-guard the clone
//     against the touched-system set and (b) drive the per-clone real-kind
//     gate; it also reaches the three writing agents' prompts as
//     ${external-system-name}.
//   - real-kind: cfg.ExternalSystems[name].RealKind, baked at load time so the
//     enum check is a static, analyzable value. resolve-external-system copies
//     it into gate-readable state for GATE_CONTRACT_REAL_RED_KIND.
//
// Unlike channels — which are *always* exercised — a ticket touches only a
// subset of external systems, so each clone is guarded INSIDE the cycle (by
// resolve-external-system + GATE_EXTERNAL_SYSTEM_TOUCHED) rather than by a
// per-clone gateway here: a gateway preceding the clone cannot read the clone's
// baked params (call-activity params land in ctx.Params only at sub-process
// entry, run.go). An untouched clone no-ops to its skip end-event; the linear
// chain then advances to the next clone.
//
// realKind maps each name to its real-kind string; a missing entry bakes "" and
// surfaces at the GATE_CONTRACT_REAL_RED_KIND enum check. The driver calls this
// only when len(cfg.ExternalSystems) > 0, so the empty-list guard in unrollAnchor
// is never reached here.
func (e *Engine) UnrollExternalSystems(names []string, realKind map[string]string) error {
	return e.unrollAnchor(
		sharedContractProcess,
		implementExternalDriverAdaptersAnchor,
		implementAndVerifyExternalDriverAdaptersProc,
		names,
		func(_ int, name string, anchorParams map[string]string) map[string]string {
			params := make(map[string]string, len(anchorParams)+2)
			maps.Copy(params, anchorParams)
			params["external-system-name"] = name
			params["real-kind"] = realKind[name]
			return params
		},
		func(name string) string {
			return fmt.Sprintf("Implement and Verify External System Driver Adapters Contract Tests (%s)", name)
		},
	)
}

// unrollAnchor replaces a single template call-activity node (anchorID)
// in process procName with one cloned call-activity per item, stitched
// linearly (pred → c0 → c1 → … → cN-1 → succ). The anchor must call
// expectProcess and sit on a linear segment (exactly one edge in, one out).
// "item" is a channel for the channel unrolls and a registered external
// system for UnrollExternalSystems — the rewrite is identical either way.
//
// perItemParams builds each clone's params from the anchor's params and the
// (index, item); nameFor builds each clone's display name. Only the keys
// perItemParams overrides differ between callers — the rest of the rewrite
// (clone, drift guards, edge re-stitching) is shared by the channel and
// external-system unrolls.
//
// Edge predicates are preserved at the seams: the incoming edge's `when:`
// clause moves onto pred → ch0, and the outgoing edge's onto chN-1 → succ; the
// intermediate ch_i → ch_{i+1} edges are unconditional. This matters for the
// driver-adapter anchor, whose predecessor is a gateway whose TRUE branch must
// keep guarding the (now per-channel) adapter block. For the system anchor
// both seam edges are unconditional, so predicate preservation is a no-op
// there — the rewrite stays backward-compatible.
//
// Idempotency / drift guard: the rewrite requires the anchor node to be
// present with exactly one incoming and one outgoing edge. Calling twice (the
// anchor is gone after the first call) or against a process whose shape has
// drifted returns an error rather than silently mis-rewriting. An empty
// item list is rejected — the driver only calls this when config declares a
// non-empty `channels:` / `external-systems:`; the absent path skips the call
// entirely and keeps the single static node.
func (e *Engine) unrollAnchor(
	procName, anchorID, expectProcess string,
	items []string,
	perItemParams func(i int, item string, anchorParams map[string]string) map[string]string,
	nameFor func(item string) string,
) error {
	if len(items) == 0 {
		return fmt.Errorf("unroll anchor: empty item list")
	}
	proc, ok := e.Processes[procName]
	if !ok {
		return fmt.Errorf("unroll anchor: process %q not found", procName)
	}
	anchor, ok := proc.Nodes[anchorID]
	if !ok {
		return fmt.Errorf("unroll anchor: process %q has no %q node to unroll (already unrolled, or template drifted)",
			procName, anchorID)
	}
	if anchor.Raw.Process != expectProcess {
		return fmt.Errorf("unroll anchor: anchor %q calls %q, expected %q",
			anchorID, anchor.Raw.Process, expectProcess)
	}

	// The anchor sits on a linear segment: exactly one edge in, one out.
	// Anything else means the template shape changed and the rewrite's
	// re-stitching assumptions no longer hold.
	inEdge, outEdge, err := linearNeighbours(proc, anchorID)
	if err != nil {
		return fmt.Errorf("unroll anchor: %w", err)
	}

	// Build one call-activity per item, cloning the anchor and overriding
	// only the item-specific params.
	itemNodes := make([]Node, 0, len(items))
	for i, item := range items {
		id := anchorID + "_" + strings.ToUpper(item)
		if _, dup := proc.Nodes[id]; dup {
			return fmt.Errorf("unroll anchor: synthesized node id %q already exists", id)
		}
		raw := anchor.Raw
		raw.ID = id
		raw.Name = nameFor(item)
		raw.Params = perItemParams(i, item, anchor.Raw.Params)
		itemNodes = append(itemNodes, Node{ID: id, Kind: CallActivity, Raw: raw})
	}

	// Swap the anchor out for the per-item nodes.
	delete(proc.Nodes, anchorID)
	for _, n := range itemNodes {
		proc.Nodes[n.ID] = n
	}

	// Re-stitch edges: drop the two edges that touched the anchor, then wire
	// pred → c0 → c1 → … → cN-1 → succ in declared item order, carrying
	// the original seam predicates so any gateway guard on entry/exit survives.
	rebuilt := make([]Edge, 0, len(proc.Edges)+len(itemNodes))
	for _, ed := range proc.Edges {
		if ed.From == anchorID || ed.To == anchorID {
			continue
		}
		rebuilt = append(rebuilt, ed)
	}
	prevID := inEdge.From
	for i, n := range itemNodes {
		edge := Edge{From: prevID, To: n.ID}
		if i == 0 {
			edge.Predicate = inEdge.Predicate
		}
		rebuilt = append(rebuilt, edge)
		prevID = n.ID
	}
	rebuilt = append(rebuilt, Edge{From: prevID, To: outEdge.To, Predicate: outEdge.Predicate})

	proc.Edges = rebuilt
	proc.OutgoingByNode = indexOutgoing(rebuilt)
	return nil
}

// linearNeighbours returns the sole incoming and outgoing edge of node id,
// erroring unless it has exactly one of each. Returning the edges (not just
// the neighbour IDs) lets the unroll preserve any `when:` predicate on the
// seam edges.
func linearNeighbours(proc *Process, id string) (in, out Edge, err error) {
	var ins, outs []Edge
	for _, ed := range proc.Edges {
		if ed.To == id {
			ins = append(ins, ed)
		}
		if ed.From == id {
			outs = append(outs, ed)
		}
	}
	if len(ins) != 1 {
		return Edge{}, Edge{}, fmt.Errorf("node %q must have exactly one incoming edge to unroll, found %d", id, len(ins))
	}
	if len(outs) != 1 {
		return Edge{}, Edge{}, fmt.Errorf("node %q must have exactly one outgoing edge to unroll, found %d", id, len(outs))
	}
	return ins[0], outs[0], nil
}

// indexOutgoing rebuilds the by-source-node edge index after an edge-list
// rewrite, mirroring buildProcess's OutgoingByNode construction.
func indexOutgoing(edges []Edge) map[string][]Edge {
	idx := make(map[string][]Edge)
	for _, ed := range edges {
		idx[ed.From] = append(idx[ed.From], ed)
	}
	return idx
}

// boolParam renders a Go bool as the lowercase string the BPMN param layer
// carries (params are map[string]string).
func boolParam(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
