package spec

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
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

// BaselineSchemaVersion is the version of the baseline file format.
const BaselineSchemaVersion = 1

// Baseline lists the criteria that were not Verified when the gate was adopted.
type Baseline struct {
	SchemaVersion int      `json:"schemaVersion"`
	Unverified    []string `json:"unverified"`
}

// NewBaseline builds a baseline from the criteria that are not Verified in rep.
func NewBaseline(rep Report) Baseline {
	b := Baseline{SchemaVersion: BaselineSchemaVersion, Unverified: []string{}}
	for _, uc := range rep.UseCases {
		for _, c := range uc.Criteria {
			if c.State != StateVerified {
				b.Unverified = append(b.Unverified, c.ID)
			}
		}
	}
	slices.Sort(b.Unverified)
	return b
}

// Contains reports whether id is listed as unverified in the baseline.
func (b Baseline) Contains(id string) bool {
	return slices.Contains(b.Unverified, id)
}

// WriteBaseline writes the baseline as indented JSON, creating parent directories.
func WriteBaseline(file string, b Baseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	if err := os.WriteFile(file, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", file, err)
	}
	return nil
}

// LoadBaseline reads a baseline file. A missing file yields an error that
// satisfies errors.Is(err, fs.ErrNotExist) so that callers can treat "no
// baseline" as a deliberate choice; a malformed file is always an error.
func LoadBaseline(file string) (Baseline, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Baseline{}, fmt.Errorf("read baseline %s: %w", file, err)
	}
	var b Baseline
	if err := json.Unmarshal(data, &b); err != nil {
		return Baseline{}, fmt.Errorf("parse baseline %s: %w", file, err)
	}
	if b.SchemaVersion != BaselineSchemaVersion {
		return Baseline{}, fmt.Errorf("baseline %s: unsupported schemaVersion %d", file, b.SchemaVersion)
	}
	return b, nil
}

// ApplyGate marks baselined criteria and fills rep.Gate for rep.Mode. The
// baseline may be nil. An unknown mode is an error (exit code 2).
//
// Lenient blocks on validation errors and Failing criteria. Strict also blocks
// on every in-scope criterion that is not Verified and not baselined, and on
// coverage below the threshold. In scope means the flag is on (or absent) and
// the criterion is not waived. Failing always blocks, even when baselined.
// Strict coverage is Verified over flag-on criteria, leaving out baselined
// unverified ones so that an adopting repo is not blocked by its old gaps.
func ApplyGate(rep *Report, b *Baseline) error {
	if rep.Mode != ModeLenient && rep.Mode != ModeStrict {
		return fmt.Errorf("unknown mode %q (expected lenient or strict)", rep.Mode)
	}
	reasons := []string{}
	for _, e := range rep.Validation.Errors {
		reasons = append(reasons, fmt.Sprintf("validation error (%s): %s", e.Code, e.Message))
	}
	verified, denom := 0, 0
	for i := range rep.UseCases {
		for j := range rep.UseCases[i].Criteria {
			c := &rep.UseCases[i].Criteria[j]
			c.Baselined = b != nil && c.State != StateVerified && b.Contains(c.ID)
			if c.State == StateFailing {
				reasons = append(reasons, fmt.Sprintf("%s is Failing", c.ID))
				continue
			}
			if rep.Mode != ModeStrict || !c.FlagOn {
				continue
			}
			if c.State == StateVerified {
				verified++
				denom++
				continue
			}
			if c.Baselined {
				continue
			}
			denom++
			if c.State != StateWaived {
				reasons = append(reasons, fmt.Sprintf("%s is %s (strict requires Verified)", c.ID, c.State))
			}
		}
	}
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
