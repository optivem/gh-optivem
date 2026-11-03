package projectconfig

import (
	"fmt"
	"strings"
)

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

// ComponentTestPathKey is the placeholder / tier `paths:` key naming the
// component's inner test source set (the component tests an inner loop writes
// and runs). It is declared on the backend or frontend tier that opts in, not
// under system-test.paths, and is scoped only to the inner-loop phase.
const ComponentTestPathKey = "component-test"

// IsTierInnerLoopPathKey reports whether key is a placeholder resolved from a
// system tier's own `paths:` block rather than system-test.paths.
func IsTierInnerLoopPathKey(key string) bool {
	return key == ComponentTestPathKey
}

// ComponentTestPath returns the configured component-test path of the
// multitier backend tier (falling back to the frontend tier), or "" when no
// tier declares one.
func (c *Config) ComponentTestPath() string {
	if c == nil {
		return ""
	}
	if v := c.System.Backend.Paths[ComponentTestPathKey]; v != "" {
		return v
	}
	return c.System.Frontend.Paths[ComponentTestPathKey]
}

// validateTierPaths enforces the tier `paths:` contract: only the inner-loop
// keys are accepted, values are fully-resolved repo-relative paths, and
// component-test is required exactly when the tier's inner loop is enabled.
func validateTierPaths(prefix string, t TierSpec) error {
	if err := t.InnerLoop.validate(prefix + ".inner-loop"); err != nil {
		return err
	}
	for _, k := range sortedKeys(t.Paths) {
		if !IsTierInnerLoopPathKey(k) {
			return fmt.Errorf("config: %s.paths.%s is not a supported key (only %q is allowed on a system tier; other paths belong under system-test.paths)", prefix, k, ComponentTestPathKey)
		}
		if strings.Contains(t.Paths[k], "${") {
			return fmt.Errorf("config: %s.paths.%s %q contains a ${...} marker; paths must be fully resolved", prefix, k, t.Paths[k])
		}
		if err := validatePath(prefix+".paths."+k, t.Paths[k]); err != nil {
			return err
		}
	}
	has := t.Paths[ComponentTestPathKey] != ""
	switch {
	case t.InnerLoop.Enabled() && !has:
		return fmt.Errorf("config: %s.inner-loop.mode is %s; %s.paths.%s is required", prefix, t.InnerLoop.EffectiveMode(), prefix, ComponentTestPathKey)
	case !t.InnerLoop.Enabled() && has:
		return fmt.Errorf("config: %s.paths.%s requires %s.inner-loop.mode to be %s or %s", prefix, ComponentTestPathKey, prefix, InnerLoopTestAfter, InnerLoopTDD)
	}
	return nil
}

// InnerLoopForChannel returns the inner-loop declaration governing a system
// dispatch for the given delivery channel, plus the logical component name the
// component-test runner knows it by: the monolith's system-level block
// ("monolith"), or the multitier tier the channel maps to (api -> "backend",
// ui -> "frontend"). An empty or unmapped channel (whole-system dispatch,
// microservices) yields the zero value (mode none) and an empty name.
func (c *Config) InnerLoopForChannel(channel string) (InnerLoop, string) {
	if c == nil {
		return InnerLoop{}, ""
	}
	switch c.System.Architecture {
	case ArchMonolith:
		return c.System.InnerLoop, "monolith"
	case ArchMultitier:
		switch channel {
		case "api":
			return c.System.Backend.InnerLoop, "backend"
		case "ui":
			return c.System.Frontend.InnerLoop, "frontend"
		}
	}
	return InnerLoop{}, ""
}
