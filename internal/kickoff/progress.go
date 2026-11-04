package kickoff

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Approval records one user approval: who approved, when, and the fingerprint
// of the approved content. Content edited after approval no longer matches the
// fingerprint, which sends that phase and every later one back for re-approval.
type Approval struct {
	By          string `yaml:"by"`
	At          string `yaml:"at"`
	Fingerprint string `yaml:"fingerprint"`
}

// Progress is the committed gh-optivem-progress.yaml. It stores only
// approvals; every other fact about progress is derived from the files.
type Progress struct {
	Approvals map[string]Approval `yaml:"approvals,omitempty"`
}

// LoadProgress reads the progress file. A missing file is an empty Progress;
// any other read or parse failure is returned.
func LoadProgress(l Layout) (*Progress, error) {
	data, err := os.ReadFile(l.Abs(ProgressFile))
	if errors.Is(err, fs.ErrNotExist) {
		return &Progress{Approvals: map[string]Approval{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", ProgressFile, err)
	}
	var p Progress
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ProgressFile, err)
	}
	if p.Approvals == nil {
		p.Approvals = map[string]Approval{}
	}
	for k := range p.Approvals {
		if ph, err := ParsePhase(k); err != nil || !NeedsApproval(ph) {
			return nil, fmt.Errorf("%s: %q is not an approval phase (valid: %s)", ProgressFile, k, joinPhases(approvalPhases))
		}
	}
	return &p, nil
}

// Save writes the progress file, or removes it when no approval remains.
func (p *Progress) Save(l Layout) error {
	path := l.Abs(ProgressFile)
	if len(p.Approvals) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", ProgressFile, err)
		}
		return nil
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode %s: %w", ProgressFile, err)
	}
	header := "# Kickoff approvals recorded by `gh optivem kickoff`. Committed on purpose.\n# Editing approved content makes the matching phase (and every later one) need re-approval.\n"
	if err := os.WriteFile(path, append([]byte(header), data...), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ProgressFile, err)
	}
	return nil
}

// Approve records an approval of ph over the current content.
func (p *Progress) Approve(l Layout, ph Phase, by string, now time.Time) error {
	if !NeedsApproval(ph) {
		return fmt.Errorf("phase %s has no approval", ph)
	}
	fp, err := Fingerprint(l, ph)
	if err != nil {
		return err
	}
	p.Approvals[string(ph)] = Approval{By: by, At: now.UTC().Format(time.RFC3339), Fingerprint: fp}
	return nil
}

// Invalidate drops the approval of ph and of every later approval phase.
func (p *Progress) Invalidate(ph Phase) {
	delete(p.Approvals, string(ph))
	for _, later := range laterApprovalPhases(ph) {
		delete(p.Approvals, string(later))
	}
}
