package spec

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"testing"
)

func mkReport(mode string, threshold float64, cs ...ReportCriterion) Report {
	return Report{
		Mode: mode, Threshold: threshold,
		UseCases:   []ReportUseCase{{ID: "UC-001", Criteria: cs}},
		Validation: Validation{Errors: []ValidationError{}},
	}
}

func crit(id string, s State) ReportCriterion {
	return ReportCriterion{ID: id, State: s, FlagOn: true}
}

func TestGateMatrix(t *testing.T) {
	cases := []struct {
		name   string
		mode   string
		crits  []ReportCriterion
		passed bool
		exit   int
	}{
		{"lenient ignores planned", ModeLenient, []ReportCriterion{crit("UC-001.1", StatePlanned), crit("UC-001.2", StateVerified)}, true, ExitPass},
		{"lenient ignores in progress and waived", ModeLenient, []ReportCriterion{crit("UC-001.1", StateInProgress), crit("UC-001.2", StateWaived)}, true, ExitPass},
		{"lenient blocks failing", ModeLenient, []ReportCriterion{crit("UC-001.1", StateFailing)}, false, ExitGateFailed},
		{"strict blocks planned", ModeStrict, []ReportCriterion{crit("UC-001.1", StatePlanned)}, false, ExitGateFailed},
		{"strict blocks in progress", ModeStrict, []ReportCriterion{crit("UC-001.1", StateInProgress)}, false, ExitGateFailed},
		{"strict blocks failing", ModeStrict, []ReportCriterion{crit("UC-001.1", StateFailing)}, false, ExitGateFailed},
		{"strict passes all verified", ModeStrict, []ReportCriterion{crit("UC-001.1", StateVerified)}, true, ExitPass},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep := mkReport(tc.mode, 0, tc.crits...)
			if err := ApplyGate(&rep, nil); err != nil {
				t.Fatal(err)
			}
			if rep.Gate.Passed != tc.passed || rep.ExitCode() != tc.exit {
				t.Fatalf("passed=%v exit=%d reasons=%v", rep.Gate.Passed, rep.ExitCode(), rep.Gate.Reasons)
			}
		})
	}
}

func TestGateValidationErrorsBlockBothModes(t *testing.T) {
	for _, mode := range []string{ModeLenient, ModeStrict} {
		rep := mkReport(mode, 0, crit("UC-001.1", StateVerified))
		rep.Validation.Errors = []ValidationError{{"duplicate-id", "dup"}}
		if err := ApplyGate(&rep, nil); err != nil {
			t.Fatal(err)
		}
		if rep.Gate.Passed || rep.ExitCode() != ExitGateFailed {
			t.Fatalf("%s: validation error did not block", mode)
		}
	}
}

func TestGateUnknownModeAndNoVerdict(t *testing.T) {
	rep := mkReport("bogus", 0)
	if err := ApplyGate(&rep, nil); err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if rep.ExitCode() != ExitUsage {
		t.Fatalf("exit = %d, want %d", rep.ExitCode(), ExitUsage)
	}
}

func TestGateThreshold(t *testing.T) {
	cs := []ReportCriterion{crit("UC-001.1", StateVerified), crit("UC-001.2", StateVerified), crit("UC-001.3", StateVerified), crit("UC-001.4", StateWaived)}
	// Waived is in the denominator and not verified: 75%, and not a per-criterion gap.
	rep := mkReport(ModeStrict, 75, cs...)
	_ = ApplyGate(&rep, nil)
	if !rep.Gate.Passed {
		t.Fatalf("75%% should meet threshold 75: %v", rep.Gate.Reasons)
	}
	rep = mkReport(ModeStrict, 80, cs...)
	_ = ApplyGate(&rep, nil)
	if rep.Gate.Passed || len(rep.Gate.Reasons) != 1 {
		t.Fatalf("75%% should miss threshold 80: %v", rep.Gate.Reasons)
	}
	// Lenient never applies the threshold.
	rep = mkReport(ModeLenient, 100, cs...)
	_ = ApplyGate(&rep, nil)
	if !rep.Gate.Passed {
		t.Fatal("lenient must ignore threshold")
	}
}

func TestGateFlagOff(t *testing.T) {
	off := crit("UC-001.2", StatePlanned)
	f := "dark"
	off.Flag, off.FlagOn = &f, false
	rep := mkReport(ModeStrict, 100, crit("UC-001.1", StateVerified), off)
	_ = ApplyGate(&rep, nil)
	if !rep.Gate.Passed {
		t.Fatalf("flag-off criterion must be excluded: %v", rep.Gate.Reasons)
	}
	off.FlagOn = true
	rep = mkReport(ModeStrict, 100, crit("UC-001.1", StateVerified), off)
	_ = ApplyGate(&rep, nil)
	if rep.Gate.Passed {
		t.Fatal("flag-on planned criterion must block")
	}
}

func TestBaselineRegression(t *testing.T) {
	adopted := mkReport(ModeLenient, 100,
		crit("UC-001.1", StateVerified), crit("UC-001.2", StatePlanned), crit("UC-001.3", StateInProgress))
	b := NewBaseline(adopted)
	if !slices.Equal(b.Unverified, []string{"UC-001.2", "UC-001.3"}) {
		t.Fatalf("baseline = %v", b.Unverified)
	}

	// Same state as at adoption: strict passes, baselined flagged.
	rep := mkReport(ModeStrict, 100,
		crit("UC-001.1", StateVerified), crit("UC-001.2", StatePlanned), crit("UC-001.3", StateInProgress))
	_ = ApplyGate(&rep, &b)
	if !rep.Gate.Passed {
		t.Fatalf("baselined gaps must not block: %v", rep.Gate.Reasons)
	}
	if rep.UseCases[0].Criteria[0].Baselined || !rep.UseCases[0].Criteria[1].Baselined {
		t.Fatal("Baselined flags wrong")
	}

	// A criterion verified at baseline regresses to planned: blocks.
	rep = mkReport(ModeStrict, 100,
		crit("UC-001.1", StatePlanned), crit("UC-001.2", StatePlanned), crit("UC-001.3", StateInProgress))
	_ = ApplyGate(&rep, &b)
	if rep.Gate.Passed {
		t.Fatal("regression must block")
	}

	// A new unverified criterion blocks.
	rep = mkReport(ModeStrict, 100,
		crit("UC-001.1", StateVerified), crit("UC-001.2", StatePlanned), crit("UC-001.3", StateInProgress), crit("UC-001.4", StatePlanned))
	_ = ApplyGate(&rep, &b)
	if rep.Gate.Passed {
		t.Fatal("new unverified criterion must block")
	}

	// A baselined criterion that now fails still blocks.
	rep = mkReport(ModeStrict, 100,
		crit("UC-001.1", StateVerified), crit("UC-001.2", StateFailing), crit("UC-001.3", StateInProgress))
	_ = ApplyGate(&rep, &b)
	if rep.Gate.Passed {
		t.Fatal("failing baselined criterion must block")
	}
}

func TestBaselineFileRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "docs", "use-cases", BaselineFileName)
	want := Baseline{SchemaVersion: BaselineSchemaVersion, Unverified: []string{"UC-001.2"}}
	if err := WriteBaseline(file, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBaseline(file)
	if err != nil || !slices.Equal(got.Unverified, want.Unverified) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := LoadBaseline(filepath.Join(t.TempDir(), "none.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file err = %v", err)
	}
}
