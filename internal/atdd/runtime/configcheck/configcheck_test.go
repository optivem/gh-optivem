package configcheck

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// TestValidateTaskPrompts_RejectsUnknownTask verifies typos in MID task
// names surface at config-load, not deep inside the pipeline. This is the
// engine-backed rule relocated out of projectconfig.Validate.
func TestValidateTaskPrompts_RejectsUnknownTask(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		TaskPrompts: map[string]string{
			"not-a-real-task": "config/prompts/x.md",
		},
	}
	err := ValidateTaskPrompts(cfg)
	if err == nil {
		t.Fatal("expected error for unknown task name, got nil")
	}
	if !strings.Contains(err.Error(), "not-a-real-task") {
		t.Fatalf("error should name the bad task, got: %v", err)
	}
	if !strings.Contains(err.Error(), "is not a known embedded MID task") {
		t.Fatalf("error should preserve the canonical wording, got: %v", err)
	}
}

// TestValidateTaskPrompts_AcceptsKnownTask verifies a key that names a real
// embedded MID task passes — i.e. the rule actually enumerates the engine,
// not just rejects everything.
func TestValidateTaskPrompts_AcceptsKnownTask(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		TaskPrompts: map[string]string{
			"write-acceptance-tests": "config/prompts/write-acceptance-tests.md",
		},
	}
	if err := ValidateTaskPrompts(cfg); err != nil {
		t.Fatalf("expected known task name to pass, got: %v", err)
	}
}

// TestValidateTaskPrompts_NoTaskPrompts verifies a nil config and an empty
// task-prompts map are both no-ops (the engine is not even loaded).
func TestValidateTaskPrompts_NoTaskPrompts(t *testing.T) {
	t.Parallel()
	if err := ValidateTaskPrompts(nil); err != nil {
		t.Fatalf("nil config: want no error, got: %v", err)
	}
	if err := ValidateTaskPrompts(&projectconfig.Config{}); err != nil {
		t.Fatalf("empty task-prompts: want no error, got: %v", err)
	}
}
