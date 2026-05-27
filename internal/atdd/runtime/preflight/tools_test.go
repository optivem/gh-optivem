package preflight

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRun_ClaudeCheckNilSkips covers the --manual-agents path: the
// cobra layer leaves Options.ClaudeCheck nil and Run must not invoke
// any tool check at all. nil cfg short-circuits the structural pass,
// so a nil-nil call returns nil.
func TestRun_ClaudeCheckNilSkips(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), nil, Options{}); err != nil {
		t.Errorf("nil ClaudeCheck + nil cfg should pass, got: %v", err)
	}
}

// TestRun_ClaudeCheckFailureAggregated covers the implement-time path
// where the operator has no claude on PATH: the failure must surface
// through Run's aggregated error block, prefixed the same way as any
// other preflight failure. Asserted with nil cfg so we isolate the
// tool-check branch from the structural pass.
func TestRun_ClaudeCheckFailureAggregated(t *testing.T) {
	t.Parallel()
	called := false
	stub := func(_ context.Context) error {
		called = true
		return errors.New("claude CLI pre-flight failed: not on PATH")
	}
	err := Run(context.Background(), nil, Options{ClaudeCheck: stub})
	if !called {
		t.Fatal("ClaudeCheck stub was not invoked")
	}
	if err == nil {
		t.Fatal("expected aggregated preflight error, got nil")
	}
	if !strings.Contains(err.Error(), "preflight failed") {
		t.Errorf("error should use the aggregated preflight wrapper, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not on PATH") {
		t.Errorf("error should include the ClaudeCheck failure body, got: %v", err)
	}
}

// TestRun_ClaudeCheckSuccessNoFailure covers the happy path: the stub
// returns nil, so neither it nor the structural pass contribute to the
// failures list, and Run returns nil.
func TestRun_ClaudeCheckSuccessNoFailure(t *testing.T) {
	t.Parallel()
	stub := func(_ context.Context) error { return nil }
	if err := Run(context.Background(), nil, Options{ClaudeCheck: stub}); err != nil {
		t.Errorf("happy ClaudeCheck + nil cfg should pass, got: %v", err)
	}
}
