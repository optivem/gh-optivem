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
	// unexpanded ${at-test-names} placeholder (expansion happens at dispatch).
	// The behavioral GREEN reads the AT-cascade-namespaced key so a nested
	// contract excursion can't clobber the selection (plan 20260608-1231).
	for _, n := range []Node{api, ui} {
		checkParam(t, n, "action", "implement-system")
		checkParam(t, n, "task-name", "implement-system")
		checkParam(t, n, "test-names", "${at-test-names}")
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

// --- System Driver adapter unroll (plan 20260530-1725 Item 0) ----------------
//
// Same transform as the system unroll, but on the RED
// write-and-verify-acceptance-tests cascade's adapter step, whose predecessor
// is the GATE_SYSTEM_DRIVER_PORTS_CHANGED gateway. These assert the per-channel
// shape, the channel-only param override (no common / suite), and — the new
// wrinkle — that the gateway's TRUE-branch `when:` predicate is preserved on the
// edge into the first channel so the per-channel block stays gated.

const (
	sysDriverAdapterAnchorAPI = "IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS_API"
	sysDriverAdapterAnchorUI  = "IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS_UI"
	sysDriverPortChangedGate  = "GATE_SYSTEM_DRIVER_PORTS_CHANGED"
)

func TestUnrollSystemDriverAdapterChannels_TwoChannels(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemDriverAdapterChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemDriverAdapterChannels: %v", err)
	}
	proc := eng.Processes[writeAndVerifyAcceptanceTestsProcess]
	if proc == nil {
		t.Fatalf("process %q missing", writeAndVerifyAcceptanceTestsProcess)
	}
	if _, ok := proc.Nodes[implementSystemDriverAdaptersAnchor]; ok {
		t.Errorf("template anchor %q should be gone after unroll", implementSystemDriverAdaptersAnchor)
	}

	api := requireNode(t, proc, sysDriverAdapterAnchorAPI)
	ui := requireNode(t, proc, sysDriverAdapterAnchorUI)

	if api.Kind != CallActivity {
		t.Errorf("API node kind = %v, want CallActivity", api.Kind)
	}
	if api.Raw.Process != implementAndVerifySystemDriverAdaptersProcess {
		t.Errorf("API node calls %q, want %q", api.Raw.Process, implementAndVerifySystemDriverAdaptersProcess)
	}
	if api.Raw.Name != "Implement System Driver Adapters (API)" {
		t.Errorf("API node name = %q, want %q", api.Raw.Name, "Implement System Driver Adapters (API)")
	}

	// `channel` and `suite` are overridden — the driver adapter is channel-
	// shaped, so each node verifies ONLY its own channel (acceptance-<ch>)
	// instead of inheriting the union `acceptance` and re-running every channel.
	// There is still no common layer (channel-shaped by nature).
	checkParam(t, api, "channel", "api")
	checkParam(t, ui, "channel", "ui")
	checkParam(t, api, "suite", "acceptance-api")
	checkParam(t, ui, "suite", "acceptance-ui")
	if _, ok := api.Raw.Params["common"]; ok {
		t.Errorf("adapter node should not carry a common param, got %q", api.Raw.Params["common"])
	}

	// Params inherited verbatim from the template anchor (including the
	// unexpanded ${expected-test-result} placeholder — expansion is at dispatch).
	for _, n := range []Node{api, ui} {
		checkParam(t, n, "task-name", "implement-system-driver-adapters")
		checkParam(t, n, "test-category", "acceptance")
		checkParam(t, n, "expected-test-result", "${expected-test-result}")
	}

	// The gateway TRUE-branch predicate is preserved on the edge into the
	// first channel, so the per-channel adapter block still runs only when the
	// system driver port changed (no-arg full-run behaviour intact).
	entry := findEdge(t, proc, sysDriverPortChangedGate, sysDriverAdapterAnchorAPI)
	if entry.Predicate != "at-system-driver-port-changed == true" {
		t.Errorf("gate → first-channel edge predicate = %q, want %q", entry.Predicate, "at-system-driver-port-changed == true")
	}
	// The gateway FALSE branch (skip the whole block) survives untouched.
	skip := findEdge(t, proc, sysDriverPortChangedGate, "WAV_AT_END")
	if skip.Predicate != "at-system-driver-port-changed == false" {
		t.Errorf("gate skip edge predicate = %q, want %q", skip.Predicate, "at-system-driver-port-changed == false")
	}

	// Linear chain api → ui → WAV_AT_END, no loopback; intermediate edge
	// unconditional.
	chain := findEdge(t, proc, sysDriverAdapterAnchorAPI, sysDriverAdapterAnchorUI)
	if chain.Predicate != "" {
		t.Errorf("intermediate api→ui edge should be unconditional, got predicate %q", chain.Predicate)
	}
	assertSingleEdge(t, proc, sysDriverAdapterAnchorAPI, sysDriverAdapterAnchorUI)
	assertSingleEdge(t, proc, sysDriverAdapterAnchorUI, "WAV_AT_END")
}

