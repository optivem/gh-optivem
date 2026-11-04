package spec

import (
	"encoding/json"
	"math"
	"slices"
	"time"
)

// SchemaVersion is the version of the JSON report contract.
const SchemaVersion = 1

// State is a lifecycle state. The string values are part of the JSON contract.
type State string

const (
	StatePlanned    State = "Planned"
	StateInProgress State = "In progress"
	StateVerified   State = "Verified"
	StateFailing    State = "Failing"
	StateWaived     State = "Waived"
	// StateChanged marks a criterion whose tests pass but whose text differs
	// from the fingerprint recorded when it was last verified: the old test may
	// no longer prove the current requirement, so it counts as unverified.
	StateChanged State = "Changed"
)

// Options controls report building. Now is injectable so that waiver expiry is testable.
type Options struct {
	Mode      string
	Threshold *float64 // nil means use the config threshold
	Flags     []string // flags considered on, in addition to Config.Flags
	Now       time.Time
	// Fingerprints maps criterion id to the text fingerprint recorded when it
	// was last verified (see Baseline.Fingerprints). A criterion that would be
	// Verified but whose current text hashes differently is reported Changed.
	Fingerprints map[string]string
}

// ReportResult is one executed result of a test.
type ReportResult struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
}

// ReportTest is a test linked to a criterion.
type ReportTest struct {
	Name    string         `json:"name"`
	Results []ReportResult `json:"results"`
	WIP     bool           `json:"wip"`
}

// ReportCriterion is a criterion with its derived state and linked tests.
type ReportCriterion struct {
	ID        string       `json:"id"`
	Text      string       `json:"text"`
	Group     string       `json:"group"`
	State     State        `json:"state"`
	Flag      *string      `json:"flag"`
	FlagOn    bool         `json:"flagOn"`
	Tests     []ReportTest `json:"tests"`
	Waiver    *Waiver      `json:"waiver"`
	Baselined bool         `json:"baselined"`
}

// ReportUseCase is a use case with its derived state.
type ReportUseCase struct {
	ID       string            `json:"id"`
	Title    string            `json:"title"`
	State    State             `json:"state"`
	Criteria []ReportCriterion `json:"criteria"`
}

// Summary holds the counts per state and the spec coverage.
type Summary struct {
	Total           int     `json:"total"`
	Verified        int     `json:"verified"`
	InProgress      int     `json:"inProgress"`
	Failing         int     `json:"failing"`
	Planned         int     `json:"planned"`
	Waived          int     `json:"waived"`
	Changed         int     `json:"changed"`
	CoveragePercent float64 `json:"coveragePercent"`
}

// Orphan is a test that cannot be linked to any known criterion.
type Orphan struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// UnknownID is a test referencing a criterion ID that is absent from the spec.
type UnknownID struct {
	ID   string `json:"id"`
	Test string `json:"test"`
}

// Validation holds spec validation errors.
type Validation struct {
	Errors []ValidationError `json:"errors"`
}

// Gate is the gating verdict, filled in by the gating step.
type Gate struct {
	Passed  bool     `json:"passed"`
	Reasons []string `json:"reasons"`
}

// Report is the versioned JSON report (schemaVersion 1).
type Report struct {
	SchemaVersion int             `json:"schemaVersion"`
	Mode          string          `json:"mode"`
	Threshold     float64         `json:"threshold"`
	Summary       Summary         `json:"summary"`
	UseCases      []ReportUseCase `json:"useCases"`
	Orphans       []Orphan        `json:"orphans"`
	UnknownIDs    []UnknownID     `json:"unknownIds"`
	Validation    Validation      `json:"validation"`
	Gate          *Gate           `json:"gate,omitempty"`
}

// JSON renders the report as indented JSON.
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// BuildReport derives lifecycle states, coverage, orphans and unknown IDs from
// the parsed spec, the test results and the config. Waivers that have expired
// at opts.Now are ignored, so the criterion reverts to its test-derived state.
func BuildReport(useCases []UseCase, tests []Test, cfg Config, opts Options) Report {
	threshold := cfg.ThresholdValue()
	if opts.Threshold != nil {
		threshold = *opts.Threshold
	}
	mode := opts.Mode
	if mode == "" {
		mode = "lenient"
	}
	flagsOn := append(slices.Clone(cfg.Flags), opts.Flags...)

	waivers := activeWaivers(cfg.Waivers, opts.Now)

	known := map[string]bool{}
	for _, uc := range useCases {
		for _, c := range uc.Criteria {
			known[c.ID] = true
		}
	}
	rep := Report{
		SchemaVersion: SchemaVersion,
		Mode:          mode,
		Threshold:     threshold,
		UseCases:      []ReportUseCase{},
		Orphans:       []Orphan{},
		UnknownIDs:    []UnknownID{},
		Validation:    Validation{Errors: []ValidationError{}},
	}
	rep.Validation.Errors = append(rep.Validation.Errors, Validate(useCases)...)

	linked := linkTests(&rep, tests, known)

	for _, uc := range useCases {
		rep.UseCases = append(rep.UseCases, buildUseCase(uc, linked, waivers, flagsOn, opts.Fingerprints, &rep.Summary))
	}
	if rep.Summary.Total > 0 {
		pct := float64(rep.Summary.Verified) * 100 / float64(rep.Summary.Total)
		rep.Summary.CoveragePercent = math.Round(pct*10) / 10
	}
	return rep
}

