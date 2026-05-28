package main

import (
	"testing"
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
