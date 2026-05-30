package statemachine

import (
	"fmt"
	"maps"
	"strings"
)

// changeSystemBehaviorProcess and implementAndVerifySystemAnchor name the
// process and the single call-activity node UnrollSystemChannels rewrites.
// The anchor is the template the per-channel nodes are cloned from: its
// `params:` (task-name / action / test-names) carry over verbatim, and only
// the channel-specific keys (channel / common / suite) are overridden.
const (
	changeSystemBehaviorProcess     = "change-system-behavior"
	implementAndVerifySystemAnchor  = "IMPLEMENT_AND_VERIFY_SYSTEM"
	implementAndVerifySystemProcess = "implement-and-verify-system"
)

// UnrollSystemChannels statically unrolls the channel loop in the
// change-system-behavior CYCLE (plan 20260530-1702 Item 4, A2 per D7). The
// channel set is project-declared (config `channels:`), so the unroll cannot
// live in the shared static process-flow.yaml — it is a load-time,
// in-memory rewrite the driver applies once per run from the loaded config.
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
//     only its own acceptance suite. Cumulative 0..K verification (the
//     original D6) was dropped as overkill: a later channel regressing an
//     earlier one is caught by the next cycle's verify, and the CLI rejects
//     `--test` alongside multiple explicit `--suite` values anyway.
//
// The synthesized chain is strictly linear (predecessor → ch0 → ch1 → … →
// successor) with NO loopback edge, so it stays a terminating DAG and never
// stresses the maxDispatchesPerProcess backstop (run.go).
//
// Idempotency / drift guard: the rewrite requires the anchor node to be
// present with exactly one incoming and one outgoing edge. Calling twice (the
// anchor is gone after the first call) or against a process whose shape has
// drifted returns an error rather than silently mis-rewriting. An empty
// channel list is rejected — the driver only calls this when config declares
// a non-empty `channels:`; the absent-channels path skips the call entirely
// and keeps the single static node (today's `suite: acceptance` behaviour).
func (e *Engine) UnrollSystemChannels(channels []string) error {
	if len(channels) == 0 {
		return fmt.Errorf("unroll channels: empty channel list")
	}
	proc, ok := e.Processes[changeSystemBehaviorProcess]
	if !ok {
		return fmt.Errorf("unroll channels: process %q not found", changeSystemBehaviorProcess)
	}
	anchor, ok := proc.Nodes[implementAndVerifySystemAnchor]
	if !ok {
		return fmt.Errorf("unroll channels: process %q has no %q node to unroll (already unrolled, or template drifted)",
			changeSystemBehaviorProcess, implementAndVerifySystemAnchor)
	}
	if anchor.Raw.Process != implementAndVerifySystemProcess {
		return fmt.Errorf("unroll channels: anchor %q calls %q, expected %q",
			implementAndVerifySystemAnchor, anchor.Raw.Process, implementAndVerifySystemProcess)
	}

	// The anchor sits on a linear segment: exactly one edge in, one out.
	// Anything else means the template shape changed and the rewrite's
	// re-stitching assumptions no longer hold.
	pred, succ, err := linearNeighbours(proc, implementAndVerifySystemAnchor)
	if err != nil {
		return fmt.Errorf("unroll channels: %w", err)
	}

	// Build one call-activity per channel, cloning the anchor's params and
	// overriding only the channel-specific keys.
	channelNodes := make([]Node, 0, len(channels))
	for i, ch := range channels {
		id := implementAndVerifySystemAnchor + "_" + strings.ToUpper(ch)
		if _, dup := proc.Nodes[id]; dup {
			return fmt.Errorf("unroll channels: synthesized node id %q already exists", id)
		}
		params := make(map[string]string, len(anchor.Raw.Params)+3)
		maps.Copy(params, anchor.Raw.Params)
		params["channel"] = ch
		params["common"] = boolParam(i == 0)
		params["suite"] = "acceptance-" + ch

		raw := anchor.Raw
		raw.ID = id
		raw.Name = fmt.Sprintf("Implement System (%s)", strings.ToUpper(ch))
		raw.Params = params
		channelNodes = append(channelNodes, Node{ID: id, Kind: CallActivity, Raw: raw})
	}

	// Swap the anchor out for the per-channel nodes.
	delete(proc.Nodes, implementAndVerifySystemAnchor)
	for _, n := range channelNodes {
		proc.Nodes[n.ID] = n
	}

	// Re-stitch edges: drop the two edges that touched the anchor, then wire
	// pred → ch0 → ch1 → … → chN-1 → succ in declared channel order.
	rebuilt := make([]Edge, 0, len(proc.Edges)+len(channelNodes))
	for _, ed := range proc.Edges {
		if ed.From == implementAndVerifySystemAnchor || ed.To == implementAndVerifySystemAnchor {
			continue
		}
		rebuilt = append(rebuilt, ed)
	}
	prevID := pred
	for _, n := range channelNodes {
		rebuilt = append(rebuilt, Edge{From: prevID, To: n.ID})
		prevID = n.ID
	}
	rebuilt = append(rebuilt, Edge{From: prevID, To: succ})

	proc.Edges = rebuilt
	proc.OutgoingByNode = indexOutgoing(rebuilt)
	return nil
}

// linearNeighbours returns the sole predecessor and successor of node id,
// erroring unless it has exactly one incoming and one outgoing edge.
func linearNeighbours(proc *Process, id string) (pred, succ string, err error) {
	var preds, succs []string
	for _, ed := range proc.Edges {
		if ed.To == id {
			preds = append(preds, ed.From)
		}
		if ed.From == id {
			succs = append(succs, ed.To)
		}
	}
	if len(preds) != 1 {
		return "", "", fmt.Errorf("node %q must have exactly one incoming edge to unroll, found %d", id, len(preds))
	}
	if len(succs) != 1 {
		return "", "", fmt.Errorf("node %q must have exactly one outgoing edge to unroll, found %d", id, len(succs))
	}
	return preds[0], succs[0], nil
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