func TestUnrollSystemDriverAdapterChannels_SingleChannel(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemDriverAdapterChannels([]string{"api"}); err != nil {
		t.Fatalf("UnrollSystemDriverAdapterChannels: %v", err)
	}
	proc := eng.Processes[writeAndVerifyAcceptanceTestsProcess]
	if _, ok := proc.Nodes[sysDriverAdapterAnchorUI]; ok {
		t.Error("no UI node expected for a single-channel [api] project")
	}
	api := requireNode(t, proc, sysDriverAdapterAnchorAPI)
	checkParam(t, api, "channel", "api")
	checkParam(t, api, "suite", "acceptance-api")

	// Sole channel: gate TRUE → the one adapter node (predicate preserved),
	// then straight to WAV_AT_END.
	entry := findEdge(t, proc, sysDriverPortChangedGate, sysDriverAdapterAnchorAPI)
	if entry.Predicate != "at-system-driver-port-changed == true" {
		t.Errorf("gate → adapter edge predicate = %q, want %q", entry.Predicate, "at-system-driver-port-changed == true")
	}
	assertSingleEdge(t, proc, sysDriverAdapterAnchorAPI, "WAV_AT_END")
}

func TestUnrollSystemDriverAdapterChannels_Guards(t *testing.T) {
	t.Run("empty channel list", func(t *testing.T) {
		eng := loadSnapshot(t)
		if err := eng.UnrollSystemDriverAdapterChannels(nil); err == nil {
			t.Error("want error for an empty channel list")
		}
	})

	t.Run("double unroll", func(t *testing.T) {
		eng := loadSnapshot(t)
		if err := eng.UnrollSystemDriverAdapterChannels([]string{"api"}); err != nil {
			t.Fatalf("first unroll: %v", err)
		}
		if err := eng.UnrollSystemDriverAdapterChannels([]string{"api"}); err == nil {
			t.Error("second unroll should error — the template anchor is consumed by the first")
		}
	})
}

// TestUnrollBothChannels_BindsEndToEnd proves the two unrolls compose — the
// driver runs both per run — and the combined graph still binds.
func TestUnrollBothChannels_BindsEndToEnd(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemChannels: %v", err)
	}
	if err := eng.UnrollSystemDriverAdapterChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemDriverAdapterChannels: %v", err)
	}
	stub := func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.ActionFn, eng.AgentFn, eng.GateFn = stub, stub, stub
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind after both unrolls: %v — synthesized per-channel nodes should resolve", err)
	}
}

// --- External-system unroll (plan 20260615-0755) -----------------------------
//
// Same transform as the channel unrolls, on the shared-contract external
// driver-adapter contract anchor. Its predecessor is the upfront
// VALIDATE_EXTERNAL_SYSTEMS_REGISTERED node (unconditional edge — the
// GATE_EXTERNAL_DRIVER_PORTS_CHANGED true-branch predicate sits on GATE →
// VALIDATE), so the seam into the first clone is unconditional. Each clone bakes
// its own external-system-name + real-kind; the per-clone touched-guard lives
// INSIDE the cycle, not here.

const (
	extAdapterAnchorERP   = "IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS_ERP"
	extAdapterAnchorCLOCK = "IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS_CLOCK"
	extValidateNode       = "VALIDATE_EXTERNAL_SYSTEMS_REGISTERED"
	extTerminalGate       = "GATE_AT_TERMINAL_GREEN"
)

