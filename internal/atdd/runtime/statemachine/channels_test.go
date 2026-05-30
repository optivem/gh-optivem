// Tests for the project-declared channel unroll (plan 20260530-1702 Item 4).
// The transform rewrites change-system-behavior's single template node into
// one call-activity per channel; these assert the resulting graph shape,
// the per-channel params, the inherited template params, and the drift
// guards. They load the embedded snapshot (via loadSnapshot) and unroll in
// memory — the static YAML is never edited, so the snapshot itself stays the
// single-node template (see TestLoadSnapshot_AllProcessesParse).
package statemachine

import "testing"

func TestUnrollSystemChannels_TwoChannels(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemChannels: %v", err)
	}
	proc := eng.Processes[changeSystemBehaviorProcess]
	if proc == nil {
		t.Fatalf("process %q missing", changeSystemBehaviorProcess)
	}
	if _, ok := proc.Nodes[implementAndVerifySystemAnchor]; ok {
		t.Errorf("template anchor %q should be gone after unroll", implementAndVerifySystemAnchor)
	}

	api := requireNode(t, proc, "IMPLEMENT_AND_VERIFY_SYSTEM_API")
	ui := requireNode(t, proc, "IMPLEMENT_AND_VERIFY_SYSTEM_UI")

	if api.Kind != CallActivity {
		t.Errorf("API node kind = %v, want CallActivity", api.Kind)
	}
	if api.Raw.Process != implementAndVerifySystemProcess {
		t.Errorf("API node calls %q, want %q", api.Raw.Process, implementAndVerifySystemProcess)
	}
	if api.Raw.Name != "Implement System (API)" {
		t.Errorf("API node name = %q, want %q", api.Raw.Name, "Implement System (API)")
	}

	// Channel-specific params (overridden per channel).
	checkParam(t, api, "channel", "api")
	checkParam(t, api, "common", "true") // first channel builds the common layer (D5)
	checkParam(t, api, "suite", "acceptance-api")
	checkParam(t, ui, "channel", "ui")
	checkParam(t, ui, "common", "false") // later channels: adapter delta only
	checkParam(t, ui, "suite", "acceptance-ui")

	// Params inherited verbatim from the template anchor — including the
	// unexpanded ${test-names} placeholder (expansion happens at dispatch).
	for _, n := range []Node{api, ui} {
		checkParam(t, n, "action", "implement-system")
		checkParam(t, n, "task-name", "implement-system")
		checkParam(t, n, "test-names", "${test-names}")
	}

	// Linear chain: WRITE → API → UI → REFACTOR, no loopback.
	assertSingleEdge(t, proc, "WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL", "IMPLEMENT_AND_VERIFY_SYSTEM_API")
	assertSingleEdge(t, proc, "IMPLEMENT_AND_VERIFY_SYSTEM_API", "IMPLEMENT_AND_VERIFY_SYSTEM_UI")
	assertSingleEdge(t, proc, "IMPLEMENT_AND_VERIFY_SYSTEM_UI", "REFACTOR_OPPORTUNISTICALLY")
}

func TestUnrollSystemChannels_SingleChannel(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemChannels([]string{"api"}); err != nil {
		t.Fatalf("UnrollSystemChannels: %v", err)
	}
	proc := eng.Processes[changeSystemBehaviorProcess]
	if _, ok := proc.Nodes["IMPLEMENT_AND_VERIFY_SYSTEM_UI"]; ok {
		t.Error("no UI node expected for a single-channel [api] project")
	}
	api := requireNode(t, proc, "IMPLEMENT_AND_VERIFY_SYSTEM_API")
	checkParam(t, api, "common", "true") // the sole channel still builds the common layer
	checkParam(t, api, "suite", "acceptance-api")

	// The sole channel stitches straight from predecessor to successor.
	assertSingleEdge(t, proc, "WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL", "IMPLEMENT_AND_VERIFY_SYSTEM_API")
	assertSingleEdge(t, proc, "IMPLEMENT_AND_VERIFY_SYSTEM_API", "REFACTOR_OPPORTUNISTICALLY")
}

func TestUnrollSystemChannels_Guards(t *testing.T) {
	t.Run("empty channel list", func(t *testing.T) {
		eng := loadSnapshot(t)
		if err := eng.UnrollSystemChannels(nil); err == nil {
			t.Error("want error for an empty channel list")
		}
	})

	t.Run("double unroll", func(t *testing.T) {
		eng := loadSnapshot(t)
		if err := eng.UnrollSystemChannels([]string{"api"}); err != nil {
			t.Fatalf("first unroll: %v", err)
		}
		if err := eng.UnrollSystemChannels([]string{"api"}); err == nil {
			t.Error("second unroll should error — the template anchor is consumed by the first")
		}
	})
}

// TestUnrollSystemChannels_BindsEndToEnd proves the synthesized per-channel
// nodes are valid CallActivity nodes the engine can bind — a regression guard
// against producing a node the resolve step rejects.
func TestUnrollSystemChannels_BindsEndToEnd(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemChannels: %v", err)
	}
	stub := func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.ActionFn, eng.AgentFn, eng.GateFn = stub, stub, stub
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind after unroll: %v — synthesized per-channel nodes should resolve", err)
	}
}

func requireNode(t *testing.T, proc *Process, id string) Node {
	t.Helper()
	n, ok := proc.Nodes[id]
	if !ok {
		t.Fatalf("expected synthesized node %q in %q", id, proc.ID)
	}
	return n
}

func checkParam(t *testing.T, n Node, key, want string) {
	t.Helper()
	if got := n.Raw.Params[key]; got != want {
		t.Errorf("node %q param %q = %q, want %q", n.ID, key, got, want)
	}
}

func assertSingleEdge(t *testing.T, proc *Process, from, to string) {
	t.Helper()
	edges := proc.OutgoingByNode[from]
	if len(edges) != 1 {
		t.Fatalf("node %q should have exactly one outgoing edge, found %d", from, len(edges))
	}
	if edges[0].To != to {
		t.Errorf("node %q edge → %q, want → %q", from, edges[0].To, to)
	}
}
