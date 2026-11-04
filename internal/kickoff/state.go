package kickoff

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PhaseStatus is the derived state of one phase.
type PhaseStatus struct {
	Phase Phase
	// Generated is true once the phase's output exists on disk (approval
	// phases only; for derived phases it equals Done).
	Generated bool
	// Done is true when the phase is complete: derived from the files, or for
	// approval phases, approved with a matching fingerprint.
	Done bool
	// Detail explains a not-done state, e.g. an approval invalidated by an edit.
	Detail string
}

// State is the derived progress of the whole flow.
type State struct {
	Statuses []PhaseStatus
	// Next is the first phase that is not Done; empty when the flow is finished.
	Next Phase
}

// Status returns the status of ph.
func (s State) Status(ph Phase) PhaseStatus {
	for _, st := range s.Statuses {
		if st.Phase == ph {
			return st
		}
	}
	return PhaseStatus{Phase: ph}
}

// Probes answer the two questions that cannot be read from local files alone.
type Probes struct {
	// SampleRemoved reports whether the shop sample is gone from the repo.
	SampleRemoved func(Layout) (bool, error)
	// StoriesCreated reports whether kickoff stories already exist in the tracker.
	StoriesCreated func() (bool, error)
}

// Detect derives the state of the flow. Everything is read from the files
// except approvals, which come from the progress file; an approval whose
// fingerprint no longer matches (the content was edited) is treated as absent
// together with every later approval.
func Detect(l Layout, prog *Progress, probes Probes) (State, error) {
	var st State
	add := func(p PhaseStatus) {
		st.Statuses = append(st.Statuses, p)
		if !p.Done && st.Next == "" {
			st.Next = p.Phase
		}
	}

	descDone, descDetail, err := descriptionStatus(l)
	if err != nil {
		return State{}, err
	}
	add(PhaseStatus{Phase: PhaseDescription, Generated: descDone, Done: descDone, Detail: descDetail})

	earlierApproved := descDone
	for _, ph := range approvalPhases {
		gen, err := generated(l, ph)
		if err != nil {
			return State{}, err
		}
		ps := PhaseStatus{Phase: ph, Generated: gen}
		switch {
		case !gen:
		case !earlierApproved:
			ps.Detail = "an earlier phase is not approved"
		default:
			ps.Done, ps.Detail, err = approvedCurrent(l, prog, ph)
			if err != nil {
				return State{}, err
			}
		}
		earlierApproved = earlierApproved && ps.Done
		add(ps)
	}

	removed := false
	if probes.SampleRemoved != nil {
		removed, err = probes.SampleRemoved(l)
		if err != nil {
			return State{}, fmt.Errorf("check shop sample removal: %w", err)
		}
	}
	add(PhaseStatus{Phase: PhaseRemove, Generated: removed, Done: removed})

	stories := false
	if probes.StoriesCreated != nil {
		stories, err = probes.StoriesCreated()
		if err != nil {
			return State{}, fmt.Errorf("check created stories: %w", err)
		}
	}
	add(PhaseStatus{Phase: PhaseStories, Generated: stories, Done: stories})
	return st, nil
}

func approvedCurrent(l Layout, prog *Progress, ph Phase) (bool, string, error) {
	a, ok := prog.Approvals[string(ph)]
	if !ok {
		return false, "", nil
	}
	fp, err := Fingerprint(l, ph)
	if err != nil {
		return false, "", err
	}
	if fp != a.Fingerprint {
		return false, fmt.Sprintf("changed since approval by %s at %s; re-approval needed", a.By, a.At), nil
	}
	return true, "", nil
}

// generated reports whether the output of an approval phase exists.
func generated(l Layout, ph Phase) (bool, error) {
	switch ph {
	case PhasePrototype:
		return fileExists(l.Abs(PrototypeDir + "/index.html"))
	case PhaseDiagram:
		return fileExists(l.Abs(DiagramFile))
	case PhaseUseCases:
		files, err := narrativeFiles(l)
		return len(files) > 0, err
	}
	return false, fmt.Errorf("phase %s has no generated output", ph)
}

// narrativeFiles lists the user's use case narratives: every docs/use-cases
// markdown file except the shop sample's.
func narrativeFiles(l Layout) ([]string, error) {
	all, err := filepath.Glob(filepath.Join(l.Abs(UseCasesDir), "*.md"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range all {
		if filepath.Clean(f) == filepath.Clean(l.Abs(sampleUseCaseFile)) {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

func descriptionStatus(l Layout) (bool, string, error) {
	data, err := os.ReadFile(l.Abs(ProductFile))
	if errors.Is(err, fs.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("read %s: %w", ProductFile, err)
	}
	if err := ValidateDescription(string(data)); err != nil {
		return false, fmt.Sprintf("%s is invalid: %v", ProductFile, err), nil
	}
	return true, "", nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// Describe renders the state as one line per phase for the operator.
func (s State) Describe() string {
	var b strings.Builder
	for _, st := range s.Statuses {
		mark := "[ ]"
		switch {
		case st.Done:
			mark = "[x]"
		case st.Generated:
			mark = "[~]"
		}
		fmt.Fprintf(&b, "%s %s", mark, st.Phase)
		if st.Detail != "" {
			fmt.Fprintf(&b, " (%s)", st.Detail)
		}
		b.WriteString("\n")
	}
	return b.String()
}
