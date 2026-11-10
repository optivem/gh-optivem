package main

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestNewSystemCmd_HasStatusChild verifies that `gh optivem system status`
// is wired into the parent alongside the other lifecycle verbs. Cobra
// alphabetizes Commands() for help output regardless of AddCommand order, so
// this only asserts the set, not the order.
func TestNewSystemCmd_HasStatusChild(t *testing.T) {
	t.Parallel()
	parent := newSystemCmd()
	have := map[string]bool{}
	for _, c := range parent.Commands() {
		have[c.Use] = true
	}
	for _, want := range []string{"build", "clean", "compile", "start", "status", "stop"} {
		if !have[want] {
			t.Errorf("missing `system %s` child (have: %v)", want, have)
		}
	}
}

// TestNewSystemStatusCmd_HasExpectedFlagsAndUse: thin wiring check — the
// `status` command must declare --timeout and use "status" as its bare verb
// so the noun-first surface (`gh optivem system status`) holds.
func TestNewSystemStatusCmd_HasExpectedFlagsAndUse(t *testing.T) {
	t.Parallel()
	cmd := newSystemStatusCmd()
	if cmd.Use != "status" {
		t.Errorf("Use: got %q, want %q", cmd.Use, "status")
	}
	if cmd.Short == "" {
		t.Error("Short: must be non-empty (parent help lists this)")
	}
	if cmd.Flags().Lookup("timeout") == nil {
		t.Error("missing flag --timeout")
	}
}

// TestNewSystemStartCmd_HasExternalOnlyFlag: `system start` declares
// --external-only (default off) alongside --restart, and the help mentions it.
func TestNewSystemStartCmd_HasExternalOnlyFlag(t *testing.T) {
	t.Parallel()
	cmd := newSystemStartCmd()
	f := cmd.Flags().Lookup("external-only")
	if f == nil {
		t.Fatal("missing flag --external-only")
	}
	if f.DefValue != "false" {
		t.Errorf("--external-only default: got %q, want false", f.DefValue)
	}
	if cmd.Flags().Lookup("restart") == nil {
		t.Error("missing flag --restart")
	}
	if !strings.Contains(cmd.Example, "--restart --external-only") {
		t.Errorf("Example must show --restart --external-only: %q", cmd.Example)
	}
	if !strings.Contains(cmd.Long, "--external-only") {
		t.Error("Long must document --external-only")
	}
}

// TestStandInPathsFromConfig merges stub + simulator registry paths, deduped
// and sorted; an absent registry yields none (caller then errors).
func TestStandInPathsFromConfig(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{ExternalSystems: projectconfig.ExternalSystems{
		"erp": {RealKind: projectconfig.RealKindSimulator,
			Stub:      projectconfig.ExternalSpec{Path: "external-systems/stubs"},
			Simulator: projectconfig.ExternalSpec{Path: "external-systems/simulators"}},
		"clock": {RealKind: projectconfig.RealKindTestInstance,
			Stub: projectconfig.ExternalSpec{Path: "external-systems/stubs"}},
	}}
	got := standInPathsFromConfig(cfg)
	want := []string{"external-systems/simulators", "external-systems/stubs"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
	if got := standInPathsFromConfig(&projectconfig.Config{}); len(got) != 0 {
		t.Errorf("empty registry: got %v, want none", got)
	}
}
