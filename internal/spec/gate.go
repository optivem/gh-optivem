package spec

import (
	"fmt"
	"math"
)

// Exit codes of the gate.
const (
	ExitPass       = 0
	ExitGateFailed = 1
	ExitUsage      = 2
)

// Gate modes.
const (
	ModeLenient = "lenient"
	ModeStrict  = "strict"
)

// ApplyGate fills rep.Gate for rep.Mode. An unknown mode is an error (exit
// code 2).
//
// Lenient blocks on validation errors and Failing criteria. Strict also blocks
// on every in-scope criterion that is not Verified, and on coverage below the
// threshold. In scope means the flag is on (or absent) and the criterion is
// not waived. Failing always blocks. Strict coverage is Verified over flag-on
// criteria.
func ApplyGate(rep *Report) error {
	if rep.Mode != ModeLenient && rep.Mode != ModeStrict {
		return fmt.Errorf("unknown mode %q (expected lenient or strict)", rep.Mode)
	}
	reasons := []string{}
	for _, e := range rep.Validation.Errors {
		reasons = append(reasons, fmt.Sprintf("validation error (%s): %s", e.Code, e.Message))
	}
	criterionReasons, verified, denom := gateCriteria(rep)
	reasons = append(reasons, criterionReasons...)
	if rep.Mode == ModeStrict {
		pct := 100.0
		if denom > 0 {
			pct = math.Round(float64(verified)*1000/float64(denom)) / 10
		}
		if pct < rep.Threshold {
			reasons = append(reasons, fmt.Sprintf("coverage %.1f%% is below threshold %.1f%%", pct, rep.Threshold))
		}
	}
	rep.Gate = &Gate{Passed: len(reasons) == 0, Reasons: reasons}
	return nil
}

// gateCriteria collects the blocking reasons plus the strict coverage
// numerator and denominator.
func gateCriteria(rep *Report) (reasons []string, verified, denom int) {
	for i := range rep.UseCases {
		for j := range rep.UseCases[i].Criteria {
			c := &rep.UseCases[i].Criteria[j]
			reason, inVerified, inDenom := gateCriterion(c, rep.Mode)
			if reason != "" {
				reasons = append(reasons, reason)
			}
			if inVerified {
				verified++
			}
			if inDenom {
				denom++
			}
		}
	}
	return reasons, verified, denom
}

// gateCriterion evaluates one criterion for the mode.
// It returns a blocking reason (empty if none) and whether the criterion counts
// toward the verified numerator and the coverage denominator.
func gateCriterion(c *ReportCriterion, mode string) (reason string, inVerified, inDenom bool) {
	if c.State == StateFailing {
		return fmt.Sprintf("%s is Failing", c.ID), false, false
	}
	if mode != ModeStrict || !c.FlagOn {
		return "", false, false
	}
	if c.State == StateVerified {
		return "", true, true
	}
	if c.State != StateWaived {
		return fmt.Sprintf("%s is %s (strict requires Verified)", c.ID, c.State), false, true
	}
	return "", false, true
}

// ExitCode maps a gated report to an exit code. A report without a verdict is
// a usage error.
func (r Report) ExitCode() int {
	switch {
	case r.Gate == nil:
		return ExitUsage
	case r.Gate.Passed:
		return ExitPass
	}
	return ExitGateFailed
}
