package driver

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

// TestTargetSlices_ProcessesExistInEmbeddedYAML is the load-bearing
// cross-check between the Item 1 mapping and the Item 2a seam-extraction: every
// process the --target table points at must be a real named sub-process in the
// canonical embedded process-flow.yaml. A rename or deletion in the YAML that
// drifts from targetSlices fails here rather than at an operator's first
// `gh optivem implement --target …`.
func TestTargetSlices_ProcessesExistInEmbeddedYAML(t *testing.T) {
	eng, err := statemachine.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}
	for target, spec := range targetSlices {
		if _, ok := eng.Processes[spec.process]; !ok {
			t.Errorf("--target %q maps to process %q, which is not in the embedded YAML", target, spec.process)
		}
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		raw     string
		want    Target
		wantErr bool
	}{
		{"", TargetUnset, false},
		{"test", TargetTest, false},
		{"driver-adapter", TargetDriverAdapter, false},
		{"system", TargetSystem, false},
		{"bogus", TargetUnset, true},
		{"TEST", TargetUnset, true},  // case-sensitive: enum values are lowercase canon
		{"tests", TargetUnset, true}, // near-miss, not the plural
	}
	for _, tc := range cases {
		got, err := ParseTarget(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseTarget(%q): want error, got nil", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTarget(%q): unexpected error: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Errorf("ParseTarget(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestTarget_SliceProcess pins the mapping values, including the channel rule
// (channel-split targets require --channel; the agnostic `test` slice rejects
// it) and the TargetUnset = full-run-not-a-slice contract.
func TestTarget_SliceProcess(t *testing.T) {
	cases := []struct {
		target         Target
		wantProcess    string
		wantReqChannel bool
		wantOK         bool
	}{
		{TargetTest, "shared-contract", false, true},
		{TargetDriverAdapter, "implement-and-verify-system-driver-adapters", true, true},
		{TargetSystem, "implement-and-verify-system", true, true},
		{TargetUnset, "", false, false},
	}
	for _, tc := range cases {
		process, reqChannel, ok := tc.target.SliceProcess()
		if ok != tc.wantOK {
			t.Errorf("%q.SliceProcess() ok = %v, want %v", tc.target, ok, tc.wantOK)
		}
		if process != tc.wantProcess {
			t.Errorf("%q.SliceProcess() process = %q, want %q", tc.target, process, tc.wantProcess)
		}
		if reqChannel != tc.wantReqChannel {
			t.Errorf("%q.SliceProcess() requiresChannel = %v, want %v", tc.target, reqChannel, tc.wantReqChannel)
		}
	}
}

// TestScopedTargets_CoversMap guards against ScopedTargets (the help/usage
// list) and targetSlices (the behaviour map) drifting apart — every scoped
// target must be in the map, and every map key must be listed.
func TestScopedTargets_CoversMap(t *testing.T) {
	if len(ScopedTargets) != len(targetSlices) {
		t.Fatalf("ScopedTargets has %d entries, targetSlices has %d", len(ScopedTargets), len(targetSlices))
	}
	for _, target := range ScopedTargets {
		if _, ok := targetSlices[target]; !ok {
			t.Errorf("ScopedTargets lists %q, which is not in targetSlices", target)
		}
	}
}
