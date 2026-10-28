package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultSpecDir, DefaultThreshold and BaselineFileName are the spec.yaml defaults.
const (
	DefaultSpecDir   = "docs/use-cases"
	DefaultThreshold = 100.0
	BaselineFileName = ".spec-baseline.json"
)

const dateLayout = "2006-01-02"

// StringList decodes either a single string or a list of strings.
type StringList []string

// UnmarshalYAML accepts a scalar or a sequence.
func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var v string
		if err := n.Decode(&v); err != nil {
			return err
		}
		*s = StringList{v}
		return nil
	case yaml.SequenceNode:
		var v []string
		if err := n.Decode(&v); err != nil {
			return err
		}
		*s = v
		return nil
	}
	return fmt.Errorf("line %d: expected a string or a list of strings", n.Line)
}

// Waiver is a time-boxed exception for one criterion.
type Waiver struct {
	ID       string `yaml:"id" json:"id"`
	Reason   string `yaml:"reason" json:"reason"`
	Approver string `yaml:"approver" json:"approver"`
	Until    string `yaml:"until" json:"until"`
}

// Expired reports whether the waiver no longer applies at now. The waiver is
// valid through its Until date inclusive.
func (w Waiver) Expired(now time.Time) bool {
	until, err := time.Parse(dateLayout, w.Until)
	if err != nil {
		return true
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return today.After(until)
}

// Config is the content of spec.yaml.
type Config struct {
	SpecDir   string     `yaml:"specDir"`
	Results   StringList `yaml:"results"`
	Threshold *float64   `yaml:"threshold"`
	Waivers   []Waiver   `yaml:"waivers"`
	Exclude   StringList `yaml:"exclude"`
	Flags     StringList `yaml:"flags"`
	Baseline  string     `yaml:"baseline"`
}

// ThresholdValue returns the configured threshold or the default (100).
func (c Config) ThresholdValue() float64 {
	if c.Threshold == nil {
		return DefaultThreshold
	}
	return *c.Threshold
}

// ParseConfig parses spec.yaml content, applies defaults and validates it.
// Unknown keys are rejected so that typos are not silently ignored.
func ParseConfig(data []byte) (Config, error) {
	var c Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse spec.yaml: %w", err)
	}
	if c.SpecDir == "" {
		c.SpecDir = DefaultSpecDir
	}
	if c.Baseline == "" {
		c.Baseline = path.Join(c.SpecDir, BaselineFileName)
	}
	if c.Threshold != nil && (*c.Threshold < 0 || *c.Threshold > 100) {
		return Config{}, fmt.Errorf("spec.yaml: threshold %v must be between 0 and 100", *c.Threshold)
	}
	seen := map[string]bool{}
	for i, w := range c.Waivers {
		required := []struct{ name, value string }{
			{"id", w.ID}, {"reason", w.Reason}, {"approver", w.Approver}, {"until", w.Until},
		}
		for _, f := range required {
			if f.value == "" {
				return Config{}, fmt.Errorf("spec.yaml: waivers[%d] is missing %s", i, f.name)
			}
		}
		if _, err := time.Parse(dateLayout, w.Until); err != nil {
			return Config{}, fmt.Errorf("spec.yaml: waivers[%d] (%s) has invalid until %q (expected YYYY-MM-DD)", i, w.ID, w.Until)
		}
		if seen[w.ID] {
			return Config{}, fmt.Errorf("spec.yaml: duplicate waiver for %s", w.ID)
		}
		seen[w.ID] = true
	}
	return c, nil
}

// LoadConfig reads and parses the spec.yaml file at file.
func LoadConfig(file string) (Config, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", file, err)
	}
	return ParseConfig(data)
}

// RequireResults returns an error when no results glob is configured.
func (c Config) RequireResults() error {
	if len(c.Results) == 0 {
		return errors.New("spec.yaml: results is required (for example **/build/test-results/**/*.xml)")
	}
	return nil
}
