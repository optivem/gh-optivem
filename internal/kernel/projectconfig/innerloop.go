package projectconfig

import "fmt"

// InnerLoopMode names how (and whether) a component runs an inner loop of
// unit/component-level testing beneath the acceptance-test outer loop.
type InnerLoopMode string

const (
	// InnerLoopNone is the default: outer loop only. An absent inner-loop
	// block means this mode, so existing configs keep working unchanged.
	InnerLoopNone InnerLoopMode = "none"
	// InnerLoopTestAfter writes domain code and its tests together, then gates
	// green on the tests passing and a mutation-score threshold.
	InnerLoopTestAfter InnerLoopMode = "test-after"
	// InnerLoopTDD writes each test first, verifies it red, then writes the code.
	InnerLoopTDD InnerLoopMode = "tdd"
)

// InnerLoop is the optional per-component inner-loop declaration. Absent (the
// zero value) means InnerLoopNone.
type InnerLoop struct {
	// Mode is none | test-after | tdd. Empty is equivalent to none.
	Mode InnerLoopMode `yaml:"mode,omitempty"`
	// MutationThreshold is the percentage (0-100) of mutants that must be
	// killed for the mutation gate to pass. 0 means unset. Requires a mode
	// other than none.
	MutationThreshold int `yaml:"mutation-threshold,omitempty"`
}

// IsEmpty reports whether no inner-loop field is set.
func (il InnerLoop) IsEmpty() bool {
	return il.Mode == "" && il.MutationThreshold == 0
}

// EffectiveMode returns the mode with the empty default resolved to none.
func (il InnerLoop) EffectiveMode() InnerLoopMode {
	if il.Mode == "" {
		return InnerLoopNone
	}
	return il.Mode
}

// Enabled reports whether the inner loop runs at all (mode is not none).
func (il InnerLoop) Enabled() bool {
	return il.EffectiveMode() != InnerLoopNone
}

// ValidateInnerLoopMode reports whether s is an accepted mode string. It is
// the single validator shared by Config.Validate and the init/config prompts.
func ValidateInnerLoopMode(s string) error {
	switch InnerLoopMode(s) {
	case "", InnerLoopNone, InnerLoopTestAfter, InnerLoopTDD:
		return nil
	}
	return fmt.Errorf("inner-loop mode %q is not one of: %s, %s, %s", s, InnerLoopNone, InnerLoopTestAfter, InnerLoopTDD)
}

func (il InnerLoop) validate(prefix string) error {
	if err := ValidateInnerLoopMode(string(il.Mode)); err != nil {
		return fmt.Errorf("config: %s.mode: %w", prefix, err)
	}
	if il.MutationThreshold < 0 || il.MutationThreshold > 100 {
		return fmt.Errorf("config: %s.mutation-threshold %d is out of range (0-100)", prefix, il.MutationThreshold)
	}
	if il.MutationThreshold != 0 && !il.Enabled() {
		return fmt.Errorf("config: %s.mutation-threshold requires %s.mode to be %s or %s", prefix, prefix, InnerLoopTestAfter, InnerLoopTDD)
	}
	return nil
}
