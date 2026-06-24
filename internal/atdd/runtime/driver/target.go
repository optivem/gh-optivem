package driver

import (
	"fmt"
	"strings"
)

// Target is the `gh optivem implement --target <value>` slice selector (plan
// 20260530-1725, D-flags). A Target names a contiguous run of pipeline phases
// — a "slice" — that one team produces and hands off to the next via commit.
// It is a *refinement* of the no-arg default, never a separate mode: omit
// --target and you get TargetUnset, which walks the whole pipeline from
// DefaultProcessName exactly as today.
//
// The selector (plan Item 2b) maps a Target to a named sub-process and enters
// it via Engine.RunProcess; the flag layer (Item 4) parses/validates the raw
// flag through ParseTarget and enforces the --channel rule SliceProcess
// reports. This file is the single source of truth for that mapping.
type Target string

const (
	// TargetUnset is the no-arg default — the full pipeline (DefaultProcessName).
	// Omitting --target yields this. It is not in targetSlices: the full run
	// enters `main`, not a slice.
	TargetUnset Target = ""

	// TargetTest is the shared, channel-AGNOSTIC contract slice the whole team
	// mobs: acceptance tests + DSL port/core + driver port + external-system
	// driver adapters (D-external). Ends RED by design (no system, no
	// per-channel driver adapter yet). --channel is rejected.
	TargetTest Target = "test"

	// TargetDriverAdapter is one channel's test-side System Driver adapter
	// slice (D-adapter-ownership option A — each channel team owns its own
	// adapter). Channel-split: --channel is required.
	TargetDriverAdapter Target = "driver-adapter"

	// TargetSystem is one channel's system slice (+ the channel-agnostic common
	// layer on the first channel, D-common option b). Channel-split: --channel
	// is required.
	TargetSystem Target = "system"
)

// sliceSpec maps a scoped Target to the named process the selector enters and
// whether --channel applies to it.
type sliceSpec struct {
	// process is the process-flow.yaml sub-process Engine.RunProcess enters for
	// this slice. Every value here MUST exist in the loaded engine —
	// target_test.go cross-checks the set against the embedded YAML so a rename
	// in process-flow.yaml that drifts from this table fails the build.
	process string

	// requiresChannel encodes the D-flags --channel rule for this slice:
	//   true  → channel-split   → --channel REQUIRED  (rejected if absent)
	//   false → channel-agnostic → --channel REJECTED (rejected if present)
	// The flag layer (Item 4) branches on this single boolean for both checks.
	requiresChannel bool

	// expectedTestResult is the per-slice expected-red / expected-green gate
	// (plan Item 3, D-red-gate), expressed as the `expected-test-result`
	// call-activity param the slice is entered with. It is NOT a new gate: the
	// slice's existing verify nodes (GATE_EXPECTED_TEST_RESULT → verify-tests-
	// fail / verify-tests-pass) already enforce it via the `test-outcome`
	// classification, so seeding the right value here IS the success gate.
	//   - "failure" → expected-RED: `test` (port-deep, no system/adapter yet)
	//     and `driver-adapter <ch>` (compiles, ATs fail for the assertion
	//     reason, not compile) both end red by design.
	//   - "success" → channel-green: `system <ch>` drives its channel's
	//     acceptance suite to pass.
	// The no-arg full run pins this through its own wrappers (write-and-verify-
	// acceptance-tests-fail / change-system-behavior), so the field is consulted
	// only on scoped entry. ParseTarget / the selector read it via
	// Target.ExpectedTestResult.
	expectedTestResult string
}

// targetSlices is the Target → slice mapping. TargetUnset is intentionally
// absent (the no-arg full run is not a slice). `test` → the channel-agnostic
// write-acceptance-tests-and-dsl slice extracted in Item 2a; the two
// channel-split targets reuse the existing per-channel-unrolled sub-processes
// (system driver adapter and system).
var targetSlices = map[Target]sliceSpec{
	TargetTest:          {process: "write-acceptance-tests-and-dsl", requiresChannel: false, expectedTestResult: "failure"},
	TargetDriverAdapter: {process: "implement-and-verify-system-driver-adapters", requiresChannel: true, expectedTestResult: "failure"},
	TargetSystem:        {process: "implement-and-verify-system", requiresChannel: true, expectedTestResult: "success"},
}

// ScopedTargets lists the non-default --target values in CLI / help order, for
// flag validation and usage strings (Item 4). TargetUnset is excluded.
var ScopedTargets = []Target{TargetTest, TargetDriverAdapter, TargetSystem}

// ParseTarget validates a raw --target flag value. Empty → TargetUnset (the
// no-arg full-pipeline default). A recognised scoped value → its Target. Any
// other value → an error naming the valid set. This is the single validation
// point shared by the --flag path and any interactive path (Item 4).
func ParseTarget(raw string) (Target, error) {
	if raw == "" {
		return TargetUnset, nil
	}
	if _, ok := targetSlices[Target(raw)]; ok {
		return Target(raw), nil
	}
	return TargetUnset, fmt.Errorf("unknown --target %q (valid: %s)", raw, strings.Join(scopedTargetNames(), ", "))
}

// SliceProcess returns the named sub-process that Target t enters and whether
// --channel is required for it. ok is false for TargetUnset (the no-arg full
// run enters DefaultProcessName, not a slice) and for any unrecognised value.
func (t Target) SliceProcess() (process string, requiresChannel bool, ok bool) {
	spec, ok := targetSlices[t]
	return spec.process, spec.requiresChannel, ok
}

// ExpectedTestResult returns the `expected-test-result` param the slice is
// entered with (plan Item 3 / D-red-gate) — "failure" for the expected-red
// slices, "success" for the channel-green system slice. ok is false for
// TargetUnset and any unrecognised value (the no-arg full run pins the param
// through its own wrappers, not through this accessor). See sliceSpec.
func (t Target) ExpectedTestResult() (string, bool) {
	spec, ok := targetSlices[t]
	return spec.expectedTestResult, ok
}

// scopedTargetNames renders ScopedTargets as their raw string values, for the
// ParseTarget error message and flag-usage strings.
func scopedTargetNames() []string {
	names := make([]string, len(ScopedTargets))
	for i, t := range ScopedTargets {
		names[i] = string(t)
	}
	return names
}