func TestUnrollExternalSystems_TwoSystems(t *testing.T) {
	eng := loadSnapshot(t)
	realKind := map[string]string{"erp": "simulator", "clock": "test-instance"}
	if err := eng.UnrollExternalSystems([]string{"erp", "clock"}, realKind); err != nil {
		t.Fatalf("UnrollExternalSystems: %v", err)
	}
	proc := eng.Processes[sharedContractProcess]
	if proc == nil {
		t.Fatalf("process %q missing", sharedContractProcess)
	}
	if _, ok := proc.Nodes[implementExternalDriverAdaptersAnchor]; ok {
		t.Errorf("template anchor %q should be gone after unroll", implementExternalDriverAdaptersAnchor)
	}

	erp := requireNode(t, proc, extAdapterAnchorERP)
	clock := requireNode(t, proc, extAdapterAnchorCLOCK)

	if erp.Kind != CallActivity {
		t.Errorf("ERP node kind = %v, want CallActivity", erp.Kind)
	}
	if erp.Raw.Process != implementAndVerifyExternalDriverAdaptersProc {
		t.Errorf("ERP node calls %q, want %q", erp.Raw.Process, implementAndVerifyExternalDriverAdaptersProc)
	}

	// Per-system baked params: name + real-kind looked up at unroll.
	checkParam(t, erp, "external-system-name", "erp")
	checkParam(t, erp, "real-kind", "simulator")
	checkParam(t, clock, "external-system-name", "clock")
	checkParam(t, clock, "real-kind", "test-instance")

	// Params inherited verbatim from the template anchor.
	for _, n := range []Node{erp, clock} {
		checkParam(t, n, "test-category", "contract")
		checkParam(t, n, "verify-mode", "red")
	}

	// Seam into the first clone is unconditional (the entry predicate sits on
	// GATE → VALIDATE, upstream of the anchor).
	entry := findEdge(t, proc, extValidateNode, extAdapterAnchorERP)
	if entry.Predicate != "" {
		t.Errorf("validate → first-clone edge should be unconditional, got predicate %q", entry.Predicate)
	}
	// Linear chain erp → clock → terminal gate, no loopback.
	chain := findEdge(t, proc, extAdapterAnchorERP, extAdapterAnchorCLOCK)
	if chain.Predicate != "" {
		t.Errorf("intermediate erp→clock edge should be unconditional, got predicate %q", chain.Predicate)
	}
	assertSingleEdge(t, proc, extAdapterAnchorERP, extAdapterAnchorCLOCK)
	assertSingleEdge(t, proc, extAdapterAnchorCLOCK, extTerminalGate)
}

func TestUnrollExternalSystems_SingleSystem(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollExternalSystems([]string{"erp"}, map[string]string{"erp": "simulator"}); err != nil {
		t.Fatalf("UnrollExternalSystems: %v", err)
	}
	proc := eng.Processes[sharedContractProcess]
	if _, ok := proc.Nodes[extAdapterAnchorCLOCK]; ok {
		t.Error("no CLOCK node expected for a single-system [erp] project")
	}
	erp := requireNode(t, proc, extAdapterAnchorERP)
	checkParam(t, erp, "external-system-name", "erp")
	checkParam(t, erp, "real-kind", "simulator")
	// Sole system stitches validate → clone → terminal gate.
	assertSingleEdge(t, proc, extValidateNode, extAdapterAnchorERP)
	assertSingleEdge(t, proc, extAdapterAnchorERP, extTerminalGate)
}

func TestUnrollExternalSystems_Guards(t *testing.T) {
	t.Run("empty system list", func(t *testing.T) {
		eng := loadSnapshot(t)
		if err := eng.UnrollExternalSystems(nil, nil); err == nil {
			t.Error("want error for an empty external-system list")
		}
	})

	t.Run("double unroll", func(t *testing.T) {
		eng := loadSnapshot(t)
		rk := map[string]string{"erp": "simulator"}
		if err := eng.UnrollExternalSystems([]string{"erp"}, rk); err != nil {
			t.Fatalf("first unroll: %v", err)
		}
		if err := eng.UnrollExternalSystems([]string{"erp"}, rk); err == nil {
			t.Error("second unroll should error — the template anchor is consumed by the first")
		}
	})
}

// TestUnrollExternalSystems_BindsEndToEnd proves the synthesized per-system
// clones are valid CallActivity nodes the engine can bind, and that the
// external unroll composes with the channel unrolls (different processes).
func TestUnrollExternalSystems_BindsEndToEnd(t *testing.T) {
	eng := loadSnapshot(t)
	if err := eng.UnrollSystemChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemChannels: %v", err)
	}
	if err := eng.UnrollSystemDriverAdapterChannels([]string{"api", "ui"}); err != nil {
		t.Fatalf("UnrollSystemDriverAdapterChannels: %v", err)
	}
	if err := eng.UnrollExternalSystems([]string{"erp", "clock"}, map[string]string{"erp": "simulator", "clock": "test-instance"}); err != nil {
		t.Fatalf("UnrollExternalSystems: %v", err)
	}
	stub := func(name string) NodeFn { return func(ctx *Context) Outcome { return Outcome{} } }
	eng.ActionFn, eng.AgentFn, eng.GateFn = stub, stub, stub
	if err := eng.Bind(); err != nil {
		t.Fatalf("Bind after all unrolls: %v — synthesized per-system clones should resolve", err)
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

// findEdge returns the from→to edge, failing if absent. Unlike assertSingleEdge
// it tolerates a source with several outgoing edges (e.g. a gateway), so the
// caller can inspect a specific branch's predicate.
func findEdge(t *testing.T, proc *Process, from, to string) Edge {
	t.Helper()
	for _, ed := range proc.OutgoingByNode[from] {
		if ed.To == to {
			return ed
		}
	}
	t.Fatalf("expected edge %q → %q in %q", from, to, proc.ID)
	return Edge{}
}