// activeWaivers indexes the waivers that have not expired at now.
func activeWaivers(all []Waiver, now time.Time) map[string]Waiver {
	waivers := map[string]Waiver{}
	for _, w := range all {
		if !w.Expired(now) {
			waivers[w.ID] = w
		}
	}
	return waivers
}

// linkTests groups tests by the known criterion IDs they reference, recording
// orphans and unknown IDs on rep.
func linkTests(rep *Report, tests []Test, known map[string]bool) map[string][]Test {
	linked := map[string][]Test{}
	for _, t := range tests {
		name := t.Method
		if t.Class != "" {
			name = t.Class + "." + t.Method
		}
		if len(t.IDs) == 0 {
			rep.Orphans = append(rep.Orphans, Orphan{name, "no criterion ID"})
			continue
		}
		anyKnown := false
		for _, id := range t.IDs {
			if known[id] {
				anyKnown = true
				linked[id] = append(linked[id], t)
			} else {
				rep.UnknownIDs = append(rep.UnknownIDs, UnknownID{id, name})
			}
		}
		if !anyKnown {
			rep.Orphans = append(rep.Orphans, Orphan{name, "no known criterion ID"})
		}
	}
	return linked
}

// count records one criterion state in the summary tallies.
func (s *Summary) count(state State) {
	s.Total++
	switch state {
	case StateVerified:
		s.Verified++
	case StateInProgress:
		s.InProgress++
	case StateFailing:
		s.Failing++
	case StatePlanned:
		s.Planned++
	case StateWaived:
		s.Waived++
	case StateChanged:
		s.Changed++
	}
}

// buildUseCase builds one use case's report entry and tallies its criteria into sum.
func buildUseCase(uc UseCase, linked map[string][]Test, waivers map[string]Waiver, flagsOn []string, fps map[string]string, sum *Summary) ReportUseCase {
	ruc := ReportUseCase{ID: uc.ID, Title: uc.Title, Criteria: []ReportCriterion{}}
	anyTests := false
	allVerified := len(uc.Criteria) > 0
	for _, c := range uc.Criteria {
		rc := buildCriterion(c, linked[c.ID], waivers, flagsOn, fps)
		ruc.Criteria = append(ruc.Criteria, rc)
		sum.count(rc.State)
		if len(rc.Tests) > 0 {
			anyTests = true
		}
		if rc.State != StateVerified {
			allVerified = false
		}
	}
	switch {
	case allVerified:
		ruc.State = StateVerified
	case anyTests:
		ruc.State = StateInProgress
	default:
		ruc.State = StatePlanned
	}
	return ruc
}

func buildCriterion(c Criterion, tests []Test, waivers map[string]Waiver, flagsOn []string, fps map[string]string) ReportCriterion {
	rc := ReportCriterion{
		ID: c.ID, Text: c.Text, Group: c.Group,
		Tests:  []ReportTest{},
		FlagOn: c.Flag == "" || slices.Contains(flagsOn, c.Flag),
	}
	if c.Flag != "" {
		f := c.Flag
		rc.Flag = &f
	}
	wip, failing, allPassed := false, false, len(tests) > 0
	for _, t := range tests {
		rt := ReportTest{Name: t.Method, WIP: t.WIP, Results: []ReportResult{}}
		for _, r := range t.Results {
			rt.Results = append(rt.Results, ReportResult{r.Name, r.Status})
		}
		rc.Tests = append(rc.Tests, rt)
		if t.WIP {
			wip = true
		}
		switch t.Status() {
		case StatusFailed:
			failing = true
			allPassed = false
		case StatusSkipped:
			allPassed = false
		}
	}
	// Precedence: Waived > In progress > Failing > Changed > Verified > Planned.
	// A criterion whose linked tests are only skipped has no passing evidence
	// and stays Planned. Changed applies only to an otherwise-Verified
	// criterion whose text no longer matches its recorded fingerprint.
	stored, hasFP := fps[c.ID]
	changed := allPassed && !failing && !wip && hasFP && stored != Fingerprint(c.Text)
	w, waived := waivers[c.ID]
	switch {
	case waived:
		rc.State = StateWaived
		rc.Waiver = &w
	case wip:
		rc.State = StateInProgress
	case failing:
		rc.State = StateFailing
	case changed:
		rc.State = StateChanged
	case allPassed:
		rc.State = StateVerified
	default:
		rc.State = StatePlanned
	}
	return rc
}
