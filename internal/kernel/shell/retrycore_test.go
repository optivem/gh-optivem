package shell

import (
	"errors"
	"regexp"
	"testing"
	"time"
)

// TestRetryWithPolicy exercises the regex-driven public engine that callers
// in sibling packages (internal/sonar, internal/config) plug into. The gh
// path uses runWithRetryLoop + classifyError directly because rate-limit
// pass-through needs typed-error inspection, so this test file focuses on
// the pure regex shape.
func TestRetryWithPolicy(t *testing.T) {
	transient := regexp.MustCompile(`(?i)Error 5[0-9][0-9]|timeout`)
	hardFail := regexp.MustCompile(`(?i)Error 4[0-9][0-9]|unauthorized`)

	t.Run("immediate success skips retries", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			return "ok", nil
		}
		out, err := RetryWithPolicy(transient, hardFail, "test", fn)
		if err != nil || out != "ok" || attempts != 1 {
			t.Fatalf("got attempts=%d out=%q err=%v, want attempts=1 out=ok err=nil", attempts, out, err)
		}
		if len(sleeps) != 0 {
			t.Fatalf("sleeps = %v, want zero", sleeps)
		}
	})

	t.Run("transient match retries until success", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			if attempts < 3 {
				return "Error 504 Gateway Timeout", errors.New("exit 1")
			}
			return "ok", nil
		}
		out, err := RetryWithPolicy(transient, hardFail, "test", fn)
		if err != nil || out != "ok" || attempts != 3 {
			t.Fatalf("got attempts=%d out=%q err=%v, want attempts=3 out=ok err=nil", attempts, out, err)
		}
		if len(sleeps) != 2 || sleeps[0] != 5*time.Second || sleeps[1] != 15*time.Second {
			t.Fatalf("sleeps = %v, want [5s 15s]", sleeps)
		}
	})

	t.Run("hard-fail bypasses retries", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			return "Error 404 Not Found", errors.New("exit 1")
		}
		_, err := RetryWithPolicy(transient, hardFail, "test", fn)
		if err == nil || attempts != 1 {
			t.Fatalf("got attempts=%d err=%v, want attempts=1 err=non-nil", attempts, err)
		}
		if len(sleeps) != 0 {
			t.Fatalf("sleeps = %v, want zero", sleeps)
		}
	})

	t.Run("transient exhausted after maxAttempts", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			return "Error 503 Service Unavailable", errors.New("exit 1")
		}
		_, err := RetryWithPolicy(transient, hardFail, "test", fn)
		if err == nil {
			t.Fatalf("expected error after exhausting retries, got nil")
		}
		if attempts != 4 {
			t.Fatalf("attempts = %d, want 4 (default schedule)", attempts)
		}
		if len(sleeps) != 3 {
			t.Fatalf("sleeps = %v, want 3 inter-attempt waits", sleeps)
		}
	})

	t.Run("nil hard-fail regex is tolerated", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			if attempts < 2 {
				return "request timeout", errors.New("exit 1")
			}
			return "ok", nil
		}
		out, err := RetryWithPolicy(transient, nil, "test", fn)
		if err != nil || out != "ok" || attempts != 2 {
			t.Fatalf("got attempts=%d out=%q err=%v, want attempts=2 out=ok", attempts, out, err)
		}
	})

	t.Run("non-transient unrecognised error passes through without retry", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			return "some unrelated failure", errors.New("exit 1")
		}
		_, err := RetryWithPolicy(transient, hardFail, "test", fn)
		if err == nil || attempts != 1 {
			t.Fatalf("got attempts=%d err=%v, want attempts=1 err=non-nil", attempts, err)
		}
	})
}
