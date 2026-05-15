package shell

import (
	"errors"
	"testing"
	"time"
)

func TestClassifyError(t *testing.T) {
	err := errors.New("exit 1")
	cases := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		// 5xx + GitHub-flavoured transients
		{"HTTP 500", "HTTP 500 Internal Server Error", err, true},
		{"HTTP 502", "Bad Gateway (HTTP 502)", err, true},
		{"HTTP 503", "Service Unavailable\nHTTP 503", err, true},
		{"i/o timeout", "dial tcp: i/o timeout", err, true},
		{"connection reset", "read: connection reset by peer", err, true},
		{"TLS handshake", "TLS handshake failure", err, true},
		{"tls handshake lowercase", "tls: handshake failure", err, true},
		{"no such host", "dial tcp: lookup api.github.com: no such host", err, true},
		{"bad gateway text", "Bad Gateway", err, true},
		{"graphql internal error", "Something went wrong while executing your query", err, true},
		// Docker / sonarcloud / git transients picked up by the union
		{"sonar 5xx wording", "Error 504 on https://sonarcloud.io/api/...", err, true},
		{"sonar endpoint timeout", "Endpoint request timed out", err, true},
		{"docker context deadline", "context deadline exceeded", err, true},
		{"git RPC 5xx", "RPC failed; HTTP 502 curl 22", err, true},
		{"git could not resolve host", "fatal: unable to access 'https://...': Could not resolve host: github.com", err, true},
		{"http2 GOAWAY", "http2: server sent GOAWAY and closed the connection", err, true},

		// Hard-fail
		{"HTTP 404", "HTTP 404: Not Found", err, false},
		{"HTTP 403 rate limit", "HTTP 403: API rate limit exceeded", err, false},
		{"HTTP 422", "HTTP 422 Unprocessable Entity", err, false},
		{"docker manifest unknown", "manifest unknown: manifest unknown", err, false},
		{"docker name unknown", "name unknown: repository name not known to registry", err, false},
		{"sonar project not found", "Project key acme:foo does not exist", err, false},
		{"git remote rejected", "! [remote rejected] main -> main (pre-receive hook declined)", err, false},
		{"git fatal protocol", "fatal: protocol error: bad pack header", err, false},
		{"unauthorized", "unauthorized: authentication required", err, false},
		{"RateLimitExceeded typed", "", &RateLimitExceeded{Msg: "rl"}, false},
		{"unknown error", "some unrelated failure", err, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.out, tc.err)
			if got != tc.want {
				t.Fatalf("classifyError(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

// withFakeSleep swaps sleepFn for the duration of a test. Restores on cleanup.
func withFakeSleep(t *testing.T, calls *[]time.Duration) {
	t.Helper()
	orig := sleepFn
	sleepFn = func(d time.Duration) { *calls = append(*calls, d) }
	t.Cleanup(func() { sleepFn = orig })
}

func TestRunWithRetryLoop_ImmediateSuccess(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "ok", nil
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("out = %q, want %q", out, "ok")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want none on immediate success", sleeps)
	}
}

func TestRunWithRetryLoop_TransientThenSuccess(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	transient := errors.New("exit 1")
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		if attempts < 3 {
			return "HTTP 503 Service Unavailable", transient
		}
		return "ok", nil
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "ok" {
		t.Fatalf("out = %q, want %q", out, "ok")
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleeps = %v, want 2 retries before success", sleeps)
	}
	if sleeps[0] != 5*time.Second || sleeps[1] != 15*time.Second {
		t.Fatalf("sleeps = %v, want [5s 15s] per backoff schedule", sleeps)
	}
}

func TestRunWithRetryLoop_TransientExhausted(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	transient := errors.New("exit 1")
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "HTTP 500 Internal Server Error", transient
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err == nil {
		t.Fatalf("expected error after exhausting retries, got nil")
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4", attempts)
	}
	if out != "HTTP 500 Internal Server Error" {
		t.Fatalf("out = %q, want the last attempt's output", out)
	}
	// 4 attempts → 3 sleeps between them.
	if len(sleeps) != 3 {
		t.Fatalf("sleeps = %v, want 3 inter-attempt waits", sleeps)
	}
}

func TestRunWithRetryLoop_HardFailPassthrough(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	hardFail := errors.New("exit 1")
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "HTTP 404: Not Found", hardFail
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 — hard-fail must not retry", attempts)
	}
	if out != "HTTP 404: Not Found" {
		t.Fatalf("out = %q, want the single attempt's output", out)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want zero — hard-fail returns before any sleep", sleeps)
	}
}

// TestRunWithRetryLoop_PermissiveClassifier exercises the classifier shape
// MustRunPostCreate uses: retry on any non-RateLimitExceeded error, regardless
// of output wording. This is the property that makes it robust to GitHub
// changing the "Could not resolve to a Repository" message in the future.
func TestRunWithRetryLoop_PermissiveClassifier(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	permissive := func(_ string, err error) bool {
		var rle *RateLimitExceeded
		return !errors.As(err, &rle)
	}

	t.Run("retries arbitrary error wording", func(t *testing.T) {
		attempts := 0
		attempt := func() (string, error) {
			attempts++
			if attempts < 3 {
				return "totally novel vendor error message", errors.New("exit 1")
			}
			return "ok", nil
		}
		out, err := runWithRetryLoop(attempt, permissive, 4, defaultRetryDelays, "retry")
		if err != nil || out != "ok" || attempts != 3 {
			t.Fatalf("got attempts=%d out=%q err=%v, want attempts=3 out=ok err=nil", attempts, out, err)
		}
	})

	t.Run("rate limit still passes through", func(t *testing.T) {
		attempts := 0
		rlErr := &RateLimitExceeded{Msg: "rl"}
		attempt := func() (string, error) {
			attempts++
			return "", rlErr
		}
		_, err := runWithRetryLoop(attempt, permissive, 4, defaultRetryDelays, "retry")
		if attempts != 1 || err != rlErr {
			t.Fatalf("got attempts=%d err=%v, want attempts=1 err=rlErr", attempts, err)
		}
	})
}

func TestRunWithRetryLoop_RateLimitPassthrough(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	rlErr := &RateLimitExceeded{Msg: "rate limited"}
	attempts := 0
	attempt := func() (string, error) {
		attempts++
		return "", rlErr
	}
	out, err := runWithRetryLoop(attempt, classifyError, 4, defaultRetryDelays, "retry")
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 — rate-limit must not retry", attempts)
	}
	if err != rlErr {
		t.Fatalf("err = %v, want the typed RateLimitExceeded to pass through", err)
	}
	if out != "" {
		t.Fatalf("out = %q, want empty", out)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %v, want zero", sleeps)
	}
}
