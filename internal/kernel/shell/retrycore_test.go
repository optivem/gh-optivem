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

func TestRetryWithPolicy_WithSchedule(t *testing.T) {
	transient := regexp.MustCompile(`(?i)timeout`)

	t.Run("custom attempts and delays are honoured, last delay repeats", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			return "timeout", errors.New("exit 1")
		}
		_, err := RetryWithPolicy(transient, nil, "test", fn,
			WithSchedule(4, 1*time.Second, 2*time.Second))
		if err == nil || attempts != 4 {
			t.Fatalf("got attempts=%d err=%v, want attempts=4 and a final error", attempts, err)
		}
		want := []time.Duration{1 * time.Second, 2 * time.Second, 2 * time.Second}
		if len(sleeps) != len(want) {
			t.Fatalf("sleeps = %v, want %v", sleeps, want)
		}
		for i := range want {
			if sleeps[i] != want[i] {
				t.Fatalf("sleeps = %v, want %v", sleeps, want)
			}
		}
	})

	t.Run("single attempt never sleeps", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)

		attempts := 0
		fn := func() (string, error) {
			attempts++
			return "timeout", errors.New("exit 1")
		}
		_, _ = RetryWithPolicy(transient, nil, "test", fn, WithSchedule(1, time.Second))
		if attempts != 1 || len(sleeps) != 0 {
			t.Fatalf("attempts=%d sleeps=%v, want 1 attempt and no sleeps", attempts, sleeps)
		}
	})

	t.Run("malformed schedule panics instead of defaulting", func(t *testing.T) {
		for name, build := range map[string]func(){
			"zero attempts": func() { WithSchedule(0, time.Second) },
			"no delays":     func() { WithSchedule(3) },
		} {
			func() {
				defer func() {
					if recover() == nil {
						t.Errorf("%s: expected panic", name)
					}
				}()
				build()
			}()
		}
	})
}

func TestRetryWithPolicy_ForceRetryAndMessageLines(t *testing.T) {
	transient := RetryTransient()
	hardFail := RetryHardFail()
	failingFn := func(out string, attempts *int) func() (string, error) {
		return func() (string, error) {
			*attempts++
			return out, errors.New("exit 1")
		}
	}

	t.Run("force-retry beats hard-fail", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)
		n := 0
		out := "failed with HTTP 403 Forbidden: https://api.sonarcloud.io/analysis/jres"
		_, _ = RetryWithPolicy(transient, hardFail, "t", failingFn(out, &n),
			WithSchedule(3, time.Second), WithForceRetry(RetryForceRetry()))
		if n != 3 {
			t.Fatalf("attempts = %d, want 3 (force-retry must override hard-fail)", n)
		}
	})

	t.Run("without force-retry the same output hard-fails", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)
		n := 0
		out := "failed with HTTP 403 Forbidden: https://api.sonarcloud.io/analysis/jres"
		_, _ = RetryWithPolicy(transient, hardFail, "t", failingFn(out, &n), WithSchedule(3, time.Second))
		if n != 1 {
			t.Fatalf("attempts = %d, want 1", n)
		}
	})

	frames := "Error 504 on https://api.sonarcloud.io/analysis/analyses : {\"message\": \"Endpoint request timed out\"}\n" +
		"\tat org.sonar.scanner.http.DefaultScannerWsClient.failIfUnauthorized(DefaultScannerWsClient.java:87)\n" +
		"\t... 12 more\n"

	t.Run("stack-frame identifier hijacks classification without WithMessageLines", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)
		n := 0
		_, _ = RetryWithPolicy(transient, hardFail, "t", failingFn(frames, &n), WithSchedule(3, time.Second))
		if n != 1 {
			t.Fatalf("attempts = %d, want 1 (documents the trap WithMessageLines fixes)", n)
		}
	})

	t.Run("WithMessageLines retries a transient behind a failIfUnauthorized frame", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)
		n := 0
		_, _ = RetryWithPolicy(transient, hardFail, "t", failingFn(frames, &n),
			WithSchedule(3, time.Second), WithMessageLines())
		if n != 3 {
			t.Fatalf("attempts = %d, want 3", n)
		}
	})

	t.Run("WithMessageLines still hard-fails a genuine message-line 401", func(t *testing.T) {
		var sleeps []time.Duration
		withFakeSleep(t, &sleeps)
		n := 0
		out := "HttpException: Error 401 on https://sonarcloud.io/batch/project\n\tat a.b.C.d(C.java:1)\n"
		_, _ = RetryWithPolicy(transient, hardFail, "t", failingFn(out, &n),
			WithSchedule(3, time.Second), WithMessageLines())
		if n != 1 {
			t.Fatalf("attempts = %d, want 1", n)
		}
	})
}
