package main

import (
	"strings"
	"testing"
)

// TestNewCharacterizeCmd_Wiring: a top-level, argument-free command with the
// shared driver flags and no ticket flags.
func TestNewCharacterizeCmd_Wiring(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"characterize"})
	if err != nil || cmd == nil || cmd.Name() != "characterize" {
		t.Fatalf("root cmd has no top-level characterize (found=%v, err=%v)", cmd, err)
	}
	if cmd.GroupID != "project" {
		t.Errorf("GroupID = %q, want project (beside kickoff)", cmd.GroupID)
	}
	for _, name := range []string{"headless", "manual-agents", "workspace", "log-file", "log-level", "verbose", "keep-runs", "keep-run-history", "show-prompt"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	for _, name := range []string{"issue", "target", "channel", "autonomous"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("unexpected ticket/deprecated flag --%s", name)
		}
	}
	if err := cmd.Args(cmd, []string{"42"}); err == nil {
		t.Error("Args should reject a positional argument")
	}
	if err := cmd.Args(cmd, nil); err != nil {
		t.Errorf("Args should accept no arguments, got %v", err)
	}
	if strings.Contains(cmd.Long+cmd.Example, "shop") {
		t.Error("help text must use myorg/myrepo-style placeholders, not shop")
	}
}
