package main

import "testing"

func TestKickoffCmdShape(t *testing.T) {
	cmd := newKickoffCmd()
	for _, flag := range []string{"description", "description-file", "feedback", "approve", "redo", "autopilot", "status"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("missing --%s", flag)
		}
	}
	phase, _, err := cmd.Find([]string{"phase"})
	if err != nil || phase.Name() != "phase" || !phase.Hidden {
		t.Fatalf("phase subcommand must exist and be hidden: %v", err)
	}
}

func TestKickoffRegisteredOnRoot(t *testing.T) {
	cmd, _, err := newRootCmd().Find([]string{"kickoff"})
	if err != nil || cmd.Name() != "kickoff" {
		t.Fatalf("kickoff not registered: %v", err)
	}
}
