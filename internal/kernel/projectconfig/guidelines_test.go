package projectconfig

import "testing"

// guidelines-path (plan 20261109-1259) is optional: an unset key resolves to
// the default, a set key wins, and both validate.
func TestGuidelinesPath_DefaultAndOverride(t *testing.T) {
	t.Parallel()
	cfg := validMonolithBase()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config without guidelines-path should validate: %v", err)
	}
	if got := cfg.GuidelinesPath(); got != DefaultGuidelinesPath {
		t.Errorf("unset GuidelinesPath() = %q, want %q", got, DefaultGuidelinesPath)
	}
	if got := cfg.PlaceholderMap()[GuidelinesPathKey]; got != DefaultGuidelinesPath {
		t.Errorf("unset ${guidelines-path} = %q, want %q", got, DefaultGuidelinesPath)
	}

	cfg.SystemTest.Paths[GuidelinesPathKey] = "docs/review-rules"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config with guidelines-path should validate: %v", err)
	}
	if got := cfg.PlaceholderMap()[GuidelinesPathKey]; got != "docs/review-rules" {
		t.Errorf("set ${guidelines-path} = %q, want docs/review-rules", got)
	}
}
